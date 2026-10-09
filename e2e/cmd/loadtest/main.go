// SPDX-License-Identifier: Apache-2.0

// Command loadtest drives load against a local two-Chain IBC environment
// built by the harness: Anvil (or Besu) Chains joined by an attested mesh,
// the IBC CLI relayer, and its attestors. See README.md.
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

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/loadtest/attestorload"
	"github.com/cosmos/ibc/e2e/internal/loadtest/relayerload"
	"github.com/cosmos/ibc/e2e/internal/loadtest/session"
	"github.com/cosmos/ibc/e2e/internal/loadtest/simpleload"
)

const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
	// exitInconclusive: the relayer relayed every packet sent, but not every
	// packet could be sent.
	exitInconclusive = 3
	usageHeader      = `Usage: loadtest <scenario> [flags]

Scenarios:
  relayer    paced IFT transfers from a wallet pool in both directions; every packet must relay
  attestor   fixed-rate packet and state attestation requests against one attestor

Run "loadtest <scenario> -h" for the scenario's flags.
`
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// common holds the flags every scenario shares.
type common struct {
	provider      string
	observability bool
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
	fs.DurationVar(&c.timeout, "timeout", 30*time.Minute, "budget for setup and the run; teardown follows "+
		"with its own bound")
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

// parse parses args and rejects positional arguments. The flag package
// reports its own errors.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		err := fmt.Errorf("unexpected arguments: %q", fs.Args())
		_, _ = fmt.Fprintln(fs.Output(), err)
		return err
	}
	return nil
}

func runRelayer(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("relayer", "Sends IFT transfers from a wallet pool, --per-block per source block interval and\n"+
		"direction, and requires every packet to reach SUCCEEDED in the relayer status API.\n"+
		"The defaults are a short, dense burst; --packets 8000 --per-block 20 is a sustained, light run.", stderr)
	var c common
	c.register(fs)
	packets := fs.Int("packets", 3000, "total packets, split across directions")
	wallets := fs.Int("wallets", 200, "wallet pool size; each wallet has at most one transaction "+
		"in flight per direction")
	perBlock := fs.Int("per-block", 150, "transfers offered per source block interval and direction")
	directions := fs.String("directions", "both", "a-to-b, b-to-a, or both")
	if err := parse(fs, args); err != nil {
		return parseExit(err)
	}
	cfg := relayerload.Config{Packets: *packets, Wallets: *wallets, PerBlock: *perBlock}
	switch *directions {
	case "both":
		cfg.Directions = []relayerload.Direction{relayerload.DirectionAtoB, relayerload.DirectionBtoA}
	case string(relayerload.DirectionAtoB), string(relayerload.DirectionBtoA):
		cfg.Directions = []relayerload.Direction{relayerload.Direction(*directions)}
	default:
		return usageError(stderr, fmt.Errorf("unknown directions %q: want a-to-b, b-to-a, or both", *directions))
	}
	var err error
	switch {
	case cfg.Packets < len(cfg.Directions):
		err = fmt.Errorf("--packets must be at least the number of directions (%d)", len(cfg.Directions))
	case cfg.Wallets <= 0:
		err = errors.New("--wallets must be positive")
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
	return execute(
		c,
		stdout,
		stderr,
		relayerload.SessionOptions(opts, cfg),
		func(ctx context.Context, s *session.Session) (report, error) {
			return relayerload.Run(ctx, s, cfg)
		},
	)
}

func runAttestor(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("attestor", "Sends packet and state attestation requests at a fixed rate to the attestor\n"+
		"observing chain-a, without waiting for replies. The packets phase first seeds packets to attest.", stderr)
	var c common
	c.register(fs)
	phases := fs.String("phases", "packets,state", "comma-separated phases to run in order: packets, state")
	duration := fs.Duration("duration", 30*time.Second, "duration of each phase")
	packetsRPS := fs.Int("packets-rps", 500, "packet phase requests per second")
	stateRPS := fs.Int("state-rps", 100, "state phase requests per second")
	maxErrorRate := fs.Float64("max-error-rate", 0.01, "largest share of sent calls that may fail or time out "+
		"in a passing phase, 0 to 1")
	if err := parse(fs, args); err != nil {
		return parseExit(err)
	}
	cfg := attestorload.Config{
		Packets:      simpleload.Spec{Duration: *duration, RatePerSecond: *packetsRPS},
		State:        simpleload.Spec{Duration: *duration, RatePerSecond: *stateRPS},
		MaxErrorRate: *maxErrorRate,
	}
	if *duration <= 0 {
		return usageError(stderr, errors.New("--duration must be positive"))
	}
	rates := map[string]int{attestorload.PhasePackets: *packetsRPS, attestorload.PhaseState: *stateRPS}
	for phase := range strings.SplitSeq(*phases, ",") {
		phase = strings.TrimSpace(phase)
		switch rate, ok := rates[phase]; {
		case !ok:
			return usageError(stderr, fmt.Errorf("unknown phase %q: want packets or state", phase))
		case rate <= 0:
			return usageError(stderr, fmt.Errorf("--%s-rps must be positive", phase))
		}
		cfg.Phases = append(cfg.Phases, phase)
	}
	// Written to reject NaN, which fails every comparison.
	if !(cfg.MaxErrorRate >= 0 && cfg.MaxErrorRate <= 1) {
		return usageError(stderr, errors.New("--max-error-rate must be between 0 and 1"))
	}
	opts, err := c.sessionOptions()
	if err != nil {
		return usageError(stderr, err)
	}
	return execute(
		c,
		stdout,
		stderr,
		attestorload.SessionOptions(opts, cfg),
		func(ctx context.Context, s *session.Session) (report, error) {
			return attestorload.Run(ctx, s, cfg)
		},
	)
}

// report is a scenario's outcome.
type report interface {
	WriteText(w io.Writer) error
}

// execute wires logging, the run budget, and signal
// handling around a scenario, starts its session, writes the scenario's
// report to stdout, and tears the session down unless setup or the run
// failed and --keep-on-failure asks to keep it.
func execute(
	c common,
	stdout, stderr io.Writer,
	opts session.Options,
	scenario func(context.Context, *session.Session) (report, error),
) int {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))

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
	} else {
		var r report
		if r, err = scenario(ctx, s); err != nil {
			slog.Error("run did not pass", "err", err)
		}
		if writeErr := writeReport(stdout, r, c.jsonOut); writeErr != nil {
			slog.Error("write report", "err", writeErr)
			err = errors.Join(err, writeErr)
		}
	}
	if s == nil {
		return exitFailed
	}
	if err != nil && c.keepOnFailure {
		kept := []any{"run_id", s.Env.RunID()}
		if s.Dir() != "" {
			kept = append(kept, "dir", s.Dir())
		}
		slog.Warn("kept the environment for inspection; run `make -C e2e clean` when done", kept...)
		return exitFailed
	}
	if closeErr := s.Close(ctx); closeErr != nil {
		slog.Error("teardown failed", "err", closeErr)
		return exitFailed
	}
	return exitCode(err)
}

func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, relayerload.ErrLoadNotDelivered):
		return exitInconclusive
	default:
		return exitFailed
	}
}

func writeReport(w io.Writer, r report, asJSON bool) error {
	if !asJSON {
		return r.WriteText(w)
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
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
