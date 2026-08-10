// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/link/api/v2/attestor"
	attestordomain "github.com/cosmos/ibc/link/attestor"
	attestorsvc "github.com/cosmos/ibc/link/internal/service/attestor"
)

// AttestorHandler handles attestor RPC requests.
type AttestorHandler struct {
	logger  *slog.Logger
	service AttestorService
}

// AttestorService defines attestor business logic.
type AttestorService interface {
	LatestHeight(ctx context.Context, attestor string) (uint64, error)
	StateAttestation(ctx context.Context, attestor string, height uint64) (attestordomain.Attestation, error)
	PacketAttestation(
		ctx context.Context,
		attestor string,
		req attestordomain.PacketAttestationRequest,
	) (attestordomain.Attestation, error)
}

var (
	_ proto.AttestationServiceHandler = (*AttestorHandler)(nil)
	_ Handler                         = (*AttestorHandler)(nil)
)

func NewAttestorHandler(srv AttestorService) *AttestorHandler {
	return &AttestorHandler{
		logger:  slog.With("handler", "attestor"),
		service: srv,
	}
}

func (h *AttestorHandler) Register(opts ...connect.HandlerOption) (string, http.Handler) {
	return proto.NewAttestationServiceHandler(h, opts...)
}

func (h *AttestorHandler) Name() string {
	return proto.AttestationServiceName
}

func (h *AttestorHandler) LatestHeight(
	ctx context.Context,
	req *connect.Request[proto.LatestHeightRequest],
) (*connect.Response[proto.LatestHeightResponse], error) {
	height, err := h.service.LatestHeight(ctx, req.Msg.Attestor)
	switch {
	case errors.Is(err, attestorsvc.ErrNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, attestordomain.ErrNotFinalized):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("LatestAttestableHeight", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.LatestHeightResponse{Height: height}), nil
}

func (h *AttestorHandler) StateAttestation(
	ctx context.Context,
	req *connect.Request[proto.StateAttestationRequest],
) (*connect.Response[proto.StateAttestationResponse], error) {
	attestation, err := h.service.StateAttestation(ctx, req.Msg.Attestor, req.Msg.Height)
	switch {
	case errors.Is(err, attestorsvc.ErrNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, attestordomain.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, attestordomain.ErrNotFinalized):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("StateAttestation", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.StateAttestationResponse{
		Attestation: attestationToProto(attestation),
	}), nil
}

func (h *AttestorHandler) PacketAttestation(
	ctx context.Context,
	req *connect.Request[proto.PacketAttestationRequest],
) (*connect.Response[proto.PacketAttestationResponse], error) {
	packetReq, err := packetAttestationRequestFromProto(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	attestation, err := h.service.PacketAttestation(ctx, req.Msg.Attestor, packetReq)
	switch {
	case errors.Is(err, attestorsvc.ErrNotFound), errors.Is(err, attestordomain.ErrCommitmentNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, attestordomain.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, attestordomain.ErrNotFinalized), errors.Is(err, attestordomain.ErrReceiptExists):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("PacketAttestation", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.PacketAttestationResponse{
		Attestation: attestationToProto(attestation),
	}), nil
}

func attestationToProto(a attestordomain.Attestation) *proto.Attestation {
	var timestamp *uint64
	if a.Timestamp != nil {
		asUnix := uint64(a.Timestamp.Unix())
		timestamp = &asUnix
	}

	return &proto.Attestation{
		Height:       a.Height,
		Timestamp:    timestamp,
		AttestedData: a.AttestedData,
		Signature:    a.Signature,
	}
}

func packetAttestationRequestFromProto(
	req *proto.PacketAttestationRequest,
) (attestordomain.PacketAttestationRequest, error) {
	if len(req.Packets) == 0 {
		return attestordomain.PacketAttestationRequest{}, errors.New("packets must be provided")
	}

	ct, err := attestorsvc.CommitmentTypeFromProto(req.CommitmentType)
	if err != nil {
		return attestordomain.PacketAttestationRequest{}, err
	}

	return attestordomain.PacketAttestationRequest{
		Height:         req.Height,
		Packets:        req.Packets,
		CommitmentType: ct,
	}, nil
}
