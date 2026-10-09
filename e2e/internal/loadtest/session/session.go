// SPDX-License-Identifier: Apache-2.0

// Package session starts and tears down the environment a load scenario runs
// against: two managed EVM Chains joined by an attested mesh, the traffic
// applications, and optionally an auto-relaying relayer.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
)

// closeTimeout bounds Environment teardown, which stops containers and
// managed processes; it matches the e2e suite's cleanup budget.
const closeTimeout = 30 * time.Second

// Options shape the environment.
type Options struct {
	Provider environment.EVMProvider
	// Observability exports OTel telemetry from every managed IBC CLI process
	// (attestors and the relayer) to the local LGTM stack.
	Observability bool
	// Routes the applications are deployed for and the relayer serves.
	Routes []traffic.Route
	// ConfigureRelayer adjusts the generated relayer configuration.
	ConfigureRelayer func(*ibccli.RelayerConfig)
	// StartRelayer starts the relayer after deployment. Without it the
	// relayer configuration is written but no relayer runs.
	StartRelayer bool
}

// Session holds every resource a scenario uses. Close releases them.
type Session struct {
	Provider   environment.EVMProvider
	Env        *environment.Environment
	Deployment *traffic.Deployment
	// Relayer is nil unless Options.StartRelayer.
	Relayer *ibccli.Relayer
	// Deployer deployed the applications and holds the minted token supply.
	Deployer traffic.Signer

	dir string
}

// Start realizes the environment, deploys the applications and relayer
// configuration for opts.Routes, and starts the relayer if asked. On error
// every resource acquired so far is released.
func Start(ctx context.Context, opts Options) (_ *Session, err error) {
	chains, err := environment.ManagedEVMChains(opts.Provider, traffic.ChainA, traffic.ChainB)
	if err != nil {
		return nil, err
	}
	spec, runtime := traffic.AttestedMesh(chains...)
	runtime.Observability = opts.Observability

	dir, err := os.MkdirTemp("", "ibc-loadtest-")
	if err != nil {
		return nil, fmt.Errorf("create deployment directory: %w", err)
	}
	s := &Session{Provider: opts.Provider, dir: dir}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close(ctx))
		}
	}()

	started := time.Now()
	slog.InfoContext(ctx, "starting environment", "provider", opts.Provider, "observability", opts.Observability)
	s.Env, err = environment.Start(ctx, spec, runtime)
	if err != nil {
		return nil, fmt.Errorf("start environment: %w", err)
	}
	slog.InfoContext(ctx, "environment started", "run_id", s.Env.RunID(), "elapsed", since(started))

	s.Deployer, err = traffic.NewSigner()
	if err != nil {
		return nil, err
	}
	relayerSigner, err := traffic.NewSigner()
	if err != nil {
		return nil, err
	}

	started = time.Now()
	driver, deployment, err := traffic.DeployWithRelayerConfig(
		ctx, s.Env, dir, s.Deployer, relayerSigner, opts.ConfigureRelayer, opts.Routes...,
	)
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}
	s.Deployment = deployment
	slog.InfoContext(ctx, "applications deployed", "dir", dir, "elapsed", since(started))

	if !opts.StartRelayer {
		return s, nil
	}
	s.Relayer, err = traffic.StartRelayer(ctx, driver, s.Env)
	if err != nil {
		return nil, fmt.Errorf("start relayer: %w", err)
	}
	slog.InfoContext(ctx, "relayer started", "http", s.Relayer.Ready().HTTP)
	return s, nil
}

// Dir is the deployment directory: relayer config, database, and log.
func (s *Session) Dir() string { return s.dir }

// Close stops the relayer, closes the Environment, and removes the
// deployment directory. It runs on a context detached from ctx's
// cancellation so an interrupted run still tears down.
func (s *Session) Close(ctx context.Context) error {
	ctx = context.WithoutCancel(ctx)
	var errs []error
	if s.Relayer != nil {
		stopCtx, cancel := context.WithTimeout(ctx, traffic.RelayerStopTimeout)
		if err := s.Relayer.Stop(stopCtx); err != nil {
			errs = append(errs, fmt.Errorf("stop relayer: %w", err))
		}
		cancel()
		s.Relayer = nil
	}
	if s.Env != nil {
		closeCtx, cancel := context.WithTimeout(ctx, closeTimeout)
		if err := s.Env.Close(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("close environment: %w", err))
		}
		cancel()
		s.Env = nil
	}
	if s.dir != "" {
		if err := os.RemoveAll(s.dir); err != nil {
			errs = append(errs, fmt.Errorf("remove deployment directory: %w", err))
		}
		s.dir = ""
	}
	if len(errs) == 0 {
		slog.InfoContext(ctx, "teardown complete")
	}
	return errors.Join(errs...)
}

func since(start time.Time) string {
	return time.Since(start).Round(time.Millisecond).String()
}
