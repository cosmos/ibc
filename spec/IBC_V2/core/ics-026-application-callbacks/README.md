---
ics: 26
title: IBC Application Callbacks
stage: Draft
category: IBC/TAO
kind: instantiation
version compatibility: ibc-go v10.0.0
author: Aditya Sripal <aditya@interchain.io>
created: 2025-03-19
---

## Synopsis

IBC enables module to module communication across remote state machines by providing a secure packet flow authenticated by the ICS-4 packet handler. The IBC core protocol is responsible for TAO (transport, authentication, ordering) of packets between two chains. These packets contain payload(s) that carry the application-specific information that is being communicated between two ICS26 applications. The data in the payload is itself opaque to the IBC core protocol, IBC core only verifies that it was correctly sent by the sender and then provides that data to the receiver for application-specific interpretation and processing.

This specification standardizes the interface between ICS-4 (core IBC/TAO) and an IBC application (i.e. ICS26 app) for all the packet flow messages.

The default IBC handler uses a receiver call pattern, where modules must individually call the IBC handler in order to send packets. In turn, the IBC handler verifies incoming packet flow messages like `ReceivePacket`, `AcknowledgePacket` and `TimeoutPacket` and calls into the appropriate ICS26 application as described in [ICS5 Port Allocation](../ics-005-port-allocation/README.md).

## Technical Specification

### Payload Structure

The payload structure is reproduced from [ICS-4](../ics-004-packet-semantics/PACKET.md) since all of the following application functions are operating on the payloads that are being sent in the packets.

```typescript
interface Payload {
    sourcePort: bytes, // identifier of the sending application on the sending chain
    destPort: bytes, // identifier of the receiving application on the receiving chain
    version: string, // payload version only interpretable by sending/receiving applications
    encoding: string, // payload encoding only interpretable by sending/receiving applications
    value: bytes // application-specific data that can be parsed by receiving application given the version and encoding
}
```

### Core Handler Interface Exposed to ICS26 Applications

The IBC core handler MUST expose the following function signature to the ICS26 applications registered on the port router, so that the application can send packets.

#### SendPacket

SendPacket Inputs:

`payloads: Payload`: This is the payload that the application wishes to send to an application on the receiver chain.
`sourceClientId: bytes`: Identifier of the receiver chain client that exists on the sending chain.
`timeoutTimestamp: uint64`: The timeout in UNIX seconds after which the packet is no longer receivable on the receiving chain. NOTE: This timestamp is evaluated against the **receiving chain** clock as there may be drift between the sending chain and receiving chain clocks

SendPacket Preconditions:

- The application is registered on the port router with `payload.SourcePortId`
- The application MUST have successfully conducted any application specific logic necessary for sending the given payload.
- The sending client exists for `sourceClientId`

SendPacket Postconditions:

- The following packet gets committed and stored under the packet commitment path as specified by ICS24:

```typescript
interface Packet {
    sourceClientId: sourceClientId,
    destClientId: getCounterparty(sourceClientId).ClientId,  // destClientId should be filled in with the registered counterparty id for provided sourceClientId
    sequence: generateUniqueSequence(sourceClientId),
    timeoutTimestamp: msg.timeoutTimestamp
    data: msg.Payloads
}
```

- The sequence is returned to the ICS26 application

SendPacket ErrorConditions:

- The sending client is invalid (expired or frozen)
- The provided `timeoutTimstamp` has already elapsed
- The sending application is not allowed to send the provided payload to the requested receiving application as identified by `payload.DestPort`

NOTE: IBC v2 allows multiple payloads coming from multiple applications to be sent in the same packet. If an implementation chooses to support this feature, they may either provide an entrypoint in the core handler to send multiple packets, which must then call each individual application `OnSendPacket` callback to validate their individual payload and do application-specific sending logic; or they may queue the payloads coming from each application until the packet is ready to be committed.

#### WriteAcknowledgement

The IBC core handler MAY expose the following function signature to the ICS26 applications registed on the port router, so that the application can write acknowledgements asynchronously.

This is only necessary if the implementation supports processing packets asynchronously. In this case, an application may process the packet asynchronously from when the IBC core handler receives the packet. Thus, the acknowledgement cannot be returned as part of the `OnRecvPacket` callback and must be submitted to the core IBC handler by the ICS26 application at a later time. Thus, we must introduce a new endpoint on the IBC handler for the ICS26 application to call when it is done processing a receive packet and wants to write the acknowledgement.

WriteAcknowledgement Inputs:

`destClientId: bytes`: Identifier of the sender chain client that exist on the receiving chain (i.e. executing chain)
`sequence: uint64`: Unique sequence identifying the packet from sending chain to receiving chain
`ack: bytes`: Acknowledgement from the receiving application for the payload it was sent by the application. If the receive was unsuccessful, the `ack` must be the `SENTINEL_ERROR_ACKNOWLEDGEMENT`, otherwise it may be some application-specific data.

WriteAcknowledgement Preconditions:

- A packet receipt is stored under the specified ICS24 with the `destClientId` and `sequence`
- An acknowledgement for the `destClientId` and `sequence` has not already been written under the ICS24 path

WriteAcknowledgement Postconditions:

- The acknowledgement is committed and written to the acknowledgement path as specified in ICS24
- If the acknowledgement is successful, then all receiving applications must have executed their recvPacket logic and written state
- If the acknowledgement is unsuccessful (ie ERROR ACK), any state changes made by the receiving applications MUST all be reverted. This ensure atomic execution of the multi-payload packet.

NOTE: In the case that the packet contained multiple payloads, the IBC core handler MUST wait for all applications to return their individual acknowledgements for the packet before commiting the acknowledgment. If ANY application returns the error acknowledgement, then the acknowledgement for the entire packet only contains the `ERROR_SENTINEL_ACKNOWLEDGEMENT`. Otherwise, the acknowledgment is a list containing each applications individual acknowledgment in the same order that their associated payload existed in the packet.

### ICS26 Interface Exposed to Core Handler

Modules must expose the following function signatures to the routing module, which are called upon the receipt of various datagrams:

#### OnRecvPacket

OnRecvPacket Inputs:

`sourceClientId: bytes`: This is the identifier of the client on the sending chain. NOTE: This is an identifier on the counterparty chain provided as information for the application, but it should not be treated as a unique identifier on the receiving chain.
`destClientId: bytes`: This is the identifier of the receiving chain (i.e. executing chain)
`sequence: uint64`: This is the unique sequence for the packet in the stream of packets from sending chain to destination chain. The tuple `(destClientId, sequence)` uniquely identifies the packet on this chain.
`payload: Payload`. This is the payload that an application registered by `payload.SourcePort` on the sending chain sends to the executing application

OnRecvPacket Preconditions:

- The application is registered on the port router with `payload.DestPort`
- The destination client exists for `destClientId`
- All IBC/TAO verification checks have already been authenticated by IBC core handler. Thus, when the application receives a packet; it can be guaranteed of its authenticity and need only perform the relevant application logic for the given payload.

OnRecvPacket Postconditions:

- The application has executed all app-specific logic for the given payload and made the appropriate state changes
- The application returns an app acknowledgment `ack: bytes` to the core IBC handler to be written as an acknowledgement of the payload in this packet.

OnRecvPacket ErrorConditions:

- The sending application as identified by `payload.SourcePortId` is not allowed to send a payload to the receiving application
- The requested version as identified by `payload.Version` is unsupported
- The requested encoding as identified by `payload.Encoding` is unsupported
- An error occured while processing the `payload.Value` after decoding with `payload.Encoding` and processing the payload in the manner expected by `payload.Version`.

IMPORTANT: If the `OnRecvPacket` callback errors for any reason, the state changes made during the callback MUST be reverted and the IBC core handler MUST write the `SENTINEL_ERROR_ACKNOWLEDGEMENT` for this packet even if other payloads in the packet are received successfully.

#### OnAcknowledgePacket

OnAcknowledgePacket Inputs:

`sourceClientId: bytes`: This is the identifier of the client on the sending chain (i.e. executing chain).
`destClientId: bytes`: This is the identifier of the receiving chain. NOTE: This is an identifier on the counterparty chain provided as information for the application, but it should not be treated as a unique identifier on the receiving chain.
`sequence: uint64`: This is the unique sequence for the packet in the stream of packets from sending chain to destination chain. The tuple `(sourceClientId, sequence)` uniquely identifies the packet on this chain.
`acknowledgement: bytes`: This is the acknowledgement that the receiving application sent for the payload that we previously sent. It may be a successful acknowledgement with app-specific information or it may be the `SENTINEL_ERROR_ACKNOWLEDGEMENT` in which case we should handle any app-specific logic needed for a packet that failed to be sent.
`payload: Payload`: This is the original payload that we previously sent

OnAcknowledgementPreconditions:

- This application had previously sent the provided payload in a packet with the provided `sourceClientId` and `sequence`.
- All IBC/TAO verification checks have already been authenticated by IBC core handler. Thus, when the application receives an acknowledgement; it can be guaranteed of its authenticity and need only perform the relevant application logic for the given acknowledgement and payload.

OnAcknowledgement Postconditions:

- The application has executed all app-specific logic for the given payload and acknowledgment and made the appropriate state changes
- If the acknowledgement was the `SENTINEL_ERROR_ACKNOWLEDGEMENT`, this will usually involve reverting whatever application state changes were made during `SendPacket` (e.g. unescrowing tokens for transfer)

OnAcknowledgement Errorconditions:

- Application specific errors may occur while processing the acknowledgement. The packet lifecycle is already complete. Implementations MAY choose to allow retries or not.

#### OnTimeoutPacket

OnTimeoutPacket Inputs:

`sourceClientId: bytes`: This is the identifier of the client on the sending chain (i.e. executing chain).
`destClientId: bytes`: This is the identifier of the receiving chain. NOTE: This is an identifier on the counterparty chain provided as information for the application, but it should not be treated as a unique identifier on the receiving chain.
`sequence: uint64`: This is the unique sequence for the packet in the stream of packets from sending chain to destination chain. The tuple `(sourceClientId, sequence)` uniquely identifies the packet on this chain.
`payload: Payload`: This is the original payload that we previously sent

OnTimeoutPacket Preconditions:

- This application had previously sent the provided payload in a packet with the provided `sourceClientId` and `sequence`.
- All IBC/TAO verification checks have already been authenticated by IBC core handler. Thus, when the application receives an timeout; it can be guaranteed of its authenticity and need only perform the relevant application timeout logic for the given payload.

OnTimeoutPacket Postconditions:

- The application has executed all app-specific logic for the given payload and made the appropriate state changes. This will usually involve reverting whatever application state changes were made during `SendPacket` (e.g. unescrowing tokens for transfer)

OnTimeoutPacket Errorconditions:

- Application specific errors may occur while processing the timeout. The packet lifecycle is already complete. Implementations MAY choose to allow retries or not.
