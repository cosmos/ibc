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

// BatchRecvPacket delivers one recv tx on the destination chain for a batch
// of transfers.
type BatchRecvPacket struct {
	sourceChainClient      chains.Client
	destinationChainClient chains.Client
	route                  Route
	proofGen               proofgen.ProofGenerator
	txBuilder              txbuilder.TxBuilder
	txSubmitter            txsubmitter.TxSubmitter
	storage                TxStorage
}

func NewBatchRecvPacket(
	chainClients ChainClients,
	proofGenerators ProofGenerators,
	txBuilders TxBuilders,
	storage TxStorage,
	txSubmitter txsubmitter.TxSubmitter,
	route Route,
) (BatchRecvPacket, error) {
	sourceChainClient, ok := chainClients.Get(route.SourceChainID)
	if !ok {
		return BatchRecvPacket{}, errors.Errorf("no configured chain client for chain %s", route.SourceChainID)
	}

	destinationChainClient, ok := chainClients.Get(route.DestinationChainID)
	if !ok {
		return BatchRecvPacket{}, errors.Errorf("no configured chain client for chain %s", route.DestinationChainID)
	}

	proofGen, ok := proofGenerators.Get(route.DestinationChainID, route.DestinationClientID)
	if !ok {
		return BatchRecvPacket{}, errors.Errorf(
			"no proof generator configured for client %q on chain %q",
			route.DestinationClientID,
			route.DestinationChainID,
		)
	}

	txBuilder, ok := txBuilders.Get(route.DestinationChainID)
	if !ok {
		return BatchRecvPacket{}, errors.Errorf("no tx builder configured for chain %s", route.DestinationChainID)
	}

	return BatchRecvPacket{
		sourceChainClient:      sourceChainClient,
		destinationChainClient: destinationChainClient,
		route:                  route,
		proofGen:               proofGen,
		txBuilder:              txBuilder,
		txSubmitter:            txSubmitter,
		storage:                storage,
	}, nil
}

func (p BatchRecvPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofHeight, _, err := p.proofGen.LatestProvableHeight(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolving latest provable height")
	}

	var events []v2.PacketEvent

	for _, tr := range transfers {
		txID, errDecode := hex.DecodeString(strings.TrimPrefix(tr.SourceTxHash, "0x"))
		if errDecode != nil {
			tr.ProcessingError = errors.Wrapf(errDecode, "decoding tx hash %q", tr.SourceTxHash)

			continue
		}

		txEvents, errEvents := p.sourceChainClient.TxPacketEvents(ctx, txID)
		if errEvents != nil {
			tr.ProcessingError = errors.Wrapf(errEvents, "reading packet events for tx %s", tr.SourceTxHash)

			continue
		}

		event, errEvent := findPacketEvent(txEvents, tr.PacketSequenceNumber, tr.PacketSourceClientID, proofHeight)
		if errEvent != nil {
			tr.ProcessingError = errors.Wrapf(errEvent, "tx %s", tr.SourceTxHash)

			continue
		}

		events = append(events, event)
	}

	if len(events) == 0 {
		return transfers, nil
	}

	submission, err := relayPackets(
		ctx, p.destinationChainClient, p.proofGen, p.txBuilder, p.txSubmitter,
		p.route.DestinationClientID, v2.RelayKindRecv,
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

			if errRecord := repo.UpdatePacketRecvTx(ctx, tr.Key(), tx); errRecord != nil {
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

		tr.RecvTxHash = &tx.Hash
		tr.RecvTxTime = &tx.Time
		tr.RecvTxRelayerAddress = &tx.RelayerAddress
	}

	return transfers, nil
}

func (p BatchRecvPacket) Cancel(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch recv tx", "err", err)
	}
}

func (p BatchRecvPacket) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash == nil && !tr.IsTimedOut() && tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p BatchRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
