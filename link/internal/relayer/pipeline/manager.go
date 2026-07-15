package pipeline

import (
	"context"
	"log/slog"
	"sync"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
)

// Manager creates and caches one pipeline per route.
type Manager struct {
	logger *slog.Logger
	cfg    config.Config
	deps   Deps

	mu        sync.Mutex
	pipelines map[Route]TransferPipeline
}

// RouteManager routes transfers to their pipeline.
type RouteManager interface {
	Pipeline(ctx context.Context, transfer *Transfer) (TransferPipeline, error)
	Close()
}

var _ RouteManager = (*Manager)(nil)

func NewManager(logger *slog.Logger, cfg config.Config, deps Deps) *Manager {
	return &Manager{
		logger:    logger,
		cfg:       cfg,
		deps:      deps,
		pipelines: make(map[Route]TransferPipeline),
	}
}

// Pipeline returns the pipeline for the transfer's route, creating it on
// first use. Transfers whose source client has no configured route error.
func (m *Manager) Pipeline(ctx context.Context, transfer *Transfer) (TransferPipeline, error) {
	route := Route{
		SourceChainID:       transfer.SourceChainID,
		SourceClientID:      transfer.PacketSourceClientID,
		DestinationChainID:  transfer.DestinationChainID,
		DestinationClientID: transfer.PacketDestinationClientID,
	}

	if !m.isRouted(route) {
		return nil, errors.Errorf(
			"no route configured for client %q on chain %q",
			route.SourceClientID, route.SourceChainID,
		)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if pipeline, ok := m.pipelines[route]; ok {
		return pipeline, nil
	}

	m.logger.Info(
		"Creating pipeline",
		"sourceChainID", route.SourceChainID,
		"sourceClientID", route.SourceClientID,
		"destinationChainID", route.DestinationChainID,
		"destinationClientID", route.DestinationClientID,
	)

	pipeline := NewDeduper(NewPipeline(ctx, m.logger, m.deps, route, OptionsFromConfig(m.cfg, route)))
	m.pipelines[route] = pipeline

	return pipeline, nil
}

func (m *Manager) isRouted(route Route) bool {
	client, ok := m.cfg.Relayer.Client(route.SourceChainID, route.SourceClientID)
	if !ok {
		return false
	}

	for _, r := range m.cfg.Relayer.Routes {
		if r.SourceClient == client.Alias {
			return true
		}
	}

	return false
}

// Close closes every pipeline. The context the pipelines were created with
// must already be canceled: pipeline outputs only close on cancellation.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pipeline := range m.pipelines {
		pipeline.Close()
	}
}
