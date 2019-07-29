---
ics: 26
title: Relayer Module
stage: Draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-06-09
modified: 2019-07-28
---

## Synopsis

The relayer module is a default implementation of a secondary module which will accept external datagrams and call into the interblockchain communication protocol handler to deal with handshakes and packet relay.
The relayer module can keep a lookup table of modules, which it can use to look up and call a module when a packet is received, so that external relayers need only ever relay packets to the relayer module.

### Motivation

The default IBC handler uses a receiver call pattern, where modules must individually call the IBC handler in order to bind to ports, start handshakes, accept handshakes, send and receive packets, etc. This is flexible and simple (see [Design Patterns](../../ibc/5_IBC_DESIGN_PATTERNS.md))
but is a bit tricky to understand and may require extra work on the part of relayer processes, who must track the state of many modules. This standard describes an IBC "relayer module" to automate most common functionality, route packets, and simplify the task of relayers.

### Definitions

All functions provided by the IBC handler interface are defined as in [ICS 25](../ics-025-handler-interface).

### Desired Properties

- Modules should be able to bind to ports and own channels through the relayer module.
- No overhead should be added for packet sends and receives other than the layer of call indirection.
- The relayer module should call specified handler functions on modules when they need to act upon packets.

## Technical Specification

### Datagram handlers (write)

*Datagrams* are external data blobs accepted as transactions by the relayer module.
This section defines a *handler function* for each datagram, which is executed when the associated datagram is submitted to the relayer module in a transaction.
All datagrams can also be safely submitted by other modules to the relayer module.
No message signatures or data validity checks are assumed beyond those which are explicitly indicated.

#### Client lifecycle management

`ClientCreate` creates a new light client with the specified identifier & consensus state.

```typescript
interface ClientCreate {
  identifier: Identifier
  consensusState: ConsensusState
}
```

```typescript
function handleClientCreate(datagram: ClientCreate) {
  handler.createClient(datagram.identifier, datagram.consensusState)
}
```

`ClientUpdate` updates an existing light client with the specified identifier & new header.

```typescript
interface ClientUpdate {
  identifier: Identifier
  header: Header
}
```

```typescript
function handleClientUpdate(datagram: ClientUpdate) {
  handler.updateClient(datagram.identifier, datagram.header)
}
```

`ClientFreeze` freezes an existing light client with the specified identifier by proving that an equivocation has occurred.

```typescript
interface ClientFreeze {
  identifier: Identifier
  firstHeader: Header
  secondHeader: Header
}
```

```typescript
function handleClientFreeze(datagram: ClientUpdate) {
  handler.freezeClient(datagram.identifier, datagram.firstHeader, datagram.secondHeader)

  for (const identifier in handler.queryClientConnections(client))
    handler.handleConnCloseInit(identifier)

  // TODO: on-close hooks for modules owning channels?
}
```

#### Connection lifecycle management

The `ConnOpenInit` datagram starts the connection handshake process with an IBC module on another chain.

```typescript
interface ConnOpenInit {
  identifier: Identifier
  desiredCounterpartyIdentifier: Identifier
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
function handleConnOpenInit(datagram: ConnOpenInit) {
  handler.connOpenInit(
    datagram.identifier, datagram.desiredCounterpartyIdentifier, datagram.clientIdentifier,
    datagram.counterpartyClientIdentifier, datagram.nextTimeoutHeight
  )
}
```

The `ConnOpenTry` datagram accepts a handshake request from an IBC module on another chain.

```typescript
interface ConnOpenTry {
  desiredIdentifier: Identifier
  counterpartyConnectionIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  clientIdentifier: Identifier
  proofInit: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
}
```

```typescript
function handleConnOpenTry(datagram: ConnOpenTry) {
  handler.connOpenTry(
    datagram.desiredIdentifier, datagram.counterpartyConnectionIdentifier, datagram.counterpartyClientIdentifier,
    datagram.clientIdentifier, datagram.proofInit, datagram.timeoutHeight, datagram.nextTimeoutHeight
  )
}
```

The `ConnOpenAck` datagram confirms a handshake acceptance by the IBC module on another chain.

```typescript
interface ConnOpenAck {
  identifier: Identifier
  proofTry: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
}
```

```typescript
function handleConnOpenAck(datagram: ConnOpenAck) {
  handler.connOpenAck(
    datagram.identifier, datagram.proofTry,
    datagram.timeoutHeight, datagram.nextTimeoutHeight
  )
}
```

The `ConnOpenConfirm` datagram acknowledges a handshake acknowledgement by an IBC module on another chain & finalizes the connection.

```typescript
interface ConnOpenConfirm {
  identifier: Identifier
  proofAck: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
function handleConnOpenConfirm(datagram: ConnOpenConfirm) {
  handler.connOpenConfirm(
    datagram.identifier, datagram.proofAck, datagram.timeoutHeight
  )
}
```

The `ConnOpenTimeout` datagram proves that a connection handshake has timed out prior to completion, resetting the state.

```typescript
interface ConnOpenTimeout {
  identifier: Identifier
  proofTimeout: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
function handleConnOpenTimeout(datagram: ConnOpenTimeout) {
  handler.handleConnOpenTimeout(
    datagram.identifier, datagram.proofTimeout, datagram.timeoutHeight
  )
}
```

The `ConnCloseInit` datagram closes an unused connection.

```typescript
interface ConnCloseInit {
  identifier: Identifier
}
```

```typescript
function handleConnCloseInit(datagram: ConnCloseInit) {
  handler.handleConnCloseInit(identifier)
}
```

The `ConnCloseConfirm` datagram acknowledges that a connection has been closed on the counterparty chain and closes the end on this chain.

```typescript
interface ConnCloseConfirm {
  identifier: Identifier
  proofInit: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleConnCloseConfirm(datagram: ConnCloseConfirm) {
  handler.handleConnCloseConfirm(
    datagram.identifier, datagram.proofInit, datagram.proofHeight
  )
}
```


#### Channel lifecycle management

```typescript
interface ChanOpenInit {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  connectionHops: [Identifier]
  nextTimeoutHeight: uint64
}
```

```typescript
function handleChanOpenInit(datagram: ChanOpenInit) {
  module = lookupModule(datagram.portIdentifier)
  module.handleChanOpenInit(datagram)
  handler.chanOpenInit(
    datagram.portIdentifier, datagram.channelIdentifier, datagram.counterpartyPortIdentifier,
    datagram.counterpartyChannelIdentifier, datagram.connectionHops, datagram.nextTimeoutHeight
  )
}
```

```typescript
interface ChanOpenTry {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  connectionHops: [Identifier]
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofInit: CommitmentProof
}
```

```typescript
function handleChanOpenTry(datagram: ChanOpenTry) {
  module = lookupModule(datagram.portIdentifier)
  module.handleChanOpenTry(datagram)
  handler.chanOpenTry(
    datagram.portIdentifier, datagram.channelIdentifier, datagram.counterpartyPortIdentifier,
    datagram.counterpartyChannelIdentifier, datagram.connectionHops, datagram.timeoutHeight,
    datagram.nextTimeoutHeight, datagram.proofInit
  )
}
```

```typescript
interface ChanOpenAck {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofTry: CommitmentProof
}
```

```typescript
function handleChanOpenAck(datagram: ChanOpenAck) {
  handler.chanOpenAck(
    datagram.portIdentifier, datagram.channelIdentifier, datagram.timeoutHeight,
    datagram.nextTimeoutHeight, datagram.proofTry
  )
}
```

```typescript
interface ChanOpenConfirm {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofAck: CommitmentProof
}
```

```typescript
function handleChanOpenConfirm(datagram: ChanOpenConfirm) {
  handler.chanOpenConfirm(
    datagram.portIdentifier, datagram.channelIdentifier,
    datagram.timeoutHeight, datagram.proofAck
  )
}
```

```typescript
interface ChanOpenTimeout {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTimeout: CommitmentProof
}
```

```typescript
function handleChanOpenTimeout(datagram: ChanOpenTimeout) {
  module = lookupModule(datagram.portIdentifier)
  module.chanOpenTimeout(datagram)
  handler.chanOpenTimeout(
    datagram.portIdentifier, datagram.channelIdentifier,
    datagram.timeoutHeight, datagram.proofTimeout
  )
}
```

Channel closure can only be initiated by the owning module directly (so there is no associated datagram).

```typescript
function handleChanCloseInit(portIdentifier: Identifier, channelIdentifier: Identifier) {
  handler.chanCloseInit(portIdentifier, channelIdentifier)
}
```

```typescript
interface ChanCloseConfirm {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  proofInit: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleChanCloseConfirm(datagram: ChanCloseConfirm) {
  module = lookupModule(datagram.portIdentifier)
  module.chanCloseConfirm(datagram)
  handler.chanCloseConfirm(
    datagram.portIdentifier, datagram.channelIdentifier,
    datagram.proofInit, datagram.proofHeight
  )
}
```

#### Packet relay

`sendPacket` can only be invoked directly by the module owning the channel on which the packet is to be sent, so there is no associated datagram.

```typescript
function handlePacketSend(packet: Packet) {
  // auth module
  handler.sendPacket(packet)
}
```

```typescript
interface PacketRecv {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  acknowledgement: bytes
}
```

```typescript
function handlePacketRecv(datagram: PacketRecv) {
  module = lookupModule(datagram.portIdentifier)
  module.recvPacket(datagram.packet)
  handler.recvPacket(packet, proof, proofHeight, acknowledgement)
}
```

```typescript
interface PacketTimeoutOrdered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  nextSequenceRecv: uint64
}
```

```typescript
function handlePacketTimeoutOrdered(datagram: PacketTimeoutOrdered) {
  handler.timeoutPacketOrdered(
    datagram.packet, datagram.proof, datagram.proofHeight, datagram.nextSequenceRecv
  )
  module.timeoutPacket(datagram.packet)
}
```

```typescript
interface PacketTimeoutUnordered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handlePacketTimeoutUnordered(datagram: PacketTimeoutUnordered) {
  handler.timeoutPacketUnordered(datagram.packet, datagram.proof, datagram.proofHeight)
  module.timeoutPacket(datagram.packet)
}
```

```typescript
interface PacketTimeoutClose {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handlePacketTimeoutClose(datagram: PacketTimeoutClose) {
  handler.timeoutPacketClose(datagram.packet, datagram.proof, datagram.proofHeight)
}
```

```typescript
interface PacketCleanupOrdered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  nextSequenceRecv: uint64
}
```

```typescript
function handlePacketCleanupOrdered(datagram: PacketCleanupOrdered) {
  handler.cleanupPacketOrdered(
    datagram.packet, datagram.proof,
    datagram.proofHeight, datagram.nextSequenceRecv
  )
}
```

```typescript
interface PacketCleanupUnordered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  acknowledgement: bytes
}
```

```typescript
function handlePacketCleanupUnordered(datagram: PacketCleanupUnordered) {
  handler.cleanupPacketUnordered(
    datagram.packet, datagram.proof,
    datagram.proofHeight, datagram.acknowledgement
  )
}
```

### Query (read-only) functions

- query clients
- query connections
- query channels

### Interface usage example

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

Relayer modules are closely tied to the IBC handler interface.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

June 9 2019 - Draft submitted
July 28 2019 - Major revisions

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
