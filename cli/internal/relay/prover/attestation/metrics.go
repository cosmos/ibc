// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const (
	reasonNone                = "none"
	reasonQuorumNotMet        = "quorum_not_met"
	reasonExpectedClaimLookup = "expected_claim_lookup"
	reasonInternal            = "internal"
	reasonCanceled            = "canceled"
	reasonRPCError            = "rpc_error"
	reasonTimeout             = "timeout"
	reasonClaimMismatch       = "claim_mismatch"
	reasonInvalidSignature    = "invalid_signature"
	reasonDuplicateSigner     = "duplicate_signer"
)

type instrumentation struct {
	Rounds    metric.Int64Counter
	Responses metric.Int64Counter
}

var metrics = otel.RegisterMetrics("attestation", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	rounds, err := m.Int64Counter("aggregation_rounds_total")
	if err != nil {
		return nil, err
	}
	responses, err := m.Int64Counter("aggregation_responses_total")
	if err != nil {
		return nil, err
	}
	return &instrumentation{Rounds: rounds, Responses: responses}, nil
}

func (r *aggregationRound) attributes(reason string) []attribute.KeyValue {
	result := "error"
	if reason == reasonNone {
		result = "ok"
	}
	return []attribute.KeyValue{
		otel.AttrChainID.String(r.chainID),
		otel.AttrClientID.String(r.clientID),
		otel.AttrProofKind.String(r.proofKind),
		otel.AttrResult.String(result),
		otel.AttrFailureReason.String(reason),
	}
}

func (r *aggregationRound) fail(ctx context.Context, reason string, err error) error {
	if ctx.Err() != nil {
		reason = reasonCanceled
	}
	metrics.Rounds.Add(ctx, 1, otel.WithAttributes(r.attributes(reason)...))
	return err
}

func (r *aggregationRound) succeed(ctx context.Context) {
	metrics.Rounds.Add(ctx, 1, otel.WithAttributes(r.attributes(reasonNone)...))
}

func (r *aggregationRound) recordResponse(ctx context.Context, attestor, reason string) {
	attrs := append(r.attributes(reason), otel.AttrAttestor.String(attestor))
	metrics.Responses.Add(ctx, 1, otel.WithAttributes(attrs...))
}

func responseErrorReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled), connect.CodeOf(err) == connect.CodeCanceled:
		return reasonCanceled
	case errors.Is(err, context.DeadlineExceeded), connect.CodeOf(err) == connect.CodeDeadlineExceeded:
		return reasonTimeout
	default:
		return reasonRPCError
	}
}

func metricProofKind(kind v2.ProofKind) string {
	switch kind {
	case v2.ProofKindPacketCommitment:
		return "packet_commitment"
	case v2.ProofKindAcknowledgement:
		return "acknowledgement"
	case v2.ProofKindReceiptAbsence:
		return "receipt_absence"
	default:
		return "unknown"
	}
}
