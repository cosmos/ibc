package wire

// StatusPath is the daemon HTTP path for status queries.
const StatusPath = "/status"

const (
	// StatusQueryRoute filters status by route id.
	StatusQueryRoute = "route"
	// StatusQueryPacket filters status by packet id.
	StatusQueryPacket = "packet"
)

// StatusQuery filters the daemon status API. The zero value asks for everything.
type StatusQuery struct {
	RouteID  string
	PacketID string
}

// PacketState is the lifecycle state of a single relayed packet. It is a typed string so both the stub
// (which sets it) and the harness/correlator (which asserts on it) reference the same constants instead
// of bare literals, and so an unknown value is obvious.
type PacketState string

const (
	// PacketPending means the source accepted the packet and no terminal effect is known yet.
	PacketPending PacketState = "pending" // accepted on source; no terminal on-chain effect yet
	// PacketComplete means the destination receive and acknowledgement were observed.
	PacketComplete PacketState = "complete" // destination recv + ack observed (the happy path)
	// PacketTimedOut means the timeout elapsed before delivery.
	PacketTimedOut PacketState = "timed_out" // timeout elapsed before delivery
	// PacketErrorAck means the destination rejected the packet and returned an error acknowledgement.
	PacketErrorAck PacketState = "error_ack" // destination rejected; error acknowledgement returned
)

// Status is the daemon status API response and the harness's cross-check surface for packet lifecycle.
// It is the better signal for pending packets, where no on-chain event has fired yet and on-chain state
// alone cannot distinguish "in flight" from "lost".
type Status struct {
	Packets []PacketStatus `json:"packets"`
}

// PacketStatus is the per-packet view the harness correlates against its independently-observed
// on-chain effects. SourceTxHash records the source submission; EffectTxHash records the terminal
// on-chain effect when one exists (destination receive/deliver, source refund for timeout).
//
// Contract: the daemon flips State to a terminal value only AFTER the corresponding on-chain effect
// lands (EffectTxHash records it). A wait on the status row is therefore an effect wait seen through
// the daemon — it budgets against the observed chain like any other effect wait, and once the harness
// has seen the effect on-chain the row trails by at most the daemon's persist lag.
type PacketStatus struct {
	PacketID     string      `json:"packetId"`
	RouteID      string      `json:"routeId"`
	Sequence     uint64      `json:"sequence"`
	State        PacketState `json:"state"`
	SourceTxHash string      `json:"sourceTxHash"`           // source-chain submission
	EffectTxHash string      `json:"effectTxHash,omitempty"` // terminal on-chain effect, when present
	// Reason is the human explanation for a terminal failure. Empty (and dropped by omitempty) for a
	// healthy pending/complete packet.
	Reason string `json:"reason,omitempty"`
}

// Packet returns the PacketStatus for a packet id.
func (s *Status) Packet(packetID string) (PacketStatus, bool) {
	for _, p := range s.Packets {
		if p.PacketID == packetID {
			return p, true
		}
	}
	return PacketStatus{}, false
}
