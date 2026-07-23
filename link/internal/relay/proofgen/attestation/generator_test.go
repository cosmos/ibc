package attestation

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// signedStateAttestor builds a attestor.MockAttestor that answers
// StateAttestation with a validly-signed claim at height.
func signedStateAttestor(t *testing.T, name string, height uint64) *attestor.MockAttestor {
	t.Helper()

	data, err := encodeStateAttestation(StateAttestation{Height: height, Timestamp: 1700000000})
	require.NoError(t, err)

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	digest := taggedDigest(data, attestationTypeState)
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
		attestor.Attestation{Height: height, AttestedData: data, Signature: sig}, nil,
	)

	return a
}

// signedPacketAttestor builds a attestor.MockAttestor that answers
// PacketAttestation with a validly-signed claim covering packets.
func signedPacketAttestor(t *testing.T, name string, height uint64, packets []PacketCompact) *attestor.MockAttestor {
	t.Helper()

	dataArgs := abiArgsFor(t, PacketAttestation{Height: height, Packets: packets})

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	digest := taggedDigest(dataArgs, attestationTypePacket)
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().PacketAttestation(mock.Anything, mock.Anything).Return(
		attestor.Attestation{Height: height, AttestedData: dataArgs, Signature: sig}, nil,
	)

	return a
}

func abiArgsFor(t *testing.T, p PacketAttestation) []byte {
	t.Helper()

	encoded, err := packetAttestationArgs.Pack(p)
	require.NoError(t, err)

	return encoded
}

func TestGeneratorStateProof(t *testing.T) {
	ctx := context.Background()

	t.Run("returnsEncodedProofAtRequestedHeight", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedStateAttestor(t, "a1", 10),
			signedStateAttestor(t, "a2", 10),
		}

		gen := New(attestors, 2, nil)

		proof, err := gen.StateProof(ctx, 10)
		require.NoError(t, err)
		require.NotEmpty(t, proof)
	})

	t.Run("mismatchedHeightErrors", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedStateAttestor(t, "a1", 10),
			signedStateAttestor(t, "a2", 10),
		}

		gen := New(attestors, 2, nil)

		_, err := gen.StateProof(ctx, 11)
		require.Error(t, err)
	})
}

func TestGeneratorPacketProofs(t *testing.T) {
	ctx := context.Background()

	packets := []v2.Packet{
		{Sequence: 1, SourceClient: "src-0", DestClient: "dst-0", TimeoutTimestamp: 1000},
		{Sequence: 2, SourceClient: "src-0", DestClient: "dst-0", TimeoutTimestamp: 1000},
	}

	compact := []PacketCompact{
		{Path: [32]byte{1}, Commitment: [32]byte{2}},
		{Path: [32]byte{3}, Commitment: [32]byte{4}},
	}

	t.Run("returnsOneProofPerPacket", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedPacketAttestor(t, "a1", 20, compact),
			signedPacketAttestor(t, "a2", 20, compact),
		}

		gen := New(attestors, 2, nil)

		proofs, err := gen.PacketProofs(ctx, 20, proofgen.KindPacketCommitment, packets)
		require.NoError(t, err)
		require.Len(t, proofs, len(packets))
		require.Equal(t, proofs[0], proofs[1], "the shared attestation blob is duplicated across every packet index")
	})

	t.Run("unsupportedKindErrors", func(t *testing.T) {
		// an unsupported kind must be rejected before ever querying an
		// attestor, so the generator here is given no attestors at all.
		gen := New(nil, 2, nil)

		_, err := gen.PacketProofs(ctx, 20, proofgen.KindUnknown, packets)
		require.Error(t, err)
	})
}

func TestGeneratorLatestProvableHeight(t *testing.T) {
	ctx := context.Background()

	attestors := []attestor.Attestor{
		heightAttestor(t, "a1", 100),
		heightAttestor(t, "a2", 90),
	}

	counterpartyChain := mocks.NewMockClient(t)
	counterpartyChain.EXPECT().BlockTimestamp(mock.Anything, uint64(90)).Return(someBlockTime, nil)

	gen := New(attestors, 2, counterpartyChain)

	height, timestamp, err := gen.LatestProvableHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(90), height)
	require.Equal(t, someBlockTime, timestamp)
}
