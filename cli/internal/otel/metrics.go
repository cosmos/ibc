// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// Metric attribute keys. Keep in sync with .cursor/metrics.html CARDINALITIES.
const (
	AttrOp     attribute.Key = "operation"
	AttrType   attribute.Key = "type"
	AttrResult attribute.Key = "result"

	AttrAttestor attribute.Key = "attestor"
	AttrSigner   attribute.Key = "signer"

	AttrChainID      attribute.Key = "chain_id"
	AttrClientID     attribute.Key = "client_id"
	AttrDestChainID  attribute.Key = "dest_chain_id"
	AttrDestClientID attribute.Key = "dest_client_id"
	AttrEventType    attribute.Key = "event_type"
	AttrProcessor    attribute.Key = "processor"
	AttrProofKind    attribute.Key = "proof_kind"
	AttrRelayType    attribute.Key = "relay_type"
	AttrState        attribute.Key = "state"
)

// https://github.com/connectrpc/otelconnect-go/blob/462c595e1f85b0797f3990003b79dd39e930919b/instruments.go#L23
const (
	unitMilliseconds = "ms"
)

const serviceName = "ibc"

// MetricConstructor is a function that constructs a metric instance
type MetricConstructor[T any] func(meter metric.Meter) (*T, error)

func RegisterMetrics[T any](name string, constructor MetricConstructor[T], out *T) {
	fullName := fmt.Sprintf("%s.%s", serviceName, name)

	meter := otel.Meter(fullName)

	constructed, err := constructor(meter)
	if err != nil {
		panic(fmt.Errorf("failed to construct metrics for %s: %w", name, err))
	}

	*out = *constructed

	// note we can't exit early otherwise T.MetricFoo.Record(ctx, ...) will panic with `nil`
	if _, ok := meter.(noop.Meter); ok {
		slog.Debug("Noop meter provider", "name", fullName)
	}
}

func UnitMilliseconds() metric.InstrumentOption {
	return metric.WithUnit(unitMilliseconds)
}

func WithAttributes(attrs ...attribute.KeyValue) metric.MeasurementOption {
	return metric.WithAttributes(attrs...)
}

func AttrResultError(err error) attribute.KeyValue {
	if err == nil {
		return AttrResult.String("ok")
	}

	return AttrResult.String("error")
}

func RecordOperation(
	ctx context.Context,
	histogram metric.Float64Histogram,
	operation string,
	ts time.Time,
	attrs ...attribute.KeyValue,
) {
	elapsed := float64(time.Since(ts).Milliseconds())
	attrs = append(attrs, AttrOp.String(operation))
	histogram.Record(ctx, elapsed, metric.WithAttributes(attrs...))
}
