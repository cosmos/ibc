// Package ibclink drives the ibc link SUT as a black box over its public CLI, config YAML, and JSON output.
package ibclink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

var ErrInternal = errors.New("ibc: internal error")

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
	bin         string
	configHome  string
	configName  string
	bindings    processBindings
	relayerOpts RelayerOptions
}

// ConfigureRelayer stores the identifier mappings StartRelayer adapts the
// relayer's wire contract with. Callers set it when writing the config file.
func (r *Driver) ConfigureRelayer(opts RelayerOptions) { r.relayerOpts = opts }

func NewDriver(configPath string) (*Driver, error) {
	if configPath == "" {
		return nil, errors.New("ibclink: NewDriver requires a config path")
	}
	return &Driver{
		bin:        ResolvedBin(),
		configHome: filepath.Dir(configPath),
		configName: filepath.Base(configPath),
	}, nil
}

func (r *Driver) MigrateUp(ctx context.Context) error {
	args := append([]string{"migrate", "up"}, r.configArgs()...)
	res, err := r.exec(ctx, r.bin, "migrate up", args...)
	if err != nil {
		return err
	}
	if res.code != 0 {
		return &ExitError{Code: res.code, Class: ErrInternal, Stderr: snippet(res.stderr)}
	}
	if !json.Valid(res.stdout) {
		return fmt.Errorf("ibc migrate up: stdout is not JSON: %q", string(res.stdout))
	}
	return nil
}

func (r *Driver) configArgs() []string {
	return []string{"--home", r.configHome, "--config", r.configName}
}
