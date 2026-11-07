// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // the retry directions are structurally parallel by design
package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// ClearAckTxStorage clears a recorded relay tx so it is resubmitted.
type ClearAckTxStorage interface {
	ClearPacketAckTx(ctx context.Context, key store.PacketKey) error
}

// RetryAckPacket clears transactions that failed, stalled, or did not acknowledge
// this packet so it can be retried.
type RetryAckPacket struct {
	source      chains.Client
	txSubmitter txsubmitter.TxSubmitter
	storage     ClearAckTxStorage
	route       Route
}

func NewRetryAckPacket(route RouteDeps, storage ClearAckTxStorage) RetryAckPacket {
	return RetryAckPacket{
		source: route.Source.Chain, txSubmitter: route.Source.TxSubmitter, storage: storage, route: route.Route,
	}
}

func (p RetryAckPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	if tr.AckTxHash == nil || tr.AckTxTime == nil {
		return nil, errors.Wrap(ErrInvariant, "transfer has no ack tx details, violates ShouldProcess")
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *tr.AckTxHash, *tr.AckTxTime)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if ack tx %s should be retried", *tr.AckTxHash)
	}

	if !retry {
		metrics.txConfirmed(ctx, p.route.SourceChainID, p.route.SourceClientID, *tr.AckTxHash)
		executed, err := p.source.PacketRelayExecuted(ctx, *tr.AckTxHash, v2.RelayKindAck,
			tr.PacketSequenceNumber, tr.PacketSourceClientID, tr.PacketDestinationClientID)
		if err != nil {
			return nil, errors.Wrapf(err, "checking ack event in tx %s", *tr.AckTxHash)
		}
		if executed {
			return tr, nil
		}
	}

	if err := p.storage.ClearPacketAckTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing ack tx %s", *tr.AckTxHash)
	}

	metrics.txRetry(ctx, tr, relayTypeRecvToAck, *tr.AckTxHash)
	metrics.packetExecution(ctx, tr, "ack", executionFailed)

	return nil, ErrRetryingAckPacket
}

func (p RetryAckPacket) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, ErrRetryingAckPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "ack", "txHash", *tr.AckTxHash)
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "ack")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "ack", "err", err)
	}
}

func (p RetryAckPacket) ShouldProcess(tr *Transfer) bool {
	return tr.AckTxHash != nil && tr.AckTxTime != nil
}

func (p RetryAckPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverAckPacket
}
