package stub

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

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
	"github.com/cosmos/ibc/link/cmd/testappcmd"
)

type relayer struct {
	cfg      *configcmd.Config
	testApps testappcmd.Deployment
	conns    map[string]*chainConn
	store    *stubStore
	log      io.Writer
	// recvCursor/recvSeen: relay loop only; recvActive limits caching to pending packets.
	recvCursor map[string]uint64
	recvSeen   map[receivedKey]receivedResult
	recvActive map[receivedKey]struct{}
	// sentCursor: relay loop only; manual relay uses tx-scoped discovery without cursors.
	sentCursor map[string]uint64
}

type receivedKey struct {
	destination string
	appType     relayercmd.AppType
	routeID     string
	sequence    uint64
}

func receivedKeyFor(destination string, p storedPacket) receivedKey {
	return receivedKey{
		destination: destination,
		appType:     p.AppType,
		routeID:     p.RouteID,
		sequence:    p.Sequence,
	}
}

func (r *relayer) lookupReceived(
	ctx context.Context,
	want receivedKey,
	addr common.Address,
	scan func(
		ctx context.Context,
		fromBlock uint64,
	) (map[receivedEventKey]receivedResult, uint64, error),
) (receivedResult, bool, error) {
	cursor := cursorKey(want.destination, addr)
	results, next, err := scan(ctx, r.recvCursor[cursor])
	if err != nil {
		return receivedResult{}, false, err
	}
	r.recvCursor[cursor] = next
	for eventKey, rec := range results {
		key := receivedKey{
			destination: want.destination,
			appType:     want.appType,
			routeID:     eventKey.RouteID,
			sequence:    eventKey.Sequence,
		}
		if _, pending := r.recvActive[key]; pending {
			r.recvSeen[key] = rec
		}
	}
	rec, ok := r.recvSeen[want]
	return rec, ok, nil
}

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

func (r *relayer) pendingReceivedKeys(packets []storedPacket) map[receivedKey]struct{} {
	active := make(map[receivedKey]struct{}, len(packets))
	for _, p := range packets {
		route, ok := r.cfg.Route(p.RouteID)
		if ok {
			active[receivedKeyFor(route.Destination, p)] = struct{}{}
		}
	}
	return active
}

// Evict recv cache only after the terminal store write succeeds.
func (r *relayer) finishTerminal(key receivedKey, err error) error {
	if err != nil {
		return err
	}
	delete(r.recvActive, key)
	delete(r.recvSeen, key)
	return nil
}

func (r *relayer) reconcilePacket(ctx context.Context, p storedPacket, requested map[relayRequestKey]bool) error {
	route, ok := r.cfg.Route(p.RouteID)
	if !ok {
		return fmt.Errorf("unknown route %q", p.RouteID)
	}
	// Manual routes relay only after /relay persists the request (sqlite survives restart).
	if !route.AutoRelayEnabled() &&
		!requested[relayRequestKey{SourceChainID: route.Source, SourceTxHash: p.SourceTxHash}] {
		return nil
	}
	src, dst, srcApps, dstApps, err := r.routeContext(route)
	if err != nil {
		return err
	}
	switch p.AppType {
	case relayercmd.AppTypeIFT:
		d, err := r.iftDelivery(p, src, dst, srcApps, dstApps)
		if err != nil {
			return err
		}
		return r.deliverPending(ctx, src, dst, p, d)
	case relayercmd.AppTypeGMP:
		d, err := r.gmpDelivery(p, dst, dstApps)
		if err != nil {
			return err
		}
		return r.deliverPending(ctx, src, dst, p, d)
	default:
		return fmt.Errorf("unknown app type %q", p.AppType)
	}
}

func (r *relayer) routeContext(
	route configcmd.Route,
) (*chainConn, *chainConn, testappcmd.ChainDeployment, testappcmd.ChainDeployment, error) {
	src, ok := r.conns[route.Source]
	if !ok {
		return nil, nil, testappcmd.ChainDeployment{}, testappcmd.ChainDeployment{}, fmt.Errorf(
			"source chain %s not connected",
			route.Source,
		)
	}
	dst, ok := r.conns[route.Destination]
	if !ok {
		return nil, nil, testappcmd.ChainDeployment{}, testappcmd.ChainDeployment{}, fmt.Errorf(
			"destination chain %s not connected",
			route.Destination,
		)
	}
	srcApps, ok := r.testApps.Chain(route.Source)
	if !ok {
		return nil, nil, testappcmd.ChainDeployment{}, testappcmd.ChainDeployment{}, fmt.Errorf(
			"no test app deployment for source chain %s",
			route.Source,
		)
	}
	dstApps, ok := r.testApps.Chain(route.Destination)
	if !ok {
		return nil, nil, testappcmd.ChainDeployment{}, testappcmd.ChainDeployment{}, fmt.Errorf(
			"no test app deployment for destination chain %s",
			route.Destination,
		)
	}
	return src, dst, srcApps, dstApps, nil
}

type delivery struct {
	seq          *big.Int
	findReceived func(ctx context.Context) (receivedResult, bool, error)
	deliver      func(opts *bind.TransactOpts) (*types.Transaction, error)
	effect       string

	timeout      *big.Int
	refund       func(opts *bind.TransactOpts) (*types.Transaction, error)
	findRefunded func(ctx context.Context, seq *big.Int) (common.Hash, bool, error)
	ackOK        func(rcpt *types.Receipt) (bool, error)
}

func (r *relayer) iftDelivery(
	p storedPacket,
	src, dst *chainConn,
	srcApps, dstApps testappcmd.ChainDeployment,
) (delivery, error) {
	seq := new(big.Int).SetUint64(p.Sequence)
	amount, ok := new(big.Int).SetString(p.Amount, 10)
	if !ok {
		return delivery{}, fmt.Errorf("invalid IFT amount %q", p.Amount)
	}
	dstIFT := newTestAppIFT(common.HexToAddress(dstApps.MockIFT), dst.client)
	receiver := common.HexToAddress(p.Receiver)
	received := receivedKeyFor(dst.id, p)
	timeout := big.NewInt(0)
	if p.TimeoutTimestamp != "" {
		var ok bool
		timeout, ok = new(big.Int).SetString(p.TimeoutTimestamp, 10)
		if !ok {
			return delivery{}, fmt.Errorf("invalid IFT timeout timestamp %q", p.TimeoutTimestamp)
		}
	}
	srcIFT := newTestAppIFT(common.HexToAddress(srcApps.MockIFT), src.client)
	return delivery{
		seq: seq,
		findReceived: func(ctx context.Context) (receivedResult, bool, error) {
			return r.lookupReceived(ctx, received, dstIFT.Address, dstIFT.ScanReceivedFrom)
		},
		deliver: func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return dstIFT.ReceiveTransfer(opts, p.RouteID, seq, receiver, amount)
		},
		effect:  "recv",
		timeout: timeout,
		refund: func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return srcIFT.Refund(opts, seq)
		},
		findRefunded: srcIFT.FindRefunded,
	}, nil
}

func (r *relayer) gmpDelivery(
	p storedPacket,
	dst *chainConn,
	dstApps testappcmd.ChainDeployment,
) (delivery, error) {
	seq := new(big.Int).SetUint64(p.Sequence)
	payload, err := hexutil.Decode(p.Payload)
	if err != nil {
		return delivery{}, fmt.Errorf("invalid GMP payload %q: %w", p.Payload, err)
	}
	dstGMP := newTestAppGMP(common.HexToAddress(dstApps.MockGMP), dst.client)
	target := common.HexToAddress(p.Target)
	received := receivedKeyFor(dst.id, p)
	return delivery{
		seq: seq,
		findReceived: func(ctx context.Context) (receivedResult, bool, error) {
			return r.lookupReceived(ctx, received, dstGMP.Address, dstGMP.ScanReceivedFrom)
		},
		deliver: func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return dstGMP.Deliver(opts, p.RouteID, seq, target, payload)
		},
		ackOK: func(rcpt *types.Receipt) (bool, error) {
			ok, found, err := dstGMP.DeliveredSuccessFromReceipt(rcpt)
			if err != nil {
				return false, err
			}
			if !found {
				return false, fmt.Errorf("deliver tx %s emitted no gmpReceived", rcpt.TxHash.Hex())
			}
			return ok, nil
		},
		effect: "deliver",
	}, nil
}

// Record destination effect in store before marking complete; idempotency scan recovers crash mid-flight.
func (r *relayer) deliverPending(ctx context.Context, src, dst *chainConn, p storedPacket, d delivery) error {
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

	if rec, exists, err := d.findReceived(ctx); err != nil {
		return err
	} else if exists {
		if !rec.Success {
			reason := fmt.Sprintf("destination call reverted on %s (error acknowledgement)", dst.id)
			return r.finishTerminal(received, r.store.MarkErrorAck(ctx, p.PacketID, rec.TxHash.Hex(), reason))
		}
		return r.finishTerminal(received, r.store.MarkComplete(ctx, p.PacketID, rec.TxHash.Hex()))
	}

	opts, err := newTransactor(ctx, dst.signerKey, dst.chainID)
	if err != nil {
		return err
	}
	tx, err := d.deliver(opts)
	if err != nil {
		return fmt.Errorf("submit %s: %w", d.effect, err)
	}
	rcpt, err := waitMined(ctx, dst.client, tx)
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

func (r *relayer) maybeTimeout(ctx context.Context, src, dst *chainConn, p storedPacket, d delivery) (bool, error) {
	received := receivedKeyFor(dst.id, p)
	hdr, err := dst.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("destination head for timeout check: %w", err)
	}
	if new(big.Int).SetUint64(hdr.Time).Cmp(d.timeout) < 0 {
		return false, nil
	}
	if _, exists, receivedErr := d.findReceived(ctx); receivedErr != nil {
		return false, receivedErr
	} else if exists {
		return false, nil
	}

	// Adopt an on-chain refund before re-issuing — a second refund reverts and wedges the packet pending.
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

	opts, err := newTransactor(ctx, src.signerKey, src.chainID)
	if err != nil {
		return false, err
	}
	tx, err := d.refund(opts)
	if err != nil {
		return false, fmt.Errorf("submit refund on %s: %w", src.id, err)
	}
	rcpt, err := waitMined(ctx, src.client, tx)
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
