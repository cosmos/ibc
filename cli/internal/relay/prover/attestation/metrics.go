// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	DataMatches metric.Int64Counter
}

var metrics = otel.RegisterMetrics("prover.attestation", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	dataMatches, err := m.Int64Counter("attestation_data_matches_total")
	if err != nil {
		return nil, err
	}
	return &instrumentation{DataMatches: dataMatches}, nil
}

func (g *Generator) recordDataMatch(ctx context.Context, attestor string, matches bool) {
	metrics.DataMatches.Add(ctx, 1, metric.WithAttributes(
		otel.AttrChainID.String(g.chainID),
		otel.AttrClientID.String(g.clientID),
		otel.AttrAttestor.String(attestor),
		otel.AttrResult.Bool(matches),
	))
}
