// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

func (c *TxSubmitter) transactionAttributes(tx *types.Transaction, baseFee *big.Int) []any {
	return []any{
		"chainID", c.chainID, "txHash", tx.Hash().Hex(), "sender", c.address.Hex(),
		"nonce", tx.Nonce(), "gasLimit", tx.Gas(),
		"maxFeePerGasWei", tx.GasFeeCap().String(), "maxPriorityFeePerGasWei", tx.GasTipCap().String(),
		"baseFeePerGasWei", baseFee.String(), "minedNonce", c.lowestUnminedNonce,
	}
}
