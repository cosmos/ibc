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
	// or has been pending past expiry and should be resubmitted.
	ShouldRetry(ctx context.Context, txHash string, expiry time.Duration, sentAt time.Time) (bool, error)
}

var _ TxManager = (*evm.TxManager)(nil)

// TxManagerSet holds the submitter for every chain relayed by the configured
// routes.
type TxManagerSet struct {
	txManagers map[string]TxManager
}

func NewTxManagerSet(txManagers map[string]TxManager) *TxManagerSet {
	if txManagers == nil {
		txManagers = make(map[string]TxManager)
	}

	return &TxManagerSet{txManagers: txManagers}
}

func (s *TxManagerSet) Get(chainID string) (TxManager, bool) {
	txManager, ok := s.txManagers[chainID]
	return txManager, ok
}

// NewFromConfig builds one tx manager per chain relayed by the configured
// routes. Each route names the signer for its source and destination chains;
// a chain always resolves to a single signer (enforced by config validation).
func NewFromConfig(cfg config.Config, signers *signer.Set) (*TxManagerSet, error) {
	aliases, err := config.RelayerChainSigners(cfg)
	if err != nil {
		return nil, err
	}

	txManagers := make(map[string]TxManager, len(aliases))

	for chainID, alias := range aliases {
		chain, ok := cfg.Chain(chainID)
		if !ok || chain.Type() != config.ChainTypeEVM {
			return nil, errors.Errorf("chain %q is not a configured evm chain", chainID)
		}

		chainSigner, ok := signers.Get(alias)
		if !ok {
			return nil, errors.Errorf("unknown signer %q for chain %q", alias, chainID)
		}

		opts := evm.ChainOptions{TxSubmissionDelay: evm.DefaultTxSubmissionDelay}
		if override := cfg.Relayer.ChainOverride(chainID); override != nil {
			if override.TxSubmissionDelay != nil {
				opts.TxSubmissionDelay = *override.TxSubmissionDelay
			}
			if override.EVM != nil {
				opts.GasFeeCapMultiplier = override.EVM.GasFeeCapMultiplier
				opts.GasTipCapMultiplier = override.EVM.GasTipCapMultiplier
			}
		}

		txManager, err := evm.NewFromRPC(chainID, chain.EVM.RPC, chainSigner, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "creating tx manager for chain %q", chainID)
		}

		txManagers[chainID] = txManager
	}

	return NewTxManagerSet(txManagers), nil
}
