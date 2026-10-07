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

	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func (c *TxSubmitter) fees(ctx context.Context) (fee, tip *big.Int, err error) {
	head, err := c.header(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	if head.BaseFee == nil {
		return nil, nil, errors.New("chain must support EIP-1559")
	}
	tip, err = call(ctx, c, c.eth.SuggestGasTipCap)
	if err != nil {
		return nil, nil, err
	}
	fee = new(big.Int).Add(tip, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))
	return applyMultiplier(fee, c.opts.GasFeeCapMultiplier), applyMultiplier(tip, c.opts.GasTipCapMultiplier), nil
}

func (c *TxSubmitter) newTx(ctx context.Context, intent v2.TxIntent, nonce uint64) (*types.Transaction, error) {
	if !common.IsHexAddress(intent.To) {
		return nil, errors.New("invalid transaction destination")
	}
	to := common.HexToAddress(intent.To)
	code, err := call(ctx, c, func(ctx context.Context) ([]byte, error) { return c.eth.PendingCodeAt(ctx, to) })
	if err != nil {
		return nil, err
	}
	if len(code) == 0 {
		return nil, errors.Errorf("no contract code at %s", to)
	}
	fee, tip, err := c.fees(ctx)
	if err != nil {
		return nil, err
	}
	gas, err := call(ctx, c, func(ctx context.Context) (uint64, error) {
		return c.eth.EstimateGas(
			ctx,
			ethereum.CallMsg{From: c.address, To: &to, Data: intent.Data, GasFeeCap: fee, GasTipCap: tip},
		)
	})
	if err != nil {
		return nil, err
	}
	return types.NewTx(
		&types.DynamicFeeTx{
			ChainID:   c.ethSigner.ChainID(),
			Nonce:     nonce,
			To:        &to,
			Data:      intent.Data,
			Gas:       gas,
			GasFeeCap: fee,
			GasTipCap: tip,
		},
	), nil
}

func (c *TxSubmitter) signAndSave(ctx context.Context, tx *types.Transaction, kind string, p *pendingTx) error {
	if tx.Gas() < 21000 || tx.Gas() > c.opts.MaxGas || tx.GasFeeCap().Cmp(c.opts.MaxFeePerGas) > 0 ||
		tx.GasTipCap().Cmp(tx.GasFeeCap()) > 0 {
		return errors.New("transaction exceeds gas or fee cap")
	}
	sig, err := call(
		ctx,
		c,
		func(ctx context.Context) ([]byte, error) { return c.signer.Sign(ctx, c.ethSigner.Hash(tx).Bytes()) },
	)
	if err != nil {
		return err
	}
	if len(sig) != crypto.SignatureLength {
		return errors.Errorf("invalid signature length: %d", len(sig))
	}
	signed, err := tx.WithSignature(c.ethSigner, sig)
	if err != nil {
		return err
	}
	sender, err := types.Sender(c.ethSigner, signed)
	if err != nil || sender != c.address {
		return errors.New("transaction signature does not match configured sender")
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return err
	}
	a := store.TxAttempt{
		ChainID:     c.chainID,
		Sender:      c.address.Hex(),
		Hash:        signed.Hash().Hex(),
		Kind:        kind,
		Nonce:       signed.Nonce(),
		SignedTx:    raw,
		SubmittedAt: time.Now().UTC(),
		Status:      store.TxPending,
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	a.ID, err = store.SaveTxAttempt(ctx, c.db, a, p.packets)
	if err != nil {
		return err
	}
	p.attempts = append(p.attempts, signedAttempt{TxAttempt: a, tx: signed})
	return nil
}

func (c *TxSubmitter) broadcast(ctx context.Context, p *pendingTx) {
	a := p.latest()
	at := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	err := c.eth.SendTransaction(ctx, a.tx)
	cancel()
	if err != nil {
		message := strings.ToLower(err.Error())
		if !strings.Contains(message, "already known") && !strings.HasPrefix(message, "known transaction") {
			c.recordEvent("broadcast_error")
			c.logger.Warn("Transaction broadcast failed; retaining signed attempt",
				"txHash", a.Hash, "nonce", a.Nonce, "error", err)
		}
	}
	ctx, cancel = context.WithTimeout(c.ctx, c.opts.RequestTimeout)
	defer cancel()
	if err := c.db.SetTxBroadcast(ctx, c.chainID, a.Hash, at); err != nil {
		c.logger.Error("Recording transaction broadcast", "txHash", a.Hash, "error", err)
		return
	}
	a.LastBroadcastAt = &at
}

func (c *TxSubmitter) reprice(p *pendingTx) error {
	old := p.latest().tx
	fee, tip, err := c.fees(c.ctx)
	if err != nil {
		return err
	}
	fee = maxBig(fee, bump(old.GasFeeCap()))
	tip = maxBig(tip, bump(old.GasTipCap()))
	tx := types.NewTx(
		&types.DynamicFeeTx{
			ChainID:    old.ChainId(),
			Nonce:      old.Nonce(),
			To:         old.To(),
			Value:      old.Value(),
			Data:       old.Data(),
			Gas:        old.Gas(),
			GasFeeCap:  fee,
			GasTipCap:  tip,
			AccessList: old.AccessList(),
		},
	)
	if err = c.signAndSave(c.ctx, tx, p.latest().Kind, p); err != nil {
		return err
	}
	c.recordEvent("replacement")
	return nil
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

func applyMultiplier(v *big.Int, m *float64) *big.Int {
	if m == nil {
		return v
	}
	out, _ := new(big.Float).Mul(new(big.Float).SetInt(v), big.NewFloat(*m)).Int(nil)
	return out
}
