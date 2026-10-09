// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // the batch directions are structurally parallel by design
package processors

import (
	"context"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// BatchAckPacket delivers one ack tx on the source chain for a batch of
// transfers.
type BatchAckPacket struct {
	destinationChainClient chains.Client
	sourceChainClient      chains.Client
	route                  Route
	prover                 prover.Prover
	txBuilder              txbuilder.TxBuilder
	txSubmitter            txsubmitter.TxSubmitter
	logger                 *slog.Logger
}

func NewBatchAckPacket(
	chainClients ChainClients,
	provers Provers,
	txBuilders TxBuilders,
	txSubmitter txsubmitter.TxSubmitter,
	route Route,
	logger *slog.Logger,
) (BatchAckPacket, error) {
	destinationChainClient, ok := chainClients.Get(route.DestinationChainID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf("no configured chain client for chain %s", route.DestinationChainID)
	}

	sourceChainClient, ok := chainClients.Get(route.SourceChainID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf("no configured chain client for chain %s", route.SourceChainID)
	}

	prover, ok := provers.Get(route.SourceChainID, route.SourceClientID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf(
			"no prover configured for client %q on chain %q", route.SourceClientID, route.SourceChainID,
		)
	}

	txBuilder, ok := txBuilders.Get(route.SourceChainID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf("no tx builder configured for chain %s", route.SourceChainID)
	}

	return BatchAckPacket{
		destinationChainClient: destinationChainClient,
		sourceChainClient:      sourceChainClient,
		route:                  route,
		prover:                 prover,
		txBuilder:              txBuilder,
		txSubmitter:            txSubmitter,
		logger:                 logger,
	}, nil
}

func (p BatchAckPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofHeight, _, err := p.prover.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	var events []v2.PacketEvent

	for _, tr := range transfers {
		if tr.WriteAckTxHash == nil {
			tr.ProcessingError = errors.New("missing required tx hash")

			continue
		}

		hash := *tr.WriteAckTxHash

		txID, errDecode := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
		if errDecode != nil {
			tr.ProcessingError = errors.Wrapf(errDecode, "decoding tx hash %q", hash)

			continue
		}

		txEvents, errEvents := p.destinationChainClient.TxPacketEvents(ctx, txID)
		if errEvents != nil {
			tr.ProcessingError = errors.Wrapf(errEvents, "reading packet events for tx %s", hash)

			continue
		}

		event, errEvent := findPacketEventAtOrBeforeHeight(
			txEvents,
			tr.PacketSequenceNumber,
			tr.PacketSourceClientID,
			proofHeight,
		)
		if errEvent != nil {
			tr.ProcessingError = errors.Wrapf(errEvent, "tx %s", hash)

			continue
		}

		events = append(events, event)
	}

	if len(events) == 0 {
		return transfers, nil
	}

	err = relayPackets(
		ctx, p.logger, p.sourceChainClient, p.prover, p.txBuilder, p.txSubmitter,
		p.route.SourceClientID, p.route.SourceChainID, v2.OperationTypeAck,
		proofHeight, events,
	)
	if err != nil {
		return nil, err
	}
	for _, tr := range transfers {
		if tr.ProcessingError == nil {
			tr.ProcessingError = ErrSubmissionPending
		}
	}

	return transfers, nil
}

func (p BatchAckPacket) Cancel(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch ack tx", "err", err)
	}
}

func (p BatchAckPacket) ShouldProcess(tr *Transfer) bool {
	if tr.WriteAckTxHash == nil {
		return false
	}

	return tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p BatchAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
