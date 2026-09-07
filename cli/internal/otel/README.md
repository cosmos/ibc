<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Observability

## Guide on creating new metrics

1. Ensure unique attribute keys are present in `otel/metrics.go` (eg `AttrChainID`, `AttrAttestor`). Add new if needed.
2. Create `metrics.go` based on the following reference example. Don't enforce units if not necessary.
3. Keep metric-related code in `<pkg>/metrics.go`, create small helpers to make metrics recording
   more concise for callers. See `latestHeight(...)` and `recordOperation(...)` in attestor's metrics.
4. Leave metrics unitless (except durations).

```go
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

func (m *instrumentation) recordOperation(
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
```