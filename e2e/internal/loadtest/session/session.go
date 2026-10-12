// SPDX-License-Identifier: Apache-2.0

// Package session starts and tears down the environment a load scenario runs
// against: two managed EVM Chains joined by a mesh, attested by default, the
// traffic applications, and optionally an auto-relaying relayer.
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

// Options shape the environment.
type Options struct {
	Provider environment.EVMProvider
	// Mesh joins the Chains; nil is traffic.AttestedMesh.
	Mesh func(chains ...environment.ChainSpec) (environment.Spec, environment.Runtime)
	// Observability exports OTel telemetry from every managed IBC CLI process
	// (attestors and the relayer) to the local LGTM stack.
	Observability bool
	// Routes the applications are deployed for and the relayer serves. With
	// none, nothing is deployed.
	Routes []traffic.Route
	// ConfigureRelayer adjusts the generated relayer configuration.
	ConfigureRelayer func(*ibccli.RelayerConfig)
	// StartRelayer configures and starts a relayer for Routes. Without it
	// only the applications are deployed.
	StartRelayer bool
}

// Session holds every resource a scenario uses. Close releases them.
type Session struct {
	Provider environment.EVMProvider
	Env      *environment.Environment
	// Deployment is nil when Options.Routes is empty.
	Deployment *traffic.Deployment
	// Relayer is nil unless Options.StartRelayer.
	Relayer *ibccli.Relayer
	// Deployer deployed the applications and holds the minted token supply.
	Deployer traffic.Signer

	dir string
}

// Start realizes the environment, deploys the applications for opts.Routes,
// and configures and starts the relayer if asked. On error it returns the
// session built so far, which the caller closes or keeps for inspection; the
// session is nil only if nothing was acquired.
func Start(ctx context.Context, opts Options) (*Session, error) {
	chains, err := environment.ManagedEVMChains(opts.Provider, traffic.ChainA, traffic.ChainB)
	if err != nil {
		return nil, err
	}
	mesh := opts.Mesh
	if mesh == nil {
		mesh = traffic.AttestedMesh
	}
	spec, runtime := mesh(chains...)
	runtime.Observability = opts.Observability

	started := time.Now()
	slog.InfoContext(ctx, "starting environment", "provider", opts.Provider, "observability", opts.Observability)
	// A failed environment start releases what it acquired.
	env, err := environment.Start(ctx, spec, runtime)
	if err != nil {
		return nil, fmt.Errorf("start environment: %w", err)
	}
	s := &Session{Provider: opts.Provider, Env: env}
	slog.InfoContext(ctx, "environment started", "run_id", s.Env.RunID(), "elapsed", since(started))

	s.Deployer, err = traffic.NewSigner()
	if err != nil {
		return s, err
	}
	if len(opts.Routes) == 0 {
		return s, nil
	}

	started = time.Now()
	if !opts.StartRelayer {
		s.Deployment, err = traffic.DeployApps(ctx, s.Env, s.Deployer, opts.Routes...)
		if err != nil {
			return s, fmt.Errorf("deploy: %w", err)
		}
		slog.InfoContext(ctx, "applications deployed", "elapsed", since(started))
		return s, nil
	}
	relayerSigner, err := traffic.NewSigner()
	if err != nil {
		return s, err
	}
	if s.dir, err = os.MkdirTemp("", "ibc-loadtest-"); err != nil {
		return s, fmt.Errorf("create deployment directory: %w", err)
	}
	driver, deployment, err := traffic.Deploy(
		ctx, s.Env, s.dir, s.Deployer, relayerSigner, opts.ConfigureRelayer, opts.Routes...,
	)
	if err != nil {
		return s, fmt.Errorf("deploy: %w", err)
	}
	s.Deployment = deployment
	slog.InfoContext(ctx, "applications deployed", "dir", s.dir, "elapsed", since(started))

	s.Relayer, err = traffic.StartRelayer(ctx, driver, s.Env)
	if err != nil {
		return s, fmt.Errorf("start relayer: %w", err)
	}
	slog.InfoContext(ctx, "relayer started", "http", s.Relayer.Ready().HTTP)
	return s, nil
}

// Keep leaves the session's resources running for inspection and logs how
// to find them.
func (s *Session) Keep(ctx context.Context) {
	var kept []any
	if s.Env != nil {
		kept = append(kept, "run_id", s.Env.RunID())
	}
	if s.dir != "" {
		kept = append(kept, "dir", s.dir)
	}
	slog.WarnContext(ctx, "kept the environment for inspection; run `make -C e2e clean` when done", kept...)
}

// Close stops the relayer, closes the Environment, and removes the
// deployment directory. It runs on a context detached from ctx's
// cancellation so an interrupted run still tears down.
func (s *Session) Close(ctx context.Context) error {
	ctx = context.WithoutCancel(ctx)
	var errs []error
	if s.Relayer != nil {
		stopCtx, cancel := context.WithTimeout(ctx, traffic.StopTimeout)
		if err := s.Relayer.Stop(stopCtx); err != nil {
			errs = append(errs, fmt.Errorf("stop relayer: %w", err))
		}
		cancel()
		s.Relayer = nil
	}
	if s.Env != nil {
		closeCtx, cancel := context.WithTimeout(ctx, environment.CloseTimeout)
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
