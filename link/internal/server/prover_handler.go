// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/link/api/v2/prover"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/relay/prover/remote"
)

// ProverHandler serves the relayer's own provers over the ProverService
// contract. A custom light client replaces this handler with its own service;
// the relayer only ever speaks the wire contract.
type ProverHandler struct {
	logger *slog.Logger
	set    ProverSet
}

// ProverSet resolves a prover by the client a request names.
type ProverSet interface {
	Get(chainID, clientID string) (prover.Prover, bool)
}

var (
	_ proto.ProverServiceHandler = (*ProverHandler)(nil)
	_ Handler                    = (*ProverHandler)(nil)
)

func NewProverHandler(set ProverSet) *ProverHandler {
	return &ProverHandler{
		logger: slog.With("handler", "prover"),
		set:    set,
	}
}

func (h *ProverHandler) Register(opts ...connect.HandlerOption) (string, http.Handler) {
	return proto.NewProverServiceHandler(h, opts...)
}

func (h *ProverHandler) Name() string {
	return proto.ProverServiceName
}

// prover resolves the client a request names, so an unknown client is a
// caller error rather than a nil dereference.
func (h *ProverHandler) prover(client *proto.Client) (prover.Prover, error) {
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

func (h *ProverHandler) LatestProvableHeight(
	ctx context.Context,
	req *connect.Request[proto.LatestProvableHeightRequest],
) (*connect.Response[proto.LatestProvableHeightResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	height, timestamp, err := target.LatestProvableHeight(ctx)
	if err != nil {
		h.logger.Error("LatestProvableHeight", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.LatestProvableHeightResponse{
		Height:    height,
		Timestamp: timestamp.Unix(),
	}), nil
}

func (h *ProverHandler) StateProof(
	ctx context.Context,
	req *connect.Request[proto.StateProofRequest],
) (*connect.Response[proto.StateProofResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	proof, err := target.StateProof(ctx, req.Msg.GetHeight())
	if err != nil {
		h.logger.Error("StateProof", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.StateProofResponse{Proof: proof}), nil
}

func (h *ProverHandler) PacketProofs(
	ctx context.Context,
	req *connect.Request[proto.PacketProofsRequest],
) (*connect.Response[proto.PacketProofsResponse], error) {
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

	return connect.NewResponse(&proto.PacketProofsResponse{Proofs: proofs}), nil
}
