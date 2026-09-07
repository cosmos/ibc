package otel

import (
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// Metric attribute keys. Keep in sync with .cursor/metrics.html CARDINALITIES.
const (
	AttrType   attribute.Key = "type"
	AttrResult attribute.Key = "result"

	AttrAttestor attribute.Key = "attestor"

	AttrChainID     attribute.Key = "chain_id"
	AttrDestChainID attribute.Key = "dest_chain_id"
)

// https://github.com/connectrpc/otelconnect-go/blob/462c595e1f85b0797f3990003b79dd39e930919b/instruments.go#L23
const (
	unitMilliseconds  = "ms"
)

const serviceName = "ibc"

// MetricConstructor is a function that constructs a metric instance
type MetricConstructor[T any] func(meter metric.Meter) (*T, error)

// MeasurementOption alias to the metric.MeasurementOption type
type MeasurementOption = metric.MeasurementOption

func RegisterMetrics[T any](name string, constructor MetricConstructor[T], out *T) {
	fullName := fmt.Sprintf("%s.%s", serviceName, name)

	meter := otel.Meter(fullName)
	if _, ok := meter.(noop.Meter); ok {
		slog.Debug("Noop meter provider, skipping metrics registration", "name", name)
		return
	}

	constructed, err := constructor(meter)
	if err != nil {
		panic(fmt.Errorf("failed to construct metrics for %s: %w", name, err))
	}

	*out = *constructed
}

func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}

	return value
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
