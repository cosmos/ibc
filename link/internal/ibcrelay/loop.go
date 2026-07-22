package ibcrelay

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/cmd/configcmd"
)

const errorAckReason = "destination application reverted (error acknowledgement)"

type relayer struct {
	cfg        *configcmd.Config
	conns      map[string]*chainConn
	store      *relayStore
	log        io.Writer
	sentCursor map[string]uint64
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
	src, dst, err := r.routeContext(route)
	if err != nil {
		return err
	}

	if timedOut, timeoutErr := r.maybeTimeout(ctx, src, dst, p); timeoutErr != nil {
		return timeoutErr
	} else if timedOut {
		return nil
	}

	if p.AckHex == "" {
		if terminal, adoptErr := r.adoptSourceAck(ctx, src, p); adoptErr != nil {
			return adoptErr
		} else if terminal {
			return nil
		}
		if rec, found, findErr := dst.ops.findWriteAck(ctx, p.Packet.DestClient, p.Packet.Sequence); findErr != nil {
			return findErr
		} else if found {
			if markErr := r.store.MarkReceived(ctx, p.PacketID, rec.TxHash.Hex(), ackHex(rec.Ack)); markErr != nil {
				return markErr
			}
			p.AckHex = ackHex(rec.Ack)
			p.RecvTxHash = rec.TxHash.Hex()
		} else {
			rcpt, recvErr := dst.ops.submitRecv(ctx, p.Packet)
			if recvErr != nil {
				return recvErr
			}
			ack, hasAck, parseErr := dst.ops.writeAckFromReceipt(rcpt)
			if parseErr != nil {
				return parseErr
			}
			if !hasAck {
				rec, found, findErr := dst.ops.findWriteAck(ctx, p.Packet.DestClient, p.Packet.Sequence)
				if findErr != nil {
					return findErr
				}
				if !found {
					return fmt.Errorf("recv tx %s emitted no WriteAcknowledgement", rcpt.TxHash.Hex())
				}
				ack = rec.Ack
			}
			if markErr := r.store.MarkReceived(ctx, p.PacketID, rcpt.TxHash.Hex(), ackHex(ack)); markErr != nil {
				return markErr
			}
			p.AckHex = ackHex(ack)
			p.RecvTxHash = rcpt.TxHash.Hex()
		}
	}

	return r.submitAckLeg(ctx, src, p)
}

func (r *relayer) routeContext(route configcmd.Route) (*chainConn, *chainConn, error) {
	src, ok := r.conns[route.Source]
	if !ok {
		return nil, nil, fmt.Errorf("source chain %s not connected", route.Source)
	}
	dst, ok := r.conns[route.Destination]
	if !ok {
		return nil, nil, fmt.Errorf("destination chain %s not connected", route.Destination)
	}
	return src, dst, nil
}

func (r *relayer) maybeTimeout(
	ctx context.Context,
	src, dst *chainConn,
	p storedPacket,
) (bool, error) {
	if p.Packet.TimeoutTimestamp == 0 {
		return false, nil
	}
	now, err := dst.ops.blockTimestamp(ctx)
	if err != nil {
		return false, fmt.Errorf("destination head for timeout check: %w", err)
	}
	if now < p.Packet.TimeoutTimestamp {
		return false, nil
	}
	if _, exists, receivedErr := dst.ops.findWriteAck(ctx, p.Packet.DestClient, p.Packet.Sequence); receivedErr != nil {
		return false, receivedErr
	} else if exists {
		return false, nil
	}

	if timeoutTx, timedOut, findErr := src.ops.findTimeoutPacket(
		ctx,
		p.Packet.SourceClient,
		p.Packet.Sequence,
	); findErr != nil {
		return false, findErr
	} else if timedOut {
		reason := fmt.Sprintf(
			"destination timeout elapsed at %d and packet was already timed out on %s",
			p.Packet.TimeoutTimestamp, src.id,
		)
		if markErr := r.store.MarkTimedOut(ctx, p.PacketID, timeoutTx.Hex(), reason); markErr != nil {
			return false, markErr
		}
		return true, nil
	}

	if terminal, adoptErr := r.adoptSourceAck(ctx, src, p); adoptErr != nil {
		return false, adoptErr
	} else if terminal {
		return true, nil
	}

	rcpt, err := src.ops.submitTimeout(ctx, p.Packet)
	if err != nil {
		return false, err
	}
	reason := fmt.Sprintf(
		"destination timeout elapsed at %d and TimeoutPacket submitted on %s",
		p.Packet.TimeoutTimestamp, src.id,
	)
	_, _ = fmt.Fprintf(
		r.log,
		"ibc relayer: TIMED-OUT packet %s (seq %d) -> timeout %s on %s\n",
		p.PacketID,
		p.Packet.Sequence,
		rcpt.TxHash.Hex(),
		src.id,
	)
	if err := r.store.MarkTimedOut(ctx, p.PacketID, rcpt.TxHash.Hex(), reason); err != nil {
		return false, err
	}
	return true, nil
}

func (r *relayer) adoptSourceAck(ctx context.Context, src *chainConn, p storedPacket) (bool, error) {
	rec, found, err := src.ops.findAckPacket(ctx, p.Packet.SourceClient, p.Packet.Sequence)
	if err != nil || !found {
		return false, err
	}
	if isErrorAck(rec.Ack) {
		return true, r.store.MarkErrorAck(ctx, p.PacketID, rec.TxHash.Hex(), errorAckReason)
	}
	return true, r.store.MarkComplete(ctx, p.PacketID, rec.TxHash.Hex())
}

func (r *relayer) submitAckLeg(
	ctx context.Context,
	src *chainConn,
	p storedPacket,
) error {
	if terminal, adoptErr := r.adoptSourceAck(ctx, src, p); adoptErr != nil {
		return adoptErr
	} else if terminal {
		return nil
	}

	ack, err := hexutil.Decode(p.AckHex)
	if err != nil {
		return fmt.Errorf("decode stored ack %q: %w", p.AckHex, err)
	}
	rcpt, err := src.ops.submitAck(ctx, p.Packet, ack)
	if err != nil {
		return err
	}
	if isErrorAck(ack) {
		_, _ = fmt.Fprintf(
			r.log,
			"ibc relayer: ERROR-ACK packet %s (seq %d) -> ack %s on %s: %s\n",
			p.PacketID,
			p.Packet.Sequence,
			rcpt.TxHash.Hex(),
			src.id,
			errorAckReason,
		)
		return r.store.MarkErrorAck(ctx, p.PacketID, rcpt.TxHash.Hex(), errorAckReason)
	}
	_, _ = fmt.Fprintf(
		r.log,
		"ibc relayer: relayed packet %s on route %s (seq %d) -> ack %s on %s\n",
		p.PacketID,
		p.RouteID,
		p.Packet.Sequence,
		rcpt.TxHash.Hex(),
		src.id,
	)
	return r.store.MarkComplete(ctx, p.PacketID, rcpt.TxHash.Hex())
}
