package processors

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// WriteAckStorage persists write ack details and clears recv txs.
type WriteAckStorage interface {
	UpdatePacketWriteAck(ctx context.Context, key store.PacketKey, ack store.WriteAck) error
	ClearPacketRecvTx(ctx context.Context, key store.PacketKey) error
}

// WaitForWriteAck extracts the write acknowledgement from the recv tx. Only
// synchronous acks are supported: the write ack is expected in the recv tx.
type WaitForWriteAck struct {
	chains  ChainClients
	storage WriteAckStorage
}

func NewWaitForWriteAck(chainClients ChainClients, storage WriteAckStorage) WaitForWriteAck {
	return WaitForWriteAck{chains: chainClients, storage: storage}
}

func (p WaitForWriteAck) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(tr.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", tr.DestinationChainID)
	}

	if tr.RecvTxHash == nil {
		return nil, errors.New("transfer has no recv tx hash, violates ShouldProcess")
	}

	recvTxHash := *tr.RecvTxHash

	ackStatus, err := client.PacketWriteAckStatus(
		ctx,
		recvTxHash,
		tr.PacketSequenceNumber,
		tr.PacketSourceClientID,
		tr.PacketDestinationClientID,
	)
	switch {
	case errors.Is(err, v2.ErrWriteAckNotFoundForPacket):
		// the recv tx exists but does not contain this packet's write ack.
		// this happens when a packet times out while its batch accumulates:
		// clear the recv tx and error so the packet is retried and then timed
		// out on the next run.
		if errClear := p.storage.ClearPacketRecvTx(ctx, tr.Key()); errClear != nil {
			return nil, errors.Wrapf(errClear, "clearing recv tx %s", recvTxHash)
		}

		return nil, errors.Errorf("write ack for transfer not found in recv tx %s", recvTxHash)
	case errors.Is(err, v2.ErrWriteAckDecoding):
		// non-standard acknowledgement formats cannot be classified; record
		// the ack with an unknown status
		tr.GetLogger().Warn("Could not decode write ack, status is unknown", "err", err)
		ackStatus = v2.WriteAckStatusUnknown
	case err != nil:
		return nil, errors.Wrapf(err, "finding write ack in recv tx %s", recvTxHash)
	}

	status := writeAckStatusFromV2(ackStatus)

	// the write ack shares the recv tx, so it shares its time
	var writeAckTime time.Time
	if tr.RecvTxTime == nil {
		// a recv tx hash without a time is a bug, but not worth failing over
		tr.GetLogger().Error("Transfer has a recv tx hash but no recv tx time", "recvTxHash", recvTxHash)
	} else {
		writeAckTime = *tr.RecvTxTime
	}

	ack := store.WriteAck{TxHash: recvTxHash, TxTime: writeAckTime, Status: status}
	if err := p.storage.UpdatePacketWriteAck(ctx, tr.Key(), ack); err != nil {
		return nil, errors.Wrapf(err, "recording write ack from tx %s", recvTxHash)
	}

	tr.WriteAckTxHash = &recvTxHash
	tr.WriteAckTxTime = &ack.TxTime
	tr.WriteAckStatus = &status

	return tr, nil
}

func (p WaitForWriteAck) Cancel(tr *Transfer, err error) {
	if errors.Is(err, v2.ErrTxNotFound) {
		tr.GetLogger().Debug("Write ack tx not yet found on chain")

		return
	}

	tr.GetLogger().Error("Waiting for write ack", "err", err)
}

func (p WaitForWriteAck) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash != nil && tr.WriteAckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p WaitForWriteAck) Status() store.RelayStatus {
	return store.RelayStatusWaitForWriteAck
}

func writeAckStatusFromV2(status v2.WriteAckStatus) store.WriteAckStatus {
	switch status {
	case v2.WriteAckStatusSuccess:
		return store.WriteAckStatusSuccess
	case v2.WriteAckStatusError:
		return store.WriteAckStatusError
	default:
		return store.WriteAckStatusUnknown
	}
}
