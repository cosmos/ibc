// SPDX-License-Identifier: Apache-2.0

// Command loadtest drives load against a local two-Chain IBC environment
// built by the harness: Anvil (or Besu) Chains joined by an attested mesh,
// the IBC CLI relayer, and its attestors. See README.md.
package main

import (
	"context"
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

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/loadtest/attestorload"
	"github.com/cosmos/ibc/e2e/internal/loadtest/relayerload"
	"github.com/cosmos/ibc/e2e/internal/loadtest/session"
	"github.com/cosmos/ibc/e2e/internal/loadtest/simpleload"
)

const (
	exitOK      = 0
	exitFailed  = 1
	exitUsage   = 2
	ibcBinEnv   = "IBC_BIN"
	usageHeader = `Usage: loadtest <scenario> [flags]

Scenarios:
  relayer    paced IFT transfers from a wallet pool in both directions; every packet must relay
  attestor   fixed-rate packet and state attestation requests against one attestor

Run "loadtest <scenario> -h" for the scenario's flags.
`
)

type relayerPreset struct {
	packets, wallets, perBlock int
}

var relayerPresets = map[string]relayerPreset{
	// Short, dense bursts: 3000 packets at 150 per block interval.
	"burst": {packets: 3000, wallets: 200, perBlock: 150},
	// Sustained, light load: 8000 packets at 20 per block interval.
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
	fs.StringVar(&c.provider, "provider", string(environment.EVMProviderAnvil), "managed EVM provider: anvil or besu")
	fs.BoolVar(&c.observability, "observability", false,
		"export OTel telemetry from the relayer and attestors; start the stack first with "+
			"make -C cli/scripts/otel up (Grafana on http://localhost:3001)")
	fs.StringVar(&c.ibcBin, "ibc-bin", "", "path to the ibc binary (default: $IBC_BIN, else cli/bin/ibc)")
	fs.DurationVar(&c.timeout, "timeout", 30*time.Minute, "overall run budget, setup and teardown included")
	fs.BoolVar(&c.jsonOut, "json", false, "print the report as JSON instead of text")
	fs.BoolVar(&c.keepOnFailure, "keep-on-failure", false,
		"leave the environment (containers, relayer, deployment directory) running when the run fails")
	fs.BoolVar(&c.verbose, "v", false, "debug logging")
}

func (c *common) sessionOptions() (session.Options, error) {
	provider, err := environment.ParseEVMProvider(c.provider)
	if err != nil {
		return session.Options{}, err
	}
	if c.timeout <= 0 {
		return session.Options{}, errors.New("--timeout must be positive")
	}
	return session.Options{Provider: provider, Observability: c.observability}, nil
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

// parse parses args and returns the names of the flags set explicitly, so
// presets fill only the others. The flag package reports its own errors.
func parse(fs *flag.FlagSet, args []string) (map[string]bool, error) {
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		err := fmt.Errorf("unexpected arguments: %q", fs.Args())
		_, _ = fmt.Fprintln(fs.Output(), err)
		return nil, err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set, nil
}

func runRelayer(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("relayer", "Sends IFT transfers from a wallet pool, --per-block per source block interval and\n"+
		"direction, and requires every packet to reach SUCCEEDED in the relayer status API.\n"+
		"Explicit flags override the preset.", stderr)
	var c common
	c.register(fs)
	burst := relayerPresets["burst"]
	preset := fs.String("preset", "burst", "load preset: burst (3000 packets, 200 wallets, 150/block) or "+
		"longrun (8000 packets, 200 wallets, 20/block)")
	packets := fs.Int("packets", burst.packets, "total packets, split across directions (default from preset)")
	wallets := fs.Int("wallets", burst.wallets, "wallet pool size, at least 2; each wallet has at most one "+
		"transaction in flight per direction (default from preset)")
	perBlock := fs.Int("per-block", burst.perBlock, "transfers offered per source block interval and direction "+
		"(default from preset)")
	directions := fs.String("directions", "both", "a-to-b, b-to-a, or both")
	set, err := parse(fs, args)
	if err != nil {
		return parseExit(err)
	}
	p, ok := relayerPresets[*preset]
	if !ok {
		return usageError(stderr, fmt.Errorf("unknown preset %q: want burst or longrun", *preset))
	}
	cfg := relayerload.Config{
		Packets:  pick(set["packets"], *packets, p.packets),
		Wallets:  pick(set["wallets"], *wallets, p.wallets),
		PerBlock: pick(set["per-block"], *perBlock, p.perBlock),
		JSON:     c.jsonOut,
	}
	switch *directions {
	case "both":
		cfg.Directions = []relayerload.Direction{relayerload.DirectionAtoB, relayerload.DirectionBtoA}
	case string(relayerload.DirectionAtoB), string(relayerload.DirectionBtoA):
		cfg.Directions = []relayerload.Direction{relayerload.Direction(*directions)}
	default:
		return usageError(stderr, fmt.Errorf("unknown directions %q: want a-to-b, b-to-a, or both", *directions))
	}
	switch {
	case cfg.Packets < len(cfg.Directions):
		err = fmt.Errorf("--packets must be at least the number of directions (%d)", len(cfg.Directions))
	case cfg.Wallets < 2:
		err = errors.New("--wallets must be at least 2: receivers are pool wallets")
	case cfg.PerBlock <= 0:
		err = errors.New("--per-block must be positive")
	}
	if err != nil {
		return usageError(stderr, err)
	}
	opts, err := c.sessionOptions()
	if err != nil {
		return usageError(stderr, err)
	}
	return execute(c, stderr, relayerload.SessionOptions(opts), func(ctx context.Context, s *session.Session) error {
		return relayerload.Run(ctx, s, cfg, stdout)
	})
}

func runAttestor(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("attestor", "Seeds packets, then offers packet and state attestation requests at a fixed\n"+
		"rate to the attestor observing chain-a. Explicit flags override the preset.", stderr)
	var c common
	c.register(fs)
	burst := attestorPresets["burst"]
	preset := fs.String("preset", "burst", "load preset: burst (packets 30s@500rps x32, state 30s@100rps x16)")
	phases := fs.String("phases", "packets,state", "comma-separated phases to run: packets, state")
	duration := fs.Duration("duration", burst.packets.Duration, "duration of each phase (default from preset)")
	packetsRPS := fs.Int("packets-rps", burst.packets.RatePerSecond, "packet phase requests per second "+
		"(default from preset)")
	packetsConc := fs.Int("packets-concurrency", burst.packets.Concurrency, "packet phase concurrent callers "+
		"(default from preset)")
	stateRPS := fs.Int("state-rps", burst.state.RatePerSecond, "state phase requests per second (default from preset)")
	stateConc := fs.Int("state-concurrency", burst.state.Concurrency, "state phase concurrent callers "+
		"(default from preset)")
	maxErrorRate := fs.Float64("max-error-rate", 0.01, "largest share of sent calls that may fail or time out "+
		"in a passing phase, 0 to 1")
	set, err := parse(fs, args)
	if err != nil {
		return parseExit(err)
	}
	p, ok := attestorPresets[*preset]
	if !ok {
		return usageError(stderr, fmt.Errorf("unknown preset %q: want burst", *preset))
	}
	cfg := attestorload.Config{
		Packets: simpleload.Spec{
			Duration:      pick(set["duration"], *duration, p.packets.Duration),
			RatePerSecond: pick(set["packets-rps"], *packetsRPS, p.packets.RatePerSecond),
			Concurrency:   pick(set["packets-concurrency"], *packetsConc, p.packets.Concurrency),
		},
		State: simpleload.Spec{
			Duration:      pick(set["duration"], *duration, p.state.Duration),
			RatePerSecond: pick(set["state-rps"], *stateRPS, p.state.RatePerSecond),
			Concurrency:   pick(set["state-concurrency"], *stateConc, p.state.Concurrency),
		},
		MaxErrorRate: *maxErrorRate,
		JSON:         c.jsonOut,
	}
	for phase := range strings.SplitSeq(*phases, ",") {
		phase = strings.TrimSpace(phase)
		if phase != attestorload.PhasePackets && phase != attestorload.PhaseState {
			return usageError(stderr, fmt.Errorf("unknown phase %q: want packets or state", phase))
		}
		cfg.Phases = append(cfg.Phases, phase)
	}
	for _, spec := range []simpleload.Spec{cfg.Packets, cfg.State} {
		if spec.Duration <= 0 || spec.RatePerSecond <= 0 || spec.Concurrency <= 0 {
			return usageError(stderr, errors.New("--duration, rates, and concurrencies must be positive"))
		}
	}
	if cfg.MaxErrorRate < 0 || cfg.MaxErrorRate > 1 {
		return usageError(stderr, errors.New("--max-error-rate must be between 0 and 1"))
	}
	opts, err := c.sessionOptions()
	if err != nil {
		return usageError(stderr, err)
	}
	return execute(c, stderr, attestorload.SessionOptions(opts), func(ctx context.Context, s *session.Session) error {
		return attestorload.Run(ctx, s, cfg, stdout)
	})
}

// execute wires logging, the binary override, the run budget, and signal
// handling around a scenario, starts its session, and tears it down unless
// the run failed and --keep-on-failure asks to keep it.
func execute(
	c common,
	stderr io.Writer,
	opts session.Options,
	scenario func(context.Context, *session.Session) error,
) int {
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

	s, err := session.Start(ctx, opts)
	if err != nil {
		slog.Error("setup failed", "err", err)
		return exitFailed
	}
	err = scenario(ctx, s)
	if err != nil {
		slog.Error("run failed", "err", err)
	}
	if err != nil && c.keepOnFailure {
		slog.Warn("kept the environment for inspection; run `make -C e2e clean` when done", "dir", s.Dir())
		return exitFailed
	}
	if closeErr := s.Close(ctx); closeErr != nil {
		slog.Error("teardown failed", "err", closeErr)
		return exitFailed
	}
	if err != nil {
		return exitFailed
	}
	return exitOK
}

func parseExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitUsage
}

func usageError(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	return exitUsage
}

// pick returns value when its flag was set explicitly, else the preset's.
func pick[T any](set bool, value, preset T) T {
	if set {
		return value
	}
	return preset
}
