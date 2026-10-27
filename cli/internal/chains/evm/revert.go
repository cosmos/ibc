// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/attestation"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ift"
)

// revertContracts contains known custom errors from relay calls.
// Besu QBFT and IBFT2 share the same errors.
var revertContracts = []*bind.MetaData{
	ics26router.ContractMetaData, attestation.ContractMetaData, besuqbft.ContractMetaData,
	ibcmulticall.ContractMetaData, ift.ContractMetaData,
}

// ExplainRevert adds decoded revert data to an RPC error.
// Any error code is accepted: Besu before 26.x reports
// reverts with -32000 rather than 3.
func ExplainRevert(err error) error {
	var dataErr rpc.DataError
	if !errors.As(err, &dataErr) {
		return err
	}
	hexData, ok := dataErr.ErrorData().(string)
	if !ok {
		return err
	}
	data, decodeErr := hexutil.Decode(hexData)
	if decodeErr != nil || len(data) == 0 {
		return err
	}
	return errors.Wrap(err, ExplainRevertData(data))
}

// ExplainRevertData decodes a revert reason, falling back to its raw hex data.
func ExplainRevertData(data []byte) string {
	if len(data) == 0 {
		return "empty revert data"
	}
	if reason, err := abi.UnpackRevert(data); err == nil {
		return reason
	}
	if len(data) < 4 {
		return hexutil.Encode(data)
	}
	for _, metadata := range revertContracts {
		contractABI, abiErr := metadata.GetAbi()
		if abiErr != nil {
			continue
		}
		customErr, idErr := contractABI.ErrorByID([4]byte(data[:4]))
		if idErr != nil {
			continue
		}
		args, unpackErr := customErr.Inputs.Unpack(data[4:])
		if unpackErr != nil {
			return fmt.Sprintf("%s (malformed data: %s)", customErr.Name, hexutil.Encode(data))
		}
		values := make([]string, len(args))
		for i, arg := range args {
			values[i] = fmt.Sprint(arg)
		}
		return fmt.Sprintf("%s(%s)", customErr.Name, strings.Join(values, ", "))
	}
	return hexutil.Encode(data)
}
