// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

type instrumentation struct {
	PacketsTotal metric.Int64Counter
	BatchSize    metric.Int64Histogram
}

var metrics = otel.RegisterMetrics("relayer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	packetsTotal, err := m.Int64Counter("packets_total")
	if err != nil {
		return nil, err
	}

	// override default latency-based bucket boundaries
	batchSize, err := m.Int64Histogram(
		"batch_size",
		metric.WithExplicitBucketBoundaries(1, 2, 4, 8, 12, 16, 32, 48, 64, 96, 128),
	)
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		PacketsTotal: packetsTotal,
		BatchSize:    batchSize,
	}, nil
}

func (m *instrumentation) packetTransition(ctx context.Context, tr *processors.Transfer, state store.RelayStatus) {
	if tr.Status == state {
		return
	}

	m.PacketsTotal.Add(ctx, 1, metric.WithAttributes(
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
	kind := batchKind(status)
	chainID := batch[0].SourceChainID
	if kind == v2.RelayKindRecv {
		chainID = batch[0].DestinationChainID
	}

	m.BatchSize.Record(ctx, int64(len(batch)), metric.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrProcessor.String(kind.String()),
		otel.AttrResultError(err),
	))
}

// batchKind is the packet operation the batch processor for status relays.
func batchKind(status store.RelayStatus) v2.RelayKind {
	switch status {
	case store.RelayStatusDeliverRecvPacket:
		return v2.RelayKindRecv
	case store.RelayStatusDeliverAckPacket:
		return v2.RelayKindAck
	case store.RelayStatusDeliverTimeoutPacket:
		return v2.RelayKindTimeout
	default:
		return v2.RelayKindUnknown
	}
}
