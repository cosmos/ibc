// SPDX-License-Identifier: Apache-2.0

// Package relayerload is the relayer load scenario: paced IFT transfers from
// a wallet pool in each direction between two Chains, whose every packet must
// be relayed to SUCCEEDED by an auto-relaying relayer.
package relayerload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
	"github.com/cosmos/ibc/e2e/internal/loadtest/session"
	"github.com/cosmos/ibc/e2e/internal/loadtest/stats"
)

const (
	packetBatchSize    = 64
	packetBatchTimeout = time.Second
	clearInterval      = time.Second

	// walletFundingCoins is the gas balance and IFT amount each wallet gets
	// on both Chains: ample for thousands of transfers per wallet.
	walletFundingCoins = 1
	// transferAmount is the IFT base units each transfer moves.
	transferAmount = 10_000
	// packetTimeout keeps packets valid for any run: the router accepts at
	// most a day.
	packetTimeout = 23 * time.Hour
)

// Direction names one traffic direction.
type Direction string

const (
	DirectionAtoB Direction = "a-to-b"
	DirectionBtoA Direction = "b-to-a"
)

func (d Direction) route() traffic.Route {
	if d == DirectionBtoA {
		return traffic.BtoA(traffic.ChainB, traffic.ChainA)
	}
	return traffic.AtoB(traffic.ChainA, traffic.ChainB)
}

// Config shapes one relayer scenario run. The CLI validates it.
type Config struct {
	// Packets is the total across all directions, split evenly.
	Packets    int
	Wallets    int
	PerBlock   int
	Directions []Direction
}

// SessionOptions adds what cfg needs to base: the routes of its directions and
// a started relayer with load-sized batches.
func SessionOptions(base session.Options, cfg Config) session.Options {
	base.Routes = nil
	for _, direction := range cfg.Directions {
		base.Routes = append(base.Routes, direction.route())
	}
	base.StartRelayer = true
	base.ConfigureRelayer = func(config *ibccli.RelayerConfig) {
		config.ClearInterval = clearInterval
		config.ClearOnStart = true
		for i := range config.Chains {
			config.Chains[i].PacketBatchSize = packetBatchSize
			config.Chains[i].PacketBatchTimeout = packetBatchTimeout
		}
	}
	return base
}

// Run funds a wallet pool, drives every direction concurrently, and returns
// the report, with an error unless every packet succeeded.
func Run(ctx context.Context, s *session.Session, cfg Config) (*Report, error) {
	report := &Report{
		Scenario: "relayer",
		Provider: string(s.Provider),
		Wallets:  cfg.Wallets,
		PerBlock: cfg.PerBlock,
		Packets:  cfg.Packets,
	}
	err := run(ctx, s, cfg, report)
	report.Passed = err == nil
	if err != nil {
		report.Error = err.Error()
	}
	return report, err
}

func run(ctx context.Context, s *session.Session, cfg Config, report *Report) error {
	wallets := make([]traffic.Signer, cfg.Wallets)
	for i := range wallets {
		signer, err := traffic.NewSigner()
		if err != nil {
			return err
		}
		wallets[i] = signer
	}
	if err := fundWallets(ctx, s, cfg.Directions, wallets); err != nil {
		return err
	}

	errs := make([]error, len(cfg.Directions))
	started := time.Now()
	var wg sync.WaitGroup
	for i, direction := range cfg.Directions {
		route := direction.route()
		d := &DirectionReport{
			Route:       string(direction),
			Source:      string(route.Source),
			Destination: string(route.Destination),
			Target:      cfg.Packets / len(cfg.Directions),
		}
		// The remainder goes to the first directions.
		if i < cfg.Packets%len(cfg.Directions) {
			d.Target++
		}
		report.Directions = append(report.Directions, d)
		wg.Go(func() {
			if err := runDirection(ctx, s, route, wallets, cfg.PerBlock, d); err != nil {
				errs[i] = fmt.Errorf("%s: %w", direction, err)
			}
		})
	}
	wg.Wait()

	var succeeded int
	var finished time.Time
	for _, d := range report.Directions {
		succeeded += d.Succeeded
		if d.relayDone.After(finished) {
			finished = d.relayDone
		}
	}
	if !finished.IsZero() {
		report.WallTimeMS = finished.Sub(started).Milliseconds()
		report.Throughput = float64(succeeded) / finished.Sub(started).Seconds()
	}
	return errors.Join(errs...)
}

// fundWallets gives every wallet gas and IFT on the source Chain of each
// direction, concurrently. Receivers need no funds.
func fundWallets(ctx context.Context, s *session.Session, directions []Direction, wallets []traffic.Signer) error {
	addresses := make([]common.Address, len(wallets))
	for i, wallet := range wallets {
		addresses[i] = wallet.Address()
	}
	amount := traffic.Coins(walletFundingCoins)
	errs := make([]error, len(directions))
	var wg sync.WaitGroup
	for i, direction := range directions {
		wg.Go(func() {
			started := time.Now()
			route := direction.route()
			app, err := traffic.NewIFT(s.Env, s.Deployment, s.Deployer, route)
			if err == nil {
				err = app.FundWallets(ctx, addresses, amount, amount)
			}
			if err != nil {
				errs[i] = fmt.Errorf("fund wallets on %s: %w", route.Source, err)
				return
			}
			slog.InfoContext(ctx, "funded wallets", "chain", route.Source, "wallets", len(wallets),
				"elapsed", time.Since(started).Round(time.Millisecond).String())
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// runDirection sends d.Target packets on route while sweeping the relayer
// for their status, then measures the relay latency of those that succeeded.
func runDirection(
	ctx context.Context,
	s *session.Session,
	route traffic.Route,
	wallets []traffic.Signer,
	perBlock int,
	d *DirectionReport,
) error {
	source, err := s.Env.Chain(route.Source)
	if err != nil {
		return err
	}
	destination, err := s.Env.Chain(route.Destination)
	if err != nil {
		return err
	}
	sourceEVM, err := source.EVM()
	if err != nil {
		return err
	}
	destinationEVM, err := destination.EVM()
	if err != nil {
		return err
	}
	clients, ok := s.Deployment.RouteClients(route.ID)
	if !ok {
		return fmt.Errorf("deployment has no route %q", route.ID)
	}
	// The relay of a direction stalls when no packet goes terminal for the
	// route's completion budget.
	policy, ok := s.Relayer.WaitPolicy(string(route.ID))
	if !ok {
		return fmt.Errorf("relayer has no wait policy for route %q", route.ID)
	}

	// Each wallet sends as its own IFT sender and pays wallet i+1 on the
	// destination (itself in a pool of one).
	apps := make([]*traffic.IFT, len(wallets))
	for i, wallet := range wallets {
		if apps[i], err = traffic.NewIFT(s.Env, s.Deployment, wallet, route); err != nil {
			return err
		}
	}
	timeout, err := apps[0].TransferTimeout(ctx, packetTimeout)
	if err != nil {
		return err
	}
	amount := big.NewInt(transferAmount)
	send := func(ctx context.Context, wallet int) (sentPacket, error) {
		receiver := wallets[(wallet+1)%len(wallets)].Address()
		result, transferErr := apps[wallet].Transfer(ctx, receiver, amount, timeout)
		if transferErr != nil {
			return sentPacket{}, transferErr
		}
		packet := result.PacketTx()
		return sentPacket{
			key:   packetKey{clientID: packet.SourceClientID, sequence: packet.Sequence},
			block: packet.SourceBlockNumber,
		}, nil
	}
	list := func(ctx context.Context) ([]*relayerv2.PacketStatus, error) {
		return s.Relayer.ClientPackets(ctx, string(route.Source), clients.SourceClientID)
	}

	interval := source.Timing().BlockInterval
	poll := max(interval, destination.Timing().BlockInterval)
	slog.InfoContext(ctx, "generating load", "direction", d.Route, "packets", d.Target, "per_block", perBlock,
		"wallets", len(wallets), "interval", interval, "poll", poll, "stall", policy.CompletionBudget)

	sent := make(chan sentPacket, d.Target)
	var packets []*trackedPacket
	var sweepErr error
	var sweeping sync.WaitGroup
	// A sweep that gives up ends sending too.
	sendCtx, stopSending := context.WithCancel(ctx)
	defer stopSending()
	sweeping.Go(func() {
		packets, sweepErr = sweep(ctx, d.Route, poll, policy.CompletionBudget, list, sent)
		if sweepErr != nil {
			stopSending()
		}
	})
	ticker := time.NewTicker(interval)
	generate(sendCtx, ticker.C, perBlock, len(wallets), send, sent, d)
	ticker.Stop()
	slog.InfoContext(ctx, "send phase done", "direction", d.Route, "sent", d.Sent,
		"send_failures", d.SendFailures, "backpressure", d.Backpressure)
	sweeping.Wait()
	d.relayDone = time.Now()
	if !d.lastSend.IsZero() {
		d.RelayDrainMS = d.relayDone.Sub(d.lastSend).Milliseconds()
	}

	var problems []error
	if d.Sent != d.Target {
		problems = append(problems, fmt.Errorf("sent %d of %d packets (%d send failures)",
			d.Sent, d.Target, d.SendFailures))
	}
	if sweepErr != nil {
		problems = append(problems, sweepErr)
	}
	for _, p := range packets {
		switch p.state() {
		case relayerv2.PacketState_PACKET_STATE_SUCCEEDED:
			d.Succeeded++
		case relayerv2.PacketState_PACKET_STATE_TIMED_OUT:
			d.TimedOut++
		case relayerv2.PacketState_PACKET_STATE_REJECTED:
			d.Rejected++
		case relayerv2.PacketState_PACKET_STATE_RELAY_FAILED:
			d.RelayFailed++
		default:
			d.Unresolved++
		}
	}
	if failed := d.TimedOut + d.Rejected + d.RelayFailed; failed > 0 {
		problems = append(problems, fmt.Errorf("%d packets did not succeed: timed_out=%d rejected=%d relay_failed=%d",
			failed, d.TimedOut, d.Rejected, d.RelayFailed))
	}

	// Latency is reported, not judged; an interrupted run skips it.
	if ctx.Err() == nil {
		sendToRecv, recvToAck, sendToAck, err := relayLatencies(ctx, sourceEVM, destinationEVM, packets)
		if err != nil {
			d.LatencyError = err.Error()
			slog.WarnContext(ctx, "relay latency incomplete", "direction", d.Route, "err", err)
		}
		d.SendToRecv, d.RecvToAck, d.SendToAck = stats.Summarize(sendToRecv), stats.Summarize(recvToAck),
			stats.Summarize(sendToAck)
	}
	return errors.Join(problems...)
}
