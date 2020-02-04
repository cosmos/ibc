---
ics: 4
title: Channel & Packet Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 3, 5, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## Synopsis

The "channel" abstraction provides message delivery semantics to the interblockchain communication protocol, in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one chain and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination chain. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortising the cost of header verification across all the channels utilising a connection & light client.

Channels are payload-agnostic. The modules which send and receive IBC packets decide how to construct packet data and how to act upon the incoming packet data, and must utilise their own application logic to determine which state transactions to apply according to what data the packet contains.

### Motivation

The interblockchain communication protocol uses a cross-chain message passing model. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are visible to relayers and can be read from a blockchain by any relayer process and submitted to any other blockchain.

> The IBC protocol must provide ordering (for ordered channels) and exactly-once delivery guarantees to allow applications to reason about the combined state of connected modules on two chains. For example, an application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

In order to provide the desired ordering, exactly-once delivery, and module permissioning semantics to the application layer, the interblockchain communication protocol must implement an abstraction to enforce these semantics — channels are this abstraction.

### Definitions

`ConsensusState` is as defined in [ICS 2](../ics-002-client-semantics).

`Connection` is as defined in [ICS 3](../ics-003-connection-semantics).

`Port` and `authenticate` are as defined in [ICS 5](../ics-005-port-allocation).

`hash` is a generic collision-resistant hash function, the specifics of which must be agreed on by the modules utilising the channel. `hash` can be defined differently by different chains.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

A *channel* is a pipeline for exactly-once packet delivery between specific modules on separate blockchains, which has at least one end capable of sending packets and one end capable of receiving packets.

A *bidirectional* channel is a channel where packets can flow in both directions: from `A` to `B` and from `B` to `A`.

A *unidirectional* channel is a channel where packets can only flow in one direction: from `A` to `B` (or from `B` to `A`, the order of naming is arbitrary).

An *ordered* channel is a channel where packets are delivered exactly in the order which they were sent.

An *unordered* channel is a channel where packets can be delivered in any order, which may differ from the order in which they were sent.

```typescript
enum ChannelOrder {
  ORDERED,
  UNORDERED,
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
}
```

- The `state` is the current state of the channel end.
- The `ordering` field indicates whether the channel is ordered or unordered.
- The `counterpartyPortIdentifier` identifies the port on the counterparty chain which owns the other end of the channel.
- The `counterpartyChannelIdentifier` identifies the channel end on the counterparty chain.
- The `nextSequenceSend`, stored separately, tracks the sequence number for the next packet to be sent.
- The `nextSequenceRecv`, stored separately, tracks the sequence number for the next packet to be received.
- The `connectionHops` stores the list of connection identifiers, in order, along which packets sent on this channel will travel. At the moment this list must be of length 1. In the future multi-hop channels may be supported.
- The `version` string stores an opaque channel version, which is agreed upon during the handshake. This can determine module-level configuration such as which packet encoding is used for the channel. This version is not used by the core IBC protocol.

Channel ends have a *state*:

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  CLOSED,
}
```

- A channel end in `INIT` state has just started the opening handshake.
- A channel end in `TRYOPEN` state has acknowledged the handshake step on the counterparty chain.
- A channel end in `OPEN` state has completed the handshake and is ready to send and receive packets.
- A channel end in `CLOSED` state has been closed and can no longer be used to send or receive packets.

A `Packet`, in the interblockchain communication protocol, is a particular interface defined as follows:

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: uint64
  sourcePort: Identifier
  sourceChannel: Identifier
  destPort: Identifier
  destChannel: Identifier
  data: bytes
}
```

- The `sequence` number corresponds to the order of sends and receives, where a packet with an earlier sequence number must be sent and received before a packet with a later sequence number.
- The `timeoutHeight` indicates a consensus height on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
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

### Desired Properties

#### Efficiency

- The speed of packet transmission and confirmation should be limited only by the speed of the underlying chains.
  Proofs should be batchable where possible.

#### Exactly-once delivery

- IBC packets sent on one end of a channel should be delivered exactly once to the other end.
- No network synchrony assumptions should be required for exactly-once safety.
  If one or both of the chains halt, packets may be delivered no more than once, and once the chains resume packets should be able to flow again.

#### Ordering

- On ordered channels, packets should be sent and received in the same order: if packet *x* is sent before packet *y* by a channel end on chain `A`, packet *x* must be received before packet *y* by the corresponding channel end on chain `B`.
- On unordered channels, packets may be sent and received in any order. Unordered packets, like ordered packets, have individual timeouts specified in terms of the destination chain's height.

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
    return "ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

The capability key associated with a channel is stored under the `channelCapabilityPath`:

```typescript
function channelCapabilityPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
  return "{channelPath(portIdentifier, channelIdentifier)}/key"
}
```

The `nextSequenceSend` and `nextSequenceRecv` unsigned integer counters are stored separately so they can be proved individually:

```typescript
function nextSequenceSendPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceSend"
}

function nextSequenceRecvPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceRecv"
}
```

Constant-size commitments to packet data fields are stored under the packet sequence number:

```typescript
function packetCommitmentPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/packets/" + sequence
}
```

Absence of the path in the store is equivalent to a zero-bit.

Packet acknowledgement data are stored under the `packetAcknowledgementPath`:

```typescript
function packetAcknowledgementPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/acknowledgements/" + sequence
}
```

Unordered channels MUST always write a acknowledgement (even an empty one) to this path so that the absence of such can be used as proof-of-timeout. Ordered channels MAY write an acknowledgement, but are not required to.

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

##### Opening handshake

The `chanOpenInit` function is called by a module to initiate a channel opening handshake with a module on another chain.

The opening channel must provide the identifiers of the local channel identifier, local port, remote port, and remote channel identifier.

When the opening handshake is complete, the module which initiates the handshake will own the end of the created channel on the host ledger, and the counterparty module which
it specifies will own the other end of the created channel on the counterparty chain. Once a channel is created, ownership cannot be changed (although higher-level abstractions
could be implemented to provide this).

```typescript
function chanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string): CapabilityKey {
    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))

    abortTransactionUnless(connectionHops.length === 1) // for v1 of the IBC protocol

    abortTransactionUnless(provableStore.get(channelPath(portIdentifier, channelIdentifier)) === null)
    connection = provableStore.get(connectionPath(connectionHops[0]))

    // optimistic channel handshakes are allowed
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state !== CLOSED)
    abortTransactionUnless(authenticate(privateStore.get(portPath(portIdentifier))))
    channel = ChannelEnd{INIT, order, counterpartyPortIdentifier,
                         counterpartyChannelIdentifier, connectionHops, version}
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    key = generate()
    provableStore.set(channelCapabilityPath(portIdentifier, channelIdentifier), key)
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    return key
}
```

The `chanOpenTry` function is called by a module to accept the first step of a channel opening handshake initiated by a module on another chain.

```typescript
function chanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string,
  proofInit: CommitmentProof,
  proofHeight: uint64): CapabilityKey {
    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))
    abortTransactionUnless(connectionHops.length === 1) // for v1 of the IBC protocol
    previous = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(
      (previous === null) ||
      (previous.state === INIT &&
       previous.order === order &&
       previous.counterpartyPortIdentifier === counterpartyPortIdentifier &&
       previous.counterpartyChannelIdentifier === counterpartyChannelIdentifier &&
       previous.connectionHops === connectionHops &&
       previous.version === version)
      )
    abortTransactionUnless(authenticate(privateStore.get(portPath(portIdentifier))))
    connection = provableStore.get(connectionPath(connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{INIT, order, portIdentifier,
                          channelIdentifier, connectionHops.reverse(), counterpartyVersion}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofInit,
      counterpartyPortIdentifier,
      counterpartyChannelIdentifier,
      expected
    ))
    channel = ChannelEnd{TRYOPEN, order, counterpartyPortIdentifier,
                         counterpartyChannelIdentifier, connectionHops, version}
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    key = generate()
    provableStore.set(channelCapabilityPath(portIdentifier, channelIdentifier), key)
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    return key
}
```

The `chanOpenAck` is called by the handshake-originating module to acknowledge the acceptance of the initial request by the
counterparty module on the other chain.

```typescript
function chanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string,
  proofTry: CommitmentProof,
  proofHeight: uint64) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state === INIT || channel.state === TRYOPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{TRYOPEN, channel.order, portIdentifier,
                          channelIdentifier, channel.connectionHops.reverse(), counterpartyVersion}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofTry,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))
    channel.state = OPEN
    channel.version = counterpartyVersion
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

The `chanOpenConfirm` function is called by the handshake-accepting module to acknowledge the acknowledgement
of the handshake-originating module on the other chain and finish the channel opening handshake.

```typescript
function chanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === TRYOPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{OPEN, channel.order, portIdentifier,
                          channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofAck,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))
    channel.state = OPEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
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
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
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

Once closed, channels cannot be reopened.

```typescript
function chanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofInit: CommitmentProof,
  proofHeight: uint64) {
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{CLOSED, channel.order, portIdentifier,
                          channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofInit,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))
    channel.state = CLOSED
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
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

The `sendPacket` function is called by a module in order to send an IBC packet on a channel end owned by the calling module to the corresponding module on the counterparty chain.

Calling modules MUST execute application logic atomically in conjunction with calling `sendPacket`.

The IBC handler performs the following steps in order:

- Checks that the channel & connection are open to send packets
- Checks that the calling module owns the sending port
- Checks that the packet metadata matches the channel & connection information
- Checks that the timeout height specified has not already passed on the destination chain
- Increments the send sequence counter associated with the channel
- Stores a constant-size commitment to the packet data & packet timeout

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment to the data & timeout value. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

```typescript
function sendPacket(packet: Packet) {
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))

    // optimistic sends are permitted once the handshake has started
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))

    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state !== CLOSED)

    // sanity-check that the timeout height hasn't already passed in our local client tracking the receiving chain
    latestClientHeight = provableStore.get(clientPath(connection.clientIdentifier)).latestClientHeight()
    abortTransactionUnless(latestClientHeight < packet.timeoutHeight)

    nextSequenceSend = provableStore.get(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(packet.sequence === nextSequenceSend)

    // all assertions passed, we can alter state

    nextSequenceSend = nextSequenceSend + 1
    provableStore.set(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel), nextSequenceSend)
    provableStore.set(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence), hash(packet.data, packet.timeout))

    // log that a packet has been sent
    emitLogEntry("sendPacket", {sequence: packet.sequence, data: packet.data, timeout: packet.timeout})
}
```

#### Receiving packets

The `recvPacket` function is called by a module in order to receive & process an IBC packet sent on the corresponding channel end on the counterparty chain.

Calling modules MUST execute application logic atomically in conjunction with calling `recvPacket`, likely beforehand to calculate the acknowledgement value.

The IBC handler performs the following steps in order:

- Checks that the channel & connection are open to receive packets
- Checks that the calling module owns the receiving port
- Checks that the packet metadata matches the channel & connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive (for ordered channels)
- Checks that the timeout height has not yet passed
- Checks the inclusion proof of packet data commitment in the outgoing chain's state
- Sets the opaque acknowledgement value at a store path unique to the packet (if the acknowledgement is non-empty or the channel is unordered)
- Increments the packet receive sequence associated with the channel end (ordered channels only)

```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  acknowledgement: bytes): Packet {

    channel = provableStore.get(channelPath(packet.destPort, packet.destChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.destPort, packet.destChannel))))
    abortTransactionUnless(packet.sourcePort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.sourceChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    abortTransactionUnless(getConsensusHeight() < packet.timeoutHeight)

    abortTransactionUnless(connection.verifyPacketData(
      proofHeight,
      proof,
      packet.sourcePort,
      packet.sourceChannel,
      packet.sequence,
      concat(packet.data, packet.timeout)
    ))

    // all assertions passed (except sequence check), we can alter state

    if (acknowledgement.length > 0 || channel.order === UNORDERED)
      provableStore.set(
        packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence),
        hash(acknowledgement)
      )

    if (channel.order === ORDERED) {
      nextSequenceRecv = provableStore.get(nextSequenceRecvPath(packet.destPort, packet.destChannel))
      abortTransactionUnless(packet.sequence === nextSequenceRecv)
      nextSequenceRecv = nextSequenceRecv + 1
      provableStore.set(nextSequenceRecvPath(packet.destPort, packet.destChannel), nextSequenceRecv)
    }

    // log that a packet has been received & acknowledged
    emitLogEntry("recvPacket", {sequence: packet.sequence, timeout: packet.timeout, data: packet.data, acknowledgement})

    // return transparent packet
    return packet
}
```

#### Acknowledgements

The `acknowledgePacket` function is called by a module to process the acknowledgement of a packet previously sent by
the calling module on a channel to a counterparty module on the counterparty chain.
`acknowledgePacket` also cleans up the packet commitment, which is no longer necessary since the packet has been received and acted upon.

Calling modules MAY atomically execute appropriate application acknowledgement-handling logic in conjunction with calling `acknowledgePacket`.

```typescript
function acknowledgePacket(
  packet: OpaquePacket,
  acknowledgement: bytes,
  proof: CommitmentProof,
  proofHeight: uint64): Packet {

    // abort transaction unless that channel is open, calling module owns the associated port, and the packet fields match
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // verify we sent the packet and haven't cleared it out yet
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeout))

    // abort transaction unless correct acknowledgement on counterparty chain
    abortTransactionUnless(connection.verifyPacketAcknowledgement(
      proofHeight,
      proof,
      packet.destPort,
      packet.destChannel,
      packet.sequence,
      acknowledgement
    ))

    // all assertions passed, we can alter state

    // delete our commitment so we can't "acknowledge" again
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

#### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves.

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

##### Sending end

The `timeoutPacket` function is called by a module which originally attempted to send a packet to a counterparty module,
where the timeout height has passed on the counterparty chain without the packet being committed, to prove that the packet
can no longer be executed and to allow the calling module to safely perform appropriate state transitions.

Calling modules MAY atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutPacket`.

In the case of an ordered channel, `timeoutPacket` checks the `recvSequence` of the receiving channel end and closes the channel if a packet has timed out.

In the case of an unordered channel, `timeoutPacket` checks the absence of an acknowledgement (which will have been written if the packet was received). Unordered channels are expected to continue in the face of timed-out packets.

If relations are enforced between timeout heights of subsequent packets, safe bulk timeouts of all packets prior to a timed-out packet can be performed. This specification omits details for now.

```typescript
function timeoutPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)

    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // check that timeout height has passed on the other end
    abortTransactionUnless(proofHeight >= packet.timeoutHeight)

    // check that packet has not been received
    abortTransactionUnless(nextSequenceRecv < packet.sequence)

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeout))

    if channel.order === ORDERED
      // ordered channel: check that the recv sequence is as claimed
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
    else
      // unordered channel: verify absence of acknowledgement at packet index
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.sourcePort,
        packet.sourceChannel,
        packet.sequence
      ))

    // all assertions passed, we can alter state

    // delete our commitment
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    if channel.order === ORDERED {
      // ordered channel: close the channel
      channel.state = CLOSED
      provableStore.set(channelPath(packet.sourcePort, packet.sourceChannel), channel)
    }

    // return transparent packet
    return packet
}
```

##### Timing-out on close

The `timeoutOnClose` function is called by a module in order to prove that the channel
to which an unreceived packet was addressed has been closed, so the packet will never be received
(even if the `timeoutHeight` has not yet been reached).

```typescript
function timeoutOnClose(
  packet: Packet,
  proof: CommitmentProof,
  proofClosed: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    // note: the channel may have been closed
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeout))

    // check that the opposing channel end has closed
    expected = ChannelEnd{CLOSED, channel.order, channel.portIdentifier,
                          channel.channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofClosed,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))

    if channel.order === ORDERED
      // ordered channel: check that the recv sequence is as claimed
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
    else
      // unordered channel: verify absence of acknowledgement at packet index
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.sourcePort,
        packet.sourceChannel,
        packet.sequence
      ))

    // all assertions passed, we can alter state

    // delete our commitment
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

##### Cleaning up state

`cleanupPacket` is called by a module to remove a received packet commitment from storage. The receiving end must have already processed the packet (whether regularly or past timeout).

In the ordered channel case, `cleanupPacket` cleans-up a packet on an ordered channel by proving that the packet has been received on the other end.

In the unordered channel case, `cleanupPacket` cleans-up a packet on an unordered channel by proving that the associated acknowledgement has been written.

```typescript
function cleanupPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecvOrAcknowledgement: Either<uint64, bytes>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // abortTransactionUnless packet has been received on the other end
    abortTransactionUnless(nextSequenceRecv > packet.sequence)

    // verify we actually sent the packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
               === hash(packet.data, packet.timeout))

    if channel.order === ORDERED
      // check that the recv sequence is as claimed
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecvOrAcknowledgement
      ))
    else
      // abort transaction unless acknowledgement on the other end
      abortTransactionUnless(connection.verifyPacketAcknowledgement(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence,
        nextSequenceRecvOrAcknowledgement
      ))

    // all assertions passed, we can alter state

    // clear the store
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

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

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Jun 5, 2019 - Draft submitted

Jul 4, 2019 - Modifications for unordered channels & acknowledgements

Jul 16, 2019 - Alterations for multi-hop routing future compatibility

Jul 29, 2019 - Revisions to handle timeouts after connection closure

Aug 13, 2019 - Various edits

Aug 25, 2019 - Cleanup

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
