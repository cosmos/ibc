package attestation

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/service/attestor"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Generator implements proofgen.ProofGenerator for one configured
// attestation light client: LatestProvableHeight/StateProof/PacketProofs all
// query the same fixed attestor set with the same quorum threshold
type Generator struct {
	attestors         []attestor.Attestor
	threshold         int
	finalityOffset    uint64
	counterpartyChain chains.Client
}

func New(
	attestors []attestor.Attestor,
	threshold int,
	finalityOffset uint64,
	counterpartyChain chains.Client,
) *Generator {
	return &Generator{
		attestors:         attestors,
		threshold:         threshold,
		finalityOffset:    finalityOffset,
		counterpartyChain: counterpartyChain,
	}
}

// FinalityOffset is the finality offset resolved from this generator's
// matched attestors -- the max reported by any of them.
func (g *Generator) FinalityOffset() uint64 {
	return g.finalityOffset
}

func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	return latestProvableHeight(ctx, g.attestors, g.threshold, g.counterpartyChain)
}

func (g *Generator) StateProof(ctx context.Context, height uint64) ([]byte, error) {
	result, err := queryStateQuorum(ctx, g.attestors, g.threshold, height)
	if err != nil {
		return nil, errors.Wrap(err, "querying state attestation quorum")
	}

	decodedHeight, _, err := attestorevm.DecodeStateAttestation(result.AttestationData)
	if err != nil {
		return nil, errors.Wrap(err, "decoding state attestation quorum result")
	}

	if decodedHeight != height {
		return nil, errors.Errorf(
			"state attestation height %d does not match requested height %d",
			decodedHeight,
			height,
		)
	}

	proof, err := attestorevm.EncodeAttestationProof(attestorevm.AttestationProof{
		AttestationData: result.AttestationData,
		Signatures:      result.Signatures,
	})
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
) ([][]byte, error) {
	commitmentType, err := commitmentTypeOf(kind)
	if err != nil {
		return nil, err
	}

	encodedPackets := make([][]byte, len(packets))

	for i, packet := range packets {
		encoded, errEnc := attestorevm.EncodePacket(packet)
		if errEnc != nil {
			return nil, errors.Wrapf(errEnc, "encoding packet sequence %d", packet.Sequence)
		}

		encodedPackets[i] = encoded
	}

	result, err := queryPacketQuorum(ctx, g.attestors, g.threshold, encodedPackets, height, commitmentType)
	if err != nil {
		return nil, errors.Wrap(err, "querying packet attestation quorum")
	}

	decodedHeight, decodedPackets, err := attestorevm.DecodePacketAttestation(result.AttestationData)
	if err != nil {
		return nil, errors.Wrap(err, "decoding packet attestation quorum result")
	}

	if decodedHeight != height {
		return nil, errors.Errorf(
			"packet attestation height %d does not match requested height %d",
			decodedHeight,
			height,
		)
	}

	if len(decodedPackets) != len(packets) {
		return nil, errors.Errorf(
			"packet attestation returned %d packets, expected %d",
			len(decodedPackets),
			len(packets),
		)
	}

	proof, err := attestorevm.EncodeAttestationProof(attestorevm.AttestationProof{
		AttestationData: result.AttestationData,
		Signatures:      result.Signatures,
	})
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
