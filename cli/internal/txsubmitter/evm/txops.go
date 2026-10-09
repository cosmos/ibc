// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"log/slog"
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

type pendingTx struct {
	attempts []signedAttempt
	packets  []store.PacketKey
}

type signedAttempt struct {
	store.TxAttempt
	tx *types.Transaction
}

func (p *pendingTx) latest() *signedAttempt { return &p.attempts[len(p.attempts)-1] }

func (p *pendingTx) hashes() []common.Hash {
	hashes := make([]common.Hash, len(p.attempts))
	for i, a := range p.attempts {
		hashes[i] = a.tx.Hash()
	}
	return hashes
}

func estimateFees(ctx context.Context, eth ETHClient, opts *ChainOptions) (fee, tip *big.Int, err error) {
	head, err := call(ctx, opts.RequestTimeout, func(ctx context.Context) (*types.Header, error) {
		return eth.HeaderByNumber(ctx, nil)
	})
	if err != nil {
		return nil, nil, err
	}
	if head.BaseFee == nil {
		return nil, nil, errors.New("chain must support EIP-1559")
	}
	tip, err = call(ctx, opts.RequestTimeout, eth.SuggestGasTipCap)
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
	sig, err := call(
		ctx,
		opts.RequestTimeout,
		func(ctx context.Context) ([]byte, error) { return chainSigner.Sign(ctx, ethSigner.Hash(tx).Bytes()) },
	)
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
	p *pendingTx,
) error {
	raw, err := signed.MarshalBinary()
	if err != nil {
		return err
	}
	a.Hash = signed.Hash().Hex()
	a.Nonce = signed.Nonce()
	a.SignedTx = raw
	a.Status = store.TxPending
	a.ID, err = store.SaveTxAttempt(ctx, db, a, p.packets)
	if err != nil {
		return err
	}
	p.attempts = append(p.attempts, signedAttempt{TxAttempt: a, tx: signed})
	return nil
}

func broadcastAttempt(
	ctx, recordCtx context.Context,
	eth ETHClient,
	db store.Store,
	a *signedAttempt,
	at time.Time,
	timeout time.Duration,
) {
	logger := slog.With("module", "txsubmitter", "chainID", a.ChainID, "wallet", a.Sender)
	at = at.UTC()
	if err := broadcast(ctx, eth, a.tx, timeout); err != nil {
		logger.Warn(
			"Transaction broadcast failed; retaining signed attempt",
			"txHash",
			a.Hash,
			"nonce",
			a.Nonce,
			"error",
			err,
		)
	}
	recordCtx, cancel := context.WithTimeout(recordCtx, timeout)
	defer cancel()
	if err := db.SetTxBroadcast(recordCtx, a.ChainID, a.Hash, at); err != nil {
		logger.Error("Recording transaction broadcast", "txHash", a.Hash, "error", err)
		return
	}
	a.LastBroadcastAt = &at
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
	code, err := call(
		ctx,
		c.opts.RequestTimeout,
		func(ctx context.Context) ([]byte, error) { return c.eth.PendingCodeAt(ctx, to) },
	)
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
	gas, err := call(ctx, c.opts.RequestTimeout, func(ctx context.Context) (uint64, error) {
		return c.eth.EstimateGas(
			ctx,
			ethereum.CallMsg{From: c.address, To: &to, Data: data, GasFeeCap: fee, GasTipCap: tip},
		)
	})
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
