package processors

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
)

// waitForChainTimeout bounds how long batch delivery waits for the target
// chain to catch up to the current time before gas estimation.
const waitForChainTimeout = 2 * time.Minute

// TxStorage persists batch delivery results transactionally.
type TxStorage interface {
	Transact(ctx context.Context, fn func(store.Repository) error) error
}

// batchDeps the shared dependencies of the three batch processors.
type batchDeps struct {
	chains    ChainClients
	storage   TxStorage
	proofAPI  proto.ProofApiServiceClient
	submitter txmgr.Submitter
	route     transfer.Route
}

// collectTxIDs gathers the unique tx ids and all sequences for a batch,
// poisoning transfers whose hash cannot be decoded.
func collectTxIDs(
	transfers []*transfer.Transfer,
	txHash func(*transfer.Transfer) (string, error),
) (txIDs [][]byte, sequences []uint64) {
	txSet := make(map[string]struct{})

	for _, tr := range transfers {
		hash, err := txHash(tr)
		if err != nil {
			tr.ProcessingError = err

			continue
		}

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

	return txIDs, sequences
}

// processBatch requests relay tx bytes from the proof api, submits them on
// the target chain, and records the resulting tx on every healthy tr.
func (d batchDeps) processBatch(
	ctx context.Context,
	transfers []*transfer.Transfer,
	req *proto.RelayByTxRequest,
	targetChainID string,
	record func(repo store.Repository, key store.PacketKey, tx store.PacketTx) error,
	apply func(tr *transfer.Transfer, tx store.PacketTx),
) ([]*transfer.Transfer, error) {
	resp, err := d.proofAPI.RelayByTx(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, errors.Wrap(err, "getting relay tx from proof api")
	}

	client, ok := d.chains.Get(targetChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %s", targetChainID)
	}

	// the chain must be caught up to the current time before gas estimation
	// during delivery, or the tx reverts
	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()

	if errWait := client.WaitForChain(waitCtx); errWait != nil {
		return nil, errors.Wrap(errWait, "waiting for chain")
	}

	submission, err := d.submitter.Submit(ctx, targetChainID, txmgr.TxIntent{
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

	err = d.storage.Transact(ctx, func(repo store.Repository) error {
		for _, tr := range transfers {
			if tr.ProcessingError != nil {
				continue
			}

			if errRecord := record(repo, tr.Key(), tx); errRecord != nil {
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

		apply(tr, tx)
	}

	return transfers, nil
}
