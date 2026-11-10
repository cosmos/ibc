// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// ClearRelayTxStorage clears a recorded relay tx so it is resubmitted.
type ClearRelayTxStorage interface {
	ClearPacketRecvTx(ctx context.Context, key store.PacketKey) error
	ClearPacketAckTx(ctx context.Context, key store.PacketKey) error
	ClearPacketTimeoutTx(ctx context.Context, key store.PacketKey) error
}

// RetryRelayTx clears a recv, ack, or timeout tx that failed or stalled, so
// the packet is redelivered on the next run. An ack or timeout tx that landed
// but did not act on this packet is cleared too.
type RetryRelayTx struct {
	kind   v2.RelayKind
	leg    Leg
	status store.RelayStatus
	tx     func(tr *Transfer) (*string, *time.Time)
	clear  func(ctx context.Context, key store.PacketKey) error

	// the chain and client the relay tx is submitted to
	chainID, clientID string

	chains      ChainClients
	txSubmitter txsubmitter.TxSubmitter
}

// NewRetryRelayTx the retry stage for relay txs of the given kind.
func NewRetryRelayTx(
	kind v2.RelayKind,
	chains ChainClients,
	txSubmitter txsubmitter.TxSubmitter,
	storage ClearRelayTxStorage,
	route Route,
) RetryRelayTx {
	p := RetryRelayTx{
		kind:        kind,
		chainID:     route.SourceChainID,
		clientID:    route.SourceClientID,
		chains:      chains,
		txSubmitter: txSubmitter,
	}

	switch kind {
	case v2.RelayKindRecv:
		p.leg, p.status = LegSendToRecv, store.RelayStatusDeliverRecvPacket
		p.tx = func(tr *Transfer) (*string, *time.Time) { return tr.RecvTxHash, tr.RecvTxTime }
		p.clear = storage.ClearPacketRecvTx
		p.chainID, p.clientID = route.DestinationChainID, route.DestinationClientID
	case v2.RelayKindAck:
		p.leg, p.status = LegRecvToAck, store.RelayStatusDeliverAckPacket
		p.tx = func(tr *Transfer) (*string, *time.Time) { return tr.AckTxHash, tr.AckTxTime }
		p.clear = storage.ClearPacketAckTx
	case v2.RelayKindTimeout:
		p.leg, p.status = LegSendToTimeout, store.RelayStatusDeliverTimeoutPacket
		p.tx = func(tr *Transfer) (*string, *time.Time) { return tr.TimeoutTxHash, tr.TimeoutTxTime }
		p.clear = storage.ClearPacketTimeoutTx
	default:
		panic(fmt.Sprintf("no retry stage for relay kind %s", kind))
	}

	return p
}

func (p RetryRelayTx) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	hash, sentAt := p.tx(tr)
	if hash == nil || sentAt == nil {
		return nil, errors.Errorf("transfer has no %s tx details, violates ShouldProcess", p.kind)
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *hash, *sentAt)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if %s tx %s should be retried", p.kind, *hash)
	}

	if !retry {
		metrics.txConfirmed(ctx, p.chainID, p.clientID, *hash)

		executed, err := p.executed(ctx, tr, *hash)
		if err != nil {
			return nil, err
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

	// error so the transfer stops processing this run; it is picked up
	// without the relay tx and redelivered on the next run
	return nil, ErrRetryingRelayTx
}

// executed reports whether a confirmed relay tx acted on this packet.
func (p RetryRelayTx) executed(ctx context.Context, tr *Transfer, hash string) (bool, error) {
	// a confirmed recv tx counts as executed: the write ack stage that follows
	// reads its outcome, and chain clients only answer this for ack and timeout
	if p.kind == v2.RelayKindRecv {
		return true, nil
	}

	client, ok := p.chains.Get(p.chainID)
	if !ok {
		return false, errors.Errorf("no configured chain client for source chain %s", p.chainID)
	}

	executed, err := client.PacketRelayExecuted(ctx, hash, p.kind,
		tr.PacketSequenceNumber, tr.PacketSourceClientID, tr.PacketDestinationClientID)
	if err != nil {
		return false, errors.Wrapf(err, "checking %s event in tx %s", p.kind, hash)
	}

	return executed, nil
}

func (p RetryRelayTx) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, ErrRetryingRelayTx):
		hash, _ := p.tx(tr)
		tr.GetLogger().Warn("Retrying relay tx", "kind", p.kind.String(), "txHash", *hash)
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", p.kind.String())
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", p.kind.String(), "err", err)
	}
}

func (p RetryRelayTx) ShouldProcess(tr *Transfer) bool {
	hash, sentAt := p.tx(tr)
	if hash == nil || sentAt == nil {
		return false
	}

	// once the write ack is found the recv tx has done its job
	return p.kind != v2.RelayKindRecv || tr.WriteAckTxHash == nil
}

func (p RetryRelayTx) Status() store.RelayStatus {
	return p.status
}
