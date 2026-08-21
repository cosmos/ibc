// SPDX-License-Identifier: Apache-2.0

// Package remotepoc runs a ProverService a relayer can be pointed at. It
// serves the attestation prover, so the contract can be exercised end to end
// without a second light-client implementation.
package remotepoc

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proverv2 "github.com/cosmos/ibc/link/api/v2/prover"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/relay/prover/attestation"
	"github.com/cosmos/ibc/link/internal/relay/prover/remote"
	attestorservice "github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

const readHeaderTimeout = 5 * time.Second

var errInternal = connect.NewError(connect.CodeInternal, errors.New("internal server error"))

// Handler answers ProverService requests from a set of provers. A custom light
// client replaces it with its own implementation of the same contract.
type Handler struct {
	logger *slog.Logger
	set    *prover.Set
}

var _ proverv2.ProverServiceHandler = (*Handler)(nil)

// NewServer serves provers over the ProverService contract.
func NewServer(set *prover.Set) *http.Server {
	mux := http.NewServeMux()
	path, handler := proverv2.NewProverServiceHandler(&Handler{
		logger: slog.With("module", "prover"),
		set:    set,
	})
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

// NewAttestationServer serves an attestation prover per configured client end,
// built from a relayer config. It holds the attestors and chain clients; the
// relayer holds neither.
func NewAttestationServer(ctx context.Context, configPath string) (*http.Server, error) {
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

	return NewServer(prover.NewSet(provers)), nil
}

// prover resolves the client a request names; an unknown one is a caller error.
func (h *Handler) prover(client *proverv2.Client) (prover.Prover, error) {
	chainID, clientID := client.GetChainId(), client.GetClientId()

	found, ok := h.set.Get(chainID, clientID)
	if !ok {
		return nil, connect.NewError(
			connect.CodeNotFound,
			errors.Errorf("no prover for client %q on chain %q", clientID, chainID),
		)
	}

	return found, nil
}

func (h *Handler) LatestProvableHeight(
	ctx context.Context,
	req *connect.Request[proverv2.LatestProvableHeightRequest],
) (*connect.Response[proverv2.LatestProvableHeightResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	height, timestamp, err := target.LatestProvableHeight(ctx)
	if err != nil {
		h.logger.Error("LatestProvableHeight", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proverv2.LatestProvableHeightResponse{
		Height:    height,
		Timestamp: timestamp.Unix(),
	}), nil
}

func (h *Handler) StateProof(
	ctx context.Context,
	req *connect.Request[proverv2.StateProofRequest],
) (*connect.Response[proverv2.StateProofResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	proof, err := target.StateProof(ctx, req.Msg.GetHeight())
	if err != nil {
		h.logger.Error("StateProof", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proverv2.StateProofResponse{Proof: proof}), nil
}

func (h *Handler) PacketProofs(
	ctx context.Context,
	req *connect.Request[proverv2.PacketProofsRequest],
) (*connect.Response[proverv2.PacketProofsResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	kind, err := remote.ProofKindFromProto(req.Msg.GetKind())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	proofs, err := target.PacketProofs(
		ctx, req.Msg.GetHeight(), kind, remote.PacketsFromProto(req.Msg.GetPackets()),
	)
	if err != nil {
		h.logger.Error("PacketProofs", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proverv2.PacketProofsResponse{Proofs: proofs}), nil
}
