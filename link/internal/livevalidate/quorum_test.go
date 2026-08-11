// SPDX-License-Identifier: Apache-2.0

package livevalidate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

func attestationConnection() config.ConnectionConfig {
	conn := testConnection()
	conn.ClientA.Type = config.ClientTypeAttestation
	conn.ClientB.Type = config.ClientTypeAttestation

	return conn
}

// newLocalSignerConfig generates a real local secp256k1 key, stores it to a
// temp keyfile, and returns both the config entry referencing it and its
// derived EVM address -- resolving a local attestor derives its address the
// same way, so tests need the real address to stage a matching on-chain set.
func newLocalSignerConfig(t *testing.T, alias string) (config.SignerConfig, string) {
	t.Helper()

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), alias+".json")
	require.NoError(t, key.StoreToFile(path))

	address, err := signer.PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	return config.SignerConfig{Alias: alias, Type: config.SignerLocal, File: path}, address
}

// stubChainClient builds a mock Client that only reports its own chain ID,
// enough for local attestor resolution's client-chain-match check.
func stubChainClient(t *testing.T, chainID string) *mocks.MockClient {
	t.Helper()

	client := mocks.NewMockClient(t)
	client.EXPECT().ChainID().Return(chainID).Maybe()

	return client
}

func TestCheckAttestorQuorum(t *testing.T) {
	ctx := context.Background()

	t.Run("resolvesBothDirections", func(t *testing.T) {
		conn := attestationConnection()

		signerB, addressB := newLocalSignerConfig(t, "signer-b")
		signerA, addressA := newLocalSignerConfig(t, "signer-a")

		chainA := stubChainClient(t, conn.ClientA.ChainID)
		chainA.EXPECT().GetAttestationSet(ctx, conn.ClientA.ClientID).Return([]string{addressB}, uint8(1), nil)

		chainB := stubChainClient(t, conn.ClientB.ChainID)
		chainB.EXPECT().GetAttestationSet(ctx, conn.ClientB.ClientID).Return([]string{addressA}, uint8(1), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: chainA,
			conn.ClientB.ChainID: chainB,
		})

		cfg := config.Config{
			Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Signers: config.Signers{signerB, signerA},
			Attestors: config.Attestors{
				// watches chain B, authorized for chain A's client
				{
					Name:    "watches-b",
					Type:    config.AttestorTypeLocal,
					ChainID: conn.ClientB.ChainID,
					Signer:  signerB.Alias,
				},
				// watches chain A, authorized for chain B's client
				{
					Name:    "watches-a",
					Type:    config.AttestorTypeLocal,
					ChainID: conn.ClientA.ChainID,
					Signer:  signerA.Alias,
				},
			},
		}

		require.NoError(t, checkAttestorQuorum(ctx, cfg, clientSet))
	})

	t.Run("insufficientMatchingAttestorsErrors", func(t *testing.T) {
		conn := attestationConnection()

		signerB, _ := newLocalSignerConfig(t, "signer-b")

		selfChain := stubChainClient(t, conn.ClientA.ChainID)
		selfChain.EXPECT().
			GetAttestationSet(ctx, conn.ClientA.ClientID).
			Return([]string{"0xaaa", "0xbbb"}, uint8(2), nil)

		clientSet := chains.NewClientSet(map[string]chains.Client{
			conn.ClientA.ChainID: selfChain,
			conn.ClientB.ChainID: stubChainClient(t, conn.ClientB.ChainID),
		})

		cfg := config.Config{
			Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}},
			Signers: config.Signers{signerB},
			Attestors: config.Attestors{
				{
					Name:    "watches-b",
					Type:    config.AttestorTypeLocal,
					ChainID: conn.ClientB.ChainID,
					Signer:  signerB.Alias,
				},
			},
		}

		err := checkAttestorQuorum(ctx, cfg, clientSet)

		require.ErrorContains(t, err, `only 0 reachable/matching attestors for chain "8453"`)
		require.ErrorContains(t, err, "on-chain quorum requires 2")
	})

	t.Run("nonAttestationClientEndsSkipped", func(t *testing.T) {
		// ARRANGE -- checkAttestorQuorum isn't the right place to reject
		// unsupported client types (that's proofgen's dispatch job); it just
		// skips them.
		conn := testConnection()
		conn.ClientA.Type = "tendermint"
		conn.ClientB.Type = "tendermint"

		cfg := config.Config{Relayer: config.RelayerConfig{Connections: []config.ConnectionConfig{conn}}}

		// ACT + ASSERT -- an empty ClientSet proves neither end was looked up.
		require.NoError(t, checkAttestorQuorum(ctx, cfg, chains.NewClientSet(nil)))
	})
}
