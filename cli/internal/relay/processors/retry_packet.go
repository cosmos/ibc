// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// ErrSubmissionPending leaves the packet dispatchable while its submission is reconciled.
var ErrSubmissionPending = errors.New("waiting for packet submission or chain reconciliation")

// RetryPacket checks submission ownership before the packet enters a batch.
type RetryPacket struct {
	submitter     txsubmitter.TxSubmitter
	operationType v2.OperationType
}

func NewRetryPacket(s txsubmitter.TxSubmitter, operationType v2.OperationType) RetryPacket {
	return RetryPacket{submitter: s, operationType: operationType}
}

func (p RetryPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	op := v2.OperationMetadata{
		SourceChainID:  tr.SourceChainID,
		SourceClientID: tr.PacketSourceClientID,
		Sequence:       tr.PacketSequenceNumber,
		Type:           p.operationType,
	}
	pending, err := p.submitter.HasPending(ctx, op)
	if err != nil {
		return nil, errors.Wrap(err, "checking packet submission")
	}
	if pending {
		return nil, ErrSubmissionPending
	}
	return tr, nil
}

func (p RetryPacket) ShouldProcess(tr *Transfer) bool {
	switch p.operationType {
	case v2.OperationTypeRecv:
		return tr.WriteAckTxHash == nil && tr.AckTxHash == nil && tr.TimeoutTxHash == nil &&
			(!tr.IsTimedOut() || tr.RecvTxHash != nil)
	case v2.OperationTypeAck:
		return tr.WriteAckTxHash != nil && tr.TimeoutTxHash == nil
	default:
		return tr.IsTimedOut() && tr.RecvTxHash == nil && tr.AckTxHash == nil
	}
}

func (p RetryPacket) Cancel(tr *Transfer, err error) {
	if errors.Is(err, ErrSubmissionPending) {
		tr.GetLogger().Debug("Waiting for packet submission", "kind", p.operationType)
		return
	}
	tr.GetLogger().Error("Checking packet submission", "kind", p.operationType, "err", err)
}

func (p RetryPacket) Status() store.RelayStatus {
	switch p.operationType {
	case v2.OperationTypeRecv:
		return store.RelayStatusDeliverRecvPacket
	case v2.OperationTypeAck:
		return store.RelayStatusDeliverAckPacket
	default:
		return store.RelayStatusDeliverTimeoutPacket
	}
}
