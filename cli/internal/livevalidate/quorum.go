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

// checkAttestorQuorum resolves every configured attestor (local and remote)
// and confirms every attestation-type client end of every configured
// connection can currently satisfy its attestor quorum against on-chain
// state.
func checkAttestorQuorum(ctx context.Context, cfg config.Config, clientSet *chains.ClientSet) error {
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

	// This resolves every client end's prover, which for an attestation client
	// is what checks the quorum. A remote client end resolves here too, so the
	// error is labeled for what failed rather than assuming a quorum problem.
	if _, err := prover.NewSetFromConfig(ctx, cfg, clientSet, attestors, slog.Default(), prover.SetOptions{RequireReachable: true}); err != nil {
		return errors.Wrap(err, "provers")
	}

	return nil
}
