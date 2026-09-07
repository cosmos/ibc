package attestor

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	TotalAttestations   metric.Int64Counter
	AttestationDuration metric.Float64Histogram
	LatestHeight        metric.Int64Gauge
	LatestHeightFailure metric.Int64Counter
}

var metrics instrumentation

func init() {
	otel.RegisterMetrics("attestor", newInstrumentation, &metrics)
}

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	totalAttestations, err := m.Int64Counter("total_attestations")
	if err != nil {
		return nil, err
	}

	attestationDur, err := m.Float64Histogram("attestation_dur", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	latestHeight, err := m.Int64Gauge("latest_height")
	if err != nil {
		return nil, err
	}

	latestHeightFailure, err := m.Int64Counter("latest_height_failure")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		TotalAttestations:   totalAttestations,
		AttestationDuration: attestationDur,
		LatestHeight:        latestHeight,
		LatestHeightFailure: latestHeightFailure,
	}, nil
}

func (m *instrumentation) latestHeight(ctx context.Context, attestor, chainID string, height uint64, err error) {
	labels := otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
	)

	if err == nil {
		m.LatestHeight.Record(ctx, int64(height), labels)
	} else {
		m.LatestHeightFailure.Add(ctx, 1, labels)
	}
}

func (m *instrumentation) attestation(ctx context.Context, typ, attestor, chainID string, err error, ts time.Time) {
	labels := otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
		otel.AttrType.String(typ),
		otel.AttrResultError(err),
	)

	m.TotalAttestations.Add(ctx, 1, labels)
	m.AttestationDuration.Record(ctx, float64(time.Since(ts).Milliseconds()), labels)
}
