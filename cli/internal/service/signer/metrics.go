// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

const (
	typeRemote     = "remote"
	typeLocalECDSA = "local_ecdsa"
	typeLocalEDDSA = "local_eddsa"
)

type instrumentation struct {
	SignsTotal   metric.Int64Counter
	SignDuration metric.Float64Histogram
}

var metrics instrumentation

func init() {
	otel.RegisterMetrics("signer", newInstrumentation, &metrics)
}

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	signsTotal, err := m.Int64Counter("signs_total")
	if err != nil {
		return nil, err
	}

	signDur, err := m.Float64Histogram("sign_dur", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		SignsTotal:   signsTotal,
		SignDuration: signDur,
	}, nil
}

func (m *instrumentation) sign(ctx context.Context, alias, typ string, err error, ts time.Time) {
	labels := otel.WithAttributes(
		otel.AttrSigner.String(alias),
		otel.AttrType.String(typ),
		otel.AttrResultError(err),
	)

	m.SignsTotal.Add(ctx, 1, labels)
	m.SignDuration.Record(ctx, float64(time.Since(ts).Milliseconds()), labels)
}
