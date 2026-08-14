// SPDX-License-Identifier: Apache-2.0

// Package livevalidate exercises every network-dependent check a relayer
// config implies, without starting a runnable process.
package livevalidate

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
)

// Validate exercises network-dependent config validation:
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

	return checkAttestorQuorum(ctx, cfg, clientSet)
}
