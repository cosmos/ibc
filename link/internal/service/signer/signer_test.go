package signer

import (
	"context"
	"testing"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/stretchr/testify/require"
)

func TestSignerSet(t *testing.T) {
	t.Run("setGet", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		signerSet := NewSet()

		signerA, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		signerB, err := GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		signerC, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		remoteTS := newRemoteTestSuite(t)
		remoteTS.OnKeyRequest("remote-key", &signerservice.GetKeyResponse{
			Key: &signerservice.Key{
				Id:     "remote-key",
				Pubkey: []byte("remote-public-key"),
				Scheme: signerservice.SignatureScheme_ED25519,
			},
		}, nil)

		remoteSignerD, err := NewRemote(ctx, remoteTS.Client, "remote-key")
		require.NoError(t, err)

		signerSet.Set("A", signerA)
		signerSet.Set("B", signerB)
		signerSet.Set("C", signerC)
		signerSet.Set("D", remoteSignerD)

		// ACT
		actualSigner, found := signerSet.Get("A")
		missingSigner, missingFound := signerSet.Get("E")

		// ASSERT
		require.True(t, found)
		require.Same(t, signerA, actualSigner)
		require.Nil(t, missingSigner)
		require.False(t, missingFound)
	})
}
