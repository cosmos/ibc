//nolint:dupl // the retry directions are structurally parallel by design
package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/relayer/transfer"
	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// RetryAckPacket clears a stuck or failed ack tx so the ack is redelivered on
// the next run.
type RetryAckPacket struct {
	submitter txmgr.Submitter
	storage   ClearTxStorage
	route     transfer.Route
}

func NewRetryAckPacket(submitter txmgr.Submitter, storage ClearTxStorage, route transfer.Route) RetryAckPacket {
	return RetryAckPacket{submitter: submitter, storage: storage, route: route}
}

func (p RetryAckPacket) Process(ctx context.Context, tr *transfer.Transfer) (*transfer.Transfer, error) {
	if tr.AckTxHash == nil || tr.AckTxTime == nil {
		return nil, errors.New("transfer has no ack tx details, violates ShouldProcess")
	}

	retry, err := p.submitter.ShouldRetry(
		ctx,
		p.route.SourceChainID,
		*tr.AckTxHash,
		RetryAckExpiry,
		*tr.AckTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if ack tx %s should be retried", *tr.AckTxHash)
	}

	if !retry {
		return tr, nil
	}

	if err := p.storage.ClearPacketAckTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing ack tx %s", *tr.AckTxHash)
	}

	return nil, transfer.ErrRetryingAckPacket
}

func (p RetryAckPacket) Cancel(tr *transfer.Transfer, err error) {
	cancelRetry(tr, err, transfer.ErrRetryingAckPacket, "ack")
}

func (p RetryAckPacket) ShouldProcess(tr *transfer.Transfer) bool {
	return tr.AckTxHash != nil && tr.AckTxTime != nil
}

func (p RetryAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
