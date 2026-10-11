// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// PacketTransactionStorage clears failed or expired relay transactions.
type PacketTransactionStorage interface {
	ClearPacketRecvTx(context.Context, store.PacketKey) error
	ClearPacketAckTx(context.Context, store.PacketKey) error
	ClearPacketTimeoutTx(context.Context, store.PacketKey) error
}

// ErrSubmissionPending leaves the packet dispatchable while waiting for a transaction.
var ErrSubmissionPending = errors.New("waiting for packet submission or chain reconciliation")

// RetryPacket clears failed or expired transactions before another relay attempt.
type RetryPacket struct {
	source, destination txsubmitter.TxSubmitter
	storage             PacketTransactionStorage
}

func NewRetryPacket(source, destination txsubmitter.TxSubmitter, storage PacketTransactionStorage) RetryPacket {
	return RetryPacket{source: source, destination: destination, storage: storage}
}

func (p RetryPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	observations := []struct {
		hash      *string
		sentAt    *time.Time
		submitter txsubmitter.TxSubmitter
		clear     func(context.Context, store.PacketKey) error
		reset     func()
	}{
		{tr.RecvTxHash, tr.RecvTxTime, p.destination, p.storage.ClearPacketRecvTx, func() {
			tr.RecvTxHash, tr.RecvTxTime, tr.RecvTxRelayerAddress = nil, nil, nil
			tr.WriteAckTxHash, tr.WriteAckTxTime, tr.WriteAckStatus = nil, nil, nil
			tr.WriteAckTxFinalizedTime = nil
		}},
		{tr.AckTxHash, tr.AckTxTime, p.source, p.storage.ClearPacketAckTx, func() {
			tr.AckTxHash, tr.AckTxTime, tr.AckTxRelayerAddress = nil, nil, nil
		}},
		{tr.TimeoutTxHash, tr.TimeoutTxTime, p.source, p.storage.ClearPacketTimeoutTx, func() {
			tr.TimeoutTxHash, tr.TimeoutTxTime, tr.TimeoutTxRelayerAddress = nil, nil, nil
		}},
	}
	for _, observation := range observations {
		if observation.hash == nil {
			continue
		}
		if observation.sentAt == nil {
			return nil, errors.New("recorded transaction has no submission time")
		}
		retry, err := observation.submitter.ShouldRetry(ctx, *observation.hash, *observation.sentAt)
		if errors.Is(err, v2.ErrTxNotFound) {
			return nil, ErrSubmissionPending
		}
		if err != nil {
			return nil, err
		}
		if retry {
			if err = observation.clear(ctx, tr.Key()); err != nil {
				return nil, err
			}
			observation.reset()
			return nil, ErrSubmissionPending
		}
	}
	return tr, nil
}

func (p RetryPacket) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash != nil || tr.AckTxHash != nil || tr.TimeoutTxHash != nil
}

func (p RetryPacket) Status() store.RelayStatus { return store.RelayStatusPending }

func (p RetryPacket) Cancel(tr *Transfer, err error) {
	if !errors.Is(err, ErrSubmissionPending) {
		tr.GetLogger().Error("Checking packet transaction retries", "err", err)
	}
}
