// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/internal/service/relayer"
)

// RelayerHandler handles relayer RPC requests.
type RelayerHandler struct {
	logger *slog.Logger
	srv    RelayerService
}

// RelayerService defines relayer business logic.
type RelayerService interface {
	Relay(ctx context.Context, request relayer.RelayRequest) error
	Status(ctx context.Context, chainID string, txHash string) ([]relayer.PacketStatus, error)
}

var (
	_ proto.RelayerApiServiceHandler = (*RelayerHandler)(nil)
	_ Handler                        = (*RelayerHandler)(nil)
)

func NewRelayerHandler(srv RelayerService) *RelayerHandler {
	return &RelayerHandler{
		logger: slog.With("handler", "relayer"),
		srv:    srv,
	}
}

func (h *RelayerHandler) Register(opts ...connect.HandlerOption) (string, http.Handler) {
	return proto.NewRelayerApiServiceHandler(h, opts...)
}

func (h *RelayerHandler) Name() string {
	return proto.RelayerApiServiceName
}

func (h *RelayerHandler) Relay(
	ctx context.Context,
	req *connect.Request[proto.RelayRequest],
) (*connect.Response[proto.RelayResponse], error) {
	h.logger.Info("Relay", "sourceChainID", req.Msg.SourceChainId, "txHash", req.Msg.TxHash)

	request := relayer.RelayRequest{ChainID: req.Msg.SourceChainId, TxHash: req.Msg.TxHash}
	switch selection := req.Msg.Selection.(type) {
	case *proto.RelayRequest_AllPackets:
		request.Selection = relayer.SelectionAll
	case *proto.RelayRequest_SelectedPackets:
		request.Selection = relayer.SelectionExplicit
		for _, packet := range selection.SelectedPackets.GetPackets() {
			request.Packets = append(request.Packets, relayer.PacketSelector{
				SourceClientID: packet.GetSourceClientId(),
				SequenceNumber: packet.GetSequenceNumber(),
			})
		}
	}

	err := h.srv.Relay(ctx, request)
	switch {
	case errors.Is(err, relayer.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, relayer.ErrFailedPrecondition):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, relayer.ErrNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("Relay", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.RelayResponse{}), nil
}

func (h *RelayerHandler) Status(
	ctx context.Context,
	req *connect.Request[proto.StatusRequest],
) (*connect.Response[proto.StatusResponse], error) {
	h.logger.Info("Status", "sourceChainID", req.Msg.SourceChainId, "txHash", req.Msg.TxHash)

	statuses, err := h.srv.Status(ctx, req.Msg.SourceChainId, req.Msg.TxHash)
	switch {
	case errors.Is(err, relayer.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, relayer.ErrNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("Status", "err", err)
		return nil, errInternal
	}

	packetStatuses := make([]*proto.PacketStatus, len(statuses))
	for i, status := range statuses {
		packetStatuses[i] = &proto.PacketStatus{
			State:          packetStateToProto(status.State),
			SequenceNumber: status.SequenceNumber,
			SourceClientId: status.SourceClientID,
			SendTx:         txInfoToProto(&status.SendTx),
			RecvTx:         txInfoToProto(status.RecvTx),
			AckTx:          txInfoToProto(status.AckTx),
			TimeoutTx:      txInfoToProto(status.TimeoutTx),
		}
	}

	return connect.NewResponse(&proto.StatusResponse{PacketStatuses: packetStatuses}), nil
}

func packetStateToProto(state relayer.PacketState) proto.PacketState {
	switch state {
	case relayer.StateNotSelected:
		return proto.PacketState_PACKET_STATE_NOT_SELECTED
	case relayer.StatePending:
		return proto.PacketState_PACKET_STATE_PENDING
	case relayer.StateSucceeded:
		return proto.PacketState_PACKET_STATE_SUCCEEDED
	case relayer.StateTimedOut:
		return proto.PacketState_PACKET_STATE_TIMED_OUT
	case relayer.StateRejected:
		return proto.PacketState_PACKET_STATE_REJECTED
	case relayer.StateRelayFailed:
		return proto.PacketState_PACKET_STATE_RELAY_FAILED
	default:
		return proto.PacketState_PACKET_STATE_UNSPECIFIED
	}
}

func txInfoToProto(info *relayer.TxInfo) *proto.TransactionInfo {
	if info == nil {
		return nil
	}

	return &proto.TransactionInfo{TxHash: info.TxHash, ChainId: info.ChainID}
}
