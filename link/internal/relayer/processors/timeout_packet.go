//nolint:dupl // the batch directions are structurally parallel by design
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
)

// BatchTimeoutPacket delivers one timeout tx on the source chain for a batch
// of transfers. Timeouts flow back toward the original source chain, so the
// proof api's source and destination are inverted.
type BatchTimeoutPacket struct {
	chains    ChainClients
	storage   TxStorage
	proofAPI  proto.ProofApiServiceClient
	submitter txmgr.Submitter
	route     transfer.Route
}

func NewBatchTimeoutPacket(
	chainClients ChainClients,
	storage TxStorage,
	proofAPI proto.ProofApiServiceClient,
	submitter txmgr.Submitter,
	route transfer.Route,
) BatchTimeoutPacket {
	return BatchTimeoutPacket{
		chains:    chainClients,
		storage:   storage,
		proofAPI:  proofAPI,
		submitter: submitter,
		route:     route,
	}
}

func (p BatchTimeoutPacket) Process(ctx context.Context, transfers []*transfer.Transfer) ([]*transfer.Transfer, error) {
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
		SrcChain:           p.route.DestinationChainID,
		DstChain:           p.route.SourceChainID,
		TimeoutTxIds:       txIDs,
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

	submission, err := p.submitter.Submit(ctx, txmgr.TxIntent{
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

func (p BatchTimeoutPacket) Cancel(transfers []*transfer.Transfer, err error) {
	for _, tr := range transfers {
		tr.GetLogger().Error("Delivering batch timeout tx", "error", err)
	}
}

func (p BatchTimeoutPacket) ShouldProcess(tr *transfer.Transfer) bool {
	shouldBeTimedOut := tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil

	return shouldBeTimedOut && tr.TimeoutTxHash == nil
}

func (p BatchTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
