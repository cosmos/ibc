// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chainsevm "github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
)

const (
	chainID       = "1"
	routerAddress = "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC"
)

func newTestClient(t *testing.T) (*Client, *mocks.MockETHClient) {
	t.Helper()

	eth := mocks.NewMockETHClient(t)
	evmClient, err := chainsevm.NewWithClient(chainID, eth, routerAddress)
	require.NoError(t, err)
	client, err := New(evmClient)
	require.NoError(t, err)

	return client, eth
}

func TestSealedHeaderRejectsNonQBFT(t *testing.T) {
	ctx := context.Background()
	client, eth := newTestClient(t)

	header := &types.Header{Number: big.NewInt(7), Time: 1700000000, Difficulty: big.NewInt(1), Extra: []byte{1, 2, 3}}
	eth.EXPECT().HeaderByNumber(ctx, big.NewInt(7)).Return(header, nil).Once()

	_, err := client.SealedHeader(ctx, 7)
	require.ErrorContains(t, err, "not a Besu QBFT header")
}

type fakeDataError struct{ data any }

func (e fakeDataError) Error() string  { return "execution reverted" }
func (e fakeDataError) ErrorData() any { return e.data }

func TestLightClientReads(t *testing.T) {
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

		state := besumsgs.IBesuLightClientMsgsClientState{
			IbcRouter:      routerAddr,
			LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: 112},
			TrustingPeriod: 10,
			MaxClockDrift:  15,
		}
		encoded := besumsgs.NewBindings().PackClientState(state)[4:]

		getClientStateCallData, err := clientABI.Pack("getClientState")
		require.NoError(t, err)

		output, err := clientABI.Methods["getClientState"].Outputs.Pack(encoded)
		require.NoError(t, err)

		eth.EXPECT().CallContract(ctx, ethereum.CallMsg{To: &routerAddr, Data: getClientCallData}, (*big.Int)(nil)).
			Return(getClientOutput, nil).Once()
		eth.EXPECT().CallContract(
			ctx, ethereum.CallMsg{To: &lightClientAddress, Data: getClientStateCallData}, (*big.Int)(nil),
		).Return(output, nil).Once()

		got, err := client.ClientState(ctx, "besu-0")
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

		got, err := client.ConsensusStateHash(ctx, "besu-0", 112)
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

		_, err = client.ConsensusStateHash(ctx, "besu-0", 999)
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

		_, err = client.ConsensusStateHash(ctx, "besu-0", 5)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrConsensusStateNotFound)
	})
}

// Test against the deployed contract ABI, independently of the error bindings.
func TestIsConsensusStateNotFound(t *testing.T) {
	contractABI, err := besuqbft.ContractMetaData.GetAbi()
	require.NoError(t, err)
	encodeError := func(name string, args ...any) []byte {
		t.Helper()
		definition := contractABI.Errors[name]
		arguments, packErr := definition.Inputs.Pack(args...)
		require.NoError(t, packErr)
		return append(definition.ID.Bytes()[:4], arguments...)
	}
	missing := encodeError("ConsensusStateNotFound", uint64(999))
	unrelated := encodeError("InvalidRevisionNumber", uint64(1))
	overflow := append([]byte(nil), missing...)
	overflow[4] = 1
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"valid", fakeDataError{data: hexutil.Encode(missing)}, true},
		{"wrapped", fmt.Errorf("RPC call: %w", fakeDataError{data: hexutil.Encode(missing)}), true},
		{"nil", nil, false},
		{"no error data", errors.New("execution reverted"), false},
		{"non-string data", fakeDataError{data: 123}, false},
		{"invalid hex", fakeDataError{data: "0xzz"}, false},
		{"empty", fakeDataError{data: "0x"}, false},
		{"short selector", fakeDataError{data: hexutil.Encode(missing[:3])}, false},
		{"selector only", fakeDataError{data: hexutil.Encode(missing[:4])}, false},
		{"truncated argument", fakeDataError{data: hexutil.Encode(missing[:len(missing)-1])}, false},
		{"uint64 overflow", fakeDataError{data: hexutil.Encode(overflow)}, false},
		{"unrelated error", fakeDataError{data: hexutil.Encode(unrelated)}, false},
		{"unknown selector", fakeDataError{data: "0xffffffff"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isConsensusStateNotFound(tc.err))
		})
	}
}
