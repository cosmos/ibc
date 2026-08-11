// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"context"
	"log/slog"
	"sync"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/pipeline"
	"github.com/cosmos/ibc/link/internal/relay/processors"
)

// PipelineSet creates and caches one pipeline per route.
type PipelineSet struct {
	logger *slog.Logger
	cfg    config.Config
	deps   pipeline.Deps

	mu        sync.Mutex
	pipelines map[processors.Route]pipeline.TransferPipeline
}

// Pipelines identifies the pipeline a transfer is relayed through.
type Pipelines interface {
	Pipeline(ctx context.Context, tr *processors.Transfer) (pipeline.TransferPipeline, error)
	Close()
}

var _ Pipelines = (*PipelineSet)(nil)

func NewPipelineSet(logger *slog.Logger, cfg config.Config, deps pipeline.Deps) *PipelineSet {
	return &PipelineSet{
		logger:    logger,
		cfg:       cfg,
		deps:      deps,
		pipelines: make(map[processors.Route]pipeline.TransferPipeline),
	}
}

// Pipeline returns the pipeline for the transfer's route, creating it on
// first use. Transfers whose source client has no configured route error.
func (s *PipelineSet) Pipeline(ctx context.Context, tr *processors.Transfer) (pipeline.TransferPipeline, error) {
	route := processors.Route{
		SourceChainID:       tr.SourceChainID,
		SourceClientID:      tr.PacketSourceClientID,
		DestinationChainID:  tr.DestinationChainID,
		DestinationClientID: tr.PacketDestinationClientID,
	}

	if !s.isRouted(route) {
		return nil, errors.Errorf(
			"no route configured for client %q on chain %q",
			route.SourceClientID, route.SourceChainID,
		)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if pl, ok := s.pipelines[route]; ok {
		return pl, nil
	}

	s.logger.Info(
		"Creating pipeline",
		"sourceChainID", route.SourceChainID,
		"sourceClientID", route.SourceClientID,
		"destinationChainID", route.DestinationChainID,
		"destinationClientID", route.DestinationClientID,
	)

	inner, err := pipeline.NewPipeline(ctx, s.logger, s.deps, route, pipeline.OptionsFromConfig(s.cfg, route))
	if err != nil {
		return nil, errors.Wrap(err, "creating pipeline")
	}

	pl := NewDeduper(inner)
	s.pipelines[route] = pl

	return pl, nil
}

func (s *PipelineSet) isRouted(route processors.Route) bool {
	_, _, ok := s.cfg.Relayer.ClientEnd(route.SourceChainID, route.SourceClientID)
	return ok
}

// Close closes every pipeline. The context the pipelines were created with
// must already be canceled: pipeline outputs only close on cancellation.
func (s *PipelineSet) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pl := range s.pipelines {
		pl.Close()
	}
}
