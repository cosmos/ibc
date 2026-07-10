package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/e2e/stub/internal/cosmos"
	"github.com/cosmos/ibc/link/e2e/stub/internal/onchain"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// discoverSources scans every configured source chain and inserts newly observed packets.
//
// Cosmos scans currently re-page matching history; idempotent inserts make restart and overlap safe.
//
// Startup guarantees a deployment entry and a live connection for every configured chain, and deploy
// emits every fixture or fails wholesale — so a miss below is a broken invariant (config/DB skew) and
// returns loudly rather than leaving the relayer silently idle.
func (r *relayer) discoverSources(ctx context.Context) error {
	var errs []error
	for _, ch := range r.cfg.Chains {
		dep, ok := r.dep.Chain(ch.ID)
		if !ok {
			errs = append(errs, fmt.Errorf("discover source %s: no deployment for chain", ch.ID))
			continue
		}
		switch ch.Type {
		case wire.ChainTypeCosmos:
			if err := r.discoverCosmosSource(ctx, ch.ID, dep); err != nil {
				errs = append(errs, fmt.Errorf("discover cosmos source %s: %w", ch.ID, err))
			}
		default:
			if err := r.discoverEVMSource(ctx, ch.ID, dep); err != nil {
				errs = append(errs, fmt.Errorf("discover evm source %s: %w", ch.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}

// discoverSourceTx resolves packets from one committed transaction without touching periodic scan cursors.
func (r *relayer) discoverSourceTx(ctx context.Context, chainID, txHash string) error {
	chain, ok := r.cfg.Chain(chainID)
	if !ok {
		return fmt.Errorf("unknown source chain %q", chainID)
	}
	dep, ok := r.dep.Chain(chainID)
	if !ok {
		return fmt.Errorf("discover source %s: no deployment for chain", chainID)
	}
	if chain.Type == wire.ChainTypeCosmos {
		return r.discoverCosmosSourceTx(ctx, chainID, txHash, dep)
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

func (r *relayer) discoverCosmosSourceTx(
	ctx context.Context,
	chainID string,
	txHash string,
	dep wire.ChainDeployment,
) error {
	client, ok := r.cosmos[chainID]
	if !ok {
		return fmt.Errorf("chain %s is not connected", chainID)
	}
	route, hasGMPRoute := r.cosmosSourceGMPRoute(chainID)
	var sourceClient string
	if hasGMPRoute {
		var err error
		sourceClient, err = dep.Fixture(fixturekeys.AttestationsClient)
		if err != nil {
			return err
		}
	}
	denom, err := dep.Fixture(fixturekeys.IFTDenom)
	if err != nil {
		return err
	}
	ifts, gmps, err := client.DiscoverSentTx(ctx, txHash, sourceClient, denom)
	if err != nil {
		return err
	}
	for _, event := range ifts {
		if err := r.insertCosmosIFT(ctx, route, event); err != nil {
			return err
		}
	}
	if hasGMPRoute {
		for _, event := range gmps {
			if err := r.insertCosmosGMP(ctx, route, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// discoverEVMSource scans a chain's MockIFT/MockGMP fixtures for IFTSent/GMPSent from a per-fixture cursor
// (mirroring the destination-side recvCursor scan) and inserts a pending row for each event whose routeId
// names a configured route sourced here. The source fixture is shared across every route that leaves this
// chain, so each event is attributed to its route by the routeId the event carries.
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
	// Advance the cursor only after every discovered row landed: a failed insert leaves it behind so
	// the same window re-scans next tick, and the idempotent insert converges instead of duplicating.
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
	// Same insert-before-advance ordering as the IFT scan above.
	r.sentCursor[gmpKey] = gmpNext
	return nil
}

// discoverCosmosSource discovers native IFT and GMP packets from their canonical module events.
func (r *relayer) discoverCosmosSource(ctx context.Context, chainID string, dep wire.ChainDeployment) error {
	cc, ok := r.cosmos[chainID]
	if !ok {
		return fmt.Errorf("chain %s is not connected", chainID)
	}
	route, ok := r.cosmosSourceGMPRoute(chainID)
	if !ok {
		return nil
	}
	sourceClient, err := dep.Fixture(fixturekeys.AttestationsClient)
	if err != nil {
		return err
	}
	denom, err := dep.Fixture(fixturekeys.IFTDenom)
	if err != nil {
		return err
	}
	discovered, err := cc.DiscoverIFTSent(ctx, sourceClient, denom)
	if err != nil {
		return err
	}
	for _, d := range discovered {
		if insertErr := r.insertCosmosIFT(ctx, route, d); insertErr != nil {
			return insertErr
		}
	}
	discoveredGMP, err := cc.DiscoverGMPSent(ctx, sourceClient)
	if err != nil {
		return err
	}
	for _, d := range discoveredGMP {
		if insertErr := r.insertCosmosGMP(ctx, route, d); insertErr != nil {
			return insertErr
		}
	}
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

func (r *relayer) insertCosmosIFT(ctx context.Context, route wire.Route, event cosmos.DiscoveredIFT) error {
	if event.ClientID == "" {
		return nil
	}
	return r.store.InsertPending(ctx, store.Packet{
		PacketID:     wire.PacketID(route.ID, wire.AppTypeIFT, event.Seq),
		RouteID:      route.ID,
		AppType:      wire.AppTypeIFT,
		Sequence:     event.Seq,
		SourceTxHash: event.TxHash,
		Receiver:     event.Receiver,
		Amount:       event.Amount.String(),
		Target:       event.Target,
		Payload:      hexutil.Encode(event.Payload),
	})
}

func (r *relayer) insertCosmosGMP(ctx context.Context, route wire.Route, event cosmos.DiscoveredGMP) error {
	return r.store.InsertPending(ctx, store.Packet{
		PacketID:     wire.PacketID(route.ID, wire.AppTypeGMP, event.Seq),
		RouteID:      route.ID,
		AppType:      wire.AppTypeGMP,
		Sequence:     event.Seq,
		SourceTxHash: event.TxHash,
		Target:       event.Target,
		Payload:      hexutil.Encode(event.Payload),
	})
}

// cursorKey keys every per-fixture block-scan cursor (sent and received) by (chain id, fixture
// address). The chain id is required because deterministic CREATE addresses make every chain's
// MockIFT/MockGMP share one address — an address-only key would let one chain's cursor skip past
// another chain's blocks.
func cursorKey(chainID string, addr common.Address) string {
	return chainID + "|" + addr.Hex()
}

// cosmosSourceGMPRoute returns the cosmosToEvm route sourced at chainID (the one whose source client the GMP
// send_packet scan reads). config validate rejects a second such route per source, so first-match is exact.
func (r *relayer) cosmosSourceGMPRoute(chainID string) (wire.Route, bool) {
	for _, route := range r.cfg.Relayer.Routes {
		if route.Source == chainID && route.Type == wire.RouteCosmosToEVMAttested {
			return route, true
		}
	}
	return wire.Route{}, false
}
