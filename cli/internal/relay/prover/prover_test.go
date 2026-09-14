// SPDX-License-Identifier: Apache-2.0

package prover

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

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

		set, err := NewSetFromConfig(ctx, cfg, clientSet, attestors, slog.Default())
		require.NoError(t, err)

		_, ok := set.Get(conn.ClientA.ChainID, conn.ClientA.ClientID)
		require.True(t, ok)

		_, ok = set.Get(conn.ClientB.ChainID, conn.ClientB.ClientID)
		require.True(t, ok)
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
		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil, slog.Default())

		// ASSERT
		require.ErrorContains(t, err, `unsupported client type "tendermint"`)
	})
}

// A besu-qbft end resolves against the counterparty's configured router and
// warms its trusted consensus state from the counterparty chain.
func TestNewSetFromConfigBesuQBFT(t *testing.T) {
	ctx := context.Background()
	fixture := besutest.MustFixture(t)
	anchor := fixture.NonAdjacentUpdate
	anchorState := anchor.ExpectedConsensusState()

	anchorHash, err := anchorState.Hash()
	require.NoError(t, err)

	routerA := "0x00000000000000000000000000000000000000aa"
	routerB := fixture.RouterAddress.Hex()

	conn := config.ConnectionConfig{
		Alias:   "besu-a-besu-b",
		ClientA: config.ClientEnd{ChainID: "1", Signer: "relayer", ClientID: "besu-b", Type: config.ClientTypeBesuQBFT},
		ClientB: config.ClientEnd{ChainID: "2", Signer: "relayer", ClientID: "besu-a", Type: config.ClientTypeBesuQBFT},
	}
	cfg := config.Config{
		Chains: config.Chains{
			{ChainID: "1", EVM: &config.EVMChainConfig{RPC: "http://a", ICS26Router: routerA}},
			{ChainID: "2", EVM: &config.EVMChainConfig{RPC: "http://b", ICS26Router: routerB}},
		},
		Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
	}

	chainA := mocks.NewMockClient(t)
	chainB := mocks.NewMockClient(t)
	accountNodes, err := anchor.AccountProofNodes()
	require.NoError(t, err)

	// the client on chain 1 tracks chain 2's router; the one on chain 2 tracks chain 1's
	for _, end := range []struct {
		host, counterparty *mocks.MockClient
		clientID, router   string
	}{
		{chainA, chainB, "besu-b", routerB},
		{chainB, chainA, "besu-a", routerA},
	} {
		end.host.EXPECT().GetBesuQBFTClientState(mock.Anything, end.clientID).Return(besu.ClientState{
			IBCRouter: common.HexToAddress(end.router), LatestHeight: anchor.Height, MaxClockDrift: 15,
		}, nil).Once()
		end.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, end.clientID, anchor.Height).
			Return(anchorHash, nil).Once()
		end.counterparty.EXPECT().
			GetHeaderRLP(mock.Anything, anchor.Height).
			Return([]byte(anchor.HeaderRLP), nil).
			Once()
		end.counterparty.EXPECT().GetRouterProof(mock.Anything, anchor.Height, mock.Anything).
			Return(v2.AccountProof{StorageRoot: anchor.ExpectedStorageRoot, AccountProof: accountNodes}, nil).Once()
	}

	clientSet := chains.NewClientSet(map[string]chains.Client{"1": chainA, "2": chainB})

	set, err := NewSetFromConfig(ctx, cfg, clientSet, nil, slog.Default())
	require.NoError(t, err)

	_, ok := set.Get("1", "besu-b")
	require.True(t, ok)
	_, ok = set.Get("2", "besu-a")
	require.True(t, ok)

	t.Run("missing counterparty chain config", func(t *testing.T) {
		broken := cfg
		broken.Chains = config.Chains{cfg.Chains[0]}

		_, err := NewSetFromConfig(ctx, broken, chains.NewClientSet(nil), nil, slog.Default())
		require.ErrorContains(t, err, "no EVM chain config")
	})
}
