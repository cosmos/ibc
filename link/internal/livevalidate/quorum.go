// SPDX-License-Identifier: Apache-2.0

package livevalidate

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
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

	if _, err := prover.NewSetFromConfig(ctx, cfg, clientSet, attestors); err != nil {
		return errors.Wrap(err, "attestor quorum")
	}

	return nil
}
