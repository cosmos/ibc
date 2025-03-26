# Packet Structure and Provable Commitment Specification

## Packet V2 Structure

The IBC packet sends application data from a source chain to a destination chain with a timeout that specifies when the packet is no longer valid. The packet will be committed to by the source chain as specified in the ICS-24 specification. The receiver chain will then verify the packet commitment under the ICS-24 specified packet commitment path. If the proof succeeds, the IBC handler sends the application data(s) to the relevant application(s).

```typescript
interface Packet {
    // identifier for the destination-chain client existing on source chain
    sourceClientId: bytes,
    // identifier for the source-chain client existing on destination chain
    destClientId: bytes,
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

The source and destination client identifiers at the top-level of the packet identify the chains communicating. The `sourceClientId` identifier **must** be unique on the source chain and is a pointer to the destination chain client on the source chain. The `destClientId` identifier **must** be a unique identifier on the destination chain and is a pointer to the source chain client on the destination chain. The sequence is a monotonically incrementing nonce to uniquely identify packets sent between the source and destination chain.

The timeout is the UNIX timestamp in seconds that must be passed on the **destination** chain before the packet is invalid and no longer capable of being received. Note that the timeout timestamp is assessed against the destination chain's clock which may drift relative to the clocks of the sender chain or a third party observer. If a packet is received on the destination chain after the timeout timestamp has passed relative to the destination chain's clock; the packet must be rejected so that it can be safely timed out and reverted by the sender chain.

In version 2 of the IBC specification, implementations **MAY** support multiple application data within the same packet. This can be represented by a list of payloads. Implementations may choose to only support a single payload per packet, in which case they can just reject incoming packets sent with multiple payloads.

Each payload will include its own `Encoding` and `AppVersion` that will be sent to the application to instruct it how to decode and interpret the opaque application data. The application must be able to support the provided `Encoding` and `AppVersion` in order to process the `AppData`. If the receiving application does not support the encoding or app version, then the application **must** return an error to IBC core. If the receiving application does support the provided encoding and app version, then the application must decode the application as specified by the `Encoding` string and then process the application as expected by the counterparty given the agreed-upon app version. Since the `Encoding` and `AppVersion` are now in each packet they can be changed on a per-packet basis and an application can simultaneously support many encodings and app versions from a counterparty. This is in stark contrast to IBC version 1 where the channel prenegotiated the channel version (which implicitly negotiates the encoding as well); so that changing the app version after channel opening is very difficult.

All implementations must commit the packet in the standardized IBC commitment format to satisfy the protocol. In order to do this we must first commit the packet data and timeout. The timeout is encoded in LittleEndian format. The packet data which is a list of payloads is committed to by hashing each individual field of the payload and successively concatenating them together. This ensures a standard unambigious commitment for a given packet. Thus a given packet will always create the exact same commitment by all compliant implementations and two different packets will never create the same commitment by a compliant implementation. This commitment value is then stored under the standardized provable packet commitment key as defined below:

```typescript
func packetCommitmentPath(packet: Packet): bytes {
    return packet.sourceClientId + byte(0x01) + bigEndian(packet.sequence)
}
```

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
// Note: SourceClient and the sequence are omitted since they will be included in the key
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
    buffer = sha256.Hash(packet.destClient)
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

All acknowledgements must be committed to and stored under the standardized acknowledgment path. Note that since each acknowledgement is associated with a given received packet, the acnowledgement path is constructed using the packet `destClientId` and its `sequence` to generate a unique key for the acknowledgement.

```typescript
func acknowledgementPath(packet: Packet) {
    return packet.destClientId + byte(0x02) + bigEndian(packet.Sequence)
}
```

```typescript
// commitV2Acknowledgement hashes each app acknowledgment and hashes them together
// the final preimage will be prepended with the byte 0x02 before hashing in order to clearly define the protocol version
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

## Packet Receipt V2

A packet receipt will only tell the sending chain that the counterparty has successfully received the packet. Thus we just need a provable boolean flag uniquely associated with the sent packet. Thus, the receiver chain stores the packet receipt keyed on the destination identifier and the sequence to uniquely identify the packet.

For chains that support nonexistence proofs of their own state, they can simply write a `SENTINEL_RECEIPT_VALUE` under the receipt path. This `SENTINEL_RECEIPT_PATH` can be any non-nil value so it is recommended to write a single byte. The receipt path is standardized as below. Similar to the acknowledgement, each receipt is associated with a given received packet the receipt path is constructed using the packet `destClientId` and its `sequence` to generate a unique key for the receipt.

```typescript
func receiptPath(packet: Packet) {
    return packet.destClientId + byte(0x03) + bigEndian(packet.Sequence)
}
```

## Provable Path-space

IBC/TAO implementations MUST implement the following paths for the `provableStore` in the exact format specified. This is because counterparty IBC/TAO implementations will construct the paths according to this specification and send it to the light client to verify the IBC specified value stored under the IBC specified path. The `provableStore` is specified in [ICS24 Host Requirements](../ics-024-host-requirements/README.md)

Future paths may be used in future versions of the protocol, so the entire key-space in the provable store MUST be reserved for the IBC handler.

| Value                      | Path format                                    |
| -------------------------- | ---------------------------------------------- |
| Packet Commitment          | {sourceClientId}0x1{bigEndianUint64Sequence}   |
| Packet Receipt             | {destClientId}0x2{bigEndianUint64Sequence}     |
| Acknowledgement Commitment | {destClientId}0x3{bigEndianUint64Sequence}     |

Note that the IBC protocol ensures that the packet `(sourceClientId, sequence)` tuple uniquely identifies a packet on the sending chain, and the `(destClientId, sequence)` tuple uniquely identifies a packet on the receiving chain. This property along with the byte separator between the client identifier and sequence in the standardized paths ensures that commitments, receipts, and acknowledgements are each written to different paths for the same packet. Thus, so long as the host requirements specified in ICS24 are respected; a provable key written to state by the IBC handler for a given packet will never be overwritten with a different value. This ensures secure and correct communication between chains in the IBC ecosystem.
