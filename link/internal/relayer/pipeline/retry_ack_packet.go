//nolint:dupl // the retry directions are structurally parallel by design
package pipeline

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
	"github.com/cosmos/ibc/link/internal/txmgr"
)

// RetryAckPacket clears a stuck or failed ack tx so the ack is redelivered on
// the next run.
type RetryAckPacket struct {
	submitter txmgr.Submitter
	storage   ClearTxStorage
	route     Route
}

func NewRetryAckPacket(submitter txmgr.Submitter, storage ClearTxStorage, route Route) RetryAckPacket {
	return RetryAckPacket{submitter: submitter, storage: storage, route: route}
}

func (p RetryAckPacket) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	if transfer.AckTxHash == nil || transfer.AckTxTime == nil {
		return nil, errors.New("transfer has no ack tx details, violates ShouldProcess")
	}

	retry, err := p.submitter.ShouldRetry(
		ctx,
		p.route.SourceChainID,
		*transfer.AckTxHash,
		RetryAckExpiry,
		*transfer.AckTxTime,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if ack tx %s should be retried", *transfer.AckTxHash)
	}

	if !retry {
		return transfer, nil
	}

	if err := p.storage.ClearPacketAckTx(ctx, transfer.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing ack tx %s", *transfer.AckTxHash)
	}

	return nil, ErrRetryingAckPacket
}

func (p RetryAckPacket) Cancel(transfer *Transfer, err error) {
	cancelRetry(transfer, err, ErrRetryingAckPacket, "ack")
}

func (p RetryAckPacket) ShouldProcess(transfer *Transfer) bool {
	return transfer.AckTxHash != nil && transfer.AckTxTime != nil
}

func (p RetryAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
