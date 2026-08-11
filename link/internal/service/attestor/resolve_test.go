// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

func TestResolveFromConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("resolvesLocalAndSkipsUnreachableRemote", func(t *testing.T) {
		backingSigner, err := signer.GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		signers := signer.NewSet()
		signers.Set("key", backingSigner)

		clients := chains.NewClientSet(map[string]chains.Client{"1": stubChainClient(t, "1")})

		entries := config.Attestors{
			{Name: "alice", Type: config.AttestorTypeLocal, ChainID: "1", Signer: "key"},
			{Name: "bob", Type: config.AttestorTypeRemote, GRPC: "127.0.0.1:0"},
		}

		local, remote, err := ResolveFromConfig(ctx, entries, clients, signers)

		require.NoError(t, err)
		require.Len(t, local, 1)
		require.Equal(t, "alice", local[0].Name())
		require.Empty(t, remote, "an unreachable remote is skipped, not fatal")
	})

	t.Run("remoteMissingGRPCSkipped", func(t *testing.T) {
		entries := config.Attestors{{Name: "bob", Type: config.AttestorTypeRemote}}

		local, remote, err := ResolveFromConfig(ctx, entries, chains.NewClientSet(nil), signer.NewSet())

		require.NoError(t, err)
		require.Empty(t, local)
		require.Empty(t, remote)
	})

	t.Run("unsupportedTypeSkipped", func(t *testing.T) {
		entries := config.Attestors{{Name: "carol", Type: "tendermint"}}

		local, remote, err := ResolveFromConfig(ctx, entries, chains.NewClientSet(nil), signer.NewSet())

		require.NoError(t, err)
		require.Empty(t, local)
		require.Empty(t, remote)
	})

	t.Run("localMissingClientErrorsFatally", func(t *testing.T) {
		signers := signer.NewSet()
		s, err := signer.GenerateLocalSecp256k1Signer()
		require.NoError(t, err)
		signers.Set("key", s)

		entries := config.Attestors{
			{Name: "alice", Type: config.AttestorTypeLocal, ChainID: "unknown-chain", Signer: "key"},
		}

		_, _, err = ResolveFromConfig(ctx, entries, chains.NewClientSet(nil), signers)

		require.ErrorContains(t, err, "attestor alice")
		require.ErrorContains(t, err, "client not found for chain unknown-chain")
	})

	t.Run("localMissingSignerErrorsFatally", func(t *testing.T) {
		clients := chains.NewClientSet(map[string]chains.Client{"1": stubChainClient(t, "1")})

		entries := config.Attestors{
			{Name: "alice", Type: config.AttestorTypeLocal, ChainID: "1", Signer: "missing"},
		}

		_, _, err := ResolveFromConfig(ctx, entries, clients, signer.NewSet())

		require.ErrorContains(t, err, "attestor alice")
		require.ErrorContains(t, err, "unknown signer missing")
	})
}
