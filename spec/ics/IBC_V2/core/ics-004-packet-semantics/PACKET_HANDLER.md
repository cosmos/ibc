# IBC Packet Handler

The packet handler specification defines the semantics and behavior that implementations must enforce in order to support IBC v2 protocol.

## Packet Structure

A `Packet` in the interblockchain communication protocol is the primary interface by which applications will send data to counterparty applications on other chains. It is defined as follows:

```typescript
interface Packet {
    sourceClientId: bytes // identifier of the client on the sending chain
    destClientId: bytes // identifier of the client on the receiving chain
    sequence: uint64 // unique number identifying this packet in the stream of packets from sourceClientId to destClientId
    timeoutTimestamp: uint64, // indicates the timeout as a UNIX timestamp in seconds. If the timeout timestamp is reached on destination chain, it is no longer receivable
    data: Payload[] // a list of payloads intended for applications on the receiving chain
}
```

```typescript
interface Payload {
    sourcePort: bytes, // identifier of the sending application on the sending chain
    destPort: bytes, // identifier of the receiving application on the receiving chain
    version: string, // payload version only interpretable by sending/receiving applications
    encoding: string, // payload encoding only interpretable by sending/receiving applications
    value: bytes // application-specific data that can be parsed by receiving application given the version and encoding
}
```

The packet is never directly serialised and sent to counterparty chains. Instead a standardized non-malleable committment to the packet data is stored under the standardized unique key for the packet as defined in ICS-24. Thus, implementations MAY make individual choices on the exact packet structure and serialization scheme they use internally so long as they respect the standardized commitment defined by the IBC protocol when writing to the provable store.

Packet Invariants:

- None of the packet fields are allowed to be empty
- For every payload included, none of the payload fields are allowed to be empty

## Receipt

A `Receipt` is a sentinel byte that is stored under the standardized provable ReceiptPath of a given packet by the receiving chain when it successfully receives the packet. This prevents replay attacks and also the possibility of timing out a packet on the sender chain when the packet has already been received. The specific value of the receipt does not matter so long as its not empty.

## Acknowledgement Structure

An `Acknowledgement` is the interface that will be used by receiving applications to return application specific information back to the sender. If every application successfully received its payload, then each receiving application will return their custom acknowledgement bytes which will be appended to the acknowledgement array. If **any** application returns an error, then the acknowledgement will have a single element with a sentinel error acknowledgement.

```typescript
const ErrorAcknowledgement = sha256("UNIVERSAL_ERROR_ACKNOWLEDGEMENT")

interface Acknowledgement {
    appAcknowledgement bytes[] // array of an array of bytes. Each element of the array contains an acknowledgement from a specific application
}
```

Acknowledgement Invariants:

- If the acknowledgement interface includes an error acknowledgement then there must be only a single element in the array with the error acknowledgement
- There CANNOT be multiple app acknowledgements where an element is the error acknowledgement
- If there are multiple app acknowledgements, the length of the app acknowledgements is the same length as the payloads in the associated packet and each acknowledgement is associated with the payload in the same position in the payload array.

## SendPacket

SendPacket is called by users to execute an inter-blockchain flow. The user submits a message with a payload(s) for each IBC application they wish to interact with. The SendPacket handler must call the sendPacket logic of each sending application as identified by the sourcePort of the payload. If none of the sending applications error, then the sendPacket handler must construct the packet with the user-provided sourceClient, payloads, and timeout and the destinationClient it retrieves from the counterparty storage given the sourceClient and a generated sequence that is unique for the sourceClientId. It will commit the packet with the ICS24 commitment function under the ICS24 path. The sending chain MAY store the ICS24 path under a custom prefix in the provable store. In this case, the counterparty must have knowledge of the custom prefix as provided by the relayer on setup. The sending chain SHOULD check the provided timestamp against an authenticated time oracle (local BFT time or destination client latest timestamp) and preemptively reject a user-provided packet with a timestamp that has already passed.

The user may be an off-chain process or an on-chain actor. In either case, the user is not trusted by the IBC protocol. The IBC application is responsible for properly authenticating that the user is allowed to send the requested app data using the IBC application's port as specified in the source port of the payload. The IBC application is also responsible for executing any app-specific logic that must run before the IBC packet can be sent (e.g. escrowing user's tokens before sending a fungible token transfer packet).

SendPacket Inputs:

`payloads: Payload[]`: List of payloads that are to be sent from source applications on sending chain to corresponding destination applications on the receiving chain. Implementations MAY choose to only support a single payload per packet.
`sourceClientId: bytes`: Identifier of the receiver chain client that exists on the sending chain.
`timeoutTimestamp: uint64`: The timeout in UNIX seconds after which the packet is no longer receivable on the receiving chain. NOTE: This timestamp is evaluated against the **receiving chain** clock as there may be drift between the sending chain and receiving chain clocks

SendPacket Preconditions:

- A valid client exists on the sending chain with the `sourceClientId`
- There exists a mapping on the sending chain from `sourceClientId` to `Counterparty`

SendPacket Postconditions:

- The sending application(s) as identified by the source port(s) in the payload(s) have all executed their sendPacket logic successfully
- The following packet gets committed and stored under the packet commitment path as specified by ICS24:

```typescript
interface Packet {
    sourceClientId: sourceClientId,
    destClientId: getCounterparty(sourceClientId).ClientId, // destClientId should be filled in with the registered counterparty id for provided sourceClientId
    sequence: generateUniqueSequence(sourceClientId),
    timeoutTimestamp: timeoutTimestamp
    data: payloads
}
```

- Since the packet is committed to with a hash in-state, implementations must provide the packet fields for relayers to reconstruct. This can be emitted in an event system or stored in state as the full packet under an auxilliary key if the implementing platform does not have an event system.

SendPacket Errorconditions:

- Any of the sending applications returns an error during its sendPacket logic execution
- The sending client is invalid (expired or frozen)

SendPacket Invariants:

- The sourceClientId MUST exist on the sending chain
- The destClientId MUST be the registered counterparty of the sourceClientId on the sending chain
- The sending chain MUST NOT have sent a previous packet with the same `sourceClientId` and `sequence`

## RecvPacket

RecvPacket is called by relayers once a packet has been committed on the sender chain in order to process the packet on the receiving chain. Since the relayer is not trusted, the relayer must provide a proof that the sender chain had indeed committed the provided packet which will be verified against the `destClient` on the receiving chain.

If the proof succeeds, and the packet passes replay and timeout checks; then each payload is sent to the receiving application as part of the receiving application callback.

RecvPacket Inputs:
`packet: Packet`: The packet sent from the sending chain to our chain
`proof: bytes`: An opaque proof that will be sent to the destination client. The destination client is responsible for interpreting the bytes as a proof and verifying the packet commitment key/value provided by the packet handler against the provided proof.
`proofHeight: Number`: This is the height of the counterparty chain from which the proof was generated. A corresponding consensus state for this height must exist on the destination client for the proof to verify correctly.

RecvPacket Preconditions:

- A valid client exists on the receiving chain with `destClientId`
- There exists a mapping from `destClientId` to `Counterparty`

RecvPacket Postconditions:

- A packet receipt is stored under the specified ICS24 with the `destClientId` and `sequence`
- All receiving application(s) as identified by the destPort(s) in the payload(s) have executed their recvPacket logic. If **any** of the payloads return an error during processing, then all application state changes for all payloads **must** be reverted. If all payloads are processed successfully, then all applications state changes are written. This ensures atomic execution for the payloads batched together in a single packet.
- If any payload returns an error, then the single `SENTINEL ERROR ACKNOWLEDGEMENT` is written using `WriteAcknowledgment`. If all payloads succeed and return an app-specific acknowledgement, then each app acknowledgement is included in the list of `AppAcknowledgement` in the final packet `Acknowledgement` in the **exact** order that their corresponding payloads were included in the packet.

NOTE: It is possible for applications to process their payload asynchronously to the `RecvPacket` transaction execution. In this case, the IBC core handler **must** await all applications returning their individual application acknowledgements before writing the acknowledgement with app acknowledgements in the order of their corresponding payloads in the original packet **not** the order in which the applications return their asynchronous acknowledgements which may be different orders. IBC allows multiple payloads intended for the same application to be batched in the same packet. Thus, if an implementation wishes to support multiple payloads and asynchronous acknowledgements together, then there must be a way for core IBC to know which payload a particular acknowledgment is being written for. This may be done by providing the index of the payload list during `recvPacket` application callback, so that the application can return the same index when writing the acknowledgment so that it can be placed in the right order. Otherwise, implementations may simply block asynchronous acknowledgment support for multi-payload packets

RecvPacket Errorconditions:

- `Counterparty.ClientId` != `packet.sourceClientId` ensures that packet was sent by expected counterparty
- `packet.TimeoutTimestamp` >= `chain.BlockTime()` ensures we cannot receive successfully if packet can be timed out on sending chain
- Packet receipt does not already exist in state for the `destClientId` and `sequence`. This prevents replay attacks
- Membership proof does not successfully verify

## WriteAcknowledgement

WriteAcknowledgement Inputs:

`destClientId: bytes`: Identifier of the sender chain client that exist on the receiving chain
`sequence: uint64`: Unique sequence identifying the packet from sending chain to receiving chain
`ack: Acknowledgement`: Acknowledgement collected by receiving chain from all receiving applications after they have returned their individual acknowledgement. If any individual application errors, the entire acknowledgement MUST have a single element with just the SENTINEL ERROR ACKNOWLEDGEMENT. If all applications successfully received, then every application must have its own acknowledgement set in the `Acknowledgement` in the same order that they existed in the payload of the sending packet.

WriteAcknowledgement Preconditions:

- A packet receipt is stored under the specified ICS24 with the `destClientId` and `sequence`
- An acknowledgement for the `destClientId` and `sequence` has not already been written under the ICS24 path

WriteAcknowledgement Postconditions:

- The acknowledgement is committed and written to the acknowledgement path as specified in ICS24
- Since the acknowledgement is being hashed, the full acknowledgement fields should be made available for relayers to reconstruct. This can be emitted in an event system or stored in state as the full packet under an auxilliary key if the implementing platform does not have an event system.
- Implementors SHOULD also emit the full packet again in `WriteAcknowledgement` since the sender chain is only expected to store the packet commitment and not the full packet; relayers are expected to pass the packet back to the sender chain to process the acknowledgement. Thus, in order to support stateless relayers it is helpful to re-emit the packet fields on `WriteAcknowledgement` so the relayer can reconstruct the packet. 
- If the acknowledgement is successful, then all receiving applications must have executed their recvPacket logic and written state
- If the acknowledgement is unsuccessful (ie ERROR ACK), any state changes made by the receiving applications MUST all be reverted. This ensure atomic execution of the multi-payload packet.

## AcknowledgePacket

AcknowledgePacket Inputs:

`packet: Packet`: The packet that was originally sent by our chain
`acknowledgement: Acknowledgement`: The acknowledgement written by the receiving chain for the packet
`proof: bytes`:  An opaque proof that will be sent to the source client. The source client is responsible for interpreting the proof and verifying it against the acknowledgement key/value provided by the packet handler.
`proofHeight: Number`: This is the height of the counterparty chain from which the proof was generated. A corresponding consensus state for this height must exist on the source client for the proof to verify correctly.

AcknowledgePacket Preconditions:

- A valid client exists on the sending chain with the `sourceClientId`
- There exists a mapping on the sending chain from `sourceClientId` to `Counterparty`
- A packet commitment has been stored under the ICS24 packet path with `sourceClientId` and `sequence`

AcknowledgePacket Postconditions:

- All sending applications execute the ackPacket logic with the payload and the individual acknowledgement for that payload or the universal `ErrorAcknowledgement`.
- Stored commitment for the packet is deleted

AcknowledgePacket Errorconditions:

- `packet.destClient` != `counterparty.ClientId`. This should never happen if the second error condition is not true, since we constructed the packet correctly earlier
- The packet provided by the relayer does not commit to the stored commitment we have stored for the `sourceClientId` and `sequence`
- Membership proof of the acknowledgement commitment on the receiving chain as standardized by ICS24 does not verify
- Any of the applications return an error during the `AcknowledgePacket` callback for their payload. Applications should generally not error on AcknowledgePacket. If this occurs, it is most likely a bug so the error should revert the transaction and allow for the bug to be patched before resubmitting the transaction.

## TimeoutPacket

TimeoutPacket Inputs:

`packet: Packet`: The packet that was originally sent by our chain
`proof: bytes`: An opaque non-existence proof that will be sent to the source client. The source client is responsible for interpreting the proof and verifying it against the receipt key provided by the packet handler.
`proofHeight: Number`: This is the height of the counterparty chain from which the proof was generated. A corresponding consensus state for this height must exist on the source client for the proof to verify correctly.

TimeoutPacket Preconditions:

- A valid client exists on the sending chain with the `sourceClientId`
- There exists a mapping on the sending chain from `sourceClientId` to `Counterparty`
- A packet commitment has been stored under the ICS24 packet path with `sourceClientId` and `sequence`

TimeoutPacket Postconditions:

- All sending applications execute the timeoutPacket logic with the payload.
- Stored commitment for the packet is deleted

TimeoutPacket Errorconditions:

- `packet.destClient` != `counterparty.ClientId`. This should never happen if the second error condition is not true, since we constructed the packet correctly earlier
- The packet provided by the relayer does not commit to the stored commitment we have stored for the `sourceClientId` and `sequence`
- Non-Membership proof of the packet receipt on the receiving chain as standardized by ICS24 does not verify
- Any of the applications return an error during the `TimeoutPacket` callback for their payload. Applications should generally not error on TimeoutPacket. If this occurs, it is most likely a bug so the error should revert the transaction and allow for the bug to be patched before resubmitting the transaction.
