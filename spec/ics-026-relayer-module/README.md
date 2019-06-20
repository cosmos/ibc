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

The default IBC handler uses a receiver call pattern, where modules must individually call the IBC handler in order to start handshakes, accept handshakes, send and receive packets, etc. This is flexible and simple (see [Design Patterns](../../ibc/5_IBC_DESIGN_PATTERNS.md))
but is a bit tricky to understand and may require extra work on the part of relayer processes, who must track the state of many modules. This standard describes an IBC "relayer module" to automate most common functionality, route packets, and simplify the task of relayers.

### Definitions

All functions provided by the IBC handler interface are defined as in [ICS 25](../ics-025-handler-interface).

### Desired Properties

- Modules should be able to own channels through the relayer module.
- No overhead should be added for packet sends and receives other than the layer of call indirection.

## Technical Specification

### Datagrams

*Datagrams* are external data blobs accepted as transactions by the relayer module.

#### Client lifecycle management

(todo)

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
interface ConnOpenAck {
  identifier: Identifier
  proofTry: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
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
interface ConnOpenTimeout {
  identifier: Identifier
  proofTimeout: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
interface ConnCloseInit {
  identifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnCloseTry {
  identifier: Identifier
  proofInit: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnCloseAck {
  identifier: Identifier
  proofTry: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
interface ConnCloseTimeout {
  identifier: Identifier
  proofTimeout: CommitmentProof
  timeoutHeight: uint64
}
```

#### Channel lifecycle management

```typescript
interface ChanOpenInit {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ChanOpenTry {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  moduleIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofInit: CommitmentProof
}
```

```typescript
interface ChanOpenAck {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofTry: CommitmentProof
}
```

```typescript
interface ChanOpenConfirm {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofAck: CommitmentProof
}
```

```typescript
interface ChanOpenTimeout {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTimeout: CommitmentProof
}
```

```typescript
interface ChanCloseInit {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ChanCloseTry {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofInit: CommitmentProof
}
```

```typescript
interface ChanCloseAck {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTry: CommitmentProof
}
```

```typescript
interface ChanCloseTimeout {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTimeout: CommitmentProof
}
```

#### Packet relay

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

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
