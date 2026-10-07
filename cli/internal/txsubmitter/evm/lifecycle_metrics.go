// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type lifecycleInstrumentation struct {
	pending    metric.Int64Gauge
	oldest     metric.Float64Gauge
	events     metric.Int64Counter
	resolution metric.Float64Histogram
}

var lifecycleMetrics = otel.RegisterMetrics("relayer", newLifecycleInstrumentation)

func newLifecycleInstrumentation(m metric.Meter) (*lifecycleInstrumentation, error) {
	var i lifecycleInstrumentation
	var err error
	if i.pending, err = m.Int64Gauge("evm_tx_pending"); err != nil {
		return nil, err
	}
	if i.oldest, err = m.Float64Gauge("evm_tx_oldest_pending_seconds", metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if i.events, err = m.Int64Counter("evm_tx_events"); err != nil {
		return nil, err
	}
	if i.resolution, err = m.Float64Histogram("evm_tx_resolution_seconds", metric.WithUnit("s")); err != nil {
		return nil, err
	}
	return &i, nil
}

func (c *TxSubmitter) metricAttrs() []attribute.KeyValue {
	return []attribute.KeyValue{otel.AttrChainID.String(c.chainID), otel.AttrWallet.String(c.address.Hex())}
}

func (c *TxSubmitter) recordEvent(event string) {
	lifecycleMetrics.events.Add(
		c.ctx,
		1,
		metric.WithAttributes(append(c.metricAttrs(), attribute.String("event", strings.ToLower(event)))...),
	)
}

func (c *TxSubmitter) observe(pending map[uint64]*pendingTx) {
	attrs := metric.WithAttributes(c.metricAttrs()...)
	var oldest float64
	for _, p := range pending {
		oldest = max(oldest, time.Since(p.attempts[0].SubmittedAt).Seconds())
	}
	lifecycleMetrics.pending.Record(c.ctx, int64(len(pending)), attrs)
	lifecycleMetrics.oldest.Record(c.ctx, oldest, attrs)
}

func (c *TxSubmitter) recordResolution(since time.Time) {
	lifecycleMetrics.resolution.Record(c.ctx, time.Since(since).Seconds(), metric.WithAttributes(c.metricAttrs()...))
}
