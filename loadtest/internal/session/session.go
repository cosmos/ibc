// SPDX-License-Identifier: Apache-2.0

// Package session starts and tears down the environment a load scenario runs
// against: two managed EVM Chains joined by an attested mesh, the traffic
// applications, and an auto-relaying relayer.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/ibccli"
	"github.com/cosmos/ibc/harness/traffic"
)

// closeTimeout bounds Environment teardown, which stops containers and
// managed processes; it matches the e2e suite's cleanup budget.
const closeTimeout = 30 * time.Second

// Provider selects the managed EVM node implementation.
type Provider string

const (
	ProviderAnvil Provider = "anvil"
	ProviderBesu  Provider = "besu"
)

// EVM chain ids, matching the e2e suite's managed Chains.
const (
	anvilChainIDBase = 31337
	besuChainIDBase  = 32337
)

// ParseProvider validates a --provider value.
func ParseProvider(value string) (Provider, error) {
	switch Provider(value) {
	case ProviderAnvil, ProviderBesu:
		return Provider(value), nil
	default:
		return "", fmt.Errorf("unknown provider %q: want anvil or besu", value)
	}
}

// Options shape the environment.
type Options struct {
	Provider Provider
	// Observability exports OTel telemetry from every managed IBC CLI process
	// (attestors and the relayer) to the local LGTM stack.
	Observability bool
	// KeepOnFailure keeps the deployment directory (relayer config, database,
	// and log) when the scenario fails, for post-mortems.
	KeepOnFailure bool
	// Routes the relayer serves. Empty means none.
	Routes []traffic.Route
	// ConfigureRelayer adjusts the generated relayer configuration.
	ConfigureRelayer func(*ibccli.RelayerConfig)
}

// Session holds every resource a scenario uses. Close releases them.
type Session struct {
	Env        *environment.Environment
	Deployment *traffic.Deployment
	Relayer    *ibccli.Relayer
	// Deployer deployed the applications and holds the minted token supply.
	Deployer traffic.Signer

	dir           string
	keepOnFailure bool
}

// Chains returns the two managed Chain declarations for provider.
func Chains(provider Provider) []environment.ChainSpec {
	ids := []environment.ChainID{traffic.ChainA, traffic.ChainB}
	chains := make([]environment.ChainSpec, len(ids))
	for i, id := range ids {
		switch provider {
		case ProviderBesu:
			chains[i] = environment.ManagedBesu{ID: id, EVMChainID: besuChainIDBase + uint64(i)}
		default:
			chains[i] = environment.ManagedAnvil{ID: id, EVMChainID: anvilChainIDBase + uint64(i)}
		}
	}
	return chains
}

// Start realizes the environment, deploys the applications and relayer
// configuration for opts.Routes, and starts the relayer. On error every
// resource acquired so far is released.
func Start(ctx context.Context, opts Options) (_ *Session, err error) {
	spec, runtime := traffic.AttestedMesh(Chains(opts.Provider)...)
	runtime.Observability = opts.Observability

	dir, err := os.MkdirTemp("", "ibc-loadtest-")
	if err != nil {
		return nil, fmt.Errorf("create deployment directory: %w", err)
	}
	s := &Session{dir: dir, keepOnFailure: opts.KeepOnFailure}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close(ctx, err))
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

	s.Relayer, err = traffic.StartRelayer(ctx, driver, s.Env)
	if err != nil {
		return nil, fmt.Errorf("start relayer: %w", err)
	}
	slog.InfoContext(ctx, "relayer started", "http", s.Relayer.Ready().HTTP)
	return s, nil
}

// Close stops the relayer, closes the Environment, and removes the
// deployment directory — unless failure is non-nil and the session keeps
// its directory on failure. It runs on a context detached from ctx's
// cancellation so an interrupted run still tears down.
func (s *Session) Close(ctx context.Context, failure error) error {
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
		if failure != nil && s.keepOnFailure {
			slog.WarnContext(ctx, "kept deployment directory for inspection", "dir", s.dir)
		} else if err := os.RemoveAll(s.dir); err != nil {
			errs = append(errs, fmt.Errorf("remove deployment directory: %w", err))
		}
		s.dir = ""
	}
	if len(errs) == 0 {
		slog.InfoContext(ctx, "teardown complete")
	}
	return errors.Join(errs...)
}

// Chain resolves one of the session's Chains.
func (s *Session) Chain(id environment.ChainID) (*environment.Chain, error) {
	return s.Env.Chain(id)
}

func since(start time.Time) string {
	return time.Since(start).Round(time.Millisecond).String()
}
