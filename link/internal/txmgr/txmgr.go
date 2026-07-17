// Package txmgr signs and broadcasts transactions for the relayer.
package txmgr

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/txmgr/evm"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// TxManager signs and broadcasts transactions on a single chain;
// implementations own nonce selection and gas pricing.
type TxManager interface {
	Submit(ctx context.Context, intent v2.TxIntent) (*v2.Submission, error)

	// ShouldRetry reports whether a transaction submitted at sentAt is failed
	// or has been pending past the implementation's retry expiry and should be
	// resubmitted.
	ShouldRetry(ctx context.Context, txHash string, sentAt time.Time) (bool, error)
}

var _ TxManager = (*evm.TxManager)(nil)

// TxManagerSet holds one tx manager per (chain, signer) pair relayed by the
// configured routes. A chain may carry several signers when different clients
// on it are relayed by different routes.
type TxManagerSet struct {
	txManagers map[config.ChainSignerBinding]TxManager
}

func NewTxManagerSet(txManagers map[config.ChainSignerBinding]TxManager) *TxManagerSet {
	if txManagers == nil {
		txManagers = make(map[config.ChainSignerBinding]TxManager)
	}

	return &TxManagerSet{txManagers: txManagers}
}

func (s *TxManagerSet) Get(chainID, signerAlias string) (TxManager, bool) {
	txManager, ok := s.txManagers[config.ChainSignerBinding{ChainID: chainID, SignerAlias: signerAlias}]
	return txManager, ok
}

// NewFromConfig builds one tx manager per chain relayed by the configured
// routes. Each route names the signer for its source and destination chains;
// a chain always resolves to a single signer (enforced by config validation).
func NewFromConfig(cfg config.Config, signers *signer.Set) (*TxManagerSet, error) {
	bindings, err := config.RelayerChainSigners(cfg)
	if err != nil {
		return nil, err
	}

	txManagers := make(map[config.ChainSignerBinding]TxManager, len(bindings))

	for _, binding := range bindings {
		chain, ok := cfg.Chain(binding.ChainID)
		if !ok || chain.Type() != config.ChainTypeEVM {
			return nil, errors.Errorf("chain %q is not a configured evm chain", binding.ChainID)
		}

		chainSigner, ok := signers.Get(binding.SignerAlias)
		if !ok {
			return nil, errors.Errorf("unknown signer %q for chain %q", binding.SignerAlias, binding.ChainID)
		}

		opts := evm.ChainOptions{TxSubmissionDelay: evm.DefaultTxSubmissionDelay}
		if override := cfg.Relayer.ChainOverride(binding.ChainID); override != nil {
			if override.TxSubmissionDelay != nil {
				opts.TxSubmissionDelay = *override.TxSubmissionDelay
			}
			if override.EVM != nil {
				opts.GasFeeCapMultiplier = override.EVM.GasFeeCapMultiplier
				opts.GasTipCapMultiplier = override.EVM.GasTipCapMultiplier
			}
		}

		txManager, err := evm.NewFromRPC(binding.ChainID, chain.EVM.RPC, chainSigner, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "creating tx manager for chain %q", binding.ChainID)
		}

		txManagers[binding] = txManager
	}

	return NewTxManagerSet(txManagers), nil
}
