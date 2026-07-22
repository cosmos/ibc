package attestor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/internal/config"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	kms "github.com/cosmos/kms/signing/file"
	eth "github.com/ethereum/go-ethereum/core/types"
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
			client       EVMClient
			signer       signer.Signer

			errContains string
		}{
			{
				name:         "ok",
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubEvmClient(t, "chain-1"),
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
				client:       stubEvmClient(t, "chain-2"),
				signer:       ecdsaSigner,
				errContains:  "client chainID mismatch: got chain-2, want chain-1",
			},
			{
				name:         "eddsaSigner",
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubEvmClient(t, "chain-1"),
				signer:       eddsaSigner,
				errContains:  "ECDSA signer required, got eddsa",
			},
			{
				name:         "emptyChainID",
				attestorName: "alice",
				chainID:      "",
				client:       stubEvmClient(t, "chain-1"),
				signer:       ecdsaSigner,
				errContains:  "chainID required",
			},
			{
				name:         "emptyName",
				attestorName: "",
				chainID:      "chain-1",
				client:       stubEvmClient(t, "chain-1"),
				signer:       ecdsaSigner,
				errContains:  "name required",
			},
			{
				name:         "nilSigner",
				signer:       nil,
				attestorName: "alice",
				chainID:      "chain-1",
				client:       stubEvmClient(t, "chain-1"),
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
			header         *eth.Header
			rpcErr         error
			expectedBlock  *big.Int
			expectedHeight uint64
			errContains    string
		}{
			{
				name:           "finalized block",
				header:         &eth.Header{Number: big.NewInt(100)},
				expectedBlock:  blockFinalized,
				expectedHeight: 100,
			},
			{
				name:           "latest block minus offset",
				finalityOffset: 10,
				header:         &eth.Header{Number: big.NewInt(100)},
				expectedBlock:  blockLatest,
				expectedHeight: 90,
			},
			{
				name:           "offset greater than latest block",
				finalityOffset: 101,
				header:         &eth.Header{Number: big.NewInt(100)},
				expectedBlock:  blockLatest,
			},
			{
				name:          "rpc error",
				rpcErr:        errors.New("rpc down"),
				expectedBlock: blockFinalized,
				errContains:   "rpc down",
			},
			{
				name:          "missing header",
				expectedBlock: blockFinalized,
				errContains:   "header is nil",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				client := stubEvmClient(t, "chain-1")
				client.EXPECT().
					HeaderByNumber(mock.Anything, tt.expectedBlock).
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
			client := stubEvmClient(t, "chain-1")
			client.EXPECT().
				HeaderByNumber(mock.Anything, blockFinalized).
				Return(&eth.Header{Number: big.NewInt(100)}, nil).
				Once()
			client.EXPECT().
				HeaderByNumber(mock.Anything, new(big.Int).SetUint64(height)).
				Return(&eth.Header{
					Number: new(big.Int).SetUint64(height),
					Time:   timestamp,
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
			client := stubEvmClient(t, "chain-1")
			client.EXPECT().
				HeaderByNumber(mock.Anything, blockFinalized).
				Return(&eth.Header{Number: big.NewInt(41)}, nil).
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

		t.Run("rejectsMissingHistoricalHeader", func(t *testing.T) {
			// ARRANGE
			client := stubEvmClient(t, "chain-1")
			client.EXPECT().
				HeaderByNumber(mock.Anything, blockFinalized).
				Return(&eth.Header{Number: big.NewInt(100)}, nil).
				Once()
			client.EXPECT().
				HeaderByNumber(mock.Anything, big.NewInt(42)).
				Return(nil, nil).
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
	})
}

func stubEvmClient(t *testing.T, chainID string) *mocks.MockEVMClient {
	t.Helper()

	client := mocks.NewMockEVMClient(t)
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
