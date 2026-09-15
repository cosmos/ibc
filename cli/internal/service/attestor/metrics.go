// SPDX-License-Identifier: Apache-2.0

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

var metrics = otel.RegisterMetrics("attestor", newInstrumentation)

const callerDefault = "internal"

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	// also exposes _count for total count
	operation, err := m.Float64Histogram("attestor_operation", otel.UnitMilliseconds())
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
		otel.AttrCallerFromCaller(ctx, callerDefault),
	)
}

func (m *instrumentation) latestHeight(ctx context.Context, attestor, chainID string, height uint64) {
	m.LatestHeight.Record(ctx, int64(height), otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
		otel.AttrCallerFromCaller(ctx, callerDefault),
	))
}

type instrumentedAttestor struct {
	Attestor
	metrics *instrumentation
	name    string
}

// MetricsWrapper wraps an Attestor with operation instrumentation.
func MetricsWrapper(attestor Attestor) Attestor {
	if _, ok := attestor.(*instrumentedAttestor); ok {
		return attestor
	}

	return &instrumentedAttestor{
		Attestor: attestor,
		name:     attestor.Name(),
		metrics:  metrics,
	}
}

func (a *instrumentedAttestor) LatestHeight(ctx context.Context) (uint64, error) {
	started := time.Now()
	height, err := a.Attestor.LatestHeight(ctx)
	a.metrics.record(ctx, "latest_height", a.ChainID(), a.name, err, started)
	if err == nil {
		a.metrics.latestHeight(ctx, a.name, a.ChainID(), height)
	}

	return height, err
}

func (a *instrumentedAttestor) StateAttestation(ctx context.Context, height uint64) (Attestation, error) {
	started := time.Now()
	result, err := a.Attestor.StateAttestation(ctx, height)
	a.metrics.record(ctx, "state_attestation", a.ChainID(), a.name, err, started)

	return result, err
}

func (a *instrumentedAttestor) PacketAttestation(
	ctx context.Context,
	req PacketAttestationRequest,
) (Attestation, error) {
	started := time.Now()
	result, err := a.Attestor.PacketAttestation(ctx, req)
	a.metrics.record(ctx, "packet_attestation", a.ChainID(), a.name, err, started)

	return result, err
}
