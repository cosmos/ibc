// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"fmt"
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

func metricsWrapper(signer Signer, alias string) Signer {
	if _, ok := signer.(*instrumentedSigner); ok {
		return signer
	}

	var keyType string
	if signer.IsLocal() {
		keyType = fmt.Sprintf("local_%s", signer.Type())
	} else {
		keyType = "remote"
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
