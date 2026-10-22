// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	chainsevm "github.com/cosmos/ibc/cli/internal/chains/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// SimulateRelayTx decodes multicall outcomes in execution order.
func (c *TxSubmitter) SimulateRelayTx(ctx context.Context, tx v2.RelayTx) (v2.RelaySimulation, error) {
	contractABI, err := ibcmulticall.ContractMetaData.GetAbi()
	if err != nil {
		return v2.RelaySimulation{}, err
	}
	multicall := contractABI.Methods["multicall"]
	if len(tx.Data) < 4 || !bytes.Equal(tx.Data[:4], multicall.ID) {
		return v2.RelaySimulation{}, errors.New("expected a multicall transaction")
	}
	args, err := multicall.Inputs.Unpack(tx.Data[4:])
	if err != nil {
		return v2.RelaySimulation{}, errors.Wrap(err, "decoding multicall inputs")
	}
	calls := *abi.ConvertType(args[0], new([]ibcmulticall.IIBCMulticallMsgsCall)).(*[]ibcmulticall.IIBCMulticallMsgsCall)
	to := common.BytesToAddress(tx.To)
	output, err := c.eth.CallContract(
		ctx,
		ethereum.CallMsg{From: c.address, To: &to, Data: tx.Data},
		big.NewInt(int64(rpc.PendingBlockNumber)),
	)
	if err != nil {
		return v2.RelaySimulation{}, errors.Wrap(chainsevm.ExplainRevert(err), "simulating multicall")
	}
	values, err := multicall.Outputs.Unpack(output)
	if err != nil {
		return v2.RelaySimulation{}, errors.Wrap(err, "decoding multicall results")
	}
	results := *abi.ConvertType(values[0], new([]ibcmulticall.IIBCMulticallMsgsResult)).(*[]ibcmulticall.IIBCMulticallMsgsResult)
	if len(results) != len(calls) {
		return v2.RelaySimulation{}, errors.New("multicall result count does not match call count")
	}
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return v2.RelaySimulation{}, err
	}
	simulation := v2.RelaySimulation{Operations: make([]v2.OperationResult, len(results))}
	for i, result := range results {
		outcome := v2.OperationResult{Success: result.Success}
		if !result.Success {
			outcome.FailureReason = chainsevm.ExplainRevertData(result.ReturnData)
		}
		method, methodErr := routerABI.MethodById(calls[i].CallData)
		if methodErr != nil {
			return v2.RelaySimulation{}, errors.Wrapf(methodErr, "decoding multicall call[%d]", i)
		}
		switch method.Name {
		case "updateClient":
			outcome.Type = v2.OperationTypeUpdate
		case "recvPacket", "ackPacket", "timeoutPacket":
			outcome.Type = v2.OperationTypePacket
		default:
			return v2.RelaySimulation{}, errors.Errorf("unsupported relay call[%d]: %s", i, method.Name)
		}
		simulation.Operations[i] = outcome
	}
	return simulation, nil
}
