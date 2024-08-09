---
ics: 4
title: Channel & Packet Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 3, 5, 24
version compatibility: ibc-go v7.0.0
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## Synopsis

The "channel" abstraction provides message delivery semantics to the interblockchain communication protocol, in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one chain and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination chain. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortising the cost of header verification across all the channels utilising a connection & light client.

Channels are payload-agnostic. The modules which send and receive IBC packets decide how to construct packet data and how to act upon the incoming packet data, and must utilise their own application logic to determine which state transactions to apply according to what data the packet contains.

### Motivation

The interblockchain communication protocol uses a cross-chain message passing model. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are visible to relayers and can be read from a blockchain by any relayer process and submitted to any other blockchain.

The IBC protocol must provide ordering (for ordered channels) and exactly-once delivery guarantees to allow applications to reason about the combined state of connected modules on two chains.

> **Example**: An application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

In order to provide the desired ordering, exactly-once delivery, and module permissioning semantics to the application layer, the interblockchain communication protocol must implement an abstraction to enforce these semantics — channels are this abstraction.

### Definitions

`ConsensusState` is as defined in [ICS 2](../ics-002-client-semantics).

`Connection` is as defined in [ICS 3](../ics-003-connection-semantics).

`Port` and `authenticateCapability` are as defined in [ICS 5](../ics-005-port-allocation).

`hash` is a generic collision-resistant hash function, the specifics of which must be agreed on by the modules utilising the channel. `hash` can be defined differently by different chains.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

See [upgrades spec](./UPGRADES.md) for definition of `pendingInflightPackets` and `restoreChannel`.

A *channel* is a pipeline for exactly-once packet delivery between specific modules on separate blockchains, which has at least one end capable of sending packets and one end capable of receiving packets.

A *bidirectional* channel is a channel where packets can flow in both directions: from `A` to `B` and from `B` to `A`.

A *unidirectional* channel is a channel where packets can only flow in one direction: from `A` to `B` (or from `B` to `A`, the order of naming is arbitrary).

An *ordered* channel is a channel where packets are delivered exactly in the order which they were sent. This channel type offers a very strict guarantee of ordering. Either, the packets are received in the order they were sent, or if a packet in the sequence times out; then all future packets are also not receivable and the channel closes.

An *ordered_allow_timeout* channel is a less strict version of the *ordered* channel. Here, the channel logic will take a *best effort* approach to delivering the packets in order. In a stream of packets, the channel will relay all packets in order and if a packet in the stream times out, the timeout logic for that packet will execute and the rest of the later packets will continue processing in order. Thus, we **do not close** the channel on a timeout with this channel type.

An *unordered* channel is a channel where packets can be delivered in any order, which may differ from the order in which they were sent.

```typescript
enum ChannelOrder {
  ORDERED,
  UNORDERED,
  ORDERED_ALLOW_TIMEOUT,
}
```

Directionality and ordering are independent, so one can speak of a bidirectional unordered channel, a unidirectional ordered channel, etc.

All channels provide exactly-once packet delivery, meaning that a packet sent on one end of a channel is delivered no more and no less than once, eventually, to the other end.

This specification only concerns itself with *bidirectional* channels. *Unidirectional* channels can use almost exactly the same protocol and will be outlined in a future ICS.

An end of a channel is a data structure on one chain storing channel metadata:

```typescript
interface ChannelEnd {
  state: ChannelState
  ordering: ChannelOrder
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  connectionHops: [Identifier]
  version: string
  upgradeSequence: uint64
}
```

- The `state` is the current state of the channel end.
- The `ordering` field indicates whether the channel is `unordered`, `ordered`, or `ordered_allow_timeout`.
- The `counterpartyPortIdentifier` identifies the port on the counterparty chain which owns the other end of the channel.
- The `counterpartyChannelIdentifier` identifies the channel end on the counterparty chain.
- The `nextSequenceSend`, stored separately, tracks the sequence number for the next packet to be sent.
- The `nextSequenceRecv`, stored separately, tracks the sequence number for the next packet to be received.
- The `nextSequenceAck`, stored separately, tracks the sequence number for the next packet to be acknowledged.
- The `connectionHops` stores the list of connection identifiers ordered starting from the receiving end towards the sender. `connectionHops[0]` is the connection end on the receiving chain. More than one connection hop indicates a multi-hop channel.
- The `version` string stores an opaque channel version, which is agreed upon during the handshake. This can determine module-level configuration such as which packet encoding is used for the channel. This version is not used by the core IBC protocol. If the version string contains structured metadata for the application to parse and interpret, then it is considered best practice to encode all metadata in a JSON struct and include the marshalled string in the version field.

See the [upgrade spec](./UPGRADES.md) for details on `upgradeSequence`.

Channel ends have a *state*:

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  CLOSED,
  FLUSHING,
  FLUSHINGCOMPLETE,
}
```

- A channel end in `INIT` state has just started the opening handshake.
- A channel end in `TRYOPEN` state has acknowledged the handshake step on the counterparty chain.
- A channel end in `OPEN` state has completed the handshake and is ready to send and receive packets.
- A channel end in `CLOSED` state has been closed and can no longer be used to send or receive packets.

See the [upgrade spec](./UPGRADES.md) for details on `FLUSHING` and `FLUSHCOMPLETE`.

A `Packet`, in the interblockchain communication protocol, is a particular interface defined as follows:

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: Height
  timeoutTimestamp: uint64
  sourcePort: Identifier
  sourceChannel: Identifier
  destPort: Identifier
  destChannel: Identifier
  data: bytes
}
```

- The `sequence` number corresponds to the order of sends and receives, where a packet with an earlier sequence number must be sent and received before a packet with a later sequence number.
- The `timeoutHeight` indicates a consensus height on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
- The `timeoutTimestamp` indicates a timestamp on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
- The `sourcePort` identifies the port on the sending chain.
- The `sourceChannel` identifies the channel end on the sending chain.
- The `destPort` identifies the port on the receiving chain.
- The `destChannel` identifies the channel end on the receiving chain.
- The `data` is an opaque value which can be defined by the application logic of the associated modules.

Note that a `Packet` is never directly serialised. Rather it is an intermediary structure used in certain function calls that may need to be created or processed by modules calling the IBC handler.

An `OpaquePacket` is a packet, but cloaked in an obscuring data type by the host state machine, such that a module cannot act upon it other than to pass it to the IBC handler. The IBC handler can cast a `Packet` to an `OpaquePacket` and vice versa.

```typescript
type OpaquePacket = object
```

In order to enable new channel types (e.g. ORDERED_ALLOW_TIMEOUT), the protocol introduces standardized packet receipts that will serve as sentinel values for the receiving chain to explicitly write to its store the outcome of a `recvPacket`.

```typescript
enum PacketReceipt {
  SUCCESSFUL_RECEIPT,
  TIMEOUT_RECEIPT,
}
```

### Desired Properties

#### Efficiency

- The speed of packet transmission and confirmation should be limited only by the speed of the underlying chains.
  Proofs should be batchable where possible.

#### Exactly-once delivery

- IBC packets sent on one end of a channel should be delivered exactly once to the other end.
- No network synchrony assumptions should be required for exactly-once safety.
  If one or both of the chains halt, packets may be delivered no more than once, and once the chains resume packets should be able to flow again.

#### Ordering

- On *ordered* channels, packets should be sent and received in the same order: if packet *x* is sent before packet *y* by a channel end on chain `A`, packet *x* must be received before packet *y* by the corresponding channel end on chain `B`. If packet *x* is sent before packet *y* by a channel and packet *x* is timed out; then packet *y* and any packet sent after *x* cannot be received.
- On *ordered_allow_timeout* channels, packets should be sent and received in the same order: if packet *x* is sent before packet *y* by a channel end on chain `A`, packet *x* must be received **or** timed out before packet *y* by the corresponding channel end on chain `B`.
- On *unordered* channels, packets may be sent and received in any order. Unordered packets, like ordered packets, have individual timeouts specified in terms of the destination chain's height.

#### Permissioning

- Channels should be permissioned to one module on each end, determined during the handshake and immutable afterwards (higher-level logic could tokenize channel ownership by tokenising ownership of the port).
  Only the module associated with a channel end should be able to send or receive on it.

## Technical Specification

### Dataflow visualisation

The architecture of clients, connections, channels and packets:

![Dataflow Visualisation](dataflow.png)

### Preliminaries

#### Store paths

Channel structures are stored under a store path prefix unique to a combination of a port identifier and channel identifier:

```typescript
function channelPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelEnds/ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

The capability key associated with a channel is stored under the `channelCapabilityPath`:

```typescript
function channelCapabilityPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
  return "{channelPath(portIdentifier, channelIdentifier)}/key"
}
```

The `nextSequenceSend`, `nextSequenceRecv`, and `nextSequenceAck` unsigned integer counters are stored separately so they can be proved individually:

```typescript
function nextSequenceSendPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "nextSequenceSend/ports/{portIdentifier}/channels/{channelIdentifier}"
}

function nextSequenceRecvPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "nextSequenceRecv/ports/{portIdentifier}/channels/{channelIdentifier}"
}

function nextSequenceAckPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "nextSequenceAck/ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

Constant-size commitments to packet data fields are stored under the packet sequence number:

```typescript
function packetCommitmentPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "commitments/ports/{portIdentifier}/channels/{channelIdentifier}/sequences/{sequence}"
}
```

Absence of the path in the store is equivalent to a zero-bit.

Packet receipt data are stored under the `packetReceiptPath`. In the case of a successful receive, the destination chain writes a sentinel success value of `SUCCESSFUL_RECEIPT`.
Some channel types MAY write a sentinel timeout value `TIMEOUT_RECEIPT` if the packet is received after the specified timeout.

```typescript
function packetReceiptPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "receipts/ports/{portIdentifier}/channels/{channelIdentifier}/sequences/{sequence}"
}
```

Packet acknowledgement data are stored under the `packetAcknowledgementPath`:

```typescript
function packetAcknowledgementPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "acks/ports/{portIdentifier}/channels/{channelIdentifier}/sequences/{sequence}"
}
```

### Versioning

During the handshake process, two ends of a channel come to agreement on a version bytestring associated
with that channel. The contents of this version bytestring are and will remain opaque to the IBC core protocol.
Host state machines MAY utilise the version data to indicate supported IBC/APP protocols, agree on packet
encoding formats, or negotiate other channel-related metadata related to custom logic on top of IBC.

Host state machines MAY also safely ignore the version data or specify an empty string.

### Sub-protocols

> Note: If the host state machine is utilising object capability authentication (see [ICS 005](../ics-005-port-allocation)), all functions utilising ports take an additional capability parameter.

#### Identifier validation

Channels are stored under a unique `(portIdentifier, channelIdentifier)` prefix.
The validation function `validatePortIdentifier` MAY be provided.

```typescript
type validateChannelIdentifier = (portIdentifier: Identifier, channelIdentifier: Identifier) => boolean
```

If not provided, the default `validateChannelIdentifier` function will always return `true`.

#### Channel lifecycle management

![Channel State Machine](channel-state-machine.png)

| Initiator | Datagram         | Chain acted upon | Prior state (A, B) | Posterior state (A, B) |
| --------- | ---------------- | ---------------- | ------------------ | ---------------------- |
| Actor     | ChanOpenInit     | A                | (none, none)       | (INIT, none)           |
| Relayer   | ChanOpenTry      | B                | (INIT, none)       | (INIT, TRYOPEN)        |
| Relayer   | ChanOpenAck      | A                | (INIT, TRYOPEN)    | (OPEN, TRYOPEN)        |
| Relayer   | ChanOpenConfirm  | B                | (OPEN, TRYOPEN)    | (OPEN, OPEN)           |

| Initiator | Datagram         | Chain acted upon | Prior state (A, B) | Posterior state (A, B) |
| --------- | ---------------- | ---------------- | ------------------ | ---------------------- |
| Actor     | ChanCloseInit    | A                | (OPEN, OPEN)       | (CLOSED, OPEN)         |
| Relayer   | ChanCloseConfirm | B                | (CLOSED, OPEN)     | (CLOSED, CLOSED)       |
| Actor     | ChanCloseFrozen  | A or B           | (OPEN, OPEN)       | (CLOSED, CLOSED)       |

##### Opening handshake

The `chanOpenInit` function is called by a module to initiate a channel opening handshake with a module on another chain. Functions `chanOpenInit` and `chanOpenTry` do no set the new channel end in state because the channel version might be modified by the application callback. A function `writeChannel` should be used to write the channel end in state after executing the application callback:

```typescript
function writeChannel(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  state: ChannelState,
  order: ChannelOrder,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  connectionHops: [Identifier],
  version: string) {
    channel = ChannelEnd{
      state, order,
      counterpartyPortIdentifier, counterpartyChannelIdentifier,
      connectionHops, version
    }
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

See handler functions `handleChanOpenInit` and `handleChanOpenTry` in [Channel lifecycle management](../ics-026-routing-module/README.md#channel-lifecycle-management) for more details.

The opening channel must provide the identifiers of the local channel identifier, local port, remote port, and remote channel identifier.

When the opening handshake is complete, the module which initiates the handshake will own the end of the created channel on the host ledger, and the counterparty module which
it specifies will own the other end of the created channel on the counterparty chain. Once a channel is created, ownership cannot be changed (although higher-level abstractions
could be implemented to provide this).

Chains MUST implement a function `generateIdentifier` which chooses an identifier, e.g. by incrementing a counter:

```typescript
type generateIdentifier = () -> Identifier
```

```typescript
function chanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier): (channelIdentifier: Identifier, channelCapability: CapabilityKey) {
    channelIdentifier = generateIdentifier()
    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))

    abortTransactionUnless(provableStore.get(channelPath(portIdentifier, channelIdentifier)) === null)
    connection = provableStore.get(connectionPath(connectionHops[0]))

    // optimistic channel handshakes are allowed
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(authenticateCapability(portPath(portIdentifier), portCapability))

    channelCapability = newCapability(channelCapabilityPath(portIdentifier, channelIdentifier))
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceAckPath(portIdentifier, channelIdentifier), 1)

    return channelIdentifier, channelCapability
}
```

The `chanOpenTry` function is called by a module to accept the first step of a channel opening handshake initiated by a module on another chain.

```typescript
function chanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string,
  proofInit: CommitmentProof | MultihopProof,
  proofHeight: Height): (channelIdentifier: Identifier, channelCapability: CapabilityKey) {
    channelIdentifier = generateIdentifier()

    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))
    abortTransactionUnless(authenticateCapability(portPath(portIdentifier), portCapability))

    connection = provableStore.get(connectionPath(connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // return hops from counterparty's view
    counterpartyHops = getCounterPartyHops(proofInit, connection)

    expected = ChannelEnd{
      INIT, order, portIdentifier,
      "", counterpartyHops,
      counterpartyVersion
    }

    if (connectionHops.length > 1) {
      key = channelPath(counterparty.PortId, counterparty.ChannelId)
      abortTransactionUnless(connection.verifyMultihopMembership(
        connection,
        proofHeight,
        proofInit,
        connectionHops,
        key
        expected))
    } else {
      abortTransactionUnless(connection.verifyChannelState(
        proofHeight,
        proofInit,
        counterpartyPortIdentifier,
        counterpartyChannelIdentifier,
        expected
      ))
    }

    channelCapability = newCapability(channelCapabilityPath(portIdentifier, channelIdentifier))

    // initialize channel sequences
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceAckPath(portIdentifier, channelIdentifier), 1)

    return channelIdentifier, channelCapability
}
```

The `chanOpenAck` is called by the handshake-originating module to acknowledge the acceptance of the initial request by the
counterparty module on the other chain.

```typescript
function chanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string,
  proofTry: CommitmentProof | MultihopProof,
  proofHeight: Height) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state === INIT)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // return hops from counterparty's view
    counterpartyHops = getCounterPartyHops(proofTry, connection)

    expected = ChannelEnd{TRYOPEN, channel.order, portIdentifier,
        channelIdentifier, counterpartyHops, counterpartyVersion}

    if (channel.connectionHops.length > 1) {
      key = channelPath(counterparty.PortId, counterparty.ChannelId)
      abortTransactionUnless(connection.verifyMultihopMembership(
        connection,
        proofHeight,
        proofTry,
        channel.connectionHops,
        key,
        expected))
    } else {
      abortTransactionUnless(connection.verifyChannelState(
        proofHeight,
        proofTry,
        channel.counterpartyPortIdentifier,
        counterpartyChannelIdentifier,
        expected
      ))
    }
    // write will happen in the handler defined in the ICS26 spec
}
```

The `chanOpenConfirm` function is called by the handshake-accepting module to acknowledge the acknowledgement
of the handshake-originating module on the other chain and finish the channel opening handshake.

```typescript
function chanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofAck: CommitmentProof | MultihopProof,
  proofHeight: Height) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === TRYOPEN)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // return hops from counterparty's view
    counterpartyHops = getCounterPartyHops(proofAck, connection)

    expected = ChannelEnd{OPEN, channel.order, portIdentifier,
      channelIdentifier, counterpartyHops, channel.version}

    if (connectionHops.length > 1) {
      key = channelPath(counterparty.PortId, counterparty.ChannelId)
      abortTransactionUnless(connection.verifyMultihopMembership(
        connection,
        proofHeight,
        proofAck,
        channel.connectionHops,
        key
        expected))
    } else {
      abortTransactionUnless(connection.verifyChannelState(
        proofHeight,
        proofAck,
        channel.counterpartyPortIdentifier,
        channel.counterpartyChannelIdentifier,
        expected
      ))
    }

    // write will happen in the handler defined in the ICS26 spec
}
```

##### Closing handshake

The `chanCloseInit` function is called by either module to close their end of the channel. Once closed, channels cannot be reopened.

Calling modules MAY atomically execute appropriate application logic in conjunction with calling `chanCloseInit`.

Any in-flight packets can be timed-out as soon as a channel is closed.

```typescript
function chanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    channel.state = CLOSED
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

The `chanCloseConfirm` function is called by the counterparty module to close their end of the channel,
since the other end has been closed.

Calling modules MAY atomically execute appropriate application logic in conjunction with calling `chanCloseConfirm`.

Once closed, channels cannot be reopened and identifiers cannot be reused. Identifier reuse is prevented because
we want to prevent potential replay of previously sent packets. The replay problem is analogous to using sequence
numbers with signed messages, except where the light client algorithm "signs" the messages (IBC packets), and the replay
prevention sequence is the combination of port identifier, channel identifier, and packet sequence - hence we cannot
allow the same port identifier & channel identifier to be reused again with a sequence reset to zero, since this
might allow packets to be replayed. It would be possible to safely reuse identifiers if timeouts of a particular
maximum height/time were mandated & tracked, and future specification versions may incorporate this feature.

```typescript
function chanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofInit: CommitmentProof | MultihopProof,
  proofHeight: Height) {
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // return hops from counterparty's view
    counterpartyHops = getCounterPartyHops(proofInit, connection)

    expected = ChannelEnd{CLOSED, channel.order, portIdentifier,
                          channelIdentifier, counterpartyHops, channel.version}

    if (connectionHops.length > 1) {
      key = channelPath(counterparty.PortId, counterparty.ChannelId)
      abortTransactionUnless(connection.verifyMultihopMembership(
        connection,
        proofHeight,
        proofInit,
        channel.connectionHops,
        key
        expected))
    } else {
      abortTransactionUnless(connection.verifyChannelState(
        proofHeight,
        proofInit,
        channel.counterpartyPortIdentifier,
        channel.counterpartyChannelIdentifier,
        expected
      ))
    }

    // write may happen asynchronously in the handler defined in the ICS26 spec
    // if the channel is closing during an upgrade, 
    // then we can delete all auxiliary upgrade information
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(counterpartyUpgradePath(portIdentifier, channelIdentifier))

    channel.state = CLOSED
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

The `chanCloseFrozen` function is called by a relayer to force close a multi-hop channel if any client state in the
channel path is frozen. A relayer should send proof of the frozen client state to each end of the channel with a
proof of the frozen client state in the channel path starting from each channel end up until the first frozen client.
The multi-hop proof for each channel end will be different and consist of a proof formed starting from each channel
end up to the frozen client.

The multi-hop proof starts with a chain with a frozen client for the misbehaving chain. However, the frozen client exists
on the next blockchain in the channel path so the key/value proof is indexed to evaluate on the consensus state holding
that client state. The client state path requires knowledge of the client id which can be determined from the
connectionEnd on the misbehaving chain prior to the misbehavior submission.

Once frozen, it is possible for a channel to be unfrozen (reactivated) via governance processes once the misbehavior in
the channel path has been resolved. However, this process is out-of-protocol.

Example:

Given a multi-hop channel path over connections from chain `A` to chain `E` and misbehaving chain `C`

`A <--> B <--x C x--> D <--> E`

Assume any relayer submits evidence of misbehavior to chain `B` and chain `D` to freeze their respective clients for chain `C`.

A relayer may then provide a multi-hop proof of the frozen client on chain `B` to chain `A` to close the channel on chain `A`, and another relayer (or the same one) may also relay a multi-hop proof of the frozen client on chain `D` to chain `E` to close the channel end on chain `E`.

However, it must also be proven that the frozen client state corresponds to a specific hop in the channel path.

Therefore, a proof of the connection end on chain `B` with counterparty connection end on chain `C` must also be provided along with the client state proof to prove that the `clientID` for the client state matches the `clientID` in the connection end. Furthermore, the `connectionID` for the connection end MUST match the expected ID from the channel's `connectionHops` field.

```typescript
function chanCloseFrozen(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofConnection: MultihopProof,
  proofClientState: MultihopProof,
  proofHeight: Height) {
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    hopsLength = channel.connectionHops.length
    abortTransactionUnless(hopsLength === 1)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // lookup connectionID for connectionEnd corresponding to misbehaving chain
    let connectionIdx = proofConnection.ConnectionProofs.length + 1
    abortTransactionUnless(connectionIdx < hopsLength)
    let connectionID = channel.ConnectionHops[connectionIdx]
    let connectionProofKey = connectionPath(connectionID)
    let connectionProofValue = proofConnection.KeyProof.Value
    let frozenConnectionEnd = abortTransactionUnless(Unmarshal(connectionProofValue))

    // the clientID in the connection end must match the clientID for the frozen client state
    let clientID = frozenConnectionEnd.ClientId

    // truncated connectionHops. e.g. client D on chain C is frozen: A, B, C, D, E -> A, B, C
    let connectionHops = channel.ConnectionHops[:len(mProof.ConnectionProofs)+1]

    // verify the connection proof
    abortTransactionUnless(connection.verifyMultihopMembership(
      connection,
      proofHeight,
      proofConnection,
      connectionHops,
      connectionProofKey,
      connectionProofValue))


    // key and value for the frozen client state
    let clientStateKey = clientStatePath(clientID)
    let clientStateValue = proofClientState.KeyProof.Value
    let frozenClientState = abortTransactionUnless(Unmarshal(clientStateValue))

    // ensure client state is frozen by checking FrozenHeight
    abortTransactionUnless(frozenClientState.FrozenHeight.RevisionHeight !== 0)

   // verify the frozen client state proof
    abortTransactionUnless(connection.verifyMultihopMembership(
      connection,
      proofHeight,
      proofConnection,
      connectionHops,
      clientStateKey,
      clientStateValue))

    channel.state = FROZEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

##### Multihop utility functions

```typescript
// Return the counterparty connectionHops
function getCounterPartyHops(proof: CommitmentProof | MultihopProof, lastConnection: ConnectionEnd) string[] {

  let counterpartyHops: string[] = [lastConnection.counterpartyConnectionIdentifier]
  if typeof(proof) === 'MultihopProof' {
    for connData in proofs.ConnectionProofs {
      connectionEnd = abortTransactionUnless(Unmarshal(connData.Value))
      counterpartyHops.push(connectionEnd.GetCounterparty().GetConnectionID())
    }

    // reverse the hops so they are ordered from sender --> receiver
    counterpartyHops = counterpartyHops.reverse()
  }

  return counterpartyHops
}
```

#### Packet flow & handling

![Packet State Machine](packet-state-machine.png)

##### A day in the life of a packet

The following sequence of steps must occur for a packet to be sent from module *1* on machine *A* to module *2* on machine *B*, starting from scratch.

The module can interface with the IBC handler through [ICS 25](../ics-025-handler-interface) or [ICS 26](../ics-026-routing-module).

1. Initial client & port setup, in any order
    1. Client created on *A* for *B* (see [ICS 2](../ics-002-client-semantics))
    1. Client created on *B* for *A* (see [ICS 2](../ics-002-client-semantics))
    1. Module *1* binds to a port (see [ICS 5](../ics-005-port-allocation))
    1. Module *2* binds to a port (see [ICS 5](../ics-005-port-allocation)), which is communicated out-of-band to module *1*
1. Establishment of a connection & channel, optimistic send, in order
    1. Connection opening handshake started from *A* to *B* by module *1* (see [ICS 3](../ics-003-connection-semantics))
    1. Channel opening handshake started from *1* to *2* using the newly created connection (this ICS)
    1. Packet sent over the newly created channel from *1* to *2* (this ICS)
1. Successful completion of handshakes (if either handshake fails, the connection/channel can be closed & the packet timed-out)
    1. Connection opening handshake completes successfully (see [ICS 3](../ics-003-connection-semantics)) (this will require participation of a relayer process)
    1. Channel opening handshake completes successfully (this ICS) (this will require participation of a relayer process)
1. Packet confirmation on machine *B*, module *2* (or packet timeout if the timeout height has passed) (this will require participation of a relayer process)
1. Acknowledgement (possibly) relayed back from module *2* on machine *B* to module *1* on machine *A*

Represented spatially, packet transit between two machines can be rendered as follows:

![Packet Transit](packet-transit.png)

##### Sending packets

The `sendPacket` function is called by a module in order to send *data* (in the form of an IBC packet) on a channel end owned by the calling module.

Calling modules MUST execute application logic atomically in conjunction with calling `sendPacket`.

The IBC handler performs the following steps in order:

- Checks that the channel is not closed to send packets
- Checks that the calling module owns the sending port (see [ICS 5](../ics-005-port-allocation))
- Checks that the timeout height specified has not already passed on the destination chain
- Increments the send sequence counter associated with the channel
- Stores a constant-size commitment to the packet data & packet timeout
- Returns the sequence number of the sent packet

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment to the data & timeout value. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

```typescript
function sendPacket(
  capability: CapabilityKey,
  sourcePort: Identifier,
  sourceChannel: Identifier,
  timeoutHeight: Height,
  timeoutTimestamp: uint64,
  data: bytes): uint64 {
    channel = provableStore.get(channelPath(sourcePort, sourceChannel))

    // check that the channel must be OPEN to send packets;
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)

    // check if the calling module owns the sending port
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(sourcePort, sourceChannel), capability))

    // disallow packets with a zero timeoutHeight and timeoutTimestamp
    abortTransactionUnless(timeoutHeight !== 0 || timeoutTimestamp !== 0)

    // check that the timeout height hasn't already passed in the local client tracking the receiving chain
    latestClientHeight = provableStore.get(clientPath(connection.clientIdentifier)).latestClientHeight()
    abortTransactionUnless(timeoutHeight === 0 || latestClientHeight < timeoutHeight)

    // increment the send sequence counter
    sequence = provableStore.get(nextSequenceSendPath(sourcePort, sourceChannel))
    provableStore.set(nextSequenceSendPath(sourcePort, sourceChannel), sequence+1)

    // store commitment to the packet data & packet timeout
    provableStore.set(
      packetCommitmentPath(sourcePort, sourceChannel, sequence),
      hash(hash(data), timeoutHeight, timeoutTimestamp)
    )

    // log that a packet can be safely sent
    emitLogEntry("sendPacket", {
      sequence: sequence,
      data: data,
      timeoutHeight: timeoutHeight,
      timeoutTimestamp: timeoutTimestamp
    })

    return sequence
}
```

#### Receiving packets

The `recvPacket` function is called by a module in order to receive an IBC packet sent on the corresponding channel end on the counterparty chain.

Atomically in conjunction with calling `recvPacket`, calling modules MUST either execute application logic or queue the packet for future execution.

The IBC handler performs the following steps in order:

- Checks that the channel & connection are open to receive packets
- Checks that the calling module owns the receiving port
- Checks that the packet metadata matches the channel & connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive (for ordered and ordered_allow_timeout channels)
- Checks that the timeout height and timestamp have not yet passed
- Checks the inclusion proof of packet data commitment in the outgoing chain's state
- Optionally (in case channel upgrades and deletion of acknowledgements and packet receipts are implemented): reject any packet with a sequence already used before a successful channel upgrade
- Sets a store path to indicate that the packet has been received (unordered channels only)
- Increments the packet receive sequence associated with the channel end (ordered and ordered_allow_timeout channels only)

We pass the address of the `relayer` that signed and submitted the packet to enable a module to optionally provide some rewards. This provides a foundation for fee payment, but can be used for other techniques as well (like calculating a leaderboard).

```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof | MultihopProof,
  proofHeight: Height,
  relayer: string): Packet {

    channel = provableStore.get(channelPath(packet.destPort, packet.destChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN || (channel.state === FLUSHING) || (channel.state === FLUSHCOMPLETE))
    counterpartyUpgrade = privateStore.get(counterpartyUpgradePath(packet.destPort, packet.destChannel))
    // defensive check that ensures chain does not process a packet higher than the last packet sent before
    // counterparty went into FLUSHING mode. If the counterparty is implemented correctly, this should never abort
    abortTransactionUnless(counterpartyUpgrade.nextSequenceSend == 0 || packet.sequence < counterpartyUpgrade.nextSequenceSend)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.destPort, packet.destChannel), capability))
    abortTransactionUnless(packet.sourcePort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.sourceChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    if (len(channel.connectionHops) > 1) {
      key = packetCommitmentPath(packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
      abortTransactionUnless(connection.verifyMultihopMembership(
        connection,
        proofHeight,
        proof,
        channel.ConnectionHops,
        key,
        hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)
      ))
    } else {
      abortTransactionUnless(connection.verifyPacketData(
        proofHeight,
        proof,
        packet.sourcePort,
        packet.sourceChannel,
        packet.sequence,
        hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)
      ))
    }

    // do sequence check before any state changes
    if channel.order == ORDERED || channel.order == ORDERED_ALLOW_TIMEOUT {
        nextSequenceRecv = provableStore.get(nextSequenceRecvPath(packet.destPort, packet.destChannel))
        if (packet.sequence < nextSequenceRecv) {
          // event is emitted even if transaction is aborted
          emitLogEntry("recvPacket", {
            data: packet.data
            timeoutHeight: packet.timeoutHeight,
            timeoutTimestamp: packet.timeoutTimestamp,
            sequence: packet.sequence,
            sourcePort: packet.sourcePort,
            sourceChannel: packet.sourceChannel,
            destPort: packet.destPort,
            destChannel: packet.destChannel,
            order: channel.order,
            connection: channel.connectionHops[0]
          })
        }

        abortTransactionUnless(packet.sequence === nextSequenceRecv)
    }

    switch channel.order {
      case ORDERED:
      case UNORDERED:
        abortTransactionUnless(packet.timeoutHeight === 0 || getConsensusHeight() < packet.timeoutHeight)
        abortTransactionUnless(packet.timeoutTimestamp === 0 || currentTimestamp() < packet.timeoutTimestamp)
        break;

      case ORDERED_ALLOW_TIMEOUT:
        // for ORDERED_ALLOW_TIMEOUT, we do not abort on timeout
        // instead increment next sequence recv and write the sentinel timeout value in packet receipt
        // then return
        if (getConsensusHeight() >= packet.timeoutHeight && packet.timeoutHeight != 0) || (currentTimestamp() >= packet.timeoutTimestamp && packet.timeoutTimestamp != 0) {
          nextSequenceRecv = nextSequenceRecv + 1
          provableStore.set(nextSequenceRecvPath(packet.destPort, packet.destChannel), nextSequenceRecv)
          provableStore.set(
            packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence),
            TIMEOUT_RECEIPT
          )
        }
        return;

      default:
        // unsupported channel type
        abortTransactionUnless(false)
    }

    // REPLAY PROTECTION: in order to free storage, implementations may choose to 
    // delete acknowledgements and packet receipts when a channel upgrade is successfully 
    // completed. In that case, implementations must also make sure that any packet with 
    // a sequence already used before the channel upgrade is rejected. This is needed to 
    // prevent replay attacks (see this PR in ibc-go for an example of how this is achieved:
    // https://github.com/cosmos/ibc-go/pull/5651).
    
    // all assertions passed (except sequence check), we can alter state

    switch channel.order {
      case ORDERED:
      case ORDERED_ALLOW_TIMEOUT:
        nextSequenceRecv = nextSequenceRecv + 1
        provableStore.set(nextSequenceRecvPath(packet.destPort, packet.destChannel), nextSequenceRecv)
        break;

      case UNORDERED:
        // for unordered channels we must set the receipt so it can be verified on the other side
        // this receipt does not contain any data, since the packet has not yet been processed
        // it's the sentinel success receipt: []byte{0x01}
        packetReceipt = provableStore.get(packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence))
        if (packetReceipt != null) {
          emitLogEntry("recvPacket", {
            data: packet.data
            timeoutHeight: packet.timeoutHeight,
            timeoutTimestamp: packet.timeoutTimestamp,
            sequence: packet.sequence,
            sourcePort: packet.sourcePort,
            sourceChannel: packet.sourceChannel,
            destPort: packet.destPort,
            destChannel: packet.destChannel,
            order: channel.order,
            connection: channel.connectionHops[0]
          })
        }

        abortTransactionUnless(packetReceipt === null)
        provableStore.set(
          packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence),
          SUCCESSFUL_RECEIPT
        )
      break;
    }

    // log that a packet has been received
    emitLogEntry("recvPacket", {
      data: packet.data
      timeoutHeight: packet.timeoutHeight,
      timeoutTimestamp: packet.timeoutTimestamp,
      sequence: packet.sequence,
      sourcePort: packet.sourcePort,
      sourceChannel: packet.sourceChannel,
      destPort: packet.destPort,
      destChannel: packet.destChannel,
      order: channel.order,
      connection: channel.connectionHops[0]
    })

    // return transparent packet
    return packet
}
```

#### Writing acknowledgements

The `writeAcknowledgement` function is called by a module in order to write data which resulted from processing an IBC packet that the sending chain can then verify, a sort of "execution receipt" or "RPC call response".

Calling modules MUST execute application logic atomically in conjunction with calling `writeAcknowledgement`.

This is an asynchronous acknowledgement, the contents of which do not need to be determined when the packet is received, only when processing is complete. In the synchronous case, `writeAcknowledgement` can be called in the same transaction (atomically) with `recvPacket`.

Acknowledging packets is not required; however, if an ordered channel uses acknowledgements, either all or no packets must be acknowledged (since the acknowledgements are processed in order). Note that if packets are not acknowledged, packet commitments cannot be deleted on the source chain. Future versions of IBC may include ways for modules to specify whether or not they will be acknowledging packets in order to allow for cleanup.

`writeAcknowledgement` *does not* check if the packet being acknowledged was actually received, because this would result in proofs being verified twice for acknowledged packets. This aspect of correctness is the responsibility of the calling module.
The calling module MUST only call `writeAcknowledgement` with a packet previously received from `recvPacket`.

The IBC handler performs the following steps in order:

- Checks that an acknowledgement for this packet has not yet been written
- Sets the opaque acknowledgement value at a store path unique to the packet

```typescript
function writeAcknowledgement(
  packet: Packet,
  acknowledgement: bytes) {
    // acknowledgement must not be empty
    abortTransactionUnless(len(acknowledgement) !== 0)

    // cannot already have written the acknowledgement
    abortTransactionUnless(provableStore.get(packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence) === null))

    // write the acknowledgement
    provableStore.set(
      packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence),
      hash(acknowledgement)
    )

    // log that a packet has been acknowledged
    emitLogEntry("writeAcknowledgement", {
      sequence: packet.sequence,
      timeoutHeight: packet.timeoutHeight,
      port: packet.destPort,
      channel: packet.destChannel,
      timeoutTimestamp: packet.timeoutTimestamp,
      data: packet.data,
      acknowledgement
    })
}
```

#### Processing acknowledgements

The `acknowledgePacket` function is called by a module to process the acknowledgement of a packet previously sent by
the calling module on a channel to a counterparty module on the counterparty chain.
`acknowledgePacket` also cleans up the packet commitment, which is no longer necessary since the packet has been received and acted upon.

Calling modules MAY atomically execute appropriate application acknowledgement-handling logic in conjunction with calling `acknowledgePacket`.

We pass the `relayer` address just as in [Receiving packets](#receiving-packets) to allow for possible incentivization here as well.

```typescript
function acknowledgePacket(
  packet: OpaquePacket,
  acknowledgement: bytes,
  proof: CommitmentProof | MultihopProof,
  proofHeight: Height,
  relayer: string): Packet {

    // abort transaction unless that channel is open, calling module owns the associated port, and the packet fields match
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN || channel.state === FLUSHING)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // verify we sent the packet and haven't cleared it out yet
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // abort transaction unless correct acknowledgement on counterparty chain
    if (len(channel.connectionHops) > 1) {
      key = packetAcknowledgementPath(packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
      abortTransactionUnless(connection.verifyMultihopMembership(
        connection,
        proofHeight,
        proof,
        channel.ConnectionHops,
        key,
        acknowledgement
      ))
    } else {
      abortTransactionUnless(connection.verifyPacketAcknowledgement(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence,
        acknowledgement
      ))
    }

    // abort transaction unless acknowledgement is processed in order
    if (channel.order === ORDERED || channel.order == ORDERED_ALLOW_TIMEOUT) {
      nextSequenceAck = provableStore.get(nextSequenceAckPath(packet.sourcePort, packet.sourceChannel))
      abortTransactionUnless(packet.sequence === nextSequenceAck)
      nextSequenceAck = nextSequenceAck + 1
      provableStore.set(nextSequenceAckPath(packet.sourcePort, packet.sourceChannel), nextSequenceAck)
    }

    // all assertions passed, we can alter state

    // delete our commitment so we can't "acknowledge" again
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    if channel.state == FLUSHING {
      upgradeTimeout = privateStore.get(counterpartyUpgradeTimeout(portIdentifier, channelIdentifier))
      if upgradeTimeout != nil {
        // counterparty-specified timeout must not have exceeded
        // if it has, then restore the channel and abort upgrade handshake
        if (upgradeTimeout.timeoutHeight != 0 && currentHeight() >= upgradeTimeout.timeoutHeight) ||
            (upgradeTimeout.timeoutTimestamp != 0 && currentTimestamp() >= upgradeTimeout.timeoutTimestamp ) {
                restoreChannel(portIdentifier, channelIdentifier)
        } else if pendingInflightPackets(portIdentifier, channelIdentifier) == nil {
          // if this was the last in-flight packet, then move channel state to FLUSHCOMPLETE
          channel.state = FLUSHCOMPLETE
          publicStore.set(channelPath(portIdentifier, channelIdentifier), channel)
        }
      }
    }

    // return transparent packet
    return packet
}
```

##### Acknowledgement Envelope

The acknowledgement returned from the remote chain is defined as arbitrary bytes in the IBC protocol. This data
may either encode a successful execution or a failure (anything besides a timeout). There is no generic way to
distinguish the two cases, which requires that any client-side packet visualiser understands every app-specific protocol
in order to distinguish the case of successful or failed relay. In order to reduce this issue, we offer an additional
specification for acknowledgement formats, which [SHOULD](https://www.ietf.org/rfc/rfc2119.txt) be used by the
app-specific protocols.

```proto
message Acknowledgement {
  oneof response {
    bytes result = 21;
    string error = 22;
  }
}
```

If an application uses a different format for acknowledgement bytes, it MUST not deserialise to a valid protobuf message
of this format. Note that all packets contain exactly one non-empty field, and it must be result or error.  The field
numbers 21 and 22 were explicitly chosen to avoid accidental conflicts with other protobuf message formats used
for acknowledgements. The first byte of any message with this format will be the non-ASCII values `0xaa` (result)
or `0xb2` (error).

#### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves.

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

##### Sending end

The `timeoutPacket` function is called by a module which originally attempted to send a packet to a counterparty module,
where the timeout height or timeout timestamp has passed on the counterparty chain without the packet being committed, to prove that the packet
can no longer be executed and to allow the calling module to safely perform appropriate state transitions.

Calling modules MAY atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutPacket`.

In the case of an ordered channel, `timeoutPacket` checks the `recvSequence` of the receiving channel end and closes the channel if a packet has timed out.

In the case of an unordered channel, `timeoutPacket` checks the absence of the receipt key (which will have been written if the packet was received). Unordered channels are expected to continue in the face of timed-out packets.

If relations are enforced between timeout heights of subsequent packets, safe bulk timeouts of all packets prior to a timed-out packet can be performed. This specification omits details for now.

Since we allow optimistic sending of packets (i.e. sending a packet before a channel opens), we must also allow optimistic timing out of packets. With optimistic sends, the packet may be sent on a channel that eventually opens or a channel that will never open. If the channel does open after the packet has timed out, then the packet will never be received on the counterparty so we can safely timeout optimistically. If the channel never opens, then we MUST timeout optimistically so that any state changes made during the optimistic send by the application can be safely reverted.

We pass the `relayer` address just as in [Receiving packets](#receiving-packets) to allow for possible incentivization here as well.

```typescript
function timeoutPacket(
  packet: OpaquePacket,
  proof: CommitmentProof | MultihopProof,
  proofHeight: Height,
  nextSequenceRecv: Maybe<uint64>,
  relayer: string): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)

    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)

    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // get the timestamp from the final consensus state in the channel path
    var proofTimestamp
    if (channel.connectionHops.length > 1) {
      consensusState = abortTransactionUnless(Unmarshal(proof.ConsensusProofs[proof.ConsensusProofs.length-1].Value))
      proofTimestamp = consensusState.GetTimestamp()
    } else {
      proofTimestamp, err = connection.getTimestampAtHeight(connection, proofHeight)
    }

    // check that timeout height or timeout timestamp has passed on the other end
    abortTransactionUnless(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && proofTimestamp >= packet.timeoutTimestamp))

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    switch channel.order {
      case ORDERED:
        // ordered channel: check that packet has not been received
        // only allow timeout on next sequence so all sequences before the timed out packet are processed (received/timed out)
        // before this packet times out
        abortTransactionUnless(packet.sequence == nextSequenceRecv)
        // ordered channel: check that the recv sequence is as claimed
        if (channel.connectionHops.length > 1) {
          key = nextSequenceRecvPath(packet.srcPort, packet.srcChannel)
          abortTransactionUnless(connection.verifyMultihopMembership(
              connection,
              proofHeight,
              proof,
              channel.ConnectionHops,
              key,
              nextSequenceRecv
          ))
        } else {
            abortTransactionUnless(connection.verifyNextSequenceRecv(
              proofHeight,
              proof,
              packet.destPort,
              packet.destChannel,
              nextSequenceRecv
          ))
        }
        break;

      case UNORDERED:
        if (channel.connectionHops.length > 1) {
          key = packetReceiptPath(packet.srcPort, packet.srcChannel, packet.sequence)
          abortTransactionUnless(connection.verifyMultihopNonMembership(
            connection,
            proofHeight,
            proof,
            channel.ConnectionHops,
            key
          ))
        } else {
          // unordered channel: verify absence of receipt at packet index
          abortTransactionUnless(connection.verifyPacketReceiptAbsence(
            proofHeight,
            proof,
            packet.destPort,
            packet.destChannel,
            packet.sequence
          ))
        }
        break;

      // NOTE: For ORDERED_ALLOW_TIMEOUT, the relayer must first attempt the receive on the destination chain
      // before the timeout receipt can be written and subsequently proven on the sender chain in timeoutPacket
      case ORDERED_ALLOW_TIMEOUT:
        abortTransactionUnless(packet.sequence == nextSequenceRecv - 1)

        if (channel.connectionHops.length > 1) {
          abortTransactionUnless(connection.verifyMultihopMembership(
              connection,
              proofHeight,
              proof,
              channel.ConnectionHops,
              packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence),
              TIMEOUT_RECEIPT
          ))
        } else {
          abortTransactionUnless(connection.verifyPacketReceipt(
            proofHeight,
            proof,
            packet.destPort,
            packet.destChannel,
            packet.sequence
            TIMEOUT_RECEIPT,
          ))
        }
        break;

      default:
        // unsupported channel type
        abortTransactionUnless(true)
    }

    // all assertions passed, we can alter state

    // delete our commitment
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    if channel.state == FLUSHING {
      upgradeTimeout = privateStore.get(counterpartyUpgradeTimeout(portIdentifier, channelIdentifier))
      if upgradeTimeout != nil {
        // counterparty-specified timeout must not have exceeded
        // if it has, then restore the channel and abort upgrade handshake
        if (upgradeTimeout.timeoutHeight != 0 && currentHeight() >= upgradeTimeout.timeoutHeight) ||
            (upgradeTimeout.timeoutTimestamp != 0 && currentTimestamp() >= upgradeTimeout.timeoutTimestamp ) {
                restoreChannel(portIdentifier, channelIdentifier)
        } else if pendingInflightPackets(portIdentifier, channelIdentifier) == nil {
          // if this was the last in-flight packet, then move channel state to FLUSHCOMPLETE
          channel.state = FLUSHCOMPLETE
          publicStore.set(channelPath(portIdentifier, channelIdentifier), channel)
        }
      }
    }

    // only close on strictly ORDERED channels
    if channel.order === ORDERED {
      // if the channel is ORDERED and a packet is timed out in FLUSHING state then
      // all upgrade information is deleted and the channel is set to CLOSED.

      if channel.State == FLUSHING {
        // delete auxiliary upgrade state
        provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
        privateStore.delete(counterpartyUpgradePath(portIdentifier, channelIdentifier))
      }

      // ordered channel: close the channel
      channel.state = CLOSED
      provableStore.set(channelPath(packet.sourcePort, packet.sourceChannel), channel)
    }
    // on ORDERED_ALLOW_TIMEOUT, increment NextSequenceAck so that next packet can be acknowledged after this packet timed out.
    if channel.order === ORDERED_ALLOW_TIMEOUT {
      nextSequenceAck = nextSequenceAck + 1
      provableStore.set(nextSequenceAckPath(packet.srcPort, packet.srcChannel), nextSequenceAck)
    }

    // return transparent packet
    return packet
}
```

##### Timing-out on close

The `timeoutOnClose` function is called by a module in order to prove that the channel
to which an unreceived packet was addressed has been closed, so the packet will never be received
(even if the `timeoutHeight` or `timeoutTimestamp` has not yet been reached).

Calling modules MAY atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutOnClose`.

We pass the `relayer` address just as in [Receiving packets](#receiving-packets) to allow for possible incentivization here as well.

```typescript
function timeoutOnClose(
  packet: Packet,
  proof: CommitmentProof | MultihopProof,
  proofClosed: CommitmentProof | MultihopProof,
  proofHeight: Height,
  nextSequenceRecv: Maybe<uint64>,
  relayer: string): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    // note: the channel may have been closed
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // return hops from counterparty's view
    counterpartyHops = getCounterpartyHops(proof, connection)

    // check that the opposing channel end has closed
    expected = ChannelEnd{CLOSED, channel.order, channel.portIdentifier,
                          channel.channelIdentifier, counterpartyHops, channel.version}

    // verify channel is closed
    if (channel.connectionHops.length > 1) {
      key = channelPath(counterparty.PortId, counterparty.ChannelId)
      abortTransactionUnless(connection.VerifyMultihopMembership(
        connection,
        proofHeight,
        proofClosed,
        channel.ConnectionHops,
        key,
        expected
      ))
    } else {
      abortTransactionUnless(connection.verifyChannelState(
        proofHeight,
        proofClosed,
        channel.counterpartyPortIdentifier,
        channel.counterpartyChannelIdentifier,
        expected
      ))
    }

    switch channel.order {
      case ORDERED:
        // ordered channel: check that packet has not been received
        abortTransactionUnless(packet.sequence >= nextSequenceRecv)

        // ordered channel: check that the recv sequence is as claimed
        if (channel.connectionHops.length > 1) {
          key = nextSequenceRecvPath(packet.destPort, packet.destChannel)
          abortTransactionUnless(connection.verifyMultihopMembership(
            connection,
            proofHeight,
            proof,
            channel.ConnectionHops,
            key,
            nextSequenceRecv
          ))
        } else {
          abortTransactionUnless(connection.verifyNextSequenceRecv(
            proofHeight,
            proof,
            packet.destPort,
            packet.destChannel,
            nextSequenceRecv
          ))
        }
        break;

      case UNORDERED:
        // unordered channel: verify absence of receipt at packet index
        if (channel.connectionHops.length > 1) {
          abortTransactionUnless(connection.verifyMultihopNonMembership(
            connection,
            proofHeight,
            proof,
            channel.ConnectionHops,
            key
          ))
        } else {
          abortTransactionUnless(connection.verifyPacketReceiptAbsence(
            proofHeight,
            proof,
            packet.destPort,
            packet.destChannel,
            packet.sequence
          ))
        }
        break;

      case ORDERED_ALLOW_TIMEOUT:
        // if packet.sequence >= nextSequenceRecv, then the relayer has not attempted
        // to receive the packet on the destination chain (e.g. because the channel is already closed).
        // In this situation it is not needed to verify the presence of a timeout receipt.
        // Otherwise, if packet.sequence < nextSequenceRecv, then the relayer has attempted
        // to receive the packet on the destination chain, and nextSequenceRecv has been incremented.
        // In this situation, verify the presence of timeout receipt. 
        if packet.sequence < nextSequenceRecv {
          abortTransactionUnless(connection.verifyPacketReceipt(
            proofHeight,
            proof,
            packet.destPort,
            packet.destChannel,
            packet.sequence
            TIMEOUT_RECEIPT,
          ))
        }
        break;

      default:
        // unsupported channel type
        abortTransactionUnless(true)
    }

    // all assertions passed, we can alter state

    // delete our commitment
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

##### Cleaning up state

Packets must be acknowledged in order to be cleaned-up.

#### Reasoning about race conditions

##### Simultaneous handshake attempts

If two machines simultaneously initiate channel opening handshakes with each other, attempting to use the same identifiers, both will fail and new identifiers must be used.

##### Identifier allocation

There is an unavoidable race condition on identifier allocation on the destination chain. Modules would be well-advised to utilise pseudo-random, non-valuable identifiers. Managing to claim the identifier that another module wishes to use, however, while annoying, cannot man-in-the-middle a handshake since the receiving module must already own the port to which the handshake was targeted.

##### Timeouts / packet confirmation

There is no race condition between a packet timeout and packet confirmation, as the packet will either have passed the timeout height prior to receipt or not.

##### Man-in-the-middle attacks during handshakes

Verification of cross-chain state prevents man-in-the-middle attacks for both connection handshakes & channel handshakes since all information (source, destination client, channel, etc.) is known by the module which starts the handshake and confirmed prior to handshake completion.

##### Connection / channel closure with in-flight packets

If a connection or channel is closed while packets are in-flight, the packets can no longer be received on the destination chain and can be timed-out on the source chain.

#### Querying channels

Channels can be queried with `queryChannel`:

```typescript
function queryChannel(connId: Identifier, chanId: Identifier): ChannelEnd | void {
    return provableStore.get(channelPath(connId, chanId))
}
```

### Properties & Invariants

- The unique combinations of channel & port identifiers are first-come-first-serve: once a pair has been allocated, only the modules owning the ports in question can send or receive on that channel.
- Packets are delivered exactly once, assuming that the chains are live within the timeout window, and in case of timeout can be timed-out exactly once on the sending chain.
- The channel handshake cannot be man-in-the-middle attacked by another module on either blockchain or another blockchain's IBC handler.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Data structures & encoding can be versioned at the connection or channel level. Channel logic is completely agnostic to packet data formats, which can be changed by the modules any way they like at any time.

## Example Implementations

- Implementation of ICS 04 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).
- Implementation of ICS 04 in Rust can be found in [ibc-rs repository](https://github.com/cosmos/ibc-rs).

## History

Jun 5, 2019 - Draft submitted

Jul 4, 2019 - Modifications for unordered channels & acknowledgements

Jul 16, 2019 - Alterations for multi-hop routing future compatibility

Jul 29, 2019 - Revisions to handle timeouts after connection closure

Aug 13, 2019 - Various edits

Aug 25, 2019 - Cleanup

Jan 10, 2022 - Add ORDERED_ALLOW_TIMEOUT channel type and appropriate logic

Mar 28, 2023 - Add `writeChannel` function to write channel end after executing application callback

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
