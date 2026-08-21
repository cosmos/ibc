// SPDX-License-Identifier: Apache-2.0

// Package remotepoc runs a ProverService a relayer can be pointed at. It shows
// how a custom light client is supported: the implementer runs a service, and
// the relayer speaks only the wire contract. The prover served here is the
// attestation one, so the contract can be exercised end to end without a
// second light-client implementation.
package remotepoc

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/relay/prover/attestation"
	"github.com/cosmos/ibc/link/internal/server"
	attestorservice "github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

const readHeaderTimeout = 5 * time.Second

// NewHandler serves provers over the ProverService contract.
func NewHandler(set *prover.Set) *http.Server {
	mux := http.NewServeMux()
	path, handler := server.NewProverHandler(set).Register()
	mux.Handle(path, handler)

	// gRPC needs HTTP/2, and the relayer dials over plain http.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Server{
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}

// NewAttestationHandler serves an attestation prover for every configured
// client end, built from a relayer config file. This is the standalone
// counterpart to the relayer: it holds the attestors and the chain clients,
// and the relayer holds neither.
func NewAttestationHandler(ctx context.Context, configPath string) (*http.Server, error) {
	cfg, err := config.LoadFromFile(configPath, true, true)
	if err != nil {
		return nil, errors.Wrap(err, "load config")
	}

	clients, err := chains.NewClientSetFromConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "build chain clients")
	}

	signers, err := signer.NewSetFromConfig(ctx, cfg.Signers)
	if err != nil {
		return nil, errors.Wrap(err, "build signers")
	}

	local, remoteAttestors, err := attestorservice.ResolveFromConfig(ctx, cfg.Attestors, clients, signers)
	if err != nil {
		return nil, errors.Wrap(err, "resolve attestors")
	}

	attestors := make([]attestorservice.Attestor, 0, len(local)+len(remoteAttestors))
	attestors = append(attestors, local...)
	attestors = append(attestors, remoteAttestors...)
	provers := make(map[string]prover.Prover)

	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []struct{ self, counterparty config.ClientEnd }{
			{conn.ClientA, conn.ClientB},
			{conn.ClientB, conn.ClientA},
		} {
			gen, resolveErr := attestation.ResolveGenerator(ctx, end.self, end.counterparty, clients, attestors)
			if resolveErr != nil {
				return nil, errors.Wrapf(resolveErr, "connection %q", conn.Alias)
			}

			provers[prover.Key(end.self.ChainID, end.self.ClientID)] = gen
		}
	}

	return NewHandler(prover.NewSet(provers)), nil
}
