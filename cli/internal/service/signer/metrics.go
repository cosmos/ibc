// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/lfdt-ibc/ibc/cli/internal/otel"
	"github.com/lfdt-ibc/ibc/cli/keyfile"
)

// typeRemote labels a remote signer; a local one is labeled by localType.
const typeRemote = "remote"

func localType(keyType keyfile.Type) string {
	return "local_" + string(keyType)
}

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
	operation, signerAlias, signerType string,
	err error,
	ts time.Time,
) {
	otel.RecordOperation(ctx, m.Operation, operation, ts,
		otel.AttrSigner.String(signerAlias),
		otel.AttrType.String(signerType),
		otel.AttrResultError(err),
	)
}

type instrumentedSigner struct {
	Signer
	alias   string
	keyType string
}

func metricsWrapper(signer Signer, alias, keyType string) Signer {
	if _, ok := signer.(*instrumentedSigner); ok {
		return signer
	}

	return &instrumentedSigner{
		Signer:  signer,
		alias:   alias,
		keyType: keyType,
	}
}

func (s *instrumentedSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	started := time.Now()
	signature, err := s.Signer.Sign(ctx, message)
	metrics.record(ctx, "sign", s.alias, s.keyType, err, started)
	return signature, err
}
