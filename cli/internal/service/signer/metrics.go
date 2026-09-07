// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	"github.com/cosmos/ibc/cli/keyfile"
)

const (
	typeRemote     = "remote"
	typeLocalECDSA = "local_ecdsa"
	typeLocalEDDSA = "local_eddsa"
)

type instrumentation struct {
	Operation metric.Float64Histogram
}

var metrics = otel.RegisterMetrics("signer", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	operation, err := m.Float64Histogram("signer_operation", otel.UnitMilliseconds())
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

type instrumentedSigner struct {
	Signer
	alias string
	typ   string
}

func metricsWrapper(alias, typ string, signer Signer) Signer {
	return &instrumentedSigner{
		Signer: signer,
		alias:  alias,
		typ:    typ,
	}
}

func localSignerType(keyType keyfile.Type) string {
	if keyType == ECDSA {
		return typeLocalECDSA
	}
	return typeLocalEDDSA
}

func (s *instrumentedSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	started := time.Now()
	signature, err := s.Signer.Sign(ctx, message)
	metrics.record(ctx, "sign", s.alias, s.typ, err, started)
	return signature, err
}
