// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/internal/service/relayer"
	"github.com/cosmos/ibc/link/internal/store"
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
	Packets(
		ctx context.Context,
		filter relayer.PacketFilter,
		page store.Page,
	) ([]relayer.PacketStatus, uint64, error)
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

	return connect.NewResponse(&proto.StatusResponse{
		PacketStatuses: packetStatusesToProto(statuses),
	}), nil
}

// Packets lists the packets the relayer knows about, filtered and paged.
func (h *RelayerHandler) Packets(
	ctx context.Context,
	req *connect.Request[proto.PacketsRequest],
) (*connect.Response[proto.PacketsResponse], error) {
	filter := packetFilterFromProto(req.Msg.GetFilter())

	h.logger.Info("Packets", "limit", req.Msg.GetLimit(), "offset", req.Msg.GetOffset())

	statuses, total, err := h.srv.Packets(ctx, filter, store.Page{
		Limit:  int64(req.Msg.GetLimit()),
		Offset: int64(req.Msg.GetOffset()),
	})

	switch {
	case errors.Is(err, relayer.ErrInvalidInput):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("Packets", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.PacketsResponse{
		Packets: packetStatusesToProto(statuses),
		Total:   total,
	}), nil
}

// packetFilterFromProto lowers the wire filter. An absent filter, or an absent
// field within it, means no constraint on that column.
func packetFilterFromProto(filter *proto.PacketFilter) relayer.PacketFilter {
	if filter == nil {
		return relayer.PacketFilter{}
	}

	out := relayer.PacketFilter{
		SourceChainID:       filter.SourceChainId,
		DestinationChainID:  filter.DestinationChainId,
		SourceClientID:      filter.SourceClientId,
		DestinationClientID: filter.DestinationClientId,
		SourceTxHash:        filter.SourceTxHash,
		SequenceNumber:      filter.SequenceNumber,
	}

	if filter.State != nil {
		state := packetStateFromProto(*filter.State)
		out.State = &state
	}

	if filter.CreatedFrom != nil {
		from := time.Unix(*filter.CreatedFrom, 0).UTC()
		out.CreatedFrom = &from
	}

	if filter.CreatedTo != nil {
		to := time.Unix(*filter.CreatedTo, 0).UTC()
		out.CreatedTo = &to
	}

	return out
}

func packetStatusesToProto(statuses []relayer.PacketStatus) []*proto.PacketStatus {
	out := make([]*proto.PacketStatus, len(statuses))
	for i, status := range statuses {
		out[i] = &proto.PacketStatus{
			State:          packetStateToProto(status.State),
			SequenceNumber: status.SequenceNumber,
			SourceClientId: status.SourceClientID,
			SendTx:         txInfoToProto(&status.SendTx),
			RecvTx:         txInfoToProto(status.RecvTx),
			AckTx:          txInfoToProto(status.AckTx),
			TimeoutTx:      txInfoToProto(status.TimeoutTx),
			CreatedAt:      status.CreatedAt.Unix(),
			UpdatedAt:      status.UpdatedAt.Unix(),
		}
	}

	return out
}

// packetStateFromProto is the inverse of packetStateToProto. An unrecognized
// or unspecified state maps to StateUnspecified, which matches no relay status
// and so returns an empty listing rather than silently ignoring the filter.
func packetStateFromProto(state proto.PacketState) relayer.PacketState {
	switch state {
	case proto.PacketState_PACKET_STATE_NOT_SELECTED:
		return relayer.StateNotSelected
	case proto.PacketState_PACKET_STATE_PENDING:
		return relayer.StatePending
	case proto.PacketState_PACKET_STATE_SUCCEEDED:
		return relayer.StateSucceeded
	case proto.PacketState_PACKET_STATE_TIMED_OUT:
		return relayer.StateTimedOut
	case proto.PacketState_PACKET_STATE_REJECTED:
		return relayer.StateRejected
	case proto.PacketState_PACKET_STATE_RELAY_FAILED:
		return relayer.StateRelayFailed
	default:
		return relayer.StateUnspecified
	}
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
