// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric attribute keys.
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
	AttrProcessor    attribute.Key = "processor"
	AttrProofKind    attribute.Key = "proof_kind"
	AttrState        attribute.Key = "state"
)

// https://github.com/connectrpc/otelconnect-go/blob/462c595e1f85b0797f3990003b79dd39e930919b/instruments.go#L23
const (
	unitMilliseconds = "ms"
	unitSeconds      = "s"
)

const serviceName = "ibc"

// MetricConstructor is a function that constructs a metric instance
type MetricConstructor[T any] func(meter metric.Meter) (*T, error)

func RegisterMetrics[T any](name string, constructor MetricConstructor[T]) *T {
	fullName := fmt.Sprintf("%s.%s", serviceName, name)

	meter := otel.Meter(fullName)

	constructed, err := constructor(meter)
	if err != nil {
		panic(fmt.Errorf("failed to construct metrics for %s: %w", name, err))
	}

	if constructed == nil {
		panic(fmt.Errorf("failed to construct metrics for %s: constructor returned nil", name))
	}

	return constructed
}

func UnitMilliseconds() metric.InstrumentOption {
	return metric.WithUnit(unitMilliseconds)
}

func UnitSeconds() metric.InstrumentOption {
	return metric.WithUnit(unitSeconds)
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
	elapsed := durationMilliseconds(time.Since(ts))
	attrs = append(attrs, AttrOp.String(operation))
	histogram.Record(ctx, elapsed, metric.WithAttributes(attrs...))
}

func durationMilliseconds(duration time.Duration) float64 {
	return duration.Seconds() * 1000
}
