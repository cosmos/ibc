package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/pkg/errors"
)

// Server wraps the HTTP server and registered RPC handlers.
type Server struct {
	mux    *http.ServeMux
	server *http.Server
	logger *slog.Logger

	useReflection        bool
	serviceNames         []string
	reflectionRegistered bool
}

// Handler represents GRPC handler
type Handler interface {
	Register(opts ...connect.HandlerOption) (prefix string, handler http.Handler)
	Name() string
}

var errInternal = connect.NewError(connect.CodeInternal, errors.New("internal server error"))

// New Server constructor.
func New(addr string, useReflection bool) *Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	mux := http.NewServeMux()

	return &Server{
		mux:           mux,
		useReflection: useReflection,
		server: &http.Server{
			Addr:      addr,
			Handler:   mux,
			Protocols: protocols,
		},
		logger: slog.With("module", "server"),
	}
}

// Start starts the server. Not safe to call twice.
func (s *Server) Start() error {
	s.setupReflection()

	s.logger.Info("Starting server", "address", s.server.Addr)

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	go s.start(ln)

	return nil
}

func (s *Server) Stop() error {
	const timeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.logger.Info("Shutting down server")

	return s.server.Shutdown(ctx)
}

func (s *Server) Register(h Handler) {
	// we might want to pass some global options here later
	prefix, handler := h.Register()
	s.logger.Debug("Registered handler", "prefix", prefix)

	s.mux.Handle(prefix, handler)
	s.serviceNames = append(s.serviceNames, h.Name())
}

func (s *Server) start(ln net.Listener) {
	err := s.server.Serve(ln)
	switch err {
	case nil, http.ErrServerClosed:
		s.logger.Info("Server stopped")
	default:
		s.logger.Error("Failed to serve", "err", err)
	}
}

func (s *Server) setupReflection() {
	if !s.useReflection || s.reflectionRegistered || len(s.serviceNames) == 0 {
		return
	}

	reflector := grpcreflect.NewStaticReflector(s.serviceNames...)
	s.mux.Handle(grpcreflect.NewHandlerV1(reflector))
	s.mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	s.reflectionRegistered = true
}
