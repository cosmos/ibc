---
ics: 4
title: Channel & Packet Semantics
stage: draft
category: ibc-core
requires: 2, 3, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-05-17
---

## Synopsis

The "channel" abstraction provides message delivery semantics to the interblockchain communication protocol, in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one chain and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination chain. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortizing the cost of header verification across all the channels utilizing a connection & light client.

Channels are payload-agnostic. The module which receives an IBC packet on chain `B` decides how to act upon the incoming data, and must utilize its own application logic to determine which state transactions to apply according to what data the packet contains. Both chains must only agree that the packet has been received.

### Motivation

The interblockchain communication protocol uses a cross-chain message passing model which makes no assumptions about network synchrony. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are public and can be read from a blockchain by any relayer and submitted to any other blockchain. In order to provide the desired ordering, exactly-once delivery, and module permissioning semantics to the application layer, the interblockchain communication protocol must implement an abstraction to enforce these semantics — channels are this abstraction.

### Definitions

`Connection` is as defined in [ICS 3](../ics-3-connection-semantics).

`Commitment`, `CommitmentProof`, and `CommitmentRoot` are as defined in [ICS 23](../ics-23-vector-commitments).

`Identifier`, `get`, `set`, `delete`, and module-system related primitives are as defined in [ICS 24](../ics-24-host-requirements).

A *channel* is a pipeline for exactly-once packet delivery between specific modules on separate blockchains, which has at least one send and one receive end.

A *bidirectional* channel is a channel where packets can flow in both directions: from `A` to `B` and from `B` to `A`.

A *unidirectional* channel is a channel where packets can only flow in one direction: from `A` to `B`.

An *ordered* channel is a channel where packets are delivered exactly in the order which they were sent.

An *unordered* channel is a channel where packets can be delivered in any order, which may differ from the order in which they were sent.

Directionality and ordering are independent, so one can speak of a bidirectional unordered channel, a unidirectional ordered channel, etc.

All channels provide exactly-once packet delivery.

This specification only concerns itself with *bidirectional ordered* channels. *Unidirectional* and *unordered* channels use almost exactly the same protocol and will be outlined in a future ICS.

Channels have a *state*:

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  TRYCLOSE,
  CLOSED,
}
```

An *end* of a channel is a data structure on one chain storing channel metadata:

```typescript
interface ChannelEnd {
  state: ChannelState
  counterpartyChannelIdentifier: Identifier
  moduleIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  connectionIdentifier: Identifier
  counterpartyConnectionIdentifier: Identifier
  version: Version
  nextSequenceSend: uint64
  nextSequenceRecv: uint64
}
```

An IBC *packet* is a particular datagram, defined as follows:

```typescript
interface Packet {
  sequence: uint64
  sourceChannel: Identifier
  destChannel: Identifier
  data: bytes
}
```

### Desired Properties

#### Efficiency

- The speed of packet transmission and confirmation should be limited only by the speed of the underlying chains.

#### Exactly-once delivery

- Exactly-once packet delivery, without assumptions of network synchrony and even if one or both of the chains should halt (no-more-than-once delivery in that case).

#### Ordering

- Ordering, for ordered channels, whereby if packet *x* is sent before packet *y* on chain `A`, packet *x* must be received before packet *y* on chain `B`.

IBC channels implement a vector clock for the restricted case of two processes (in our case, blockchains). Given *x* → *y* means *x* is causally before *y*, chains `A` and `B`, and *a* ⇒ *b* means *a* implies *b*:

Every transaction on the same chain already has a well-defined causality relation (order in history). IBC provides an ordering guarantee across two chains which can be used to reason about the combined state of both chains as a whole.

For example, an application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

#### Permissioning

- Channel ends should be permissioned to one module on each end.

## Technical Specification

![channel-state-machine](channel-state-machine.png)

### Channel opening handshake

```typescript
interface ChanOpenInit {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  version: Version
}
```

```typescript
function chanOpenInit() {
  moduleIdentifier = getCallingModule()
  assert(get("connections/{connectionIdentifier}/channels/{channelIdentifier}") === null)
  connection = get("connections/{connectionIdentifier}")
  assert(connection.state === OPEN)
  channel = Channel{INIT, moduleIdentifier, counterpartyModuleIdentifier, counterpartyChannelIdentifier,
                    connectionIdentifier, counterpartyConnectionIdentifier, version, 0, 0}
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}", channel)
}
```

```typescript
interface ChanOpenTry {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  moduleIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  requestedVersion: Version
  acceptedVersion: Version
  proofInit: CommitmentProof
}
```

```typescript
function chanOpenTry() {
  assert(get("connections/{connectionIdentifier}/channels/{channelIdentifier}") === null)
  assert(getCallingModule() === moduleIdentifier)
  connection = get("connections/{connectionIdentifier}")
  assert(connection.state === OPEN)
  consensusState = get("clients/{connection.clientIdentifier}")
  assert(verifyMembership(
    consensusState,
    proofInit,
    "connections/{connectionIdentifier}/channels/{connection.counterpartyChannelIdentifier}",
    Channel{INIT, counterpartyModuleIdentifier, moduleIdentifier, channelIdentifier,
            connection.counterpartyConnectionIdentifier, connectionIdentifier, requestedVersion, 0, 0}
  ))
  channel = Channel{TRYOPEN, moduleIdentifier, counterpartyModuleIdentifier, counterpartyChannelIdentifier,
                    connectionIdentifier, counterpartyConnectionIdentifier, acceptedVersion, 0, 0}
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}", channel)
}
```

```typescript
interface ChanOpenAck {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  acceptedVersion: Version
  proofTry: CommitmentProof
}
```

```typescript
function chanOpenAck() {
  channel = get("connections/{connectionIdentifier}/channels/{channelIdentifier}")
  assert(channel.state === INIT)
  assert(getCallingModule() === channel.moduleIdentifier)
  connection = get("connections/{connectionIdentifier}")
  assert(connection.state === OPEN)
  consensusState = get("clients/{connection.clientIdentifier}")
  assert(verifyMembership(
    consensusState.getRoot(),
    proofTry,
    "connections/{connectionIdentifier}/channels/{channel.counterpartyChannelIdentifier}",
    Channel{TRYOPEN, channel.counterpartyModuleIdentifier, channel.moduleIdentifier, channelIdentifier,
            channel.counterpartyConnectionIdentifier, connectionIdentifier, acceptedVersion, 0, 0}
  ))
  channel.state = OPEN
  channel.version = acceptedVersion
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}", channel)
}
```

```typescript
interface ChanOpenConfirm {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  proofAck: CommitmentProof
}
```

```typescript
function chanOpenConfirm() {
  channel = get("connections/{connectionIdentifier}/channels/{channelIdentifier}")
  assert(channel.state === TRYOPEN)
  assert(getCallingModule() === channel.moduleIdentifier)
  connection = get("connections/{connectionIdentifier}")
  assert(connection.state === OPEN)
  consensusState = get("clients/{connection.clientIdentifier}")
  assert(verifyMembership(
    consensusState.getRoot(),
    proofAck,
    "connections/{connectionIdentifier}/channels/{channel.counterpartyChannelIdentifier}",
    Channel{OPEN, channel.counterpartyModuleIdentifier, channel.moduleIdentifier, channelIdentifier,
            channel.counterpartyConnectionIdentifier, connectionIdentifier, version, 0, 0}
  ))
  channel.state = OPEN
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}", channel)
}
```

### Channel closing handshake

(todo, isomorphic to connection closing handshake)

![packet-state-machine](packet-state-machine.png)

### Sending packets

`sendPacket` is called by a module.

```typescript
function sendPacket(packet: Packet) {
  channel = get("connections/{connectionIdentifier}/channels/{channelIdentifier}")
  assert(channel.state === OPEN)
  assert(getCallingModule() === channel.moduleIdentifier)
  connection = get("connections/{connectionIdentifier}")
  assert(connection.state === OPEN)
  consensusState = get("clients/{connection.clientIdentifier}")
  assert(consensusState.getHeight() < packet.timeoutHeight)
  assert(packet.sequence === channel.nextSequenceSend)
  channel.nextSequenceSend = channel.nextSequenceSend + 1
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}", channel)
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}/packets/{sequence}", commit(packet.data))
}
```

### Receiving packets

`recvPacket` is called by a module.

```typescript
function recvPacket(packet: Packet) {
  channel = get("connections/{connectionIdentifier}/channels/{channelIdentifier}")
  assert(channel.state === OPEN)
  assert(getCallingModule() === channel.moduleIdentifier)
  assert(sequence === channel.nextSequenceRecv)
  connection = get("connections/{connectionIdentifier}")
  assert(connection.state === OPEN)
  consensusState = getConsensusState()
  assert(consensusState.getHeight() < packet.timeoutHeight)
  key = "connections/{channel.counterpartyConnectionIdentifier}/channels/" ++
        "{channel.counterpartyChannelIdentifier}/packets/{sequence}"
  assert(verifyMembership(
    consensusState.getRoot(),
    proof,
    key,
    commit(packet.data)
  ))
  channel.nextSequenceRecv = channel.nextSequenceRecv + 1
  set("connections/{connectionIdentifier}/channels/{channelIdentifier}", channel)
}
```

todo: clear up to timeout

### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves.

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

`timeoutPacket` is called by a module.

```typescript
function timeoutPacket(packet: Packet) {
  (state, moduleIdentifier, _, _, _, _, _) = get("connections/{connectionIdentifier}/channels/{channelIdentifier}")
  assert(state === OPEN)
  assert(getCallingModule() === moduleIdentifier)

  // check that this packet has not yet been received
  assert(nextSequenceRecv <= packet.sequence)

  // check that timeout height has passed
  assert(consensusState.getHeight() >= timeoutHeight)

  // verify we actually sent this packet, check the store
  assert(
    get("connections/{connectionIdentifier}/channels/{channelIdentifier}/packets/{sequence}") === commit(packet.data)
  )

  // check that the recv sequence is as claimed
  assert(verifyMembership(
    consensusState.getRoot(),
    proof,
    "connections/{connectionIdentifier}/channels/{channelIdentifier}/nextSequenceRecv",
    nextSequenceRecv
  ))

  // clear the store so we can't "timeout" again"
  delete("connections/{connectionIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
}
```

Notes
- Can "bulk timeout" because of ordering guarantees if we enforce relations between timeout height and channels

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Data structures & encoding can be versioned at the connection or channel level.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

17 May 2019 - Draft submitted

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
