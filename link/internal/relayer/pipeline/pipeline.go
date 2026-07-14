package pipeline

import (
	"context"
	"log/slog"

	"github.com/deliveryhero/pipeline/v2"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
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
	RecvTxStorage
	AckTimeoutTxStorage
	WriteAckStorage
	ClearTxStorage
	TxStorage
}

// Deps the external systems a pipeline relays through.
type Deps struct {
	Storage   Storage
	Chains    ChainClients
	ProofAPI  proto.ProofApiServiceClient
	Submitter txmgr.Submitter
}

// Pipeline relays transfers pushed to its input through the full packet
// lifecycle. The pipeline owner closes the input via Close; the output closes
// once the context is canceled and in-flight transfers drain.
type Pipeline struct {
	input  chan *Transfer
	output <-chan *Transfer
}

// TransferPipeline accepts transfers to relay and emits them once processed.
type TransferPipeline interface {
	Push(ctx context.Context, transfer *Transfer) bool
	Poll() (*Transfer, error)
	Close()
}

var _ TransferPipeline = (*Pipeline)(nil)

// NewPipeline builds the relaying pipeline for one route. Each pipeline is
// unique to a route because batch stages assume all packets in a batch share
// the route's destination.
func NewPipeline(ctx context.Context, logger *slog.Logger, deps Deps, route Route, opts Options) *Pipeline {
	logger = logger.With(
		"sourceChainID", route.SourceChainID,
		"sourceClientID", route.SourceClientID,
		"destinationChainID", route.DestinationChainID,
		"destinationClientID", route.DestinationClientID,
	)

	input := make(chan *Transfer, inputBufferSize)

	output := pipeline.Emitter(ctx, func() *Transfer {
		return <-input
	})

	// populate the recv tx if the packet was already delivered by someone else
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewCheckRecvPacketDelivery(deps.Chains, deps.Storage)), output)

	// populate the ack or timeout tx if the packet commitment is already gone;
	// the packet may have been timed out before we saw it
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewCheckPacketCommitment(deps.Chains, deps.Storage)), output)

	// wait for the send tx to finalize on the source chain
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewCheckSendFinality(deps.Chains, opts.SourceFinalityOffset)), output)

	// before timing out, wait for the timeout timestamp to finalize on the
	// destination chain
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewCheckTimeoutFinality(deps.Chains, opts.DestinationFinalityOffset)), output)

	// deliver timeouts in batches on the source chain
	output = ConditionallyBatchProcess(
		ctx,
		logger,
		batchConcurrency,
		opts.TimeoutBatchSize,
		opts.TimeoutBatchTimeout,
		output,
		NewBatchProcessorMW(
			deps.Storage,
			NewBatchTimeoutPacket(deps.Chains, deps.Storage, deps.ProofAPI, deps.Submitter, route),
		),
	)

	// clear stuck or failed timeout txs so they are redelivered next run
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewRetryTimeoutPacket(deps.Submitter, deps.Storage, route)), output)

	// deliver recvs in batches on the destination chain
	output = ConditionallyBatchProcess(
		ctx,
		logger,
		batchConcurrency,
		opts.RecvBatchSize,
		opts.RecvBatchTimeout,
		output,
		NewBatchProcessorMW(
			deps.Storage,
			NewBatchRecvPacket(deps.Chains, deps.Storage, deps.ProofAPI, deps.Submitter, route),
		),
	)

	// clear stuck or failed recv txs so they are redelivered next run
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewRetryRecvPacket(deps.Submitter, deps.Storage, route)), output)

	// extract the write ack from the recv tx on the destination chain
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewWaitForWriteAck(deps.Chains, deps.Storage)), output)

	// wait for the write ack tx to finalize on the destination chain
	output = pipeline.ProcessConcurrently(
		ctx,
		stageConcurrency,
		NewProcessorMW(
			deps.Storage,
			NewCheckWriteAckFinality(
				deps.Chains,
				opts.RelaySuccessAcks,
				opts.RelayErrorAcks,
				opts.DestinationFinalityOffset,
			),
		),
		output,
	)

	// populate the ack or timeout tx if the packet commitment is now gone
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewCheckPacketCommitment(deps.Chains, deps.Storage)), output)

	// deliver acks in batches on the source chain
	output = ConditionallyBatchProcess(
		ctx,
		logger,
		batchConcurrency,
		opts.AckBatchSize,
		opts.AckBatchTimeout,
		output,
		NewBatchProcessorMW(
			deps.Storage,
			NewBatchAckPacket(
				deps.Chains,
				deps.Storage,
				deps.ProofAPI,
				deps.Submitter,
				route,
				opts.RelaySuccessAcks,
				opts.RelayErrorAcks,
			),
		),
	)

	// clear stuck or failed ack txs so they are redelivered next run
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewProcessorMW(deps.Storage, NewRetryAckPacket(deps.Submitter, deps.Storage, route)), output)

	// assign terminal statuses
	output = pipeline.ProcessConcurrently(ctx, stageConcurrency,
		NewStateFinisher(deps.Storage, opts.RelaySuccessAcks, opts.RelayErrorAcks), output)

	return &Pipeline{input: input, output: output}
}

func (p *Pipeline) Push(_ context.Context, transfer *Transfer) bool {
	p.input <- transfer

	return true
}

func (p *Pipeline) Poll() (*Transfer, error) {
	transfer, ok := <-p.output
	if !ok {
		return nil, errors.New("pipeline closed")
	}

	return transfer, nil
}

func (p *Pipeline) Close() {
	close(p.input)
}
