package signer

import (
	"context"
	"errors"
	"testing"

	"github.com/cosmos/ibc/link/internal/tests/mocks"
	"github.com/cosmos/kms/gen/signerservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRemote(t *testing.T) {
	ctx := context.Background()
	keyID := "test-key"
	pubKey := []byte("public-key")

	t.Run("happyPath", func(t *testing.T) {
		// ARRANGE
		ts := newRemoteTestSuite(t)
		message := []byte("message")
		signature := []byte("signature")

		ts.OnKeyRequest(keyID, &signerservice.GetKeyResponse{
			Key: &signerservice.Key{
				Id:     keyID,
				Pubkey: pubKey,
				Scheme: signerservice.SignatureScheme_ED25519,
			},
		}, nil)

		ts.OnSignRequest(keyID, message, &signerservice.SignResponse{
			Signature: signature,
		}, nil)

		signer, err := NewRemote(ctx, ts.Client, keyID)
		require.NoError(t, err)

		// ACT
		actualSignature, err := signer.Sign(ctx, message)

		// ASSERT
		require.NoError(t, err)
		assert.False(t, signer.IsLocal())
		assert.Equal(t, EDDSA, signer.Type())
		assert.Equal(t, pubKey, signer.PublicKey())
		assert.Equal(t, signature, actualSignature)
	})

	t.Run("keyNotFound", func(t *testing.T) {
		// ARRANGE
		ts := newRemoteTestSuite(t)

		ts.OnKeyRequest(keyID, nil, errors.New("key not found"))

		// ACT
		signer, err := NewRemote(ctx, ts.Client, keyID)

		// ASSERT
		require.ErrorContains(t, err, "get key request failed")
		assert.Nil(t, signer)
	})

	t.Run("invalidKeyScheme", func(t *testing.T) {
		// ARRANGE
		ts := newRemoteTestSuite(t)

		ts.OnKeyRequest(keyID, &signerservice.GetKeyResponse{
			Key: &signerservice.Key{
				Id:     keyID,
				Pubkey: pubKey,
				// ecdsa but NOT Ethereum keccak
				Scheme: signerservice.SignatureScheme_ECDSA_SECP256K1,
			},
		}, nil)

		// ACT
		signer, err := NewRemote(ctx, ts.Client, keyID)

		// ASSERT
		require.ErrorContains(t, err, "unsupported remote key scheme")
		assert.Nil(t, signer)
	})
}

type remoteTestSuite struct {
	t      *testing.T
	Client *mocks.MockSignerServiceClient
}

func newRemoteTestSuite(t *testing.T) *remoteTestSuite {
	suite := &remoteTestSuite{
		t:      t,
		Client: mocks.NewMockSignerServiceClient(t),
	}

	return suite
}

func (ts *remoteTestSuite) OnKeyRequest(keyID string, r *signerservice.GetKeyResponse, err error) {
	ts.Client.EXPECT().
		GetKey(mock.Anything, &signerservice.GetKeyRequest{Id: keyID}).
		Return(r, err).
		Once()
}

func (ts *remoteTestSuite) OnSignRequest(keyID string, msg []byte, r *signerservice.SignResponse, err error) {
	ts.Client.EXPECT().
		Sign(mock.Anything, &signerservice.SignRequest{
			KeyId:   keyID,
			Payload: bytesToPayload(msg),
		}).
		Return(r, err).
		Once()
}
