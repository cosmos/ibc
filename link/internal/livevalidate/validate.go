// Package livevalidate exercises every network-dependent check a relayer
// config implies, without starting a runnable process.
package livevalidate

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/store"
)

// Validate exercises every network-dependent check a config implies:
// database reachability, on-chain counterparty registration for each
// connection, and attestor quorum satisfaction.
func Validate(ctx context.Context, cfg config.Config) error {
	if err := store.ValidateConfigLive(cfg); err != nil {
		return errors.Wrap(err, "db")
	}

	clientSet, err := chains.NewClientSetFromConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "chains")
	}

	if err = validateConnectionsLive(ctx, cfg, clientSet); err != nil {
		return errors.Wrap(err, "connections")
	}

	var signers *signer.Set
	if len(cfg.Attestors.Locals()) > 0 {
		signers, err = signer.NewSetFromConfig(ctx, cfg.Signers)
		if err != nil {
			return errors.Wrap(err, "signers")
		}
	}

	local, remote, err := attestor.ResolveFromConfig(ctx, cfg.Attestors, clientSet, signers)
	if err != nil {
		return errors.Wrap(err, "attestors")
	}

	if err := checkAttestorQuorum(ctx, cfg, clientSet, append(local, remote...)); err != nil {
		return errors.Wrap(err, "attestor quorum")
	}

	return nil
}
