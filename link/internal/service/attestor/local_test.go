package attestor

import (
	"testing"

	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocal(t *testing.T) {
	ecdsaSigner, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	eddsaSigner, err := signer.GenerateLocalEd25519Signer()
	require.NoError(t, err)

	t.Run("NewLocal", func(t *testing.T) {
		for _, tt := range []struct {
			name string

			attestorName string
			chainID      string
			signer       signer.Signer

			errContains string
		}{
			{
				name:         "ok",
				attestorName: "alice",
				chainID:      "chain-1",
				signer:       ecdsaSigner,
			},
			{
				name:         "eddsaSigner",
				attestorName: "alice",
				chainID:      "chain-1",
				signer:       eddsaSigner,
				errContains:  "ECDSA signer required, got eddsa",
			},
			{
				name:         "emptyChainID",
				attestorName: "alice",
				chainID:      "",
				signer:       ecdsaSigner,
				errContains:  "chainID required",
			},
			{
				name:         "emptyName",
				attestorName: "",
				chainID:      "chain-1",
				signer:       ecdsaSigner,
				errContains:  "name required",
			},
			{
				name:         "nilSigner",
				signer:       nil,
				attestorName: "alice",
				chainID:      "chain-1",
				errContains:  "signer required",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ACT
				attestor, err := NewLocal(tt.chainID, tt.attestorName, tt.signer)

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
}
