// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
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

func fixtureAccountResult(t *testing.T) (*gethclient.AccountResult, [][]byte, [32]byte, *big.Int) {
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
	}, accountNodes, slot, value
}

type ethProofAPI struct {
	account common.Address
	keys    []string
	block   string
	result  *gethclient.AccountResult
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

	client, err := NewWithClient(chainIDEth, ethclient.NewClient(rpc.DialInProc(srv)), routerAddress)
	require.NoError(t, err)

	return client
}

func TestGetRouterProof(t *testing.T) {
	ctx := context.Background()
	result, accountNodes, slot, value := fixtureAccountResult(t)

	t.Run("requires RPC client", func(t *testing.T) {
		client, _ := newTestClient(t)
		_, err := client.GetRouterProof(ctx, 114, [][32]byte{slot})
		require.ErrorContains(t, err, "eth_getProof")
	})

	t.Run("converts by key", func(t *testing.T) {
		api := &ethProofAPI{result: result}
		proof, err := clientWithProofAPI(t, api).GetRouterProof(ctx, 114, [][32]byte{slot})
		require.NoError(t, err)
		assert.Equal(t, common.HexToAddress(routerAddress), api.account)
		assert.Equal(t, []string{common.Hash(slot).Hex()}, api.keys)
		assert.Equal(t, hexutil.EncodeUint64(114), api.block)
		assert.Equal(t, accountNodes, proof.AccountProof)
		require.Len(t, proof.StorageProofs, 1)
		assert.Equal(t, slot, proof.StorageProofs[0].Key)
		assert.Equal(t, value, proof.StorageProofs[0].Value)
		assert.NotEmpty(t, proof.StorageProofs[0].Proof)
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

func TestAccountProofFromResultValidation(t *testing.T) {
	result, _, slot, _ := fixtureAccountResult(t)
	other := common.HexToHash("0x01")

	t.Run("reordered results are matched by key", func(t *testing.T) {
		two := *result
		two.StorageProof = []gethclient.StorageResult{
			{Key: other.Hex(), Value: big.NewInt(0), Proof: nil},
			result.StorageProof[0],
		}

		proof, err := accountProofFromResult(&two, [][32]byte{slot, other})
		require.NoError(t, err)
		assert.Equal(t, slot, proof.StorageProofs[0].Key)
		assert.Equal(t, [32]byte(other), proof.StorageProofs[1].Key)
	})

	for _, key := range []string{"0x01", "0x0", "0x1"} {
		t.Run("short key "+key, func(t *testing.T) {
			short := *result
			short.StorageProof = []gethclient.StorageResult{{Key: key, Value: big.NewInt(0)}}
			slot := common.HexToHash(key)
			proof, err := accountProofFromResult(&short, [][32]byte{slot})
			require.NoError(t, err)
			assert.Equal(t, [32]byte(slot), proof.StorageProofs[0].Key)
		})
	}

	for name, tc := range map[string]struct {
		mutate func(*gethclient.AccountResult)
		slots  [][32]byte
		want   string
	}{
		"missing": {
			mutate: func(r *gethclient.AccountResult) { r.StorageProof = nil },
			slots:  [][32]byte{slot},
			want:   "no storage proof returned",
		},
		"duplicate returned": {
			mutate: func(r *gethclient.AccountResult) { r.StorageProof = append(r.StorageProof, r.StorageProof[0]) },
			slots:  [][32]byte{slot},
			want:   "duplicate storage proof",
		},
		"nil value": {
			mutate: func(r *gethclient.AccountResult) { r.StorageProof[0].Value = nil },
			slots:  [][32]byte{slot},
			want:   "invalid value",
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := *result
			r.StorageProof = append([]gethclient.StorageResult(nil), result.StorageProof...)
			tc.mutate(&r)

			_, err := accountProofFromResult(&r, tc.slots)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestSealedHeaderRejectsNonQBFT(t *testing.T) {
	ctx := context.Background()
	client, eth := newTestClient(t)

	header := &types.Header{Number: big.NewInt(7), Time: 1700000000, Difficulty: big.NewInt(1), Extra: []byte{1, 2, 3}}
	eth.EXPECT().HeaderByNumber(ctx, big.NewInt(7)).Return(header, nil).Once()

	_, err := client.SealedHeader(ctx, 7)
	require.ErrorContains(t, err, "not a Besu QBFT header")
}

type fakeDataError struct{ data string }

func (e fakeDataError) Error() string  { return "execution reverted" }
func (e fakeDataError) ErrorData() any { return e.data }

func TestBesuQBFTReads(t *testing.T) {
	ctx := context.Background()
	lightClientAddress := common.HexToAddress("0x00000000000000000000000000000000000abc")
	routerAddr := common.HexToAddress(routerAddress)

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	clientABI, err := besuqbft.ContractMetaData.GetAbi()
	require.NoError(t, err)

	getClientCallData, err := routerABI.Pack("getClient", "besu-0")
	require.NoError(t, err)

	getClientOutput, err := routerABI.Methods["getClient"].Outputs.Pack(lightClientAddress)
	require.NoError(t, err)

	t.Run("client state", func(t *testing.T) {
		client, eth := newTestClient(t)

		state := besu.ClientState{IBCRouter: routerAddr, LatestHeight: 112, TrustingPeriod: 10, MaxClockDrift: 15}
		encoded, err := besutest.EncodeClientState(state)
		require.NoError(t, err)

		getClientStateCallData, err := clientABI.Pack("getClientState")
		require.NoError(t, err)

		output, err := clientABI.Methods["getClientState"].Outputs.Pack(encoded)
		require.NoError(t, err)

		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil)).
			Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &lightClientAddress, Data: getClientStateCallData}, (*big.Int)(nil),
		).Return(output, nil).Once()

		got, err := client.GetBesuQBFTClientState(ctx, "besu-0")
		require.NoError(t, err)
		assert.Equal(t, state, got)
	})

	t.Run("consensus state hash", func(t *testing.T) {
		client, eth := newTestClient(t)

		callData, err := clientABI.Pack("getConsensusStateHash", uint64(112))
		require.NoError(t, err)

		want := common.HexToHash("0xe90678de9bc0821f64b0634c05ba8dc7726ff865d40fbf283d3bdb32ac38d4b9")
		output, err := clientABI.Methods["getConsensusStateHash"].Outputs.Pack(want)
		require.NoError(t, err)

		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil)).
			Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &lightClientAddress, Data: callData}, (*big.Int)(nil)).
			Return(output, nil).Once()

		got, err := client.GetBesuQBFTConsensusStateHash(ctx, "besu-0", 112)
		require.NoError(t, err)
		assert.Equal(t, [32]byte(want), got)
	})

	t.Run("consensus state not found", func(t *testing.T) {
		client, eth := newTestClient(t)

		callData, err := clientABI.Pack("getConsensusStateHash", uint64(999))
		require.NoError(t, err)

		revert, err := clientABI.Errors["ConsensusStateNotFound"].Inputs.Pack(uint64(999))
		require.NoError(t, err)
		revert = append(clientABI.Errors["ConsensusStateNotFound"].ID.Bytes()[:4], revert...)

		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil)).
			Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &lightClientAddress, Data: callData}, (*big.Int)(nil)).
			Return(nil, fakeDataError{data: hexutil.Encode(revert)}).Once()

		_, err = client.GetBesuQBFTConsensusStateHash(ctx, "besu-0", 999)
		require.ErrorIs(t, err, ErrConsensusStateNotFound)
	})

	t.Run("other revert is not mistaken for not found", func(t *testing.T) {
		client, eth := newTestClient(t)

		callData, err := clientABI.Pack("getConsensusStateHash", uint64(5))
		require.NoError(t, err)

		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil)).
			Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &lightClientAddress, Data: callData}, (*big.Int)(nil)).
			Return(nil, errors.New("boom")).Once()

		_, err = client.GetBesuQBFTConsensusStateHash(ctx, "besu-0", 5)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrConsensusStateNotFound)
	})
}
