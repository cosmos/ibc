// SPDX-License-Identifier: Apache-2.0

// Package clienttypes assembles the proof generator registry every relayer
// entry point starts from.
//
// It exists so that the built-in light client types are registered identically
// everywhere — the relayer process, live config validation, and the public
// link/app entrypoint — without the generic proofgen package having to import
// any particular client implementation.
package clienttypes

import (
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	pgen "github.com/cosmos/ibc/link/proofgen"
)

// Builtins returns a registry holding the light client types shipped with the
// relayer, wired to the attestors and chain clients resolved from config.
//
// custom may be nil. When supplied, its factories are folded in; a custom type
// may not shadow a built-in name.
func Builtins(
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
	custom *pgen.Registry,
) (*pgen.Registry, error) {
	reg := pgen.NewRegistry()

	if err := reg.Register(attestation.ClientType, attestation.NewFactory(clientSet, attestors)); err != nil {
		return nil, errors.Wrap(err, "registering built-in client types")
	}

	if err := reg.Merge(custom); err != nil {
		return nil, errors.Wrap(err, "registering custom client types")
	}

	return reg, nil
}
