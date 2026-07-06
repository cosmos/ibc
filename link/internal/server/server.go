package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
)

type Server struct {
	mux    *http.ServeMux
	server *http.Server
	logger *slog.Logger
}

type ServerHandler interface {
	Register(opts ...connect.HandlerOption) (prefix string, handler http.Handler)
}

var errInternal = connect.NewError(connect.CodeInternal, errors.New("internal server error"))

// New Server constructor.
func New(addr string) *Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	mux := http.NewServeMux()

	return &Server{
		mux: mux,
		server: &http.Server{
			Addr:      addr,
			Handler:   mux,
			Protocols: protocols,
		},
		logger: slog.With("module", "server"),
	}
}

func (s *Server) Start() error {
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

	return s.server.Shutdown(ctx)
}

func (s *Server) Register(h ServerHandler) {
	// we might want to pass some global options here later
	prefix, handler := h.Register(nil)
	s.logger.Debug("Registered handler", "prefix", prefix)

	s.mux.Handle(prefix, handler)
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
