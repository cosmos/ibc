// Package relayercmd owns the relayer command's wire contract.
package relayercmd

// RelayRequest identifies source traffic for manual relay.
type RelayRequest struct {
	SourceChainID string `json:"sourceChainId"`
	SourceTxHash  string `json:"sourceTxHash"`
}

// PacketState is the acceptance-test view of a packet's relay state.
type PacketState string

const (
	// PacketPending has no terminal on-chain effect yet.
	PacketPending PacketState = "pending"
	// PacketComplete was delivered successfully.
	PacketComplete PacketState = "complete"
	// PacketTimedOut was refunded after timeout.
	PacketTimedOut PacketState = "timed_out"
	// PacketErrorAck was delivered with an error acknowledgement.
	PacketErrorAck PacketState = "error_ack"
)

// PacketStatus describes one packet's relay state.
type PacketStatus struct {
	PacketID     string      `json:"packetId"`
	RouteID      string      `json:"routeId"`
	Sequence     uint64      `json:"sequence"`
	State        PacketState `json:"state"`
	SourceTxHash string      `json:"sourceTxHash"`
	EffectTxHash string      `json:"effectTxHash,omitempty"`
}
