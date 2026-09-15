// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
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
		attestor.Attestation{Height: height, AttestedData: data, Signature: sig}, nil,
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
		attestor.Attestation{Height: height, AttestedData: dataArgs, Signature: sig}, nil,
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

		chain := mocks.NewMockClient(t)
		chain.EXPECT().
			GetBlockHeader(mock.Anything, uint64(10)).
			Return(v2.BlockHeader{Timestamp: someBlockTime}, nil).
			Once()
		gen := New(attestors, 2, chain, slog.Default())

		proof, err := gen.StateProof(ctx, 10)
		require.NoError(t, err)
		require.NotEmpty(t, proof)
	})

	t.Run("mismatchedHeightErrors", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedStateAttestor(t, "a1", 10),
			signedStateAttestor(t, "a2", 10),
		}

		chain := mocks.NewMockClient(t)
		chain.EXPECT().
			GetBlockHeader(mock.Anything, uint64(11)).
			Return(v2.BlockHeader{Timestamp: someBlockTime}, nil).
			Once()
		gen := New(attestors, 2, chain, slog.Default())

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

	compact := make([]attestorevm.PacketCompact, len(packets))
	for i, packet := range packets {
		compact[i] = attestorevm.PacketCompact{
			Path:       crypto.Keccak256Hash(hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence)),
			Commitment: [32]byte(channeltypesv2.CommitPacket(packet)),
		}
	}

	t.Run("returnsOneProofPerPacket", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedPacketAttestor(t, "a1", 20, compact),
			signedPacketAttestor(t, "a2", 20, compact),
		}

		gen := New(attestors, 2, nil, slog.Default())

		proofs, err := gen.PacketProofs(ctx, 20, v2.ProofKindPacketCommitment, packets)
		require.NoError(t, err)
		require.Len(t, proofs, len(packets))
		require.Equal(t, proofs[0], proofs[1], "the shared attestation blob is duplicated across every packet index")
	})

	t.Run("unsupportedKindErrors", func(t *testing.T) {
		// an unsupported kind must be rejected before ever querying an
		// attestor, so the generator here is given no attestors at all.
		gen := New(nil, 2, nil, slog.Default())

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

	gen := New(attestors, 2, counterpartyChain, slog.Default())

	height, timestamp, err := gen.LatestProvableHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(90), height)
	require.Equal(t, someBlockTime, timestamp)
}

func TestGeneratorRejectsUnexpectedPacketClaims(t *testing.T) {
	packets := []channeltypesv2.Packet{
		{Sequence: 1, SourceClient: "src-0", DestinationClient: "dst-0", TimeoutTimestamp: 1000},
		{Sequence: 2, SourceClient: "src-0", DestinationClient: "dst-0", TimeoutTimestamp: 1000},
	}

	tests := []struct {
		name    string
		height  uint64
		mutate  func([]attestorevm.PacketCompact) []attestorevm.PacketCompact
		wantErr bool
	}{
		{name: "valid", height: 20},
		{
			name: "path", height: 20, wantErr: true,
			mutate: func(claims []attestorevm.PacketCompact) []attestorevm.PacketCompact {
				claims[0].Path[0] ^= 1
				return claims
			},
		},
		{
			name: "commitment", height: 20, wantErr: true,
			mutate: func(claims []attestorevm.PacketCompact) []attestorevm.PacketCompact {
				claims[0].Commitment[0] ^= 1
				return claims
			},
		},
		{
			name: "reordered", height: 20, wantErr: true,
			mutate: func(claims []attestorevm.PacketCompact) []attestorevm.PacketCompact {
				return []attestorevm.PacketCompact{claims[1], claims[0]}
			},
		},
		{
			name: "duplicate", height: 20, wantErr: true,
			mutate: func(claims []attestorevm.PacketCompact) []attestorevm.PacketCompact {
				return []attestorevm.PacketCompact{claims[0], claims[0]}
			},
		},
		{
			name: "missing", height: 20, wantErr: true,
			mutate: func(claims []attestorevm.PacketCompact) []attestorevm.PacketCompact {
				return claims[:1]
			},
		},
		{name: "height", height: 21, wantErr: true},
	}

	for _, kind := range []v2.ProofKind{v2.ProofKindPacketCommitment, v2.ProofKindAcknowledgement, v2.ProofKindReceiptAbsence} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%v/%s", kind, tt.name), func(t *testing.T) {
				chain := mocks.NewMockClient(t)
				claims := make([]attestorevm.PacketCompact, len(packets))
				for i, packet := range packets {
					switch kind {
					case v2.ProofKindPacketCommitment:
						claims[i].Path = crypto.Keccak256Hash(
							hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence),
						)
						claims[i].Commitment = [32]byte(channeltypesv2.CommitPacket(packet))
					case v2.ProofKindAcknowledgement:
						claims[i].Path = crypto.Keccak256Hash(
							hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence),
						)
						claims[i].Commitment = [32]byte{byte(i + 1)}
						chain.EXPECT().
							GetCommitment(mock.Anything, uint64(20), claims[i].Path).
							Return(claims[i].Commitment, nil).
							Once()
					case v2.ProofKindReceiptAbsence:
						claims[i].Path = crypto.Keccak256Hash(
							hostv2.PacketReceiptKey(packet.DestinationClient, packet.Sequence),
						)
					}
				}
				if tt.mutate != nil {
					claims = tt.mutate(claims)
				}
				gen := New([]attestor.Attestor{
					signedPacketAttestor(t, "a1", tt.height, claims),
					signedPacketAttestor(t, "a2", tt.height, claims),
				}, 2, chain, slog.Default())

				proofs, err := gen.PacketProofs(context.Background(), 20, kind, packets)
				if !tt.wantErr {
					require.NoError(t, err)
					require.Len(t, proofs, len(packets))
				} else {
					require.ErrorContains(t, err, "attested data does not match expected claim")
					require.Nil(t, proofs)
				}
			})
		}
	}
}

func TestGeneratorStateTimestampMismatch(t *testing.T) {
	chain := mocks.NewMockClient(t)
	chain.EXPECT().
		GetBlockHeader(mock.Anything, uint64(10)).
		Return(v2.BlockHeader{Timestamp: someBlockTime.Add(time.Second)}, nil).
		Once()
	gen := New([]attestor.Attestor{signedStateAttestor(t, "a1", 10)}, 1, chain, slog.Default())

	proof, err := gen.StateProof(context.Background(), 10)
	require.ErrorContains(t, err, "attested data does not match expected claim")
	require.Nil(t, proof)
}

func TestGeneratorExpectedClaimLookupFailure(t *testing.T) {
	t.Run("stateHeader", func(t *testing.T) {
		chain := mocks.NewMockClient(t)
		chain.EXPECT().GetBlockHeader(mock.Anything, uint64(10)).Return(v2.BlockHeader{}, assert.AnError).Once()
		gen := New(nil, 1, chain, slog.Default())

		proof, err := gen.StateProof(context.Background(), 10)
		require.ErrorIs(t, err, assert.AnError)
		require.Nil(t, proof)
	})

	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "ackLookupError", err: assert.AnError},
		{name: "missingAck"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			packet := channeltypesv2.Packet{
				Sequence:          1,
				SourceClient:      "src-0",
				DestinationClient: "dst-0",
				TimeoutTimestamp:  1000,
			}
			path := crypto.Keccak256Hash(hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence))
			chain := mocks.NewMockClient(t)
			chain.EXPECT().GetCommitment(mock.Anything, uint64(20), [32]byte(path)).Return([32]byte{}, tt.err).Once()
			gen := New(nil, 1, chain, slog.Default())

			proofs, err := gen.PacketProofs(
				context.Background(),
				20,
				v2.ProofKindAcknowledgement,
				[]channeltypesv2.Packet{packet},
			)
			require.Error(t, err)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.ErrorContains(t, err, "acknowledgement commitment not found")
			}
			require.Nil(t, proofs)
		})
	}
}
