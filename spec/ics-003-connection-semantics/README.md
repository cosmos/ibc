---
ics: 3
title: Connection Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 24
required-by: 4, 25
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together facilitate cross-chain sub-state verification and packet association (through channels). A protocol for safely establishing a connection between two chains is described.

### Motivation

The core IBC protocol provides *authorisation* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction specified in this standard, in conjunction with the *client* abstraction specified in [ICS 2](../ics-002-client-semantics), defines the *authorisation* semantics of IBC. Ordering semantics are described in [ICS 4](../ics-004-channel-and-packet-semantics)).

### Definitions

Client-related types & functions are as defined in [ICS 2](../ics-002-client-semantics).

Commitment proof related types & functions are defined in [ICS 23](../ics-023-vector-commitments)

`Identifier` and other host state machine requirements are as defined in [ICS 24](../ics-024-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

An *actor*, as referred to in this specification, is an entity capable of executing datagrams who is paying for computation / storage (via gas or a similar mechanism) but is otherwise untrusted. Possible actors include:

- End users signing with an account key
- On-chain smart contracts acting autonomously or in response to another transaction
- On-chain modules acting in response to another transaction or in a scheduled manner

### Desired Properties

- Implementing blockchains should be able to safely allow untrusted actors to open and update connections.

#### Pre-Establishment

Prior to connection establishment:

- No further IBC sub-protocols should operate, since cross-chain sub-states cannot be verified.
- The initiating actor (who creates the connection) must be able to specify an initial consensus state for the chain to connect to and an initial consensus state for the connecting chain (implicitly, e.g. by sending the transaction).

#### During Handshake

Once a negotiation handshake has begun:

- Only the appropriate handshake datagrams can be executed in order.
- No third chain can masquerade as one of the two handshaking chains

#### Post-Establishment

Once a negotiation handshake has completed:

- The created connection objects on both chains contain the consensus states specified by the initiating actor.
- No other connection objects can be maliciously created on other chains by replaying datagrams.

## Technical Specification

### Data Structures

This ICS defines the `ConnectionState` and `ConnectionEnd` types:

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
}
```

```typescript
interface ConnectionEnd {
  state: ConnectionState
  counterpartyConnectionIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string | []string
}
```

- The `state` field describes the current state of the connection end.
- The `counterpartyConnectionIdentifier` field identifies the connection end on the counterparty chain associated with this connection.
- The `counterpartyPrefix` field contains the prefix used for state verification on the counterparty chain associated with this connection.
  Chains should expose an endpoint to allow relayers to query the connection prefix.
  If not specified, a default `counterpartyPrefix` of `"ibc"` should be used.
- The `clientIdentifier` field identifies the client associated with this connection.
- The `counterpartyClientIdentifier` field identifies the client on the counterparty chain associated with this connection.
- The `version` field is an opaque string which can be utilised to determine encodings or protocols for channels or packets utilising this connection.
  If not specified, a default `version` of `""` should be used.

### Store paths

Connection paths are stored under a unique identifier.

```typescript
function connectionPath(id: Identifier): Path {
    return "connections/{id}"
}
```

A reverse mapping from clients to a set of connections (utilised to look up all connections using a client) is stored under a unique prefix per-client:

```typescript
function clientConnectionsPath(clientIdentifier: Identifier): Path {
    return "clients/{clientIdentifier}/connections"
}
```

### Helper functions

`addConnectionToClient` is used to add a connection identifier to the set of connections associated with a client.

```typescript
function addConnectionToClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier))
    conns.add(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier), conns)
}
```

Helper functions are defined by the connection to pass the `CommitmentPrefix` associated with the connection to the verification function
provided by the client. In the other parts of the specifications, these functions MUST be used for introspecting other chains' state,
instead of directly calling the verification functions on the client.

```typescript
function verifyClientConsensusState(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: Height,
  consensusState: ConsensusState) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyClientConsensusState(connection, height, connection.counterpartyPrefix, proof, clientIdentifier, consensusStateHeight, consensusState)
}

function verifyConnectionState(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyConnectionState(connection, height, connection.counterpartyPrefix, proof, connectionIdentifier, connectionEnd)
}

function verifyChannelState(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyChannelState(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, channelEnd)
}

function verifyPacketData(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: Height,
  data: bytes) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketData(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, data)
}

function verifyPacketAcknowledgement(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketAcknowledgement(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, acknowledgement)
}

function verifyPacketReceiptAbsence(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketReceiptAbsence(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier)
}

function verifyNextSequenceRecv(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyNextSequenceRecv(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, nextSequenceRecv)
}

function getTimestampAtHeight(
  connection: ConnectionEnd,
  height: Height) {
    client = queryClient(connection.clientIdentifier)
    return client.queryConsensusState(height).getTimestamp()
}
```

### Sub-protocols

This ICS defines the opening handshake subprotocol. Once opened, connections cannot be closed and identifiers cannot be reallocated (this prevents packet replay or authorisation confusion).

Header tracking and misbehaviour detection are defined in [ICS 2](../ics-002-client-semantics).

![State Machine Diagram](state.png)

#### Identifier validation

Connections are stored under a unique `Identifier` prefix.
The validation function `validateConnectionIdentifier` MAY be provided.

```typescript
type validateConnectionIdentifier = (id: Identifier) => boolean
```

If not provided, the default `validateConnectionIdentifier` function will always return `true`. 

#### Versioning

During the handshake process, two ends of a connection come to agreement on a version bytestring associated
with that connection. At the moment, the contents of this version bytestring are opaque to the IBC core protocol.
In the future, it might be used to indicate what kinds of channels can utilise the connection in question, or
what encoding formats channel-related datagrams will use. At present, host state machine MAY utilise the version data
to negotiate encodings, priorities, or connection-specific metadata related to custom logic on top of IBC.

Host state machines MAY also safely ignore the version data or specify an empty string. It is assumed that the two chains running the opening handshake have at least one compatible version in common (i.e., the compatible versions of the two chains must have a non-empty intersection). If the two chains do not have any mutually acceptable versions, the handshake will fail.

An implementation MUST define a function `getCompatibleVersions` which returns the list of versions it supports, ranked by descending preference order.

```typescript
type getCompatibleVersions = () => []string
```

An implementation MUST define a function `pickVersion` to choose a version from a list of versions. Note that if the two chains performing the handshake implement different `pickVersion` functions, a (possibly misbehaving) relayer may be able to stall the handshake by executing `INIT` and `OPENTRY` on both chains, at which point they will pick different versions and be unable to continue.

```typescript
type pickVersion = ([]string) => string
```

#### Opening Handshake

The opening handshake sub-protocol serves to initialise consensus states for two chains on each other.

The opening handshake defines four datagrams: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, and *ConnOpenConfirm*.

A correct protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram          | Chain acted upon | Prior state (A, B) | Posterior state (A, B) |
| --------- | ----------------- | ---------------- | ------------------ | ---------------------- |
| Actor     | `ConnOpenInit`    | A                | (none, none)       | (INIT, none)           |
| Relayer   | `ConnOpenTry`     | B                | (INIT, none)       | (INIT, TRYOPEN)        |
| Relayer   | `ConnOpenAck`     | A                | (INIT, TRYOPEN)    | (OPEN, TRYOPEN)        |
| Relayer   | `ConnOpenConfirm` | B                | (OPEN, TRYOPEN)    | (OPEN, OPEN)           |

At the end of an opening handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain has each other's correct consensus state as originally specified by the initiating actor.
- Each chain has knowledge of and has agreed to its identifier on the other chain.

This sub-protocol need not be permissioned, modulo anti-spam measures.

Chains MUST implement a function `generateIdentifier` which chooses an identifier, e.g. by incrementing a counter:

```typescript
type generateIdentifier = () -> Identifier
```

A specific version can optionally be passed as `version` to ensure that the handshake will either complete with that version or fail.

*ConnOpenInit* initialises a connection attempt on chain A.

```typescript
function connOpenInit(
  identifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  clientIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier,
  version: string) {
    abortTransactionUnless(validateConnectionIdentifier(identifier))
    abortTransactionUnless(provableStore.get(connectionPath(identifier)) == null)
    state = INIT
    if version != "" {
      // manually selected version must be one we can support
      abortTransactionUnless(getCompatibleVersions().indexOf(version) > -1)
      versions = [version]
    } else {
      versions = getCompatibleVersions()
    }
    connection = ConnectionEnd{state, "", counterpartyPrefix,
      clientIdentifier, counterpartyClientIdentifier, versions}
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenTry* relays notice of a connection attempt on chain A to chain B (this code is executed on chain B).

```typescript
function connOpenTry(
  desiredIdentifier: Identifier,
  counterpartyChosenConnectionIdentifer: Identifier,
  counterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  counterpartyClientIdentifier: Identifier,
  clientIdentifier: Identifier,
  counterpartyVersions: string[],
  proofInit: CommitmentProof,
  proofConsensus: CommitmentProof,
  proofHeight: Height,
  consensusHeight: Height) {
    abortTransactionUnless(validateConnectionIdentifier(desiredIdentifier))
    abortTransactionUnless(consensusHeight < getCurrentHeight())
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{INIT, "", getCommitmentPrefix(), counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersions}
    previous = provableStore.get(connectionPath(desiredIdentifier))
    // use the provided identifier only if the handshake can progress with it
    if ((previous === null) ||
        !(previous.state === INIT &&
          previous.counterpartyConnectionIdentifier === counterpartyConnectionIdentifier &&
          previous.counterpartyPrefix === counterpartyPrefix &&
          previous.clientIdentifier === clientIdentifier &&
          previous.counterpartyClientIdentifier === counterpartyClientIdentifier)) {
      identifier = generateIdentifier()
    } else {
      identifier = desiredIdentifier
    }
    versionsIntersection = intersection(counterpartyVersions, previous !== null ? previous.version : getCompatibleVersions())
    version = pickVersion(versionsIntersection) // throws if there is no intersection
    connection = ConnectionEnd{TRYOPEN, counterpartyConnectionIdentifier, counterpartyPrefix,
                               clientIdentifier, counterpartyClientIdentifier, version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofInit, counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenAck* relays acceptance of a connection open attempt from chain B back to chain A (this code is executed on chain A).

```typescript
function connOpenAck(
  identifier: Identifier,
  version: string,
  counterpartyIdentifier: Identifier,
  proofTry: CommitmentProof,
  proofConsensus: CommitmentProof,
  proofHeight: Height,
  consensusHeight: Height) {
    abortTransactionUnless(consensusHeight < getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(
        (connection.state === INIT && connection.version.indexOf(version) !== -1)
        || (connection.state === TRYOPEN && connection.version === version))
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{TRYOPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier, connection.clientIdentifier,
                             version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofTry, counterpartyIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, connection.counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
    connection.state = OPEN
    connection.version = version
    connection.counterpartyConnectionIdentifier = counterpartyIdentifier
    provableStore.set(connectionPath(identifier), connection)
}
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains (this code is executed on chain B).

```typescript
function connOpenConfirm(
  identifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: Height) {
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === TRYOPEN)
    expected = ConnectionEnd{OPEN, identifier, getCommitmentPrefix(), connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofAck, connection.counterpartyConnectionIdentifier, expected))
    connection.state = OPEN
    provableStore.set(connectionPath(identifier), connection)
}
```

#### Querying

Connections can be queried by identifier with `queryConnection`.

```typescript
function queryConnection(id: Identifier): ConnectionEnd | void {
    return provableStore.get(connectionPath(id))
}
```

Connections associated with a particular client can be queried by client identifier with `queryClientConnections`.

```typescript
function queryClientConnections(id: Identifier): Set<Identifier> {
    return privateStore.get(clientConnectionsPath(id))
}
```

### Properties & Invariants

- Connection identifiers are first-come-first-serve: once a connection has been negotiated, a unique identifier pair exists between two chains.
- The connection handshake cannot be man-in-the-middled by another blockchain's IBC handler.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

A future version of this ICS will include version negotiation in the opening handshake. Once a connection has been established and a version negotiated, future version updates can be negotiated per ICS 6.

The consensus state can only be updated as allowed by the `updateConsensusState` function defined by the consensus protocol chosen when the connection is established.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Parts of this document were inspired by the [previous IBC specification](https://github.com/cosmos/cosmos-sdk/tree/master/docs/spec/ibc).

Mar 29, 2019 - Initial draft version submitted

May 17, 2019 - Draft finalised

Jul 29, 2019 - Revisions to track connection set associated with client

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
