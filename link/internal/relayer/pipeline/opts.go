package pipeline

import (
	"time"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relayer/transfer"
)

// Batch defaults applied when a chain has no overrides.
const (
	DefaultBatchSize           = 50
	DefaultBatchTimeout        = 10 * time.Second
	DefaultTimeoutBatchTimeout = time.Minute
)

// Ack relaying defaults
const (
	defaultRelaySuccessAcks = false
	defaultRelayErrorAcks   = true
)

// Options the per-route pipeline settings.
type Options struct {
	RelaySuccessAcks bool
	RelayErrorAcks   bool

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
func OptionsFromConfig(cfg config.Config, route transfer.Route) Options {
	opts := Options{
		RelaySuccessAcks:    defaultRelaySuccessAcks,
		RelayErrorAcks:      defaultRelayErrorAcks,
		RecvBatchSize:       DefaultBatchSize,
		RecvBatchTimeout:    DefaultBatchTimeout,
		AckBatchSize:        DefaultBatchSize,
		AckBatchTimeout:     DefaultBatchTimeout,
		TimeoutBatchSize:    DefaultBatchSize,
		TimeoutBatchTimeout: DefaultTimeoutBatchTimeout,
	}

	if src := cfg.Relayer.ChainOverride(route.SourceChainID); src != nil {
		if src.RelaySuccessAcks != nil {
			opts.RelaySuccessAcks = *src.RelaySuccessAcks
		}

		if src.RelayErrorAcks != nil {
			opts.RelayErrorAcks = *src.RelayErrorAcks
		}

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

	if dst := cfg.Relayer.ChainOverride(route.DestinationChainID); dst != nil {
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
