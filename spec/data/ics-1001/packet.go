package main


import "fmt"

// this how event looks in Go and Rust implementations
type SendPacketEvent struct {
    packet_data []byte
    timeout_height Height
    timeout_timestamp uint64
    sequence uint64
    src_port_id Identifier
    src_channel_id Identifier
    dst_port_id Identifier
    dst_channel_id Identifier
    channel_ordering ChannelOrdering
    src_connection_id ConnectionId
}


type ReceivePacketEvent struct {
    packet_data []byte
    timeout_height Height
    timeout_timestamp uint64
    sequence uint64
    src_port_id Identifier
    src_channel_id Identifier
    dst_port_id Identifier
    dst_channel_id Identifier
    channel_ordering ChannelOrdering
    src_connection_id ConnectionId
}

// never changes (at least should not) for specified channel/port source and counterparty
type ChannelOrdering interface {

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
    src_transaction_id []byte
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

    sender string

    packet_data []byte
	// depends on implementation
	packet_hash []byte

	/// on receiver chain (client updates are continuously posted to this chain)
    timeout_height Height
	// on receiver chain (client updates are continuously posted to this chain)
    timeout_timestamp uint64
    channel_ordering ChannelOrdering
	// one of "sent", "try_open", "ack", "timeout", "failed"
	src_state string
	
    /// one of "timeout", "success"
    dst_state *string
    dst_acknowledgement *[]byte
    dst_acknowledgement_hash *[]byte
    
    src_acknowledgement *[]byte

    // basically we are going to track multihop staff, may need some debug field
    index_create_at uint64
    index_updated_at uint64


    /// routing path
    connection_0_relayer *string
    connection_0 Identifier
    connection_0_reached bool
    connection_1_relayer *string
    connection_1 Identifier
    connection_1_reached bool
    // .... up to 8
}



type TransferPacketTrackingRecord struct {
    // natural id
    src_sequence uint64
	src_port_id Identifier
    src_channel_id Identifier
    src_network_id NetworkConnectionId
    
    // src denom, target denom based on `src_port/src_channel/denom` as per spec on target chain
    denom string
    
    memo_json string
    // usually JSON string, but not required to be such
    memo []byte    
}

// usually just string
type Identifier interface {
}

func to_send_packet(packet SendPacketEvent, src_network  NetworkConnectionId, dst_network  NetworkConnectionId) PacketTrackingRecord {
    // set all other fields
	panic("not implemented")
}

func create_packet_tracing(packet PacketTrackingRecord) {
    packet.state = "init"
	panic("store into database here")
}

/// operates in context of some chain subscription/RPC URL
func get_src_networks(packet SendPacketEvent) (NetworkConnectionId, NetworkConnectionId) {
    panic("return src and dst network ids as  tracked by registry")
}

/// operates in context of some chain subscription/RPC URL
func get_dst_networks(packet ReceivePacketEvent) (NetworkConnectionId, NetworkConnectionId) {
    panic("return src and dst network ids as  tracked by registry")
}



func on_src_on_send_packet_event(packet SendPacketEvent) {
    src_network, dst_network := get_src_networks(packet)
	record := to_send_packet(packet, src_network, dst_network)
    create_packet_tracing(record)
    if_ics_20_packet(record)
}

func on_dst_on_receive_packet_event(packet ReceivePacketEvent) {
    src_network, dst_network := get_dst_networks(packet)
	update_packet_tracing_receive(packet)
}

func on_dst_on_packet_receipt_path(packet ReceivePacketEvent) {
    src_network, dst_network := get_dst_networks(packet)
	update_packet_tracing_receive(packet)
    // update on timeout_receipt
    SUCCESSFUL_RECEIPT
}

func main() {

	fmt.Println("Hello, 世界")
}
