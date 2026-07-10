// Package relay implements `ibc relayer run`: the long-lived relay daemon. On start it loads
// the config + persisted deployment, dials every chain, brings up the daemon HTTP API (status, relay,
// health) on a dynamic free port, then prints the readiness JSON line to stdout (the harness's semantic
// readiness signal; there is no exit code for a daemon). Thereafter it discovers packets by scanning
// each route source and reconciles the pending rows — manual routes only once /relay has been asked —
// into destination effects or terminal failures.
package relay

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/link/e2e/stub/internal/cfg"
	"github.com/cosmos/ibc/link/e2e/stub/internal/cosmos"
	"github.com/cosmos/ibc/link/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/link/e2e/stub/internal/jsonout"
	"github.com/cosmos/ibc/link/e2e/stub/internal/onchain"
	"github.com/cosmos/ibc/link/e2e/stub/internal/rpcsafe"
	"github.com/cosmos/ibc/link/e2e/stub/internal/statusapi"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/internal/config"
)

const (
	// pollInterval is how often the relay loop ticks (source discovery, then reconciling pending rows to
	// destination effects). A short poll (not a subscription) keeps the stub dependency-free and is plenty
	// responsive against Anvil's instant mining. This is work polling, not a readiness sleep.
	pollInterval = 250 * time.Millisecond
	// startupDialTimeout bounds dialing + chain-id probing one chain at start, so a dead RPC fails the
	// daemon's startup promptly (exit 65) instead of hanging before it can ever signal readiness.
	startupDialTimeout = 30 * time.Second
	// shutdownTimeout bounds the graceful HTTP server drain on SIGTERM.
	shutdownTimeout = 5 * time.Second
)

// chainConn is one dialed chain: its client, the chain id reported by the node, and the relayer signing
// key parsed from this chain's configured EVMSignerKey (used to build the relayer transactor for the
// destination effect and source refund).
type chainConn struct {
	id        string
	client    *ethclient.Client
	chainID   *big.Int
	signerKey *ecdsa.PrivateKey
}

// Command builds the `relayer run` command.
func Command(flags *config.FlagSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "run",
		Short:        "run the auto-relay daemon: watch IFTSent/GMPSent on each route source and complete each on the destination",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, flags)
		},
	}
	return cmd
}

func run(cmd *cobra.Command, flags *config.FlagSet) error {
	// cmd.Context() is canceled on SIGTERM/SIGINT (main wires signal.NotifyContext); the daemon blocks
	// on it and shuts down gracefully when it fires. The harness must not route this through the
	// one-shot per-command timeout; this command is unbounded by design.
	ctx := cmd.Context()
	stderr := cmd.ErrOrStderr()

	c, err := cfg.Setup(flags)
	if err != nil {
		return err
	}
	if storeErr := cfg.RequireStore(c); storeErr != nil {
		return exitcode.New(wire.ExitConfigInvalid, storeErr)
	}

	st, err := store.Open(c.DB.URL)
	if err != nil {
		return exitcode.New(wire.ExitInternal, err)
	}
	defer st.Close() //nolint:errcheck

	dep, err := st.RequireDeployment(ctx)
	if err != nil {
		// No deployment persisted -> the relayer cannot resolve fixture addresses, so it can never reach
		// readiness. Surface that as not-ready (69) with a precise next step; any other load error is
		// internal.
		if errors.Is(err, store.ErrNoDeployment) {
			return exitcode.New(wire.ExitNotReady, fmt.Errorf("%w (db %q)", err, c.DB.URL))
		}
		return exitcode.New(wire.ExitInternal, err)
	}

	conns, cosmosConns, err := dialChains(ctx, c, stderr)
	if err != nil {
		return err // already an *exitcode.Error with the right class
	}
	defer closeConns(conns)
	defer closeCosmosConns(cosmosConns)

	// The relayer state is built before the HTTP server so the /relay handler can trigger an on-demand
	// source discovery of the named tx (a manual relay must not depend on a periodic tick having run).
	rel := &relayer{
		cfg:        c,
		dep:        dep,
		conns:      conns,
		cosmos:     cosmosConns,
		store:      st,
		log:        stderr,
		recvCursor: map[string]uint64{},
		recvSeen:   map[receivedKey]onchain.ReceivedResult{},
		sentCursor: map[string]uint64{},
	}

	// Status HTTP server on a dynamic free port (never fixed): bind first so the address is known for
	// the readiness line, start serving, then announce readiness.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return exitcode.New(wire.ExitInternal, fmt.Errorf("listen status http: %w", err))
	}
	srv := &http.Server{Handler: statusapi.Handler(st, c, rel.discoverSourceTx)}
	go func() {
		// ErrServerClosed is the expected result of the graceful Shutdown below.
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			_, _ = fmt.Fprintf(stderr, "ibc relayer: status http server: %v\n", serveErr)
		}
	}()

	connectedIDs := make([]string, 0, len(conns)+len(cosmosConns))
	for _, ch := range c.Chains { // config order, for deterministic output
		if _, ok := conns[ch.ID]; ok {
			connectedIDs = append(connectedIDs, ch.ID)
		} else if _, ok := cosmosConns[ch.ID]; ok {
			connectedIDs = append(connectedIDs, ch.ID)
		}
	}

	// First stdout line = readiness JSON. Every boolean is a precondition the daemon has satisfied.
	readiness := wire.Readiness{
		Event:             wire.ReadinessEvent,
		ConfigLoaded:      true,
		DBReady:           true,
		ChainsConnected:   connectedIDs,
		RelayerSubscribed: true, // the poll loop (below) is the subscription
		Status:            wire.ReadinessStatus{HTTP: ln.Addr().String()},
	}
	if err := jsonout.Write(cmd.OutOrStdout(), readiness); err != nil {
		return exitcode.New(wire.ExitInternal, fmt.Errorf("write readiness: %w", err))
	}
	_, _ = fmt.Fprintf(
		stderr,
		"ibc relayer ready: status http %s, chains %v, routes %d\n",
		ln.Addr().String(),
		connectedIDs,
		len(c.Relayer.Routes),
	)

	// Run the relay loop until SIGTERM cancels ctx.
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		rel.loop(ctx)
	}()

	<-ctx.Done()
	_, _ = fmt.Fprintln(stderr, "ibc relayer: shutdown signal received, draining")

	// Graceful shutdown: stop accepting status requests, then wait for the loop to unwind. Use a fresh
	// context because ctx is already canceled.
	shCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(shCtx)
	<-loopDone
	return nil // SIGTERM -> exit 0
}

// dialChains connects to every configured chain, dispatching on family: an EVM chain is dialed and its
// chain id probed (a liveness check + the value signing needs); a cosmos chain is "connected" once its
// CometBFT /status answers. Chains are connected CONCURRENTLY (each bounded by its own
// startupDialTimeout), so total startup is ~one dial budget regardless of chain count — keeping it well
// inside the harness's fixed readiness wait. A failure to reach any chain means the daemon can't become
// ready, so it returns an ExitRPCUnreachable error with the offending endpoint redacted (the first failure
// wins; errgroup cancels the rest). It returns the EVM and cosmos conns separately, keyed by chain id.
func dialChains(
	ctx context.Context,
	c *wire.ConfigYAML,
	stderr io.Writer,
) (map[string]*chainConn, map[string]*cosmos.Client, error) {
	var (
		mu          sync.Mutex
		conns       = make(map[string]*chainConn, len(c.Chains))
		cosmosConns = make(map[string]*cosmos.Client, len(c.Chains))
	)
	g, gctx := errgroup.WithContext(ctx)
	for _, ch := range c.Chains {
		g.Go(func() error {
			dialCtx, cancel := context.WithTimeout(gctx, startupDialTimeout)
			defer cancel()
			switch ch.Type {
			case wire.ChainTypeCosmos:
				// A cosmos chain is "connected" once its CometBFT /status answers (the cosmos analog of an
				// eth chain-id probe). The client derives its escrow signer from the config's signer key.
				client, err := cosmos.Connect(dialCtx, ch.RPC.URL, ch.GRPCURL, ch.CosmosChainID, ch.SignerKey)
				if err != nil {
					return exitcode.New(
						wire.ExitRPCUnreachable,
						fmt.Errorf("connect chain %s: %s", ch.ID, rpcsafe.RedactURLs(err.Error())),
					)
				}
				mu.Lock()
				cosmosConns[ch.ID] = client
				mu.Unlock()
				_, _ = fmt.Fprintf(stderr, "ibc relayer: connected cosmos chain %s (%s)\n", ch.ID, ch.CosmosChainID)
				return nil
			default:
				// The relayer signs this EVM chain's effects from its config-declared EVMSignerKey; a
				// missing/invalid key is a config fault (the daemon could never deliver), not an unreachable RPC.
				signerKey, keyErr := onchain.ParseKey(ch.EVMSignerKey)
				if keyErr != nil {
					return exitcode.New(
						wire.ExitConfigInvalid,
						fmt.Errorf("chain %s: %w", ch.ID, keyErr),
					)
				}
				conn, err := onchain.Connect(dialCtx, ch.RPC.URL)
				if err != nil {
					return exitcode.New(
						wire.ExitRPCUnreachable,
						fmt.Errorf("connect chain %s: %s", ch.ID, rpcsafe.RedactURLs(err.Error())),
					)
				}
				mu.Lock()
				conns[ch.ID] = &chainConn{id: ch.ID, client: conn.Client, chainID: conn.ChainID, signerKey: signerKey}
				mu.Unlock()
				_, _ = fmt.Fprintf(stderr, "ibc relayer: connected chain %s (id %s)\n", ch.ID, conn.ChainID)
				return nil
			}
		})
	}
	if err := g.Wait(); err != nil {
		// g.Wait joined all goroutines, so both maps are safe to range without the lock.
		closeConns(conns)
		closeCosmosConns(cosmosConns)
		return nil, nil, err
	}
	return conns, cosmosConns, nil
}

func closeConns(conns map[string]*chainConn) {
	for _, c := range conns {
		c.client.Close()
	}
}

// closeCosmosConns releases each cosmos chain's gRPC conn (the daemon holds one per cosmos chain for its
// lifetime; the CometBFT RPC client needs no close). Symmetric with closeConns for the EVM conns.
func closeCosmosConns(conns map[string]*cosmos.Client) {
	for _, c := range conns {
		_ = c.Close()
	}
}
