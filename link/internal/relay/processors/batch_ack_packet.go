//nolint:dupl // the batch directions are structurally parallel by design
package processors

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// BatchAckPacket delivers one ack tx on the source chain for a batch of
// transfers.
type BatchAckPacket struct {
	destinationChainClient chains.Client
	sourceChainClient      chains.Client
	route                  Route
	proofGen               proofgen.ProofGenerator
	txBuilder              txbuilder.TxBuilder
	txSubmitter            txsubmitter.TxSubmitter
	storage                TxStorage
}

func NewBatchAckPacket(
	chainClients ChainClients,
	proofGenerators ProofGenerators,
	txBuilders TxBuilders,
	storage TxStorage,
	txSubmitter txsubmitter.TxSubmitter,
	route Route,
) (BatchAckPacket, error) {
	destinationChainClient, ok := chainClients.Get(route.DestinationChainID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf("no configured chain client for chain %s", route.DestinationChainID)
	}

	sourceChainClient, ok := chainClients.Get(route.SourceChainID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf("no configured chain client for chain %s", route.SourceChainID)
	}

	proofGen, ok := proofGenerators.Get(route.SourceChainID, route.SourceClientID)
	if !ok {
		return BatchAckPacket{}, errors.Errorf(
			"no proof generator configured for client %q on chain %q", route.SourceClientID, route.SourceChainID,
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
		proofGen:               proofGen,
		txBuilder:              txBuilder,
		txSubmitter:            txSubmitter,
		storage:                storage,
	}, nil
}

func (p BatchAckPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofHeight, _, err := p.proofGen.LatestProvableHeight(ctx)
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

	submission, err := relayPackets(
		ctx, p.sourceChainClient, p.proofGen, p.txBuilder, p.txSubmitter,
		p.route.SourceClientID, v2.RelayKindAck,
		proofHeight, events,
	)
	if err != nil {
		return nil, err
	}

	tx := store.PacketTx{
		Hash:           submission.TxHash,
		Time:           submission.SubmittedAt,
		RelayerAddress: submission.RelayerAddress,
	}

	err = p.storage.Transact(ctx, func(repo store.Repository) error {
		for _, tr := range transfers {
			if tr.ProcessingError != nil {
				continue
			}

			if errRecord := repo.UpdatePacketAckTx(ctx, tr.Key(), tx); errRecord != nil {
				return errors.Wrapf(
					errRecord,
					"recording relay tx %s for sequence %d",
					tx.Hash,
					tr.PacketSequenceNumber,
				)
			}
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "recording batch relay txs")
	}

	for _, tr := range transfers {
		if tr.ProcessingError != nil {
			continue
		}

		tr.AckTxHash = &tx.Hash
		tr.AckTxTime = &tx.Time
		tr.AckTxRelayerAddress = &tx.RelayerAddress
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
