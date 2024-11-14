# Packet Specification

## Packet V2

The IBC packet sends application data from a source chain to a destination chain with a timeout that specifies when the packet is no longer valid. The packet will be committed to by the source chain as specified in the ICS-24 specification. The receiver chain will then verify the packet commitment under the ICS-24 specified packet commitment path. If the proof succeeds, the IBC handler sends the application data(s) to the relevant application(s).

```typescript
interface Packet {
    // identifier for the channel on source chain
    // channel must contain identifier of counterparty channel
    // and the client identifier for the client on source chain
    // that tracks dest chain
    sourceChannel: bytes,
    // identifier for the channel on dest chain
    // channel must contain identifier of counterparty channel
    // and the client identifier for the client on dest chain
    // that tracks source chain
    destChannel: bytes,
    // the sequence uniquely identifies this packet
    // in the stream of packets from source to dest chain
    sequence: uint64,
    // the timeout is the timestamp in seconds on the destination chain
    // at which point the packet is no longer valid.
    // It cannot be received on the destination chain and can
    // be timed out on the source chain
    timeout: uint64,
    // the data includes the messages that are intended
    // to be sent to application(s) on the destination chain
    // from application(s) on the source chain
    // IBC core handlers will route the payload to the desired
    // application using the port identifiers but the rest of the
    // payload will be processed by the application
    data: [Payload]
}

interface Payload {
    // sourcePort identifies the sending application on the source chain
    sourcePort: bytes,
    // destPort identifies the receiving application on the dest chain
    destPort: bytes,
    // version identifies the version that sending application
    // expects destination chain to use in processing the message
    // if dest chain does not support the version, the payload must
    // be rejected with an error acknowledgement
    version: string,
    // encoding allows the sending application to specify which
    // encoding was used to encode the app data
    // the receiving applicaton will decode the appData into
    // the strucure expected given the version provided
    // if the encoding is not supported, receiving application
    // must be rejected with an error acknowledgement.
    // the encoding string MUST be in MIME format
    encoding: string,
    // appData is the opaque content sent from the source application
    // to the dest application. It will be decoded and interpreted
    // as specified by the version and encoding fields
    appData: bytes,
}
```

The source and destination identifiers at the top-level of the packet identifiers the chains communicating. The source identifier **must** be unique on the source chain and is a pointer to the destination chain. The destination identifier **must** be a unique identifier on the destination chain and is a pointer to the source chain. The sequence is a monotonically incrementing nonce to uniquely identify packets sent between the source and destination chain.

The timeout is the UNIX timestamp in seconds that must be passed on the **destination** chain before the packet is invalid and no longer capable of being received. Note that the timeout timestamp is assessed against the destination chain's clock which may drift relative to the clocks of the sender chain or a third party observer. If a packet is received on the destination chain after the timeout timestamp has passed relative to the destination chain's clock; the packet must be rejected so that it can be safely timed out and reverted by the sender chain.

In version 2 of the IBC specification, implementations **MAY** support multiple application data within the same packet. This can be represented by a list of payloads. Implementations may choose to only support a single payload per packet, in which case they can just reject incoming packets sent with multiple payloads.

Each payload will include its own `Encoding` and `AppVersion` that will be sent to the application to instruct it how to decode and interpret the opaque application data. The application must be able to support the provided `Encoding` and `AppVersion` in order to process the `AppData`. If the receiving application does not support the encoding or app version, then the application **must** return an error to IBC core. If the receiving application does support the provided encoding and app version, then the application must decode the application as specified by the `Encoding` enum and then process the application as expected by the counterparty given the agreed-upon app version. Since the `Encoding` and `AppVersion` are now in each packet they can be changed on a per-packet basis and an application can simultaneously support many encodings and app versions from a counterparty. This is in stark contrast to IBC version 1 where the channel prenegotiated the channel version (which implicitly negotiates the encoding as well); so that changing the app version after channel opening is very difficult.

The packet must be committed to as specified in the ICS24 specification. In order to do this we must first commit the packet data and timeout. The timeout is encoded in LittleEndian format. The packet data which is a list of payloads is committed to by hashing each individual field of the payload and successively concatenating them together. This ensures a standard unambigious commitment for a given packet. Thus a given packet will always create the exact same commitment by all compliant implementations and two different packets will never create the same commitment by a compliant implementation.

```typescript
// commitPayload hashes all the fields of the packet data to create a standard size
// preimage before committing it in the packet.
func commitPayload(payload: Payload): bytes {
    buffer = sha256.Hash(payload.sourcePort)
    buffer = append(sha256.Hash(payload.destPort))
    buffer = append(sha256.Hash(payload.version))
    buffer = append(sha256.Hash(payload.encoding))
    buffer = append(sha256.Hash(payload.appData))
    return sha256.Hash(buffer)
}

// commitV2Packet commits to all fields in the packet
// by hashing each individual field and then hashing these fields together
// Note: SourceChannel and the sequence are omitted since they will be included in the key
// Every other field of the packet is committed to in the packet which will be stored in the
// packet commitment value
// The final preimage will be prepended by the byte 0x02 before hashing in order to clearly define the protocol version
// and allow for future upgradability
func commitV2Packet(packet: Packet) {
    timeoutBytes = LittleEndian(packet.timeout)
    var appBytes: bytes
    for p in packet.payload {
        appBytes = append(appBytes, commitPayload(p))
    }
    buffer = sha256.Hash(packet.destChannel)
    buffer = append(buffer, sha256.hash(timeoutBytes))
    buffer = append(buffer, sha256.hash(appBytes))
    buffer = append([]byte{0x02}, buffer)
    return sha256.Hash(buffer)
}
```

## Acknowledgement V2

The acknowledgement in the version 2 specification is also modified to support multiple payloads in the packet that will each go to separate applications that can write their own acknowledgements. Each acknowledgment will be contained within the final packet acknowledgment in the same order that they were received in the original packet. Thus if a packet contains payloads for modules `A` and `B` in that order; the receiver will write an acknowledgment with the app acknowledgements `A` and `B` in the same order.

The acknowledgement which is itself a list of app acknowledgement bytes must be committed to by hashing each individual acknowledgement and concatenating them together and hashing the result. This ensures that all compliant implementations reach the same acknowledgment commitment and that two different acknowledgements never create the same commitment.

An application may not need to return an acknowledgment. In this case, it may return a sentinel acknowledgement value `SENTINEL_ACKNOWLEDGMENT` which will be the single byte in the byte array: `bytes(0x01)`. In this case, the IBC `acknowledgePacket` handler will still do the core IBC acknowledgment logic but it will not call the application's acknowledgePacket callback.

```typescript
interface Acknowledgement {
    // Each app in the payload will have an acknowledgment in this list in the same order
    // that they were received in the payload
    // If an app does not need to send an acknowledgement, there must be a SENTINEL_ACKNOWLEDGEMENT
    // in its place
    // The app acknowledgement must be encoded in the same manner specified in the payload it received
    // and must be created and processed in the manner expected by the version specified in the payload.
    appAcknowledgement: [bytes]
}
```

All acknowledgements must be committed to and stored under the ICS24 acknowledgment path.

```typescript
// commitV2Acknowledgement hashes each app acknowledgment and hashes them together
// the final preimage is then prepended with the byte 0x02 in order to clearly define the protocol version
// and allow for future upgradability
func commitV2Acknowledgment(ack: Acknowledgement) {
    var buffer: bytes
    for appAck in ack.appAcknowledgement {
        buffer = append(buffer, sha256.Hash(appAck))
    }
    buffer = append([]byte{0x02}, buffer)
    return sha256.Hash(buffer)
}
```

