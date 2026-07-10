// Package ibclink drives the ibc link SUT as a black box. It routes each command to either the real
// binary or the temporary stub, execs it with the harness-compiled config, and parses stdout JSON /
// exit codes into harness-side wire types. It never reaches into the SUT's guts: the only thing
// crossing the wall is the binary's public CLI surface, config YAML, and JSON output.
package ibclink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Sentinel errors classify a non-zero SUT exit. Stub-served commands use sysexits codes; real-served
// commands that fail with other codes map to ErrInternal unless a command-specific contract says more.
// A failing command returns an *ExitError wrapping one of these, so a test asserts the failure class
// with errors.Is(err, ErrConfigInvalid) instead of string-matching stderr.
var (
	ErrConfigInvalid  = errors.New("ibc: config invalid (exit 64)")
	ErrRPCUnreachable = errors.New("ibc: rpc unreachable (exit 65)")
	ErrDeployFailed   = errors.New("ibc: deploy failed (exit 66)")
	ErrNotReady       = errors.New("ibc: not ready (exit 69)")
	ErrInternal       = errors.New("ibc: internal error (exit 70)")
)

type sutBin uint8

const (
	sutReal sutBin = iota
	sutStub
)

// commandRoutes is the swap ledger for the progressive stub-to-real migration: flip an entry when
// the real binary implements the command, delete the stub code it replaces; all-real means the stub
// is gone.
var commandRoutes = map[string]sutBin{
	"config validate": sutStub,
	"migrate up":      sutReal,
	"deploy":          sutStub,
	"relayer run":     sutStub,
}

// ExitError reports a SUT command that exited non-zero. Class is the sentinel for the exit code
// (matchable via errors.Is); Stderr is a tail of the command's human logs for debugging.
type ExitError struct {
	Code   int
	Class  error
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("%v: %s", e.Class, e.Stderr)
	}
	return e.Class.Error()
}

// Unwrap exposes the sentinel class so errors.Is(err, ErrConfigInvalid) matches.
func (e *ExitError) Unwrap() error { return e.Class }

// Runner is the public harness handle for the ibc link binary.
type Runner interface {
	// ValidateConfig runs `config validate [--live]`. It returns the parsed result whenever the stub
	// emitted one (it does for ok, structural-invalid and live-unreachable), and an *ExitError for any
	// non-zero exit: ErrConfigInvalid (64) for structural failure, ErrRPCUnreachable (65) when --live
	// could not reach a chain. So a caller reads the result for detail and errors.Is for the class.
	ValidateConfig(ctx context.Context, live bool) (*wire.ValidateResult, error)
	// MigrateUp runs `migrate up`, applying the DB schema expected by later commands.
	MigrateUp(ctx context.Context) error
	// Deploy runs `deploy`, returning the per-chain fixture metadata (or ErrDeployFailed on exit 66).
	Deploy(ctx context.Context) (*wire.Deployment, error)
	// Run starts `ibc relayer run` as a long-lived daemon and blocks until it reports readiness.
	Run(ctx context.Context) (Daemon, error)
}

// Options configure a Runner. Only ConfigPath is required.
type Options struct {
	// ConfigPath is the compiled config; the runner passes its directory as --home and basename as --config.
	ConfigPath string
	// LogPath, if set, receives every invocation's stderr (appended) for the diagnostics bundle.
	LogPath string
}

// runner is the concrete Runner: both SUT binaries + the config they drive, plus capture wiring.
type runner struct {
	realBin    string
	stubBin    string
	configPath string
	configHome string
	configName string
	logPath    string
}

var _ Runner = (*runner)(nil)

// NewRunner builds a Runner for the given compiled config. It resolves both binary paths without
// requiring either file to exist until a routed command actually runs, so constructing one is cheap and
// side-effect free.
func NewRunner(o Options) (Runner, error) {
	if o.ConfigPath == "" {
		return nil, errors.New("ibclink: NewRunner requires a ConfigPath")
	}
	return &runner{
		realBin:    ResolvedRealBin(),
		stubBin:    ResolvedStubBin(),
		configPath: o.ConfigPath,
		configHome: filepath.Dir(o.ConfigPath),
		configName: filepath.Base(o.ConfigPath),
		logPath:    o.LogPath,
	}, nil
}

func (r *runner) binFor(label string) (string, error) {
	target, ok := commandRoutes[label]
	if !ok {
		return "", fmt.Errorf("ibclink: no SUT route for command label %q", label)
	}
	switch target {
	case sutReal:
		return r.realBin, nil
	case sutStub:
		return r.stubBin, nil
	default:
		return "", fmt.Errorf("ibclink: invalid SUT route for command label %q", label)
	}
}

func (r *runner) ValidateConfig(ctx context.Context, live bool) (*wire.ValidateResult, error) {
	args := append([]string{"config", "validate"}, r.configArgs()...)
	if live {
		args = append(args, "--live")
	}
	res, err := r.exec(ctx, "config validate", args...)
	if err != nil {
		return nil, err
	}

	// The stub emits a ValidateResult on stdout for ok (0), structural-invalid (64) and
	// live-unreachable (65), so decode it whenever it parses; a miss on a non-zero exit just means
	// that failure had no structured body.
	var out wire.ValidateResult
	decoded := json.Unmarshal(res.stdout, &out) == nil

	if res.code == wire.ExitOK {
		if !decoded {
			return nil, fmt.Errorf(
				"ibc config validate: exit 0 but stdout is not a ValidateResult: %q",
				string(res.stdout),
			)
		}
		return &out, nil
	}
	var parsed *wire.ValidateResult
	if decoded {
		parsed = &out
	}
	return parsed, &ExitError{Code: res.code, Class: classify(res.code), Stderr: snippet(res.stderr)}
}

func (r *runner) MigrateUp(ctx context.Context) error {
	args := append([]string{"migrate", "up"}, r.configArgs()...)
	_, err := runJSON[wire.MigrationUpResult](ctx, r, "migrate up", args...)
	return err
}

func (r *runner) Deploy(ctx context.Context) (*wire.Deployment, error) {
	args := append([]string{"deploy"}, r.configArgs()...)
	return runJSON[wire.Deployment](ctx, r, "deploy", args...)
}

func (r *runner) configArgs() []string {
	return []string{"--home", r.configHome, "--config", r.configName}
}

// runJSON execs a one-shot SUT command and decodes its stdout JSON into *T. It is the shared tail of
// every command whose contract is "exit 0 with a JSON body, else a non-zero sysexits code": a non-zero
// exit becomes an *ExitError classed by the code, and a stdout that will not decode is a hard error
// naming the command. Real-served commands may use non-sysexits failure codes; unknown codes classify as
// ErrInternal. ValidateConfig deliberately does not use it — that verb also decodes a body on the 64/65
// exits, so it keeps its own dual-path decode.
func runJSON[T any](ctx context.Context, r *runner, label string, args ...string) (*T, error) {
	res, err := r.exec(ctx, label, args...)
	if err != nil {
		return nil, err
	}
	if res.code != wire.ExitOK {
		return nil, &ExitError{Code: res.code, Class: classify(res.code), Stderr: snippet(res.stderr)}
	}
	var out T
	if err := json.Unmarshal(res.stdout, &out); err != nil {
		return nil, fmt.Errorf("ibc %s: decode stdout: %w (%q)", label, err, string(res.stdout))
	}
	return &out, nil
}
