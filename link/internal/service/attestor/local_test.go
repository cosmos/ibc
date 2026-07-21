package attestor

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	eth "github.com/ethereum/go-ethereum/core/types"
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
				client := &stubClient{
					chainID: "chain-1",
					header:  tt.header,
					err:     tt.rpcErr,
				}
				attestor, err := NewLocal(config.AttestationConfig{
					ChainID:       "chain-1",
					Name:          "alice",
					FinalityOffset: tt.finalityOffset,
				}, client, ecdsaSigner)
				require.NoError(t, err)

				// ACT
				height, err := attestor.LatestAttestableHeight(context.Background())

				// ASSERT
				assert.Equal(t, tt.expectedBlock, client.requestedNumber)
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
}

type stubClient struct {
	chainID        string
	header         *eth.Header
	err            error
	requestedNumber *big.Int
}

func (c *stubClient) ChainID() string { return c.chainID }

func (c *stubClient) HeaderByNumber(_ context.Context, number *big.Int) (*eth.Header, error) {
	c.requestedNumber = new(big.Int).Set(number)
	return c.header, c.err
}

func stubEvmClient(t *testing.T, chainID string) EVMClient {
	t.Helper()
	return &stubClient{
		chainID: chainID,
		header:  &eth.Header{Number: big.NewInt(1)},
	}
}
