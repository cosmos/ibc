---
ics: 4
title: Channel & Packet Semantics
stage: draft
category: ibc-core
requires: 2, 3, 5, 23, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-06-05
---

## Synopsis

The "channel" abstraction provides message delivery semantics to the interblockchain communication protocol, in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one chain and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination chain. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortizing the cost of header verification across all the channels utilizing a connection & light client.

Channels are payload-agnostic. The modules which send and receive IBC packets decide how to construct packet data and how to act upon the incoming packet data, and must utilize their own application logic to determine which state transactions to apply according to what data the packet contains.

### Motivation

The interblockchain communication protocol uses a cross-chain message passing model which makes no assumptions about network synchrony. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are public and can be read from a blockchain by any relayer and submitted to any other blockchain.

The IBC protocol must provide ordering and exactly-once delivery guarantees in order to allow applications to reason about the combined state of connected modules on two chains. For example, an application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

In order to provide the desired ordering, exactly-once delivery, and module permissioning semantics to the application layer, the interblockchain communication protocol must implement an abstraction to enforce these semantics — channels are this abstraction.

### Definitions

`ConsensusState` is as defined in [ICS 2](../ics-002-consensus-verification).

`Connection` is as defined in [ICS 3](../ics-003-connection-semantics).

`Port` and `authenticate` are as defined in [ICS 5](../ics-005-port-allocation).

`Commitment`, `CommitmentProof`, and `CommitmentRoot` are as defined in [ICS 23](../ics-023-vector-commitments).

`commit` is a generic collision-resistant hash function, the specifics of which must be agreed on by the modules utilizing the channel.

`Identifier`, `get`, `set`, `delete`, `getConsensusState`, and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

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

An *end* of a channel is a data structure on one chain storing channel metadata:

```typescript
interface ChannelEnd {
  state: ChannelEndState
  ordering: ChannelOrder
  counterpartyChannelIdentifier: Identifier
  portIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

- The `state` is the current state of the channel end.
- The `counterpartyChannelIdentifier` identifies the channel end on the counterparty chain.
- The `portIdentifier` identifies the module which owns this channel end.
- The `counterpartyPortIdentifier` identifies the module on the counterparty chain which owns the other end of the channel.
- The `nextSequenceSend`, stored separately, tracks the sequence number for the next packet to be sent.
- The `nextSequenceRecv`, stored separately, tracks the sequence number for the next packet to be received.
- The `nextTimeoutHeight` stores the timeout height for the next stage of the handshake, used only in channel opening and closing handshakes.

Channel ends have a *state*:

```typescript
enum ChannelEndState {
  INIT,
  OPENTRY,
  OPEN,
  CLOSETRY,
  CLOSED,
}
```

- A channel end in `INIT` state has just started the opening handshake.
- A channel end in `OPENTRY` state has acknowledged the handshake step on the counterparty chain.
- A channel end in `OPEN` state has completed the handshake and is ready to send and receive packets.
- A channel end in `CLOSETRY` state has just started the closing handshake.
- A channel end in `CLOSED` state has been closed and can no longer be used to send or receive packets.

A *packet*, in the interblockchain communication protocol, is a particular datagram, defined as follows:

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: uint64
  sourceConnection: Identifier
  sourceChannel: Identifier
  destConnection: Identifier
  destChannel: Identifier
  data: bytes
}
```

- The `sequence` number corresponds to the order of sends and receives, where a packet with an earlier sequence number must be sent and received before a packet with a later sequence number.
- The `timeoutHeight` indicates a consensus height on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
- The `sourceConnection` identifies the connection end on the sending chain.
- The `sourceChannel` identifies the channel end on the sending chain.
- The `destConnection` identifies the connection end on the receiving chain.
- The `destChannel` identifies the channel end on the receiving chain.
- The `data` is an opaque value which can be defined by the application logic of the associated modules.

### Desired Properties

#### Efficiency

- The speed of packet transmission and confirmation should be limited only by the speed of the underlying chains.
  Proofs should be batcheable where possible.

#### Exactly-once delivery

- IBC packets sent on one end of a channel should be delivered exactly once to the other end.
- No network synchrony assumptions should be required for safety of exactly-once delivery.
  If one or both of the chains should halt, packets should be delivered no more than once, and once the chains resume packets should be able to flow again.

#### Ordering

- Packets should be sent and received in the same order: if packet *x* is sent before packet *y* by a channel end on chain `A`, packet *x* must be received before packet *y* by the corresponding channel end on chain `B`.

#### Permissioning

- Channels should be permissioned to one module on each end, determined during the handshake and immutable afterwards (higher-level logic could tokenize channel ownership).
  Only the module associated with a channel end should be able to send or receive on it.

## Technical Specification

### Dataflow visualization

The architecture of clients, connections, channels and packets:

![dataflow](dataflow.png)

### Preliminaries

#### Store keys

Channel structures are stored under a store key prefix unique to a combination of a connection identifier and channel identifier:

```typescript
function channelKey(connectionIdentifier: Identifier, channelIdentifier: Identifier) {
  return "connections/{connectionIdentifier}/channels/{channelIdentifier}"
}
```

The `nextSequenceSend` and `nextSequenceRecv` unsigned integer counters are stored separately so they can be proved individually:

```typescript
function nextSequenceSendKey(connectionIdentifier: Identifier, channelIdentifier: Identifier) {
  return channelKey(connectionIdentifier, channelIdentifier) + "/nextSequenceSend"
}

function nextSequenceRecvKey(connectionIdentifier: Identifier, channelIdentifier: Identifier) {
  return channelKey(connectionIdentifier, channelIdentifier) + "/nextSequenceRecv"
}
```

Succint commitments to packet data fields are stored under the packet sequence number:

```typescript
function packetCommitmentKey(connectionIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64) {
  return channelKey(connectionIdentifier, channelIdentifier) + "/packets/" + sequence
}
```

Absence of the key in the store is equivalent to a zero-bit.

Packet acknowledgement data are stored under the `packetAcknowledgementKey`:

```typescript
function packetAcknowledgementKey(connectionIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64) {
  return channelKey(connectionIdentifier, channelIdentifier) + "/acknowledgements/" + sequence
}
```

Unordered channels must always write a acknowledgement (even an empty one) to this key so that the absence of such can be used as proof-of-timeout.

### Subprotocols

#### Channel lifecycle management

![channel-state-machine](channel-state-machine.png)

##### Opening handshake

The `chanOpenInit` function is called by a module to initiate a channel opening handshake with a module on another chain.
The opening channel must provide the identifiers of the local channel end, local connection, and desired remote channel end.

When the opening handshake is complete, the module which initiates the handshake will own the end of the created channel on the host ledger, and the counterparty module which
it specifies will own the other end of the created channel on the counterparty chain. Once a channel is created, ownership cannot be changed (although higher-level abstractions
could be implemented to provide this).

```typescript
function chanOpenInit(
  order: ChannelOrder, connectionHops: [Identifier], channelIdentifier: Identifier,
  portIdentifier: Identifier, counterpartyChannelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier, nextTimeoutHeight: uint64) {
  assert(get(channelKey(connectionIdentifier, channelIdentifier)) === nil)
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  assert(authenticate(get(portKey(portIdentifier))))
  channel = Channel{INIT, order, portIdentifier, counterpartyPortIdentifier,
                    counterpartyChannelIdentifier, nextTimeoutHeight}
  set(channelKey(connectionIdentifier, channelIdentifier), channel)
  set(nextSequenceSendKey(connectionIdentifier, channelIdentifier), 0)
  set(nextSequenceRecvKey(connectionIdentifier, channelIdentifier), 0)
}
```

The `chanOpenTry` function is called by a module to accept the first step of a chanel opening handshake initiated by a module on another chain.

```typescript
function chanOpenTry(
  order: ChannelOrder, connectionHops: [Identifier],
  channelIdentifier: Identifier, counterpartyChannelIdentifier: Identifier,
  portIdentifier: Identifier, counterpartyPortIdentifier: Identifier,
  timeoutHeight: uint64, nextTimeoutHeight: uint64,
  proofInit: CommitmentProof, proofHeight: uint64) {
  assert(getConsensusState().height < timeoutHeight)
  assert(get(channelKey(connectionIdentifier, channelIdentifier)) === null)
  assert(authenticate(get(portKey(portIdentifier))))
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(verifyMembership(
    counterpartyStateRoot,
    proofInit,
    channelKey(connection.counterpartyConnectionIdentifier, counterpartyChannelIdentifier),
    Channel{INIT, order, counterpartyPortIdentifier, portIdentifier, channelIdentifier, timeoutHeight}
  ))
  channel = Channel{OPENTRY, order, portIdentifier, counterpartyPortIdentifier,
                    counterpartyChannelIdentifier, nextTimeoutHeight}
  set(channelKey(connectionIdentifier, channelIdentifier), channel)
  set(nextSequenceSendKey(connectionIdentifier, channelIdentifier), 0)
  set(nextSequenceRecvKey(connectionIdentifier, channelIdentifier), 0)
}
```

The `chanOpenAck` is called by the handshake-originating module to acknowledge the acceptance of the initial request by the
counterparty module on the other chain.

```typescript
function chanOpenAck(
  connectionHops: [Identifier], channelIdentifier: Identifier,
  timeoutHeight: uint64, nextTimeoutHeight: uint64,
  proofTry: CommitmentProof, proofHeight: uint64) {
  assert(getConsensusState().height < timeoutHeight)
  channel = get(channelKey(connectionIdentifier, channelIdentifier))
  assert(channel.state === INIT)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(verifyMembership(
    counterpartyStateRoot,
    proofTry,
    channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier),
    Channel{OPENTRY, channel.order, channel.counterpartyPortIdentifier, channel.portIdentifier,
            channelIdentifier, timeoutHeight}
  ))
  channel.state = OPEN
  channel.nextTimeoutHeight = nextTimeoutHeight
  set(channelKey(connectionIdentifier, channelIdentifier), channel)
}
```

The `chanOpenConfirm` function is called by the handshake-accepting module to acknowledge the acknowledgement
of the handshake-originating module on the other chain and finish the channel opening handshake.

```typescript
function chanOpenConfirm(
  connectionHops: [Identifier], channelIdentifier: Identifier,
  timeoutHeight: uint64, proofAck: CommitmentProof, proofHeight: uint64) {
  assert(getConsensusState().height < timeoutHeight)
  channel = get(channelKey(connectionIdentifier, channelIdentifier))
  assert(channel.state === OPENTRY)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(verifyMembership(
    counterpartyStateRoot,
    proofAck,
    channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier),
    Channel{OPEN, channel.order, channel.counterpartyPortIdentifier, channel.portIdentifier,
            channelIdentifier, timeoutHeight}
  ))
  channel.state = OPEN
  channel.nextTimeoutHeight = 0
  set(channelKey(connectionIdentifier, channelIdentifier), channel)
}
```

The `chanOpenTimeout` function is called by either the handshake-originating
module or the handshake-confirming module to prove that a timeout has occurred and reset the state.

```typescript
function chanOpenTimeout(
  connectionHops: [Identifier], channelIdentifier: Identifier,
  timeoutHeight: uint64, proofTimeout: CommitmentProof, proofHeight: uint64) {
  channel = get(channelKey(connectionIdentifier, channelIdentifier))
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight >= connection.nextTimeoutHeight)
  switch channel.state {
    case INIT:
      assert(verifyNonMembership(
        counterpartyStateRoot, proofTimeout,
        channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier)
      ))
    case OPENTRY:
      assert(
        verifyNonMembership(
          counterpartyStateRoot, proofTimeout,
          channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier)
        )
        ||
        verifyMembership(
          counterpartyStateRoot, proofTimeout,
          channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier),
          Channel{INIT, channel.order, channel.counterpartyPortIdentifier, channel.portIdentifier,
                  channelIdentifier, timeoutHeight}
        )
      )
    case OPEN:
      expected = Channel{OPENTRY, channel.order, channel.counterpartyPortIdentifier, channel.portIdentifier,
                         channelIdentifier, timeoutHeight}
      assert(verifyMembership(
        counterpartyStateRoot, proofTimeout,
        channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier),
        expected
      ))
  }
  delete(channelKey(connectionIdentifier, channelIdentifier))
}
```

##### Closing handshake

The `chanClose` function is called by either module to close their end of the channel.

Calling modules MAY atomically execute appropriate application logic in conjunction with calling `chanClose`.

```typescript
function chanClose(
  connectionHops: [Identifier], channelIdentifier: Identifier) {
  channel = get(channelKey(connectionIdentifier, channelIdentifier))
  assert(channel.state === OPEN)
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  channel.state = CLOSED
  set(channelKey(connectionIdentifier, channelIdentifier), channel)
}
```

The `chanCloseConfirm` function is called by the counterparty module to close their end of the channel,
since the other end has been closed.

Calling modules MAY atomically execute appropriate application logic in conjunction with calling `chanCloseConfirm`.

```typescript
function chanCloseConfirm(
  connectionHops: [Identifier], channelIdentifier: Identifier,
  proof: CommitmentProof, proofHeight: uint64) {
  channel = get(channelKey(connectionIdentifier, channelIdentifier))
  assert(channel.state === OPEN)
  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  expected = Channel{CLOSED, channel.order, channel.counterpartyPortIdentifier, channel.portIdentifier,
                     channel.channelIdentifier, 0}
  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    channelKey(connection.counterpartyConnectionIdentifier, channel.counterpartyChannelIdentifier),
    expected
  ))
  channel.state = CLOSED
  set(channelKey(connectionIdentifier, channelIdentifier), channel)
}
```

#### Packet flow & handling

![packet-state-machine](packet-state-machine.png)

##### Sending packets

The `sendPacket` function is called by a module in order to send an IBC packet on a channel end owned by the calling module to the corresponding module the counterparty chain.

Calling modules MUST execute application logic atomically in conjunction with calling `sendPacket`.

The IBC handler performs the following steps in order:
- Checks that the channel & connection are open to send packets
- Checks that the calling module owns the sending port
- Checks that the packet metadata matches the channel & connection information
- Checks that the timeout height specified has not already passed on the destination chain
- Increments the send sequence counter associated with the channel
- Stores a succinct hash commitment to the packet data

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

```typescript
function sendPacket(packet: Packet) {
  channel = get(channelKey(packet.sourceConnection, packet.sourceChannel))
  assert(channel.state === OPEN)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.destChannel === channel.counterpartyChannelIdentifier)
  connection = get(connectionKey(packet.sourceConnection))
  assert(connection.state === OPEN)
  assert(packet.destConnection === connection.counterpartyConnectionIdentifier)
  consensusState = get(consensusStateKey(connection.clientIdentifier))
  assert(consensusState.getHeight() < packet.timeoutHeight)
  nextSequenceSend = get(nextSequenceSendKey(packet.sourceConnection, packet.sourceChannel))
  assert(packet.sequence === nextSequenceSend)
  nextSequenceSend = nextSequenceSend + 1
  set(nextSequenceSendKey(packet.sourceConnection, packet.sourceChannel), nextSequenceSend)
  set(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, sequence), commit(packet.data))
}
```

#### Receiving packets

The `recvPacket` function is called by a module in order to receive & process an IBC packet sent on the corresponding channel end on the counterparty chain.

Calling modules MUST execute application logic atomically in conjunction with calling `recvPacket`, likely beforehand to calculate the acknowledgement value.

The IBC handler performs the following steps in order:
- Checks that the channel & connection are open to receive packets
- Checks that the calling module owns the receiving port
- Checks that the packet metadata matches the channel & connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive
- Checks that the timeout height has not yet passed
- Checks the inclusion proof of packet data commitment in the outgoing chain's state
- Sets the opaque acknowledgement value at a store key unique to the packet (if the acknowledgement is non-empty or the channel is unordered)
- Increments the packet receive sequence associated with the channel end (ordered channels only)

```typescript
function recvPacket(
  packet: Packet, proof: CommitmentProof,
  proofHeight: uint64, acknowledgement: bytes) {

  channel = get(channelKey(packet.destConnection, packet.destChannel))
  assert(channel.state === OPEN)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.sourceChannel === channel.counterpartyChannelIdentifier)
  connection = get(connectionKey(connectionIdentifier))
  assert(packet.sourceConnection === connection.counterpartyConnectionIdentifier)
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight < packet.timeoutHeight)
  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence),
    commit(packet.data)
  ))

  if (acknowledgement.length > 0 || channel.order === UNORDERED)
    set(packetAcknowledgementKey(packet.destConnection, packet.destChannel, packet.sequence), commit(acknowledgement))

  if (channel.order === ORDERED) {
    nextSequenceRecv = get(nextSequenceRecvKey(packet.destConnection, packet.destChannel))
    assert(packet.sequence === nextSequenceRecv)
    nextSequenceRecv = nextSequenceRecv + 1
    set(nextSequenceRecvKey(packet.destConnection, packet.destChannel), nextSequenceRecv)
  }
}
```

#### Acknowledgements

The `acknowledgePacket` function is called by a module to process the acknowledgement of a packet previously sent on a
channel to a counterparty module on the counterparty chain. `acknowledgePacket` also cleans up the packet commitment,
which is no longer necessary since the packet has been received and acted upon.

Calling modules MAY atomically execute appropriate application acknowledgement-handling logic in conjunction with calling `acknowledgePacket`.

```typescript
function acknowledgePacket(
  packet: Packet, proof: CommitmentProof,
  proofHeight: uint64, acknowledgement: bytes) {

  channel = get(channelKey(packet.destConnection, packet.destChannel))
  assert(channel.state === OPEN)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.sourceChannel === channel.counterpartyChannelIdentifier)
  connection = get(connectionKey(connectionIdentifier))
  assert(packet.sourceConnection === connection.counterpartyConnectionIdentifier)
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))

  // verify we sent the packet and haven't cleared it out yet
  assert(get(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, sequence)) === commit(packet.data))

  // assert correct acknowledgement on counterparty chain
  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    packetAcknowledgementKey(packet.destConnection, packet.destChannel, packet.sequence),
    commit(acknowledgement)
  ))

  // delete our commitment so we can't "acknowledge" again
  delete(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
}
```

#### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves.

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

##### Sending end

The `timeoutPacket` function is called by a module which originally attempted to send a packet to a counterparty module,
where the timeout height has passed on the counterparty chain without the packet being committed, to prove that the packet
can no longer be executed and to allow the calling module to safely perform appropriate state transitions.

There are two variants, for ordered & unordered channels: `timeoutPacketOrdered` and `timeoutPacketUnordered`.

Calling modules MAY atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutPacketOrdered` or `timeoutPacketUnordered`.

`timeoutPacketOrdered`, the variant for ordered channels, checks the recvSequence of the receiving channel end and closes the channel if a packet has timed out.

```typescript
function timeoutPacketOrdered(packet: Packet, proof: CommitmentProof, proofHeight: uint64, nextSequenceRecv: uint64) {
  channel = get(channelKey(packet.sourceConnection, packet.sourceChannel))
  assert(channel.state === OPEN)
  assert(channel.order === ORDERED)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.destChannel === channel.counterpartyChannelIdentifier)

  connection = get(connectionKey(packet.sourceConnection))
  assert(connection.state === OPEN)
  assert(packet.destConnection === connection.counterpartyConnectionIdentifier)

  // check that timeout height has passed on the other end
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight >= timeoutHeight)

  // check that packet has not been received
  assert(nextSequenceRecv < packet.sequence)

  // verify we actually sent this packet, check the store
  assert(get(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, sequence))
         === commit(packet.data))

  // check that the recv sequence is as claimed
  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    nextSequenceRecvKey(packet.destConnection, packet.destChannel),
    nextSequenceRecv
  ))

  // delete our commitment
  delete(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))

  // close the channel
  channel.state = CLOSED
  set(channelKey(packet.sourceConnection, packet.sourceChannel), channel)
}
```

If relations are enforced between timeout heights of subsequent packets, safe bulk timeouts of all packets prior to a timed-out packet can be performed.
This specification omits details for now.

`timeoutPacketUnordered`, the variant for unordered channels, checks the absence of an acknowledgement (which will have been written if the packet was receieved).

`timeoutPacketUnordered` does not close the channel; unordered channels are expected to continue in the face of timed-out packets.

```typescript
function timeoutPacketUnordered(packet: Packet, proof: CommitmentProof, proofHeight: uint64) {
  channel = get(channelKey(packet.sourceConnection, packet.sourceChannel))
  assert(channel.state === OPEN)
  assert(channel.order === UNORDERED)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.destChannel === channel.counterpartyChannelIdentifier)

  connection = get(connectionKey(packet.sourceConnection))
  assert(connection.state === OPEN)
  assert(packet.destConnection === connection.counterpartyConnectionIdentifier)

  // check that timeout height has passed on the other end
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight >= timeoutHeight)

  // verify we actually sent this packet, check the store
  assert(get(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
         === commit(packet.data))

  // verify absence of acknowledgement at packet index
  assert(verifyNonMembership(
    counterpartyStateRoot,
    proof,
    packetAcknowledgementKey(packet.sourceConnection, packet.sourceChannel, packet.sequence)
  ))

  // delete our commitment
  delete(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
}
```

###### Closing on timeout

The `timeoutClose` function is called by a module in order to prove that a packet which ought to have
been received on a particular ordered channel has timed out, and the channel must be closed.

This is an alternative to closing the other end of the channel and proving that closure. Either works.

Calling modules MAY atomically execute any application logic associated with channel closure in conjunction with calling `recvTimeoutPacket`.

```typescript
function timeoutClose(packet: Packet, proof: CommitmentProof, proofHeight: uint64) {
  channel = get(channelKey(packet.destConnection, packet.destChannel))
  assert(channel.state === OPEN)
  assert(channel.order === ORDERED)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.sourceChannel === channel.counterpartyChannelIdentifier)

  connection = get(connectionKey(connectionIdentifier))
  assert(connection.state === OPEN)
  assert(packet.sourceConnection === connection.counterpartyConnectionIdentifier)

  nextSequenceRecv = get(nextSequenceRecvKey(packet.destConnection, packet.destChannel))
  assert(packet.sequence === nextSequenceRecv)

  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight >= packet.timeoutHeight)

  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, sequence),
    commit(packet.data)
  ))

  channel.state = CLOSED
  set(channelKey(packet.destConnection, packet.destChannel), channel)
}
```

##### Cleaning up state

`cleanupPacketOrdered` and `cleanupPacketUnordered`, variants for ordered and unordered channels respectively, are called by a module to remove a received packet commitment from storage. The receiving end must have already processed the packet (whether regularly or past timeout).

`cleanupPacketOrdered` cleans-up a packet on an ordered channel by proving that the packet has been received on the other end.

```typescript
function cleanupPacketOrdered(packet: Packet, proof: CommitmentProof, proofHeight: uint64, nextSequenceRecv: uint64) {
  channel = get(channelKey(packet.sourceConnection, packet.sourceChannel))
  assert(channel.state === OPEN)
  assert(channel.order === ORDERED)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.destChannel === channel.counterpartyChannelIdentifier)

  connection = get(connectionKey(packet.sourceConnection))
  assert(connection.state === OPEN)
  assert(packet.destConnection === connection.counterpartyConnectionIdentifier)

  // assert packet has been received on the other end
  assert(nextSequenceRecv > packet.sequence)

  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))

  // check that the recv sequence is as claimed
  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    nextSequenceRecvKey(packet.destConnection, packet.destChannel),
    nextSequenceRecv
  ))

  // verify we actually sent the packet, check the store
  assert(get(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
             === commit(packet.data))

  // clear the store
  delete(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
}
```

`cleanupPacketUnordered` cleans-up a packet on an unordered channel by proving that the associated acknowledgement has been written.

```typescript
function cleanupPacketUnordered(packet: Packet, proof: CommitmentProof, proofHeight: uint64, acknowledgement: bytes) {
  channel = get(channelKey(packet.sourceConnection, packet.sourceChannel))
  assert(channel.state === OPEN)
  assert(channel.order === UNORDERED)
  assert(authenticate(get(portKey(channel.portIdentifier))))
  assert(packet.destChannel === channel.counterpartyChannelIdentifier)

  connection = get(connectionKey(packet.sourceConnection))
  assert(connection.state === OPEN)
  assert(packet.destConnection === connection.counterpartyConnectionIdentifier)

  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))

  // assert acknowledgement on the other end
  assert(verifyMembership(
    counterpartyStateRoot,
    proof,
    packetAcknowledgementKey(packet.destConnection, packet.destChannel, packet.sequence),
    acknowledgement
  ))

  // verify we actually sent the packet, check the store
  assert(get(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
             === commit(packet.data))

  // clear the store
  delete(packetCommitmentKey(packet.sourceConnection, packet.sourceChannel, packet.sequence))
}
```

#### Querying channels

Channels can be queried with `queryChannel`:

```typescript
function queryChannel(connId: Identifier, chanId: Identifier): ChannelEnd | void {
  return get(channelKey(connId, chanId))
}
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Data structures & encoding can be versioned at the connection or channel level. Channel logic is completely agnostic to packet data formats, which can be changed by the modules any way they like at any time.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

5 June 2019 - Draft submitted
4 July 2019 - Modifications for unordered channels & acknowledgements

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
