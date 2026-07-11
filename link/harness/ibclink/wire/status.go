package wire

const StatusPath = "/status"

const (
	StatusQueryRoute  = "route"
	StatusQueryPacket = "packet"
)

// The zero value asks for everything.
type StatusQuery struct {
	RouteID  string
	PacketID string
}

type PacketState string

const (
	PacketPending  PacketState = "pending"
	PacketComplete PacketState = "complete"
	PacketTimedOut PacketState = "timed_out"
	PacketErrorAck PacketState = "error_ack"
)

type Status struct {
	Packets []PacketStatus `json:"packets"`
}

// Terminal states are persisted only after their on-chain effect lands.
type PacketStatus struct {
	PacketID     string      `json:"packetId"`
	RouteID      string      `json:"routeId"`
	Sequence     uint64      `json:"sequence"`
	State        PacketState `json:"state"`
	SourceTxHash string      `json:"sourceTxHash"`
	EffectTxHash string      `json:"effectTxHash,omitempty"`
	Reason       string      `json:"reason,omitempty"`
}

func (s *Status) Packet(packetID string) (PacketStatus, bool) {
	for _, p := range s.Packets {
		if p.PacketID == packetID {
			return p, true
		}
	}
	return PacketStatus{}, false
}
