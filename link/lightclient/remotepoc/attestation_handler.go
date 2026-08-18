// SPDX-License-Identifier: Apache-2.0

package remotepoc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	attestorservice "github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/lightclient"
)

// NewHandler exposes generator through the proof-of-concept HTTP protocol.
func NewHandler(generator lightclient.ProofGenerator) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /proof", func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			_ = json.NewEncoder(w).Encode(response{Error: err.Error()})
			return
		}

		res := response{}
		var err error
		switch req.Operation {
		case "latest":
			res.Height, res.Timestamp, err = generator.LatestProvableHeight(r.Context())
		case "state":
			res.Proof, err = generator.StateProof(r.Context(), req.Height)
		case "packets":
			var proofs [][]byte
			proofs, err = generator.PacketProofs(r.Context(), req.Height, req.Kind, req.Packets)
			res.Proofs = proofs
		default:
			err = fmt.Errorf("unknown operation %q", req.Operation)
		}
		if err != nil {
			res = response{Error: err.Error()}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	return &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

// NewAttestationHandler constructs the configured attestation generator for
// one client and exposes it through the remote light-client HTTP protocol.
func NewAttestationHandler(ctx context.Context, configPath, chainID, clientID string) (*http.Server, error) {
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

	self, counterparty, ok := cfg.Relayer.ClientEnd(chainID, clientID)
	if !ok {
		return nil, errors.Errorf("client %q on chain %q is not configured", clientID, chainID)
	}
	generator, err := attestation.ResolveGenerator(
		ctx, self, counterparty, clients, append(local, remoteAttestors...),
	)
	if err != nil {
		return nil, errors.Wrap(err, "resolve attestation proof generator")
	}

	return NewHandler(generator), nil
}
