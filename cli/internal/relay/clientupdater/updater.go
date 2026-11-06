// SPDX-License-Identifier: Apache-2.0

// Package clientupdater keeps light clients from expiring by updating them on
// a fixed interval, independent of packet relay: a client that sees no
// packets for longer than its trusting period would otherwise expire.
package clientupdater

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
)

// TxSubmitters resolves the tx submitter for a (chain, signer) pair.
type TxSubmitters interface {
	Get(chainID, signerAlias string) (txsubmitter.TxSubmitter, bool)
}

// Updater updates one light client every interval.
type Updater struct {
	clientID string
	interval time.Duration

	prover      prover.Prover
	txBuilder   txbuilder.TxBuilder
	txSubmitter txsubmitter.TxSubmitter

	logger *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New Updater constructor.
func New(
	chainID, clientID string,
	interval time.Duration,
	prover prover.Prover,
	txBuilder txbuilder.TxBuilder,
	txSubmitter txsubmitter.TxSubmitter,
	logger *slog.Logger,
) *Updater {
	return &Updater{
		clientID:    clientID,
		interval:    interval,
		prover:      prover,
		txBuilder:   txBuilder,
		txSubmitter: txSubmitter,
		logger:      logger.With("module", "clientupdater", "chainID", chainID, "clientID", clientID),
	}
}

// Start runs the first update right away, then one every interval, until Stop.
func (u *Updater) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel

	u.wg.Go(func() {
		ticker := time.NewTicker(u.interval)
		defer ticker.Stop()

		for {
			u.update(ctx)

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

// Stop cancels the update loop and blocks until it has exited.
func (u *Updater) Stop() {
	if u.cancel == nil {
		return
	}

	u.cancel()
	u.wg.Wait()
	u.cancel = nil
}

// update logs a failure rather than retrying it: the next tick retries, and
// the interval is meant to be well inside the client's trusting period.
func (u *Updater) update(ctx context.Context) {
	height, updated, err := processors.UpdateClient(
		ctx, u.logger, u.prover, u.txBuilder, u.txSubmitter, u.clientID,
	)

	switch {
	case ctx.Err() != nil:
	case errors.Is(err, txsubmitter.ErrBusy):
		u.logger.Warn("Client update postponed, tx submitter busy", "err", err)
	case err != nil:
		u.logger.Error("Client update failed", "err", err)
	case updated:
		u.logger.Info("Client updated", "height", height)
	default:
		u.logger.Debug("Client already trusts the latest provable height", "height", height)
	}
}

// Set the updaters for every client end with automatic updates enabled.
type Set []*Updater

// NewSetFromConfig builds an updater for each client end with autoUpdate
// enabled, submitting through the end's own signer on its chain.
func NewSetFromConfig(
	cfg config.Config,
	provers processors.Provers,
	txBuilders processors.TxBuilders,
	txSubmitters TxSubmitters,
	logger *slog.Logger,
) (Set, error) {
	var set Set

	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []config.ClientEnd{conn.ClientA, conn.ClientB} {
			if !end.AutoUpdate.IsEnabled() {
				continue
			}

			clientProver, ok := provers.Get(end.ChainID, end.ClientID)
			if !ok {
				return nil, errors.Errorf("no prover for client %q on chain %q", end.ClientID, end.ChainID)
			}

			txBuilder, ok := txBuilders.Get(end.ChainID)
			if !ok {
				return nil, errors.Errorf("no tx builder for chain %q", end.ChainID)
			}

			txSubmitter, ok := txSubmitters.Get(end.ChainID, end.Signer)
			if !ok {
				return nil, errors.Errorf("no tx submitter for chain %q and signer %q", end.ChainID, end.Signer)
			}

			set = append(set, New(
				end.ChainID, end.ClientID, *end.AutoUpdate.Interval,
				clientProver, txBuilder, txSubmitter, logger,
			))
		}
	}

	return set, nil
}

// Start starts every updater.
func (s Set) Start() {
	for _, u := range s {
		u.Start()
	}
}

// Stop stops every updater, blocking until each loop has exited.
func (s Set) Stop() {
	for _, u := range s {
		u.Stop()
	}
}
