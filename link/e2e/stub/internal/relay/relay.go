// Package relay implements `ibc relayer run`; first stdout line is readiness JSON the harness parses.
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
	pollInterval       = 250 * time.Millisecond
	startupDialTimeout = 30 * time.Second
	shutdownTimeout    = 5 * time.Second
)

type chainConn struct {
	id        string
	client    *ethclient.Client
	chainID   *big.Int
	signerKey *ecdsa.PrivateKey
}

func Command(flags *config.FlagSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "run",
		Short:        "run the relay daemon: scan route sources and complete packets on destinations",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, flags)
		},
	}
	return cmd
}

func run(cmd *cobra.Command, flags *config.FlagSet) error {
	// cmd.Context() is canceled on SIGTERM; shutdown drains HTTP then waits for the relay loop.
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
		if errors.Is(err, store.ErrNoDeployment) {
			return exitcode.New(wire.ExitNotReady, fmt.Errorf("%w (db %q)", err, c.DB.URL))
		}
		return exitcode.New(wire.ExitInternal, err)
	}

	conns, err := dialChains(ctx, c, stderr)
	if err != nil {
		return err
	}
	defer closeConns(conns)

	rel := &relayer{
		cfg:        c,
		dep:        dep,
		conns:      conns,
		store:      st,
		log:        stderr,
		recvCursor: map[string]uint64{},
		recvSeen:   map[receivedKey]onchain.ReceivedResult{},
		sentCursor: map[string]uint64{},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return exitcode.New(wire.ExitInternal, fmt.Errorf("listen status http: %w", err))
	}
	srv := &http.Server{Handler: statusapi.Handler(st, c, rel.discoverSourceTx)}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			_, _ = fmt.Fprintf(stderr, "ibc relayer: status http server: %v\n", serveErr)
		}
	}()

	connectedIDs := make([]string, 0, len(conns))
	for _, ch := range c.Chains {
		if _, ok := conns[ch.ID]; ok {
			connectedIDs = append(connectedIDs, ch.ID)
		}
	}

	// First stdout line is readiness JSON (wire.Readiness); harness blocks on it before HTTP probes.
	readiness := wire.Readiness{
		Event:             wire.ReadinessEvent,
		ConfigLoaded:      true,
		DBReady:           true,
		ChainsConnected:   connectedIDs,
		RelayerSubscribed: true,
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

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		rel.loop(ctx)
	}()

	<-ctx.Done()
	_, _ = fmt.Fprintln(stderr, "ibc relayer: shutdown signal received, draining")

	// ctx is already canceled; use a fresh timeout for HTTP drain before waiting on the loop.
	shCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(shCtx)
	<-loopDone
	return nil
}

func dialChains(
	ctx context.Context,
	c *wire.ConfigYAML,
	stderr io.Writer,
) (map[string]*chainConn, error) {
	var (
		mu    sync.Mutex
		conns = make(map[string]*chainConn, len(c.Chains))
	)
	g, gctx := errgroup.WithContext(ctx)
	for _, ch := range c.Chains {
		g.Go(func() error {
			dialCtx, cancel := context.WithTimeout(gctx, startupDialTimeout)
			defer cancel()
			signerKey, keyErr := onchain.ParseKey(ch.EVMSignerKey)
			if keyErr != nil {
				return exitcode.New(wire.ExitConfigInvalid, fmt.Errorf("chain %s: %w", ch.ID, keyErr))
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
		})
	}
	if err := g.Wait(); err != nil {
		closeConns(conns)
		return nil, err
	}
	return conns, nil
}

func closeConns(conns map[string]*chainConn) {
	for _, c := range conns {
		c.client.Close()
	}
}
