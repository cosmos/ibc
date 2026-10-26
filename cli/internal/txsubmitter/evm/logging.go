// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"log/slog"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

type transactionWarning struct {
	hash   string
	reason string
}

func decimalString(value *big.Int) string {
	if value == nil {
		return "unavailable"
	}
	return value.String()
}

func (c *TxSubmitter) transactionAttributes(tx *types.Transaction, baseFee *big.Int) []any {
	return []any{
		"chainID", c.chainID, "txHash", tx.Hash().Hex(), "sender", c.address.Hex(),
		"nonce", tx.Nonce(), "gasLimit", tx.Gas(),
		"maxFeePerGasWei", tx.GasFeeCap().String(), "maxPriorityFeePerGasWei", tx.GasTipCap().String(),
		"baseFeePerGasWei", baseFee.String(), "minedNonce", c.lowestUnminedNonce,
	}
}

func (c *TxSubmitter) warnTransaction(ctx context.Context, hash, reason, message string, attrs ...any) {
	if seen, _ := c.warnings.ContainsOrAdd(transactionWarning{hash: hash, reason: reason}, struct{}{}); seen {
		return
	}
	attrs = append(attrs, "chainID", c.chainID, "txHash", hash, "retryReason", reason)
	slog.WarnContext(ctx, message, attrs...)
}
