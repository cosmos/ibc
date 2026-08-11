// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"
	"log/slog"

	"github.com/deliveryhero/pipeline/v2"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/processors"
	"github.com/cosmos/ibc/link/internal/txsubmitter"
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

// Deps the external systems a pipeline relays through.
type Deps struct {
	Storage         Storage
	Chains          processors.ChainClients
	ProofGenerators processors.ProofGenerators
	TxBuilders      processors.TxBuilders
	TxSubmitters    TxSubmitters
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
	srcTxSubmitter, ok := deps.TxSubmitters.Get(route.SourceChainID, opts.SourceSignerAlias)
	if !ok {
		return nil, errors.Errorf(
			"no configured tx submitter for source chain %s and signer %q",
			route.SourceChainID, opts.SourceSignerAlias,
		)
	}

	dstTxSubmitter, ok := deps.TxSubmitters.Get(route.DestinationChainID, opts.DestSignerAlias)
	if !ok {
		return nil, errors.Errorf(
			"no configured tx submitter for destination chain %s and signer %q",
			route.DestinationChainID, opts.DestSignerAlias,
		)
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

	// populate the recv tx if the packet was already delivered by someone else
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewCheckRecvPacketDelivery(deps.Chains, deps.Storage)), output)

	// populate the ack or timeout tx if the packet commitment is already gone;
	// the packet may have been timed out before we saw it
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewCheckPacketCommitment(deps.Chains, deps.Storage)), output)

	// wait for the send tx's packet event to be at or before what the
	// destination client can currently prove
	checkSendFinality, err := processors.NewCheckSendFinality(deps.Chains, deps.ProofGenerators, route)
	if err != nil {
		return nil, errors.Wrap(err, "constructing check send finality processor")
	}

	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, checkSendFinality), output)

	// before timing out, wait for the source client to be able to prove a
	// destination-chain timestamp past the timeout
	checkTimeoutFinality, err := processors.NewCheckTimeoutFinality(deps.ProofGenerators, route)
	if err != nil {
		return nil, errors.Wrap(err, "constructing check timeout finality processor")
	}

	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, checkTimeoutFinality), output)

	// deliver timeouts in batches on the source chain
	batchTimeoutPacket, err := processors.NewBatchTimeoutPacket(
		deps.Chains, deps.ProofGenerators, deps.TxBuilders, deps.Storage, srcTxSubmitter, route,
	)
	if err != nil {
		return nil, errors.Wrap(err, "constructing batch timeout packet processor")
	}

	output = BatchProcess(
		ctx,
		logger,
		batchConcurrency,
		opts.TimeoutBatchSize,
		opts.TimeoutBatchTimeout,
		output,
		NewBatchProcessorMW(deps.Storage, batchTimeoutPacket),
	)

	// clear stuck or failed timeout txs so they are redelivered next run
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewRetryTimeoutPacket(srcTxSubmitter, deps.Storage, route)), output)

	// deliver recvs in batches on the destination chain
	batchRecvPacket, err := processors.NewBatchRecvPacket(
		deps.Chains, deps.ProofGenerators, deps.TxBuilders, deps.Storage, dstTxSubmitter, route,
	)
	if err != nil {
		return nil, errors.Wrap(err, "constructing batch recv packet processor")
	}

	output = BatchProcess(
		ctx,
		logger,
		batchConcurrency,
		opts.RecvBatchSize,
		opts.RecvBatchTimeout,
		output,
		NewBatchProcessorMW(deps.Storage, batchRecvPacket),
	)

	// clear stuck or failed recv txs so they are redelivered next run
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewRetryRecvPacket(dstTxSubmitter, deps.Storage, route)), output)

	// extract the write ack from the recv tx on the destination chain
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewWaitForWriteAck(deps.Chains, deps.Storage)), output)

	// wait for the write ack tx's packet event to be at or before what the
	// source client can currently prove
	checkWriteAckFinality, err := processors.NewCheckWriteAckFinality(deps.Chains, deps.ProofGenerators, route)
	if err != nil {
		return nil, errors.Wrap(err, "constructing check write ack finality processor")
	}

	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, checkWriteAckFinality), output)

	// populate the ack or timeout tx if the packet commitment is now gone
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewCheckPacketCommitment(deps.Chains, deps.Storage)), output)

	// deliver acks in batches on the source chain
	batchAckPacket, err := processors.NewBatchAckPacket(
		deps.Chains, deps.ProofGenerators, deps.TxBuilders, deps.Storage, srcTxSubmitter, route,
	)
	if err != nil {
		return nil, errors.Wrap(err, "constructing batch ack packet processor")
	}

	output = BatchProcess(
		ctx,
		logger,
		batchConcurrency,
		opts.AckBatchSize,
		opts.AckBatchTimeout,
		output,
		NewBatchProcessorMW(deps.Storage, batchAckPacket),
	)

	// clear stuck or failed ack txs so they are redelivered next run
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, processors.NewRetryAckPacket(srcTxSubmitter, deps.Storage, route)), output)

	// assign terminal statuses
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		processors.NewStateFinisher(deps.Storage), output)

	return &Pipeline{input: input, output: output}, nil
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
