// SPDX-License-Identifier: Apache-2.0

package prover

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/relay/prover/remote"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
)

func TestProbeRemoteDeadlines(t *testing.T) {
	for _, tt := range []struct {
		name             string
		requireReachable bool
		parentTimeout    time.Duration
		wantTimeout      time.Duration
	}{
		{name: "startup", wantTimeout: 5 * time.Second},
		{name: "live validation", requireReachable: true, wantTimeout: time.Minute},
		{name: "caller deadline", requireReachable: true, parentTimeout: time.Second, wantTimeout: time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.parentTimeout != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.parentTimeout)
				defer cancel()
			}
			called := false
			client := &http.Client{Transport: probeTransport(func(req *http.Request) (*http.Response, error) {
				called = true
				deadline, ok := req.Context().Deadline()
				require.True(t, ok)
				require.InDelta(t, tt.wantTimeout.Seconds(), time.Until(deadline).Seconds(), 0.5)
				return nil, errors.New("probe transport stopped")
			})}
			p := remote.New(client, "https://prover.example.com", "1", "client-0", slog.Default())
			require.ErrorContains(t, probeRemote(ctx, p, tt.requireReachable), "probe transport stopped")
			require.True(t, called)
		})
	}
}

type probeTransport func(*http.Request) (*http.Response, error)

func (f probeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testConnection() config.ConnectionConfig {
	return config.ConnectionConfig{
		Alias: "eth-base",
		ClientA: config.ClientEnd{
			ChainID:  "1",
			Signer:   "relayer-key",
			ClientID: "base-0",
			Type:     config.ClientTypeAttestation,
		},
		ClientB: config.ClientEnd{
			ChainID:  "8453",
			Signer:   "relayer-key",
			ClientID: "ethereum-0",
			Type:     config.ClientTypeAttestation,
		},
	}
}

// localCandidate builds a mock Attestor registered under the Service.
func localCandidate(t *testing.T, alias, watchedChainID, address string) attestor.Attestor {
	t.Helper()

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(alias).Maybe()
	a.EXPECT().ChainID().Return(watchedChainID).Maybe()
	a.EXPECT().Address().Return(address).Maybe()

	return a
}

// testConfig builds a config, matching *ClientSet, and candidate list whose
// connection is trivially satisfiable, isolating dispatch-level coverage
// from the attestor-matching specifics (covered in
// internal/relay/prover/attestation).
func testConfig(t *testing.T) (config.Config, *chains.ClientSet, []attestor.Attestor) {
	t.Helper()

	conn := testConnection()

	chainA := mocks.NewMockClient(t)
	chainA.EXPECT().
		GetAttestationSet(context.Background(), conn.ClientA.ClientID).
		Return([]string{"0xaaa"}, uint8(1), nil)

	chainB := mocks.NewMockClient(t)
	chainB.EXPECT().
		GetAttestationSet(context.Background(), conn.ClientB.ClientID).
		Return([]string{"0xbbb"}, uint8(1), nil)

	clientSet := chains.NewClientSet(map[string]chains.Client{
		conn.ClientA.ChainID: chainA,
		conn.ClientB.ChainID: chainB,
	})

	watchesB := localCandidate(t, "watches-b", conn.ClientB.ChainID, "0xaaa")
	watchesA := localCandidate(t, "watches-a", conn.ClientA.ChainID, "0xbbb")

	cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

	return cfg, clientSet, []attestor.Attestor{watchesB, watchesA}
}

func TestNewSetFromConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("resolvesBothDirections", func(t *testing.T) {
		// proves forEachClientEnd/addGenerator wiring: both the connection's
		// client ends land in the returned Set under their own key.
		cfg, clientSet, attestors := testConfig(t)
		conn := cfg.Relayer.Connections[0]

		set, err := NewSetFromConfig(ctx, cfg, clientSet, attestors, slog.Default(), SetOptions{})
		require.NoError(t, err)

		_, ok := set.Get(conn.ClientA.ChainID, conn.ClientA.ClientID)
		require.True(t, ok)

		_, ok = set.Get(conn.ClientB.ChainID, conn.ClientB.ClientID)
		require.True(t, ok)
	})

	t.Run("unreachableRemoteProverIsNotFatal", func(t *testing.T) {
		// A remote prover that is briefly down must not block relayer boot:
		// the probe warns and the client end still resolves, matching how an
		// unresolvable attestor is skipped rather than failing startup.
		conn := testConnection()
		// Both ends remote, so no attestation generator is resolved and the
		// chain clients are not consulted.
		// 127.0.0.1:1 refuses immediately, so this does not wait on a timeout.
		conn.ClientA.Type = config.ClientTypeRemote
		conn.ClientA.Params = []byte(`{"url":"http://127.0.0.1:1"}`)
		conn.ClientB.Type = config.ClientTypeRemote
		conn.ClientB.Params = []byte(`{"url":"http://127.0.0.1:1"}`)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: mocks.NewMockClient(t),
			conn.ClientB.ChainID: mocks.NewMockClient(t),
		})

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		set, err := NewSetFromConfig(ctx, cfg, clientSet, nil, slog.Default(), SetOptions{})
		require.NoError(t, err, "an unreachable remote prover must not fail the set")

		_, ok := set.Get(conn.ClientA.ChainID, conn.ClientA.ClientID)
		require.True(t, ok, "the unreachable prover is still registered")
	})

	t.Run("unresolvableRemoteProverTLSErrors", func(t *testing.T) {
		// A bad tls block is a config error, not a transient outage, so it
		// still fails rather than warning.
		conn := testConnection()
		conn.ClientA.Type = config.ClientTypeRemote
		conn.ClientA.Params = []byte(
			`{"url":"https://127.0.0.1:1","tls":{"caFile":"/nonexistent/ca.crt"}}`,
		)
		conn.ClientB.Type = config.ClientTypeRemote
		conn.ClientB.Params = []byte(`{"url":"http://127.0.0.1:1"}`)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: mocks.NewMockClient(t),
			conn.ClientB.ChainID: mocks.NewMockClient(t),
		})

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil, slog.Default(), SetOptions{})
		require.ErrorContains(t, err, "tls")
	})

	t.Run("unsupportedClientTypeErrors", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()
		conn.ClientA.Type = "tendermint"
		conn.ClientB.Type = "tendermint"

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: mocks.NewMockClient(t),
			conn.ClientB.ChainID: mocks.NewMockClient(t),
		})

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT
		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil, slog.Default(), SetOptions{})

		// ASSERT
		require.ErrorContains(t, err, `unsupported client type "tendermint"`)
	})
}

func TestNewSetFromConfigBesuQBFT(t *testing.T) {
	ctx := context.Background()
	conn := config.ConnectionConfig{
		Alias:   "besu-a-besu-b",
		ClientA: config.ClientEnd{ChainID: "1", Signer: "relayer", ClientID: "besu-b", Type: config.ClientTypeBesuQBFT},
		ClientB: config.ClientEnd{ChainID: "2", Signer: "relayer", ClientID: "besu-a", Type: config.ClientTypeBesuQBFT},
	}
	cfg := config.Config{
		Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
	}

	t.Run("requires EVM clients", func(t *testing.T) {
		clientSet := chains.NewClientSet(map[string]chains.Client{
			"1": mocks.NewMockClient(t),
			"2": mocks.NewMockClient(t),
		})
		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil, slog.Default())
		require.ErrorContains(t, err, "is not an EVM chain")
	})

	t.Run("missing chain client", func(t *testing.T) {
		_, err := NewSetFromConfig(ctx, cfg, chains.NewClientSet(nil), nil, slog.Default())
		require.ErrorContains(t, err, `no client for chain "1"`)
	})
}
