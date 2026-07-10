package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/e2e/stub/internal/onchain"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func (r *relayer) discoverSources(ctx context.Context) error {
	var errs []error
	for _, ch := range r.cfg.Chains {
		dep, ok := r.dep.Chain(ch.ID)
		if !ok {
			errs = append(errs, fmt.Errorf("discover source %s: no deployment for chain", ch.ID))
			continue
		}
		if err := r.discoverEVMSource(ctx, ch.ID, dep); err != nil {
			errs = append(errs, fmt.Errorf("discover evm source %s: %w", ch.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *relayer) discoverSourceTx(ctx context.Context, chainID, txHash string) error {
	if _, ok := r.cfg.Chain(chainID); !ok {
		return fmt.Errorf("unknown source chain %q", chainID)
	}
	dep, ok := r.dep.Chain(chainID)
	if !ok {
		return fmt.Errorf("discover source %s: no deployment for chain", chainID)
	}
	return r.discoverEVMSourceTx(ctx, chainID, txHash, dep)
}

func (r *relayer) discoverEVMSourceTx(
	ctx context.Context,
	chainID string,
	txHash string,
	dep wire.ChainDeployment,
) error {
	conn, ok := r.conns[chainID]
	if !ok {
		return fmt.Errorf("chain %s is not connected", chainID)
	}
	rawHash, err := hexutil.Decode(txHash)
	if err != nil || len(rawHash) != common.HashLength {
		return fmt.Errorf("invalid EVM transaction hash %q", txHash)
	}
	receipt, err := conn.client.TransactionReceipt(ctx, common.BytesToHash(rawHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return nil
		}
		return err
	}
	if receipt.Status != 1 {
		return fmt.Errorf("source transaction %s failed", receipt.TxHash.Hex())
	}

	iftAddr, err := dep.Fixture(fixturekeys.MockIFT)
	if err != nil {
		return err
	}
	ift := onchain.NewMockIFT(common.HexToAddress(iftAddr), conn.client)
	if event, found, eventErr := ift.SentFromReceipt(receipt); eventErr != nil {
		return eventErr
	} else if found {
		if insertErr := r.insertEVMIFT(ctx, chainID, event); insertErr != nil {
			return insertErr
		}
	}

	gmpAddr, err := dep.Fixture(fixturekeys.MockGMP)
	if err != nil {
		return err
	}
	gmp := onchain.NewMockGMP(common.HexToAddress(gmpAddr), conn.client)
	if event, found, eventErr := gmp.SentFromReceipt(receipt); eventErr != nil {
		return eventErr
	} else if found {
		return r.insertEVMGMP(ctx, chainID, event)
	}
	return nil
}

func (r *relayer) discoverEVMSource(ctx context.Context, chainID string, dep wire.ChainDeployment) error {
	conn, ok := r.conns[chainID]
	if !ok {
		return fmt.Errorf("chain %s is not connected", chainID)
	}
	iftAddr, err := dep.Fixture(fixturekeys.MockIFT)
	if err != nil {
		return err
	}
	ift := onchain.NewMockIFT(common.HexToAddress(iftAddr), conn.client)
	iftKey := cursorKey(chainID, ift.Address)
	iftEvents, iftNext, err := ift.ScanSentFrom(ctx, r.sentCursor[iftKey])
	if err != nil {
		return err
	}
	for _, ev := range iftEvents {
		if insertErr := r.insertEVMIFT(ctx, chainID, ev); insertErr != nil {
			return insertErr
		}
	}
	// Advance cursor only after inserts land; failed insert retries the same window next tick.
	r.sentCursor[iftKey] = iftNext

	gmpAddr, err := dep.Fixture(fixturekeys.MockGMP)
	if err != nil {
		return err
	}
	gmp := onchain.NewMockGMP(common.HexToAddress(gmpAddr), conn.client)
	gmpKey := cursorKey(chainID, gmp.Address)
	gmpEvents, gmpNext, err := gmp.ScanSentFrom(ctx, r.sentCursor[gmpKey])
	if err != nil {
		return err
	}
	for _, ev := range gmpEvents {
		if insertErr := r.insertEVMGMP(ctx, chainID, ev); insertErr != nil {
			return insertErr
		}
	}
	r.sentCursor[gmpKey] = gmpNext
	return nil
}

func (r *relayer) insertEVMIFT(ctx context.Context, chainID string, event onchain.IFTSent) error {
	route, ok := r.routeByID(event.RouteID)
	if !ok || route.Source != chainID {
		return nil
	}
	return r.store.InsertPending(ctx, store.Packet{
		PacketID:         wire.PacketID(route.ID, wire.AppTypeIFT, event.Seq.Uint64()),
		RouteID:          route.ID,
		AppType:          wire.AppTypeIFT,
		Sequence:         event.Seq.Uint64(),
		SourceTxHash:     event.TxHash.Hex(),
		Receiver:         event.Receiver,
		Amount:           event.Amount.String(),
		TimeoutTimestamp: event.TimeoutTimestamp.String(),
	})
}

func (r *relayer) insertEVMGMP(ctx context.Context, chainID string, event onchain.GMPSent) error {
	route, ok := r.routeByID(event.RouteID)
	if !ok || route.Source != chainID {
		return nil
	}
	return r.store.InsertPending(ctx, store.Packet{
		PacketID:     wire.PacketID(route.ID, wire.AppTypeGMP, event.Seq.Uint64()),
		RouteID:      route.ID,
		AppType:      wire.AppTypeGMP,
		Sequence:     event.Seq.Uint64(),
		SourceTxHash: event.TxHash.Hex(),
		Target:       event.Target,
		Payload:      hexutil.Encode(event.Payload),
	})
}

// CREATE fixture addresses collide across chains; key cursors by chain|addr, not addr alone.
func cursorKey(chainID string, addr common.Address) string {
	return chainID + "|" + addr.Hex()
}
