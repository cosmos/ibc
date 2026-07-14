package wire

const RelayPath = "/relay"

type RelayRequest struct {
	SourceChainID string `json:"sourceChainId"`
	SourceTxHash  string `json:"sourceTxHash"`
}

type RelayResult struct {
	PacketIDs []string `json:"packetIds"`
}
