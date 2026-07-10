// Package ibclink drives the ibc link SUT as a black box over its public CLI, config YAML, and JSON output.
package ibclink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

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

var commandRoutes = map[string]sutBin{
	"config validate": sutStub,
	"migrate up":      sutReal,
	"deploy":          sutStub,
	"relayer run":     sutStub,
}

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

func (e *ExitError) Unwrap() error { return e.Class }

type Runner interface {
	ValidateConfig(ctx context.Context, live bool) (*wire.ValidateResult, error)
	MigrateUp(ctx context.Context) error
	Deploy(ctx context.Context) (*wire.Deployment, error)
	Run(ctx context.Context) (Daemon, error)
}

type Options struct {
	ConfigPath string
	LogPath    string
}

type runner struct {
	realBin    string
	stubBin    string
	configPath string
	configHome string
	configName string
	logPath    string
}

var _ Runner = (*runner)(nil)

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
