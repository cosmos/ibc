// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/service/signer"
)

func estimateFees(ctx context.Context, eth ETHClient, opts *ChainOptions) (fee, tip *big.Int, err error) {
	head, err := eth.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	if head.BaseFee == nil {
		return nil, nil, errors.New("chain must support EIP-1559")
	}
	tip, err = eth.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, err
	}
	fee = new(big.Int).Add(tip, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))
	return applyMultiplier(fee, opts.GasFeeCapMultiplier), applyMultiplier(tip, opts.GasTipCapMultiplier), nil
}

func signTransaction(
	ctx context.Context,
	tx *types.Transaction,
	chainSigner signer.Signer,
	ethSigner types.Signer,
	address common.Address,
	opts *ChainOptions,
) (*types.Transaction, error) {
	if tx.Gas() < 21000 || tx.Gas() > opts.MaxGas || tx.GasFeeCap().Cmp(opts.MaxFeePerGas) > 0 ||
		tx.GasTipCap().Cmp(tx.GasFeeCap()) > 0 {
		return nil, errors.New("transaction exceeds gas or fee cap")
	}
	sig, err := chainSigner.Sign(ctx, ethSigner.Hash(tx).Bytes())
	if err != nil {
		return nil, err
	}
	if len(sig) != crypto.SignatureLength {
		return nil, errors.Errorf("invalid signature length: %d", len(sig))
	}
	signed, err := tx.WithSignature(ethSigner, sig)
	if err != nil {
		return nil, err
	}
	sender, err := types.Sender(ethSigner, signed)
	if err != nil || sender != address {
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

func applyMultiplier(v *big.Int, m *float64) *big.Int {
	if m == nil {
		return v
	}
	out, _ := new(big.Float).Mul(new(big.Float).SetInt(v), big.NewFloat(*m)).Int(nil)
	return out
}

func (c *TxSubmitter) buildAndSignTransaction(
	ctx context.Context,
	destination string,
	data []byte,
	nonce uint64,
	fee, tip *big.Int,
) (*types.Transaction, error) {
	if !common.IsHexAddress(destination) {
		return nil, errors.New("invalid transaction destination")
	}
	to := common.HexToAddress(destination)
	code, err := c.eth.PendingCodeAt(ctx, to)
	if err != nil {
		return nil, err
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
		return nil, errors.Wrap(explainRevert(err), "estimating gas")
	}
	return signTransaction(ctx, types.NewTx(
		&types.DynamicFeeTx{
			ChainID:   c.ethSigner.ChainID(),
			Nonce:     nonce,
			To:        &to,
			Data:      data,
			Gas:       gas,
			GasFeeCap: fee,
			GasTipCap: tip,
		},
	), c.signer, c.ethSigner, c.address, &c.opts)
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
