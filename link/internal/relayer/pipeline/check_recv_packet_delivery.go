package pipeline

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/store"
)

// CheckRecvPacketDelivery populates the recv tx details when another relayer
// already delivered the packet on the destination chain.
type CheckRecvPacketDelivery struct {
	chains  ChainClients
	storage RecvTxStorage
}

// RecvTxStorage persists recv tx details.
type RecvTxStorage interface {
	UpdatePacketRecvTx(ctx context.Context, key store.PacketKey, tx store.PacketTx) error
}

func NewCheckRecvPacketDelivery(chainClients ChainClients, storage RecvTxStorage) CheckRecvPacketDelivery {
	return CheckRecvPacketDelivery{chains: chainClients, storage: storage}
}

func (p CheckRecvPacketDelivery) Process(ctx context.Context, transfer *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(transfer.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", transfer.DestinationChainID)
	}

	received, err := client.IsPacketReceived(ctx, transfer.PacketDestinationClientID, transfer.PacketSequenceNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "checking packet receipt on destination chain %s", transfer.DestinationChainID)
	}

	if !received {
		// not received; continue to deliver the recv packet as normal
		return transfer, nil
	}

	transfer.GetLogger().Info("Packet already received on destination chain, searching for the recv tx")

	recvTx, err := client.FindRecvTx(ctx, transfer.PacketDestinationClientID, transfer.PacketSequenceNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "finding recv tx on destination chain %s", transfer.DestinationChainID)
	}

	tx := store.PacketTx{Hash: recvTx.Hash, Time: recvTx.Timestamp, RelayerAddress: recvTx.RelayerAddress}
	if err := p.storage.UpdatePacketRecvTx(ctx, transfer.Key(), tx); err != nil {
		return nil, errors.Wrapf(err, "recording existing recv tx %s", recvTx.Hash)
	}

	transfer.RecvTxHash = &recvTx.Hash
	transfer.RecvTxTime = &recvTx.Timestamp
	transfer.RecvTxRelayerAddress = &recvTx.RelayerAddress

	return transfer, nil
}

func (p CheckRecvPacketDelivery) Cancel(transfer *Transfer, err error) {
	transfer.GetLogger().Error("Checking recv packet delivery", "error", err)
}

func (p CheckRecvPacketDelivery) ShouldProcess(transfer *Transfer) bool {
	return transfer.RecvTxHash == nil && transfer.AckTxHash == nil && transfer.TimeoutTxHash == nil
}

func (p CheckRecvPacketDelivery) Status() store.RelayStatus {
	return store.RelayStatusCheckRecvPacketDelivery
}
