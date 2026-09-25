// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func hexBytes(nodes [][]byte) []hexutil.Bytes {
	out := make([]hexutil.Bytes, len(nodes))
	for i, node := range nodes {
		out[i] = node
	}

	return out
}

func fixtureProofResult(t *testing.T) (*proofResult, [][]byte, common.Hash) {
	t.Helper()

	fixture := besutest.MustFixture(t)

	accountNodes, err := fixture.Membership.AccountProofNodes()
	require.NoError(t, err)

	storageNodes, err := fixture.Membership.ProofNodes()
	require.NoError(t, err)

	return &proofResult{
		AccountProof: hexBytes(accountNodes),
		StorageProof: []storageProofResult{{Proof: hexBytes(storageNodes)}},
	}, accountNodes, besu.CommitmentSlot(fixture.Membership.Path)
}

type ethProofAPI struct {
	account common.Address
	keys    []string
	block   string
	result  *proofResult
}

func (a *ethProofAPI) GetProof(
	_ context.Context,
	account common.Address,
	keys []string,
	blockNr string,
) (*proofResult, error) {
	a.account = account
	a.keys = keys
	a.block = blockNr

	return a.result, nil
}

func clientWithProofAPI(t *testing.T, api *ethProofAPI) *Client {
	t.Helper()

	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("eth", api))

	client, err := NewWithClient(chainIDEth, newRPCClient(rpc.DialInProc(srv)), routerAddress)
	require.NoError(t, err)

	return client
}

func TestGetRouterProof(t *testing.T) {
	ctx := context.Background()
	result, accountNodes, slot := fixtureProofResult(t)

	t.Run("propagates client error", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().
			GetProof(ctx, common.HexToAddress(routerAddress), []common.Hash{slot}, big.NewInt(114)).
			Return(nil, nil, assert.AnError).
			Once()
		_, err := client.GetRouterProof(ctx, 114, []common.Hash{slot})
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("converts", func(t *testing.T) {
		api := &ethProofAPI{result: result}
		proof, err := clientWithProofAPI(t, api).GetRouterProof(ctx, 114, []common.Hash{slot})
		require.NoError(t, err)
		assert.Equal(t, common.HexToAddress(routerAddress), api.account)
		assert.Equal(t, []string{slot.Hex()}, api.keys)
		assert.Equal(t, hexutil.EncodeUint64(114), api.block)
		assert.Equal(t, accountNodes, proof.AccountProof)
		require.Len(t, proof.StorageProofs, 1)
		assert.NotEmpty(t, proof.StorageProofs[0])
	})

	t.Run("latest", func(t *testing.T) {
		api := &ethProofAPI{result: result}
		_, err := clientWithProofAPI(t, api).GetRouterProof(ctx, v2.LatestBlock, []common.Hash{slot})
		require.NoError(t, err)
		assert.Equal(t, "latest", api.block)
	})

	t.Run("no slots", func(t *testing.T) {
		accountOnly := *result
		accountOnly.StorageProof = nil
		api := &ethProofAPI{result: &accountOnly}
		proof, err := clientWithProofAPI(t, api).GetRouterProof(ctx, 114, nil)
		require.NoError(t, err)
		// sent as [] rather than null, which some nodes reject
		assert.NotNil(t, api.keys)
		assert.Empty(t, api.keys)
		assert.Empty(t, proof.StorageProofs)
		assert.Equal(t, accountNodes, proof.AccountProof)
	})

	t.Run("empty proof arrays", func(t *testing.T) {
		empty := &proofResult{StorageProof: []storageProofResult{{Proof: []hexutil.Bytes{}}}}
		proof, err := clientWithProofAPI(t, &ethProofAPI{result: empty}).GetRouterProof(ctx, 114, []common.Hash{slot})
		require.NoError(t, err)
		assert.Empty(t, proof.AccountProof)
		require.Len(t, proof.StorageProofs, 1)
		assert.Empty(t, proof.StorageProofs[0])
	})
}

func TestGetRouterProofChecksProofCount(t *testing.T) {
	result, _, slot := fixtureProofResult(t)

	for name, tc := range map[string]struct {
		storage []storageProofResult
		want    string
	}{
		"missing": {storage: nil, want: "0 storage proofs returned for 1 slots"},
		"extra":   {storage: append(result.StorageProof, result.StorageProof[0]), want: "2 storage proofs returned for 1 slots"},
	} {
		t.Run(name, func(t *testing.T) {
			r := *result
			r.StorageProof = tc.storage
			_, err := clientWithProofAPI(
				t,
				&ethProofAPI{result: &r},
			).GetRouterProof(t.Context(), 114, []common.Hash{slot})
			require.ErrorContains(t, err, tc.want)
			require.ErrorContains(t, err, "height 114")
		})
	}
}
