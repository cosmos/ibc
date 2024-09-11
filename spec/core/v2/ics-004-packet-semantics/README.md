---
ics: 4
title: Packet Semantics
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

The ICS-04 defines the packet data strcuture, the packet-flow semantics and the mechanisms to route packets to their specific IBC applications. 

### Motivation

The interblockchain communication protocol uses a cross-chain message passing model. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are visible to relayers and can be read from a blockchain by any relayer process and submitted to any other blockchain.

> **Example**: An application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

### Definitions

`ConsensusState` is as defined in [ICS 2](../ics-002-client-semantics).

`Port` and `authenticateCapability` are as defined in [ICS 5](../ics-005-port-allocation).

`hash` is a generic collision-resistant hash function, the specifics of which must be agreed on by the modules utilising the channel. `hash` can be defined differently by different chains.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

- The `state` is the current state of the channel end.
- NOTE - do we need to keep this as only `unordered` for some compatibility reasons? Otherwise being only underored we can delete this, no? - The `ordering` field indicates whether the channel is `unordered`, `ordered`, or `ordered_allow_timeout`.
NOTE - do we need to keep these two? 
- The `counterpartyPortIdentifier` identifies the port on the counterparty chain which owns the other end of the channel.
- The `counterpartyClientIdentifier` identifies the client end on the counterparty chain.
- The `nextSequenceSend`, stored separately, tracks the sequence number for the next packet to be sent.
- The `nextSequenceRecv`, stored separately, tracks the sequence number for the next packet to be received.
- The `nextSequenceAck`, stored separately, tracks the sequence number for the next packet to be acknowledged.
- The `version` string stores an opaque channel version, which is agreed upon during the handshake. This can determine module-level configuration such as which packet encoding is used for the channel. This version is not used by the core IBC protocol. If the version string contains structured metadata for the application to parse and interpret, then it is considered best practice to encode all metadata in a JSON struct and include the marshalled string in the version field.

`Counterparty` is the data structure responsible for maintaining the counterparty information.

```typescript
interface Counterparty {
    channelId: Identifier
    keyPrefix: CommitmentPrefix 
}
```

NOTE - Do we want to keep an upgrade spec to specify how to change the client and so on? See the [upgrade spec](../../ics-004-channel-and-packet-semantics/UPGRADES.md) for details on `upgradeSequence`.

A `Packet`, in the interblockchain communication protocol, is a particular interface defined as follows:

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: Height
  timeoutTimestamp: uint64
  sourcePort: Identifier // identifier of the application on sender
  sourceChannel: Identifier // identifier of the client of destination on sender chain
  destPort: Identifier // identifier of the application on destination
  destChannel: Identifier // identifier of the client of sender on the destination chain
  data: [] bytes
}
```

IBC version 2 will provide packet delivery between two chains communicating and identifying each other by on-chain light clients as specified in ICS-02. The channelID derived from the clientIDs will tell the IBC router which chain to send the packets to and which chain a received packet came from, while the portID specifies which application on the router the packet should be sent to.

- The `sequence` number corresponds to the order of sends and receives, where a packet with an earlier sequence number must be sent and received before a packet with a later sequence number.
- The `timeoutHeight` indicates a consensus height on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
- The `timeoutTimestamp` indicates a timestamp on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
- The `sourcePort` identifies the port on the sending chain.
- The `sourceChannel` identifies the channel end on the sending chain.
- The `destPort` identifies the port on the receiving chain.
- The `destChannel` identifies the channel end on the receiving chain.
- The `data` is an opaque array of data values which can be defined by the application logic of the associated modules. When multiple values are passed-in the system will handle the packet as a multi-data packet. 

Note that a `Packet` is never directly serialised. Rather it is an intermediary structure used in certain function calls that may need to be created or processed by modules calling the IBC handler.

An `OpaquePacket` is a packet, but cloaked in an obscuring data type by the host state machine, such that a module cannot act upon it other than to pass it to the IBC handler. The IBC handler can cast a `Packet` to an `OpaquePacket` and vice versa.

```typescript
type OpaquePacket = object
```

The protocol introduces standardized packet receipts that will serve as sentinel values for the receiving chain to explicitly write to its store the outcome of a `recvPacket`.

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

- IBC version 2 supports only *unordered* communications, thus, packets may be sent and received in any order. Unordered packets, have individual timeouts specified in terms of the destination chain's height.

#### Permissioning

- Channels should be permissioned to one module on each end, determined during the handshake and immutable afterwards (higher-level logic could tokenize channel ownership by tokenising ownership of the port).
  Only the module associated with a channel end should be able to send or receive on it.

## Technical Specification

### Dataflow visualisation

The architecture of clients, connections, channels and packets:

![Dataflow Visualisation](../../ics-004-channel-and-packet-semantics/dataflow.png)

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

#### Counterparty Idenfitifcation and Registration 

A client MUST have the ability to idenfity its counterparty. With a client, we can prove any key/value path on the counterparty. However, without knowing which identifier the counterparty uses when it sends messages to us, we cannot differentiate between messages sent from the counterparty to our chain vs messages sent from the counterparty with other chains. Most implementations will not be able to store the ICS-24 paths directly as a key in the global namespace, but will instead write to a reserved, prefixed keyspace so as not to conflict with other application state writes. Thus the counteparty information we must have includes both its identifier for our chain as well as the key prefix under which it will write the provable ICS-24 paths.

Thus, IBC version 2 introduces a new message `RegisterCounterparty` that will associate the counterparty client of our chain with our client of the counterparty. Thus, if the `RegisterCounterparty` message is submitted to both sides correctly. Then both sides have mirrored <client,client> pairs that can be treated as channel identifiers. Assuming they are correct, the client on each side is unique and provides an authenticated stream of packet data between the two chains. If the `RegisterCounterparty` message submits the wrong clientID, this can lead to invalid behaviour; but this is equivalent to a relayer submitting an invalid client in place of a correct client for the desired chain. In the simplest case, we can rely on out-of-band social consensus to only send on valid <client, client> pairs that represent a connection between the desired chains of the user; just as we currently rely on out-of-band social consensus that a given clientID and channel built on top of it is the valid, canonical identifier of our desired chain.

```typescript
function RegisterCounterparty(
    channelIdentifier: Identifier, // this will be our own client identifier representing our channel to desired chain
    counterpartyChannelIdentifier: Identifier, // this is the counterparty's identifier of our chain
    counterpartyKeyPrefix: CommitmentPrefix,
    authentication: data, // implementation-specific authentication data
) {
    assert(verify(authentication))

    counterparty = Counterparty{
        channelId: counterpartyChannelIdentifier,
        keyPrefix: counterpartyKeyPrefix
    }

    privateStore.set(counterpartyPath(channelIdentifier), counterparty)
}
```

The `RegisterCounterparty` method allows for authentication data that implementations may verify before storing the provided counterparty identifier. The strongest authentication possible is to have a valid clientState and consensus state of our chain in the authentication along with a proof it was stored at the claimed counterparty identifier.
A simpler but weaker authentication would simply be to check that the `RegisterCounterparty` message is sent by the same relayer that initialized the client. This would make the client parameters completely initialized by the relayer. Thus, users must verify that the client is pointing to the correct chain and that the counterparty identifier is correct as well before using the lite channel identified by the provided client-client pair.

```typescript
// getCounterparty retrieves the stored counterparty identifier
// given the channelIdentifier on our chain once it is provided
function getCounterparty(channelIdentifier: Identifier): Counterparty {
    return privateStore.get(counterpartyPath(channelIdentifier))
}
```

#### Identifier validation

Channels are stored under a unique `(portIdentifier, channelIdentifier)` prefix.
The validation function `validatePortIdentifier` MAY be provided.

```typescript
type validateChannelIdentifier = (portIdentifier: Identifier, channelIdentifier: Identifier) => boolean
```

If not provided, the default `validateChannelIdentifier` function will always return `true`.

When the opening handshake is complete, the module which initiates the handshake will own the end of the created channel on the host ledger, and the counterparty module which
it specifies will own the other end of the created channel on the counterparty chain. Once a channel is created, ownership cannot be changed (although higher-level abstractions
could be implemented to provide this).

Chains MUST implement a function `generateIdentifier` which chooses an identifier, e.g. by incrementing a counter:

```typescript
type generateIdentifier = () -> Identifier
```

##### Multihop utility functions

MMM,
 
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

![Packet State Machine](../../ics-004-channel-and-packet-semantics/packet-state-machine.png)

##### A day in the life of a packet

The following sequence of steps must occur for a packet to be sent from module *1* on machine *A* to module *2* on machine *B*, starting from scratch.

The module can interface with the IBC handler through [ICS 25]( ../../ics-025-handler-interface) or [ICS 26]( ../../ics-026-routing-module).

1. Initial client & port setup, in any order
    1. Client created on *A* for *B* (see [ICS 2](../ics-002-client-semantics))
    1. Client created on *B* for *A* (see [ICS 2](../ics-002-client-semantics))
    1. Module *1* binds to a port (see [ICS 5](../ics-005-port-allocation))
    1. Module *2* binds to a port (see [ICS 5](../ics-005-port-allocation)), which is communicated out-of-band to module *1*
1. Establishment of a connection & channel, optimistic send, in order
    1. Connection opening handshake started from *A* to *B* by module *1* (see [ICS 3](../../ics-003-connection-semantics))
    1. Channel opening handshake started from *1* to *2* using the newly created connection (this ICS)
    1. Packet sent over the newly created channel from *1* to *2* (this ICS)
1. Successful completion of handshakes (if either handshake fails, the connection/channel can be closed & the packet timed-out)
    1. Connection opening handshake completes successfully (see [ICS 3](../../ics-003-connection-semantics)) (this will require participation of a relayer process)
    1. Channel opening handshake completes successfully (this ICS) (this will require participation of a relayer process)
1. Packet confirmation on machine *B*, module *2* (or packet timeout if the timeout height has passed) (this will require participation of a relayer process)
1. Acknowledgement (possibly) relayed back from module *2* on machine *B* to module *1* on machine *A*

Represented spatially, packet transit between two machines can be rendered as follows:

![Packet Transit](../../ics-004-channel-and-packet-semantics/packet-transit.png)

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
