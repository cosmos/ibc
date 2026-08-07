// Package txsubmitter signs and broadcasts transactions for the relayer.
package txsubmitter

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/txsubmitter/evm"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// TxSubmitter signs and broadcasts transactions on a single chain;
// implementations own nonce selection and gas pricing.
type TxSubmitter interface {
	Submit(ctx context.Context, intent v2.TxIntent) (*v2.Submission, error)

	// ShouldRetry reports whether a transaction submitted at sentAt is failed
	// or has been pending past the implementation's retry expiry and should be
	// resubmitted.
	ShouldRetry(ctx context.Context, txHash string, sentAt time.Time) (bool, error)
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
func NewFromConfig(cfg config.Config, signers *signer.Set) (*Set, error) {
	pairs, err := config.RelayerChainSignerPairs(cfg)
	if err != nil {
		return nil, err
	}

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
				opts.GasFeeCapMultiplier = override.EVM.GasFeeCapMultiplier
				opts.GasTipCapMultiplier = override.EVM.GasTipCapMultiplier
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
