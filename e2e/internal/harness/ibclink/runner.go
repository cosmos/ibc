// Package ibclink drives the ibc link SUT as a black box over its public CLI, config YAML, and JSON output.
package ibclink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
)

var (
	ErrConfigInvalid       = errors.New("ibc: config invalid (exit 64)")
	ErrRPCUnreachable      = errors.New("ibc: rpc unreachable (exit 65)")
	ErrTestAppDeployFailed = errors.New("ibc: test app deploy failed (exit 66)")
	ErrNotReady            = errors.New("ibc: not ready (exit 69)")
	ErrInternal            = errors.New("ibc: internal error (exit 70)")
)

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

type Driver struct {
	realBin    string
	stubBin    string
	configHome string
	configName string
	bindings   processBindings
}

func NewDriver(configPath string) (*Driver, error) {
	if configPath == "" {
		return nil, errors.New("ibclink: NewDriver requires a config path")
	}
	return &Driver{
		realBin:    ResolvedRealBin(),
		stubBin:    ResolvedStubBin(),
		configHome: filepath.Dir(configPath),
		configName: filepath.Base(configPath),
	}, nil
}

func (r *Driver) ValidateConfig(ctx context.Context, live bool) (*wire.ValidateResult, error) {
	args := append([]string{"config", "validate"}, r.configArgs()...)
	if live {
		args = append(args, "--live")
	}
	res, err := r.exec(ctx, r.stubBin, "config validate", args...)
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

func (r *Driver) MigrateUp(ctx context.Context) error {
	args := append([]string{"migrate", "up"}, r.configArgs()...)
	res, err := r.exec(ctx, r.realBin, "migrate up", args...)
	if err != nil {
		return err
	}
	if res.code != wire.ExitOK {
		return &ExitError{Code: res.code, Class: classify(res.code), Stderr: snippet(res.stderr)}
	}
	var out wire.MigrationUpResult
	if err := json.Unmarshal(res.stdout, &out); err != nil {
		return fmt.Errorf("ibc migrate up: decode stdout: %w (%q)", err, string(res.stdout))
	}
	return nil
}

func (r *Driver) DeployTestApps(ctx context.Context) (*wire.TestAppDeployment, error) {
	args := append([]string{"test-apps", "deploy"}, r.configArgs()...)
	res, err := r.exec(ctx, r.stubBin, "test-apps deploy", args...)
	if err != nil {
		return nil, err
	}
	return decodeTestAppDeploymentResult(res)
}

// A non-zero deployment may still have created durable contracts. The
// stub prints every receipt it has before exiting, so callers receive the
// partial deployment together with the classified error.
func decodeTestAppDeploymentResult(res *result) (*wire.TestAppDeployment, error) {
	var deployment wire.TestAppDeployment
	decoded := json.Unmarshal(res.stdout, &deployment) == nil
	if res.code == wire.ExitOK {
		if !decoded {
			return nil, fmt.Errorf(
				"ibc test-apps deploy: exit 0 but stdout is not a TestAppDeployment: %q",
				string(res.stdout),
			)
		}
		return &deployment, nil
	}
	exitErr := &ExitError{Code: res.code, Class: classify(res.code), Stderr: snippet(res.stderr)}
	if decoded {
		return &deployment, exitErr
	}
	return nil, exitErr
}

func (r *Driver) configArgs() []string {
	return []string{"--home", r.configHome, "--config", r.configName}
}
