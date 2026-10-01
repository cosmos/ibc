// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"fmt"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

// revertContracts declare the custom errors a relay transaction can revert
// with: the router's and those of every light client it routes to.
var revertContracts = []*bind.MetaData{
	ics26router.ContractMetaData, attestation.ContractMetaData, besuqbft.ContractMetaData,
}

// explainRevert names the custom error in err's revert data, which nodes
// return undecoded. Any error code is accepted: Besu before 26.x reports
// reverts with -32000 rather than 3.
func explainRevert(err error) error {
	var dataErr rpc.DataError
	if !errors.As(err, &dataErr) {
		return err
	}
	hexData, ok := dataErr.ErrorData().(string)
	if !ok {
		return err
	}
	data, decodeErr := hexutil.Decode(hexData)
	if decodeErr != nil || len(data) < 4 {
		return err
	}
	for _, metadata := range revertContracts {
		contractABI, abiErr := metadata.GetAbi()
		if abiErr != nil {
			return errors.Wrapf(err, "parsing ABI to decode revert: %v", abiErr)
		}
		customErr, idErr := contractABI.ErrorByID([4]byte(data[:4]))
		if idErr != nil {
			continue
		}
		args, unpackErr := customErr.Inputs.Unpack(data[4:])
		if unpackErr != nil {
			return errors.Wrap(err, customErr.Name)
		}
		values := make([]string, len(args))
		for i, arg := range args {
			values[i] = fmt.Sprint(arg)
		}
		return errors.Wrapf(err, "%s(%s)", customErr.Name, strings.Join(values, ", "))
	}
	return err
}
