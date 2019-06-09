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

`Connection` and `ConnectionState` are as defined in [ICS 3](../ics-003-connection-semantics).

`Channel` and `Packet` are as defined in [ICS 4](../ics-004-channel-and-packet-semantics).

`CommitmentProof` is as defined in [ICS 23](../ics-023-vector-commitments).

`Identifier`s must conform to the schema defined in [ICS 24](../ics-024-host-requirements).

### Desired Properties

- Creation of clients, connections, and channels should be as permissionless as possible.
- The module set should be dynamic: chains should be able to add and destroy modules at will with a persistent IBC handler.
- Modules should be able to write their own more complex abstractions on top of IBC to provide additional semantics or guarantees.

## Technical Specification

### Clients

By default, clients are unowned: any module can create a new client, query any existing client, update any existing client, and delete any existing client not in use.

`createClient` creates a new client with a specified identifier.

```typescript
function createClient(id: Identifier, consensusState: ConsensusState): void {
  // defined in ICS 2
}
```

`queryClientConsensusState` queries a client by a known identifier, returning the associated consensus state if found.

```typescript
function queryClientConsensusState(id: string): ConsensusState | void {
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
function freezeClient(id: Identifier, headerOne: Header, headerTwo: Header): error | void {
  // defined in ICS 2
}
```

`deleteClient` deletes an existing client, returning an error if the identifier is not found or if the associated client was just created or is still in use by any connection.

```typescript
function deleteClient(id: Identifier): error | void {
  // defined in ICS 2
}
```

Implementations of `createClient`, `queryClientConsensusState`, `updateClient`, `freezeClient`, and `deleteClient` are defined in [ICS 2](../ics-002-consensus-verification).

### Connections

By default, connections are unowned. Connections can be closed by any module, but only when all channels associated with the connection have been closed by the modules which opened them and a timeout has passed since the connection was opened.

`ConnectionKind` enumerates the connection types supported by the handler implementation.

```golang
type ConnectionKind enum {
  Transmit
  Receive
  Broadcast
  Bidirectional
}
```

`ConnectionInfo` contains metadata about & state of an existing connection.

```golang
type ConnectionInfo struct {
  ConnectionOptions options
  ConnectionState   state
}
```

`initConnection` tries to create a new connection with the provided options, failing if the client is not found or the options are invalid.

```coffeescript
function initConnection(string identifier, string desiredVersion,
  string desiredCounterpartyIdentifier, string lightClientIdentifier) -> Maybe<err>
```

`tryConnection` tries to initialize a connection based on an initialization attempt on another chain (part one of the three-way connection handshake), returning an error if the proof was invalid or the requested identifier cannot be reserved.

The default IBC relayer module will allow external calls to `tryConnection`.

```coffeescript
function tryConnection(string desiredIdentifier, string counterpartyIdentifier, string desiredVersion,
  string counterpartyLightClientIdentifier, string lightClientIdentifier, CommitmentProof proofInit) -> Maybe<err>
```

`ackConnection` acknowledges a connection in progress on another chain (part two of the three-way connection handshake).

The default IBC relayer module will allow external calls to `ackConnection`.

```coffeescript
function ackConnection(string identifier, string agreedVersion, CommitmentProof proofTry)
```

`confirmConnection` finalizes a connection (part three of the three-way connection handshake).

The default IBC relayer module will allow external calls to `confirmConnection`.

```coffeescript
function confirmConnection(string identifier, CommitmentProof proofAck)
```

`timeoutConnection` handles a timeout in a connection opening handshake.

```coffeescript
function timeoutConnection(string identifier, CommitmentProof proofTimeout)
```

`queryConnection` queries an existing connection by known identifier, returning the associated metadata if found.

```coffeescript
function queryConnection(string identifier) -> Maybe<ConnectionInfo>
```

`initCloseConnection` initiates the graceful connection closing process. It will fail if there are any open channels using the connection or if the identifier is invalid.

```coffeescript
function initCloseConnection(string identifier) -> Maybe<Err>
```

`tryCloseConnection` continues the graceful connection closing process. It will fail if there are any open channels using the connection, if the proof is invalid, or if the identifier is invalid.

The default IBC relayer module will allow external calls to `tryCloseConnection`.

```coffeescript
function tryCloseConnection(string identifier, proofInit) -> Maybe<Err>
```

`ackCloseConnection` finalizes the graceful connection closing process. It will fail if the proof is invalid or if the identifier is invalid.

The default IBC relayer module will allow external calls to `ackCloseConnection`.

```coffeescript
function ackCloseConnection(string identifier, proofTry) -> Maybe<Err>
```

Implementations of `initConnection`, `tryConnection`, `ackConnection`, `confirmConnection`, `queryConnection`, `initCloseConnection`, `tryCloseConnection`, and `ackCloseConnection` are defined in [ICS 3](../ics-003-connection-semantics).

### Channels

By default, channels are owned by the creating module, meaning only the creating module can inspect, close, or send on the channel. A module can create any number of channels.

`ChannelOptions` contains all the parameter choices required to create a new channel.

```golang
type ChannelOptions struct {
  string          connectionIdentifier
  bool            ordered
}
```

`ChannelInfo` contains metadata about an existing channel.

```golang
type ChannelInfo struct {
  string         channelIdentifier
  ChannelOptions options
}
```

`initChannel` tries to start the handshake to create a new channel with the provided options, failing if the connection is not found or the options are invalid.

```coffeescript
function initChannel(ChannelOptions options) -> Maybe<err>
```

`tryChannel` tries to initialize a channel based on proof of an initialization attempt on the counterparty chain, failing if the channel identifier is unavailable, the proof is invalid, or the calling module is not authorized.

```coffeescript
function tryChannel() -> Maybe<Err>
```

`ackChannel` acknowledges a channel creation in progress on another chain, failing if the channel identifier is not found, the proof is invalid, or the calling module is not authorized.

```coffeescript
function ackChannel() -> Maybe<Err>
```

`confirmChannel` finalizes the channel opening handshake, failing if the channel identifier is not found, the proof is invalid, or the calling module is not authorized.

```coffeescript
function confirmChannel() -> Maybe<Err>
```

`queryChannel` queries an existing channel by known identifier, returning the associated metadata if found.

```coffeescript
function queryChannel(string identifier) -> Maybe<ChannelInfo>
```

`closeChannel` initiates the graceful channel closing process as defined in [ICS 4](../ics-004-channel-and-packet-semantics).

```coffeescript
function closeChannel(string identifier) -> Future<Maybe<Err>>
```

Implementations of `createChannel`, `queryChannel`, and `closeChannel` are defined in [ICS 4](../ics-004-channel-and-packet-semantics).

### Packets

Packets are permissioned by channel (only a module which owns a channel can send on it).

`sendPacket` attempts to send a packet, returning an error if the packet cannot be sent (perhaps because the sending module does not own the channel in question or because the channel is frozen), and returning a unique identifier if successful.

The returned identifier will be the same as that sent by the timeout handler `timeoutPacket`, so it can be used by the sending module to associate a specific action with a specific packet timeout.

```coffeescript
function sendPacket(Packet packet) -> string | Err
```

`recvPacket` attempts to receive a packet, returning an error if the calling module is not authorized to handle the packet, or if the packet does not exist or has been already handled.

```coffeescript
function recvPacket(Packet packet) -> string | Err
```

`timeoutPacket` attemps to handle a packet timeout, returning an error if the calling module is not authorized to handle the packet timeout, or if the packet does not exist, has not timed out, or has already been handled.

```coffeescript
function timeoutPacket(Packet packet) -> string | Err
```

Implementations of `sendPacket`, `recvPacket`, and `timeoutPacket` are defined in ICS 5.

#### Example

As a demonstration of interface usage, a simple module handling send/receive of a native asset could be implemented as follows:

```golang
type State struct {
  channel     string
}
```

```golang
type PacketData struct {
  asset         string
  amount        integer
  source        address
  destination   address
}
```

```coffeescript
function myModuleInit()
  client = createClient(consensusState)
  connection = createConnection(nil, client)
  state.channel = createChannel(nil, connection, myModuleRecv, myModuleTimeout)
```

```coffeescript
function myModuleSend(string asset, integer amount, address source, address destination)
  checkSignature(source)
  deductBalance(source, asset, amount)
  escrow(asset, amount)
  sendPacket({
    channel: state.channel,
    data: {
      asset       : asset,
      amount      : amount,
      source      : source,
      destination : destination,
    }
  })
```

```coffeescript
function myModuleRecv(Packet packet)
  recvPacket(packet)
  assert(packet.channel == channel)
  data = packet.data
  unescrow(data.asset, data.amount)
  increaseBalance(data.destination, data.asset, data.amount)
```

```coffeescript
function myModuleTimeout(Packet packet)
  timeoutPacket(packet)
  data = packet.data
  unescrow(packet.asset, packet.amount)
  increaseBalance(packet.source, packet.asset, packet.amount)
```

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

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
