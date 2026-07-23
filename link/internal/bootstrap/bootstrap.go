package bootstrap

import (
	"context"
	"log/slog"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/dispatch"
	"github.com/cosmos/ibc/link/internal/relay/pipeline"
	proofgenattestation "github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	txbuilderevm "github.com/cosmos/ibc/link/internal/relay/txbuilder/evm"
	"github.com/cosmos/ibc/link/internal/server"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/relayer"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// Services is an outcome of IBC Link wiring (dep inject)
type Services struct {
	Context context.Context
	Logger  *slog.Logger
	Server  *server.Server

	Store   store.Store
	Signers *signer.Set

	// Dispatcher drives packet relaying
	Dispatcher *dispatch.RelayDispatcher

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

	// Chain clients
	clientSet, err := chains.NewClientSetFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Signers
	signers, err := signerSet(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Services
	relayerService := relayer.New(cfg, db, clientSet)

	// Handlers
	relayerHandler := server.NewRelayerHandler(relayerService)

	// Server
	srv := server.New(cfg.Server.ListenAddress, true)
	srv.Register(relayerHandler)

	// Relaying dispatcher
	var dispatcher *dispatch.RelayDispatcher
	if len(cfg.Relayer.Routes) > 0 {
		txManagers, errTxManagers := txmgr.NewFromConfig(cfg, signers)
		if errTxManagers != nil {
			return nil, errTxManagers
		}

		proofGenerators, errProofGenerators := proofgenattestation.NewSetFromConfig(cfg, clientSet, signers)
		if errProofGenerators != nil {
			return nil, errProofGenerators
		}

		txBuilders, errTxBuilders := txbuilderevm.NewSetFromConfig(cfg)
		if errTxBuilders != nil {
			return nil, errTxBuilders
		}

		pipelineManager := dispatch.NewManager(logger, cfg, pipeline.Deps{
			Storage:         db,
			Chains:          clientSet,
			ProofGenerators: proofGenerators,
			TxBuilders:      txBuilders,
			TxManagers:      txManagers,
		})

		dispatcher = dispatch.NewRelayDispatcher(db, pipelineManager, dispatch.DefaultPollInterval, logger)
	}

	services := &Services{
		Context:        ctx,
		Logger:         logger,
		Server:         srv,
		Store:          db,
		Signers:        signers,
		RelayerService: relayerService,
		Dispatcher:     dispatcher,
	}

	// Dual mode: if .attestor config is provided, then we can run both relayer and attestor in the same process.
	// This might be useful for PoC/testing environments or when an operator wants to run the relayer
	// and one of attestors in the same process
	if len(cfg.Attestor.Attestations) > 0 {
		logger.Info("Attestor config provided, running in dual mode: relayer with attestor")

		attestorService, attestorHandler, err := buildAttestor(cfg, clientSet, signers)
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

	// Chain clients
	clientSet, err := chains.NewClientSetFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Signers
	signers, err := signerSet(ctx, cfg)
	if err != nil {
		return nil, err
	}

	attestorService, attestorHandler, err := buildAttestor(cfg, clientSet, signers)
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
		Signers:         signers,
		RelayerService:  nil,
		AttestorService: attestorService,
	}, nil
}

func buildAttestor(
	cfg config.Config,
	clients *chains.ClientSet,
	signers *signer.Set,
) (*attestor.Service, *server.AttestorHandler, error) {
	// Services
	attestorService, err := attestor.NewFromConfig(cfg, clients, signers)
	if err != nil {
		return nil, nil, err
	}

	// Handlers
	attestorHandler := server.NewAttestorHandler(attestorService)

	return attestorService, attestorHandler, nil
}

func signerSet(ctx context.Context, cfg config.Config) (*signer.Set, error) {
	set := signer.NewSet()

	for _, signerConfig := range cfg.Signers {
		signer, alias, err := signer.NewSignerFromConfig(ctx, signerConfig)
		if err != nil {
			return nil, err
		}

		set.Set(alias, signer)
	}

	return set, nil
}
