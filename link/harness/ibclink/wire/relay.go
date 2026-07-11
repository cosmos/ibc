package wire

// RelayPath is the daemon HTTP path for manual relay requests.
const RelayPath = "/relay"

// RelayRequest asks the daemon to relay packets from one source transaction.
type RelayRequest struct {
	SourceChainID string `json:"sourceChainId"`
	SourceTxHash  string `json:"sourceTxHash"`
}

// RelayResult is the daemon's response: the packet ids the request matched.
type RelayResult struct {
	PacketIDs []string `json:"packetIds"`
}
