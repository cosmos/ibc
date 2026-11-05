// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/cli/api/v2/attestor"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
)

// AttestorHandler handles attestor RPC requests.
type AttestorHandler struct {
	logger  *slog.Logger
	service AttestorService
}

// AttestorService defines attestor business logic.
type AttestorService interface {
	CheckReady(ctx context.Context) error
	LatestHeight(ctx context.Context, attestor string) (uint64, error)
	StateAttestation(ctx context.Context, attestor string, height uint64) (attestor.Attestation, error)
	PacketAttestation(
		ctx context.Context,
		attestor string,
		req attestor.PacketAttestationRequest,
	) (attestor.Attestation, error)
	Info(alias string) (attestor.Info, error)
}

// attestorCodes maps the attestor service's domain errors.
var attestorCodes = []errorCode{
	{attestor.ErrNotFound, connect.CodeNotFound},
	{attestor.ErrCommitmentNotFound, connect.CodeNotFound},
	{attestor.ErrInvalidInput, connect.CodeInvalidArgument},
	{attestor.ErrNotFinalized, connect.CodeFailedPrecondition},
	{attestor.ErrReceiptExists, connect.CodeFailedPrecondition},
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

func (h *AttestorHandler) CheckReady(ctx context.Context) error {
	return h.service.CheckReady(ctx)
}

func (h *AttestorHandler) LatestHeight(
	ctx context.Context,
	req *connect.Request[proto.LatestHeightRequest],
) (*connect.Response[proto.LatestHeightResponse], error) {
	height, err := h.service.LatestHeight(ctx, req.Msg.Attestor)
	if err != nil {
		return nil, connectError(h.logger, "LatestHeight", err, attestorCodes)
	}

	return connect.NewResponse(&proto.LatestHeightResponse{Height: height}), nil
}

func (h *AttestorHandler) StateAttestation(
	ctx context.Context,
	req *connect.Request[proto.StateAttestationRequest],
) (*connect.Response[proto.StateAttestationResponse], error) {
	attestation, err := h.service.StateAttestation(ctx, req.Msg.Attestor, req.Msg.Height)
	if err != nil {
		return nil, connectError(h.logger, "StateAttestation", err, attestorCodes)
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
	if err != nil {
		return nil, connectError(h.logger, "PacketAttestation", err, attestorCodes)
	}

	return connect.NewResponse(&proto.PacketAttestationResponse{
		Attestation: attestationToProto(attestation),
	}), nil
}

func (h *AttestorHandler) Info(
	_ context.Context,
	req *connect.Request[proto.InfoRequest],
) (*connect.Response[proto.InfoResponse], error) {
	info, err := h.service.Info(req.Msg.Attestor)
	if err != nil {
		return nil, connectError(h.logger, "Info", err, attestorCodes)
	}

	return connect.NewResponse(&proto.InfoResponse{
		ChainId: info.ChainID,
		Address: info.Address,
	}), nil
}

func attestationToProto(a attestor.Attestation) *proto.Attestation {
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

func packetAttestationRequestFromProto(req *proto.PacketAttestationRequest) (attestor.PacketAttestationRequest, error) {
	if len(req.Packets) == 0 {
		return attestor.PacketAttestationRequest{}, errors.New("packets must be provided")
	}

	ct, err := attestor.CommitmentTypeFromProto(req.CommitmentType)
	if err != nil {
		return attestor.PacketAttestationRequest{}, err
	}

	return attestor.PacketAttestationRequest{
		Height:         req.Height,
		Packets:        req.Packets,
		CommitmentType: ct,
	}, nil
}
