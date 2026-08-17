// SPDX-License-Identifier: Apache-2.0

// Package app is the public entrypoint for running an IBC Link relayer.
//
// It exists so that a custom light client can actually be used: implement
// proofgen.ProofGenerator and proofgen.Factory, register the factory under a
// client type name, and pass the registry here. The relayer's built-in client
// types are registered automatically, so a caller's registry only carries its
// own additions.
//
//	reg := proofgen.NewRegistry()
//	_ = reg.Register("myclient", myclient.Factory{})
//
//	relayer, err := app.New(ctx, app.Options{ConfigPath: path, Registry: reg})
//	addr, err := relayer.Start(ctx)
//	defer relayer.Stop(ctx)
//
// Everything the relayer is built from stays internal; this package deliberately
// exposes only what is needed to start and stop one.
package app

import (
	"context"
	"log/slog"
	"net"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/bootstrap"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/proofgen"
)

// Options configure a relayer.
type Options struct {
	// ConfigPath is the path to the relayer's YAML config. A path rather than
	// a parsed struct, so the config schema stays an internal detail.
	ConfigPath string

	// Registry holds light client types beyond the built-ins. May be nil.
	// Built-in types are registered regardless and may not be shadowed.
	Registry *proofgen.Registry

	// Logger defaults to slog.Default().
	Logger *slog.Logger

	// SkipMigrations starts without applying pending database migrations.
	SkipMigrations bool

	// AllowUnknownConfigFields relaxes the strict config decode. Off by
	// default: a misspelled key should fail loudly.
	AllowUnknownConfigFields bool
}

// Relayer is a built, not yet running, relayer process.
type Relayer struct {
	services *bootstrap.Services
	cfg      config.Config
	logger   *slog.Logger
	opts     Options
}

// New loads config, resolves every dependency, and validates that each
// configured client end names a registered client type. It does not start
// anything.
func New(_ context.Context, opts Options) (*Relayer, error) {
	if opts.ConfigPath == "" {
		return nil, errors.New("ConfigPath required")
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	cfg, err := config.LoadFromFile(opts.ConfigPath, true, !opts.AllowUnknownConfigFields)
	if err != nil {
		return nil, errors.Wrapf(err, "loading config %s", opts.ConfigPath)
	}

	services, err := bootstrap.BuildRelayer(cfg, opts.Registry)
	if err != nil {
		return nil, err
	}

	return &Relayer{services: services, cfg: cfg, logger: logger, opts: opts}, nil
}

// Start applies migrations, starts the API server, and starts the relay
// dispatch loop. It returns the address the server is listening on. Start does
// not block; call Stop to shut down.
func (r *Relayer) Start(_ context.Context) (net.Addr, error) {
	if err := r.migrate(); err != nil {
		return nil, err
	}

	address, err := r.services.Server.Start()
	if err != nil {
		return nil, errors.Wrap(err, "starting relayer server")
	}

	if err := r.services.RelayerService.Start(); err != nil {
		if stopErr := r.services.Server.Stop(); stopErr != nil {
			r.logger.Error("Stopping server after failed dispatch start", "err", stopErr)
		}

		return nil, errors.Wrap(err, "starting relayer dispatch loop")
	}

	return address, nil
}

// Stop shuts the relayer down in the reverse of start order: dispatch loop,
// then server, then database. Every step is attempted even if an earlier one
// fails, and the first error is returned.
func (r *Relayer) Stop(_ context.Context) error {
	var firstErr error

	record := func(step string, err error) {
		if err == nil {
			return
		}

		r.logger.Error("Stopping relayer", "step", step, "err", err)

		if firstErr == nil {
			firstErr = errors.Wrap(err, step)
		}
	}

	record("relayer service", r.services.RelayerService.Stop())
	record("server", r.services.Server.Stop())
	record("store", r.services.Store.Close())

	return firstErr
}

// ChainIDs lists the chains this relayer is configured against, in config
// order. Callers use it to report readiness.
func (r *Relayer) ChainIDs() []string {
	chains := make([]string, 0, len(r.cfg.Chains))
	for _, chain := range r.cfg.Chains {
		chains = append(chains, chain.ChainID)
	}

	return chains
}

func (r *Relayer) migrate() error {
	if r.opts.SkipMigrations {
		r.logger.Info("Skipping database migrations")

		return nil
	}

	applied, err := r.services.Store.MigrateUp()
	if err != nil {
		return errors.Wrap(err, "migrating database")
	}

	if applied == 0 {
		r.logger.Info("No migrations to apply")
	} else {
		r.logger.Info("Migrated database", "migrations_applied", applied)
	}

	return nil
}
