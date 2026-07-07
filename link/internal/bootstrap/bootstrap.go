package bootstrap

import (
	"context"
	"log/slog"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/server"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/relayer"
	"github.com/cosmos/ibc/link/internal/store"
)

// Services is an outcome of IBC Link wiring (dep inject)
type Services struct {
	Context context.Context
	Logger  *slog.Logger
	Server  *server.Server

	Store store.Store

	RelayerService  *relayer.Service
	AttestorService *attestor.Service
}

// BuildRelayer converts config into a runnable relayer process with all of the deps provisioned
func BuildRelayer(cfg config.Config) (*Services, error) {
	ctx := context.Background()
	logger := slog.With("module", "bootstrap")

	// Storage
	db, err := store.NewStore(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Services
	relayerService := relayer.New()

	// Handlers
	relayerHandler := server.NewRelayerHandler(relayerService)

	// Server
	srv := server.New(cfg.Server.ListenAddress, true)
	srv.Register(relayerHandler)

	services := &Services{
		Context:         ctx,
		Logger:          logger,
		Server:          srv,
		Store:           db,
		RelayerService:  relayerService,
		AttestorService: nil,
	}

	// Dual mode: if .attestor config is provided, then we can run both relayer and attestor in the same process.
	// This might be useful for PoC/testing environments or when an operator wants to run the relayer
	// and one of attestors in the same process
	if len(cfg.Attestor.Attestations) > 0 {
		logger.Info("Attestor config provided, running in dual mode: relayer with attestor")

		attestorService, attestorHandler, err := buildAttestor(cfg)
		if err != nil {
			return nil, err
		}

		services.AttestorService = attestorService
		srv.Register(attestorHandler)
	}

	return services, nil
}

// BuildAttestors converts config into a runnable attestor process with all of the deps provisioned
func BuildAttestor(cfg config.Config) (*Services, error) {
	ctx := context.Background()
	logger := slog.With("module", "bootstrap")

	attestorService, attestorHandler, err := buildAttestor(cfg)
	if err != nil {
		return nil, err
	}

	// Server
	srv := server.New(cfg.Server.ListenAddress, true)
	srv.Register(attestorHandler)

	return &Services{
		Context:         ctx,
		Logger:          logger,
		Server:          srv,
		Store:           nil, // attestor is stateless
		RelayerService:  nil,
		AttestorService: attestorService,
	}, nil
}

func buildAttestor(cfg config.Config) (*attestor.Service, *server.AttestorHandler, error) {
	// Services
	attestorService, err := attestor.NewFromConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	// Handlers
	attestorHandler := server.NewAttestorHandler(attestorService)

	return attestorService, attestorHandler, nil
}
