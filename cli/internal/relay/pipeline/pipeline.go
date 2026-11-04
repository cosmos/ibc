// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliveryhero/pipeline/v2"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
)

const (
	// inputBufferSize is high enough that pushing to the pipeline never blocks
	inputBufferSize = 10000
	// stageConcurrency transfers processed concurrently per stage
	stageConcurrency = 10
	// batchConcurrency batches submitted concurrently per batch stage
	batchConcurrency = 10
)

// Storage the persistence used by the pipeline.
type Storage interface {
	StatusStorage
	processors.RecvTxStorage
	processors.AckTimeoutTxStorage
	processors.WriteAckStorage
	processors.ClearRecvTxStorage
	processors.ClearAckTxStorage
	processors.ClearTimeoutTxStorage
	processors.TxStorage
}

// TxSubmitters resolves the tx submitter for a (chain, signer) pair.
type TxSubmitters interface {
	Get(chainID, signerAlias string) (txsubmitter.TxSubmitter, bool)
}

// ChainClients resolves chain clients by chain id.
type ChainClients interface {
	Get(chainID string) (chains.Client, bool)
}

// Provers resolves provers by (chainID, clientID).
type Provers interface {
	Get(chainID, clientID string) (prover.Prover, bool)
}

// TxBuilders resolves tx builders by chain id.
type TxBuilders interface {
	Get(chainID string) (txbuilder.TxBuilder, bool)
}

// Deps the external systems a pipeline relays through.
type Deps struct {
	Storage      Storage
	Chains       ChainClients
	Provers      Provers
	TxBuilders   TxBuilders
	TxSubmitters TxSubmitters
}

// Pipeline relays transfers pushed to its input through the full packet
// lifecycle. The pipeline owner closes the input via Close; the output closes
// once the context is canceled and in-flight transfers drain.
type Pipeline struct {
	input  chan *processors.Transfer
	output <-chan *processors.Transfer
}

// TransferPipeline accepts transfers to relay and emits them once processed.
type TransferPipeline interface {
	Push(ctx context.Context, tr *processors.Transfer) bool
	Poll() (*processors.Transfer, error)
	Close()
}

var _ TransferPipeline = (*Pipeline)(nil)

// NewPipeline builds the relaying pipeline for one route. Each pipeline is
// unique to a route because batch stages assume all packets in a batch share
// the route's destination.
func NewPipeline(
	ctx context.Context,
	logger *slog.Logger,
	deps Deps,
	route processors.Route,
	opts Options,
) (*Pipeline, error) {
	resolved, err := resolveRoute(deps, route, opts)
	if err != nil {
		return nil, err
	}

	logger = logger.With(
		"sourceChainID", route.SourceChainID,
		"sourceClientID", route.SourceClientID,
		"destinationChainID", route.DestinationChainID,
		"destinationClientID", route.DestinationClientID,
	)

	input := make(chan *processors.Transfer, inputBufferSize)

	output := pipeline.Emitter(ctx, func() *processors.Transfer {
		return <-input
	})

	for _, step := range stages(logger, deps.Storage, resolved, opts) {
		if step.single == nil {
			output = BatchProcess(ctx, logger, batchConcurrency, step.batchSize, step.batchTimeout, output, step.batch)
			continue
		}

		output = pipeline.ProcessConcurrently(ctx, stageConcurrency, step.single, output)
	}

	return &Pipeline{input: input, output: output}, nil
}

// resolveRoute looks up everything the route relays through, failing on the
// first piece of configuration that is missing.
func resolveRoute(deps Deps, route processors.Route, opts Options) (processors.RouteDeps, error) {
	end := func(chainID, clientID, signerAlias, side string) (processors.RouteEnd, error) {
		chain, ok := deps.Chains.Get(chainID)
		if !ok {
			return processors.RouteEnd{}, errors.Errorf("no configured chain client for %s chain %s", side, chainID)
		}

		prover, ok := deps.Provers.Get(chainID, clientID)
		if !ok {
			return processors.RouteEnd{}, errors.Errorf(
				"no prover configured for client %q on chain %q",
				clientID,
				chainID,
			)
		}

		txBuilder, ok := deps.TxBuilders.Get(chainID)
		if !ok {
			return processors.RouteEnd{}, errors.Errorf("no tx builder configured for %s chain %s", side, chainID)
		}

		txSubmitter, ok := deps.TxSubmitters.Get(chainID, signerAlias)
		if !ok {
			return processors.RouteEnd{}, errors.Errorf(
				"no configured tx submitter for %s chain %s and signer %q", side, chainID, signerAlias,
			)
		}

		return processors.RouteEnd{Chain: chain, Prover: prover, TxBuilder: txBuilder, TxSubmitter: txSubmitter}, nil
	}

	source, err := end(route.SourceChainID, route.SourceClientID, opts.SourceSignerAlias, "source")
	if err != nil {
		return processors.RouteDeps{}, err
	}

	destination, err := end(route.DestinationChainID, route.DestinationClientID, opts.DestSignerAlias, "destination")
	if err != nil {
		return processors.RouteDeps{}, err
	}

	return processors.RouteDeps{Route: route, Source: source, Destination: destination}, nil
}

// stage one step of the pipeline: a per-transfer processor, or, when single
// is nil, a batch processor with the limits its batches are collected under.
type stage struct {
	single       pipeline.Processor[*processors.Transfer, *processors.Transfer]
	batch        BatchProcessorMW
	batchSize    int
	batchTimeout time.Duration
}

// stages lists the route's pipeline steps in the order transfers pass
// through them.
func stages(logger *slog.Logger, storage Storage, route processors.RouteDeps, opts Options) []stage {
	single := func(p Processor) stage { return stage{single: NewProcessorMW(storage, p)} }
	batch := func(p BatchProcessor, size int, timeout time.Duration) stage {
		return stage{batch: NewBatchProcessorMW(storage, p), batchSize: size, batchTimeout: timeout}
	}

	return []stage{
		// populate the recv tx if the packet was already delivered by someone else
		single(processors.NewCheckRecvPacketDelivery(route, storage)),
		// populate the ack or timeout tx if the packet commitment is already gone;
		// the packet may have been timed out before we saw it
		single(processors.NewCheckPacketCommitment(route, storage)),
		// wait for the send tx's packet event to be at or before what the
		// destination client can currently prove
		single(processors.NewCheckSendFinality(route)),
		// before timing out, wait for the source client to be able to prove a
		// destination-chain timestamp past the timeout
		single(processors.NewCheckTimeoutFinality(route)),
		// deliver timeouts in batches on the source chain
		batch(
			processors.NewBatchTimeoutPacket(route, storage, logger),
			opts.TimeoutBatchSize,
			opts.TimeoutBatchTimeout,
		),
		// clear timeout txs that failed, stalled, or omitted this packet
		single(processors.NewRetryTimeoutPacket(route, storage)),
		// deliver recvs in batches on the destination chain
		batch(processors.NewBatchRecvPacket(route, storage, logger), opts.RecvBatchSize, opts.RecvBatchTimeout),
		// clear stuck or failed recv txs so they are redelivered next run
		single(processors.NewRetryRecvPacket(route, storage)),
		// extract the write ack from the recv tx on the destination chain
		single(processors.NewWaitForWriteAck(route, storage)),
		// wait for the write ack tx's packet event to be at or before what the
		// source client can currently prove
		single(processors.NewCheckWriteAckFinality(route)),
		// populate the ack or timeout tx if the packet commitment is now gone
		single(processors.NewCheckPacketCommitment(route, storage)),
		// deliver acks in batches on the source chain
		batch(processors.NewBatchAckPacket(route, storage, logger), opts.AckBatchSize, opts.AckBatchTimeout),
		// clear ack txs that failed, stalled, or omitted this packet
		single(processors.NewRetryAckPacket(route, storage)),
		// assign terminal statuses
		{single: processors.NewStateFinisher(storage)},
	}
}

func (p *Pipeline) Push(ctx context.Context, tr *processors.Transfer) bool {
	select {
	case p.input <- tr:
		return true
	case <-ctx.Done():
		return false
	}
}

func (p *Pipeline) Poll() (*processors.Transfer, error) {
	tr, ok := <-p.output
	if !ok {
		return nil, errors.New("pipeline closed")
	}

	return tr, nil
}

func (p *Pipeline) Close() {
	close(p.input)
}
