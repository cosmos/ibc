package pipeline

import (
	"time"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/processors"
)

// Batch defaults applied when a chain has no overrides.
const (
	DefaultBatchSize           = 50
	DefaultBatchTimeout        = 10 * time.Second
	DefaultTimeoutBatchTimeout = time.Minute
)

// Options the per-route pipeline settings.
type Options struct {
	// SourceSignerAlias and DestSignerAlias select the tx managers used to
	// submit on the route's source and destination chains.
	SourceSignerAlias string
	DestSignerAlias   string

	RecvBatchSize       int
	RecvBatchTimeout    time.Duration
	AckBatchSize        int
	AckBatchTimeout     time.Duration
	TimeoutBatchSize    int
	TimeoutBatchTimeout time.Duration

	SourceFinalityOffset      *uint64
	DestinationFinalityOffset *uint64
}

// OptionsFromConfig maps chain overrides onto pipeline options: recv batching
// follows the destination chain, ack and timeout batching and the ack relay
// flags follow the source chain.
func OptionsFromConfig(cfg config.Config, route processors.Route) Options {
	opts := Options{
		RecvBatchSize:       DefaultBatchSize,
		RecvBatchTimeout:    DefaultBatchTimeout,
		AckBatchSize:        DefaultBatchSize,
		AckBatchTimeout:     DefaultBatchTimeout,
		TimeoutBatchSize:    DefaultBatchSize,
		TimeoutBatchTimeout: DefaultTimeoutBatchTimeout,
	}

	if client, ok := cfg.Relayer.Client(route.SourceChainID, route.SourceClientID); ok {
		for _, r := range cfg.Relayer.Routes {
			if r.SourceClient == client.Alias {
				opts.SourceSignerAlias = r.SourceSignerAlias
				opts.DestSignerAlias = r.DestSignerAlias

				break
			}
		}
	}

	if src, ok := cfg.Relayer.ChainOverride(route.SourceChainID); ok {

		if src.PacketBatchSize != nil {
			opts.AckBatchSize = *src.PacketBatchSize
			opts.TimeoutBatchSize = *src.PacketBatchSize
		}

		if src.PacketBatchTimeout != nil {
			opts.AckBatchTimeout = *src.PacketBatchTimeout
			opts.TimeoutBatchTimeout = *src.PacketBatchTimeout
		}

		opts.SourceFinalityOffset = src.FinalityOffset
	}

	if dst, ok := cfg.Relayer.ChainOverride(route.DestinationChainID); ok {
		if dst.PacketBatchSize != nil {
			opts.RecvBatchSize = *dst.PacketBatchSize
		}

		if dst.PacketBatchTimeout != nil {
			opts.RecvBatchTimeout = *dst.PacketBatchTimeout
		}

		opts.DestinationFinalityOffset = dst.FinalityOffset
	}

	return opts
}
