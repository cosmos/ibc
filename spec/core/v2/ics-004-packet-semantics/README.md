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

The ICS-04 defines the mechanism to bidirectionally set up clients for each pair of communicating chains with specific Identifiers to establish the ground truth for the secure packet delivery, the packet data strcuture including the multi-data packet, the packet-flow semantics, the mechanisms to route the verification to the underlying clients, and how to route packets to their specific IBC applications. 

### Motivation

The interblockchain communication protocol uses a cross-chain message passing model. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are visible to relayers and can be read from a blockchain by any relayer process and submitted to any other blockchain. 

> **Example**: An application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

The IBC version 2 will provide packet delivery between two chains communicating and identifying each other by on-chain light clients as specified in ICS-02.

### Definitions

`ConsensusState` is as defined in [ICS 2](../ics-002-client-semantics).
// NOTE what about Port and Capabilities? 
`Port` and `authenticateCapability` are as defined in [ICS 5](../ics-005-port-allocation).

`hash` is a generic collision-resistant hash function, the specifics of which must be agreed on by the modules utilising the channel. `hash` can be defined differently by different chains.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

`Counterparty` is the data structure responsible for maintaining the counterparty information necessary to establish the ground truth for securing the interchain communication.

```typescript
interface Counterparty {
    channelId: Identifier
    keyPrefix: CommitmentPrefix 
}
```

- The `nextSequenceSend`, stored separately, tracks the sequence number for the next packet to be sent.
- The `nextSequenceRecv`, stored separately, tracks the sequence number for the next packet to be received.
- The `nextSequenceAck`, stored separately, tracks the sequence number for the next packet to be acknowledged.
// Note do we need to keep the version in the end? Should this be just the protocol version?
- The `version` string stores an opaque channel version, which is agreed upon during the handshake. This can determine module-level configuration such as which packet encoding is used for the channel. This version is not used by the core IBC protocol. If the version string contains structured metadata for the application to parse and interpret, then it is considered best practice to encode all metadata in a JSON struct and include the marshalled string in the version field.

// NOTE - Do we want to keep an upgrade spec to specify how to change the client and so on? Probably unnecessary, just showing here how to do that would be enough. 

See the [upgrade spec](../../ics-004-channel-and-packet-semantics/UPGRADES.md) for details on `upgradeSequence`.

A `Packet`, in the interblockchain communication protocol, is a particular interface defined as follows:

```typescript
interface Packet {
    sourceIdentifier: bytes,
    destIdentifier: bytes,
    sequence: uint64
    timeout: uint64,
    data: [Payload]
}
```

- The `sourceIdentifier` derived from the `clientIdentifier` will tell the IBC router which chain a received packet came from.
- The `destIdentifier` derived from the `clientIdentifier` will tell the IBC router which chain to send the packets to. 
- The `sequence` number corresponds to the order of sends and receives, where a packet with an earlier sequence number must be sent and received before a packet with a later sequence number.
- The `timeout` indicates the UNIX timestamp in seconds and is encoded in LittleEndian. It must be passed on the destination chain and once elapsed, will no longer allow the packet processing, and will instead generate a time-out.

The `Payload` is a particular interface defined as follows:

```typescript
interface Payload {
    sourcePort: bytes,
    destPort: bytes,
    version: string,
    encoding: Encoding,
    appData: bytes,
}

enum Encoding {
  NO_ENCODING_SPECIFIED,
    PROTO_3,
    JSON,
    RLP,
    BCS,
}
```

- The `sourcePort` identifies the source application.
- The `destPort` identifies the destination application. 
- The `version` to specify the application version to be used.  
- The `encoding` to allow the specification of custom data encoding among those agreed in the `Encoding` enum.   
- The `appData` that can be defined by the application logic of the associated modules. 

When the array of payloads, passed-in the packet, is populated with multiple values, the system will handle the packet as a multi-data packet. 

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
- Proofs should be batchable where possible.
- The system must be able to process the multiple packetData contained in a single IBC packet, to reduce the amount of packet flows. 

#### Exactly-once delivery

- IBC packets sent on one end of a channel should be delivered exactly once to the other end.
- No network synchrony assumptions should be required for exactly-once safety.
  If one or both of the chains halt, packets may be delivered no more than once, and once the chains resume packets should be able to flow again.

#### Ordering

- IBC version 2 supports only *unordered* communications, thus, packets may be sent and received in any order. Unordered packets, have individual timeouts specified in terms of the destination chain's height.

#### Permissioning

// NOTE - here what about capabilities and permissions? 

- Channels should be permissioned to one module on each end, determined during the handshake and immutable afterwards (higher-level logic could tokenize channel ownership by tokenising ownership of the port).
  Only the module associated with a channel end should be able to send or receive on it.

## Technical Specification

### Dataflow visualisation

The architecture of clients, connections, channels and packets:

![Dataflow Visualisation](../../ics-004-channel-and-packet-semantics/dataflow.png)

### Preliminaries

#### Store paths

// Unnecessary? 

```typescript
function counterpartyPath(sourceClientID: Identifier, destClientID: Identifier, keyPrefix: CommitmentPrefix): Path {
    return "counterparty/client/{sourceClientID}/client/{destClientID}/CommitmentPrefix/{keyPrefix}"
}
```

/* NOTE Channel paths and capabilities should be maintained for backward compatibility? 
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

*/

// Note what about this: https://github.com/cosmos/ibc/issues/1129

// Note - Am I breaking something here with the approach taken? Is that ok? 

The `nextSequenceSend`, `nextSequenceRecv`, and `nextSequenceAck` unsigned integer counters are stored separately so they can be proved individually:

```typescript
function nextSequenceSendPath(sourceID: Identifier, destID: Identifier): Path {
    return "nextSequenceSend/clients/{sourceID}/clients/{destID}"
}

function nextSequenceRecvPath(sourceID: Identifier,destID: Identifier): Path {
    return "nextSequenceRecv/clients/{sourceID}/clients/{destID}"
}
 
function nextSequenceAckPath(sourceID: Identifier, destID: Identifier): Path {
    return "nextSequenceAck/clients/{sourceID}/clients/{destID}"
}
```

Constant-size commitments to packet data fields are stored under the packet sequence number:

```typescript
function packetCommitmentPath(sourceID: Identifier, destID: Identifier, sequence: uint64): Path {
    return "commitments/clients/{sourceID}/clients/{destID}/sequences/{sequence}"
}
```

Absence of the path in the store is equivalent to a zero-bit.

Packet receipt data are stored under the `packetReceiptPath`. In the case of a successful receive, the destination chain writes a sentinel success value of `SUCCESSFUL_RECEIPT`.
Some channel types MAY write a sentinel timeout value `TIMEOUT_RECEIPT` if the packet is received after the specified timeout.

```typescript
function packetReceiptPath(sourceID: Identifier, destID: Identifier, sequence: uint64): Path {
    return "receipts/clients/{sourceID}/clients/{destID}/sequences/{sequence}"
}
```

Packet acknowledgement data are stored under the `packetAcknowledgementPath`:

```typescript
function packetAcknowledgementPath(sourceID: Identifier, destID: Identifier, sequence: uint64): Path {
    return "acks/clients/{sourceID}/clients/{destID}/sequences/{sequence}"
}
```

### Sub-protocols

> Note: If the host state machine is utilising object capability authentication (see [ICS 005](../ics-005-port-allocation)), all functions utilising ports take an additional capability parameter.

#### Counterparty Idenfitifcation and Registration 

A client MUST have the ability to idenfity its counterparty. With a client, we can prove any key/value path on the counterparty. However, without knowing which identifier the counterparty uses when it sends messages to us, we cannot differentiate between messages sent from the counterparty to our chain vs messages sent from the counterparty with other chains. Most implementations will not be able to store the ICS-24 paths directly as a key in the global namespace, but will instead write to a reserved, prefixed keyspace so as not to conflict with other application state writes. Thus the counteparty information we must have includes both its identifier for our chain as well as the key prefix under which it will write the provable ICS-24 paths.

Thus, IBC version 2 introduces a new message `RegisterCounterparty` that will associate the counterparty client of our chain with our client of the counterparty. Thus, if the `RegisterCounterparty` message is submitted to both sides correctly. Then both sides have mirrored <client,client> pairs that can be treated as channel identifiers. Assuming they are correct, the client on each side is unique and provides an authenticated stream of packet data between the two chains. If the `RegisterCounterparty` message submits the wrong clientID, this can lead to invalid behaviour; but this is equivalent to a relayer submitting an invalid client in place of a correct client for the desired chain. In the simplest case, we can rely on out-of-band social consensus to only send on valid <client, client> pairs that represent a connection between the desired chains of the user; just as we currently rely on out-of-band social consensus that a given clientID and channel built on top of it is the valid, canonical identifier of our desired chain.

```typescript
function RegisterCounterparty(
    clientID: Identifier, // this will be our own client identifier representing our channel to desired chain
    counterpartyClientID: Identifier, // this is the counterparty's identifier of our chain
    counterpartyKeyPrefix: CommitmentPrefix,
    authentication: data, // implementation-specific authentication data
) {
    assert(verify(authentication))

    counterparty = Counterparty{
        channelId: counterpartyClientID,
        keyPrefix: counterpartyKeyPrefix
    }

    privateStore.set(counterpartyPath(clientID), counterparty)
}
```

The `RegisterCounterparty` method allows for authentication data that implementations may verify before storing the provided counterparty identifier. The strongest authentication possible is to have a valid clientState and consensus state of our chain in the authentication along with a proof it was stored at the claimed counterparty identifier.
A simpler but weaker authentication would simply be to check that the `RegisterCounterparty` message is sent by the same relayer that initialized the client. This would make the client parameters completely initialized by the relayer. Thus, users must verify that the client is pointing to the correct chain and that the counterparty identifier is correct as well before using the lite channel identified by the provided client-client pair.

```typescript
// getCounterparty retrieves the stored counterparty identifier
// given the channelIdentifier on our chain once it is provided
function getCounterparty(clientID: Identifier): Counterparty {
    return privateStore.get(counterpartyPath(clientID))
}
```

Thus, once two chains have set up clients for each other with specific Identifiers, they can send IBC packets using the packet interface defined before.

Since the packets are addressed **directly** with the underlying light clients, there are **no** more handshakes necessary. Instead the packet sender must be capable of providing the correct <client, client> pair.

Sending a packet with the wrong source client is equivalent to sending a packet with the wrong source channel. Sending a packet on a channel with the wrong provided counterparty is a new source of errors, however this is added to the burden of out-of-band social consensus.

If the client and counterparty identifiers are setup correctly, then the correctness and soundness properties of IBC holds. IBC packet flow is guaranteed to succeed. If a user sends a packet with the wrong destination channel, then as we will see it will be impossible for the intended destination to correctly verify the packet thus, the packet will simply time out.

#### Registering IBC applications on the router

The IBC router contains a mapping from a reserved application port and the supported versions of that application as well as a mapping from channelIdentifiers to channels.

```typescript
type IBCRouter struct {
    versions: portID -> [Version]
    callbacks: portID -> [Callback]
    clients: clientId -> Client
    ports: portID -> counterpartyPortID
}
```

#### Packet Flow through the Router & handling

TODO : Adapat to new flow

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

The `sendPacket` function is called by a module in order to send *data* in the form of an IBC packet. 

Calling modules MUST execute application logic atomically in conjunction with calling `sendPacket`.

The IBC handler performs the following steps in order:

- Checks that the underlying clients is properly registered in the IBC router. 
- Checks that the timeout height specified has not already passed on the destination chain
- Stores a constant-size commitment to the packet data & packet timeout
- Increments the send sequence counter associated with the channel
- Returns the sequence number of the sent packet

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment to the data & timeout value. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

```typescript
function sendPacket(
    sourceClientID: Identifier,
    destClientID: Identifier,
    timeoutHeight: Height,
    timeoutTimestamp: uint64,
    packetData: []byte
): uint64 {
    // in this specification, the source channel is the clientId
    client = router.clients[packet.sourceClientID]
    assert(client !== null)

    // disallow packets with a zero timeoutHeight and timeoutTimestamp
    assert(timeoutHeight !== 0 || timeoutTimestamp !== 0)

    // check that the timeout height hasn't already passed in the local client tracking the receiving chain
    latestClientHeight = client.latestClientHeight()
    assert(timeoutHeight === 0 || latestClientHeight < timeoutHeight)
    // NOTE - What is the commit port? Should be the sourcePort? If yes, in the packet we should put destPort and destID?
    // if the sequence doesn't already exist, this call initializes the sequence to 0
    //sequence = channelStore.get(nextSequenceSendPath(commitPort, sourceID))
    sequence = channelStore.get(nextSequenceSendPath(sourceClientID, destClientID))
    
    // store commitment to the packet data & packet timeout
    // Note do we need to keep the channelStore? Should this be instead the counterParty store or something similar? Do we keep it for backward compatibility?
    channelStore.set(
      packetCommitmentPath(sourceClientID, destClientID, sequence),
      hash(hash(data), timeoutHeight, timeoutTimestamp)
    )

    // increment the sequence. Thus there are monotonically increasing sequences for packet flow
    // from sourcePort, sourceChannel pair
    channelStore.set(nextSequenceSendPath(sourceClientID, destClientID), sequence+1)

    // log that a packet can be safely sent
    // introducing sourceID and destID can be useful for monitoring - e.g. if one wants to monitor all packets between sourceID and destID emitting this in the event would simplify his life. 

    emitLogEntry("sendPacket", {
      sourceID: Identifier, 
      destID: Identifier, 
      sequence: sequence,
      data: data,
      timeoutHeight: timeoutHeight,
      timeoutTimestamp: timeoutTimestamp
    })

}
```

#### Receiving packets

The `recvPacket` function is called by a module in order to receive an IBC packet sent on the corresponding client on the counterparty chain.

Atomically in conjunction with calling `recvPacket`, calling modules MUST either execute application logic or queue the packet for future execution.

The IBC handler performs the following steps in order:

- Checks that the clients is properly set in IBC router
- Checks that the packet metadata matches the channel & connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive (for ordered and ordered_allow_timeout channels)
- Checks that the timeout height and timestamp have not yet passed
- Checks the inclusion proof of packet data commitment in the outgoing chain's state
- Sets a store path to indicate that the packet has been received (unordered channels only)
- Increments the packet receive sequence associated with the channel end (ordered and ordered_allow_timeout channels only)

We pass the address of the `relayer` that signed and submitted the packet to enable a module to optionally provide some rewards. This provides a foundation for fee payment, but can be used for other techniques as well (like calculating a leaderboard).

```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {
    // in this specification, the destination channel is the clientId
    client = router.clients[packet.destClientID]
    assert(client !== null)

    // assert source channel is destChannel's counterparty channel identifier
    counterparty = getCounterparty(packet.sourceClientID)
    assert(packet.sourceClientID == counterparty.clientId)

    // assert source port is destPort's counterparty port identifier
    assert(packet.sourcePort == ports[packet.destPort]) // Needed? 

    packetPath = packetCommitmentPath(packet.sourceClientID, packet.destClientID, packet.sequence)
    merklePath = applyPrefix(counterparty.keyPrefix, packetPath)
    // DISCUSSION NEEDED: Should we have an in-protocol notion of Prefixing the path
    // or should we make this a concern of the client's VerifyMembership
    // proofPath = applyPrefix(client.counterpartyChannelStoreIdentifier, packetPath)
    assert(client.verifyMembership(
        proofHeight,
        0, 0, // zeroed out delay period
        proof,
        merklePath,
        hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)
    ))

    assert(packet.timeoutHeight === 0 || getConsensusHeight() < packet.timeoutHeight)
    assert(packet.timeoutTimestamp === 0 || currentTimestamp() < packet.timeoutTimestamp)

  
    // we must set the receipt so it can be verified on the other side
    // this receipt does not contain any data, since the packet has not yet been processed
    // it's the sentinel success receipt: []byte{0x01}
    packetReceipt = channelStore.get(packetReceiptPath(packet.sourceChannelID, packet.destChannelID, packet.sequence))
    assert(packetReceipt === null)

    channelStore.set(
        packetReceiptPath(packet.sourceChannelID, packet.destChannelID, packet.sequence),
        SUCCESSFUL_RECEIPT
    )

    // log that a packet has been received
    emitLogEntry("recvPacket", {
      data: packet.data
      timeoutHeight: packet.timeoutHeight,
      timeoutTimestamp: packet.timeoutTimestamp,
      sequence: packet.sequence,
      sourceClientID: packet.sourceClientID,
      destClientID: packet.destClientID,
      // MMM app= packet.PacketData.destPort?? I mean shall we use the app in somehow here? 
    })

    cbs = router.callbacks[packet.destClientID]
    // IMPORTANT: if the ack is error, then the callback reverts its internal state changes, but the entire tx continues
    ack = cbs.OnRecvPacket(packet, relayer)
    
    if ack != nil {
        channelStore.set(packetAcknowledgementPath(packet.sourceClientID, packet.destClientID, packet.sequence), ack)
    }
}
```

#### Writing acknowledgements

TODO: Define multidata ack, adpat description

The `writeAcknowledgement` function is called by a module in order to write data which resulted from processing an IBC packet that the sending chain can then verify, a sort of "execution receipt" or "RPC call response".

Calling modules MUST execute application logic atomically in conjunction with calling `writeAcknowledgement`.

This is an asynchronous acknowledgement, the contents of which do not need to be determined when the packet is received, only when processing is complete. In the synchronous case, `writeAcknowledgement` can be called in the same transaction (atomically) with `recvPacket`.

Acknowledging packets is not required; however, if packets are not acknowledged, packet commitments cannot be deleted on the source chain. Future versions of IBC may include ways for modules to specify whether or not they will be acknowledging packets in order to allow for cleanup.

`writeAcknowledgement` *does not* check if the packet being acknowledged was actually received, because this would result in proofs being verified twice for acknowledged packets. This aspect of correctness is the responsibility of the calling module.
The calling module MUST only call `writeAcknowledgement` with a packet previously received from `recvPacket`.

The IBC handler performs the following steps in order:

- Checks that an acknowledgement for this packet has not yet been written
- Sets the opaque acknowledgement value at a store path unique to the packet

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
    proof: CommitmentProof,
    proofHeight: Height,
    relayer: string
) {
    // in this specification, the source channel is the clientId
    client = router.clients[packet.sourceChannel]
    assert(client !== null)

    // assert dest channel is sourceChannel's counterparty channel identifier
    counterparty = getCounterparty(packet.destChannel)
    assert(packet.sourceChannel == counterparty.channelId)
   
    // assert dest port is sourcePort's counterparty port identifier
    assert(packet.destPort == ports[packet.sourcePort])

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(hash(packet.data), packet.timeoutHeight, packet.timeoutTimestamp))

    ackPath = packetAcknowledgementPath(packet.destPort, packet.destChannel)
    merklePath = applyPrefix(counterparty.keyPrefix, ackPath)
    assert(client.verifyMembership(
        proofHeight,
        0, 0,
        proof,
        merklePath,
        hash(acknowledgement)
    ))

    cbs = router.callbacks[packet.sourcePort]
    cbs.OnAcknowledgePacket(packet, acknowledgement, relayer)

    channelStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
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
    proof: CommitmentProof,
    proofHeight: Height,
    relayer: string
) {
    // in this specification, the source channel is the clientId
    client = router.clients[packet.sourceChannel]
    assert(client !== null)

    // assert dest channel is sourceChannel's counterparty channel identifier
    counterparty = getCounterparty(packet.destChannel)
    assert(packet.sourceChannel == counterparty.channelId)
   
    // assert dest port is sourcePort's counterparty port identifier
    assert(packet.destPort == ports[packet.sourcePort])

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(hash(packet.data), packet.timeoutHeight, packet.timeoutTimestamp))

    // get the timestamp from the final consensus state in the channel path
    var proofTimestamp
    proofTimestamp = client.getTimestampAtHeight(proofHeight)
    assert(err != nil)

    // check that timeout height or timeout timestamp has passed on the other end
    asert(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && proofTimestamp >= packet.timeoutTimestamp))

    receiptPath = packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence)
    merklePath = applyPrefix(counterparty.keyPrefix, receiptPath)
    assert(client.verifyNonMembership(
        proofHeight
        0, 0,
        proof,
        merklePath
    ))

    cbs = router.callbacks[packet.sourcePort]
    cbs.OnTimeoutPacket(packet, relayer)

    channelStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
}
```

##### Cleaning up state

Packets must be acknowledged in order to be cleaned-up.

#### Reasoning about race conditions

##### Identifier allocation

There is an unavoidable race condition on identifier allocation on the destination chain. Modules would be well-advised to utilise pseudo-random, non-valuable identifiers. Managing to claim the identifier that another module wishes to use, however, while annoying, cannot man-in-the-middle a handshake since the receiving module must already own the port to which the handshake was targeted.

##### Timeouts / packet confirmation

There is no race condition between a packet timeout and packet confirmation, as the packet will either have passed the timeout height prior to receipt or not.

##### Man-in-the-middle attacks during handshakes

Verification of cross-chain state prevents man-in-the-middle attacks for both connection handshakes & channel handshakes since all information (source, destination client, channel, etc.) is known by the module which starts the handshake and confirmed prior to handshake completion.

##### Clients unreachability with in-flight packets

If a client has been frozen while packets are in-flight, the packets can no longer be received on the destination chain and can be timed-out on the source chain.

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


/* NOTE What to do with this? 

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

*/
