// SPDX-License-Identifier: Apache-2.0

// Command loadtest drives load against a local two-Chain IBC environment
// built by the harness: Anvil (or Besu) Chains joined by an attested (or Besu
// QBFT) mesh, the IBC CLI relayer, and its attestors. See README.md.
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
	fs.DurationVar(&c.timeout, "timeout", 30*time.Minute, "budget for setup and the run beyond its planned "+
		"send time (a relayer --duration, or the attestor's phases); teardown follows with its own bound")
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
		"The defaults are a short, dense burst; --duration 20m --per-block 10 is a long, moderate run.", stderr)
	var c common
	c.register(fs)
	packets := fs.Int("packets", 3000, "total packets, split across directions")
	duration := fs.Duration("duration", 0, "send for this long in every direction instead of a packet count "+
		"(not with --packets); --timeout comes on top of it")
	wallets := fs.Int("wallets", 200, "wallet pool size; each wallet has at most one transaction "+
		"in flight per direction")
	perBlock := fs.Int("per-block", 150, "transfers offered per source block interval and direction")
	batch := fs.Int("batch", 1, "transfers per transaction, sent through the IFT batch shim when above 1; "+
		"it must divide --per-block")
	directions := fs.String("directions", "both", "a-to-b, b-to-a, or both")
	lightClient := fs.String("light-client", string(relayerload.LightClientAttested),
		"light clients between the Chains: attested, or besu-qbft (needs --provider besu)")
	if err := parse(fs, args); err != nil {
		return parseExit(err)
	}
	cfg := relayerload.Config{
		LightClient: relayerload.LightClient(*lightClient),
		Packets:     *packets,
		Duration:    *duration,
		Wallets:     *wallets,
		PerBlock:    *perBlock,
		Batch:       *batch,
		Directions:  []relayerload.Direction{relayerload.Direction(*directions)},
	}
	if *directions == "both" {
		cfg.Directions = []relayerload.Direction{relayerload.DirectionAtoB, relayerload.DirectionBtoA}
	}
	// A duration replaces the default packet count, but not one given.
	if cfg.Duration != 0 && !flagSet(fs, "packets") {
		cfg.Packets = 0
	}
	opts, err := c.sessionOptions()
	if err == nil {
		err = cfg.Validate(opts.Provider)
	}
	if err != nil {
		return usageError(stderr, err)
	}
	return execute(
		c,
		stdout,
		stderr,
		cfg.Duration,
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
	for phase := range strings.SplitSeq(*phases, ",") {
		cfg.Phases = append(cfg.Phases, strings.TrimSpace(phase))
	}
	opts, err := c.sessionOptions()
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		return usageError(stderr, err)
	}
	return execute(
		c,
		stdout,
		stderr,
		time.Duration(len(cfg.Phases))*(*duration),
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

// execute wires logging, the run budget (--timeout on top of the scenario's
// planned send time), and signal handling around a scenario, starts its
// session, writes the scenario's report to stdout, and tears the session
// down unless setup or the run failed and --keep-on-failure asks to keep it.
// Teardown is deferred, so a panic tears down too.
func execute(
	c common,
	stdout, stderr io.Writer,
	planned time.Duration,
	opts session.Options,
	scenario func(context.Context, *session.Session) (report, error),
) (code int) {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))

	if c.observability {
		slog.Info("observability enabled: expects the stack from `make -C cli/scripts/otel up`")
	}

	budget := c.timeout + planned
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	announce := context.AfterFunc(ctx, func() {
		if errors.Is(ctx.Err(), context.Canceled) {
			slog.Warn("interrupted; tearing down (interrupt again to force-exit, then run `make -C e2e clean`)")
		} else {
			slog.Warn("run budget exhausted; tearing down", "budget", budget)
		}
		// A second signal takes the default action.
		stop()
	})
	defer announce()

	s, err := session.Start(ctx, opts)
	// finish tears down and sets the exit code once the run ends; a panic
	// still tears down, though its exit status wins.
	defer func() { code = finish(ctx, s, err, c.keepOnFailure) }()
	if err != nil {
		slog.Error("setup failed", "err", err)
		return code
	}
	var r report
	if r, err = scenario(ctx, s); err != nil {
		slog.Error("run did not pass", "err", err)
	}
	if writeErr := writeReport(stdout, r, c.jsonOut); writeErr != nil {
		slog.Error("write report", "err", writeErr)
		err = errors.Join(err, fmt.Errorf("%w: %w", errReportNotWritten, writeErr))
	}
	return code
}

// errReportNotWritten fails a run whatever its verdict, as a failed teardown
// does.
var errReportNotWritten = errors.New("report not written")

// finish keeps s if err is set and keep asks to, closes it otherwise, and
// returns the exit code for err. A failed teardown fails the run.
func finish(ctx context.Context, s *session.Session, err error, keep bool) int {
	code := exitCode(err)
	switch {
	case s == nil:
	case err != nil && keep:
		s.Keep(ctx)
	default:
		if closeErr := s.Close(ctx); closeErr != nil {
			slog.Error("teardown failed", "err", closeErr)
			code = exitFailed
		}
	}
	return code
}

func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, errReportNotWritten):
		return exitFailed
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

// flagSet reports whether name was given on the command line.
func flagSet(fs *flag.FlagSet, name string) bool {
	given := false
	fs.Visit(func(f *flag.Flag) { given = given || f.Name == name })
	return given
}

func usageError(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	return exitUsage
}
