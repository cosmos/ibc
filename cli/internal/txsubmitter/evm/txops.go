// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
)

func estimateFees(ctx context.Context, eth ETHClient, opts *ChainOptions) (fee, tip *big.Int, err error) {
	requestCtx, cancel := context.WithTimeout(ctx, opts.RequestTimeout)
	head, err := eth.HeaderByNumber(requestCtx, nil)
	cancel()
	if err != nil {
		return nil, nil, err
	}
	if head.BaseFee == nil {
		return nil, nil, errors.New("chain must support EIP-1559")
	}
	requestCtx, cancel = context.WithTimeout(ctx, opts.RequestTimeout)
	tip, err = eth.SuggestGasTipCap(requestCtx)
	cancel()
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
	ctx, cancel := context.WithTimeout(ctx, opts.RequestTimeout)
	defer cancel()
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

func broadcast(ctx context.Context, eth ETHClient, tx *types.Transaction, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
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

func saveAttempt(
	ctx context.Context,
	db store.Store,
	signed *types.Transaction,
	a store.TxAttempt,
	operations []store.TxOperation,
) error {
	raw, err := signed.MarshalBinary()
	if err != nil {
		return err
	}
	a.Hash, a.Nonce, a.SignedTx = signed.Hash().Hex(), signed.Nonce(), raw
	_, err = store.SaveTxAttempt(ctx, db, a, operations)
	return err
}

func (c *TxSubmitter) buildAndSignTransaction(
	ctx context.Context,
	destination string,
	data []byte,
	nonce uint64,
) (*types.Transaction, error) {
	if !common.IsHexAddress(destination) {
		return nil, errors.New("invalid transaction destination")
	}
	to := common.HexToAddress(destination)
	requestCtx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	code, err := c.eth.PendingCodeAt(requestCtx, to)
	cancel()
	if err != nil {
		return nil, err
	}
	if len(code) == 0 {
		return nil, errors.Errorf("no contract code at %s", to)
	}
	fee, tip, err := estimateFees(ctx, c.eth, &c.opts)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel = context.WithTimeout(ctx, c.opts.RequestTimeout)
	attempts, err := c.db.ListTxAttemptsAtNonce(requestCtx, c.chainID, c.address.Hex(), nonce)
	cancel()
	if err != nil {
		return nil, err
	}
	for _, a := range attempts {
		var previous types.Transaction
		if err = previous.UnmarshalBinary(a.SignedTx); err != nil {
			return nil, err
		}
		fee = maxBig(fee, bump(previous.GasFeeCap()))
		tip = maxBig(tip, bump(previous.GasTipCap()))
	}
	requestCtx, cancel = context.WithTimeout(ctx, c.opts.RequestTimeout)
	// Pending state may already include the transaction this submission replaces.
	gas, err := c.eth.EstimateGasAtBlock(
		requestCtx,
		ethereum.CallMsg{From: c.address, To: &to, Data: data, GasFeeCap: fee, GasTipCap: tip},
		nil,
	)
	cancel()
	if err != nil {
		return nil, err
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
