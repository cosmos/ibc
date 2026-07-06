package bootstrap

import (
	"log/slog"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/server"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/relayer"
)

// Services is an outcome of IBC Link wiring (dep inject)
type Services struct {
	Server *server.Server
	Logger *slog.Logger
}

// BuildRelayer converts config into a runnable relayer process with all of the deps provisioned
func BuildRelayer(cfg config.Config) (*Services, error) {
	// Services
	relayerService := relayer.New()

	// Handlers
	relayerHandler := server.NewRelayerHandler(relayerService)

	// Server
	srv := server.New(cfg.GRPC.ListenAddress, true)
	srv.Register(relayerHandler)

	// FUTURE: if config has .attestor.attestations has length > 0,
	// then also provision attestor GRPC!
	// A single grpc port would contain two services: relayer and attestor.

	return &Services{
		Server: srv,
		Logger: slog.With("module", "bootstrap"),
	}, nil
}

// BuildAttestors converts config into a runnable attestor process with all of the deps provisioned
func BuildAttestor(cfg config.Config) (*Services, error) {
	// Services
	attestorService := attestor.New()

	// Handlers
	attestorHandler := server.NewAttestorHandler(attestorService)

	// Server
	srv := server.New(cfg.GRPC.ListenAddress, true)
	srv.Register(attestorHandler)

	// todo provision attestors (local/remote)

	return &Services{
		Server: srv,
		Logger: slog.With("module", "bootstrap"),
	}, nil
}
