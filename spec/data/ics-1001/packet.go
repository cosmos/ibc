package main

// never changes (at least should not) for specified channel/port source and counterparty
type ChannelOrdering interface {

}

type ComposableMemo interface {

}

type OsmosisSwapMemo interface {

}


type IbcCwHookMemo interface {

}

type PortForwardMemo interface {

}


// defines "network(consensus)"(along with client id), see relevant specs
type ConnectionId interface {

}

// usually block number
type Height interface {
}

// as obtained from some network/client/connection registry
// can be connection id, client id, with reference to some human name and genesis hash
type NetworkConnectionId interface {

}

// is used to track packet
type PacketTrackingRecord struct {
    // block height(usually number) when it was sent  
    src_sent_height Height
    // timestamp when it was sent
    src_sent_timestamp uint64

    // sequence is same on dst chain, so sequence with src/dst port and channel are unique across two  chains
    src_sequence uint64
	src_port_id Identifier
    src_channel_id Identifier
    // network id makes this globally unique
    src_network_id NetworkConnectionId
    dst_port_id Identifier
    dst_channel_id Identifier
    dst_network_id NetworkConnectionId
    // will be known when received by destination chain
    dst_sequence *uint64

    packet_data []byte
	// depends on implementation
	packet_hash []byte

	/// on receiver chain (client updates are continuously posted to this chain)
    timeout_height Height
	// on receiver chain (client updates are continuously posted to this chain)
    timeout_timestamp uint64
    channel_ordering ChannelOrdering

    /// one of "timeout", "success"
    dst_receipt_state *string
    // acknowledgement
    dst_ack *[]byte
    
    // either none are set of one of two, can be set if and only `dst_ack` was set
    dst_ack_one_of_result *[]byte
    dst_ack_one_of_error *string

    dst_ack_hash *[]byte
    
    src_ack_sequence_id *uint64
    /// so as soon as packet committment deleted, set to true
    src_commitment_deleted bool

    /// this is calculated field, happens when IBC client update brings counter party height and timestamp, so it can be compared with sent packet chain
    /// channel closure sets timeout too 
    src_timeout bool

    // basically we are going to track multihop staff, may need some debug field
    index_create_at uint64
    index_updated_at uint64

    /// routing path

    // so we can lookup sender
    src_tx_id []byte
    // so we know that packet was delivered with some transaction and can lookup relayer if needed
    connection_0_tx_id *[]byte
    connection_0 Identifier
    connection_1_tx_id *[]byte
    connection_1 Identifier
    // .... up to 8
}

// if by events/logs order or data parsing detected it is transfer event
type TransferPacketTrackingRecord struct {
    // natural id
    src_sequence uint64
	src_port_id Identifier
    src_channel_id Identifier
    src_network_id NetworkConnectionId
    
    // whether the token originates from this chain
    is_source bool
    // src denom, target denom based on `src_port/src_channel/denom` as per spec on target chain
    denom string
    amount_u256 string

    // address in very free form
    sender string
    receiver string
    /// specific acknowledgement as defined by ICS-20 module
    src_ack *string

    success *bool
    
    // usually JSON string, but not required to be such
    memo *[]byte
    memo_json *string
    
    memo_one_of_packet_forwarding *PortForwardMemo
    memo_one_of_packet_swap *OsmosisSwapMemo
    memo_one_of_packet_wasm *IbcCwHookMemo
    memo_one_of_packet_forward_xcm *ComposableMemo
}

// usually just string
type Identifier interface {
}

func main() {
}
