// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// sourceChain is the source-Chain RPC surface the generator drives.
type sourceChain interface {
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	// BlockReceipts returns every receipt in block number.
	BlockReceipts(ctx context.Context, number uint64) ([]*types.Receipt, error)
	// TransactionReceipt returns ethereum.NotFound for an unmined transaction.
	TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
}

// txBuilder signs the transfer wallet sends at nonce in one dispatch.
type txBuilder func(wallet int, nonce uint64) (*types.Transaction, error)

// prepareFunc readies one dispatch at head: it fixes per-dispatch inputs
// (fees, packet timeout) shared by every transaction of the dispatch.
type prepareFunc func(ctx context.Context, head *types.Header) (txBuilder, error)

type generatorConfig struct {
	// Packets is the number of transfers to attempt. Each attempt is one
	// transaction emitting one packet.
	Packets int
	// PerBlock caps the transfers dispatched per new head.
	PerBlock int
	// MinDispatchInterval debounces dispatches to at most one per interval.
	// Heads are the trigger, but a mixed-mining Chain mines a block per
	// arriving transaction, so pacing on heads alone would feed back on
	// itself; the Chain's nominal block interval restores per-block pacing.
	MinDispatchInterval time.Duration
	// StaleAfter is how long a sent transaction may stay unmined before the
	// generator asks for its receipt directly and, if none exists, counts it
	// dropped and frees its wallet.
	StaleAfter time.Duration
	// DrainTimeout bounds the wait for outstanding transactions after the
	// last dispatch.
	DrainTimeout time.Duration
	// SendConcurrency bounds concurrent submissions within a dispatch.
	SendConcurrency int
}

// included is one of the generator's transactions observed in a block.
type included struct {
	Receipt    *types.Receipt
	SentAt     time.Time
	IncludedAt time.Time
}

// generatorStats counts what the generator did.
type generatorStats struct {
	Attempted    int
	Sent         int
	SendErrors   int
	Included     int
	Reverted     int
	Dropped      int
	Backpressure int
	HeadsSeen    int
	Dispatches   int
	FirstSend    time.Time
	LastSend     time.Time
	// SendToInclusion holds one sample per included transaction.
	SendToInclusion []time.Duration
}

type walletState struct {
	nonce      uint64
	nonceKnown bool
	inFlight   bool
}

type pendingTx struct {
	wallet int
	nonce  uint64
	sentAt time.Time
}

// generator is an open-loop, head-paced transaction generator for one source
// Chain. Each wallet has at most one transaction in flight, so a sender never
// queues behind itself in the transaction pool. One goroutine owns all state.
type generator struct {
	cfg     generatorConfig
	chain   sourceChain
	prepare prepareFunc
	wallets []common.Address
	label   string
	now     func() time.Time

	states    []walletState
	cursor    int
	pending   map[common.Hash]pendingTx
	remaining int
	stats     generatorStats

	lastProcessed    uint64
	haveProcessed    bool
	lastDispatch     time.Time
	drainDeadline    time.Time
	blockReceiptsOff bool
}

func newGenerator(
	label string,
	cfg generatorConfig,
	chain sourceChain,
	wallets []common.Address,
	prepare prepareFunc,
) (*generator, error) {
	switch {
	case cfg.Packets <= 0:
		return nil, errors.New("packets must be positive")
	case cfg.PerBlock <= 0:
		return nil, errors.New("per-block must be positive")
	case len(wallets) == 0:
		return nil, errors.New("at least one wallet is required")
	case cfg.StaleAfter <= 0 || cfg.DrainTimeout <= 0:
		return nil, errors.New("stale and drain timeouts must be positive")
	}
	cfg.SendConcurrency = max(cfg.SendConcurrency, 1)
	return &generator{
		cfg:       cfg,
		chain:     chain,
		prepare:   prepare,
		wallets:   wallets,
		label:     label,
		now:       time.Now,
		states:    make([]walletState, len(wallets)),
		pending:   map[common.Hash]pendingTx{},
		remaining: cfg.Packets,
	}, nil
}

// run dispatches on heads until every transfer was attempted and every sent
// transaction was included or written off. Included transactions go to out,
// which must have room for cfg.Packets entries so run never blocks on it.
func (g *generator) run(
	ctx context.Context,
	heads <-chan *types.Header,
	subscriptionErr <-chan error,
	out chan<- included,
) (generatorStats, error) {
	sweep := time.NewTicker(g.cfg.StaleAfter / 2)
	defer sweep.Stop()
	for !g.done() {
		select {
		case <-ctx.Done():
			return g.stats, ctx.Err()
		case err := <-subscriptionErr:
			return g.stats, fmt.Errorf("head subscription: %w", err)
		case head := <-heads:
			if err := g.onHead(ctx, head, out); err != nil {
				return g.stats, err
			}
		case <-sweep.C:
			// Heads normally drive sweeps; the ticker covers a quiet Chain.
			g.sweepStale(ctx, out)
			g.checkDrain()
		}
	}
	return g.stats, nil
}

func (g *generator) done() bool {
	return g.remaining == 0 && len(g.pending) == 0
}

func (g *generator) onHead(ctx context.Context, head *types.Header, out chan<- included) error {
	seenAt := g.now()
	g.stats.HeadsSeen++
	g.collect(ctx, head.Number.Uint64(), seenAt, out)
	g.sweepStale(ctx, out)
	if g.remaining > 0 && (g.lastDispatch.IsZero() || seenAt.Sub(g.lastDispatch) >= g.cfg.MinDispatchInterval) {
		if err := g.dispatch(ctx, head, seenAt); err != nil {
			return err
		}
	}
	g.checkDrain()
	return nil
}

// collect settles pending transactions mined in blocks up to head.
func (g *generator) collect(ctx context.Context, head uint64, seenAt time.Time, out chan<- included) {
	from := head
	if g.haveProcessed {
		from = g.lastProcessed + 1
	}
	if len(g.pending) == 0 {
		g.lastProcessed, g.haveProcessed = max(g.lastProcessed, head), true
		return
	}
	if !g.blockReceiptsOff {
		for number := from; number <= head; number++ {
			receipts, err := g.chain.BlockReceipts(ctx, number)
			if err != nil {
				slog.WarnContext(ctx, "block receipts unavailable; polling pending transactions instead",
					"direction", g.label, "block", number, "err", err)
				g.blockReceiptsOff = true
				break
			}
			for _, receipt := range receipts {
				g.settle(receipt, seenAt, out)
			}
			g.lastProcessed, g.haveProcessed = number, true
		}
	}
	if g.blockReceiptsOff {
		g.pollPending(ctx, seenAt, out, func(pendingTx) bool { return true })
		g.lastProcessed, g.haveProcessed = max(g.lastProcessed, head), true
	}
}

// sweepStale asks for the receipt of every transaction pending longer than
// StaleAfter and writes off those the Chain does not know.
func (g *generator) sweepStale(ctx context.Context, out chan<- included) {
	now := g.now()
	stale := func(tx pendingTx) bool { return now.Sub(tx.sentAt) >= g.cfg.StaleAfter }
	for hash, tx := range g.pollPending(ctx, now, out, stale) {
		slog.WarnContext(ctx, "transaction not mined; counting it dropped",
			"direction", g.label, "tx", hash, "wallet", g.wallets[tx.wallet], "nonce", tx.nonce)
		g.drop(hash, tx)
	}
}

// pollPending fetches receipts for pending transactions selected by want and
// returns the selected transactions the Chain reported not mined.
func (g *generator) pollPending(
	ctx context.Context,
	seenAt time.Time,
	out chan<- included,
	want func(pendingTx) bool,
) map[common.Hash]pendingTx {
	unmined := map[common.Hash]pendingTx{}
	for hash, tx := range g.pending {
		if !want(tx) {
			continue
		}
		receipt, err := g.chain.TransactionReceipt(ctx, hash)
		switch {
		case errors.Is(err, ethereum.NotFound):
			unmined[hash] = tx
		case err != nil:
			// Transient; the next sweep retries.
			slog.DebugContext(ctx, "receipt lookup failed", "direction", g.label, "tx", hash, "err", err)
		default:
			g.settle(receipt, seenAt, out)
		}
	}
	return unmined
}

// checkDrain writes off whatever is still pending once the drain deadline
// after the last dispatch passes.
func (g *generator) checkDrain() {
	if g.remaining > 0 || len(g.pending) == 0 {
		return
	}
	if g.drainDeadline.IsZero() {
		g.drainDeadline = g.now().Add(g.cfg.DrainTimeout)
		return
	}
	if g.now().Before(g.drainDeadline) {
		return
	}
	slog.Warn("drain timeout; counting outstanding transactions dropped",
		"direction", g.label, "outstanding", len(g.pending))
	for hash, tx := range g.pending {
		g.drop(hash, tx)
	}
}

func (g *generator) settle(receipt *types.Receipt, seenAt time.Time, out chan<- included) {
	tx, ours := g.pending[receipt.TxHash]
	if !ours {
		return
	}
	delete(g.pending, receipt.TxHash)
	g.states[tx.wallet].inFlight = false
	g.stats.Included++
	g.stats.SendToInclusion = append(g.stats.SendToInclusion, seenAt.Sub(tx.sentAt))
	if receipt.Status != types.ReceiptStatusSuccessful {
		g.stats.Reverted++
		return
	}
	out <- included{Receipt: receipt, SentAt: tx.sentAt, IncludedAt: seenAt}
}

func (g *generator) drop(hash common.Hash, tx pendingTx) {
	delete(g.pending, hash)
	g.stats.Dropped++
	state := &g.states[tx.wallet]
	state.inFlight = false
	// The pool may or may not still hold the transaction; ask the Chain.
	state.nonceKnown = false
}

// pickFree returns up to n wallets with nothing in flight, round-robin from
// the cursor so load spreads across the pool.
func (g *generator) pickFree(n int) []int {
	picked := make([]int, 0, n)
	for scanned := 0; scanned < len(g.states) && len(picked) < n; scanned++ {
		index := (g.cursor + scanned) % len(g.states)
		if !g.states[index].inFlight {
			picked = append(picked, index)
		}
	}
	if len(picked) > 0 {
		g.cursor = (picked[len(picked)-1] + 1) % len(g.states)
	}
	return picked
}

type sendResult struct {
	wallet int
	nonce  uint64
	hash   common.Hash
	sentAt time.Time
	err    error
}

// dispatch sends up to PerBlock transfers from free wallets. A shortfall of
// free wallets is counted as backpressure and left for later heads.
func (g *generator) dispatch(ctx context.Context, head *types.Header, at time.Time) error {
	want := min(g.cfg.PerBlock, g.remaining)
	wallets := g.pickFree(want)
	g.stats.Backpressure += want - len(wallets)
	if len(wallets) == 0 {
		return nil
	}
	build, err := g.prepare(ctx, head)
	if err != nil {
		return fmt.Errorf("prepare dispatch: %w", err)
	}
	g.lastDispatch = at
	g.stats.Dispatches++

	results := make([]sendResult, len(wallets))
	gate := make(chan struct{}, g.cfg.SendConcurrency)
	var wg sync.WaitGroup
	for k, wallet := range wallets {
		gate <- struct{}{}
		wg.Go(func() {
			defer func() { <-gate }()
			results[k] = g.send(ctx, wallet, g.states[wallet], build)
		})
	}
	wg.Wait()

	for _, result := range results {
		g.remaining--
		g.stats.Attempted++
		state := &g.states[result.wallet]
		if result.err != nil {
			g.stats.SendErrors++
			state.nonceKnown = false
			slog.WarnContext(ctx, "send failed", "direction", g.label,
				"wallet", g.wallets[result.wallet], "err", result.err)
			continue
		}
		state.nonce, state.nonceKnown, state.inFlight = result.nonce+1, true, true
		g.pending[result.hash] = pendingTx{wallet: result.wallet, nonce: result.nonce, sentAt: result.sentAt}
		g.stats.Sent++
		if g.stats.FirstSend.IsZero() {
			g.stats.FirstSend = result.sentAt
		}
		if result.sentAt.After(g.stats.LastSend) {
			g.stats.LastSend = result.sentAt
		}
	}
	return nil
}

// send runs concurrently with other wallets' sends; it only reads its own
// wallet's state and reports the outcome for the owner goroutine to apply.
func (g *generator) send(ctx context.Context, wallet int, state walletState, build txBuilder) sendResult {
	result := sendResult{wallet: wallet}
	nonce := state.nonce
	if !state.nonceKnown {
		var err error
		nonce, err = g.chain.PendingNonceAt(ctx, g.wallets[wallet])
		if err != nil {
			result.err = fmt.Errorf("pending nonce: %w", err)
			return result
		}
	}
	tx, err := build(wallet, nonce)
	if err != nil {
		result.err = fmt.Errorf("build transaction: %w", err)
		return result
	}
	result.sentAt = g.now()
	if err := g.chain.SendTransaction(ctx, tx); err != nil {
		result.err = fmt.Errorf("send transaction nonce %d: %w", nonce, err)
		return result
	}
	result.nonce, result.hash = nonce, tx.Hash()
	return result
}
