---
ics: 25
title: Handler Interface
stage: draft
category: ibc-core
requires: 2, 3, 4, 23, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-23
modified: 2019-06-09
---

## Synopsis

This document describes the interface exposed by the standard IBC implementation (referred to as the IBC handler) to modules within the same state machine, and the implementation of that interface by the IBC handler.

### Motivation

IBC is an inter-module communication protocol, designed to faciliate reliable, authentication message passing between modules on separate blockchains. Modules should be able to reason about the interface they interact with and the requirements they must adhere to in order to utilize it safely.

### Definitions

`ClientState`, `Header`, and `ConsensusState` are as defined in [ICS 2](../ics-002-consensus-verification).

`ConnectionEnd` and `ConnectionState` are as defined in [ICS 3](../ics-003-connection-semantics).

`ChannelEnd`, `ChannelState`, and `Packet` are as defined in [ICS 4](../ics-004-channel-and-packet-semantics).

`CommitmentProof` is as defined in [ICS 23](../ics-023-vector-commitments).

`Identifier`s must conform to the schema defined in [ICS 24](../ics-024-host-requirements).

### Desired Properties

- Creation of clients, connections, and channels should be as permissionless as possible.
- The module set should be dynamic: chains should be able to add and destroy modules, which can themselves bind to and unbind from ports, at will with a persistent IBC handler.
- Modules should be able to write their own more complex abstractions on top of IBC to provide additional semantics or guarantees.

## Technical Specification

### Client lifecycle management

By default, clients are unowned: any module can create a new client, query any existing client, update any existing client, and delete any existing client not in use.

`createClient` creates a new client with a specified identifier.

```typescript
function createClient(id: Identifier, consensusState: ConsensusState): void {
  // defined in ICS 2
}
```

`queryClientConsensusState` queries a client by a known identifier, returning the associated consensus state if found.

```typescript
function queryClientConsensusState(id: Identifier): ConsensusState | void {
  // defined in ICS 2
}
```

`queryClientFrozen` queries whether or not a client is frozen:

```typescript
function queryClientFrozen(id: Identifier): boolean | void {
  // defined in ICS 2
}
```

`queryClientRoot` queries a state root by height:

```typescript
function queryClientRoot(id: Identifier, height: uint64): CommitmentRoot | void {
  // defined in ICS 2
}
```

`updateClient` updates an existing client with a new header, returning an error if the client was not found or the header was not a valid update.

The default IBC relayer module will allow external calls to `updateClient`.

```typescript
function updateClient(id: Identifier, header; Header): error | void {
  // defined in ICS 2
}
```

`freezeClient` freezes an existing client by providing proof-of-equivocation, automatically freezing any associated connections & channels.

The default IBC relayer module will allow external calls to `freezeClient`.

```typescript
function freezeClient(id: Identifier, firstHeader: Header, secondHeader: Header): error | void {
  // defined in ICS 2
}
```

### Connection lifecycle management

By default, connections are unowned. Connections can be closed by any module, but only when all channels associated with the connection have been closed by the modules which opened them and a timeout has passed since the connection was opened.

```typescript
function connOpenInit(
  identifier: Identifier, desiredCounterpartyIdentifier: Identifier,
  clientIdentifier: Identifier, counterpartyClientIdentifier: Identifier, nextTimeoutHeight: uint64) {
  // defined in ICS 3
}
```

`connOpenTry` acknowledges a connection initialization on the initiating chain.

The default IBC relayer module will allow external calls to `connOpenTry`.

```typescript
function connOpenTry(
  desiredIdentifier: Identifier, counterpartyConnectionIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier, clientIdentifier: Identifier,
  proofInit: CommitmentProof, timeoutHeight: uint64, nextTimeoutHeight: uint64) {
  // defined in ICS 3
}
```

`connOpenAck` acknowledges a connection in progress on another chain.

The default IBC relayer module will allow external calls to `connOpenAck`.

```typescript
function connOpenAck(
  identifier: Identifier, proofTry: CommitmentProof,
  timeoutHeight: uint64, nextTimeoutHeight: uint64) {
  // defined in ICS 3
}
```

`connOpenConfirm` acknowledges the acknowledgement and finalizes a new connection.

The default IBC relayer module will allow external calls to `connOpenConfirm`.

```typescript
function connOpenConfirm(identifier: Identifier, proofAck: CommitmentProof, timeoutHeight: uint64) {
  // defined in ICS 3
}
```

`connOpenTimeout` proves that a connection handshake has timed-out and resets the process.

The default IBC relayer module will allow external calls to `connOpenTimeout`.

```typescript
function connOpenTimeout(identifier: Identifier, proofTimeout: CommitmentProof, timeoutHeight: uint64) {
  // defined in ICS 3
}
```

`connCloseInit` initiates the graceful connection closing process. It will fail if there are any open channels using the connection or if the identifier is invalid.

```typescript
function connCloseInit(identifier: Identifier, nextTimeoutHeight: uint64) {
  // defined in ICS 3
}
```

`connCloseConfirm` finalizes the graceful connection closing process. It will fail if the proof is invalid or if the identifier is invalid.

The default IBC relayer module will allow external calls to `connCloseConfirm`.

```typescript
function connCloseConfirm(identifier: Identifier, proofInit: CommitmentProof, proofHeight: uint64) {
  // defined in ICS 3
}
```

`queryConnection` queries for a connection by identifier.

```typescript
function queryConnection(id: Identifier): ConnectionEnd | void {
  // defined in ICS 3
}
```

### Channel lifecycle management

By default, channels are owned by the creating port, meaning only the module bound to that port can inspect, close, or send on the channel. A port can create any number of channels.

`chanOpenInit` tries to start the handshake to create a new channel with the provided options, failing if the connection is not found or the options are invalid.

```typescript
function chanOpenInit(
  connectionHops: [Identifier], portIdentifier: Identifier, channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier, counterpartyPortIdentifier: Identifier, nextTimeoutHeight: uint64) {
  // defined in ICS 4
}
```

`chanOpenTry` tries to initialize a channel based on proof of an initialization attempt on the counterparty chain, failing if the channel identifier is unavailable, the proof is invalid, or the calling module is not bound to the port.

The default IBC relayer module will allow external calls to `chanOpenTry`.

```typescript
function chanOpenTry(
  connectionHops: [Identifier], portIdentifier: Identifier, channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier, counterpartyChannelIdentifier: Identifier,
  timeoutHeight: uint64, nextTimeoutHeight: uint64, proofInit: CommitmentProof) {
  // defined in ICS 4
}
```

`chanOpenAck` acknowledges a channel creation in progress on another chain, failing if the channel identifier is not found, the proof is invalid, or the calling module is not bound to the port.

The default IBC relayer module will allow external calls to `chanOpenAck`.

```typescript
function chanOpenAck(
  portIdentifier: Identifier, channelIdentifier: Identifier,
  timeoutHeight: uint64, nextTimeoutHeight: uint64, proofTry: CommitmentProof) {
  // defined in ICS 4
}
```

`chanOpenConfirm` finalizes the channel opening handshake, failing if the channel identifier is not found, the proof is invalid, or the calling module is not bound to the port.

The default IBC relayer module will allow external calls to `chanOpenConfirm`.

```typescript
function chanOpenConfirm(
  portIdentifier: Identifier, channelIdentifier: Identifier,
  timeoutHeight: uint64, proofAck: CommitmentProof) {
  // defined in ICS 4
}
```

`chanOpenTimeout` proves that a channel opening handshake has timed-out and resets the process.

The default IBC relayer module will allow external calls to `chanOpenTimeout`.

```typescript
function chanOpenTimeout(
  portIdentifier: Identifier, channelIdentifier: Identifier,
  timeoutHeight: uint64, proofTimeout: CommitmentProof) {
  // defined in ICS 4
}
```

`queryChannel` queries an existing channel by known connection & channel identifier, returning the associated metadata if found.

```typescript
function queryChannel(connId: Identifier, chanId: Identifier): void {
  // defined in ICS 4
}
```

`chanCloseInit` initiates the channel closing handshake.

```typescript
function chanCloseInit(portIdentifier: Identifier, channelIdentifier: Identifier) {
  // defined in ICS 4
}
```

`chanCloseConfirm` acknowledges the closure of a channel on the counterparty chain and closes the corresponding end on this chain.

The default IBC relayer module will allow external calls to `chanCloseConfirm`.

```typescript
function chanCloseConfirm(
  portIdentifier: Identifier, channelIdentifier: Identifier,
  proofInit: CommitmentProof, proofHeight: uint64) {
  // defined in ICS 4
}
```

### Packet relay

Packets are permissioned by channel (only a port which owns a channel can send or receive on it).

`sendPacket` attempts to send a packet, returning an error if the packet cannot be sent (perhaps because the sending module is not bound to the port in question or because the channel is frozen), and returning a unique identifier if successful.

The returned identifier will be the same as that sent by the timeout handler `timeoutPacket`, so it can be used by the sending module to associate a specific action with a specific packet timeout.

The default IBC relayer module will allow external calls to `sendPacket`.

```typescript
function sendPacket(packet: Packet) {
  // defined in ICS 4
}
```

`recvPacket` attempts to receive a packet, returning an error if the calling module is not bound to the associated port, or if the packet does not exist or has been already handled.

The default IBC relayer module will allow external calls to `recvPacket`.

```typescript
function recvPacket(packet: Packet, proof: CommitmentProof) {
  // defined in ICS 4
}
```

`timeoutPacket` attemps to handle a packet timeout, returning an error if the calling module is not bound to the associated port, or if the packet does not exist, has not timed out, or has already been handled.

The default IBC relayer module will allow external calls to `timeoutPacket`.

```coffeescript
function timeoutPacket(packet: Packet, proof: CommitmentProof, nextSequenceRecv: uint64) {
  // defined in ICS 4
}
```

`recvTimeoutPacket` function is called by a module in order to process an IBC packet sent on the corresponding channel which has timed out.

The default IBC relayer module will allow external calls to `recvTimeoutPacket`.

```typescript
function recvTimeoutPacket(packet: Packet, proof: CommitmentProof) {
  // defined in ICS 4
}
```

### Properties & Invariants

The IBC handler module interface as defined here inherits properties of functions as defined in their associated specifications.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

The interface can change when implemented on new chains (or upgrades to an existing chain) as long as the semantics remain the same.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

9 June 2019 - Draft written

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
