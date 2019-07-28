---
ics: 26
title: Relayer Module
stage: Draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-06-09
modified: 2019-06-09
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

### Datagrams

*Datagrams* are external data blobs accepted as transactions by the relayer module.

#### Client lifecycle management

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
}
```

#### Connection lifecycle management

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

- expose to modules (hook): on-close hooks

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
interface ChanOpenAck {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofTry: CommitmentProof
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
interface ChanOpenTimeout {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTimeout: CommitmentProof
}
```

```typescript
interface ChanCloseInit {
  portIdentifier: Identifier
  channelIdentifier: Identifier
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

- expose publicly (write): chanopeninit, chanopentry, chanopenack, chanopenconfirm, chanopentimeout, chancloseconfirm
- expose to modules (hooks): chanopeninit, chanopentry, chanopenack, chanopenconfirm, chanopentimeout, chancloseconfirm
- expose to modules (write): chancloseinit
- expose publicly (read): query
- expose to modules (read): query

#### Packet relay

```typescript
interface PacketSend {
  packet: Packet
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

- call handlePacket on module

```typescript
interface PacketTimeoutOrdered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  nextSequenceRecv: uint64
}
```

- call handlePacketTimeout on module

```typescript
interface PacketTimeoutUnordered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
}
```

- call handlePacketTimeout on module

```typescript
interface PacketTimeoutClose {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
}
```

- on-close hooks

```typescript
interface PacketCleanupOrdered {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  nextSequenceRecv: uint64
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

- expose publicly (write): recvpacket, all timeouts, cleanup
- expose publicly (read): query
- expose to modules (write): sendpacket
- expose to modules (read): query

### Subprotocols

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
