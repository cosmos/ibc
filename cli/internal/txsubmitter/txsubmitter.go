// SPDX-License-Identifier: Apache-2.0

// Package txsubmitter simulates, signs, and broadcasts transactions for the relayer.
package txsubmitter

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/txsubmitter/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// TxSubmitter simulates, signs, and broadcasts transactions on a single chain;
// implementations own nonce selection and gas pricing.
type TxSubmitter interface {
	// SimulateRelayTx returns decoded outcomes in execution order, including failures.
	SimulateRelayTx(ctx context.Context, tx v2.RelayTx) (v2.RelaySimulation, error)

	Submit(ctx context.Context, intent v2.TxIntent) (*v2.Submission, error)

	// ShouldRetry reports whether a transaction submitted at sentAt is failed
	// or has been pending past the implementation's retry expiry and should be
	// resubmitted.
	// A reverted receipt returns true with v2.ErrTxReverted; callers must still retry it.
	ShouldRetry(ctx context.Context, txHash string, sentAt time.Time) (bool, error)
}

var _ TxSubmitter = (*evm.TxSubmitter)(nil)

// ErrBusy indicates that a submission should be retried when capacity is available.
var ErrBusy = evm.ErrBusy

// Set holds one tx submitter per (chain, signer) pair relayed by
// the configured routes. A chain may carry several signers when different
// clients on it are relayed by different routes.
type Set struct {
	txSubmitters map[config.ChainSignerPair]TxSubmitter
}

func NewSet(txSubmitters map[config.ChainSignerPair]TxSubmitter) *Set {
	if txSubmitters == nil {
		txSubmitters = make(map[config.ChainSignerPair]TxSubmitter)
	}

	return &Set{txSubmitters: txSubmitters}
}

func (s *Set) Get(chainID, signerAlias string) (TxSubmitter, bool) {
	txSubmitter, ok := s.txSubmitters[config.ChainSignerPair{ChainID: chainID, SignerAlias: signerAlias}]
	return txSubmitter, ok
}

// NewFromConfig builds one tx submitter per (chain, signer) pair relayed by
// the configured routes. Routes naming the same pair share a tx submitter; a
// chain carries several when different clients on it are relayed with
// different signers.
func NewFromConfig(cfg config.Config, signers *signer.Set) (*Set, error) {
	pairs := config.RelayerChainSignerPairs(cfg)

	txSubmitters := make(map[config.ChainSignerPair]TxSubmitter, len(pairs))

	for _, pair := range pairs {
		chain, ok := cfg.Chain(pair.ChainID)
		if !ok || chain.Type() != config.ChainTypeEVM {
			return nil, errors.Errorf("chain %q is not a configured evm chain", pair.ChainID)
		}

		chainSigner, ok := signers.Get(pair.SignerAlias)
		if !ok {
			return nil, errors.Errorf("unknown signer %q for chain %q", pair.SignerAlias, pair.ChainID)
		}

		var opts evm.ChainOptions
		if override, ok := cfg.Relayer.ChainOverride(pair.ChainID); ok {
			if override.EVM != nil {
				e := override.EVM
				if e.PendingTxTimeout != nil {
					opts.PendingTxTimeout = *e.PendingTxTimeout
				}
			}
		}

		txSubmitter, err := evm.NewFromRPC(pair.ChainID, chain.EVM.RPC, chainSigner, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "creating tx submitter for chain %q", pair.ChainID)
		}

		txSubmitters[pair] = txSubmitter
	}

	return NewSet(txSubmitters), nil
}
