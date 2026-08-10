package livevalidate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

func attestationConnection() config.ConnectionConfig {
	conn := testConnection()
	conn.ClientA.Type = config.ClientTypeAttestation
	conn.ClientB.Type = config.ClientTypeAttestation

	return conn
}

// quorumCandidate builds a mock Attestor identified by watchedChainID/address.
func quorumCandidate(t *testing.T, watchedChainID, address string) attestor.Attestor {
	t.Helper()

	a := attestor.NewMockAttestor(t)
	a.EXPECT().ChainID().Return(watchedChainID).Maybe()
	a.EXPECT().Address().Return(address).Maybe()

	return a
}

func TestCheckAttestorQuorum(t *testing.T) {
	ctx := context.Background()

	t.Run("resolvesBothDirections", func(t *testing.T) {
		conn := attestationConnection()

		chainA := mocks.NewMockClient(t)
		chainA.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa"}, uint8(1), nil)

		chainB := mocks.NewMockClient(t)
		chainB.EXPECT().GetAttestationSet(ctx, conn.ClientB.ClientID).Return([]string{"0xbbb"}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: chainA,
			conn.ClientB.ChainID: chainB,
		})

		watchesB := quorumCandidate(t, conn.ClientB.ChainID, "0xaaa")
		watchesA := quorumCandidate(t, conn.ClientA.ChainID, "0xbbb")

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		require.NoError(t, checkAttestorQuorum(ctx, cfg, clientSet, []attestor.Attestor{watchesB, watchesA}))
	})

	t.Run("insufficientMatchingAttestorsErrors", func(t *testing.T) {
		conn := attestationConnection()

		selfChain := mocks.NewMockClient(t)
		selfChain.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{"0xaaa", "0xbbb"}, uint8(2), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{conn.ClientA.ChainID: selfChain})

		candidate := quorumCandidate(t, conn.ClientB.ChainID, "0xaaa")

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		err := checkAttestorQuorum(ctx, cfg, clientSet, []attestor.Attestor{candidate})

		require.ErrorContains(t, err, `only 1 reachable/matching attestors for chain "8453"`)
		require.ErrorContains(t, err, "on-chain quorum requires 2")
	})

	t.Run("nonAttestationClientEndsSkipped", func(t *testing.T) {
		// ARRANGE -- checkAttestorQuorum isn't the right place to reject
		// unsupported client types (that's proofgen's dispatch job); it just
		// skips them.
		conn := testConnection()
		conn.ClientA.Type = "tendermint"
		conn.ClientB.Type = "tendermint"

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT + ASSERT -- an empty ClientSet proves neither end was looked up.
		require.NoError(t, checkAttestorQuorum(ctx, cfg, chains.NewClientSet(nil), nil))
	})
}
