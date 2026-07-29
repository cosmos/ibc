package processors

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

func (p CheckRecvPacketDelivery) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(tr.DestinationChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for destination chain %s", tr.DestinationChainID)
	}

	received, err := client.IsPacketReceived(ctx, tr.PacketDestinationClientID, tr.PacketSequenceNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "checking packet receipt on destination chain %s", tr.DestinationChainID)
	}

	if !received {
		// not received; continue to deliver the recv packet as normal
		return tr, nil
	}

	tr.GetLogger().Info("Packet already received on destination chain, searching for the recv tx")

	recvTx, err := client.FindRecvTx(ctx, tr.PacketDestinationClientID, tr.PacketSequenceNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "finding recv tx on destination chain %s", tr.DestinationChainID)
	}

	tx := store.PacketTx{Hash: recvTx.Hash, Time: recvTx.Timestamp, RelayerAddress: recvTx.RelayerAddress}
	if err := p.storage.UpdatePacketRecvTx(ctx, tr.Key(), tx); err != nil {
		return nil, errors.Wrapf(err, "recording existing recv tx %s", recvTx.Hash)
	}

	tr.RecvTxHash = &recvTx.Hash
	tr.RecvTxTime = &recvTx.Timestamp
	tr.RecvTxRelayerAddress = &recvTx.RelayerAddress

	return tr, nil
}

func (p CheckRecvPacketDelivery) Cancel(tr *Transfer, err error) {
	tr.GetLogger().Error("Checking recv packet delivery", "err", err)
}

func (p CheckRecvPacketDelivery) ShouldProcess(tr *Transfer) bool {
	return tr.RecvTxHash == nil && tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p CheckRecvPacketDelivery) Status() store.RelayStatus {
	return store.RelayStatusCheckRecvPacketDelivery
}
