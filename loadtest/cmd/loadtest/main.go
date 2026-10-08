// SPDX-License-Identifier: Apache-2.0

// Command loadtest drives load against a local two-Chain IBC environment
// built by the harness: Anvil (or Besu) Chains joined by an attested mesh,
// the IBC CLI relayer, and its attestors. See loadtest/README.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cosmos/ibc/loadtest/internal/attestorload"
	"github.com/cosmos/ibc/loadtest/internal/relayerload"
	"github.com/cosmos/ibc/loadtest/internal/session"
	"github.com/cosmos/ibc/loadtest/internal/simpleload"
)

const (
	exitOK      = 0
	exitFailed  = 1
	exitUsage   = 2
	ibcBinEnv   = "IBC_BIN"
	usageHeader = `Usage: loadtest <scenario> [flags]

Scenarios:
  relayer    open-loop, block-paced IFT transfers in both directions; every packet must relay
  attestor   fixed-rate packet and state attestation requests against one attestor

Run "loadtest <scenario> -h" for the scenario's flags.
`
)

type relayerPreset struct {
	packets, wallets, perBlock int
}

var relayerPresets = map[string]relayerPreset{
	// Short, dense bursts: 3000 packets at 150 per block.
	"burst": {packets: 3000, wallets: 200, perBlock: 150},
	// Sustained, light load: 8000 packets at 20 per block.
	"longrun": {packets: 8000, wallets: 200, perBlock: 20},
}

type attestorPreset struct {
	packets, state simpleload.Spec
}

var attestorPresets = map[string]attestorPreset{
	"burst": {
		packets: simpleload.Spec{Duration: 30 * time.Second, RatePerSecond: 500, Concurrency: 32},
		state:   simpleload.Spec{Duration: 30 * time.Second, RatePerSecond: 100, Concurrency: 16},
	},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// common holds the flags every scenario shares.
type common struct {
	provider      string
	observability bool
	ibcBin        string
	timeout       time.Duration
	jsonOut       bool
	keepOnFailure bool
	verbose       bool
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.provider, "provider", string(session.ProviderAnvil), "managed EVM provider: anvil or besu")
	fs.BoolVar(&c.observability, "observability", false,
		"export OTel telemetry from the relayer and attestors; start the stack first with "+
			"make -C cli/scripts/otel up (Grafana on http://localhost:3001)")
	fs.StringVar(&c.ibcBin, "ibc-bin", "", "path to the ibc binary (default: $IBC_BIN, else cli/bin/ibc)")
	fs.DurationVar(&c.timeout, "timeout", 30*time.Minute, "overall run budget, setup and teardown included")
	fs.BoolVar(&c.jsonOut, "json", false, "print the report as JSON instead of text")
	fs.BoolVar(&c.keepOnFailure, "keep-on-failure", false,
		"keep the deployment directory (relayer config, database, log) when the run fails")
	fs.BoolVar(&c.verbose, "v", false, "debug logging")
}

func (c *common) sessionOptions() (session.Options, error) {
	provider, err := session.ParseProvider(c.provider)
	if err != nil {
		return session.Options{}, err
	}
	return session.Options{
		Provider:      provider,
		Observability: c.observability,
		KeepOnFailure: c.keepOnFailure,
	}, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stderr, usageHeader)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "relayer":
		return runRelayer(args[1:], stdout, stderr)
	case "attestor":
		return runAttestor(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown scenario %q\n\n%s", args[0], usageHeader)
		return exitUsage
	}
}

func newFlagSet(name, summary string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: loadtest %s [flags]\n\n%s\n\nFlags:\n", name, summary)
		fs.PrintDefaults()
	}
	return fs
}

func runRelayer(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("relayer", "Sends IFT transfers from a pool of wallets, paced per new head on each source\n"+
		"Chain, and requires every packet to reach SUCCEEDED in the relayer status API.\n"+
		"Explicit flags override the preset.", stderr)
	var c common
	c.register(fs)
	preset := fs.String("preset", "burst", "load preset: burst (3000 packets, 200 wallets, 150/block) or "+
		"longrun (8000 packets, 200 wallets, 20/block)")
	packets := fs.Int("packets", 0, "total packets, split across directions (default from preset)")
	wallets := fs.Int("wallets", 0, "wallet pool size; each wallet has at most one transaction in flight "+
		"per Chain (default from preset)")
	perBlock := fs.Int("per-block", 0, "transfers dispatched per new head and direction (default from preset)")
	directions := fs.String("directions", "both", "a-to-b, b-to-a, or both")
	if err := fs.Parse(args); err != nil {
		return usageExit(err)
	}
	p, ok := relayerPresets[*preset]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "unknown preset %q: want burst or longrun\n", *preset)
		return exitUsage
	}
	cfg := relayerload.Config{
		Packets:  orDefault(*packets, p.packets),
		Wallets:  orDefault(*wallets, p.wallets),
		PerBlock: orDefault(*perBlock, p.perBlock),
	}
	switch *directions {
	case "both":
		cfg.Directions = []relayerload.Direction{relayerload.DirectionAtoB, relayerload.DirectionBtoA}
	case string(relayerload.DirectionAtoB), string(relayerload.DirectionBtoA):
		cfg.Directions = []relayerload.Direction{relayerload.Direction(*directions)}
	default:
		_, _ = fmt.Fprintf(stderr, "unknown directions %q: want a-to-b, b-to-a, or both\n", *directions)
		return exitUsage
	}
	opts, err := c.sessionOptions()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	cfg.Session = opts

	return execute(c, stdout, stderr, func(ctx context.Context) (reporter, error) {
		report, err := relayerload.Run(ctx, cfg)
		if report == nil {
			return nil, err
		}
		return report, err
	})
}

func runAttestor(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("attestor", "Seeds delivered transfers, then offers packet and state attestation requests\n"+
		"at a fixed rate to the attestor observing chain-a. Explicit flags override the preset.", stderr)
	var c common
	c.register(fs)
	preset := fs.String("preset", "burst", "load preset: burst (packets 30s@500rps x32, state 30s@100rps x16)")
	phases := fs.String("phases", "packets,state", "comma-separated phases to run: packets, state")
	duration := fs.Duration("duration", 0, "duration of each phase (default from preset)")
	packetsRPS := fs.Int("packets-rps", 0, "packet phase requests per second (default from preset)")
	packetsConc := fs.Int("packets-concurrency", 0, "packet phase concurrent callers (default from preset)")
	stateRPS := fs.Int("state-rps", 0, "state phase requests per second (default from preset)")
	stateConc := fs.Int("state-concurrency", 0, "state phase concurrent callers (default from preset)")
	if err := fs.Parse(args); err != nil {
		return usageExit(err)
	}
	p, ok := attestorPresets[*preset]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "unknown preset %q: want burst\n", *preset)
		return exitUsage
	}
	cfg := attestorload.Config{
		Packets: simpleload.Spec{
			Duration:      orDefault(*duration, p.packets.Duration),
			RatePerSecond: orDefault(*packetsRPS, p.packets.RatePerSecond),
			Concurrency:   orDefault(*packetsConc, p.packets.Concurrency),
		},
		State: simpleload.Spec{
			Duration:      orDefault(*duration, p.state.Duration),
			RatePerSecond: orDefault(*stateRPS, p.state.RatePerSecond),
			Concurrency:   orDefault(*stateConc, p.state.Concurrency),
		},
	}
	for phase := range strings.SplitSeq(*phases, ",") {
		phase = strings.TrimSpace(phase)
		if phase != attestorload.PhasePackets && phase != attestorload.PhaseState {
			_, _ = fmt.Fprintf(stderr, "unknown phase %q: want packets or state\n", phase)
			return exitUsage
		}
		cfg.Phases = append(cfg.Phases, phase)
	}
	opts, err := c.sessionOptions()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	cfg.Session = opts

	return execute(c, stdout, stderr, func(ctx context.Context) (reporter, error) {
		report, err := attestorload.Run(ctx, cfg)
		if report == nil {
			return nil, err
		}
		return report, err
	})
}

type reporter interface {
	WriteText(io.Writer) error
}

func passed(r reporter) bool {
	switch report := r.(type) {
	case *relayerload.Report:
		return report.Passed
	case *attestorload.Report:
		return report.Passed
	default:
		return false
	}
}

// execute wires logging, the binary override, the run budget, and signal
// handling around a scenario, then prints its report.
func execute(c common, stdout, stderr io.Writer, scenario func(context.Context) (reporter, error)) int {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))

	if c.ibcBin != "" {
		if err := os.Setenv(ibcBinEnv, c.ibcBin); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
	}
	if c.observability {
		slog.Info("observability enabled: expects the stack from `make -C cli/scripts/otel up`; " +
			"Grafana at http://localhost:3001")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-finished:
			return
		case <-ctx.Done():
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			slog.Warn("interrupted; tearing down (interrupt again to force-exit, then run `make -C e2e clean`)")
		} else {
			slog.Warn("run budget exhausted; tearing down", "timeout", c.timeout)
		}
		// A second signal takes the default action.
		stop()
	}()

	report, err := scenario(ctx)
	if err != nil {
		slog.Error("run failed", "err", err)
	}
	if report != nil {
		if c.jsonOut {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			if encodeErr := encoder.Encode(report); encodeErr != nil {
				slog.Error("encode report", "err", encodeErr)
				return exitFailed
			}
		} else if writeErr := report.WriteText(stdout); writeErr != nil {
			slog.Error("write report", "err", writeErr)
			return exitFailed
		}
	}
	if err != nil || report == nil || !passed(report) {
		return exitFailed
	}
	return exitOK
}

func usageExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitUsage
}

func orDefault[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}
