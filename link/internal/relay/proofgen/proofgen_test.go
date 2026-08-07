package proofgen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
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
func localCandidate(t *testing.T, alias, watchedChainID, address string, finalityOffset uint64) attestor.Attestor {
	t.Helper()

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Alias().Return(alias).Maybe()
	a.EXPECT().ChainID().Return(watchedChainID).Maybe()
	a.EXPECT().Address().Return(address).Maybe()
	a.EXPECT().FinalityOffset().Return(finalityOffset).Maybe()

	return a
}

func TestNewSetFromConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("quorumSatisfiedByLocalAttestor", func(t *testing.T) {
		// ARRANGE -- both directions need a matching candidate to succeed.
		conn := testConnection()

		chainA := mocks.NewMockClient(t)
		chainA.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil)

		chainB := mocks.NewMockClient(t)
		chainB.EXPECT().GetAttestationSet(ctx, conn.ClientB.ClientID).Return([]string{"0xbbb"}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: chainA,
			conn.ClientB.ChainID: chainB,
		})

		// authorized for chainA's generator
		watchesB := localCandidate(t, "watches-b", conn.ClientB.ChainID, "0xAAA", 3)
		// authorized for chainB's generator
		watchesA := localCandidate(t, "watches-a", conn.ClientA.ChainID, "0xBBB", 7)

		localAttestors, err := attestor.New([]attestor.Attestor{watchesB, watchesA})
		require.NoError(t, err)

		cfg := config.Config{
			Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Attestors: config.Attestors{
				{Name: "watches-b", Type: config.AttestorTypeLocal, ChainID: conn.ClientB.ChainID, Signer: "s"},
				{Name: "watches-a", Type: config.AttestorTypeLocal, ChainID: conn.ClientA.ChainID, Signer: "s"},
			},
		}

		// ACT
		set, err := NewSetFromConfig(ctx, cfg, clientSet, localAttestors)

		// ASSERT
		require.NoError(t, err)

		genA, ok := set.Get(conn.ClientA.ChainID, conn.ClientA.ClientID)
		require.True(t, ok)
		require.Equal(t, uint64(3), genA.FinalityOffset(), "address match is case-insensitive")

		genB, ok := set.Get(conn.ClientB.ChainID, conn.ClientB.ClientID)
		require.True(t, ok)
		require.Equal(t, uint64(7), genB.FinalityOffset())
	})

	t.Run("insufficientMatchingAttestorsErrors", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa", "0xbbb"}, uint8(2), nil).Maybe()

		counterpartyChain := mocks.NewMockClient(t)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: selfChain,
			conn.ClientB.ChainID: counterpartyChain,
		})

		candidate := localCandidate(t, "watcher", conn.ClientB.ChainID, "0xaaa", 0)
		localAttestors, err := attestor.New([]attestor.Attestor{candidate})
		require.NoError(t, err)

		cfg := config.Config{
			Relayer:   config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Attestors: config.Attestors{{Name: "watcher", Type: config.AttestorTypeLocal, ChainID: conn.ClientB.ChainID, Signer: "s"}},
		}

		// ACT
		_, err = NewSetFromConfig(ctx, cfg, clientSet, localAttestors)

		// ASSERT
		require.ErrorContains(t, err, `only 1 reachable/matching attestors for chain "8453"`)
		require.ErrorContains(t, err, "on-chain quorum requires 2")
	})

	t.Run("nonMatchingAddressExcluded", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil).Maybe()

		counterpartyChain := mocks.NewMockClient(t)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: selfChain,
			conn.ClientB.ChainID: counterpartyChain,
		})

		// address not registered on-chain
		candidate := localCandidate(t, "watcher", conn.ClientB.ChainID, "0xdeadbeef", 0)
		localAttestors, err := attestor.New([]attestor.Attestor{candidate})
		require.NoError(t, err)

		cfg := config.Config{
			Relayer:   config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Attestors: config.Attestors{{Name: "watcher", Type: config.AttestorTypeLocal, ChainID: conn.ClientB.ChainID, Signer: "s"}},
		}

		// ACT
		_, err = NewSetFromConfig(ctx, cfg, clientSet, localAttestors)

		// ASSERT
		require.ErrorContains(t, err, "only 0 reachable/matching attestors")
	})

	t.Run("wrongChainExcluded", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil).Maybe()

		counterpartyChain := mocks.NewMockClient(t)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: selfChain,
			conn.ClientB.ChainID: counterpartyChain,
		})

		// wrong chain, despite matching address
		candidate := localCandidate(t, "watcher", "some-other-chain", "0xaaa", 0)
		localAttestors, err := attestor.New([]attestor.Attestor{candidate})
		require.NoError(t, err)

		cfg := config.Config{
			Relayer:   config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Attestors: config.Attestors{{Name: "watcher", Type: config.AttestorTypeLocal, ChainID: "some-other-chain", Signer: "s"}},
		}

		// ACT
		_, err = NewSetFromConfig(ctx, cfg, clientSet, localAttestors)

		// ASSERT
		require.ErrorContains(t, err, "only 0 reachable/matching attestors")
	})

	t.Run("unresolvableAttestorSkippedNotFatal", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		chainA := mocks.NewMockClient(t)
		chainA.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil)

		chainB := mocks.NewMockClient(t)
		chainB.EXPECT().GetAttestationSet(ctx, conn.ClientB.ClientID).Return([]string{"0xbbb"}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: chainA,
			conn.ClientB.ChainID: chainB,
		})

		watchesB := localCandidate(t, "watches-b", conn.ClientB.ChainID, "0xaaa", 0)
		watchesA := localCandidate(t, "watches-a", conn.ClientA.ChainID, "0xbbb", 0)

		localAttestors, err := attestor.New([]attestor.Attestor{watchesB, watchesA})
		require.NoError(t, err)

		cfg := config.Config{
			Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Attestors: config.Attestors{
				// no matching entry in the running Service
				{Name: "ghost", Type: config.AttestorTypeLocal, ChainID: conn.ClientB.ChainID, Signer: "s"},
				{Name: "watches-b", Type: config.AttestorTypeLocal, ChainID: conn.ClientB.ChainID, Signer: "s"},
				{Name: "watches-a", Type: config.AttestorTypeLocal, ChainID: conn.ClientA.ChainID, Signer: "s"},
			},
		}

		// ACT
		set, err := NewSetFromConfig(ctx, cfg, clientSet, localAttestors)

		// ASSERT
		require.NoError(t, err)

		_, ok := set.Get(conn.ClientA.ChainID, conn.ClientA.ClientID)
		require.True(t, ok)
	})

	t.Run("selfChainClientMissingErrors", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()
		clientSet := chains.NewClientSet(nil)

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT
		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil)

		// ASSERT
		require.ErrorContains(t, err, `no configured chain client for "1"`)
	})

	t.Run("counterpartyChainClientMissingErrors", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(mock.Anything, conn.ClientA.ClientID).Return(nil, uint8(0), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT
		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil)

		// ASSERT
		require.ErrorContains(t, err, `no configured chain client for counterparty chain "8453"`)
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
		_, err := NewSetFromConfig(ctx, cfg, clientSet, nil)

		// ASSERT
		require.ErrorContains(t, err, `unsupported client type "tendermint"`)
	})
}
