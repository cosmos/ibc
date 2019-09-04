---
ics: 3
title: Connection Semantics
stage: draft
category: IBC/TAO
requires: 2, 24
required-by: 4, 25
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together facilitate cross-chain sub-state verification and packet association (through channels). Protocols for safely establishing a connection between two chains and cleanly closing a connection are described.

### Motivation

The core IBC protocol provides *authorisation* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction specified in this standard, in conjunction with the *client* abstraction specified in [ICS 2](../ics-002-client-semantics), defines the *authorisation* semantics of IBC. Ordering semantics are described in [ICS 4](../ics-004-channel-and-packet-semantics)).

### Definitions

Client-related types & functions are as defined in [ICS 2](../ics-002-client-semantics).

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
- The connection should be able to be voluntarily & cleanly closed by either blockchain.
- The connection should be able to be immediately closed upon discovery of a consensus misbehaviour.

## Technical Specification

### Data Structures

This ICS defines the `ConnectionState` and `ConnectionEnd` types:

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
  CLOSED,
}
```

```typescript
interface ConnectionEnd {
  state: ConnectionState
  counterpartyConnectionIdentifier: Identifier
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string
}
```

- The `state` field describes the current state of the connection end.
- The `counterpartyConnectionIdentifier` field identifies the connection end on the counterparty chain associated with this connection.
- The `clientIdentifier` field identifies the client associated with this connection.
- The `counterpartyClientIdentifier` field identifies the client on the counterparty chain associated with this connection.
- The `version` field is an opaque string which can be utilised to determine encodings or protocols for channels or packets utilising this connection.

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

`removeConnectionFromClient` is used to remove a connection identifier from the set of connections associated with a client.

```typescript
function removeConnectionFromClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier, connectionIdentifier))
    conns.remove(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier, connectionIdentifier), conns)
}
```

### Versioning

During the handshake process, two ends of a connection come to agreement on a version bytestring associated
with that connection. At the moment, the contents of this version bytestring are opaque to the IBC core protocol.
In the future, it might be used to indicate what kinds of channels can utilise the connection in question, or
what encoding formats channel-related datagrams will use. At present, host state machine MAY utilise the version data
to negotiate encodings, priorities, or connection-specific metadata related to custom logic on top of IBC.

Host state machines MAY also safely ignore the version data or specify an empty string.

### Sub-protocols

This ICS defines two sub-protocols: opening handshake and closing handshake. Header tracking and closing-by-misbehaviour are defined in [ICS 2](../ics-002-client-semantics). Datagrams defined herein are handled as external messages by the IBC relayer module defined in [ICS 26](../ics-026-relayer-module).

![State Machine Diagram](state.png)

#### Opening Handshake

The opening handshake sub-protocol serves to initialise consensus states for two chains on each other.

The opening handshake defines four datagrams: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, and *ConnOpenConfirm*.

A correct protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram          | Chain acted upon | Prior state (A, B) | Post state (A, B) |
| --------- | ----------------- | ---------------- | ------------------ | ----------------- |
| Actor     | `ConnOpenInit`    | A                | (none, none)       | (INIT, none)      |
| Relayer   | `ConnOpenTry`     | B                | (INIT, none)       | (INIT, TRYOPEN)   |
| Relayer   | `ConnOpenAck`     | A                | (INIT, TRYOPEN)    | (OPEN, TRYOPEN)   |
| Relayer   | `ConnOpenConfirm` | B                | (OPEN, TRYOPEN)    | (OPEN, OPEN)      |

At the end of an opening handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain has each other's correct consensus state as originally specified by the initiating actor.
- Each chain has knowledge of and has agreed to its identifier on the other chain.

This sub-protocol need not be permissioned, modulo anti-spam measures.

*ConnOpenInit* initialises a connection attempt on chain A.

```typescript
function connOpenInit(
  identifier: Identifier,
  desiredCounterpartyConnectionIdentifier: Identifier,
  clientIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier,
  version: string) {
    abortTransactionUnless(provableStore.get(connectionPath(identifier)) == null)
    state = INIT
    connection = ConnectionEnd{state, desiredCounterpartyConnectionIdentifier, clientIdentifier,
      counterpartyClientIdentifier, version}
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenTry* relays notice of a connection attempt on chain A to chain B.

```typescript
function connOpenTry(
  desiredIdentifier: Identifier,
  counterpartyConnectionIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier,
  clientIdentifier: Identifier,
  version: string,
  counterpartyVersion: string
  proofInit: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    client = queryClient(connection.clientIdentifier)
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{INIT, desiredIdentifier, counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersion}
    abortTransactionUnless(
      client.verifyMembership(proofHeight, proofInit,
                              connectionPath(counterpartyConnectionIdentifier), expected))
    abortTransactionUnless(
      client.verifyMembership(proofHeight, proofInit,
                              consensusStatePath(counterpartyClientIdentifier),
                              expectedConsensusState))
    abortTransactionUnless(provableStore.get(connectionPath(desiredIdentifier)) === null)
    identifier = desiredIdentifier
    state = TRYOPEN
    connection = ConnectionEnd{state, counterpartyConnectionIdentifier, clientIdentifier,
                               counterpartyClientIdentifier, version}
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenAck* relays acceptance of a connection open attempt from chain B back to chain A.

```typescript
function connOpenAck(
  identifier: Identifier,
  version: string,
  proofTry: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === INIT)
    client = queryClient(connection.clientIdentifier)
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{TRYOPEN, identifier, connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, version}
    abortTransactionUnless(
      client.verifyMembership(proofHeight, proofTry,
                              connectionPath(connection.counterpartyConnectionIdentifier), expected))
    abortTransactionUnless(
      client.verifyMembership(proofHeight, proofTry,
                              consensusStatePath(connection.counterpartyClientIdentifier), expectedConsensusState))
    connection.state = OPEN
    connection.version = version
    provableStore.set(connectionPath(identifier), connection)
}
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains.

```typescript
function connOpenConfirm(
  identifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === TRYOPEN)
    expected = ConnectionEnd{OPEN, identifier, connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    client = queryClient(connection.clientIdentifier)
    abortTransactionUnless(
      client.verifyMembership(proofHeight, proofAck,
                              connectionPath(connection.counterpartyConnectionIdentifier), expected))
    connection.state = OPEN
    provableStore.set(connectionPath(identifier), connection)
}
```

#### Header Tracking

Headers are tracked at the client level. See [ICS 2](../ics-002-client-semantics).

#### Closing Handshake

The closing handshake protocol serves to cleanly close a connection on two chains.

This sub-protocol will likely need to be permissioned to an entity who "owns" the connection on the initiating chain, such as a particular end user, smart contract, or governance mechanism.

The closing handshake sub-protocol defines two datagrams: *ConnCloseInit*, and *ConnCloseConfirm*.

A correct protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram            | Chain acted upon | Prior state (A, B) | Post state (A, B) |
| --------- | ------------------- | ---------------- | ------------------ | ----------------- |
| Actor     | `ConnCloseInit`     | A                | (ANY, ANY)         | (CLOSED, ANY)     |
| Relayer   | `ConnCloseConfirm`  | B                | (CLOSED, ANY)      | (CLOSED, CLOSED)  |

*ConnCloseInit* initialises a close attempt on chain A. It will succeed only if the associated connection does not have any associated open channels.

Once closed, connections cannot be reopened.

```typescript
function connCloseInit(identifier: Identifier) {
    abortTransactionUnless(queryConnectionChannels(identifier).size() === 0)
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state !== CLOSED)
    connection.state = CLOSED
    provableStore.set(connectionPath(identifier), connection)
}
```

*ConnCloseConfirm* relays the intent to close a connection from chain A to chain B. It will succeed only if the associated connection does not have any channels.

Once closed, connections cannot be reopened.

```typescript
function connCloseConfirm(
  identifier: Identifier,
  proofInit: CommitmentProof,
  proofHeight: uint64) {
    abortTransactionUnless(queryConnectionChannels(identifier).size() === 0)
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state !== CLOSED)
    client = queryClient(connection.clientIdentifier)
    expected = ConnectionEnd{CLOSED, identifier, connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    abortTransactionUnless(client.verifyMembership(proofHeight, proofInit, connectionPath(counterpartyConnectionIdentifier), expected))
    connection.state = CLOSED
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

29 March 2019 - Initial draft version submitted
17 May 2019 - Draft finalised
29 July 2019 - Revisions to track connection set associated with client

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
