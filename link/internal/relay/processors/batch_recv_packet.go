//nolint:dupl // the batch directions are structurally parallel by design
package processors

import (
	"context"
	"encoding/hex"
	"strings"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relay/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// BatchRecvPacket delivers one recv tx on the destination chain for a batch
// of transfers.
type BatchRecvPacket struct {
	chains    ChainClients
	storage   TxStorage
	proofAPI  proto.ProofApiServiceClient
	txManager txmgr.TxManager
	route     transfer.Route
}

func NewBatchRecvPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofAPI proto.ProofApiServiceClient,
	txManager txmgr.TxManager,
	route transfer.Route,
) BatchRecvPacket {
	return BatchRecvPacket{
		chains:    chainClients,
		storage:   storage,
		proofAPI:  proofAPI,
		txManager: txManager,
		route:     route,
	}
}

func (p BatchRecvPacket) Process(ctx context.Context, transfers []*transfer.Transfer) ([]*transfer.Transfer, error) {
	txSet := make(map[string]struct{})

	var txIDs [][]byte

	var sequences []uint64

	for _, tr := range transfers {
		hash := tr.SourceTxHash

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
		SrcChain:           p.route.SourceChainID,
		DstChain:           p.route.DestinationChainID,
		SourceTxIds:        txIDs,
		SrcClientId:        p.route.SourceClientID,
		DstClientId:        p.route.DestinationClientID,
		SrcPacketSequences: sequences,
	}))
	if err != nil {
		return nil, errors.Wrap(err, "getting relay tx from proof api")
	}

	client, ok := p.chains.Get(p.route.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", p.route.DestinationChainID)
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

func (p BatchRecvPacket) Cancel(transfers []*transfer.Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch recv tx", "error", err)
	}
}

func (p BatchRecvPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.RecvTxHash == nil && !tr.IsTimedOut()
}

func (p BatchRecvPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverRecvPacket
}
