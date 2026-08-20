// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const (
	chainIDEth  = "1"
	chainIDBase = "8453"
	txHashLower = "0x60016c34c02278856c81a41ce857ac4bb837a2f4a13c95207e08cbc9e8f2b706"
	txHashUpper = "0x60016C34C02278856C81A41CE857AC4BB837A2F4A13C95207E08CBC9E8F2B706"
)

func relayerConfig() config.Config {
	return config.Config{
		Chains: []config.ChainConfig{
			{
				ChainID: chainIDEth,
				EVM: &config.EVMChainConfig{
					RPC:         "https://ethereum-rpc.example.com",
					ICS26Router: "0x0000000000000000000000000000000000000000",
				},
			},
		},
		Relayer: config.RelayerConfig{
			Connections: []config.ConnectionConfig{
				{
					Alias: "base-client",
					ClientA: config.ClientEnd{
						ClientID: "base-0",
						ChainID:  chainIDEth,
						Type:     config.ClientTypeAttestation,
					},
					ClientB: config.ClientEnd{
						ClientID: "ethereum-0",
						ChainID:  chainIDBase,
						Type:     config.ClientTypeAttestation,
					},
				},
			},
		},
	}
}

func txHashBytes(t *testing.T) []byte {
	t.Helper()

	raw, err := hex.DecodeString(txHashLower[2:])
	require.NoError(t, err)

	return raw
}

func relayAll(chainID, txHash string) RelayRequest {
	return RelayRequest{ChainID: chainID, TxHash: txHash, Selection: SelectionAll}
}

func relaySelected(chainID, txHash string, packets ...PacketSelector) RelayRequest {
	return RelayRequest{ChainID: chainID, TxHash: txHash, Selection: SelectionExplicit, Packets: packets}
}

func TestRelay(t *testing.T) {
	t.Run("tracksExtractedPacketsAtomically", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		repo := mocks.NewMockRepository(t)
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), st, clients, nil)

		blockTime := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
		events := []v2.PacketEvent{
			{
				Height:    100,
				BlockTime: blockTime,
				Kind:      v2.KindSendPacket,
				Packet: channeltypesv2.Packet{
					Sequence:          42,
					SourceClient:      "base-0",
					DestinationClient: "ethereum-0",
					TimeoutTimestamp:  1780000000,
				},
			},
			{
				Height:    100,
				BlockTime: blockTime,
				Kind:      v2.KindSendPacket,
				Packet: channeltypesv2.Packet{
					Sequence:          43,
					SourceClient:      "base-0",
					DestinationClient: "ethereum-0",
					TimeoutTimestamp:  1780000060,
				},
			},
			{
				// packets from unconfigured clients are skipped
				Height:    100,
				BlockTime: blockTime,
				Kind:      v2.KindSendPacket,
				Packet: channeltypesv2.Packet{
					Sequence:          7,
					SourceClient:      "unknown-0",
					DestinationClient: "ethereum-0",
				},
			},
		}

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(events, nil).Once()

		// request and packets land in one transaction; hash normalized to lowercase
		st.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(store.Repository) error")).
			RunAndReturn(func(_ context.Context, fn func(store.Repository) error) error {
				return fn(repo)
			}).
			Once()
		repo.EXPECT().CreateRelayRequest(ctx, chainIDEth, txHashLower).Return(nil).Once()
		repo.EXPECT().UpsertPacket(ctx, store.UpsertPacket{
			Status:                    store.RelayStatusPending,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashLower,
			SourceTxTime:              blockTime,
			PacketSequenceNumber:      42,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Unix(1780000000, 0).UTC(),
		}).Return(nil).Once()
		repo.EXPECT().UpsertPacket(ctx, store.UpsertPacket{
			Status:                    store.RelayStatusNotSelected,
			SourceChainID:             chainIDEth,
			DestinationChainID:        chainIDBase,
			SourceTxHash:              txHashLower,
			SourceTxTime:              blockTime,
			PacketSequenceNumber:      43,
			PacketSourceClientID:      "base-0",
			PacketDestinationClientID: "ethereum-0",
			PacketTimeoutTimestamp:    time.Unix(1780000060, 0).UTC(),
		}).Return(nil).Once()

		// we do not expect UpsertPacket to be called for "unknown-0"

		// ACT
		err := service.Relay(ctx, relaySelected(chainIDEth, txHashUpper, PacketSelector{
			SourceClientID: "base-0",
			SequenceNumber: 42,
		}))

		// ASSERT
		require.NoError(t, err)
	})

	t.Run("allWithNoRelayablePackets", func(t *testing.T) {
		ctx := context.Background()
		st, err := store.NewSqliteInMemory()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, st.Close()) })
		_, err = st.MigrateUp()
		require.NoError(t, err)

		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), st, clients, nil)

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return([]v2.PacketEvent{{
			Kind: v2.KindSendPacket,
			Packet: channeltypesv2.Packet{
				Sequence:     42,
				SourceClient: "unknown-0",
			},
		}}, nil).Once()

		require.NoError(t, service.Relay(ctx, relayAll(chainIDEth, txHashLower)))

		page, err := service.Packets(ctx, PacketFilter{
			SourceChainID: &chainIDEthVar,
			SourceTxHash:  &txHashLowerVar,
		}, PacketQuery{})
		require.NoError(t, err)
		assert.Empty(t, page.Packets)
	})

	t.Run("selectionIsIdempotent", func(t *testing.T) {
		statuses := []store.RelayStatus{
			store.RelayStatusPending,
			store.RelayStatusDeliverRecvPacket,
			store.RelayStatusCompleteWithAck,
			store.RelayStatusFailed,
		}
		for i, status := range statuses {
			t.Run(string(status), func(t *testing.T) {
				ctx := context.Background()
				st, err := store.NewSqliteInMemory()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, st.Close()) })
				_, err = st.MigrateUp()
				require.NoError(t, err)

				sequence := uint64(i + 1)
				input := store.UpsertPacket{
					Status:                    store.RelayStatusPending,
					SourceChainID:             chainIDEth,
					DestinationChainID:        chainIDBase,
					SourceTxHash:              txHashLower,
					SourceTxTime:              time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC),
					PacketSequenceNumber:      sequence,
					PacketSourceClientID:      "base-0",
					PacketDestinationClientID: "ethereum-0",
					PacketTimeoutTimestamp:    time.Unix(1780000000, 0).UTC(),
				}
				require.NoError(t, st.UpsertPacket(ctx, input))
				key := store.PacketKey{SourceChainID: chainIDEth, SourceClientID: "base-0", Sequence: sequence}
				require.NoError(t, st.UpdatePacketStatus(ctx, key, status))

				client := mocks.NewMockClient(t)
				clients := NewMockChainClients(t)
				service := New(relayerConfig(), st, clients, nil)
				clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
				client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return([]v2.PacketEvent{{
					BlockTime: input.SourceTxTime,
					Kind:      v2.KindSendPacket,
					Packet: channeltypesv2.Packet{
						Sequence:          sequence,
						SourceClient:      "base-0",
						DestinationClient: "ethereum-0",
						TimeoutTimestamp:  1780000000,
					},
				}}, nil).Once()

				selector := PacketSelector{SourceClientID: "base-0", SequenceNumber: sequence}
				selectors := []PacketSelector{selector}
				if status == store.RelayStatusPending {
					selectors = append(selectors, selector)
				}
				require.NoError(t, service.Relay(ctx, relaySelected(chainIDEth, txHashLower, selectors...)))
				packets, err := st.ListPacketsBySourceTx(ctx, chainIDEth, txHashLower)
				require.NoError(t, err)
				require.Len(t, packets, 1)
				assert.Equal(t, status, packets[0].Status)
			})
		}
	})

	t.Run("selectionValidation", func(t *testing.T) {
		service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)
		requests := []RelayRequest{
			{ChainID: chainIDEth, TxHash: txHashLower},
			relaySelected(chainIDEth, txHashLower),
			{ChainID: chainIDEth, TxHash: txHashLower, Selection: SelectionMode(99)},
			{
				ChainID: chainIDEth, TxHash: txHashLower, Selection: SelectionAll,
				Packets: []PacketSelector{{SourceClientID: "base-0", SequenceNumber: 42}},
			},
		}
		for _, request := range requests {
			require.ErrorIs(t, service.Relay(context.Background(), request), ErrInvalidInput)
		}
	})

	t.Run("selectedPacketsAreValidatedBeforeMutation", func(t *testing.T) {
		events := []v2.PacketEvent{
			{Kind: v2.KindSendPacket, Packet: channeltypesv2.Packet{Sequence: 42, SourceClient: "base-0"}},
			{Kind: v2.KindSendPacket, Packet: channeltypesv2.Packet{Sequence: 7, SourceClient: "unknown-0"}},
			{
				Kind: v2.KindSendPacket,
				Packet: channeltypesv2.Packet{
					Sequence: 9, SourceClient: "base-0", DestinationClient: "other-0",
				},
			},
		}

		for _, tt := range []struct {
			name     string
			selector PacketSelector
			want     error
		}{
			{name: "absent", selector: PacketSelector{SourceClientID: "base-0", SequenceNumber: 99}, want: ErrInvalidInput},
			{name: "unconfigured", selector: PacketSelector{SourceClientID: "unknown-0", SequenceNumber: 7}, want: ErrFailedPrecondition},
			{
				name: "wrong destination", selector: PacketSelector{SourceClientID: "base-0", SequenceNumber: 9},
				want: ErrFailedPrecondition,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				ctx := context.Background()
				client := mocks.NewMockClient(t)
				clients := NewMockChainClients(t)
				cfg := relayerConfig()
				service := New(cfg, NewMockStore(t), clients, nil)
				clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
				client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(events, nil).Once()

				err := service.Relay(ctx, relaySelected(chainIDEth, txHashLower, tt.selector))
				require.ErrorIs(t, err, tt.want)
			})
		}
	})

	t.Run("sortsPacketLocksDeterministically", func(t *testing.T) {
		packets := map[PacketSelector]store.UpsertPacket{
			{SourceClientID: "client-b", SequenceNumber: 1}: {},
			{SourceClientID: "client-a", SequenceNumber: 9}: {},
			{SourceClientID: "client-a", SequenceNumber: 2}: {},
		}
		assert.Equal(t, []PacketSelector{
			{SourceClientID: "client-a", SequenceNumber: 2},
			{SourceClientID: "client-a", SequenceNumber: 9},
			{SourceClientID: "client-b", SequenceNumber: 1},
		}, sortedPacketSelectors(packets))
	})

	t.Run("unsupportedChain", func(t *testing.T) {
		// ARRANGE
		service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

		// ACT
		err := service.Relay(context.Background(), relayAll("999", txHashLower))

		// ASSERT
		require.ErrorIs(t, err, ErrInvalidInput)
		require.ErrorContains(t, err, "unsupported chain")
	})

	t.Run("chainClientError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), NewMockStore(t), clients, nil)

		// config knows the chain but the chain client set has no client for it
		clients.EXPECT().Get(chainIDEth).Return(nil, false).Once()

		// ACT
		err := service.Relay(ctx, relayAll(chainIDEth, txHashLower))

		// ASSERT
		// a missing client is a server-side inconsistency, not a caller error
		require.ErrorContains(t, err, "client for chain")
		require.ErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unknownTransaction", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), NewMockStore(t), clients, nil)

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, ethereum.NotFound).Once()

		// ACT
		err := service.Relay(ctx, relayAll(chainIDEth, txHashLower))

		// ASSERT
		require.ErrorIs(t, err, ErrNotFound)
		require.ErrorContains(t, err, "no packets found")
	})

	t.Run("extractionError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), NewMockStore(t), clients, nil)

		// nothing is recorded when extraction fails
		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, errors.New("rpc down")).Once()

		// ACT
		err := service.Relay(ctx, relayAll(chainIDEth, txHashLower))

		// ASSERT
		require.ErrorContains(t, err, "extracting packet events")
		require.NotErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("validation", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			chainID string
			txHash  string
		}{
			{name: "empty chainID", chainID: "", txHash: txHashLower},
			{name: "empty txHash", chainID: chainIDEth, txHash: ""},
			{name: "not hex", chainID: chainIDEth, txHash: "0xnothex"},
			{name: "too short", chainID: chainIDEth, txHash: "0xdeadbeef"},
			{name: "missing prefix", chainID: chainIDEth, txHash: txHashLower[2:] + "00"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

				// ACT
				err := service.Relay(context.Background(), relayAll(tt.chainID, tt.txHash))

				// ASSERT
				require.ErrorIs(t, err, ErrInvalidInput)
			})
		}
	})

	t.Run("storeError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		client := mocks.NewMockClient(t)
		clients := NewMockChainClients(t)
		service := New(relayerConfig(), st, clients, nil)

		clients.EXPECT().Get(chainIDEth).Return(client, true).Once()
		client.EXPECT().TxPacketEvents(ctx, txHashBytes(t)).Return(nil, nil).Once()
		st.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(store.Repository) error")).
			Return(errors.New("boom")).
			Once()

		// ACT
		err := service.Relay(ctx, relayAll(chainIDEth, txHashLower))

		// ASSERT
		require.ErrorContains(t, err, "recording relay request")
		require.NotErrorIs(t, err, ErrInvalidInput)
	})
}

func TestPackets(t *testing.T) {
	t.Run("unknownTransactionIsEmptyNotError", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClients(t), nil)

		st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).Return(nil, nil).Once()

		// ACT
		page, err := service.Packets(ctx, PacketFilter{
			SourceChainID: &chainIDEthVar,
			SourceTxHash:  &txHashLowerVar,
		}, PacketQuery{})

		// ASSERT
		require.NoError(t, err)
		require.Empty(t, page.Packets)
		require.False(t, page.HasMore)
	})

	t.Run("normalizesTxHashCasing", func(t *testing.T) {
		// The store holds canonical hashes, so an uppercase filter must be
		// normalized or it silently matches nothing.
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClients(t), nil)

		var seen *string

		st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).
			Run(func(_ context.Context, filter store.PacketFilter, _ store.Page) {
				seen = filter.SourceTxHash
			}).Return(nil, nil).Once()

		_, err := service.Packets(ctx, PacketFilter{
			SourceChainID: &chainIDEthVar,
			SourceTxHash:  &txHashUpperVar,
		}, PacketQuery{})

		require.NoError(t, err)
		require.NotNil(t, seen)
		require.Equal(t, txHashLower, *seen)
	})

	t.Run("rejectsMalformedTxHash", func(t *testing.T) {
		ctx := context.Background()
		service := New(relayerConfig(), NewMockStore(t), NewMockChainClients(t), nil)

		malformed := "not-a-hash"
		_, err := service.Packets(ctx, PacketFilter{SourceTxHash: &malformed}, PacketQuery{})

		require.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("expandsStateIntoRelayStatuses", func(t *testing.T) {
		ctx := context.Background()
		st := NewMockStore(t)
		service := New(relayerConfig(), st, NewMockChainClients(t), nil)

		var seen []store.RelayStatus

		st.EXPECT().ListPackets(ctx, mock.Anything, mock.Anything).
			Run(func(_ context.Context, filter store.PacketFilter, _ store.Page) {
				seen = filter.Statuses
			}).Return(nil, nil).Once()

		pending := StatePending
		_, err := service.Packets(ctx, PacketFilter{State: &pending}, PacketQuery{})

		require.NoError(t, err)
		require.Contains(t, seen, store.RelayStatusAwaitingSendFinality,
			"a PENDING filter must cover in-flight statuses, not just literal PENDING")
		require.NotContains(t, seen, store.RelayStatusCompleteWithAck)
	})
}

var (
	chainIDEthVar  = chainIDEth
	txHashLowerVar = txHashLower
	txHashUpperVar = txHashUpper
)

func TestMapPacketState(t *testing.T) {
	assert.Equal(t, StateNotSelected, mapPacketState(store.RelayStatusNotSelected))

	pending := []store.RelayStatus{
		store.RelayStatusPending,
		store.RelayStatusAwaitingSendFinality,
		store.RelayStatusCheckRecvPacketDelivery,
		store.RelayStatusGetRecvPacket,
		store.RelayStatusDeliverRecvPacket,
		store.RelayStatusWaitForWriteAck,
		store.RelayStatusAwaitingWriteAckFinality,
		store.RelayStatusCheckAckPacketDelivery,
		store.RelayStatusGetAckPacket,
		store.RelayStatusDeliverAckPacket,
		store.RelayStatusAwaitingTimeoutFinality,
		store.RelayStatusCheckTimeoutPacketDelivery,
		store.RelayStatusGetTimeoutPacket,
		store.RelayStatusDeliverTimeoutPacket,
	}
	for _, status := range pending {
		assert.Equal(t, StatePending, mapPacketState(status), string(status))
	}

	assert.Equal(t, StateSucceeded, mapPacketState(store.RelayStatusCompleteWithAck))
	assert.Equal(t, StateTimedOut, mapPacketState(store.RelayStatusCompleteWithTimeout))
	assert.Equal(t, StateRejected, mapPacketState(store.RelayStatusCompleteWithWriteAckError))
	assert.Equal(t, StateRelayFailed, mapPacketState(store.RelayStatusFailed))
}
