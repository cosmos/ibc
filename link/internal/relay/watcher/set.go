// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"log/slog"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
)

// Set the watchers for every chain that sources an auto-relayed route.
type Set []*Watcher

// NewSetFromConfig builds a watcher for each chain with auto-relay enabled on
// at least one of its routes.
func NewSetFromConfig(
	cfg config.Config,
	clientSet *chains.ClientSet,
	storage PacketStore,
	logger *slog.Logger,
) (Set, error) {
	var set Set

	for _, chain := range cfg.Chains {
		connections := cfg.Relayer.AutoRelayConnections(chain.ChainID)
		if len(connections) == 0 {
			continue
		}

		client, ok := clientSet.Get(chain.ChainID)
		if !ok {
			return nil, errors.Errorf("no chain client for auto-relayed chain %q", chain.ChainID)
		}

		set = append(set, New(chain.ChainID, connections, client, storage, logger))
	}

	return set, nil
}

func (s Set) Start() error {
	for _, w := range s {
		if err := w.Start(); err != nil {
			_ = s.Stop() // best effort stop on startup error
			return errors.Wrapf(err, "starting watcher for chain %q", w.chainID)
		}
	}

	return nil
}

// Stop stops every watcher, blocking until each loop has exited.
func (s Set) Stop() error {
	for _, w := range s {
		if err := w.Stop(); err != nil {
			return errors.Wrapf(err, "stopping watcher for chain %q", w.chainID)
		}
	}

	return nil
}
