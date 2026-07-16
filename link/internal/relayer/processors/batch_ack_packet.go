package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
)

// BatchAckPacket delivers one ack tx on the source chain for a batch of
// transfers.
type BatchAckPacket struct {
	batchDeps

	relaySuccessAcks bool
	relayErrorAcks   bool
}

func NewBatchAckPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofAPI proto.ProofApiServiceClient,
	submitter txmgr.Submitter,
	route transfer.Route,
	relaySuccessAcks, relayErrorAcks bool,
) BatchAckPacket {
	return BatchAckPacket{
		batchDeps: batchDeps{
			chains:    chainClients,
			storage:   storage,
			proofAPI:  proofAPI,
			submitter: submitter,
			route:     route,
		},
		relaySuccessAcks: relaySuccessAcks,
		relayErrorAcks:   relayErrorAcks,
	}
}

func (p BatchAckPacket) Process(ctx context.Context, transfers []*transfer.Transfer) ([]*transfer.Transfer, error) {
	txIDs, sequences := collectTxIDs(transfers, func(t *transfer.Transfer) (string, error) {
		if t.WriteAckTxHash == nil {
			return "", errors.New("trying to deliver ack packet without a write ack tx hash")
		}

		return *t.WriteAckTxHash, nil
	})

	// acks flow back toward the original source chain, so the proof api's
	// source and destination are inverted
	out, err := p.processBatch(ctx, transfers, &proto.RelayByTxRequest{
		SrcChain:           p.route.DestinationChainID,
		DstChain:           p.route.SourceChainID,
		SourceTxIds:        txIDs,
		SrcClientId:        p.route.DestinationClientID,
		DstClientId:        p.route.SourceClientID,
		DstPacketSequences: sequences,
	}, p.route.SourceChainID,
		func(repo store.Repository, key store.PacketKey, tx store.PacketTx) error {
			return repo.UpdatePacketAckTx(ctx, key, tx)
		},
		func(tr *transfer.Transfer, tx store.PacketTx) {
			tr.AckTxHash = &tx.Hash
			tr.AckTxTime = &tx.Time
			tr.AckTxRelayerAddress = &tx.RelayerAddress
		})
	if err != nil {
		return nil, errors.Wrapf(err, "delivering ack tx for batch of %d transfers", len(sequences))
	}

	return out, nil
}

func (p BatchAckPacket) Cancel(transfers []*transfer.Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch ack tx", "error", err)
	}
}

func (p BatchAckPacket) ShouldProcess(tr *transfer.Transfer) bool {
	if tr.WriteAckTxHash == nil {
		return false
	}

	if tr.WriteAckStatus == nil {
		tr.GetLogger().Warn("This is a bug! Transfer has a write ack tx hash but no write ack status")

		return false
	}

	if (transfer.IsErrorAck(*tr.WriteAckStatus) && !p.relayErrorAcks) ||
		(transfer.IsSuccessAck(*tr.WriteAckStatus) && !p.relaySuccessAcks) {
		return false
	}

	return tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p BatchAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
