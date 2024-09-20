# Packet Specification

## Packet V2

The IBC packet sends application data from a source chain to a destination chain with a timeout that specifies when the packet is no longer valid. The packet will be committed to by the source chain as specified in the ICS-24 specification. The receiver chain will then verify the packet commitment under the ICS-24 specified packet commitment path. If the proof succeeds, the IBC handler sends the application data(s) to the relevant application(s).

```typescript
interface Packet {
    sourceIdentifier: bytes,
    destIdentifier: bytes,
    sequence: uint64
    timeout: uint64,
    data: [Payload]
}

interface Payload {
    sourcePort: bytes,
    destPort: bytes,
    version: string,
    encoding: Encoding,
    appData: bytes,
}

enum Encoding {
    NO_ENCODING_SPECIFIED,
    PROTO_3,
    JSON,
    RLP,
    BCS,
}
```

The source and destination identifiers at the top-level of the packet identifiers the chains communicating. The source identifier **must** be unique on the source chain and is a pointer to the destination chain. The destination identifier **must** be a unique identifier on the destination chain and is a pointer to the source chain. The sequence is a monotonically incrementing nonce to uniquely identify packets sent between the source and destination chain.

The timeout is the UNIX timestamp in seconds that must be passed on the **destination** chain before the packet is invalid and no longer capable of being received. Note that the timeout timestamp is assessed against the destination chain's clock which may drift relative to the clocks of the sender chain or a third party observer. If a packet is received on the destination chain after the timeout timestamp has passed relative to the destination chain's clock; the packet must be rejected so that it can be safely timed out and reverted by the sender chain.

In version 2 of the IBC specification, implementations **MAY** support multiple application data within the same packet. This can be represented by a list of payloads. Implementations may choose to only support a single payload per packet, in which case they can just reject incoming packets sent with multiple payloads.

Each payload will include its own `Encoding` and `AppVersion` that will be sent to the application to instruct it how to decode and interpret the opaque application data. The application must be able to support the provided `Encoding` and `AppVersion` in order to process the `AppData`. If the receiving application does not support the encoding or app version, then the application **must** return an error to IBC core. If the receiving application does support the provided encoding and app version, then the application must decode the application as specified by the `Encoding` enum and then process the application as expected by the counterparty given the agreed-upon app version. Since the `Encoding` and `AppVersion` are now in each packet they can be changed on a per-packet basis and an application can simultaneously support many encodings and app versions from a counterparty. This is in stark contrast to IBC version 1 where the channel prenegotiated the channel version (which implicitly negotiates the encoding as well); so that changing the app version after channel opening is very difficult.

The packet must be committed to as specified in the ICS24 specification. In order to do this we must first commit the packet data and timeout.

```typescript
func commitV2Packet(packet: Packet) {
    timeoutBytes = LittleEndian(timeout)
    // TODO: Decide on canonical encoding scheme
    appBytes = encoding(payload)
    ics24.commitPacket(packet.destinationIdentifier, timeoutBytes, appBytes)
}
```

## Acknowledgement V2

The acknowledgement in the version 2 specification is also modified to support multiple payloads in the packet that will each go to separate applications that can write their own acknowledgements. Each acknowledgment will be contained within the final packet acknowledgment in the same order that they were received in the original packet. Thus if a packet contains payloads for modules `A` and `B` in that order; the receiver will write an acknowledgment with the app acknowledgements `A` and `B` in the same order.

```typescript
interface Acknowledgement {
    appAcknowledgement: [bytes]
}
```

All acknowledgements must be committed to and stored under the ICS24 acknowledgment path.

```typescript
func commitV2Acknowledgment(ack: Acknowledgement) {
    // TODO: Decide on canonical encoding scheme
    ackBytes = encoding(ack)
    ics24.commitAcknowledgment(ackBytes)
}
```

