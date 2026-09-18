// SPDX-License-Identifier: Apache-2.0

package livevalidate

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/service/signer"
)

// checkProvers resolves every configured attestor (local and remote) and
// builds a prover for every client end of every configured connection, which
// confirms against on-chain state that attestation ends can satisfy their
// attestor quorum and that besu-qbft ends track the configured counterparty
// router with a trusted consensus state the relayer can rebuild.
func checkProvers(ctx context.Context, cfg config.Config, clientSet *chains.ClientSet) error {
	signers, err := signer.NewSetFromConfig(ctx, cfg.Signers)
	if err != nil {
		return errors.Wrap(err, "signers")
	}

	local, remote, err := attestor.ResolveFromConfig(ctx, cfg.Attestors, clientSet, signers)
	if err != nil {
		return errors.Wrap(err, "attestors")
	}

	attestors := make([]attestor.Attestor, 0, len(local)+len(remote))
	attestors = append(attestors, local...)
	attestors = append(attestors, remote...)

	if _, err := prover.NewSetFromConfig(ctx, cfg, clientSet, attestors, slog.Default()); err != nil {
		return errors.Wrap(err, "prover validation")
	}

	return nil
}
