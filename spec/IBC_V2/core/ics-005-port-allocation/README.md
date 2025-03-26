---
ics: 5
title: Port Allocation
stage: Draft
required-by: 4
category: IBC/TAO
kind: interface
version compatibility: ibc-go v10.0.0
author: Aditya Sripal <aditya@interchain.io>
created: 2024-05-17
---

## Synopsis

This standard specifies the port allocation system by which modules can bind to uniquely named ports allocated by the IBC handler.
The port identifiers in the packet defines which application to route the packet callback to. The source portID is an identifier of the application sending the packet, thus it will also receive the `AcknowledgePacket` and `TimeoutPacket` callback. The destination portID is the identifier of the application receiving the packet and will receive the `ReceivePacket` callback.

Modules may register multiple ports on a state machine and send from any of their registered ports to any arbitrary port on a remote state machine. Each port on a state machine must be mapped to a specific IBC module as defined by [ICS-26](../ics-026-application-callbacks/README.md). Thus the IBC application to portID mapping is one-to-many.

NOTE: IBC v1 included a channel along with a channel handshake that explicitly associated a unique channel between two portIDs on counterparty chains. Thus, the portIDs on both sides were tightly coupled such that no other application other than the ones bound by the portIDs were allowed to send packets on the dedicated channel. IBC v2 removed the concept of a channel and all packet flow is between chains rather than being isolated module-module communication. Thus, an application on a sending chain is allowed to send a packet to ANY other application on a destination chain by identifying the application with the portIDs in the packet. Thus, it is now the responsibility of applications to restrict which applications are allowed to send packets to them by checking the portID in the callback and rejecting any packet that comes from an unauthorized application.

### Motivation

The interblockchain communication protocol is designed to facilitate module-to-module communication, where modules are independent, possibly mutually distrusted, self-contained
elements of code executing on sovereign ledgers.

## Technical Specification

### Registering a port

The IBC handler MUST provide a way for applications to register their callbacks on a portID.

```typescript
function registerPort(portId: Identifier, cbs: ICS26App) => void
```

RegisterPort Preconditions:

- There is no other application that is registered on the port router for the given `portId`.

RegisterPort Postconditions:

- The ICS26 application is registered on the provided `portId`.
- Any incoming packet flow message addressed to the `portId` is routed to the ICS26 application. Any outgoing packet flow message addressed by the `portId` MUST come from the ICS26 application

### Authenticating and Routing Packet Flow Messages

Once an application is registered with a port, it is the port router's responsibility to properly route packet flow messages to the appropriate application identified by the portId in the payload. Similarly when the application sends packet flow messages to the port router, the router MUST ensure that the application is authenticated to send the packet flow message by checking if the payload portIDs are registered to the application.

For packet flow messages on the packet sending chain (e.g. `SendPacket`, `AcknowledgePacket`, `TimeoutPacket`); the port router MUST do this authentication and routing using the packet payload's `sourcePortId`.

For packet flow messages on the packet receiving chain (e.g. `RecvPacket` and optionally the asynchronous `WriteAcknowledgement`); the port router MUST do this authentication and routing using the packet payload's `destPortId`.

[ICS-4](../ics-004-packet-semantics/PACKET_HANDLER.md) defines the packet flow messages and the expected behavior of their respected handlers. When the packet flow message arrives from the core ICS-4 handler to the application (e.g. `RecvPacket`, `AcknowledgePacket`, `TimeoutPacket`); then the portRouter acts as a router routing the message from the core handler to the ICS26 application. When the packet flow message arrives from the application to the core ICS-4 handler (e.g. `SendPacket`, or the optional `WriteAcknowledgement`); then the portRouter acts as an authenticator by checking that the calling application is registered as the owner of port they wish to send the message on before sending the message to the ICS-4 handler.

NOTE: It is possible for implementations to change the order of execution flow so long as they still respect all the expected semantics and behavior defined in ICS-4. In this case, the port router's role as router or authenticator will change accordingly.
