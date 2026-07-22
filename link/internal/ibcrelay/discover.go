package ibcrelay

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func (r *relayer) discoverSources(ctx context.Context) error {
	var errs []error
	for _, ch := range r.cfg.Chains {
		if err := r.discoverEVMSource(ctx, ch.ID); err != nil {
			errs = append(errs, fmt.Errorf("discover evm source %s: %w", ch.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *relayer) discoverSourceTx(ctx context.Context, chainID, txHash string) error {
	if _, ok := r.cfg.Chain(chainID); !ok {
		return fmt.Errorf("unknown source chain %q", chainID)
	}
	return r.discoverEVMSourceTx(ctx, chainID, txHash)
}

func (r *relayer) discoverEVMSourceTx(ctx context.Context, chainID, txHash string) error {
	conn, ok := r.conns[chainID]
	if !ok {
		return fmt.Errorf("chain %s is not connected", chainID)
	}
	rawHash, err := hexutil.Decode(txHash)
	if err != nil || len(rawHash) != common.HashLength {
		return fmt.Errorf("invalid EVM transaction hash %q", txHash)
	}
	events, err := conn.ops.sendPacketsFromTx(ctx, common.BytesToHash(rawHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return nil
		}
		return err
	}
	for _, ev := range events {
		if insertErr := r.insertSendPacket(ctx, chainID, ev); insertErr != nil {
			return insertErr
		}
	}
	return nil
}

func (r *relayer) discoverEVMSource(ctx context.Context, chainID string) error {
	conn, ok := r.conns[chainID]
	if !ok {
		return fmt.Errorf("chain %s is not connected", chainID)
	}
	key := cursorKey(chainID, conn.routerAddr)
	events, next, err := conn.ops.scanSendPackets(ctx, r.sentCursor[key])
	if err != nil {
		return err
	}
	for _, ev := range events {
		if insertErr := r.insertSendPacket(ctx, chainID, ev); insertErr != nil {
			return insertErr
		}
	}
	// Advance cursor only after inserts land; failed insert retries the same window next tick.
	r.sentCursor[key] = next
	return nil
}

func (r *relayer) insertSendPacket(ctx context.Context, chainID string, event sentPacket) error {
	for _, route := range r.cfg.Relayer.Routes {
		if route.Source != chainID || event.Packet.SourceClient != route.SourceClient {
			continue
		}
		return r.store.InsertPending(ctx, storedPacket{
			PacketID:     relayercmd.RoutePacketID(route.ID, event.Packet.Sequence),
			RouteID:      route.ID,
			Packet:       event.Packet,
			SourceTxHash: event.TxHash.Hex(),
		})
	}
	return nil
}

func cursorKey(chainID string, addr common.Address) string {
	return chainID + "|" + addr.Hex()
}
