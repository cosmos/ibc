// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	pgen "github.com/cosmos/ibc/link/proofgen"
)

// ClientType is the config `type:` name this factory registers under.
const ClientType = string(config.ClientTypeAttestation)

var (
	_ pgen.Factory        = factory{}
	_ pgen.ProofGenerator = (*Generator)(nil)
)

// factory builds attestation proof generators. It closes over the attestor set
// and chain clients it needs, so the generic registry never carries
// attestation-specific arguments.
type factory struct {
	clientSet *chains.ClientSet
	attestors []attestor.Attestor
}

// NewFactory returns the built-in attestation client factory. attestors is
// every attestor this process can reach, local and remote; which of them back
// a given client is resolved per client end against on-chain state.
func NewFactory(clientSet *chains.ClientSet, attestors []attestor.Attestor) pgen.Factory {
	return factory{clientSet: clientSet, attestors: attestors}
}

// ValidateParams rejects params outright: the attestation client is configured
// through the top-level `attestors` block and its on-chain attestation set, not
// through per-client params.
func (f factory) ValidateParams(params *pgen.RawParams) error {
	if !params.IsEmpty() {
		return errors.New("attestation clients take no params; configure attestors under .attestors")
	}

	return nil
}

// New resolves the attestor quorum for self and builds its generator. Deps are
// unused: this factory already holds richer chain access than the public
// CounterpartyChain view exposes, because it must query self's on-chain
// attestation set.
func (f factory) New(
	ctx context.Context,
	_ pgen.Deps,
	self, counterparty pgen.ClientEnd,
) (pgen.ProofGenerator, error) {
	return ResolveGenerator(ctx, toConfigEnd(self), toConfigEnd(counterparty), f.clientSet, f.attestors)
}

func toConfigEnd(end pgen.ClientEnd) config.ClientEnd {
	return config.ClientEnd{
		ChainID:  end.ChainID,
		ClientID: end.ClientID,
		Type:     config.ClientType(end.Type),
	}
}
