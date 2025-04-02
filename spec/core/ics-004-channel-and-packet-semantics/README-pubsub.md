---
ics: 4
title: Broadcast Channel Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 3, 5, 24
author: Manuel Bravo <manuel@informal.systems>
created: 2023-01-16
modified: 2023-01-16
---

## Synopsis

This standard document specifies the state machine handling logic of the broadcast channel abstraction.

## Overview and Basic Concepts

### Motivation

TODO

### Definitions

`Connection` is as defined in [ICS 3](../ics-003-connection-semantics).

`Port` and `authenticateCapability` are as defined in [ICS 5](../ics-005-port-allocation).

`hash` is a generic collision-resistant hash function, the specifics of which must be agreed on by the modules utilising the channel. `hash` can be defined differently by different chains.

`Identifier`, `get`, `set` and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

## System Model and Properties

### Desired Properties

TODO

## Technical Specification

### General Design

TODO

During the handshake process, two ends of a channel come to agreement on a version bytestring associated
with that channel. The contents of this version bytestring are and will remain opaque to the IBC core protocol.
Host state machines MAY utilise the version data to indicate supported IBC/APP protocols, agree on packet
encoding formats, or negotiate other channel-related metadata related to custom logic on top of IBC.

Host state machines MAY also safely ignore the version data or specify an empty string.

### Sub-protocols

> Note: If the host state machine is utilising object capability authentication (see [ICS 005](../ics-005-port-allocation)), all functions utilising ports take an additional capability parameter.

#### Opening a broadcast channel

The `broadcastChanOpen` function is called by a module to open a broadcast channel. 

```typescript
function broadcastChanOpen(
  order: ChannelOrder,
  portIdentifier: Identifier,
  version: string): CapabilityKey {
    abortTransactionUnless(order !== ORDERED_ALLOW_TIMEOUT)

    channelIdentifier = generateIdentifier()
    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))

    abortTransactionUnless(provableStore.get(channelPath(portIdentifier, channelIdentifier)) === null)

    abortTransactionUnless(authenticateCapability(portPath(portIdentifier), portCapability))
    channel = ChannelEnd{state: OPEN,
                         ordering: order,
                         counterpartyPortIdentifier: "",
                         counterpartyChannelIdentifier: "",
                         connectionHops: "",
                         version: version}

    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    channelCapability = newCapability(channelCapabilityPath(portIdentifier, channelIdentifier))
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    return channelCapability
}
```

#### Subscribing to broadcast channel

The `broadcastChanSubscribe` function is called by a module to subscribe to an already open broadcast channel.

```typescript
function broadcastChanSubscribe(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string,
  proofInit: CommitmentProof,
  proofHeight: Height): CapabilityKey {
    channelIdentifier = generateIdentifier()

    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))
    abortTransactionUnless(connectionHops.length === 1) // for v1 of the IBC protocol
    abortTransactionUnless(authenticateCapability(portPath(portIdentifier), portCapability))
    connection = provableStore.get(connectionPath(connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{state: OPEN,
                         ordering: order,
                         counterpartyPortIdentifier: "",
                         counterpartyChannelIdentifier: "",
                         connectionHops: "",
                         version: counterpartyVersion}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofInit,
      counterpartyPortIdentifier,
      counterpartyChannelIdentifier,
      expected
    ))
    channel = ChannelEnd{state: OPEN,
                         ordering: order,
                         counterpartyPortIdentifier: counterpartyPortIdentifier,
                         counterpartyChannelIdentifier: counterpartyChannelIdentifier,
                         connectionHops: connectionHops,
                         version: version}
    
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    channelCapability = newCapability(channelCapabilityPath(portIdentifier, channelIdentifier))

    // initialize channel sequences
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)

    return channelCapability
}
```

#### Closing a broadcast channel

The `broadcastChanClose` function is called by either the broadcaster or subscriber to close the broadcast channel or unsubscribe respectively. 

```typescript
function broadcastChanClose(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    channel.state = CLOSED
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

Once closed, broadcast channels cannot be reopened and identifiers cannot be reused.

#### Packet flow & handling

The `broadcastPacket` function is called by a module in order to send *data* (in the form of an IBC packet) on a channel end owned by the calling module.

The IBC handler performs the following steps in order:

- Checks that the broadcast channel is open.
- Checks that the calling module owns the sending port (see [ICS 5](../ics-005-port-allocation)).
- Increments the send sequence counter associated with the channel.
- Stores a constant-size commitment to the packet data.
- Emits a `broadcastPacket` event.

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment to the data. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

```typescript
function broadcastPacket(
  capability: CapabilityKey,
  sourcePort: Identifier,
  sourceChannel: Identifier,
  data: bytes) {
    channel = provableStore.get(channelPath(sourcePort, sourceChannel))

    // check that the channel is not closed to send packets; 
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)

    // check if the calling module owns the sending port
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(sourcePort, sourceChannel), capability))

    // increment the send sequence counter
    sequence = provableStore.get(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel))
    provableStore.set(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel), sequence+1)

    // store commitment to the packet data
    provableStore.set(
      packetCommitmentPath(sourcePort, sourceChannel, sequence),
      hash(data)
    )

    // log that a packet can be safely sent
    emitLogEntry("broadcastPacket", {sequence: sequence, data: data})
}
```

#### Delivering packets

The `deliverPacket` function is called by a module in order to receive an IBC broadcast packet sent on the corresponding broadcast channel end on the counterparty chain.

The IBC handler performs the following steps in order:

- Checks that the broadcast channel & connection are open to receive packets
- Checks that the calling module owns the receiving port
- Checks that the packet metadata matches the channel & connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive (for ordered)
- Checks the inclusion proof of packet data commitment in the outgoing chain's state
- Sets a store path to indicate that the packet has been received (for unordered channels only)
- Increments the packet receive sequence associated with the channel end (ordered)

We pass the address of the `relayer` that signed and submitted the packet to enable a module to optionally provide some rewards. This provides a foundation for fee payment, but can be used for other techniques as well (like calculating a leaderboard).

```typescript
function deliverPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {

    channel = provableStore.get(channelPath(packet.destPort, packet.destChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.destPort, packet.destChannel), capability))
    abortTransactionUnless(packet.sourcePort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.sourceChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))

    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    abortTransactionUnless(connection.verifyPacketData(
      proofHeight,
      proof,
      packet.sourcePort,
      packet.sourceChannel,
      packet.sequence,
      hash(packet.data)
    ))

    switch channel.order {
      case ORDERED:
        nextSequenceRecv = provableStore.get(nextSequenceRecvPath(packet.destPort, packet.destChannel))
        abortTransactionUnless(packet.sequence === nextSequenceRecv)
  
        // all assertions passed, we can alter state
        nextSequenceRecv = nextSequenceRecv + 1
        provableStore.set(nextSequenceRecvPath(packet.destPort, packet.destChannel), nextSequenceRecv)

      case UNORDERED:
        packetReceipt = provableStore.get(packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence))
        abortTransactionUnless(packetReceipt === null)

        // all assertions passed, we can alter state
        provableStore.set(packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence), SUCCESSFUL_RECEIPT)
    }
    // return transparent packet
    return packet
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

Jan 16, 2023 - Draft submitted

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
