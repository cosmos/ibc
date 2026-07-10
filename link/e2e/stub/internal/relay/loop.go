package relay

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/link/e2e/stub/internal/cosmos"
	"github.com/cosmos/ibc/link/e2e/stub/internal/onchain"
	"github.com/cosmos/ibc/link/e2e/stub/internal/rpcsafe"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// relayer holds the loop's state: config, deployment, dialed chains (EVM conns + cosmos conns), store, and
// the source-discovery + destination-idempotency cursors. IFT/GMP are end-user transactions submitted
// directly on-chain, so the relayer scans each source for the packets it must complete (the real
// auto-relay contract), then reconciles them to their destination effect.
type relayer struct {
	cfg    *wire.ConfigYAML
	dep    wire.Deployment
	conns  map[string]*chainConn
	cosmos map[string]*cosmos.Client
	store  *store.Store
	log    io.Writer
	// recvCursor bounds destination scans; recvSeen retains results only while their packets are pending.
	recvCursor map[string]uint64
	recvSeen   map[receivedKey]onchain.ReceivedResult
	recvActive map[receivedKey]struct{}
	// sentCursor is the next block to scan per source fixture (IFTSent/GMPSent), keyed by cursorKey.
	// The relay loop is its sole owner; manual relay uses transaction-scoped discovery without cursors.
	sentCursor map[string]uint64
}

type receivedKey struct {
	destination string
	appType     wire.AppType
	seq         uint64
}

func receivedKeyFor(destination string, p store.Packet) receivedKey {
	return receivedKey{
		destination: destination,
		appType:     p.AppType,
		seq:         fixturekeys.RouteScopedSeq(p.RouteID, p.Sequence),
	}
}

// lookupReceived scans only new blocks and caches results for packets that are still pending.
func (r *relayer) lookupReceived(
	ctx context.Context,
	want receivedKey,
	addr common.Address,
	scan func(ctx context.Context, fromBlock uint64) (map[uint64]onchain.ReceivedResult, uint64, error),
) (onchain.ReceivedResult, bool, error) {
	cursor := receivedCursorKey(want, addr)
	results, next, err := scan(ctx, r.recvCursor[cursor])
	if err != nil {
		return onchain.ReceivedResult{}, false, err
	}
	r.recvCursor[cursor] = next
	for s, rec := range results {
		key := receivedKey{destination: want.destination, appType: want.appType, seq: s}
		if _, pending := r.recvActive[key]; pending {
			r.recvSeen[key] = rec
		}
	}
	rec, ok := r.recvSeen[want]
	return rec, ok, nil
}

func receivedCursorKey(key receivedKey, addr common.Address) string {
	return key.destination + "|" + string(key.appType) + "|" + addr.Hex()
}

// loop polls every route on a ticker until ctx is canceled. It processes once immediately so a restart
// resumes pending work without waiting a full interval.
func (r *relayer) loop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick discovers packets from every source, then reconciles every pending packet to its destination effect.
// Both discovery and delivery errors are logged and retried next tick; the daemon does not die on transient
// RPC hiccups.
func (r *relayer) tick(ctx context.Context) {
	if err := r.discoverSources(ctx); err != nil && ctx.Err() == nil {
		_, _ = fmt.Fprintf(r.log, "ibc relayer: discover: %v\n", err)
	}
	if err := r.reconcilePending(ctx); err != nil && ctx.Err() == nil {
		_, _ = fmt.Fprintf(r.log, "ibc relayer: reconcile: %v\n", err)
	}
}

func (r *relayer) reconcilePending(ctx context.Context) error {
	packets, err := r.store.PendingPackets(ctx)
	if err != nil {
		return err
	}
	r.recvActive = r.pendingReceivedKeys(packets)
	requested, err := r.store.RelayRequests(ctx)
	if err != nil {
		return err
	}
	for _, p := range packets {
		if err := r.reconcilePacket(ctx, p, requested); err != nil {
			if ctx.Err() != nil {
				return err
			}
			_, _ = fmt.Fprintf(r.log, "ibc relayer: packet %s: %v\n", p.PacketID, err)
		}
	}
	return nil
}

func (r *relayer) pendingReceivedKeys(packets []store.Packet) map[receivedKey]struct{} {
	active := make(map[receivedKey]struct{}, len(packets))
	for _, p := range packets {
		route, ok := r.routeByID(p.RouteID)
		if ok {
			active[receivedKeyFor(route.Destination, p)] = struct{}{}
		}
	}
	return active
}

// finishTerminal evicts retry state only after the terminal ledger write succeeds.
func (r *relayer) finishTerminal(key receivedKey, err error) error {
	if err != nil {
		return err
	}
	delete(r.recvActive, key)
	delete(r.recvSeen, key)
	return nil
}

func (r *relayer) reconcilePacket(ctx context.Context, p store.Packet, requested map[store.RelayRequestKey]bool) error {
	route, ok := r.routeByID(p.RouteID)
	if !ok {
		return fmt.Errorf("unknown route %q", p.RouteID)
	}
	// The manual-relay gate, before any lifecycle work: an unrequested packet on a manual route is
	// invisible to the relayer — no delivery and no timeout processing. Requests are persisted (loaded
	// once per tick), so a restarted daemon resumes requested manual packets from sqlite exactly like
	// auto ones.
	if !route.AutoRelayEnabled() &&
		!requested[store.RelayRequestKey{SourceChainID: route.Source, SourceTxHash: p.SourceTxHash}] {
		return nil
	}
	// Dispatch on the destination family: a Cosmos destination delivers through IBC v2 into its native
	// modules (it has no EVM conn), so it takes a separate path from the EVM-typed delivery abstraction.
	if r.destinationIsCosmos(route) {
		return r.reconcileCosmosPacket(ctx, route, p)
	}
	// A Cosmos source has already burned through its native IFT module. It still mints on the EVM side, but
	// has no EVM source conn, so handle it before routeContext. This mirrors destinationIsCosmos above.
	if r.sourceIsCosmos(route) {
		return r.reconcileCosmosSourcePacket(ctx, route, p)
	}
	src, dst, srcDep, dstDep, err := r.routeContext(route)
	if err != nil {
		return err
	}
	switch p.AppType {
	case wire.AppTypeIFT:
		d, err := r.iftDelivery(p, src, dst, srcDep, dstDep)
		if err != nil {
			return err
		}
		return r.deliverPending(ctx, src, dst, p, d)
	case wire.AppTypeGMP:
		d, err := r.gmpDelivery(p, dst, dstDep)
		if err != nil {
			return err
		}
		return r.deliverPending(ctx, src, dst, p, d)
	default:
		return fmt.Errorf("unknown app type %q", p.AppType)
	}
}

// reconcileCosmosPacket delivers one pending packet to a cosmos destination through IBC v2. IFT executes
// the native module's MsgIFTMint through ICS-27-GMP; GMP executes its CosmosTx directly. Packet receipts and
// acknowledgements provide destination idempotency for both applications.
//
// The IFT timeout/refund leg is out of scope for cosmos (a packet carrying a timeout would otherwise drive
// the EVM refund machinery, which has no cosmos analog). GMP has no timeout leg anywhere.
func (r *relayer) reconcileCosmosPacket(ctx context.Context, route wire.Route, p store.Packet) error {
	dst, ok := r.cosmos[route.Destination]
	if !ok {
		return fmt.Errorf("cosmos destination chain %s not connected", route.Destination)
	}
	received := receivedKeyFor(route.Destination, p)
	switch p.AppType {
	case wire.AppTypeIFT:
		if hasTimeout(p) {
			return fmt.Errorf(
				"packet %s carries a timeout, but the IFT timeout/refund leg is out of scope for cosmos destination %s",
				p.PacketID,
				route.Destination,
			)
		}
		amount, ok := new(big.Int).SetString(p.Amount, 10)
		if !ok {
			return fmt.Errorf("invalid IFT amount %q", p.Amount)
		}
		dstDep, ok := r.dep.Chain(route.Destination)
		if !ok {
			return fmt.Errorf("no deployment for cosmos destination chain %s", route.Destination)
		}
		srcDep, ok := r.dep.Chain(route.Source)
		if !ok {
			return fmt.Errorf("no deployment for EVM source chain %s", route.Source)
		}
		destClient, err := dstDep.Fixture(fixturekeys.AttestationsClient)
		if err != nil {
			return err
		}
		denom, err := dstDep.Fixture(fixturekeys.IFTDenom)
		if err != nil {
			return err
		}
		counterpartyIFT, err := srcDep.Fixture(fixturekeys.MockIFT)
		if err != nil {
			return err
		}
		txHash, success, reason, err := dst.DeliverIFT(
			ctx, destClient, denom, counterpartyIFT, p.Receiver, amount, received.seq,
		)
		if err != nil {
			return err
		}
		if !success {
			return r.finishTerminal(received, r.store.MarkErrorAck(ctx, p.PacketID, txHash, reason))
		}
		_, _ = fmt.Fprintf(
			r.log,
			"ibc relayer: relayed IFT packet %s on route %s (seq %d) -> cosmos MsgRecvPacket %s on %s\n",
			p.PacketID,
			p.RouteID,
			p.Sequence,
			txHash,
			route.Destination,
		)
		return r.finishTerminal(received, r.store.MarkComplete(ctx, p.PacketID, txHash))
	case wire.AppTypeGMP:
		return r.deliverCosmosGMP(ctx, route, dst, p)
	default:
		return fmt.Errorf("unknown app type %q on cosmos destination %s", p.AppType, route.Destination)
	}
}

// deliverCosmosGMP delivers one pending GMP packet to a cosmos destination for real over IBC v2: a signed
// MsgRecvPacket into the chain's native 27-gmp module, proven by the attestations light client (see the stub's
// cosmos.DeliverGMP). The module atomically runs the delivered CosmosTx as the ICS-27 account: an increment
// moves 1 counter-denom ICS27->target (packet complete, recv tx = the effect); a deliberately-failing payload
// yields the universal error acknowledgement with the target left unchanged (packet error_ack, recv tx = the
// effect, reason from the module's error event) — mirroring the EVM GMP error-ack, so the status surface is
// identical across families. The destination client id, ICS-27 executor account, and counter denom come from
// the destination's deployment fixtures the stub emitted at deploy.
func (r *relayer) deliverCosmosGMP(ctx context.Context, route wire.Route, dst *cosmos.Client, p store.Packet) error {
	payload, err := hexutil.Decode(p.Payload)
	if err != nil {
		return fmt.Errorf("invalid GMP payload %q: %w", p.Payload, err)
	}
	dstDep, ok := r.dep.Chain(route.Destination)
	if !ok {
		return fmt.Errorf("no deployment for cosmos destination chain %s", route.Destination)
	}
	destClient, err := dstDep.Fixture(fixturekeys.AttestationsClient)
	if err != nil {
		return err
	}
	ics27, err := dstDep.Fixture(fixturekeys.ICS27Account)
	if err != nil {
		return err
	}
	gmpDenom, err := dstDep.Fixture(fixturekeys.GMPDenom)
	if err != nil {
		return err
	}
	received := receivedKeyFor(route.Destination, p)
	// The fabricated IBC v2 packet's receipt/ack replay guard is keyed on (destination client, sequence); the
	// route-scoped sequence keeps two EVM sources into one cosmos destination from colliding at the same seq.
	txHash, success, reason, err := dst.DeliverGMP(
		ctx, destClient, ics27, gmpDenom, p.Target, payload, received.seq,
	)
	if err != nil {
		return err
	}
	if !success {
		_, _ = fmt.Fprintf(
			r.log,
			"ibc relayer: ERROR-ACK GMP packet %s on route %s (seq %d) -> cosmos recv %s on %s: %s\n",
			p.PacketID,
			p.RouteID,
			p.Sequence,
			txHash,
			route.Destination,
			reason,
		)
		return r.finishTerminal(received, r.store.MarkErrorAck(ctx, p.PacketID, txHash, reason))
	}
	_, _ = fmt.Fprintf(
		r.log,
		"ibc relayer: relayed GMP packet %s on route %s (seq %d) -> cosmos recv %s on %s\n",
		p.PacketID,
		p.RouteID,
		p.Sequence,
		txHash,
		route.Destination,
	)
	return r.finishTerminal(received, r.store.MarkComplete(ctx, p.PacketID, txHash))
}

// reconcileCosmosSourcePacket delivers one pending packet whose source is a Cosmos chain and whose
// destination is EVM. IFT forwards the native packet's canonical mint calldata through the GMP adapter;
// GMP replays target.call through MockGMP.deliver. The source action already ran through the native IFT or
// 27-gmp module, so no EVM source conn or fixture is bound.
//
// Every destination effect uses the route-scoped sequence; the raw native sequence remains source identity
// for acknowledgement.
//
// A successful IFT delivery relays MsgAcknowledgement back to the Cosmos source, completing the native
// pending transfer. The GMP acknowledgement leg remains outside this fixture's scope.
func (r *relayer) reconcileCosmosSourcePacket(ctx context.Context, route wire.Route, p store.Packet) error {
	if hasTimeout(p) {
		return fmt.Errorf(
			"packet %s carries a timeout, but the timeout/refund leg is out of scope for cosmos source %s",
			p.PacketID,
			route.Source,
		)
	}
	dst, ok := r.conns[route.Destination]
	if !ok {
		return fmt.Errorf("destination chain %s not connected", route.Destination)
	}
	dstDep, ok := r.dep.Chain(route.Destination)
	if !ok {
		return fmt.Errorf("no deployment for destination chain %s", route.Destination)
	}
	var (
		d   delivery
		err error
	)
	switch p.AppType {
	case wire.AppTypeIFT:
		d, err = r.cosmosIFTDelivery(p, dst, dstDep)
	case wire.AppTypeGMP:
		d, err = r.gmpDelivery(p, dst, dstDep)
	default:
		return fmt.Errorf("unknown app type %q on cosmos source %s", p.AppType, route.Source)
	}
	if err != nil {
		return err
	}
	if p.AppType == wire.AppTypeIFT {
		srcCosmos, ok := r.cosmos[route.Source]
		if !ok {
			return fmt.Errorf("cosmos source chain %s not connected", route.Source)
		}
		srcDep, ok := r.dep.Chain(route.Source)
		if !ok {
			return fmt.Errorf("no deployment for cosmos source chain %s", route.Source)
		}
		sourceClient, err := srcDep.Fixture(fixturekeys.AttestationsClient)
		if err != nil {
			return err
		}
		d.finalize = func(ctx context.Context) error {
			ackHash, err := srcCosmos.AcknowledgeIFT(ctx, p.SourceTxHash, sourceClient, p.Sequence)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(
				r.log,
				"ibc relayer: acknowledged IFT packet %s on cosmos source %s in %s\n",
				p.PacketID,
				route.Source,
				ackHash,
			)
			return nil
		}
	}
	// A cosmos source has no EVM source conn; the timeout/refund leg (the only src user) is out of scope
	// here, so dst stands in for the unused src.
	return r.deliverPending(ctx, dst, dst, p, d)
}

// cosmosIFTDelivery forwards the native Cosmos module's unchanged EVM mint calldata through the mock GMP
// adapter. The adapter supplies source-client context while MockIFT executes the canonical iftMint call.
func (r *relayer) cosmosIFTDelivery(
	p store.Packet,
	dst *chainConn,
	dstDep wire.ChainDeployment,
) (delivery, error) {
	payload, err := hexutil.Decode(p.Payload)
	if err != nil {
		return delivery{}, fmt.Errorf("invalid IFT payload %q: %w", p.Payload, err)
	}
	gmpAddr, err := dstDep.Fixture(fixturekeys.MockGMP)
	if err != nil {
		return delivery{}, err
	}
	iftAddr, err := dstDep.Fixture(fixturekeys.MockIFT)
	if err != nil {
		return delivery{}, err
	}
	target := common.HexToAddress(p.Target)
	if target != common.HexToAddress(iftAddr) {
		return delivery{}, fmt.Errorf("IFT packet target %s does not match destination IFT %s", p.Target, iftAddr)
	}
	// IFTMintReceived identifies the destination's local IBC client, matching upstream ICS27 account
	// semantics. The outer GMP event carries the route-scoped destination sequence.
	destinationClientID := dstDep.ClientID
	if destinationClientID == "" {
		return delivery{}, fmt.Errorf("destination EVM deployment has no IBC client id")
	}
	gmp := onchain.NewMockGMP(common.HexToAddress(gmpAddr), dst.client)
	seq := new(big.Int).SetUint64(p.Sequence)
	received := receivedKeyFor(dst.id, p)
	scoped := new(big.Int).SetUint64(received.seq)
	return delivery{
		seq: seq,
		findReceived: func(ctx context.Context) (onchain.ReceivedResult, bool, error) {
			return r.lookupReceived(ctx, received, gmp.Address, func(
				ctx context.Context,
				fromBlock uint64,
			) (map[uint64]onchain.ReceivedResult, uint64, error) {
				return gmp.ScanIFTReceivedFrom(ctx, target, destinationClientID, fromBlock)
			})
		},
		deliver: func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return gmp.DeliverIFT(opts, scoped, destinationClientID, target, payload)
		},
		ackOK: func(rcpt *types.Receipt) (bool, error) {
			ok, found, err := gmp.DeliveredSuccessFromReceipt(rcpt)
			if err != nil {
				return false, err
			}
			if !found {
				return false, fmt.Errorf("deliver tx %s emitted no GMPReceived", rcpt.TxHash.Hex())
			}
			return ok, nil
		},
		effect: "deliver",
	}, nil
}

// sourceIsCosmos reports whether the route's source chain is a cosmos-family chain (per the config).
func (r *relayer) sourceIsCosmos(route wire.Route) bool {
	ch, ok := r.chainByID(route.Source)
	return ok && ch.Type == wire.ChainTypeCosmos
}

// destinationIsCosmos reports whether the route's destination chain is a cosmos-family chain (per the config).
func (r *relayer) destinationIsCosmos(route wire.Route) bool {
	ch, ok := r.chainByID(route.Destination)
	return ok && ch.Type == wire.ChainTypeCosmos
}

// chainByID looks up a configured chain entry by id.
func (r *relayer) chainByID(id string) (wire.Chain, bool) {
	for _, ch := range r.cfg.Chains {
		if ch.ID == id {
			return ch, true
		}
	}
	return wire.Chain{}, false
}

// hasTimeout reports whether a packet carries a non-zero IFT timeout deadline. The store keeps the
// timestamp as a decimal string; "" and "0" both mean "no timeout" (the happy path).
func hasTimeout(p store.Packet) bool {
	return p.TimeoutTimestamp != "" && p.TimeoutTimestamp != "0"
}

func (r *relayer) routeByID(id string) (wire.Route, bool) {
	for _, route := range r.cfg.Relayer.Routes {
		if route.ID == id {
			return route, true
		}
	}
	return wire.Route{}, false
}

func (r *relayer) routeContext(
	route wire.Route,
) (*chainConn, *chainConn, wire.ChainDeployment, wire.ChainDeployment, error) {
	src, ok := r.conns[route.Source]
	if !ok {
		return nil, nil, wire.ChainDeployment{}, wire.ChainDeployment{}, fmt.Errorf(
			"source chain %s not connected",
			route.Source,
		)
	}
	dst, ok := r.conns[route.Destination]
	if !ok {
		return nil, nil, wire.ChainDeployment{}, wire.ChainDeployment{}, fmt.Errorf(
			"destination chain %s not connected",
			route.Destination,
		)
	}
	srcDep, ok := r.dep.Chain(route.Source)
	if !ok {
		return nil, nil, wire.ChainDeployment{}, wire.ChainDeployment{}, fmt.Errorf(
			"no deployment for source chain %s",
			route.Source,
		)
	}
	dstDep, ok := r.dep.Chain(route.Destination)
	if !ok {
		return nil, nil, wire.ChainDeployment{}, wire.ChainDeployment{}, fmt.Errorf(
			"no deployment for destination chain %s",
			route.Destination,
		)
	}
	return src, dst, srcDep, dstDep, nil
}

// delivery is the app-specific work needed to progress one pending packet. The destination effect (deliver)
// and its idempotency guard (findReceived) run under the route-scoped sequence — captured in their closures,
// so they take no seq argument — while the source-side refund leg keeps the raw sequence in seq (the source
// escrow is keyed by it and is already unique per source fixture).
type delivery struct {
	seq          *big.Int
	findReceived func(ctx context.Context) (onchain.ReceivedResult, bool, error)
	deliver      func(opts *bind.TransactOpts) (*types.Transaction, error)
	effect       string

	timeout      *big.Int
	refund       func(opts *bind.TransactOpts) (*types.Transaction, error)
	findRefunded func(ctx context.Context, seq *big.Int) (common.Hash, bool, error)
	ackOK        func(rcpt *types.Receipt) (bool, error)
	finalize     func(context.Context) error
}

// iftMintDelivery builds the destination mint for an IFT packet: it binds the destination MockIFT and
// returns a delivery whose deliver() mints the store row's amount to its receiver for its sequence, with
// FindReceived as the idempotency guard. It carries no source refund closure and a zero timeout — the
// delivery a cosmos-source route uses (the refund/timeout leg is out of scope there); iftDelivery is the
// evm->evm superset that also wires the source-side refund for the timeout leg.
func (r *relayer) iftMintDelivery(p store.Packet, dst *chainConn, dstDep wire.ChainDeployment) (delivery, error) {
	seq := new(big.Int).SetUint64(p.Sequence)
	amount, ok := new(big.Int).SetString(p.Amount, 10)
	if !ok {
		return delivery{}, fmt.Errorf("invalid IFT amount %q", p.Amount)
	}
	dstIFTAddr, err := dstDep.Fixture(fixturekeys.MockIFT)
	if err != nil {
		return delivery{}, err
	}
	dstIFT := onchain.NewMockIFT(common.HexToAddress(dstIFTAddr), dst.client)
	receiver := common.HexToAddress(p.Receiver)
	// The mint (and its idempotency scan) key on the route-scoped sequence so two routes minting to one
	// receiver on a shared destination fixture cannot cross-match at the same source seq.
	received := receivedKeyFor(dst.id, p)
	scoped := new(big.Int).SetUint64(received.seq)
	return delivery{
		seq: seq,
		findReceived: func(ctx context.Context) (onchain.ReceivedResult, bool, error) {
			return r.lookupReceived(ctx, received, dstIFT.Address, dstIFT.ScanReceivedFrom)
		},
		deliver: func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return dstIFT.ReceiveTransfer(opts, scoped, receiver, amount)
		},
		effect: "recv",
	}, nil
}

func (r *relayer) iftDelivery(
	p store.Packet,
	src, dst *chainConn,
	srcDep, dstDep wire.ChainDeployment,
) (delivery, error) {
	d, err := r.iftMintDelivery(p, dst, dstDep)
	if err != nil {
		return delivery{}, err
	}
	// Add the source-side refund for the timeout leg (evm->evm only): the source is an EVM MockIFT the relayer
	// calls refund() on once the destination deadline elapses undelivered.
	timeout := big.NewInt(0)
	if p.TimeoutTimestamp != "" {
		var ok bool
		timeout, ok = new(big.Int).SetString(p.TimeoutTimestamp, 10)
		if !ok {
			return delivery{}, fmt.Errorf("invalid IFT timeout timestamp %q", p.TimeoutTimestamp)
		}
	}
	srcIFTAddr, err := srcDep.Fixture(fixturekeys.MockIFT)
	if err != nil {
		return delivery{}, err
	}
	srcIFT := onchain.NewMockIFT(common.HexToAddress(srcIFTAddr), src.client)
	d.timeout = timeout
	d.refund = func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return srcIFT.Refund(opts, d.seq)
	}
	d.findRefunded = srcIFT.FindRefunded
	return d, nil
}

func (r *relayer) gmpDelivery(p store.Packet, dst *chainConn, dstDep wire.ChainDeployment) (delivery, error) {
	seq := new(big.Int).SetUint64(p.Sequence)
	payload, err := hexutil.Decode(p.Payload)
	if err != nil {
		return delivery{}, fmt.Errorf("invalid GMP payload %q: %w", p.Payload, err)
	}
	dstGMPAddr, err := dstDep.Fixture(fixturekeys.MockGMP)
	if err != nil {
		return delivery{}, err
	}
	dstGMP := onchain.NewMockGMP(common.HexToAddress(dstGMPAddr), dst.client)
	target := common.HexToAddress(p.Target)
	// The delivery (and its idempotency scan) key on the route-scoped sequence so two routes delivering to
	// one destination fixture cannot cross-match at the same source seq.
	received := receivedKeyFor(dst.id, p)
	scoped := new(big.Int).SetUint64(received.seq)
	return delivery{
		seq: seq,
		findReceived: func(ctx context.Context) (onchain.ReceivedResult, bool, error) {
			return r.lookupReceived(ctx, received, dstGMP.Address, dstGMP.ScanReceivedFrom)
		},
		deliver: func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return dstGMP.Deliver(opts, scoped, target, payload)
		},
		ackOK: func(rcpt *types.Receipt) (bool, error) {
			ok, found, err := dstGMP.DeliveredSuccessFromReceipt(rcpt)
			if err != nil {
				return false, err
			}
			if !found {
				return false, fmt.Errorf("deliver tx %s emitted no GMPReceived", rcpt.TxHash.Hex())
			}
			return ok, nil
		},
		effect: "deliver",
	}, nil
}

// deliverPending ensures the destination effect for a pending packet happens once, or records a
// terminal failure. The loop is intentionally single-goroutine and waits for mining before moving to the
// next packet; Anvil instant mining keeps this simple and deterministic for the POC.
func (r *relayer) deliverPending(ctx context.Context, src, dst *chainConn, p store.Packet, d delivery) error {
	received := receivedKeyFor(dst.id, p)
	if d.timeout != nil && d.timeout.Sign() > 0 {
		timedOut, err := r.maybeTimeout(ctx, src, dst, p, d)
		if err != nil {
			return err
		}
		if timedOut {
			return nil
		}
	}

	// Idempotency: if the destination already recorded this sequence, adopt that outcome instead of
	// re-delivering. The success bit distinguishes a real delivery from a reverted one (GMPReceived is
	// emitted for both), so a crash between delivery and the terminal mark is recovered to the right
	// state — not collapsed into complete.
	if rec, exists, err := d.findReceived(ctx); err != nil {
		return err
	} else if exists {
		if !rec.Success {
			reason := fmt.Sprintf("destination call reverted on %s (error acknowledgement)", dst.id)
			return r.finishTerminal(received, r.store.MarkErrorAck(ctx, p.PacketID, rec.TxHash.Hex(), reason))
		}
		if d.finalize != nil {
			if err := d.finalize(ctx); err != nil {
				return err
			}
		}
		return r.finishTerminal(received, r.store.MarkComplete(ctx, p.PacketID, rec.TxHash.Hex()))
	}

	opts, err := onchain.RelayerTransactor(ctx, dst.signerKey, dst.chainID)
	if err != nil {
		return err
	}
	tx, err := d.deliver(opts)
	if err != nil {
		return fmt.Errorf("submit %s: %s", d.effect, rpcsafe.RedactURLs(err.Error()))
	}
	rcpt, err := onchain.WaitMined(ctx, dst.client, tx)
	if err != nil {
		return err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("%s reverted on %s (tx %s)", d.effect, dst.id, tx.Hash().Hex())
	}
	if d.ackOK != nil {
		ok, err := d.ackOK(rcpt)
		if err != nil {
			return err
		}
		if !ok {
			reason := fmt.Sprintf("destination call reverted on %s (error acknowledgement)", dst.id)
			_, _ = fmt.Fprintf(
				r.log,
				"ibc relayer: ERROR-ACK %s packet %s (seq %d) -> deliver %s on %s: %s\n",
				p.AppType,
				p.PacketID,
				p.Sequence,
				tx.Hash().Hex(),
				dst.id,
				reason,
			)
			return r.finishTerminal(received, r.store.MarkErrorAck(ctx, p.PacketID, tx.Hash().Hex(), reason))
		}
	}
	if d.finalize != nil {
		if err := d.finalize(ctx); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(
		r.log,
		"ibc relayer: relayed %s packet %s on route %s (seq %d) -> %s %s on %s\n",
		p.AppType,
		p.PacketID,
		p.RouteID,
		p.Sequence,
		d.effect,
		tx.Hash().Hex(),
		dst.id,
	)
	return r.finishTerminal(received, r.store.MarkComplete(ctx, p.PacketID, tx.Hash().Hex()))
}

// maybeTimeout refunds the source escrow and marks the packet timed_out when its deadline has passed
// and the destination never received it.
func (r *relayer) maybeTimeout(ctx context.Context, src, dst *chainConn, p store.Packet, d delivery) (bool, error) {
	received := receivedKeyFor(dst.id, p)
	hdr, err := dst.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("destination head for timeout check: %s", rpcsafe.RedactURLs(err.Error()))
	}
	if new(big.Int).SetUint64(hdr.Time).Cmp(d.timeout) < 0 {
		return false, nil
	}
	if _, exists, receivedErr := d.findReceived(ctx); receivedErr != nil {
		return false, receivedErr
	} else if exists {
		return false, nil
	}

	// If the escrow was already refunded (a refund committed on-chain before a crash marked the packet
	// timed_out), adopt that tx instead of re-issuing refund — a second refund reverts on the fixture's
	// "already refunded" guard and would wedge the packet pending forever.
	if refundTx, refunded, refundedErr := d.findRefunded(ctx, d.seq); refundedErr != nil {
		return false, refundedErr
	} else if refunded {
		reason := fmt.Sprintf(
			"destination timeout elapsed at %s and source escrow was already refunded on %s",
			d.timeout, src.id,
		)
		if markErr := r.finishTerminal(
			received,
			r.store.MarkTimedOut(ctx, p.PacketID, refundTx.Hex(), reason),
		); markErr != nil {
			return false, markErr
		}
		return true, nil
	}

	opts, err := onchain.RelayerTransactor(ctx, src.signerKey, src.chainID)
	if err != nil {
		return false, err
	}
	tx, err := d.refund(opts)
	if err != nil {
		return false, fmt.Errorf("submit refund on %s: %s", src.id, rpcsafe.RedactURLs(err.Error()))
	}
	rcpt, err := onchain.WaitMined(ctx, src.client, tx)
	if err != nil {
		return false, err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return false, fmt.Errorf("refund reverted on %s (tx %s)", src.id, tx.Hash().Hex())
	}
	reason := fmt.Sprintf("destination timeout elapsed at %s and source escrow was refunded on %s", d.timeout, src.id)
	_, _ = fmt.Fprintf(
		r.log,
		"ibc relayer: TIMED-OUT %s packet %s (seq %d) -> refund %s on %s\n",
		p.AppType,
		p.PacketID,
		p.Sequence,
		tx.Hash().Hex(),
		src.id,
	)
	if err := r.finishTerminal(received, r.store.MarkTimedOut(ctx, p.PacketID, tx.Hash().Hex(), reason)); err != nil {
		return false, err
	}
	return true, nil
}
