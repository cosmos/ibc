// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/store"
)

type instrumentation struct {
	PacketsTotal metric.Int64Counter
	BatchesTotal metric.Int64Counter
	BatchSize    metric.Int64Histogram
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	packetsTotal, err := m.Int64Counter("packets_total")
	if err != nil {
		return nil, err
	}

	batchesTotal, err := m.Int64Counter("batches_total")
	if err != nil {
		return nil, err
	}

	batchSize, err := m.Int64Histogram("batch_size")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		PacketsTotal: packetsTotal,
		BatchesTotal: batchesTotal,
		BatchSize:    batchSize,
	}, nil
}

func (m *instrumentation) packetTransition(ctx context.Context, tr *processors.Transfer, state store.RelayStatus) {
	m.PacketsTotal.Add(ctx, 1, otel.WithAttributes(
		otel.AttrChainID.String(tr.SourceChainID),
		otel.AttrDestChainID.String(tr.DestinationChainID),
		otel.AttrClientID.String(tr.PacketSourceClientID),
		otel.AttrDestClientID.String(tr.PacketDestinationClientID),
		otel.AttrState.String(string(state)),
	))
}

func (m *instrumentation) batch(
	ctx context.Context,
	status store.RelayStatus,
	batch []*processors.Transfer,
	err error,
) {
	processor := batchProcessorName(status)
	attrProcessor := otel.AttrProcessor.String(processor)
	attrChain := otel.AttrChainID.String(batchChainID(batch[0], processor))

	m.BatchSize.Record(ctx, int64(len(batch)), otel.WithAttributes(attrChain, attrProcessor))
	m.BatchesTotal.Add(ctx, 1, otel.WithAttributes(attrChain, attrProcessor, otel.AttrResultError(err)))
}

// processors/batch_ack.go
// processors/batch_recv.go
// processors/batch_relay.go
func batchProcessorName(status store.RelayStatus) string {
	switch status {
	case store.RelayStatusDeliverRecvPacket:
		return "recv"
	case store.RelayStatusDeliverAckPacket:
		return "ack"
	case store.RelayStatusDeliverTimeoutPacket:
		return "timeout"
	default:
		return "unknown"
	}
}

func batchChainID(tr *processors.Transfer, processor string) string {
	if processor == "recv" {
		return tr.DestinationChainID
	}

	return tr.SourceChainID
}
