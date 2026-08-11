package attestation

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
func localCandidate(t *testing.T, alias, watchedChainID, address string) attestor.Attestor {
	t.Helper()

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(alias).Maybe()
	a.EXPECT().Alias().Return(alias).Maybe()
	a.EXPECT().ChainID().Return(watchedChainID).Maybe()
	a.EXPECT().Address().Return(address).Maybe()

	return a
}

func TestResolveGenerator(t *testing.T) {
	ctx := context.Background()

	t.Run("quorumSatisfiedByLocalAttestor", func(t *testing.T) {
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: selfChain,
			conn.ClientB.ChainID: mocks.NewMockClient(t),
		})

		candidate := localCandidate(t, "watches-b", conn.ClientB.ChainID, "0xAAA")

		gen, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, []attestor.Attestor{candidate})

		require.NoError(t, err, "address match is case-insensitive")
		require.NotNil(t, gen)
	})

	t.Run("insufficientMatchingAttestorsErrors", func(t *testing.T) {
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().
			GetAttestationSet(ctx, conn.ClientA.ClientID).
			Return([]string{"0xaaa", "0xbbb"}, uint8(2), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		candidate := localCandidate(t, "watcher", conn.ClientB.ChainID, "0xaaa")

		_, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, []attestor.Attestor{candidate})

		require.ErrorContains(t, err, `only 1 reachable/matching attestors for chain "8453"`)
		require.ErrorContains(t, err, "on-chain quorum requires 2")
		require.ErrorContains(t, err, "on-chain addresses: [0xaaa, 0xbbb]")
		require.ErrorContains(t, err, "configured attestors: [watcher=0xaaa]")
	})

	t.Run("duplicateAttestorAddressErrors", func(t *testing.T) {
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(2), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		// same address (case-insensitive), configured under two different names
		first := localCandidate(t, "watcher-1", conn.ClientB.ChainID, "0xaaa")
		second := localCandidate(t, "watcher-2", conn.ClientB.ChainID, "0xAAA")

		_, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, []attestor.Attestor{first, second})

		require.ErrorContains(t, err, `attestors "watcher-1" and "watcher-2" share address "0xAAA"`)
	})

	t.Run("nonMatchingAddressExcluded", func(t *testing.T) {
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		// address not registered on-chain
		candidate := localCandidate(t, "watcher", conn.ClientB.ChainID, "0xdeadbeef")

		_, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, []attestor.Attestor{candidate})

		require.ErrorContains(t, err, "only 0 reachable/matching attestors")
	})

	t.Run("wrongChainExcluded", func(t *testing.T) {
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		// wrong chain, despite matching address
		candidate := localCandidate(t, "watcher", "some-other-chain", "0xaaa")

		_, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, []attestor.Attestor{candidate})

		require.ErrorContains(t, err, "only 0 reachable/matching attestors")
	})

	t.Run("selfChainClientMissingErrors", func(t *testing.T) {
		conn := testConnection()
		clientSet := chains.NewClientSet(nil)

		_, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, nil)

		require.ErrorContains(t, err, `no configured chain client for "1"`)
	})

	t.Run("counterpartyChainClientMissingErrors", func(t *testing.T) {
		conn := testConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(mock.Anything, conn.ClientA.ClientID).Return(nil, uint8(0), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		_, err := ResolveGenerator(ctx, conn.ClientA, conn.ClientB, clientSet, nil)

		require.ErrorContains(t, err, `no configured chain client for counterparty chain "8453"`)
	})
}
