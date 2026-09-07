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
	Operation metric.Float64Histogram
}

var metrics instrumentation

func init() {
	otel.RegisterMetrics("signer", newInstrumentation, &metrics)
}

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	operation, err := m.Float64Histogram("operation", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		Operation: operation,
	}, nil
}

func (m *instrumentation) record(
	ctx context.Context,
	operation, alias, typ string,
	err error,
	ts time.Time,
) {
	otel.RecordOperation(ctx, m.Operation, operation, ts,
		otel.AttrSigner.String(alias),
		otel.AttrType.String(typ),
		otel.AttrResultError(err),
	)
}
