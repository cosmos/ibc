package processors

import (
	"context"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/link/internal/store"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// CheckPacketCommitment populates the ack or timeout tx details when the
// packet commitment is already gone from the source chain.
type CheckPacketCommitment struct {
	chains  ChainClients
	storage AckTimeoutTxStorage
}

// AckTimeoutTxStorage persists ack and timeout tx details.
type AckTimeoutTxStorage interface {
	UpdatePacketAckTx(ctx context.Context, key store.PacketKey, tx store.PacketTx) error
	UpdatePacketTimeoutTx(ctx context.Context, key store.PacketKey, tx store.PacketTx) error
}

func NewCheckPacketCommitment(chainClients ChainClients, storage AckTimeoutTxStorage) CheckPacketCommitment {
	return CheckPacketCommitment{chains: chainClients, storage: storage}
}

func (p CheckPacketCommitment) Process(ctx context.Context, tr *Transfer) (*Transfer, error) {
	client, ok := p.chains.Get(tr.SourceChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for source chain %s", tr.SourceChainID)
	}

	committed, err := client.IsPacketCommitted(ctx, tr.PacketSourceClientID, tr.PacketSequenceNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "checking packet commitment on source chain %s", tr.SourceChainID)
	}

	if committed {
		// the packet has not been acked or timed out yet; continue as normal
		return tr, nil
	}

	tr.GetLogger().Info("Packet commitment gone from source chain, searching for the ack or timeout tx")

	// race the two lookups; whichever finds its tx cancels the other
	var ackTx, timeoutTx *v2.Tx
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		tx, errFind := client.FindAckTx(gctx, tr.PacketSourceClientID, tr.PacketSequenceNumber)
		if errFind != nil {
			if !errors.Is(errFind, v2.ErrTxNotFound) && !errors.Is(errFind, context.Canceled) {
				tr.GetLogger().Warn("Finding ack tx after missing packet commitment", "error", errFind)
			}

			// do not cancel the timeout lookup
			return nil
		}

		ackTx = tx

		return errors.New("found ack tx")
	})

	g.Go(func() error {
		tx, errFind := client.FindTimeoutTx(gctx, tr.PacketSourceClientID, tr.PacketSequenceNumber)
		if errFind != nil {
			if !errors.Is(errFind, v2.ErrTxNotFound) && !errors.Is(errFind, context.Canceled) {
				tr.GetLogger().Warn("Finding timeout tx after missing packet commitment", "error", errFind)
			}

			// do not cancel the ack lookup
			return nil
		}

		timeoutTx = tx

		return errors.New("found timeout tx")
	})

	_ = g.Wait()

	switch {
	case ackTx != nil && timeoutTx != nil:
		return nil, errors.Errorf(
			"found both ack tx %s and timeout tx %s, should not be possible",
			ackTx.Hash,
			timeoutTx.Hash,
		)
	case ackTx != nil:
		tx := store.PacketTx{Hash: ackTx.Hash, Time: ackTx.Timestamp, RelayerAddress: ackTx.RelayerAddress}
		if err := p.storage.UpdatePacketAckTx(ctx, tr.Key(), tx); err != nil {
			return nil, errors.Wrapf(err, "recording existing ack tx %s", ackTx.Hash)
		}

		tr.AckTxHash = &ackTx.Hash
		tr.AckTxTime = &ackTx.Timestamp
		tr.AckTxRelayerAddress = &ackTx.RelayerAddress

		return tr, nil
	case timeoutTx != nil:
		tx := store.PacketTx{Hash: timeoutTx.Hash, Time: timeoutTx.Timestamp, RelayerAddress: timeoutTx.RelayerAddress}
		if err := p.storage.UpdatePacketTimeoutTx(ctx, tr.Key(), tx); err != nil {
			return nil, errors.Wrapf(err, "recording existing timeout tx %s", timeoutTx.Hash)
		}

		tr.TimeoutTxHash = &timeoutTx.Hash
		tr.TimeoutTxTime = &timeoutTx.Timestamp
		tr.TimeoutTxRelayerAddress = &timeoutTx.RelayerAddress

		return tr, nil
	default:
		return nil, errors.Errorf(
			"no ack or timeout tx found after packet commitment missing on chain %s",
			tr.SourceChainID,
		)
	}
}

func (p CheckPacketCommitment) Cancel(tr *Transfer, err error) {
	tr.GetLogger().Error("Checking packet commitment", "error", err)
}

func (p CheckPacketCommitment) ShouldProcess(tr *Transfer) bool {
	return tr.AckTxHash == nil && tr.TimeoutTxHash == nil
}

func (p CheckPacketCommitment) Status() store.RelayStatus {
	return store.RelayStatusCheckAckPacketDelivery
}
