// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	kms "github.com/cosmos/kms/signing/file"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
	"github.com/cosmos/ibc/link/internal/config"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

func TestLocal(t *testing.T) {
	ecdsaSigner := generateECDSASigner(t)

	eddsaSigner, err := signer.GenerateLocalEd25519Signer()
	require.NoError(t, err)

	t.Run("NewLocal", func(t *testing.T) {
		for _, tt := range []struct {
			name string

			attestorName string
			chainID      string
			client       chains.Client
			signer       signer.Signer

			errContains string
		}{
			{
				name:         "ok",
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubChainClient(t, "chain-1"),
				signer:       ecdsaSigner,
			},
			{
				name:         "nilClient",
				attestorName: "alice",
				chainID:      "chain-1",
				client:       nil,
				signer:       ecdsaSigner,
				errContains:  "client required",
			},
			{
				name:         "clientChainIDMismatch",
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubChainClient(t, "chain-2"),
				signer:       ecdsaSigner,
				errContains:  "client chainID mismatch: got chain-2, want chain-1",
			},
			{
				name:         "eddsaSigner",
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubChainClient(t, "chain-1"),
				signer:       eddsaSigner,
				errContains:  "ECDSA signer required, got eddsa",
			},
			{
				name:         "emptyChainID",
				attestorName: "alice",
				chainID:      "",
				client:       stubChainClient(t, "chain-1"),
				signer:       ecdsaSigner,
				errContains:  "chainID required",
			},
			{
				name:         "emptyName",
				attestorName: "",
				chainID:      "chain-1",
				client:       stubChainClient(t, "chain-1"),
				signer:       ecdsaSigner,
				errContains:  "name required",
			},
			{
				name:         "nilSigner",
				signer:       nil,
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubChainClient(t, "chain-1"),
				errContains:  "signer required",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ACT
				cfg := config.AttestorConfig{
					ChainID: tt.chainID,
					Name:    tt.attestorName,
				}
				attestor, err := NewLocal(cfg, tt.client, tt.signer)

				// ASSERT
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					return
				}

				require.NoError(t, err)
				require.NotNil(t, attestor)
				assert.Equal(t, tt.attestorName, attestor.Name())
				assert.Equal(t, tt.chainID, attestor.ChainID())
				assert.True(t, attestor.IsLocal())
			})
		}
	})

	t.Run("LatestAttestableHeight", func(t *testing.T) {
		for _, tt := range []struct {
			name              string
			finalityOffset    uint
			header            v2.BlockHeader
			rpcErr            error
			expectedHeightArg uint64
			expectedHeight    uint64
			errContains       string
		}{
			{
				name:              "finalizedBlock",
				header:            v2.BlockHeader{Height: 100},
				expectedHeightArg: v2.FinalizedBlock,
				expectedHeight:    100,
			},
			{
				name:              "latestBlockMinusOffset",
				finalityOffset:    10,
				header:            v2.BlockHeader{Height: 100},
				expectedHeightArg: v2.LatestBlock,
				expectedHeight:    90,
			},
			{
				name:              "offsetGreaterThanLatestBlock",
				finalityOffset:    101,
				header:            v2.BlockHeader{Height: 100},
				expectedHeightArg: v2.LatestBlock,
				errContains:       "latest height 100 does not exceed finality offset 101",
			},
			{
				name:              "rpcError",
				rpcErr:            errors.New("rpc down"),
				expectedHeightArg: v2.FinalizedBlock,
				errContains:       "rpc down",
			},
			{
				name:              "missingHeader",
				rpcErr:            errors.New("header is nil for height 18446744073709551614"),
				expectedHeightArg: v2.FinalizedBlock,
				errContains:       "header is nil",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				client := stubChainClient(t, "chain-1")
				client.EXPECT().
					GetBlockHeader(mock.Anything, tt.expectedHeightArg).
					Return(tt.header, tt.rpcErr).
					Once()

				attestor, err := NewLocal(config.AttestorConfig{
					ChainID:        "chain-1",
					Name:           "alice",
					FinalityOffset: tt.finalityOffset,
				}, client, ecdsaSigner)
				require.NoError(t, err)

				// ACT
				height, err := attestor.LatestHeight(context.Background())

				// ASSERT
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					assert.Zero(t, height)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.expectedHeight, height)
			})
		}
	})

	t.Run("StateAttestation", func(t *testing.T) {
		t.Run("signsFinalizedState", func(t *testing.T) {
			// ARRANGE
			const (
				height    = uint64(42)
				timestamp = uint64(1_700_000_000)
			)
			client := stubChainClient(t, "chain-1")
			client.EXPECT().
				GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
				Return(v2.BlockHeader{Height: 100}, nil).
				Once()
			client.EXPECT().
				GetBlockHeader(mock.Anything, height).
				Return(v2.BlockHeader{
					Height:    height,
					Timestamp: time.Unix(int64(timestamp), 0).UTC(),
				}, nil).
				Once()

			attestor, err := NewLocal(config.AttestorConfig{
				ChainID: "chain-1",
				Name:    "alice",
			}, client, ecdsaSigner)
			require.NoError(t, err)

			// ACT
			result, err := attestor.StateAttestation(context.Background(), height)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, height, result.Height)
			require.NotNil(t, result.Timestamp)
			assert.Equal(t, time.Unix(int64(timestamp), 0).UTC(), *result.Timestamp)

			expectedData, err := attestorevm.EncodeStateAttestation(height, timestamp)
			require.NoError(t, err)
			assert.Equal(t, expectedData, result.AttestedData)

			require.Len(t, result.Signature, 65)
			assert.Contains(t, []byte{27, 28}, result.Signature[64])
			innerHash := sha256.Sum256(result.AttestedData)
			signingInput := append([]byte{0x01}, innerHash[:]...)
			expectedDigest := sha256.Sum256(signingInput)
			assertSignatureFromSigner(t, ecdsaSigner, expectedDigest, result.Signature)
		})

		t.Run("rejectsUnfinalizedHeight", func(t *testing.T) {
			// ARRANGE
			client := stubChainClient(t, "chain-1")
			client.EXPECT().
				GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
				Return(v2.BlockHeader{Height: 41}, nil).
				Once()

			attestor, err := NewLocal(
				config.AttestorConfig{ChainID: "chain-1", Name: "alice"},
				client,
				ecdsaSigner,
			)
			require.NoError(t, err)

			// ACT
			result, err := attestor.StateAttestation(context.Background(), 42)

			// ASSERT
			require.ErrorIs(t, err, ErrNotFinalized)
			assert.Empty(t, result)
		})

		t.Run("propagatesHistoricalHeaderError", func(t *testing.T) {
			// ARRANGE
			client := stubChainClient(t, "chain-1")
			client.EXPECT().
				GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
				Return(v2.BlockHeader{Height: 100}, nil).
				Once()
			client.EXPECT().
				GetBlockHeader(mock.Anything, uint64(42)).
				Return(v2.BlockHeader{}, errors.New("header is nil for height 42")).
				Once()

			attestor, err := NewLocal(
				config.AttestorConfig{ChainID: "chain-1", Name: "alice"},
				client,
				ecdsaSigner,
			)
			require.NoError(t, err)

			// ACT
			result, err := attestor.StateAttestation(context.Background(), 42)

			// ASSERT
			require.ErrorContains(t, err, "header is nil for height 42")
			assert.Empty(t, result)
		})
	})

	t.Run("PacketAttestation", func(t *testing.T) {
		validPacket := sampleEvmPacket(t)
		decodedPacket, err := attestorevm.DecodePacket(validPacket)
		require.NoError(t, err)
		pathHash := [32]byte(
			crypto.Keccak256Hash(hostv2.PacketCommitmentKey(decodedPacket.SourceClient, decodedPacket.Sequence)),
		)
		ackPathHash := [32]byte(
			crypto.Keccak256Hash(
				hostv2.PacketAcknowledgementKey(decodedPacket.DestinationClient, decodedPacket.Sequence),
			),
		)
		receiptPathHash := [32]byte(
			crypto.Keccak256Hash(hostv2.PacketReceiptKey(decodedPacket.DestinationClient, decodedPacket.Sequence)),
		)
		packetCommitment := [32]byte(channeltypesv2.CommitPacket(decodedPacket))

		for _, tt := range []struct {
			name            string
			request         PacketAttestationRequest
			latestHeader    *v2.BlockHeader
			latestHeightErr error
			commitment      *[32]byte
			errContains     string
		}{
			{
				name: "rejectsEmptyPacketBatch",
				request: PacketAttestationRequest{
					CommitmentType: CommitmentTypePacket,
				},
				errContains: "packet count 0",
			},
			{
				name: "rejectsPacketBatchAboveLimit",
				request: PacketAttestationRequest{
					Packets:        make([][]byte, MaxPacketsPerAttestation+1),
					CommitmentType: CommitmentTypePacket,
				},
				errContains: "packet count 101",
			},
			{
				name: "rejectsMalformedPacket",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{{1, 2, 3}},
					CommitmentType: CommitmentTypePacket,
				},
				errContains: "decode packet 0",
			},
			{
				name: "rejectsZeroHeight",
				request: PacketAttestationRequest{
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				errContains: "height must be greater than 0",
			},
			{
				name: "rejectsLatestHeight",
				request: PacketAttestationRequest{
					Height:         v2.LatestBlock,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				errContains: "invalid height",
			},
			{
				name: "rejectsFinalizedHeight",
				request: PacketAttestationRequest{
					Height:         uint64(v2.FinalizedBlock),
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				errContains: "invalid height",
			},
			{
				name: "rejectsUnsupportedCommitmentType",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypeInvalid,
				},
				errContains: "unsupported commitment type 0",
			},
			{
				name: "rejectsUnfinalizedHeight",
				request: PacketAttestationRequest{
					Height:         11,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				latestHeader: &v2.BlockHeader{Height: 10},
				errContains:  "block is not finalized",
			},
			{
				name: "returnsLatestHeightError",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				latestHeightErr: errors.New("rpc down"),
				errContains:     "get latest attestable height: rpc down",
			},
			{
				name: "returnsCommitmentNotFound",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				latestHeader: &v2.BlockHeader{Height: 10},
				commitment:   new([32]byte),
				errContains:  `packet commitment for client "source-client" sequence 7`,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				client := stubChainClient(t, "chain-1")
				if tt.latestHeader != nil || tt.latestHeightErr != nil {
					var header v2.BlockHeader
					if tt.latestHeader != nil {
						header = *tt.latestHeader
					}
					client.EXPECT().
						GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
						Return(header, tt.latestHeightErr).
						Once()
				}
				if tt.commitment != nil {
					client.EXPECT().
						GetCommitment(mock.Anything, tt.request.Height, pathHash).
						Return(*tt.commitment, nil).
						Once()
				}
				attestor, err := NewLocal(
					config.AttestorConfig{ChainID: "chain-1", Name: "alice"},
					client,
					ecdsaSigner,
				)
				require.NoError(t, err)

				// ACT
				result, err := attestor.PacketAttestation(context.Background(), tt.request)

				// ASSERT
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					require.Empty(t, result)
					return
				}

				require.NoError(t, err)
				require.Equal(t, tt.request.Height, result.Height)
			})
		}

		t.Run("signsPacket", func(t *testing.T) {
			// ARRANGE
			const height uint64 = 10

			client := stubChainClient(t, "chain-1")
			client.EXPECT().
				GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
				Return(v2.BlockHeader{Height: height}, nil).
				Once()
			client.EXPECT().
				GetCommitment(mock.Anything, height, pathHash).
				Return(packetCommitment, nil).
				Once()

			attestor, err := NewLocal(
				config.AttestorConfig{ChainID: "chain-1", Name: "alice"},
				client,
				ecdsaSigner,
			)
			require.NoError(t, err)

			// Given expected data that we'll compare against the result.
			expectedData, err := attestorevm.EncodePacketAttestation(height, []attestorevm.PacketCompact{
				{Path: pathHash, Commitment: packetCommitment},
			})
			require.NoError(t, err)

			// ACT
			result, err := attestor.PacketAttestation(context.Background(), PacketAttestationRequest{
				Height:         height,
				Packets:        [][]byte{validPacket},
				CommitmentType: CommitmentTypePacket,
			})

			// ASSERT
			require.NoError(t, err)

			require.Equal(t, height, result.Height)
			require.Nil(t, result.Timestamp)
			require.Equal(t, expectedData, result.AttestedData)

			// Check signature
			require.Len(t, result.Signature, 65)
			require.Contains(t, []byte{27, 28}, result.Signature[64])
			innerHash := sha256.Sum256(result.AttestedData)
			signingInput := append([]byte{0x02}, innerHash[:]...)
			expectedDigest := sha256.Sum256(signingInput)
			assertSignatureFromSigner(t, ecdsaSigner, expectedDigest, result.Signature)
		})

		t.Run("commitmentSemantics", func(t *testing.T) {
			for _, tt := range []struct {
				name           string
				commitmentType CommitmentType
				path           [32]byte
				commitment     [32]byte
				errContains    string
			}{
				{
					name:           "rejectsPacketMismatch",
					commitmentType: CommitmentTypePacket,
					path:           pathHash,
					commitment:     [32]byte{1},
					errContains:    "packet commitment mismatch",
				},
				{
					name:           "acceptsPresentAck",
					commitmentType: CommitmentTypeAck,
					path:           ackPathHash,
					commitment:     [32]byte{1},
				},
				{
					name:           "rejectsMissingAck",
					commitmentType: CommitmentTypeAck,
					path:           ackPathHash,
					errContains:    "acknowledgement commitment",
				},
				{
					name:           "acceptsMissingReceipt",
					commitmentType: CommitmentTypeReceipt,
					path:           receiptPathHash,
				},
				{
					name:           "rejectsPresentReceipt",
					commitmentType: CommitmentTypeReceipt,
					path:           receiptPathHash,
					commitment:     [32]byte{1},
					errContains:    "receipt exists",
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					// ARRANGE
					const height = uint64(10)
					client := stubChainClient(t, "chain-1")
					client.EXPECT().
						GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
						Return(v2.BlockHeader{Height: height}, nil).
						Once()
					client.EXPECT().
						GetCommitment(mock.Anything, height, tt.path).
						Return(tt.commitment, nil).
						Once()
					attestor, err := NewLocal(
						config.AttestorConfig{ChainID: "chain-1", Name: "alice"},
						client,
						ecdsaSigner,
					)
					require.NoError(t, err)

					// ACT
					result, err := attestor.PacketAttestation(context.Background(), PacketAttestationRequest{
						Height:         height,
						Packets:        [][]byte{validPacket},
						CommitmentType: tt.commitmentType,
					})

					// ASSERT
					if tt.errContains != "" {
						require.ErrorContains(t, err, tt.errContains)
						assert.Empty(t, result)
						return
					}

					require.NoError(t, err)
					expectedData, err := attestorevm.EncodePacketAttestation(height, []attestorevm.PacketCompact{
						{Path: tt.path, Commitment: tt.commitment},
					})
					require.NoError(t, err)
					assert.Equal(t, expectedData, result.AttestedData)
					assert.NotEmpty(t, result.Signature)
				})
			}
		})
	})
}

func stubChainClient(t *testing.T, chainID string) *mocks.MockClient {
	t.Helper()

	client := mocks.NewMockClient(t)
	client.EXPECT().ChainID().Return(chainID).Maybe()

	return client
}

func generateECDSASigner(t *testing.T) *signer.LocalSecp256k1Signer {
	t.Helper()

	kmsSigner, err := kms.GenerateSecp256k1Eth(rand.Reader)
	require.NoError(t, err)
	privateKey, err := kms.PrivateKeyFromSigner(kmsSigner)
	require.NoError(t, err)
	localSigner, err := signer.NewLocalSecp256k1Signer(privateKey)
	require.NoError(t, err)

	return localSigner
}

func sampleEvmPacket(t *testing.T) []byte {
	t.Helper()

	contractABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	encoded, err := contractABI.Methods["isPacketReceived"].Inputs.Pack(
		ics26router.IICS26RouterMsgsPacket{
			Sequence:         7,
			SourceClient:     "source-client",
			DestClient:       "destination-client",
			TimeoutTimestamp: 1_700_000_000,
			Payloads: []ics26router.IICS26RouterMsgsPayload{
				{
					SourcePort: "transfer",
					DestPort:   "transfer",
					Version:    "ics20-1",
					Encoding:   "application/json",
					Value:      []byte("payload"),
				},
			},
		},
	)
	require.NoError(t, err)

	return encoded
}

func assertSignatureFromSigner(
	t *testing.T,
	expectedSigner *signer.LocalSecp256k1Signer,
	digest [sha256.Size]byte,
	signature []byte,
) {
	t.Helper()

	recoverySignature := append([]byte(nil), signature...)
	recoverySignature[64] -= 27
	publicKey, err := crypto.SigToPub(digest[:], recoverySignature)
	require.NoError(t, err)

	expectedPublicKey, err := crypto.DecompressPubkey(expectedSigner.PublicKey())
	require.NoError(t, err)
	assert.Equal(t, crypto.PubkeyToAddress(*expectedPublicKey), crypto.PubkeyToAddress(*publicKey))
}
