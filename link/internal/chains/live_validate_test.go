package chains

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

func testConnection() config.ConnectionConfig {
	return config.ConnectionConfig{
		Alias: "eth-base",
		ClientA: config.ClientConfig{
			ChainID:  "1",
			ClientID: "base-0",
		},
		ClientB: config.ClientConfig{
			ChainID:  "8453",
			ClientID: "ethereum-0",
		},
	}
}

func TestValidateConnectionsLive(t *testing.T) {
	t.Run("bothDirectionsAgree", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		chainA := mocks.NewMockClient(t)
		chainA.EXPECT().GetCounterparty(context.Background(), conn.ClientA.ClientID).Return(conn.ClientB.ClientID, nil)

		chainB := mocks.NewMockClient(t)
		chainB.EXPECT().GetCounterparty(context.Background(), conn.ClientB.ClientID).Return(conn.ClientA.ClientID, nil)

		clients := NewClientSet(map[string]Client{conn.ClientA.ChainID: chainA, conn.ClientB.ChainID: chainB})
		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT
		err := ValidateConnectionsLive(context.Background(), cfg, clients)

		// ASSERT
		require.NoError(t, err)
	})

	t.Run("oneDirectionMismatches", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()

		chainA := mocks.NewMockClient(t)
		chainA.EXPECT().GetCounterparty(context.Background(), conn.ClientA.ClientID).Return("some-other-client", nil)

		chainB := mocks.NewMockClient(t)
		chainB.EXPECT().GetCounterparty(context.Background(), conn.ClientB.ClientID).Return(conn.ClientA.ClientID, nil).Maybe()

		clients := NewClientSet(map[string]Client{conn.ClientA.ChainID: chainA, conn.ClientB.ChainID: chainB})
		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT
		err := ValidateConnectionsLive(context.Background(), cfg, clients)

		// ASSERT
		require.ErrorContains(t, err, `connection "eth-base"`)
		require.ErrorContains(t, err, "clientA")
		require.ErrorContains(t, err, `on-chain counterparty "some-other-client" does not match configured counterparty "ethereum-0"`)
	})

	t.Run("noChainClientConfigured", func(t *testing.T) {
		// ARRANGE
		conn := testConnection()
		clients := NewClientSet(nil)
		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT
		err := ValidateConnectionsLive(context.Background(), cfg, clients)

		// ASSERT
		require.ErrorContains(t, err, `no chain client for "1"`)
	})
}
