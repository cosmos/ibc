// SPDX-License-Identifier: Apache-2.0

package prover

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/otel"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

type instrumentation struct {
	Operation            metric.Float64Histogram
	LatestProvableHeight metric.Int64Gauge
	PacketBatchSize      metric.Int64Histogram
}

var metrics = otel.RegisterMetrics("prover", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	operation, err := m.Float64Histogram("prover_operation", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	latestProvableHeight, err := m.Int64Gauge("latest_provable_height")
	if err != nil {
		return nil, err
	}

	packetBatchSize, err := m.Int64Histogram("packet_batch_size")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		Operation:            operation,
		LatestProvableHeight: latestProvableHeight,
		PacketBatchSize:      packetBatchSize,
	}, nil
}

func (m *instrumentation) record(
	ctx context.Context,
	operation, chainID, clientID string,
	proverType string,
	err error,
	ts time.Time,
	attrs ...attribute.KeyValue,
) {
	attrs = append(attrs,
		otel.AttrChainID.String(chainID),
		otel.AttrClientID.String(clientID),
		otel.AttrType.String(proverType),
		otel.AttrResultError(err),
	)
	otel.RecordOperation(ctx, m.Operation, operation, ts, attrs...)
}

func (m *instrumentation) latestProvableHeight(ctx context.Context, chainID, clientID string, height uint64) {
	m.LatestProvableHeight.Record(ctx, int64(height), otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrClientID.String(clientID),
	))
}

func (m *instrumentation) packetBatchSize(
	ctx context.Context,
	chainID, clientID, typ string,
	kind v2.ProofKind,
	size int,
) {
	m.PacketBatchSize.Record(ctx, int64(size), otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrClientID.String(clientID),
		otel.AttrType.String(typ),
		proofKindAttribute(kind),
	))
}

type instrumentedProver struct {
	Prover
	chainID    string
	clientID   string
	proverType string
}

func metricsWrapper(prover Prover, chainID, clientID string, proverType config.ClientType) Prover {
	return &instrumentedProver{
		Prover:     prover,
		chainID:    chainID,
		clientID:   clientID,
		proverType: string(proverType),
	}
}

func (p *instrumentedProver) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	started := time.Now()
	height, timestamp, err := p.Prover.LatestProvableHeight(ctx)

	metrics.record(ctx, "latest_provable_height", p.chainID, p.clientID, p.proverType, err, started)
	if err == nil {
		metrics.latestProvableHeight(ctx, p.chainID, p.clientID, height)
	}

	return height, timestamp, err
}

func (p *instrumentedProver) StateProof(ctx context.Context, height uint64) ([]byte, error) {
	started := time.Now()
	proof, err := p.Prover.StateProof(ctx, height)

	metrics.record(ctx, "state_proof", p.chainID, p.clientID, p.proverType, err, started)

	return proof, err
}

func (p *instrumentedProver) PacketProofs(
	ctx context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []types.Packet,
) ([][]byte, error) {
	started := time.Now()
	proofs, err := p.Prover.PacketProofs(ctx, height, kind, packets)

	metrics.record(ctx, "packet_proofs", p.chainID, p.clientID, p.proverType, err, started, proofKindAttribute(kind))
	metrics.packetBatchSize(ctx, p.chainID, p.clientID, p.proverType, kind, len(packets))

	return proofs, err
}

func proofKindAttribute(kind v2.ProofKind) attribute.KeyValue {
	var value string

	switch kind {
	case v2.ProofKindPacketCommitment:
		value = "packet_commitment"
	case v2.ProofKindAcknowledgement:
		value = "acknowledgement"
	case v2.ProofKindReceiptAbsence:
		value = "receipt_absence"
	default:
		value = "unknown"
	}

	return otel.AttrProofKind.String(value)
}
