package processors

import (
	"context"
	"encoding/hex"
	"strings"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// BatchAckPacket delivers one ack tx on the source chain for a batch of
// transfers. Acks flow back toward the original source chain, so the proof
// api's source and destination are inverted.
type BatchAckPacket struct {
	chains    ChainClients
	storage   TxStorage
	proofAPI  proto.ProofApiServiceClient
	txManager txmgr.TxManager
	route     transfer.Route
}

func NewBatchAckPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofAPI proto.ProofApiServiceClient,
	txManager txmgr.TxManager,
	route transfer.Route,
) BatchAckPacket {
	return BatchAckPacket{
		chains:    chainClients,
		storage:   storage,
		proofAPI:  proofAPI,
		txManager: txManager,
		route:     route,
	}
}

func (p BatchAckPacket) Process(ctx context.Context, transfers []*transfer.Transfer) ([]*transfer.Transfer, error) {
	txSet := make(map[string]struct{})

	var txIDs [][]byte

	var sequences []uint64

	for _, tr := range transfers {
		if tr.WriteAckTxHash == nil {
			tr.ProcessingError = errors.New("trying to deliver ack packet without a write ack tx hash")

			continue
		}

		hash := *tr.WriteAckTxHash

		sequences = append(sequences, tr.PacketSequenceNumber)

		if _, ok := txSet[hash]; ok {
			continue
		}

		txID, err := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
		if err != nil {
			tr.ProcessingError = errors.Wrapf(err, "decoding tx hash %q", hash)

			continue
		}

		txIDs = append(txIDs, txID)
		txSet[hash] = struct{}{}
	}

	resp, err := p.proofAPI.RelayByTx(ctx, connect.NewRequest(&proto.RelayByTxRequest{
		SrcChain:           p.route.DestinationChainID,
		DstChain:           p.route.SourceChainID,
		SourceTxIds:        txIDs,
		SrcClientId:        p.route.DestinationClientID,
		DstClientId:        p.route.SourceClientID,
		DstPacketSequences: sequences,
	}))
	if err != nil {
		return nil, errors.Wrap(err, "getting relay tx from proof api")
	}

	client, ok := p.chains.Get(p.route.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.SourceChainID)
	}

	// the chain must be caught up to the current time before gas estimation
	// during delivery, or the tx reverts
	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()

	if errWait := client.WaitForChain(waitCtx); errWait != nil {
		return nil, errors.Wrap(errWait, "waiting for chain")
	}

	submission, err := p.txManager.Submit(ctx, v2.TxIntent{
		To:   resp.Msg.GetAddress(),
		Data: resp.Msg.GetTx(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "submitting relay tx")
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

func (p BatchAckPacket) Cancel(transfers []*transfer.Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch ack tx", "error", err)
	}
}

func (p BatchAckPacket) ShouldProcess(tr *transfer.Transfer) bool {
	if tr.WriteAckTxHash == nil {
		return false
	}

	return tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p BatchAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
