package attestor

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	Operation    metric.Float64Histogram
	LatestHeight metric.Int64Gauge
}

var metrics instrumentation

func init() {
	otel.RegisterMetrics("attestor", newInstrumentation, &metrics)
}

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	// also exposes _count for total count
	operation, err := m.Float64Histogram("operation", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	latestHeight, err := m.Int64Gauge("latest_height")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		Operation:    operation,
		LatestHeight: latestHeight,
	}, nil
}

func (m *instrumentation) record(
	ctx context.Context,
	operation, chainID, attestor string,
	err error,
	ts time.Time,
) {
	otel.RecordOperation(ctx, m.Operation, operation, ts,
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
		otel.AttrResultError(err),
	)
}

func (m *instrumentation) latestHeight(ctx context.Context, attestor, chainID string, height uint64) {
	m.LatestHeight.Record(ctx, int64(height), otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
	))
}
