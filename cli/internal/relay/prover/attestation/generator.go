// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/cli/attestor/evm/ibc"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Generator implements prover.Prover for one configured
// attestation light client: LatestProvableHeight/StateProof/PacketProofs all
// query the same fixed attestor set with the same quorum threshold
type Generator struct {
	attestors         []attestor.Attestor
	threshold         int
	counterpartyChain chains.Client
	logger            *slog.Logger
}

func New(
	attestors []attestor.Attestor,
	threshold int,
	counterpartyChain chains.Client,
	logger *slog.Logger,
) *Generator {
	return &Generator{
		attestors:         attestors,
		threshold:         threshold,
		counterpartyChain: counterpartyChain,
		logger:            logger,
	}
}

func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	return latestProvableHeight(ctx, g.logger, g.attestors, g.threshold, g.counterpartyChain)
}

func (g *Generator) StateProof(ctx context.Context, height uint64) ([]byte, error) {
	header, err := g.counterpartyChain.GetBlockHeader(ctx, height)
	if err != nil {
		return nil, errors.Wrapf(err, "getting header at height %d", height)
	}

	expectedData, err := attestorevm.EncodeStateAttestation(height, uint64(header.Timestamp.Unix()))
	if err != nil {
		return nil, errors.Wrap(err, "encoding expected state attestation")
	}

	result, err := queryStateQuorum(ctx, g.logger, g.attestors, g.threshold, height, expectedData)
	if err != nil {
		return nil, errors.Wrap(err, "querying state attestation quorum")
	}

	proof, err := attestorevm.EncodeAttestationProof(result.AttestationData, result.Signatures)
	if err != nil {
		return nil, errors.Wrap(err, "encoding state attestation proof")
	}

	return proof, nil
}

func (g *Generator) PacketProofs(
	ctx context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
	acknowledgements []channeltypesv2.Acknowledgement,
) ([][]byte, error) {
	commitmentType, err := commitmentTypeOf(kind)
	if err != nil {
		return nil, err
	}

	if kind == v2.ProofKindAcknowledgement && len(acknowledgements) != len(packets) {
		return nil, errors.New("acknowledgement count must match packet count")
	}

	encodedPackets := make([][]byte, len(packets))
	expectedPackets := make([]attestorevm.PacketCompact, len(packets))

	for i, packet := range packets {
		encoded, errEnc := ibc.EncodePacket(packet)
		if errEnc != nil {
			return nil, errors.Wrapf(errEnc, "encoding packet sequence %d", packet.Sequence)
		}

		encodedPackets[i] = encoded

		var ack channeltypesv2.Acknowledgement
		if kind == v2.ProofKindAcknowledgement {
			ack = acknowledgements[i]
		}
		compact, errExpected := expectedPacket(commitmentType, packet, ack)
		if errExpected != nil {
			return nil, errors.Wrapf(errExpected, "expected commitment for packet %d", i)
		}
		expectedPackets[i] = compact
	}

	expectedData, err := attestorevm.EncodePacketAttestation(height, expectedPackets)
	if err != nil {
		return nil, errors.Wrap(err, "encoding expected packet attestation")
	}

	result, err := queryPacketQuorum(
		ctx,
		g.logger,
		g.attestors,
		g.threshold,
		encodedPackets,
		height,
		commitmentType,
		expectedData,
	)
	if err != nil {
		return nil, errors.Wrap(err, "querying packet attestation quorum")
	}

	proof, err := attestorevm.EncodeAttestationProof(result.AttestationData, result.Signatures)
	if err != nil {
		return nil, errors.Wrap(err, "encoding packet attestation proof")
	}

	// The attestor returns one shared proof blob covering every packet in the
	// batch; PacketProofs' contract is one proof per input packet, so the same
	// blob is returned len(packets) times.
	proofs := make([][]byte, len(packets))
	for i := range proofs {
		proofs[i] = proof
	}

	return proofs, nil
}

func expectedPacket(
	kind attestor.CommitmentType,
	packet channeltypesv2.Packet,
	acknowledgement channeltypesv2.Acknowledgement,
) (attestorevm.PacketCompact, error) {
	switch kind {
	case attestor.CommitmentTypePacket:
		return attestorevm.PacketCompact{
			Path:       crypto.Keccak256Hash(hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence)),
			Commitment: [32]byte(channeltypesv2.CommitPacket(packet)),
		}, nil
	case attestor.CommitmentTypeReceipt:
		return attestorevm.PacketCompact{
			Path: crypto.Keccak256Hash(hostv2.PacketReceiptKey(packet.DestinationClient, packet.Sequence)),
		}, nil
	case attestor.CommitmentTypeAck:
		path := crypto.Keccak256Hash(hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence))
		commitment := [32]byte(channeltypesv2.CommitAcknowledgement(acknowledgement))
		return attestorevm.PacketCompact{Path: path, Commitment: commitment}, nil
	default:
		return attestorevm.PacketCompact{}, errors.Errorf("unsupported commitment type %v", kind)
	}
}

func commitmentTypeOf(kind v2.ProofKind) (attestor.CommitmentType, error) {
	switch kind {
	case v2.ProofKindPacketCommitment:
		return attestor.CommitmentTypePacket, nil
	case v2.ProofKindAcknowledgement:
		return attestor.CommitmentTypeAck, nil
	case v2.ProofKindReceiptAbsence:
		return attestor.CommitmentTypeReceipt, nil
	default:
		return 0, errors.Errorf("unsupported proof kind %v", kind)
	}
}
