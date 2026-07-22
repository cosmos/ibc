package ibcrelay

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"

	internalconfig "github.com/cosmos/ibc/link/internal/config"
)

const (
	pollInterval       = 250 * time.Millisecond
	startupDialTimeout = 30 * time.Second
	shutdownTimeout    = 5 * time.Second
)

// RelayerRun returns the IBC Relayer handler.
func RelayerRun(flags *internalconfig.FlagSet) relayercmd.Handler {
	return func(cmd *cobra.Command, _ []string) error {
		return runRelayer(cmd, flags)
	}
}

func runRelayer(cmd *cobra.Command, flags *internalconfig.FlagSet) error {
	// cmd.Context() is canceled on SIGTERM; shutdown drains HTTP then waits for the relay loop.
	ctx := cmd.Context()
	stderr := cmd.ErrOrStderr()

	c, err := setupConfig(flags)
	if err != nil {
		return err
	}
	if errs := checkConfig(c); len(errs) > 0 {
		return fmt.Errorf("config invalid: %d structural error(s)", len(errs))
	}
	signerKeys, err := relaySignerKeys(c)
	if err != nil {
		return err
	}

	st, err := openStore(c.DB.URL)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck

	conns, err := dialChains(ctx, c, signerKeys, stderr)
	if err != nil {
		return err
	}
	defer closeConns(conns)

	rel := &relayer{
		cfg:        c,
		conns:      conns,
		store:      st,
		log:        stderr,
		sentCursor: map[string]uint64{},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen status http: %w", err)
	}
	srv := &http.Server{Handler: statusHandler(st, c, rel.discoverSourceTx)}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			_, _ = fmt.Fprintf(stderr, "ibc relayer: status http server: %v\n", serveErr)
		}
	}()

	connectedIDs := make([]string, 0, len(c.Chains))
	for _, ch := range c.Chains {
		connectedIDs = append(connectedIDs, ch.ID)
	}

	// First stdout line is readiness JSON (relayercmd.Readiness); harness blocks on it before HTTP probes.
	readiness := relayercmd.Readiness{
		Event:             relayercmd.ReadinessEvent,
		ConfigLoaded:      true,
		DBReady:           true,
		ChainsConnected:   connectedIDs,
		RelayerSubscribed: true,
		Status:            relayercmd.ReadinessStatus{HTTP: ln.Addr().String()},
	}
	if err := writeJSON(cmd.OutOrStdout(), readiness); err != nil {
		return fmt.Errorf("write readiness: %w", err)
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
	c *configcmd.Config,
	signerKeys map[string]*ecdsa.PrivateKey,
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
			conn, err := connectChain(dialCtx, ch.RPC.URL)
			if err != nil {
				return fmt.Errorf("connect chain %s: %w", ch.ID, err)
			}
			cc := &chainConn{
				id:         ch.ID,
				client:     conn.Client,
				chainID:    conn.ChainID,
				signerKey:  signerKeys[ch.ID],
				routerAddr: common.HexToAddress(ch.ICS26Router),
			}
			ops, err := newEVMOps(cc)
			if err != nil {
				conn.Client.Close()
				return err
			}
			cc.ops = ops
			mu.Lock()
			conns[ch.ID] = cc
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

func relaySignerKeys(c *configcmd.Config) (map[string]*ecdsa.PrivateKey, error) {
	endpoints := make(map[string]struct{}, len(c.Relayer.Routes)*2)
	for _, route := range c.Relayer.Routes {
		endpoints[route.Source] = struct{}{}
		endpoints[route.Destination] = struct{}{}
	}

	keys := make(map[string]*ecdsa.PrivateKey, len(endpoints))
	for i, ch := range c.Chains {
		if _, required := endpoints[ch.ID]; !required || ch.Type != configcmd.ChainTypeEVM {
			continue
		}
		path := fmt.Sprintf("chains[%d].evmSigner", i)
		if ch.EVMSigner == "" {
			return nil, fmt.Errorf("%s: EVM relay signer alias is empty", path)
		}
		key, err := loadECDSA(c.Signers, ch.EVMSigner)
		if err != nil {
			return nil, fmt.Errorf("%s: EVM relay signer %q: %w", path, ch.EVMSigner, err)
		}
		keys[ch.ID] = key
	}
	return keys, nil
}

func closeConns(conns map[string]*chainConn) {
	for _, c := range conns {
		if c.client != nil {
			c.client.Close()
		}
	}
}
