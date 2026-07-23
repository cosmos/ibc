package processors

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// batchPacketEvents reads packet events for a batch of transfers, one
// chains.Client.TxPacketEvents call per distinct tx hash rather than one per
// transfer. Which of a transfer's tx hashes to read depends on kind: recv
// and timeout both prove a claim against the source chain, so they read
// SourceTxHash; ack reads WriteAckTxHash, the recv tx on the destination
// chain that recorded the acknowledgement. A transfer missing the hash kind
// needs, or a hash that fails to decode or fetch, poisons only the
// transfers that depend on it (tr.ProcessingError is set, excluding them
// from this batch so they're retried next run) instead of failing the whole
// batch — transfers whose hash succeeded still get their events returned
// and go on to be relayed. A hash's tx can carry events for packets outside
// this batch (e.g. other transfers batched into the same send tx), so
// returned events are filtered down to just this batch's sequences; that
// filter is a single set built once, since sequences are unique across the
// whole batch (see transfersBySequence) and don't need to be scoped to the
// hash that happened to report them.
//
// For recv and ack, events are additionally filtered down to what's provable
// at height (see filterProvableEvents): both prove a membership claim against
// a specific height, so an event past it can't be proven yet. Timeout proves
// non-membership against the destination chain's provable time instead, an
// orthogonal check callers still need to apply themselves via
// filterProvableTimeouts.
func batchPacketEvents(
	ctx context.Context,
	client chains.Client,
	transfers []*Transfer,
	kind txbuilder.RelayKind,
	height uint64,
) []v2.PacketEvent {
	var order []string

	transfersByHash := make(map[string][]*Transfer)
	keep := make(map[uint64]struct{}, len(transfers))

	for _, tr := range transfers {
		hash, ok := txHashFor(tr, kind)
		if !ok {
			tr.ProcessingError = errors.New("missing required tx hash")

			continue
		}

		if _, seen := transfersByHash[hash]; !seen {
			order = append(order, hash)
		}

		transfersByHash[hash] = append(transfersByHash[hash], tr)
		keep[tr.PacketSequenceNumber] = struct{}{}
	}

	var events []v2.PacketEvent

	for _, hash := range order {
		group := transfersByHash[hash]

		txID, err := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
		if err != nil {
			markProcessingError(group, errors.Wrapf(err, "decoding tx hash %q", hash))

			continue
		}

		txEvents, err := client.TxPacketEvents(ctx, txID)
		if err != nil {
			markProcessingError(group, errors.Wrapf(err, "reading packet events for tx %s", hash))

			continue
		}

		for _, event := range txEvents {
			if _, ok := keep[event.Packet.Sequence]; ok {
				events = append(events, event)
			}
		}
	}

	if kind == txbuilder.KindRecv || kind == txbuilder.KindAck {
		events = filterProvableEvents(events, height, transfers)
	}

	return events
}

// txHashFor picks the tx hash to read tr's packet events from for kind. ok
// is false if that hash hasn't been recorded yet (only possible for ack:
// WriteAckTxHash is set once the recv tx lands, whereas SourceTxHash is
// always known once a transfer exists at all).
func txHashFor(tr *Transfer, kind txbuilder.RelayKind) (hash string, ok bool) {
	switch kind {
	case txbuilder.KindRecv, txbuilder.KindTimeout:
		return tr.SourceTxHash, true
	case txbuilder.KindAck:
		if tr.WriteAckTxHash == nil {
			return "", false
		}

		return *tr.WriteAckTxHash, true
	default:
		return "", false
	}
}

func markProcessingError(transfers []*Transfer, err error) {
	for _, tr := range transfers {
		tr.ProcessingError = err
	}
}

// transfersBySequence indexes transfers by packet sequence, for matching
// events (which carry the packet but not the transfer) back to the transfer
// that reported them. Sequences are unique within a batch: every transfer in
// it shares the same route, so (chain, client) is fixed and the sequence
// alone identifies the packet.
func transfersBySequence(transfers []*Transfer) map[uint64]*Transfer {
	bySeq := make(map[uint64]*Transfer, len(transfers))

	for _, tr := range transfers {
		bySeq[tr.PacketSequenceNumber] = tr
	}

	return bySeq
}

// filterProvableEvents keeps only the events observed at or below height --
// the height a membership proof can currently be generated for -- and
// poisons the transfer behind any excluded event so it's retried once the
// attestor quorum catches up, instead of blocking the rest of the batch on
// whichever packet happens to be the most recent.
func filterProvableEvents(events []v2.PacketEvent, height uint64, transfers []*Transfer) []v2.PacketEvent {
	bySeq := transfersBySequence(transfers)

	provable := make([]v2.PacketEvent, 0, len(events))

	for _, event := range events {
		if event.Height <= height {
			provable = append(provable, event)

			continue
		}

		if tr, ok := bySeq[event.Packet.Sequence]; ok {
			tr.ProcessingError = errors.Errorf(
				"packet observed at height %d exceeds currently provable height %d", event.Height, height,
			)
		}
	}

	return provable
}

// filterProvableTimeouts keeps only the events whose transfer's timeout has
// actually elapsed as of provableTime -- the counterparty chain's timestamp
// at the height a non-membership proof can currently be generated for -- and
// poisons the rest so they're retried once the attestor quorum catches up to
// the timeout, instead of blocking the rest of the batch.
func filterProvableTimeouts(events []v2.PacketEvent, provableTime time.Time, transfers []*Transfer) []v2.PacketEvent {
	bySeq := transfersBySequence(transfers)

	provable := make([]v2.PacketEvent, 0, len(events))

	for _, event := range events {
		tr, ok := bySeq[event.Packet.Sequence]
		if !ok {
			continue
		}

		if tr.PacketTimeoutTimestamp.After(provableTime) {
			tr.ProcessingError = errors.Errorf(
				"packet timeout %s is after the currently provable timestamp %s",
				tr.PacketTimeoutTimestamp, provableTime,
			)

			continue
		}

		provable = append(provable, event)
	}

	return provable
}

// filterAcksPresent keeps only events carrying a recorded acknowledgement,
// poisoning the transfer behind any without one: the recv succeeded but its
// write-ack was never actually recorded, which should never happen but must
// not silently build a malformed ack item if it somehow does.
func filterAcksPresent(events []v2.PacketEvent, transfers []*Transfer) []v2.PacketEvent {
	bySeq := transfersBySequence(transfers)

	present := make([]v2.PacketEvent, 0, len(events))

	for _, event := range events {
		if len(event.Acks) == 0 {
			if tr, ok := bySeq[event.Packet.Sequence]; ok {
				tr.ProcessingError = errors.Errorf("no acknowledgement recorded for sequence %d", event.Packet.Sequence)
			}

			continue
		}

		present = append(present, event)
	}

	return present
}

// buildRelayTx generates a state proof and per-packet proofs for events at
// height, packs them into relayKind-tagged items (order-preserved, one per
// event), and asks txBuilder for the resulting transaction -- waiting for
// client's chain time to catch up first, since gas estimation during
// delivery reverts otherwise. clientID is the destination client the state
// proof updates.
func buildRelayTx(
	ctx context.Context,
	client chains.Client,
	proofGen proofgen.ProofGenerator,
	txBuilder txbuilder.TxBuilder,
	clientID string,
	kind proofgen.ProofKind,
	relayKind txbuilder.RelayKind,
	height uint64,
	events []v2.PacketEvent,
) (txbuilder.RelayTx, error) {
	stateProof, err := proofGen.StateProof(ctx, height)
	if err != nil {
		return txbuilder.RelayTx{}, errors.Wrap(err, "generating state proof")
	}

	packets := make([]channeltypesv2.Packet, len(events))
	for i, event := range events {
		packets[i] = event.Packet
	}

	packetProofs, err := proofGen.PacketProofs(ctx, height, kind, packets)
	if err != nil {
		return txbuilder.RelayTx{}, errors.Wrap(err, "generating packet proofs")
	}

	items := make([]txbuilder.PacketRelayItem, len(events))
	for i, event := range events {
		items[i] = txbuilder.PacketRelayItem{
			Kind:        relayKind,
			Packet:      event.Packet,
			Acks:        event.Acks,
			Proof:       packetProofs[i],
			ProofHeight: height,
		}
	}

	relayTxs, err := txBuilder.BuildRelayTxs(txbuilder.ClientUpdate{
		ClientID:   clientID,
		StateProof: stateProof,
	}, items)
	if err != nil {
		return txbuilder.RelayTx{}, errors.Wrap(err, "building relay tx")
	}

	if len(relayTxs) != 1 {
		return txbuilder.RelayTx{}, errors.Errorf("expected exactly one relay tx, got %d", len(relayTxs))
	}

	// the chain must be caught up to the current time before gas estimation
	// during delivery, or the tx reverts
	waitCtx, cancel := context.WithTimeout(ctx, waitForChainTimeout)
	defer cancel()

	if err := client.WaitForChain(waitCtx); err != nil {
		return txbuilder.RelayTx{}, errors.Wrap(err, "waiting for chain")
	}

	return relayTxs[0], nil
}
