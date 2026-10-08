// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sentRecord struct {
	wallet int
	nonce  uint64
}

// fakeChain mines whatever was submitted into the block a test asks for.
type fakeChain struct {
	mu            sync.Mutex
	mempool       []common.Hash
	blocks        map[uint64][]*types.Receipt
	mined         map[common.Hash]*types.Receipt
	nonceCalls    map[common.Address]int
	pendingNonces map[common.Address]uint64
	failSend      func(tx *types.Transaction) error
	revert        map[common.Hash]bool
	noBlockRPC    bool
}

func newFakeChain() *fakeChain {
	return &fakeChain{
		blocks:        map[uint64][]*types.Receipt{},
		mined:         map[common.Hash]*types.Receipt{},
		nonceCalls:    map[common.Address]int{},
		pendingNonces: map[common.Address]uint64{},
		revert:        map[common.Hash]bool{},
	}
}

func (c *fakeChain) PendingNonceAt(_ context.Context, account common.Address) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonceCalls[account]++
	return c.pendingNonces[account], nil
}

func (c *fakeChain) SendTransaction(_ context.Context, tx *types.Transaction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSend != nil {
		if err := c.failSend(tx); err != nil {
			return err
		}
	}
	c.mempool = append(c.mempool, tx.Hash())
	return nil
}

func (c *fakeChain) BlockReceipts(_ context.Context, number uint64) ([]*types.Receipt, error) {
	if c.noBlockRPC {
		return nil, errors.New("method not found")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks[number], nil
}

func (c *fakeChain) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	receipt, ok := c.mined[hash]
	if !ok {
		return nil, ethereum.NotFound
	}
	return receipt, nil
}

// mine includes the whole mempool in block number.
func (c *fakeChain) mine(number uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, hash := range c.mempool {
		status := types.ReceiptStatusSuccessful
		if c.revert[hash] {
			status = types.ReceiptStatusFailed
		}
		receipt := &types.Receipt{TxHash: hash, Status: status, BlockNumber: new(big.Int).SetUint64(number)}
		c.blocks[number] = append(c.blocks[number], receipt)
		c.mined[hash] = receipt
	}
	c.mempool = nil
}

// forget drops the mempool without mining it.
func (c *fakeChain) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mempool = nil
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	t      *testing.T
	gen    *generator
	chain  *fakeChain
	clock  *fakeClock
	out    chan included
	mu     sync.Mutex
	sent   map[common.Hash]sentRecord
	height uint64
}

func newHarness(t *testing.T, cfg generatorConfig, wallets int) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		chain: newFakeChain(),
		clock: &fakeClock{now: time.Unix(1_000, 0)},
		out:   make(chan included, cfg.Packets),
		sent:  map[common.Hash]sentRecord{},
	}
	addresses := make([]common.Address, wallets)
	for i := range addresses {
		addresses[i] = common.BigToAddress(big.NewInt(int64(i + 1)))
	}
	prepare := func(context.Context, *types.Header) (txBuilder, error) {
		return func(wallet int, nonce uint64) (*types.Transaction, error) {
			tx := types.NewTx(&types.LegacyTx{Nonce: nonce, Data: []byte{byte(wallet)}, Gas: 21_000})
			h.mu.Lock()
			h.sent[tx.Hash()] = sentRecord{wallet: wallet, nonce: nonce}
			h.mu.Unlock()
			return tx, nil
		}, nil
	}
	gen, err := newGenerator("test", cfg, h.chain, addresses, prepare)
	require.NoError(t, err)
	gen.now = h.clock.Now
	h.gen = gen
	return h
}

// head delivers the next head, optionally mining the mempool into it first.
func (h *harness) head(mine bool) {
	h.t.Helper()
	h.height++
	if mine {
		h.chain.mine(h.height)
	}
	require.NoError(h.t, h.gen.onHead(h.t.Context(), &types.Header{Number: new(big.Int).SetUint64(h.height)}, h.out))
}

func baseConfig() generatorConfig {
	return generatorConfig{
		Packets:             10,
		PerBlock:            3,
		MinDispatchInterval: 0,
		StaleAfter:          time.Minute,
		DrainTimeout:        time.Minute,
		SendConcurrency:     4,
	}
}

func TestGeneratorSendsEveryPacketAtMostPerBlock(t *testing.T) {
	h := newHarness(t, baseConfig(), 5)

	for range 10 {
		before := h.gen.stats.Attempted
		h.clock.Advance(time.Second)
		h.head(true)
		assert.LessOrEqual(t, h.gen.stats.Attempted-before, 3, "never more than per-block in one dispatch")
		if h.gen.done() {
			break
		}
	}

	require.True(t, h.gen.done())
	stats := h.gen.stats
	assert.Equal(t, 10, stats.Attempted)
	assert.Equal(t, 10, stats.Sent)
	assert.Equal(t, 10, stats.Included)
	assert.Zero(t, stats.SendErrors)
	assert.Zero(t, stats.Backpressure)
	assert.Len(t, h.out, 10)
	assert.Len(t, stats.SendToInclusion, 10)
	for _, sample := range stats.SendToInclusion {
		assert.Equal(t, time.Second, sample, "included at the next head, one fake second later")
	}
}

func TestGeneratorTracksNoncesPerWallet(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 6, 2
	h := newHarness(t, cfg, 2)
	h.chain.pendingNonces[h.gen.wallets[0]] = 7
	h.chain.pendingNonces[h.gen.wallets[1]] = 3

	for !h.gen.done() {
		h.head(true)
	}

	perWallet := map[int][]uint64{}
	for _, record := range h.sent {
		perWallet[record.wallet] = append(perWallet[record.wallet], record.nonce)
	}
	assert.ElementsMatch(t, []uint64{7, 8, 9}, perWallet[0])
	assert.ElementsMatch(t, []uint64{3, 4, 5}, perWallet[1])
	// Nonces are fetched once per wallet and tracked locally afterwards.
	assert.Equal(t, 1, h.chain.nonceCalls[h.gen.wallets[0]])
	assert.Equal(t, 1, h.chain.nonceCalls[h.gen.wallets[1]])
}

func TestGeneratorOneTransactionInFlightPerWalletAndBackpressure(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 6, 5
	h := newHarness(t, cfg, 2)

	// Nothing is mined: the first dispatch fills both wallets and the next
	// heads find none free.
	h.head(false)
	assert.Equal(t, 2, h.gen.stats.Sent)
	assert.Equal(t, 3, h.gen.stats.Backpressure, "wanted 5, two wallets")
	h.head(false)
	h.head(false)
	assert.Equal(t, 2, h.gen.stats.Sent, "no wallet is free while its transaction is pending")
	assert.Equal(t, 3+4+4, h.gen.stats.Backpressure, "shortfall against min(per-block, remaining)")

	// Mining frees both wallets for the next dispatch.
	h.head(true)
	assert.Equal(t, 2, h.gen.stats.Included)
	assert.Equal(t, 4, h.gen.stats.Sent)
	assert.Equal(t, 2, h.gen.remaining)
}

func TestGeneratorDebouncesHeadsToTheDispatchInterval(t *testing.T) {
	cfg := baseConfig()
	cfg.MinDispatchInterval = time.Second
	cfg.Packets, cfg.PerBlock = 100, 1
	h := newHarness(t, cfg, 10)

	// A mixed-mining Chain mines a block per transaction: many heads within
	// one interval must still dispatch once.
	for range 5 {
		h.clock.Advance(100 * time.Millisecond)
		h.head(true)
	}
	assert.Equal(t, 1, h.gen.stats.Dispatches)
	assert.Equal(t, 5, h.gen.stats.HeadsSeen)

	h.clock.Advance(600 * time.Millisecond)
	h.head(true)
	assert.Equal(t, 2, h.gen.stats.Dispatches)
}

func TestGeneratorResyncsNonceAfterSendError(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 3, 1
	h := newHarness(t, cfg, 1)
	failed := false
	h.chain.failSend = func(*types.Transaction) error {
		if !failed {
			failed = true
			return errors.New("nonce too low")
		}
		return nil
	}

	for !h.gen.done() {
		h.head(true)
	}

	assert.Equal(t, 3, h.gen.stats.Attempted, "a failed send consumes its attempt")
	assert.Equal(t, 1, h.gen.stats.SendErrors)
	assert.Equal(t, 2, h.gen.stats.Sent)
	assert.Equal(t, 2, h.chain.nonceCalls[h.gen.wallets[0]], "fetched once at start and once after the error")
}

func TestGeneratorCountsRevertsAndKeepsThemOutOfPackets(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 2, 2
	h := newHarness(t, cfg, 2)

	h.head(false)
	for hash, record := range h.sent {
		if record.wallet == 0 {
			h.chain.revert[hash] = true
		}
	}
	h.head(true)

	require.True(t, h.gen.done())
	assert.Equal(t, 2, h.gen.stats.Included)
	assert.Equal(t, 1, h.gen.stats.Reverted)
	assert.Len(t, h.out, 1)
}

func TestGeneratorDropsStaleTransactionsAndFreesTheirWallets(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 2, 1
	cfg.StaleAfter = 10 * time.Second
	h := newHarness(t, cfg, 1)

	h.head(false)
	h.chain.forget()
	h.clock.Advance(5 * time.Second)
	h.head(false)
	assert.Equal(t, 1, h.gen.stats.Sent, "still pending before StaleAfter")
	assert.Zero(t, h.gen.stats.Dropped)

	h.clock.Advance(5 * time.Second)
	h.head(false)
	assert.Equal(t, 1, h.gen.stats.Dropped)
	assert.Equal(t, 2, h.gen.stats.Sent, "the freed wallet sends again")
	assert.Equal(t, 2, h.chain.nonceCalls[h.gen.wallets[0]], "a dropped transaction forces a nonce resync")
}

func TestGeneratorDrainTimeoutWritesOffOutstanding(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 1, 1
	cfg.StaleAfter = time.Hour
	cfg.DrainTimeout = 10 * time.Second
	h := newHarness(t, cfg, 1)

	h.head(false)
	assert.False(t, h.gen.done())
	h.clock.Advance(11 * time.Second)
	h.head(false)

	assert.True(t, h.gen.done())
	assert.Equal(t, 1, h.gen.stats.Dropped)
}

func TestGeneratorFallsBackToTransactionReceipts(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 4, 2
	h := newHarness(t, cfg, 2)
	h.chain.noBlockRPC = true

	for !h.gen.done() {
		h.head(true)
	}
	assert.Equal(t, 4, h.gen.stats.Included)
	assert.True(t, h.gen.blockReceiptsOff)
}

func TestGeneratorCatchesUpOnSkippedHeads(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 2, 2
	h := newHarness(t, cfg, 2)

	h.head(false)
	// The transactions land in a block whose head the generator never sees.
	h.height++
	h.chain.mine(h.height)
	h.head(false)

	assert.True(t, h.gen.done())
	assert.Equal(t, 2, h.gen.stats.Included)
}

func TestGeneratorRunConsumesHeads(t *testing.T) {
	cfg := baseConfig()
	cfg.Packets, cfg.PerBlock = 5, 2
	h := newHarness(t, cfg, 3)
	heads := make(chan *types.Header)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	go func() {
		for number := uint64(1); ctx.Err() == nil; number++ {
			h.chain.mine(number)
			select {
			case heads <- &types.Header{Number: new(big.Int).SetUint64(number)}:
			case <-ctx.Done():
			}
		}
	}()
	stats, err := h.gen.run(ctx, heads, make(chan error), h.out)

	require.NoError(t, err)
	assert.Equal(t, 5, stats.Included)
}

func TestPickFreeIsRoundRobin(t *testing.T) {
	g := &generator{states: make([]walletState, 4)}

	assert.Equal(t, []int{0, 1}, g.pickFree(2))
	for _, i := range []int{0, 1} {
		g.states[i].inFlight = true
	}
	assert.Equal(t, []int{2, 3}, g.pickFree(3))
	g.states[2].inFlight, g.states[3].inFlight = true, true
	assert.Empty(t, g.pickFree(1))
	g.states[1].inFlight = false
	assert.Equal(t, []int{1}, g.pickFree(2))
}

func TestSplitPackets(t *testing.T) {
	assert.Equal(t, []int{1500, 1500}, splitPackets(3000, 2))
	assert.Equal(t, []int{2, 1}, splitPackets(3, 2))
	assert.Equal(t, []int{40}, splitPackets(40, 1))
}
