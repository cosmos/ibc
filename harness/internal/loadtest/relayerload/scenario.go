// SPDX-License-Identifier: Apache-2.0

// Package relayerload is the relayer load scenario: an open-loop, block-paced
// IFT transfer generator in each direction between two Chains, whose every
// packet must be relayed to SUCCEEDED by an auto-relaying relayer.
package relayerload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/ibccli"
	"github.com/cosmos/ibc/harness/internal/loadtest/session"
	"github.com/cosmos/ibc/harness/internal/loadtest/stats"
	"github.com/cosmos/ibc/harness/traffic"
)

const (
	// Relayer tuning, as in the e2e load tests this replaces.
	packetBatchSize    = 64
	packetBatchTimeout = time.Second
	clearInterval      = time.Second

	// walletFundingCoins is the gas balance and IFT amount each wallet gets
	// on both Chains.
	walletFundingCoins = 10
	// transferAmount is the IFT base units each transfer moves.
	transferAmount = 10_000
	// packetTimeout keeps packets valid for the whole run.
	packetTimeout = time.Hour

	// gasHeadroomPercent pads the one-off gas estimate: every transfer
	// writes fresh commitment and pending-transfer slots, and receivers with
	// a zero balance cost more than the estimating pair.
	gasHeadroomPercent = 50
	fallbackTipCapWei  = 1_000_000_000

	sendConcurrency = 16
	// awaitWorkers bounds concurrent status waits per direction.
	awaitWorkers = 8
)

// Direction names one traffic direction.
type Direction string

const (
	DirectionAtoB Direction = "a-to-b"
	DirectionBtoA Direction = "b-to-a"
)

// Config shapes one relayer scenario run.
type Config struct {
	// Packets is the total across all directions, split evenly.
	Packets    int
	Wallets    int
	PerBlock   int
	Directions []Direction
	Session    session.Options
}

func (c Config) validate() error {
	switch {
	case c.Packets <= 0:
		return errors.New("packets must be positive")
	case c.Wallets < 2:
		return errors.New("at least two wallets are required (receivers are pool wallets)")
	case c.PerBlock <= 0:
		return errors.New("per-block must be positive")
	case len(c.Directions) == 0:
		return errors.New("at least one direction is required")
	case c.Packets < len(c.Directions):
		return errors.New("packets must cover every direction")
	}
	for _, d := range c.Directions {
		if d != DirectionAtoB && d != DirectionBtoA {
			return fmt.Errorf("unknown direction %q", d)
		}
	}
	return nil
}

func (d Direction) route() traffic.Route {
	if d == DirectionBtoA {
		return traffic.BtoA(traffic.ChainB, traffic.ChainA)
	}
	return traffic.AtoB(traffic.ChainA, traffic.ChainB)
}

// splitPackets divides total across n directions, the remainder going to
// the first ones.
func splitPackets(total, n int) []int {
	shares := make([]int, n)
	for i := range shares {
		shares[i] = total / n
		if i < total%n {
			shares[i]++
		}
	}
	return shares
}

// Run executes the scenario. It returns the report whenever the generator
// ran, even on failure; err reports setup or teardown problems.
func Run(ctx context.Context, cfg Config) (report *Report, err error) {
	if err = cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.PerBlock > cfg.Wallets {
		slog.WarnContext(ctx, "per-block exceeds wallets; every dispatch will report backpressure",
			"per_block", cfg.PerBlock, "wallets", cfg.Wallets)
	}
	started := time.Now()

	opts := cfg.Session
	opts.Routes = []traffic.Route{DirectionAtoB.route(), DirectionBtoA.route()}
	opts.ConfigureRelayer = func(config *ibccli.RelayerConfig) {
		config.ClearInterval = clearInterval
		config.ClearOnStart = true
		for i := range config.Chains {
			config.Chains[i].PacketBatchSize = packetBatchSize
			config.Chains[i].PacketBatchTimeout = packetBatchTimeout
		}
	}
	s, err := session.Start(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		var failure error
		if err != nil || report == nil || !report.Passed {
			failure = errors.New("scenario failed")
		}
		err = errors.Join(err, s.Close(ctx, failure))
	}()

	wallets, err := newWallets(cfg.Wallets)
	if err != nil {
		return nil, err
	}
	if err = fundWallets(ctx, s, wallets); err != nil {
		return nil, err
	}

	shares := splitPackets(cfg.Packets, len(cfg.Directions))
	reports := make([]DirectionReport, len(cfg.Directions))
	sendSamples := make([][]time.Duration, len(cfg.Directions))
	relaySamples := make([][]time.Duration, len(cfg.Directions))
	var wg sync.WaitGroup
	for i, direction := range cfg.Directions {
		wg.Go(func() {
			reports[i], sendSamples[i], relaySamples[i] = runDirection(
				ctx, s, direction, wallets, shares[i], cfg.PerBlock,
			)
		})
	}
	wg.Wait()

	report = &Report{
		Scenario:   "relayer",
		Provider:   string(opts.Provider),
		Wallets:    cfg.Wallets,
		PerBlock:   cfg.PerBlock,
		Directions: reports,
		Total:      total(reports),
		DurationMS: time.Since(started).Milliseconds(),
	}
	report.Total.SendToInclusion = stats.Summarize(concat(sendSamples))
	report.Total.InclusionToSucceeded = stats.Summarize(concat(relaySamples))
	report.Passed = true
	for _, d := range reports {
		if d.Failed() {
			report.Passed = false
		}
	}
	return report, nil
}

func concat(samples [][]time.Duration) []time.Duration {
	var out []time.Duration
	for _, s := range samples {
		out = append(out, s...)
	}
	return out
}

func newWallets(n int) ([]traffic.Signer, error) {
	wallets := make([]traffic.Signer, n)
	for i := range wallets {
		signer, err := traffic.NewSigner()
		if err != nil {
			return nil, err
		}
		wallets[i] = signer
	}
	return wallets, nil
}

// fundWallets gives every wallet gas and IFT on both Chains, concurrently
// per Chain and asset.
func fundWallets(ctx context.Context, s *session.Session, wallets []traffic.Signer) error {
	amount := traffic.Coins(walletFundingCoins)
	jobs := []struct {
		name string
		run  func() error
	}{
		{"gas on " + string(traffic.ChainA), func() error { return fundGas(ctx, s, traffic.ChainA, wallets, amount) }},
		{"gas on " + string(traffic.ChainB), func() error { return fundGas(ctx, s, traffic.ChainB, wallets, amount) }},
		{"IFT on " + string(traffic.ChainA), func() error { return fundIFT(ctx, s, DirectionAtoB, wallets, amount) }},
		{"IFT on " + string(traffic.ChainB), func() error { return fundIFT(ctx, s, DirectionBtoA, wallets, amount) }},
	}
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Go(func() {
			started := time.Now()
			if err := job.run(); err != nil {
				errs[i] = fmt.Errorf("fund %s: %w", job.name, err)
				return
			}
			slog.InfoContext(ctx, "funded wallets", "what", job.name, "wallets", len(wallets),
				"elapsed", time.Since(started).Round(time.Millisecond).String())
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

func fundGas(
	ctx context.Context,
	s *session.Session,
	id environment.ChainID,
	wallets []traffic.Signer,
	amount *big.Int,
) error {
	chain, err := s.Env.Chain(id)
	if err != nil {
		return err
	}
	funding, err := chain.Funding()
	if err != nil {
		return err
	}
	for _, wallet := range wallets {
		if err := funding.EnsureEOABalance(ctx, wallet.Address(), amount); err != nil {
			return err
		}
	}
	return nil
}

func fundIFT(
	ctx context.Context,
	s *session.Session,
	direction Direction,
	wallets []traffic.Signer,
	amount *big.Int,
) error {
	app, err := traffic.NewIFT(s.Env, s.Deployment, s.Deployer, direction.route())
	if err != nil {
		return err
	}
	requests := make([]traffic.ERCTransferRequest, len(wallets))
	for i, wallet := range wallets {
		requests[i] = traffic.ERCTransferRequest{Address: wallet.Address(), Amount: amount}
	}
	_, err = app.TransferBatch(ctx, requests)
	return err
}

// runDirection drives one direction end to end and never returns an error:
// failures land in the report so the other direction still reports.
func runDirection(
	ctx context.Context,
	s *session.Session,
	direction Direction,
	signers []traffic.Signer,
	packets, perBlock int,
) (DirectionReport, []time.Duration, []time.Duration) {
	route := direction.route()
	label := string(direction)
	fail := func(err error) (DirectionReport, []time.Duration, []time.Duration) {
		slog.ErrorContext(ctx, "direction failed", "direction", label, "err", err)
		return newDirectionReport(label, string(route.Source), string(route.Destination), packets,
			generatorStats{}, awaitStats{}, 0, 0, err), nil, nil
	}

	source, err := s.Env.Chain(route.Source)
	if err != nil {
		return fail(err)
	}
	destination, err := s.Env.Chain(route.Destination)
	if err != nil {
		return fail(err)
	}
	app, err := traffic.NewIFT(s.Env, s.Deployment, s.Deployer, route)
	if err != nil {
		return fail(err)
	}
	chain, err := dialChain(ctx, source)
	if err != nil {
		return fail(err)
	}
	defer chain.Close()

	addresses := make([]common.Address, len(signers))
	for i, signer := range signers {
		addresses[i] = signer.Address()
	}
	prepare, err := newIFTPrepare(ctx, chain, app, signers, addresses)
	if err != nil {
		return fail(err)
	}

	routePolicy, ok := s.Relayer.WaitPolicy(string(route.ID))
	if !ok {
		return fail(fmt.Errorf("relayer has no wait policy for route %q", route.ID))
	}
	policy := packetAwaitPolicy(routePolicy, source.Timing(), destination.Timing(), packets, packetBatchSize)
	timing := source.Timing()
	gen, err := newGenerator(label, generatorConfig{
		Packets:             packets,
		PerBlock:            perBlock,
		MinDispatchInterval: timing.BlockInterval,
		StaleAfter:          timing.CompletionBudget,
		DrainTimeout:        timing.CompletionBudget,
		SendConcurrency:     sendConcurrency,
	}, chain, addresses, prepare)
	if err != nil {
		return fail(err)
	}

	heads := make(chan *types.Header, 1024)
	subscription, err := chain.SubscribeNewHead(ctx, heads)
	if err != nil {
		return fail(fmt.Errorf("subscribe to new heads: %w", err))
	}
	defer subscription.Unsubscribe()

	slog.InfoContext(ctx, "generating load", "direction", label, "packets", packets, "per_block", perBlock,
		"wallets", len(signers), "dispatch_interval", timing.BlockInterval,
		"await_poll", policy.StatusPoll, "await_budget", policy.CompletionBudget)

	includedCh := make(chan included, packets)
	packetsCh := make(chan trackedPacket, packets)
	var located, missing int
	var convert sync.WaitGroup
	convert.Go(func() {
		defer close(packetsCh)
		for tx := range includedCh {
			found, err := app.PacketTxsFromReceipt(tx.Receipt)
			if err != nil || len(found) != 1 {
				missing++
				slog.WarnContext(ctx, "transfer did not emit exactly one packet", "direction", label,
					"tx", tx.Receipt.TxHash, "packets", len(found), "err", err)
			}
			for _, packet := range found {
				located++
				packetsCh <- trackedPacket{packet: packet, includedAt: tx.IncludedAt}
			}
		}
	})

	var await awaitStats
	var awaiting sync.WaitGroup
	awaiting.Go(func() {
		await = awaitPackets(ctx, label, s.Relayer, policy, awaitWorkers, packetsCh)
	})

	genStats, genErr := gen.run(ctx, heads, subscription.Err(), includedCh)
	close(includedCh)
	slog.InfoContext(ctx, "send phase done", "direction", label, "sent", genStats.Sent,
		"included", genStats.Included, "backpressure", genStats.Backpressure,
		"elapsed", genStats.LastSend.Sub(genStats.FirstSend).Round(time.Millisecond).String())
	convert.Wait()
	awaiting.Wait()

	report := newDirectionReport(label, string(route.Source), string(route.Destination), packets,
		genStats, await, located, missing, genErr)
	return report, genStats.SendToInclusion, await.InclusionToSucceeded
}

// newIFTPrepare estimates gas once and returns the per-dispatch builder of
// signed iftTransfer transactions. Wallet i sends to wallet i+1's address on
// the destination, so receivers are funded senders for the reverse direction.
func newIFTPrepare(
	ctx context.Context,
	chain *ethChain,
	app *traffic.IFT,
	signers []traffic.Signer,
	addresses []common.Address,
) (prepareFunc, error) {
	amount := big.NewInt(transferAmount)
	to := app.SourceContract()
	receiver := func(wallet int) common.Address { return addresses[(wallet+1)%len(addresses)] }

	timeout, err := app.TransferTimeout(ctx, packetTimeout)
	if err != nil {
		return nil, err
	}
	data, err := app.TransferCalldata(receiver(0), amount, timeout)
	if err != nil {
		return nil, err
	}
	estimate, err := chain.estimateGas(ctx, ethereum.CallMsg{From: addresses[0], To: &to, Data: data})
	if err != nil {
		return nil, fmt.Errorf("estimate IFT transfer gas: %w", err)
	}
	gas := estimate + estimate*gasHeadroomPercent/100
	tip, err := chain.suggestTipCap(ctx)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "transfer gas", "chain", chain.id, "estimate", estimate, "limit", gas, "tip", tip)

	return func(ctx context.Context, head *types.Header) (txBuilder, error) {
		timeout, err := app.TransferTimeout(ctx, packetTimeout)
		if err != nil {
			return nil, err
		}
		feeCap := new(big.Int).Set(tip)
		if head.BaseFee != nil && head.BaseFee.Sign() > 0 {
			feeCap.Add(feeCap, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))
		}
		return func(wallet int, nonce uint64) (*types.Transaction, error) {
			data, err := app.TransferCalldata(receiver(wallet), amount, timeout)
			if err != nil {
				return nil, err
			}
			return signers[wallet].SignTx(chain.evmID, types.NewTx(&types.DynamicFeeTx{
				ChainID:   chain.evmID,
				Nonce:     nonce,
				To:        &to,
				Gas:       gas,
				GasFeeCap: feeCap,
				GasTipCap: tip,
				Data:      data,
			}))
		}, nil
	}, nil
}
