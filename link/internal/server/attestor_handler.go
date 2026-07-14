package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/service/attestor"

	proto "github.com/cosmos/ibc/link/api/v2/attestor"
)

// AttestorHandler handles attestor RPC requests.
type AttestorHandler struct {
	logger *slog.Logger
	srv    AttestorService
}

// AttestorService defines attestor business logic.
type AttestorService interface {
	LatestAttestableHeight(ctx context.Context, attestor string) (uint64, error)
}

var (
	_ proto.AttestationServiceHandler = (*AttestorHandler)(nil)
	_ Handler                         = (*AttestorHandler)(nil)
)

func NewAttestorHandler(srv AttestorService) *AttestorHandler {
	return &AttestorHandler{
		logger: slog.With("handler", "attestor"),
		srv:    srv,
	}
}

func (h *AttestorHandler) Register(opts ...connect.HandlerOption) (string, http.Handler) {
	return proto.NewAttestationServiceHandler(h, opts...)
}

func (h *AttestorHandler) Name() string {
	return proto.AttestationServiceName
}

func (h *AttestorHandler) LatestAttestableHeight(
	ctx context.Context,
	req *connect.Request[proto.LatestAttestableHeightRequest],
) (*connect.Response[proto.LatestAttestableHeightResponse], error) {
	height, err := h.srv.LatestAttestableHeight(ctx, req.Msg.Attestor)
	switch {
	case errors.Is(err, attestor.ErrNotFound):
		return nil, connect.NewError(connect.CodeNotFound, err)
	case err != nil:
		// todo: move to interceptor
		h.logger.Error("LatestAttestableHeight", "error", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proto.LatestAttestableHeightResponse{Height: height}), nil
}
