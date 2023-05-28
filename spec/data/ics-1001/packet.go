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

// natural unique packet identifier
type PacketTrackingId struct {    
	sequence uint64
	src_port_id Identifier
    src_channel_id Identifier
    src_network_id NetworkConnectionId
}

// is used to track packet
type PacketTrackingRecord struct {
    id PacketTrackingId
	packet_data []byte
	// depends on implementation
	packet_hash []byte

	/// on receiver chain (client updates are continuously posted to this chain)
    timeout_height Height
	// on receiver chain (client updates are continuously posted to this chain)
    timeout_timestamp uint64

    // sequence is same on dst chain, so sequence with dst port and channel are second natural id

    // port+channel combination are unique per 
	dst_port_id Identifier
    dst_channel_id Identifier
    dst_network_id NetworkConnectionId

    channel_ordering ChannelOrdering
	// one of "init", "try_open", "ack", "timeout", "failed"
	state string
}



type TransferPacketTrackingRecord struct {
}

// usually just string
type Identifier interface {
}

func to_send_packet(packet SendPacketEvent) PacketTrackingRecord {
	panic("not implemented")
}

func create_packet_tracing(packet PacketTrackingRecord) {
    packet.state = "init"
	panic("store into database here")
}

func get_dst_connection_id()

func on_send_packet_event(packet SendPacketEvent) {

	record := to_send_packet(packet)
    create_packet_tracing(record)
}

func main() {
	fmt.Println("Hello, 世界")
}
