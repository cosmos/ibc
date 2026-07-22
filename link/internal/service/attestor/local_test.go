package attestor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
	"github.com/cosmos/ibc/link/internal/config"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
	kms "github.com/cosmos/kms/signing/file"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
				cfg := config.AttestationConfig{
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
				assert.Equal(t, tt.attestorName, attestor.Alias())
				assert.Equal(t, tt.chainID, attestor.ChainID())
				assert.True(t, attestor.IsLocal())
			})
		}
	})

	t.Run("LatestAttestableHeight", func(t *testing.T) {
		for _, tt := range []struct {
			name           string
			finalityOffset uint
			header         v2.BlockHeader
			rpcErr         error
			expectedHeightArg uint64
			expectedHeight uint64
			errContains    string
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

				attestor, err := NewLocal(config.AttestationConfig{
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

			attestor, err := NewLocal(config.AttestationConfig{
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
				config.AttestationConfig{ChainID: "chain-1", Name: "alice"},
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
				config.AttestationConfig{ChainID: "chain-1", Name: "alice"},
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
		validPacket := encodedPacket(t)
		decodedPacket, err := attestorevm.DecodePacket(validPacket)
		require.NoError(t, err)
		pathHash := attestorevm.PathHash(attestorevm.PathPacket(decodedPacket.SourceClient, decodedPacket.Sequence))
		packetCommitment := attestorevm.PacketCommitment(decodedPacket)

		for _, tt := range []struct {
			name            string
			request         PacketAttestationRequest
			latestHeader    *v2.BlockHeader
			latestHeightErr error
			commitment      *[32]byte
			expectedErr     error
			errContains     string
		}{
			{
				name: "rejectsEmptyPacketBatch",
				request: PacketAttestationRequest{
					CommitmentType: CommitmentTypePacket,
				},
				expectedErr: ErrInvalidInput,
				errContains: "packet count 0",
			},
			{
				name: "rejectsPacketBatchAboveLimit",
				request: PacketAttestationRequest{
					Packets:        make([][]byte, MaxPacketsPerAttestation+1),
					CommitmentType: CommitmentTypePacket,
				},
				expectedErr: ErrInvalidInput,
				errContains: "packet count 101",
			},
			{
				name: "rejectsMalformedPacket",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{{1, 2, 3}},
					CommitmentType: CommitmentTypePacket,
				},
				expectedErr: ErrInvalidInput,
				errContains: "decode packet 0",
			},
			{
				name: "rejectsUnsupportedCommitmentType",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypeInvalid,
				},
				expectedErr: ErrInvalidInput,
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
				expectedErr:  ErrNotFinalized,
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
				name: "acceptsValidRequest",
				request: PacketAttestationRequest{
					Height:         10,
					Packets:        [][]byte{validPacket},
					CommitmentType: CommitmentTypePacket,
				},
				latestHeader: &v2.BlockHeader{Height: 10},
				commitment:   &packetCommitment,
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
					config.AttestationConfig{ChainID: "chain-1", Name: "alice"},
					client,
					ecdsaSigner,
				)
				require.NoError(t, err)

				// ACT
				result, err := attestor.PacketAttestation(context.Background(), tt.request)

				// ASSERT
				if tt.expectedErr != nil {
					require.ErrorIs(t, err, tt.expectedErr)
					assert.Empty(t, result)
					if tt.errContains != "" {
						assert.ErrorContains(t, err, tt.errContains)
					}
					return
				}
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					assert.Empty(t, result)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.request.Height, result.Height)
			})
		}
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

func encodedPacket(t *testing.T) []byte {
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
