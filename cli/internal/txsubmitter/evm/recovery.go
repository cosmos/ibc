// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"maps"
	"math/big"
	"slices"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/store"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func (c *TxSubmitter) header(ctx context.Context, number *big.Int) (*types.Header, error) {
	return call(ctx, c, func(ctx context.Context) (*types.Header, error) { return c.eth.HeaderByNumber(ctx, number) })
}

func (c *TxSubmitter) nonce(number *big.Int) (uint64, error) {
	return call(c.ctx, c, func(ctx context.Context) (uint64, error) { return c.eth.NonceAt(ctx, c.address, number) })
}

func (c *TxSubmitter) poll() {
	pending, err := c.loadPending()
	if err != nil {
		c.logger.Warn("Loading transaction journal", "error", err)
		return
	}
	defer c.observe(pending)
	if len(pending) == 0 {
		return
	}
	confirmedHeight, confirmedNonce, err := c.confirmedNonce()
	if err != nil {
		c.logger.Warn("Reading confirmed nonce", "error", err)
		return
	}
	mined, err := c.nonce(nil)
	if err != nil {
		c.logger.Warn("Reading mined nonce", "error", err)
		return
	}
	for _, nonce := range slices.Sorted(maps.Keys(pending)) {
		p := pending[nonce]
		if nonce < confirmedNonce {
			if err := c.resolveNonce(p, confirmedHeight); err != nil {
				c.logger.Warn("Resolving transaction nonce", "nonce", nonce, "error", err)
				continue
			}
			delete(pending, nonce)
			continue
		}
		if nonce < mined {
			continue
		}
		if p.latest().LastBroadcastAt == nil {
			c.broadcast(c.ctx, p)
			continue
		}
		if nonce == mined && time.Since(p.latest().SubmittedAt) >= c.opts.RepriceInterval &&
			time.Since(c.lastReprice) >= c.opts.RepriceInterval {
			if err := c.reprice(p); err != nil {
				c.recordEvent("replacement_blocked")
				c.logger.Warn("Transaction replacement blocked", "nonce", nonce, "error", err)
			}
			// Back off even when signing or persistence fails.
			c.lastReprice = time.Now()
		}
		if p.latest().LastBroadcastAt == nil || time.Since(*p.latest().LastBroadcastAt) >= c.opts.PollInterval {
			c.broadcast(c.ctx, p)
		}
	}
}

func (c *TxSubmitter) resolveNonce(p *pendingTx, confirmedHeight *big.Int) error {
	ctx, cancel := context.WithTimeout(c.ctx, c.opts.RequestTimeout)
	defer cancel()
	receipt, err := c.confirmedReceipt(ctx, p, confirmedHeight)
	if err != nil {
		return err
	}
	outcome := store.TxOutcome{
		ChainID: c.chainID, Sender: c.address.Hex(), Nonce: p.latest().Nonce, Status: store.TxConsumed,
	}
	if receipt != nil {
		outcome.Hash = receipt.TxHash.Hex()
		outcome.Status = store.TxConfirmed
		if receipt.Status == types.ReceiptStatusFailed {
			outcome.Status = store.TxReverted
		}
	}
	if err := c.db.ResolveTxNonce(ctx, outcome); err != nil {
		return err
	}
	if receipt != nil {
		metrics.endTx(c.chainID, receipt)
	}
	for _, a := range p.attempts {
		status := outcome.Status
		if outcome.Hash != "" && a.Hash != outcome.Hash {
			status = store.TxReplaced
		}
		c.recordEvent(status)
		metrics.forgetTx(c.chainID, a.Hash)
	}
	c.recordResolution(p.attempts[0].SubmittedAt)
	c.logger.Info("Transaction nonce resolved", "nonce", outcome.Nonce, "status", outcome.Status, "hash", outcome.Hash)
	return nil
}

func (c *TxSubmitter) confirmedReceipt(ctx context.Context, p *pendingTx, height *big.Int) (*types.Receipt, error) {
	var lookupErr error
	for _, a := range p.attempts {
		receipt, err := c.eth.TransactionReceipt(ctx, a.tx.Hash())
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			lookupErr = err
			continue
		}
		if receipt == nil || receipt.BlockNumber == nil || receipt.TxHash != a.tx.Hash() ||
			receipt.BlockNumber.Cmp(height) > 0 || receipt.Status > types.ReceiptStatusSuccessful {
			lookupErr = errors.Errorf("invalid or unconfirmed receipt for %s", a.Hash)
			continue
		}
		canonical, err := c.eth.BlockHashByNumber(ctx, receipt.BlockNumber.Uint64())
		if err != nil {
			lookupErr = err
			continue
		}
		if canonical != receipt.BlockHash {
			lookupErr = errors.Errorf("noncanonical receipt for %s", a.Hash)
			continue
		}
		return receipt, nil
	}
	return nil, lookupErr
}

// Read the nonce against a stable confirmation block.
func (c *TxSubmitter) confirmedNonce() (*big.Int, uint64, error) {
	number := big.NewInt(int64(rpc.FinalizedBlockNumber))
	if c.opts.Confirmations > 0 {
		head, err := c.header(c.ctx, nil)
		if err != nil {
			return nil, 0, err
		}
		number.Sub(head.Number, new(big.Int).SetUint64(c.opts.Confirmations-1))
		if number.Sign() < 0 {
			number.SetInt64(0)
		}
	}
	head, err := c.header(c.ctx, number)
	if err != nil {
		return nil, 0, err
	}
	hash, err := c.blockHash(head.Number)
	if err != nil {
		return nil, 0, err
	}
	nonce, err := c.nonce(head.Number)
	if err != nil {
		return nil, 0, err
	}
	check, err := c.blockHash(head.Number)
	if err != nil {
		return nil, 0, err
	}
	if hash != check {
		return nil, 0, errors.New("confirmation block changed during nonce query")
	}
	return head.Number, nonce, nil
}

func (c *TxSubmitter) HasPending(ctx context.Context, op v2.OperationMetadata) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.RequestTimeout)
	defer cancel()
	return c.db.HasPendingPacketSubmission(ctx, store.PacketKey{
		SourceChainID: op.SourceChainID, SourceClientID: op.SourceClientID, Sequence: op.Sequence,
	}, op.Kind.String())
}

func (c *TxSubmitter) blockHash(number *big.Int) (common.Hash, error) {
	return call(c.ctx, c, func(ctx context.Context) (common.Hash, error) {
		return c.eth.BlockHashByNumber(ctx, number.Uint64())
	})
}
