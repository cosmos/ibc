// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSealedHeaderRejectsNonQBFT(t *testing.T) {
	ctx := context.Background()
	client, eth := newTestClient(t)

	header := &types.Header{Number: big.NewInt(7), Time: 1700000000, Difficulty: big.NewInt(1), Extra: []byte{1, 2, 3}}
	eth.EXPECT().HeaderByNumber(ctx, big.NewInt(7)).Return(header, nil).Once()

	_, err := client.SealedHeader(ctx, 7)
	require.ErrorContains(t, err, "parse header 7: extra data list")
}

func TestBesuQBFTClientState(t *testing.T) {
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

		got, err := client.BesuQBFTClientState(ctx, "besu-0")
		require.NoError(t, err)
		assert.Equal(t, state, got)
	})
}
