// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestSimulateRelayTxDecodesMisbehavior(t *testing.T) {
	submitter, eth := newSubmitter(t)
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	multicallABI, err := ibcmulticall.ContractMetaData.GetAbi()
	require.NoError(t, err)

	update, err := routerABI.Pack("updateClient", "client-0", []byte{1})
	require.NoError(t, err)
	calls := make([]ibcmulticall.IIBCMulticallMsgsCall, 4)
	results := make([]ibcmulticall.IIBCMulticallMsgsResult, len(calls))
	for i := range calls {
		calls[i] = ibcmulticall.IIBCMulticallMsgsCall{Target: common.HexToAddress("0x1111"), CallData: update}
		if i < 3 {
			returned, packErr := routerABI.Methods["updateClient"].Outputs.Pack(uint8(i))
			require.NoError(t, packErr)
			results[i] = ibcmulticall.IIBCMulticallMsgsResult{Success: true, ReturnData: returned}
		}
	}
	data, err := multicallABI.Pack("multicall", calls)
	require.NoError(t, err)
	eth.callOutput, err = multicallABI.Methods["multicall"].Outputs.Pack(results)
	require.NoError(t, err)

	simulation, err := submitter.SimulateRelayTx(
		t.Context(),
		v2.RelayTx{To: common.HexToAddress("0x2222").Bytes(), Data: data},
	)
	require.NoError(t, err)
	require.Equal(t, []v2.OperationResult{
		{Type: v2.OperationTypeUpdate, Success: true},
		{Type: v2.OperationTypeUpdate, Success: true, Misbehavior: true},
		{Type: v2.OperationTypeUpdate, Success: true},
		{Type: v2.OperationTypeUpdate, FailureReason: "empty revert data"},
	}, simulation.Operations)
}
