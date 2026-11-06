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

// ClearAckTxStorage clears a recorded relay tx so it is resubmitted.
type ClearAckTxStorage interface {
	ClearPacketAckTx(ctx context.Context, key store.PacketKey) error
}

// ClearTimeoutTxStorage clears a recorded relay tx so it is resubmitted.
type ClearTimeoutTxStorage interface {
	ClearPacketTimeoutTx(ctx context.Context, key store.PacketKey) error
}

// RetrySourceRelay clears an ack or timeout tx, both delivered to the source
// chain, that failed, stalled, or did not act on this packet so it is retried.
type RetrySourceRelay struct {
	kind        v2.RelayKind
	leg         Leg
	status      store.RelayStatus
	retrying    error
	tx          func(tr *Transfer) (*string, *time.Time)
	clear       func(ctx context.Context, key store.PacketKey) error
	chains      ChainClients
	txSubmitter txsubmitter.TxSubmitter
	route       Route
}

func NewRetryAckPacket(
	chains ChainClients,
	txSubmitter txsubmitter.TxSubmitter,
	storage ClearAckTxStorage,
	route Route,
) RetrySourceRelay {
	return RetrySourceRelay{
		kind:        v2.RelayKindAck,
		leg:         LegRecvToAck,
		status:      store.RelayStatusDeliverAckPacket,
		retrying:    ErrRetryingAckPacket,
		tx:          func(tr *Transfer) (*string, *time.Time) { return tr.AckTxHash, tr.AckTxTime },
		clear:       storage.ClearPacketAckTx,
		chains:      chains,
		txSubmitter: txSubmitter,
		route:       route,
	}
}

func NewRetryTimeoutPacket(
	chains ChainClients,
	txSubmitter txsubmitter.TxSubmitter,
	storage ClearTimeoutTxStorage,
	route Route,
) RetrySourceRelay {
	return RetrySourceRelay{
		kind:        v2.RelayKindTimeout,
		leg:         LegSendToTimeout,
		status:      store.RelayStatusDeliverTimeoutPacket,
		retrying:    ErrRetryingTimeoutPacket,
		tx:          func(tr *Transfer) (*string, *time.Time) { return tr.TimeoutTxHash, tr.TimeoutTxTime },
		clear:       storage.ClearPacketTimeoutTx,
		chains:      chains,
		txSubmitter: txSubmitter,
		route:       route,
	}
}

func (p RetrySourceRelay) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	hash, sentAt := p.tx(tr)
	if hash == nil || sentAt == nil {
		return nil, errors.Errorf("transfer has no %s tx details, violates ShouldProcess", p.kind)
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *hash, *sentAt)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if %s tx %s should be retried", p.kind, *hash)
	}

	if !retry {
		metrics.txConfirmed(ctx, p.route.SourceChainID, p.route.SourceClientID, *hash)
		client, ok := p.chains.Get(p.route.SourceChainID)
		if !ok {
			return nil, errors.Errorf("no configured chain client for source chain %s", p.route.SourceChainID)
		}
		executed, err := client.PacketRelayExecuted(ctx, *hash, p.kind,
			tr.PacketSequenceNumber, tr.PacketSourceClientID, tr.PacketDestinationClientID)
		if err != nil {
			return nil, errors.Wrapf(err, "checking %s event in tx %s", p.kind, *hash)
		}
		if executed {
			return tr, nil
		}
	}

	if err := p.clear(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing %s tx %s", p.kind, *hash)
	}

	metrics.txRetry(ctx, tr, p.leg, *hash)
	metrics.packetExecution(ctx, tr, p.kind.String(), executionFailed)

	return nil, p.retrying
}

func (p RetrySourceRelay) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, p.retrying):
		hash, _ := p.tx(tr)
		tr.GetLogger().Warn("Retrying relay tx", "kind", p.kind.String(), "txHash", *hash)
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", p.kind.String())
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", p.kind.String(), "err", err)
	}
}

func (p RetrySourceRelay) ShouldProcess(tr *Transfer) bool {
	hash, sentAt := p.tx(tr)
	return hash != nil && sentAt != nil
}

func (p RetrySourceRelay) Status() store.RelayStatus {
	return p.status
}
