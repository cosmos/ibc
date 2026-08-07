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
	// SourceSignerAlias and DestSignerAlias select the tx submitters used to
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
// flags follow the source chain. Finality offsets are read from the already-
// resolved proofGenerators (each generator's offset reflects the attestors
// matched for its counterparty chain), not from config directly.
func OptionsFromConfig(cfg config.Config, proofGenerators processors.ProofGenerators, route processors.Route) Options {
	opts := Options{
		RecvBatchSize:       DefaultBatchSize,
		RecvBatchTimeout:    DefaultBatchTimeout,
		AckBatchSize:        DefaultBatchSize,
		AckBatchTimeout:     DefaultBatchTimeout,
		TimeoutBatchSize:    DefaultBatchSize,
		TimeoutBatchTimeout: DefaultTimeoutBatchTimeout,
	}

	if sourceEnd, destEnd, ok := cfg.Relayer.ClientEnd(route.SourceChainID, route.SourceClientID); ok {
		opts.SourceSignerAlias = sourceEnd.Signer
		opts.DestSignerAlias = destEnd.Signer
	}

	if sourceGen, ok := proofGenerators.Get(route.SourceChainID, route.SourceClientID); ok {
		offset := sourceGen.FinalityOffset()
		opts.DestinationFinalityOffset = &offset
	}

	if destGen, ok := proofGenerators.Get(route.DestinationChainID, route.DestinationClientID); ok {
		offset := destGen.FinalityOffset()
		opts.SourceFinalityOffset = &offset
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
	}

	if dst, ok := cfg.Relayer.ChainOverride(route.DestinationChainID); ok {
		if dst.PacketBatchSize != nil {
			opts.RecvBatchSize = *dst.PacketBatchSize
		}

		if dst.PacketBatchTimeout != nil {
			opts.RecvBatchTimeout = *dst.PacketBatchTimeout
		}
	}

	return opts
}
