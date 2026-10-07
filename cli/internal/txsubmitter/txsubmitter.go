// SPDX-License-Identifier: Apache-2.0

// Package txsubmitter signs and broadcasts transactions for the relayer.
package txsubmitter

import (
	"context"
	"math/big"

	"github.com/cockroachdb/errors"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// TxSubmitter signs and broadcasts transactions on a single chain;
// implementations own nonce selection and gas pricing.
type TxSubmitter interface {
	Start(ctx context.Context) error
	Stop() error

	// Submit prepares and journals synchronously. Cancellation cannot revoke journaled work.
	Submit(ctx context.Context, intent v2.TxIntent) error

	// HasPending reports unresolved journaled work, not packet completion.
	HasPending(ctx context.Context, operation v2.OperationMetadata) (bool, error)
}

var _ TxSubmitter = (*evm.TxSubmitter)(nil)

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
func NewFromConfig(cfg config.Config, signers *signer.Set, db store.Store) (*Set, error) {
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

		opts := evm.ChainOptions{TxSubmissionDelay: evm.DefaultTxSubmissionDelay}
		if override, ok := cfg.Relayer.ChainOverride(pair.ChainID); ok {
			if override.TxSubmissionDelay != nil {
				opts.TxSubmissionDelay = *override.TxSubmissionDelay
			}
			if override.EVM != nil {
				e := override.EVM
				opts.Confirmations = e.Confirmations
				if e.MaxFeePerGas != nil {
					opts.MaxFeePerGas = new(big.Int).SetUint64(*e.MaxFeePerGas)
				}
				opts.GasFeeCapMultiplier = override.EVM.GasFeeCapMultiplier
				opts.GasTipCapMultiplier = override.EVM.GasTipCapMultiplier
			}
		}

		txSubmitter, err := evm.NewFromRPC(pair.ChainID, chain.EVM.RPC, chainSigner, db, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "creating tx submitter for chain %q", pair.ChainID)
		}

		txSubmitters[pair] = txSubmitter
	}

	return NewSet(txSubmitters), nil
}

// Start restores nonce ownership before the relay dispatcher starts accepting work.
func (s *Set) Start(ctx context.Context) error {
	for _, submitter := range s.txSubmitters {
		if err := submitter.Start(ctx); err != nil {
			_ = s.Stop()
			return err
		}
	}
	return nil
}

func (s *Set) Stop() error {
	for _, submitter := range s.txSubmitters {
		if err := submitter.Stop(); err != nil {
			return err
		}
	}
	return nil
}
