// SPDX-License-Identifier: Apache-2.0

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

// BatchTimeoutPacket delivers one timeout tx on the source chain for a batch
// of transfers
type BatchTimeoutPacket struct {
	sourceChainClient chains.Client
	route             Route
	proofGen          proofgen.ProofGenerator
	txBuilder         txbuilder.TxBuilder
	txSubmitter       txsubmitter.TxSubmitter
	storage           TxStorage
}

func NewBatchTimeoutPacket(
	chainClients ChainClients,
	proofGenerators ProofGenerators,
	txBuilders TxBuilders,
	storage TxStorage,
	txSubmitter txsubmitter.TxSubmitter,
	route Route,
) (BatchTimeoutPacket, error) {
	sourceChainClient, ok := chainClients.Get(route.SourceChainID)
	if !ok {
		return BatchTimeoutPacket{}, errors.Errorf("no configured chain client for chain %s", route.SourceChainID)
	}

	proofGen, ok := proofGenerators.Get(route.SourceChainID, route.SourceClientID)
	if !ok {
		return BatchTimeoutPacket{}, errors.Errorf(
			"no proof generator configured for client %q on chain %q", route.SourceClientID, route.SourceChainID,
		)
	}

	txBuilder, ok := txBuilders.Get(route.SourceChainID)
	if !ok {
		return BatchTimeoutPacket{}, errors.Errorf("no tx builder configured for chain %s", route.SourceChainID)
	}

	return BatchTimeoutPacket{
		sourceChainClient: sourceChainClient,
		route:             route,
		proofGen:          proofGen,
		txBuilder:         txBuilder,
		txSubmitter:       txSubmitter,
		storage:           storage,
	}, nil
}

func (p BatchTimeoutPacket) Process(ctx context.Context, transfers []*Transfer) ([]*Transfer, error) {
	proofHeight, timestamp, err := p.proofGen.LatestProvableHeight(ctx)
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

		// The send event supplies packet data; its source-chain height is
		// unrelated to the destination-chain timeout proof height.
		sendEvent, errEvent := findPacketEvent(txEvents, tr.PacketSequenceNumber, tr.PacketSourceClientID)
		if errEvent != nil {
			tr.ProcessingError = errors.Wrapf(errEvent, "tx %s", tr.SourceTxHash)

			continue
		}

		if tr.PacketTimeoutTimestamp.After(timestamp) {
			tr.ProcessingError = errors.Errorf(
				"packet timeout %s is after the currently provable timestamp %s", tr.PacketTimeoutTimestamp, timestamp,
			)

			continue
		}

		events = append(events, sendEvent)
	}

	if len(events) == 0 {
		return transfers, nil
	}

	submission, err := relayPackets(
		ctx, p.sourceChainClient, p.proofGen, p.txBuilder, p.txSubmitter,
		p.route.SourceClientID, v2.RelayKindTimeout,
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

			if errRecord := repo.UpdatePacketTimeoutTx(ctx, tr.Key(), tx); errRecord != nil {
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

		tr.TimeoutTxHash = &tx.Hash
		tr.TimeoutTxTime = &tx.Time
		tr.TimeoutTxRelayerAddress = &tx.RelayerAddress
	}

	return transfers, nil
}

func (p BatchTimeoutPacket) Cancel(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch timeout tx", "err", err)
	}
}

func (p BatchTimeoutPacket) ShouldProcess(tr *Transfer) bool {
	shouldBeTimedOut := tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil

	return shouldBeTimedOut && tr.TimeoutTxHash == nil
}

func (p BatchTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
