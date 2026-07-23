package processors

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// batchPacketEvents reads packet events for a batch of transfers, one
// chains.Client.TxPacketEvents call per distinct tx hash (as reported by
// hashOf) rather than one per transfer. A failure decoding or fetching one
// hash poisons only the transfers that depend on it (tr.ProcessingError is
// set, excluding them from this batch so they're retried next run) instead
// of failing the whole batch — transfers whose hash succeeded still get
// their events returned and go on to be relayed.
func batchPacketEvents(
	ctx context.Context,
	client chains.Client,
	transfers []*Transfer,
	hashOf func(*Transfer) (string, bool),
) []v2.PacketEvent {
	var order []string

	groups := make(map[string][]*Transfer)

	for _, tr := range transfers {
		hash, ok := hashOf(tr)
		if !ok {
			tr.ProcessingError = errors.New("missing required tx hash")

			continue
		}

		if _, seen := groups[hash]; !seen {
			order = append(order, hash)
		}

		groups[hash] = append(groups[hash], tr)
	}

	var events []v2.PacketEvent

	for _, hash := range order {
		group := groups[hash]

		txID, err := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
		if err != nil {
			poison(group, errors.Wrapf(err, "decoding tx hash %q", hash))

			continue
		}

		txEvents, err := client.TxPacketEvents(ctx, txID)
		if err != nil {
			poison(group, errors.Wrapf(err, "reading packet events for tx %s", hash))

			continue
		}

		keep := make(map[uint64]struct{}, len(group))
		for _, tr := range group {
			keep[tr.PacketSequenceNumber] = struct{}{}
		}

		for _, event := range txEvents {
			if _, ok := keep[event.Packet.Sequence]; ok {
				events = append(events, event)
			}
		}
	}

	return events
}

func poison(transfers []*Transfer, err error) {
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
