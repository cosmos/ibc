// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/lfdt-ibc/ibc/cli/internal/otel"
	"github.com/lfdt-ibc/ibc/cli/internal/store"
	"github.com/lfdt-ibc/ibc/cli/internal/tests/mocks"
	"github.com/lfdt-ibc/ibc/cli/internal/txsubmitter"
	v2 "github.com/lfdt-ibc/ibc/cli/internal/types/v2"
)

// clearRecorder records which relay tx a retry stage clears.
type clearRecorder struct {
	cleared []v2.RelayKind
	err     error
}

func (c *clearRecorder) ClearPacketRecvTx(context.Context, store.PacketKey) error {
	c.cleared = append(c.cleared, v2.RelayKindRecv)
	return c.err
}

func (c *clearRecorder) ClearPacketAckTx(context.Context, store.PacketKey) error {
	c.cleared = append(c.cleared, v2.RelayKindAck)
	return c.err
}

func (c *clearRecorder) ClearPacketTimeoutTx(context.Context, store.PacketKey) error {
	c.cleared = append(c.cleared, v2.RelayKindTimeout)
	return c.err
}

type retryProcessor interface {
	Process(ctx context.Context, tr *Transfer) (*Transfer, error)
	ShouldProcess(tr *Transfer) bool
	Status() store.RelayStatus
}

func TestRetryRelayTx(t *testing.T) {
	route := Route{
		SourceChainID:       "source",
		SourceClientID:      "source-client",
		DestinationChainID:  "destination",
		DestinationClientID: "destination-client",
	}
	const hash = "0xrelay"
	sentAt := time.Unix(1_000, 0)

	for _, kind := range []struct {
		name string
		kind v2.RelayKind
		// newProcessor gets no chain client for recv: recv never asks the
		// chain whether the tx executed.
		newProcessor func(ChainClients, txsubmitter.TxSubmitter, *clearRecorder) retryProcessor
		setTx        func(p *store.Packet, hash *string, at *time.Time)
		status       store.RelayStatus
		leg          Leg
		// the chain and client the relay tx is submitted to
		txChainID, txClientID string
		checksExecution       bool
		// a write ack stops recv retries, but not ack or timeout retries
		skipsWithWriteAck bool
		retrying          error
	}{
		{
			name: "recv",
			kind: v2.RelayKindRecv,
			newProcessor: func(c ChainClients, sub txsubmitter.TxSubmitter, s *clearRecorder) retryProcessor {
				return NewRetryRelayTx(v2.RelayKindRecv, c, sub, s, route)
			},
			setTx:             func(p *store.Packet, h *string, at *time.Time) { p.RecvTxHash, p.RecvTxTime = h, at },
			status:            store.RelayStatusDeliverRecvPacket,
			leg:               LegSendToRecv,
			txChainID:         route.DestinationChainID,
			txClientID:        route.DestinationClientID,
			skipsWithWriteAck: true,
			retrying:          ErrRetryingRelayTx,
		},
		{
			name: "ack",
			kind: v2.RelayKindAck,
			newProcessor: func(c ChainClients, sub txsubmitter.TxSubmitter, s *clearRecorder) retryProcessor {
				return NewRetryRelayTx(v2.RelayKindAck, c, sub, s, route)
			},
			setTx:           func(p *store.Packet, h *string, at *time.Time) { p.AckTxHash, p.AckTxTime = h, at },
			status:          store.RelayStatusDeliverAckPacket,
			leg:             LegRecvToAck,
			txChainID:       route.SourceChainID,
			txClientID:      route.SourceClientID,
			checksExecution: true,
			retrying:        ErrRetryingRelayTx,
		},
		{
			name: "timeout",
			kind: v2.RelayKindTimeout,
			newProcessor: func(c ChainClients, sub txsubmitter.TxSubmitter, s *clearRecorder) retryProcessor {
				return NewRetryRelayTx(v2.RelayKindTimeout, c, sub, s, route)
			},
			setTx:           func(p *store.Packet, h *string, at *time.Time) { p.TimeoutTxHash, p.TimeoutTxTime = h, at },
			status:          store.RelayStatusDeliverTimeoutPacket,
			leg:             LegSendToTimeout,
			txChainID:       route.SourceChainID,
			txClientID:      route.SourceClientID,
			checksExecution: true,
			retrying:        ErrRetryingRelayTx,
		},
	} {
		newTransfer := func(withTx bool) (*Transfer, *store.Packet) {
			p := store.Packet{
				SourceChainID:             route.SourceChainID,
				DestinationChainID:        route.DestinationChainID,
				PacketSequenceNumber:      7,
				PacketSourceClientID:      route.SourceClientID,
				PacketDestinationClientID: route.DestinationClientID,
			}
			if withTx {
				h, at := hash, sentAt
				kind.setTx(&p, &h, &at)
			}

			return NewTransfer(p, slog.Default()), &p
		}

		// chainsFor gives recv no chain client, so any attempt to look one
		// up fails the row.
		chainsFor := func(client *mocks.MockClient) ChainClients {
			if !kind.checksExecution {
				return staticChains{}
			}

			return staticChains{route.SourceChainID: client}
		}

		// run seeds the submitted tx on the chain it is expected on, so the
		// confirmed and retry counters only move if the stage uses that chain.
		run := func(
			t *testing.T,
			retry bool,
			retryErr error,
			client *mocks.MockClient,
			storage *clearRecorder,
		) (*Transfer, *Transfer, metricdata.ResourceMetrics, error) {
			t.Helper()

			suite := newMetricsTestSuite(t)
			suite.install(t)
			suite.instruments.txSubmitted(suite.ctx, kind.txChainID, kind.txClientID, hash)

			sub := mocks.NewMockTxSubmitter(t)
			sub.EXPECT().ShouldRetry(mock.Anything, hash, sentAt).Return(retry, retryErr).Once()

			tr, _ := newTransfer(true)
			out, err := kind.newProcessor(chainsFor(client), sub, storage).Process(suite.ctx, tr)

			var data metricdata.ResourceMetrics
			require.NoError(t, suite.reader.Collect(suite.ctx, &data))

			return tr, out, data, err
		}

		t.Run(kind.name, func(t *testing.T) {
			t.Run("shouldProcess", func(t *testing.T) {
				p := kind.newProcessor(staticChains{}, nil, &clearRecorder{})

				tr, _ := newTransfer(true)
				assert.True(t, p.ShouldProcess(tr))

				tr, _ = newTransfer(false)
				assert.False(t, p.ShouldProcess(tr))

				h := hash
				_, packet := newTransfer(false)
				kind.setTx(packet, &h, nil)
				assert.False(t, p.ShouldProcess(NewTransfer(*packet, slog.Default())), "no sent time")

				_, packet = newTransfer(true)
				writeAck := "0xwriteack"
				packet.WriteAckTxHash = &writeAck
				assert.Equal(t, !kind.skipsWithWriteAck, p.ShouldProcess(NewTransfer(*packet, slog.Default())))

				assert.Equal(t, kind.status, p.Status())
			})

			t.Run("confirmedTx", func(t *testing.T) {
				client := mocks.NewMockClient(t)
				if kind.checksExecution {
					client.EXPECT().PacketRelayExecuted(mock.Anything, hash, kind.kind,
						uint64(7), route.SourceClientID, route.DestinationClientID).Return(true, nil).Once()
				}
				storage := &clearRecorder{}

				tr, out, data, err := run(t, false, nil, client, storage)

				require.NoError(t, err)
				assert.Same(t, tr, out)
				assert.Empty(t, storage.cleared)
				assertTxConfirmed(t, data, kind.txChainID, kind.txClientID)
				assert.Empty(t, int64Points(data, "transaction_retries_total"))
				assert.Empty(t, int64Points(data, "relay_packet_execution_total"))
			})

			t.Run("retryTx", func(t *testing.T) {
				// no PacketRelayExecuted expectation: a tx being retried is
				// not checked for execution
				storage := &clearRecorder{}

				tr, out, data, err := run(t, true, nil, mocks.NewMockClient(t), storage)

				require.ErrorIs(t, err, kind.retrying)
				assert.Nil(t, out)
				assert.Equal(t, []v2.RelayKind{kind.kind}, storage.cleared)
				assert.Empty(t, int64Points(data, "transactions_confirmed_total"))

				retries := int64Points(data, "transaction_retries_total")
				require.Len(t, retries, 1)
				assert.Equal(t, int64(1), retries[0].Value)
				assert.Equal(t, string(kind.leg), attributeString(t, retries[0].Attributes, otel.AttrType))
				assert.Equal(t, tr.SourceChainID, attributeString(t, retries[0].Attributes, otel.AttrChainID))

				assertExecutionFailed(t, data, kind.name)
			})

			t.Run("shouldRetryErrors", func(t *testing.T) {
				storage := &clearRecorder{}

				_, out, _, err := run(t, false, assert.AnError, mocks.NewMockClient(t), storage)

				require.ErrorIs(t, err, assert.AnError)
				require.NotErrorIs(t, err, kind.retrying)
				assert.Nil(t, out)
				assert.Empty(t, storage.cleared)
			})

			t.Run("clearErrors", func(t *testing.T) {
				storage := &clearRecorder{err: assert.AnError}

				_, out, _, err := run(t, true, nil, mocks.NewMockClient(t), storage)

				require.ErrorIs(t, err, assert.AnError)
				require.NotErrorIs(t, err, kind.retrying)
				assert.Nil(t, out)
			})

			if !kind.checksExecution {
				return
			}

			t.Run("confirmedButNotExecuted", func(t *testing.T) {
				client := mocks.NewMockClient(t)
				client.EXPECT().PacketRelayExecuted(mock.Anything, hash, kind.kind,
					uint64(7), route.SourceClientID, route.DestinationClientID).Return(false, nil).Once()
				storage := &clearRecorder{}

				_, out, data, err := run(t, false, nil, client, storage)

				require.ErrorIs(t, err, kind.retrying)
				assert.Nil(t, out)
				assert.Equal(t, []v2.RelayKind{kind.kind}, storage.cleared)
				// the tx counts as confirmed, which consumes its submission,
				// so no retry is counted for it
				assertTxConfirmed(t, data, kind.txChainID, kind.txClientID)
				assert.Empty(t, int64Points(data, "transaction_retries_total"))
				assertExecutionFailed(t, data, kind.name)
			})

			t.Run("executionCheckErrors", func(t *testing.T) {
				client := mocks.NewMockClient(t)
				client.EXPECT().PacketRelayExecuted(mock.Anything, hash, kind.kind,
					uint64(7), route.SourceClientID, route.DestinationClientID).Return(false, assert.AnError).Once()
				storage := &clearRecorder{}

				_, out, _, err := run(t, false, nil, client, storage)

				require.ErrorIs(t, err, assert.AnError)
				require.NotErrorIs(t, err, kind.retrying)
				assert.Nil(t, out)
				assert.Empty(t, storage.cleared)
			})
		})
	}
}

func int64Points(data metricdata.ResourceMetrics, name string) []metricdata.DataPoint[int64] {
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == name {
				return sum.DataPoints
			}
		}
	}

	return nil
}

func assertTxConfirmed(t *testing.T, data metricdata.ResourceMetrics, chainID, clientID string) {
	t.Helper()

	points := int64Points(data, "transactions_confirmed_total")
	require.Len(t, points, 1)
	assert.Equal(t, int64(1), points[0].Value)
	want := attribute.NewSet(
		otel.AttrChainID.String(chainID),
		otel.AttrClientID.String(clientID),
	)
	assert.Equal(t, want.ToSlice(), points[0].Attributes.ToSlice())
}

func assertExecutionFailed(t *testing.T, data metricdata.ResourceMetrics, processor string) {
	t.Helper()

	points := int64Points(data, "relay_packet_execution_total")
	require.Len(t, points, 1)
	assert.Equal(t, int64(1), points[0].Value)
	assert.Equal(t, processor, attributeString(t, points[0].Attributes, otel.AttrProcessor))
	assert.Equal(t, executionFailed, attributeString(t, points[0].Attributes, otel.AttrResult))
}
