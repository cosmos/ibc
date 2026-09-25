// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

func hexNodes(nodes [][]byte) []string {
	out := make([]string, len(nodes))
	for i, node := range nodes {
		out[i] = hexutil.Encode(node)
	}

	return out
}

func fixtureAccountResult(t *testing.T) (*gethclient.AccountResult, [][]byte, common.Hash) {
	t.Helper()

	fixture := besutest.MustFixture(t)

	accountNodes, err := fixture.Membership.AccountProofNodes()
	require.NoError(t, err)

	storageNodes, err := fixture.Membership.ProofNodes()
	require.NoError(t, err)

	slot := besu.CommitmentSlot(fixture.Membership.Path)
	value := new(big.Int).SetBytes(fixture.Membership.Value)

	return &gethclient.AccountResult{
		Address:      fixture.RouterAddress,
		AccountProof: hexNodes(accountNodes),
		StorageHash:  common.HexToHash("0x69c8d1758a0375ec0d4ee22f16e3119c84ecb3aaaaaaaaaaaaaaaaaaaaaaaaaa"),
		StorageProof: []gethclient.StorageResult{{Key: slot.Hex(), Value: value, Proof: hexNodes(storageNodes)}},
	}, accountNodes, slot
}

type ethProofAPI struct {
	account common.Address
	keys    []string
	block   string
	result  *gethclient.AccountResult
	err     error
}

func (a *ethProofAPI) GetProof(
	_ context.Context,
	account common.Address,
	keys []string,
	blockNr string,
) (map[string]any, error) {
	a.account = account
	a.keys = keys
	a.block = blockNr

	if a.err != nil {
		return nil, a.err
	}

	return accountResultJSON(a.result), nil
}

func accountResultJSON(r *gethclient.AccountResult) map[string]any {
	storage := make([]map[string]any, len(r.StorageProof))
	for i, proof := range r.StorageProof {
		storage[i] = map[string]any{
			"key":   proof.Key,
			"value": (*hexutil.Big)(proof.Value),
			"proof": proof.Proof,
		}
	}

	return map[string]any{
		"address":      r.Address,
		"accountProof": r.AccountProof,
		"storageHash":  r.StorageHash,
		"storageProof": storage,
	}
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
	result, accountNodes, slot := fixtureAccountResult(t)

	t.Run("propagates client error", func(t *testing.T) {
		client, eth := newTestClient(t)
		eth.EXPECT().
			GetProof(ctx, common.HexToAddress(routerAddress), []string{slot.Hex()}, big.NewInt(114)).
			Return(nil, assert.AnError).
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

	t.Run("no slots", func(t *testing.T) {
		accountOnly := *result
		accountOnly.StorageProof = nil
		api := &ethProofAPI{result: &accountOnly}
		proof, err := clientWithProofAPI(t, api).GetRouterProof(ctx, 114, nil)
		require.NoError(t, err)
		assert.Empty(t, api.keys)
		assert.Empty(t, proof.StorageProofs)
		assert.Equal(t, accountNodes, proof.AccountProof)
	})
}

func TestRouterProofFromResultValidation(t *testing.T) {
	result, _, _ := fixtureAccountResult(t)

	for name, tc := range map[string]struct {
		mutate func(*gethclient.AccountResult)
		want   string
	}{
		"missing": {
			mutate: func(r *gethclient.AccountResult) { r.StorageProof = nil },
			want:   "0 storage proofs returned for 1 slots",
		},
		"extra": {
			mutate: func(r *gethclient.AccountResult) { r.StorageProof = append(r.StorageProof, r.StorageProof[0]) },
			want:   "2 storage proofs returned for 1 slots",
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := *result
			r.StorageProof = append([]gethclient.StorageResult(nil), result.StorageProof...)
			tc.mutate(&r)

			_, err := routerProofFromResult(&r, 1)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// A malformed node is reported with its proof type, index and height.
func TestGetRouterProofRejectsMalformedNodes(t *testing.T) {
	for _, proofType := range []string{"account", "storage"} {
		t.Run(proofType, func(t *testing.T) {
			result, _, slot := fixtureAccountResult(t)
			nodes := []string{"0xc0", "0xzz"}
			if proofType == "account" {
				result.AccountProof = nodes
			} else {
				result.StorageProof[0].Proof = nodes
			}
			client := clientWithProofAPI(t, &ethProofAPI{result: result})
			proof, err := client.GetRouterProof(t.Context(), 114, []common.Hash{slot})
			require.ErrorContains(t, err, proofType+" proof")
			require.ErrorContains(t, err, "node 1")
			require.ErrorContains(t, err, "height 114")
			assert.Empty(t, proof)
		})
	}
}

func TestRouterProofFromResultAllowsEmptyProofArrays(t *testing.T) {
	result, _, _ := fixtureAccountResult(t)
	result.AccountProof = nil
	result.StorageProof[0].Proof = []string{}

	proof, err := routerProofFromResult(result, 1)
	require.NoError(t, err)
	assert.Empty(t, proof.AccountProof)
	require.Len(t, proof.StorageProofs, 1)
	assert.Empty(t, proof.StorageProofs[0])
}
