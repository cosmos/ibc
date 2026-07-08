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
