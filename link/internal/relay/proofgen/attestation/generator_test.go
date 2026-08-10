// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	attestordomain "github.com/cosmos/ibc/link/attestor"
	attestorevm "github.com/cosmos/ibc/link/attestor/evm"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// signedStateAttestor builds a attestor.MockAttestor that answers
// StateAttestation with a validly-signed claim at height.
func signedStateAttestor(t *testing.T, name string, height uint64) *attestor.MockAttestor {
	t.Helper()

	data, err := attestorevm.EncodeStateAttestation(height, 1700000000)
	require.NoError(t, err)

	digest := attestorevm.Digest(attestorevm.TagStateAttestation, data)

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
		attestordomain.Attestation{Height: height, AttestedData: data, Signature: sig}, nil,
	)

	return a
}

// signedPacketAttestor builds a attestor.MockAttestor that answers
// PacketAttestation with a validly-signed claim covering packets.
func signedPacketAttestor(
	t *testing.T,
	name string,
	height uint64,
	packets []attestorevm.PacketCompact,
) *attestor.MockAttestor {
	t.Helper()

	dataArgs, err := attestorevm.EncodePacketAttestation(height, packets)
	require.NoError(t, err)

	digest := attestorevm.Digest(attestorevm.TagPacketAttestation, dataArgs)

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().PacketAttestation(mock.Anything, mock.Anything).Return(
		attestordomain.Attestation{Height: height, AttestedData: dataArgs, Signature: sig}, nil,
	)

	return a
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

	packets := []channeltypesv2.Packet{
		{Sequence: 1, SourceClient: "src-0", DestinationClient: "dst-0", TimeoutTimestamp: 1000},
		{Sequence: 2, SourceClient: "src-0", DestinationClient: "dst-0", TimeoutTimestamp: 1000},
	}

	compact := []attestorevm.PacketCompact{
		{Path: [32]byte{1}, Commitment: [32]byte{2}},
		{Path: [32]byte{3}, Commitment: [32]byte{4}},
	}

	t.Run("returnsOneProofPerPacket", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedPacketAttestor(t, "a1", 20, compact),
			signedPacketAttestor(t, "a2", 20, compact),
		}

		gen := New(attestors, 2, nil)

		proofs, err := gen.PacketProofs(ctx, 20, v2.ProofKindPacketCommitment, packets)
		require.NoError(t, err)
		require.Len(t, proofs, len(packets))
		require.Equal(t, proofs[0], proofs[1], "the shared attestation blob is duplicated across every packet index")
	})

	t.Run("unsupportedKindErrors", func(t *testing.T) {
		// an unsupported kind must be rejected before ever querying an
		// attestor, so the generator here is given no attestors at all.
		gen := New(nil, 2, nil)

		_, err := gen.PacketProofs(ctx, 20, v2.ProofKindUnknown, packets)
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
	counterpartyChain.EXPECT().
		GetBlockHeader(mock.Anything, uint64(90)).
		Return(v2.BlockHeader{Timestamp: someBlockTime}, nil)

	gen := New(attestors, 2, counterpartyChain)

	height, timestamp, err := gen.LatestProvableHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(90), height)
	require.Equal(t, someBlockTime, timestamp)
}
