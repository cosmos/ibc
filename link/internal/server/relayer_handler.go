package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/service/relayer"

	proto "github.com/cosmos/ibc/link/internal/types/v2/relayer"
)

// RelayerHandler handles relayer RPC requests.
type RelayerHandler struct {
	logger *slog.Logger
	srv    RelayerService
}

// RelayerService defines relayer business logic.
type RelayerService interface {
	Relay(ctx context.Context, chainID string, txHash string) error
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
	h.logger.Info("Relay called", "chainID", req.Msg.ChainId, "txHash", req.Msg.TxHash)

	err := h.srv.Relay(ctx, req.Msg.ChainId, req.Msg.TxHash)
	switch {
	case errors.Is(err, relayer.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("Relay", "error", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.RelayResponse{}), nil
}

func (h *RelayerHandler) Status(
	ctx context.Context,
	req *connect.Request[proto.StatusRequest],
) (*connect.Response[proto.StatusResponse], error) {
	h.logger.Info("Status called", "chainID", req.Msg.ChainId, "txHash", req.Msg.TxHash)

	statuses, err := h.srv.Status(ctx, req.Msg.ChainId, req.Msg.TxHash)
	switch {
	case errors.Is(err, relayer.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, relayer.ErrNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("Status", "error", err)
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

func packetStateToProto(state relayer.PacketState) proto.TransferState {
	switch state {
	case relayer.StatePending:
		return proto.TransferState_TRANSFER_STATE_PENDING
	case relayer.StateComplete:
		return proto.TransferState_TRANSFER_STATE_COMPLETE
	case relayer.StateFailed:
		return proto.TransferState_TRANSFER_STATE_FAILED
	default:
		return proto.TransferState_TRANSFER_STATE_UNKNOWN
	}
}

func txInfoToProto(info *relayer.TxInfo) *proto.TransactionInfo {
	if info == nil {
		return nil
	}

	return &proto.TransactionInfo{TxHash: info.TxHash, ChainId: info.ChainID}
}
