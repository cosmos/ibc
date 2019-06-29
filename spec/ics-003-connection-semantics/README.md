---
ics: 3
title: Connection Semantics
stage: draft
category: ibc-core
requires: 2, 23, 24
required-by: 4, 25
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-05-17
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together faciliate cross-chain substate verification and packet association (through channels). Protocols for safely establishing a connection between two chains and cleanly closing a connection are described.

### Motivation

The core IBC protocol provides *authorization* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction specified in this standard, in conjunction with the *client* abstraction specified in [ICS 2](../ics-002-consensus-verification), defines the *authorization* semantics of IBC. Ordering semantics are described in [ICS 4](../ics-004-channel-and-packet-semantics)).

### Definitions

`ConsensusState`, `Header`, and `updateConsensusState` are as defined in [ICS 2](../ics-002-consensus-verification).

`CommitmentProof`, `verifyMembership`, and `verifyNonMembership` are as defined in [ICS 23](../ics-023-vector-commitments).

`Identifier` and other host state machine requirements are as defined in [ICS 24](../ics-024-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

A *actor*, as referred to in this specification, is an entity capable of executing datagrams who is paying for computation / storage (via gas or a similar mechanism) but is otherwise untrusted. Possible actors include:
- End users signing with an account key
- On-chain smart contracts acting autonomously or in response to another transaction
- On-chain modules acting in response to another transaction or in a scheduled manner

### Desired Properties

- Implementing blockchains should be able to safely allow untrusted actors to open and update connections.

#### Pre-Establishment

Prior to connection establishment:

- No further IBC subprotocols should operate, since cross-chain substates cannot be verified.
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
- The connection should be able to be immediately closed upon discovery of a consensus equivocation.

## Technical Specification

### Data Structures

This ICS defines the `ConnectionState` and `ConnectionEnd` types:

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  CLOSETRY,
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
  nextTimeoutHeight: uint64
}
```

### Store keys

Connection keys are stored under a unique identifier.

```typescript
function connectionKey(id: Identifier): Key {
  return "connections/{id}"
}
```

### Subprotocols

This ICS defines two subprotocols: opening handshake and closing handshake. Header tracking and closing-by-equivocation are defined in [ICS 2](../ics-002-consensus-verification). Datagrams defined herein are handled as external messages by the IBC relayer module defined in [ICS 26](../ics-026-relayer-module).

![State Machine Diagram](state.png)

#### Opening Handshake

The opening handshake subprotocol serves to initialize consensus states for two chains on each other.

The opening handshake defines four datagrams: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, and *ConnOpenConfirm*.

A correct protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram          | Chain acted upon | Prior state (A, B) | Post state (A, B) |
| --------- | ----------------- | ---------------- | ------------------ | ----------------- |
| Actor     | `ConnOpenInit`    | A                | (none, none)       | (INIT, none)      |
| Relayer   | `ConnOpenTry`     | B                | (INIT, none)       | (INIT, TRYOPEN)   |
| Relayer   | `ConnOpenAck`     | A                | (INIT, TRYOPEN)    | (OPEN, TRYOPEN)   |
| Relayer   | `ConnOpenConfirm` | B                | (OPEN, TRYOPEN)    | (OPEN, OPEN)      |

At the end of an opening handshake between two chains implementing the subprotocol, the following properties hold:
- Each chain has each other's correct consensus state as originally specified by the initiating actor.
- Each chain has knowledge of and has agreed to its identifier on the other chain.

This subprotocol need not be permissioned, modulo anti-spam measures.

*ConnOpenInit* initializes a connection attempt on chain A.

```typescript
function connOpenInit(
  identifier: Identifier, desiredCounterpartyConnectionIdentifier: Identifier,
  clientIdentifier: Identifier, counterpartyClientIdentifier: Identifier, nextTimeoutHeight: uint64) {
  assert(get(connectionKey(identifier)) == null)
  state = INIT
  connection = ConnectionEnd{state, desiredCounterpartyConnectionIdentifier, clientIdentifier,
    counterpartyClientIdentifier, nextTimeoutHeight}
  set(connectionKey(identifier), connection)
}
```

*ConnOpenTry* relays notice of a connection attempt on chain A to chain B.

```typescript
function connOpenTry(
  desiredIdentifier: Identifier, counterpartyConnectionIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier, clientIdentifier: Identifier,
  proofInit: CommitmentProof, proofHeight: uint64,
  timeoutHeight: uint64, nextTimeoutHeight: uint64) {
  assert(getConsensusState().getHeight() <= timeoutHeight)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  expectedConsensusState = getConsensusState()
  expected = ConnectionEnd{INIT, desiredIdentifier, counterpartyClientIdentifier, clientIdentifier, timeoutHeight}
  assert(verifyMembership(counterpartyStateRoot, proofInit,
                          connectionKey(counterpartyConnectionIdentifier), expected))
  assert(verifyMembership(counterpartyStateRoot, proofInit,
                          consensusStateKey(counterpartyClientIdentifier),
                          expectedConsensusState))
  assert(get(connectionKey(desiredIdentifier)) === null)
  identifier = desiredIdentifier
  state = TRYOPEN
  connection = ConnectionEnd{state, counterpartyConnectionIdentifier, clientIdentifier,
                             counterpartyClientIdentifier, nextTimeoutHeight}
  set(connectionKey(identifier), connection)
}
```

*ConnOpenAck* relays acceptance of a connection open attempt from chain B back to chain A.

```typescript
function connOpenAck(
  identifier: Identifier, proofTry: CommitmentProof, proofHeight: uint64,
  timeoutHeight: uint64, nextTimeoutHeight: uint64) {
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get(connectionKey(identifier))
  assert(connection.state === INIT)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  expectedConsensusState = getConsensusState()
  expected = ConnectionEnd{TRYOPEN, identifier, connection.counterpartyClientIdentifier,
                           connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(counterpartyStateRoot, proofTry,
                          connectionKey(connection.counterpartyConnectionIdentifier), expected))
  assert(verifyMembership(counterpartyStateRoot, proofTry,
                          consensusStateKey(connection.counterpartyClientIdentifier), expectedConsensusState))
  connection.state = OPEN
  connection.nextTimeoutHeight = nextTimeoutHeight
  set(connectionKey(identifier), connection)
}
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains.

```typescript
function connOpenConfirm(
  identifier: Identifier, proofAck: CommitmentProof,
  proofHeight: uint64, timeoutHeight: uint64)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get(connectionKey(identifier))
  assert(connection.state === TRYOPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  expected = ConnectionEnd{OPEN, identifier, connection.counterpartyClientIdentifier,
                           connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(counterpartyStateRoot, proofAck,
                          connectionKey(connection.counterpartyConnectionIdentifier), expected))
  connection.state = OPEN
  connection.nextTimeoutHeight = 0
  set(connectionKey(identifier), connection)
```

*ConnOpenTimeout* aborts a connection opening attempt due to a timeout on the other side.

```typescript
function connOpenTimeout(
  identifier: Identifier, proofTimeout: CommitmentProof,
  proofHeight: uint64, timeoutHeight: uint64) {
  connection = get(connectionKey(identifier))
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight > connection.nextTimeoutHeight)
  switch state {
    case INIT:
      assert(verifyNonMembership(
        counterpartyStateRoot, proofTimeout,
        connectionKey(connection.counterpartyConnectionIdentifier)))
    case TRYOPEN:
      assert(
        verifyMembership(
          counterpartyStateRoot, proofTimeout,
          connectionKey(connection.counterpartyConnectionIdentifier),
          ConnectionEnd{INIT, identifier, connection.counterpartyClientIdentifier,
                        connection.clientIdentifier, timeoutHeight}
        )
        ||
        verifyNonMembership(
          counterpartyStateRoot, proofTimeout,
          connectionKey(connection.counterpartyConnectionIdentifier)
        )
      )
    case OPEN:
      assert(verifyMembership(
        counterpartyStateRoot, proofTimeout,
        connectionKey(connection.counterpartyConnectionIdentifier),
        ConnectionEnd{TRYOPEN, identifier, connection.counterpartyClientIdentifier,
                      connection.clientIdentifier, timeoutHeight}
      ))
  }
  delete(connectionKey(identifier))
}
```

#### Header Tracking

Headers are tracked at the client level. See [ICS 2](../ics-002-consensus-verification).

#### Closing Handshake

The closing handshake protocol serves to cleanly close a connection on two chains.

This subprotocol will likely need to be permissioned to an entity who "owns" the connection on the initiating chain, such as a particular end user, smart contract, or governance mechanism.

The closing handshake subprotocol defines three datagrams: *ConnCloseInit*, *ConnCloseTry*, and *ConnCloseAck*.

A correct protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram          | Chain acted upon | Prior state (A, B) | Post state (A, B)  |
| --------- | ----------------- | ---------------- | ------------------ | ------------------ |
| Actor     | `ConnCloseInit`   | A                | (OPEN, OPEN)       | (CLOSETRY, OPEN)   |
| Relayer   | `ConnCloseTry`    | B                | (CLOSETRY, OPEN)   | (CLOSETRY, CLOSED) |
| Relayer   | `ConnCloseAck`    | A                | (CLOSETRY, CLOSED) | (CLOSED, CLOSED)   |

*ConnCloseInit* initializes a close attempt on chain A.

```typescript
function connCloseInit(identifier: Identifier, nextTimeoutHeight: uint64) {
  connection = get(connectionKey(identifier))
  assert(connection.state === OPEN)
  connection.state = CLOSETRY
  connection.nextTimeoutHeight = nextTimeoutHeight
  set(connectionKey(identifier), connection)
}
```

*ConnCloseTry* relays the intent to close a connection from chain A to chain B.

```typescript
function connCloseTry(
  identifier: Identifier, proofInit: CommitmentProof, proofHeight: uint64,
  timeoutHeight: uint64, nextTimeoutHeight: uint64) {
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get(connectionKey(identifier))
  assert(connection.state === OPEN)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  expected = ConnectionEnd{CLOSETRY, identifier, connection.counterpartyClientIdentifier,
                           connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(counterpartyStateRoot, proofInit, connectionKey(counterpartyConnectionIdentifier), expected))
  connection.state = CLOSED
  connection.nextTimeoutHeight = nextTimeoutHeight
  set(connectionKey(identifier), connection)
}
```

*ConnCloseAck* acknowledges a connection closure on chain B.

```typescript
function connCloseAck(
  identifier: Identifier, proofTry: CommitmentProof,
  proofHeight: uint64, timeoutHeight: uint64) {
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get(connectionKey(identifier))
  assert(connection.state === CLOSETRY)
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  expected = ConnectionEnd{CLOSED, identifier, connection.counterpartyClientIdentifier,
                           connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(counterpartyStateRoot, proofTry, connectionKey(counterpartyConnectionIdentifier), expected))
  connection.state = CLOSED
  connection.nextTimeoutHeight = 0
  set(connectionKey(identifier), connection)
}
```

*ConnCloseTimeout* aborts a connection closing attempt due to a timeout on the other side and reopens the connection.

```typescript
function connCloseTimeout(
  identifier: Identifier, proofTimeout: CommitmentProof,
  proofHeight: uint64, timeoutHeight: uint64) {
  connection = get(connectionKey(identifier))
  counterpartyStateRoot = get(rootKey(connection.clientIdentifier, proofHeight))
  assert(proofHeight > connection.nextTimeoutHeight)
  switch state {
    case CLOSETRY:
      expected = ConnectionEnd{OPEN, identifier, connection.counterpartyClientIdentifier,
                               connection.clientIdentifier, timeoutHeight}
      assert(verifyMembership(
        counterpartyStateRoot, proofTimeout,
        connectionKey(counterpartyConnectionIdentifier), expected
      ))
      connection.state = OPEN
      connection.nextTimeoutHeight = 0
      set(connectionKey(identifier), connection)
    case CLOSED:
      expected = ConnectionEnd{CLOSETRY, identifier, connection.counterpartyClientIdentifier,
                               connection.clientIdentifier, timeoutHeight}
      assert(verifyMembership(
        counterpartyStateRoot, proofTimeout,
        connectionKey(counterpartyConnectionIdentifier), expected
      ))
      connection.state = OPEN
      connection.nextTimeoutHeight = 0
      set(connectionKey(identifier), connection)
  }
}
```

#### Freezing by Equivocation

The equivocation detection subprotocol is defined in [ICS 2](../ics-002-consensus-verification). If a client is frozen by equivocation, all associated connections are immediately frozen as well.

Implementing chains may want to allow applications to register handlers to take action upon discovery of an equivocation. Further discussion is deferred to ICS 12.

#### Querying

Connections can be queried by identifier with `queryConnection`.

```typescript
function queryConnection(id: Identifier): ConnectionEnd | void {
  return get(connectionKey(id))
}
```

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
17 May 2019 - Draft finalized

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
