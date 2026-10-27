// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // the retry directions are structurally parallel by design
package processors

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// ClearTimeoutTxStorage clears a recorded relay tx so it is resubmitted.
type ClearTimeoutTxStorage interface {
	ClearPacketTimeoutTx(ctx context.Context, key store.PacketKey) error
}

// RetryTimeoutPacket clears transactions that failed, stalled, or did not time
// out this packet so it can be retried.
type RetryTimeoutPacket struct {
	chains      ChainClients
	txSubmitter txsubmitter.TxSubmitter
	storage     ClearTimeoutTxStorage
	route       Route
}

func NewRetryTimeoutPacket(
	chains ChainClients,
	txSubmitter txsubmitter.TxSubmitter,
	storage ClearTimeoutTxStorage,
	route Route,
) RetryTimeoutPacket {
	return RetryTimeoutPacket{chains: chains, txSubmitter: txSubmitter, storage: storage, route: route}
}

func (p RetryTimeoutPacket) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	if tr.TimeoutTxHash == nil || tr.TimeoutTxTime == nil {
		return nil, errors.New("transfer has no timeout tx details, violates ShouldProcess")
	}

	retry, err := p.txSubmitter.ShouldRetry(ctx, *tr.TimeoutTxHash, *tr.TimeoutTxTime)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if timeout tx %s should be retried", *tr.TimeoutTxHash)
	}

	if !retry {
		metrics.txConfirmed(ctx, p.route.SourceChainID, p.route.SourceClientID, *tr.TimeoutTxHash)
		client, ok := p.chains.Get(p.route.SourceChainID)
		if !ok {
			return nil, errors.Errorf("no configured chain client for source chain %s", p.route.SourceChainID)
		}
		executed, err := client.PacketRelayExecuted(ctx, *tr.TimeoutTxHash, v2.RelayKindTimeout,
			tr.PacketSequenceNumber, tr.PacketSourceClientID, tr.PacketDestinationClientID)
		if err != nil {
			return nil, errors.Wrapf(err, "checking timeout event in tx %s", *tr.TimeoutTxHash)
		}
		if executed {
			return tr, nil
		}
	}

	if err := p.storage.ClearPacketTimeoutTx(ctx, tr.Key()); err != nil {
		return nil, errors.Wrapf(err, "clearing timeout tx %s", *tr.TimeoutTxHash)
	}

	metrics.txRetry(ctx, tr, relayTypeSendToTimeout, *tr.TimeoutTxHash)
	metrics.packetExecution(ctx, tr, "timeout", executionFailed)

	return nil, ErrRetryingTimeoutPacket
}

func (p RetryTimeoutPacket) Cancel(tr *Transfer, err error) {
	switch {
	case errors.Is(err, ErrRetryingTimeoutPacket):
		tr.GetLogger().Warn("Retrying relay tx", "kind", "timeout", "txHash", *tr.TimeoutTxHash)
	case errors.Is(err, v2.ErrTxNotFound):
		tr.GetLogger().Debug("Relay tx not yet found on chain", "kind", "timeout")
	default:
		tr.GetLogger().Error("Checking relay tx retry", "kind", "timeout", "err", err)
	}
}

func (p RetryTimeoutPacket) ShouldProcess(tr *Transfer) bool {
	return tr.TimeoutTxHash != nil && tr.TimeoutTxTime != nil
}

func (p RetryTimeoutPacket) Status() store.RelayStatus {
	return store.RelayStatusDeliverTimeoutPacket
}
