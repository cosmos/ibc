// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"

	chainsevm "github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/service/signer"
)

func estimateFees(
	ctx context.Context,
	eth ETHClient,
	baseFee *big.Int,
) (fee, tip *big.Int, err error) {
	if baseFee == nil {
		return nil, nil, errors.New("chain must support EIP-1559")
	}
	tip, err = eth.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting suggested gas tip cap")
	}
	fee = new(big.Int).Add(tip, new(big.Int).Mul(baseFee, big.NewInt(2)))
	return fee, tip, nil
}

func capFees(fee, tip, limit *big.Int) (*big.Int, *big.Int) {
	if fee.Cmp(limit) > 0 {
		fee = limit
	}
	if tip.Cmp(fee) > 0 {
		tip = fee
	}
	return fee, tip
}

func signTransaction(
	ctx context.Context,
	tx *types.Transaction,
	chainSigner signer.Signer,
	ethSigner types.Signer,
	address common.Address,
) (*types.Transaction, error) {
	sig, err := chainSigner.Sign(ctx, ethSigner.Hash(tx).Bytes())
	if err != nil {
		return nil, errors.Wrapf(err, "signing transaction with address %s", address)
	}
	signed, err := tx.WithSignature(ethSigner, sig)
	if err != nil {
		return nil, errors.Wrap(err, "attaching transaction signature")
	}
	sender, err := types.Sender(ethSigner, signed)
	if err != nil {
		return nil, errors.Wrap(err, "recovering transaction sender")
	}
	if sender != address {
		return nil, errors.New("transaction signature does not match configured sender")
	}
	return signed, nil
}

func broadcast(ctx context.Context, eth ETHClient, tx *types.Transaction) error {
	err := eth.SendTransaction(ctx, tx)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "already known") || strings.HasPrefix(message, "known transaction") {
			return nil
		}
	}
	return err
}

func isReplacementUnderpriced(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "replacement transaction underpriced")
}

func (c *TxSubmitter) prepareTransaction(
	ctx context.Context,
	destination string,
	data []byte,
	nonce uint64,
	fee, tip *big.Int,
) (*types.DynamicFeeTx, error) {
	if !common.IsHexAddress(destination) {
		return nil, errors.New("invalid transaction destination")
	}
	to := common.HexToAddress(destination)
	code, err := c.eth.PendingCodeAt(ctx, to)
	if err != nil {
		return nil, errors.Wrapf(err, "getting code at %s", to)
	}
	if len(code) == 0 {
		return nil, errors.Errorf("no contract code at %s", to)
	}
	// Pending state may already include the transaction this submission replaces.
	gas, err := c.eth.EstimateGasAtBlock(
		ctx,
		ethereum.CallMsg{From: c.address, To: &to, Data: data, GasFeeCap: fee, GasTipCap: tip},
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(chainsevm.ExplainRevert(err), "estimating gas")
	}
	return &types.DynamicFeeTx{
		ChainID:   c.ethSigner.ChainID(),
		Nonce:     nonce,
		To:        &to,
		Data:      data,
		Gas:       gas,
		GasFeeCap: fee,
		GasTipCap: tip,
	}, nil
}

// Round up by 20%, including when the previous tip was zero.
func bump(v *big.Int) *big.Int {
	out := new(big.Int).Mul(v, big.NewInt(120))
	out.Add(out, big.NewInt(99))
	out.Div(out, big.NewInt(100))
	return maxBig(out, new(big.Int).Add(v, big.NewInt(1)))
}

func maxBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}
