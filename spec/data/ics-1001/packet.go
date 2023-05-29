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

// is used to track packet
type PacketTrackingRecord struct {
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
	// one of "init", "try_open", "ack", "timeout", "failed"
	state string
}

type TransferPacketTrackingRecord struct {
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

func get_networks(packet SendPacketEvent) (NetworkConnectionId, NetworkConnectionId) {
    panic("return src and dst network ids as  tracked by registry")
}

// 1. 
func on_source_on_send_packet_event(packet SendPacketEvent) {
    src_network, dst_network := get_networks(packet)
	record := to_send_packet(packet, src_network, dst_network)
    create_packet_tracing(record)
}

func main() {
	fmt.Println("Hello, 世界")
}
