---
ics: 26
title: IBC Application Callbacks
stage: Draft
category: IBC/TAO
kind: instantiation
version compatibility: ibc-go v10.0.0
author: Aditya Sripal <cwgoes@tendermint.com>
created: 2025-03-19
---

## Synopsis

IBC enables module to module communication across remote state machines by providing a secure packet flow authenticated by the ICS-4 packet handler. The IBC core protocol is responsible for TAO (transport, authentication, ordering) of packets between two chains. These packets contain payload(s) that carry the application-specific information that is being communicated between two ICS26 applications. The data in the payload is itself opaque to the IBC core protocol, IBC core only verifies that it was correctly sent by the sender and then provides that data to the receiver for application-specific interpretation and processing.

This specification standardizes the interface between ICS-4 (core IBC/TAO) and an IBC application (i.e. ICS26 app) for all the packet flow messages.

The default IBC handler uses a receiver call pattern, where modules must individually call the IBC handler in order to send packets. In turn, the IBC handler verifies incoming packet flow messages like `ReceivePacket`, `AcknowledgePacket` and `TimeoutPacket` and calls into the appropriate ICS26 application as described in [ICS5 Port Allocation](../ics-005-port-allocation/README.md).

## Technical Specification

### Payload Structure

The payload structure is reproduced from [ICS-4](../ics-004-channel-and-packet-semantics/PACKET.md) since all of the following application functions are operating on the payloads that are being sent in the packets.

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

The IBC core handler MUST expose the following function signature to the ICS26 applications registed on the port router, so that the application can send packets.

### SendPacket

SendPacket Inputs:
`payload: Payload`: This is the payload that the application wishes to send to an application on the receiver chain.
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
    sourceClientId: msg.sourceClientId,
    destClientId: counterparty.ClientId,
    sequence: generateUniqueSequence(sourceClientId),
    timeoutTimestamp: msg.timeoutTimestamp
    data: msg.Payloads
}
```
- The sequence is returned to the ICS26 application

SendPacket ErrorConditions:
- The sending client is invalid (expired or frozen)
- The provided `timeoutTimstamp` has already elapsed

NOTE: IBC v2 allows multiple payloads coming from multiple applications to be sent in the same packet. If an implementation chooses to support this feature, they may either provide an entrypoint in the core handler to send multiple packets, which must then call each individual application `OnSendPacket` callback to validate their individual payload and do application-specific sending logic; or they may queue the payloads coming from each application until the packet is ready to be committed.

### WriteAcknowledgement

The IBC core handler MAY expose the following function signature to the ICS26 applications registed on the port router, so that the application can write acknowledgements asynchronously.

This is only necessary if the implementation supports processing packets asynchronously. In this case, an application may process the packet asynchronously from when the IBC core handler receives the packet. Thus, the acknowledgement cannot be returned as part of the `OnRecvPacket` callback and must be submitted to the core IBC handler by the ICS26 application at a later time. Thus, we must introduce a new endpoint on the IBC handler for the ICS26 application to call when it is done processing a receive packet and wants to write the acknowledgement.

WriteAcknowledgement Inputs:

WriteAcknowledgement Inputs:
`destClientId: bytes`: Identifier of the sender chain client that exist on the receiving chain
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
