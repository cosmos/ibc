---
ics: 3
title: Connection Semantics
stage: draft
category: ibc-core
requires: 23, 24
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-05-17
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together faciliate cross-chain substate verification and packet association (through channels). Protocols for safely establishing a connection between two chains and cleanly closing a connection are described.

### Motivation

The core IBC protocol provides *authorization* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction specified in this standard, in conjunction with the *client* abstraction specified in ICS 2, defines the *authorization* semantics of IBC. Ordering semantics are described in [ICS 4](../spec/ics-4-channel-packet-semantics)).

### Definitions

`ConsensusState`, `Header`, and `updateConsensusState` are as defined in ICS 2.

`CommitmentProof`, `verifyMembership`, and `verifyNonMembership` are as defined in [ICS 23](../spec/ics-23-vector-commitments).

`Identifier` and other host state machine requirements are as defined in [ICS 24](../spec/ics-24-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

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

This ICS defines the `Connection` type:

```golang
type ConnectionState enum {
  INIT
  TRYOPEN
  CLOSETRY
  OPEN
  CLOSED
}
```

```golang
type Connection struct {
  state                         ConnectionState
  counterpartyIdentifier        Identifier
  clientIdentifier              Identifier
  counterpartyClientIdentifier  Identifier
  nextTimeoutHeight             uint64
}
```

### Subprotocols

This ICS defines two subprotocols: opening handshake and closing handshake. Header tracking and closing-by-equivocation are defined in ICS 2. Datagrams defined herein are handled as external messages by the IBC relayer module defined in ICS 26.

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

```golang
type ConnOpenInit struct {
  identifier                    Identifier
  desiredCounterpartyIdentifier Identifier
  clientIdentifier              Identifier
  counterpartyClientIdentifier  Identifier
  nextTimeoutHeight             uint64
}
```

```coffeescript
function connOpenInit(identifier, desiredCounterpartyIdentifier, clientIdentifier, counterpartyClientIdentifier, nextTimeoutHeight)
  assert(get("connections/{identifier}") == null)
  state = INIT
  connection = Connection{state, desiredCounterpartyIdentifier, clientIdentifier, counterpartyClientIdentifier, nextTimeoutHeight}
  set("connections/{identifier}", connection)
```

*ConnOpenTry* relays notice of a connection attempt on chain A to chain B.

```golang
type ConnOpenTry struct {
  desiredIdentifier             Identifier
  counterpartyIdentifier        Identifier
  counterpartyClientIdentifier  Identifier
  clientIdentifier              Identifier
  proofInit                     CommitmentProof
  timeoutHeight                 uint64
  nextTimeoutHeight             uint64
}
```

```coffeescript
function connOpenTry(desiredIdentifier, counterpartyIdentifier, counterpartyClientIdentifier, clientIdentifier, proofInit, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  consensusState = get("clients/{clientIdentifier}")
  expectedConsensusState = getConsensusState()
  expected = Connection{INIT, desiredIdentifier, counterpartyClientIdentifier, clientIdentifier, timeoutHeight}
  assert(verifyMembership(consensusState.getRoot(), proofInit, "connections/{counterpartyIdentifier}", expected))
  assert(verifyMembership(consensusState.getRoot(), proofInit, "clients/{counterpartyClientIdentifier}", expectedConsensusState))
  assert(get("connections/{desiredIdentifier}") == nil)
  identifier = desiredIdentifier
  state = TRYOPEN
  connection = Connection{state, counterpartyIdentifier, clientIdentifier, counterpartyClientIdentifier, nextTimeoutHeight}
  set("connections/{identifier}", connection)
```

*ConnOpenAck* relays acceptance of a connection open attempt from chain B back to chain A.

```golang
type ConnOpenAck struct {
  identifier        Identifier
  proofTry          CommitmentProof
  timeoutHeight     uint64
  nextTimeoutHeight uint64
}
```

```coffeescript
function connOpenAck(identifier, proofTry, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get("connections/{identifier}")
  assert(connection.state == INIT)
  consensusState = get("clients/{connection.clientIdentifier}")
  expectedConsensusState = getConsensusState()
  expected = Connection{TRYOPEN, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(consensusState, proofTry, "connections/{connection.counterpartyIdentifier}", expected))
  assert(verifyMembership(consensusState, proofTry, "clients/{connection.counterpartyClientIdentifier}", expectedConsensusState))
  connection.state = OPEN
  connection.nextTimeoutHeight = nextTimeoutHeight
  set("connections/{identifier}", connection)
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains.

```golang
type ConnOpenConfirm struct {
  identifier    Identifier
  proofAck      CommitmentProof
  timeoutHeight uint64
}
```

```coffeescript
function connOpenConfirm(identifier, proofAck, timeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get("connections/{identifier}")
  assert(connection.state == TRYOPEN)
  consensusState = get("clients/{connection.clientIdentifier}")
  expected = Connection{OPEN, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(consensusState, proofAck, "connections/{connection.counterpartyIdentifier}", expected))
  connection.state = OPEN
  connection.nextTimeoutHeight = 0
  set("connections/{identifier}", connection)
```

*ConnOpenTimeout* aborts a connection opening attempt due to a timeout on the other side.

```golang
type ConnOpenTimeout struct {
  identifier    Identifier
  proofTimeout  CommitmentProof
  timeoutHeight uint64
}
```

```coffeescript
function connOpenTimeout(identifier, proofTimeout, timeoutHeight)
  connection = get("connections/{identifier}")
  consensusState = get("clients/{connection.clientIdentifier}")
  assert(consensusState.getHeight() > connection.nextTimeoutHeight)
  switch state {
    case INIT:
      assert(verifyNonMembership(consensusState, proofTimeout,
        "connections/{connection.counterpartyIdentifier}"))
    case TRYOPEN:
      assert(
        verifyMembership(consensusState, proofTimeout,
        "connections/{connection.counterpartyIdentifier}",
        Connection{INIT, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
        )
        ||
        verifyNonMembership(consensusState, proofTimeout,
        "connections/{connection.counterpartyIdentifier}")
      )
    case OPEN:
      assert(verifyMembership(consensusState, proofTimeout,
        "connections/{connection.counterpartyIdentifier}",
        Connection{TRYOPEN, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
      ))
  }
  delete("connections/{identifier}")
```

#### Header Tracking

Headers are tracked at the client level. See ICS 2.

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

```golang
type ConnCloseInit struct {
  identifier             Identifier
  nextTimeoutHeight      uint64
}
```

```coffeescript
function connCloseInit(identifier, nextTimeoutHeight)
  connection = get("connections/{identifier}")
  assert(connection.state == OPEN)
  connection.state = CLOSETRY
  connection.nextTimeoutHeight = nextTimeoutHeight
  set("connections/{identifier}", connection)
```

*ConnCloseTry* relays the intent to close a connection from chain A to chain B.

```golang
type ConnCloseTry struct {
  identifier              Identifier
  proofInit               CommitmentProof
  timeoutHeight           uint64
  nextTimeoutHeight       uint64
}
```

```coffeescript
function connCloseTry(identifier, proofInit, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get("connections/{identifier}")
  assert(connection.state == OPEN)
  consensusState = get("clients/{connection.clientIdentifier}")
  expected = Connection{CLOSETRY, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(consensusState, proofInit, "connections/{counterpartyIdentifier}", expected))
  connection.state = CLOSED
  connection.nextTimeoutHeight = nextTimeoutHeight
  set("connections/{identifier}", connection)
```

*ConnCloseAck* acknowledges a connection closure on chain B.

```golang
type ConnCloseAck struct {
  identifier    Identifier
  proofTry      CommitmentProof
  timeoutHeight uint64
}
```

```coffeescript
function connCloseAck(identifier, proofTry, timeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  connection = get("connections/{identifier}")
  assert(connection.state == CLOSETRY)
  consensusState = get("clients/{connection.clientIdentifier}")
  expected = Connection{CLOSED, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
  assert(verifyMembership(consensusState, proofTry, "connections/{counterpartyIdentifier}", expected))
  connection.state = CLOSED
  connection.nextTimeoutHeight = 0
  set("connections/{identifier}", connection)
```

*ConnCloseTimeout* aborts a connection closing attempt due to a timeout on the other side and reopens the connection.

```golang
type ConnCloseTimeout struct {
  identifier    Identifier
  proofTimeout  CommitmentProof
  timeoutHeight uint64
}
```

```coffeescript
function connOpenTimeout(identifier, proofTimeout, timeoutHeight)
  connection = get("connections/{identifier}")
  consensusState = get("clients/{connection/clientIdentifier}")
  assert(consensusState.getHeight() > connection.nextTimeoutHeight)
  switch state {
    case CLOSETRY:
      expected = Connection{OPEN, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
      assert(verifyMembership(consensusState, proofTimeout, "connections/{counterpartyIdentifier}", expected))
      connection.state = OPEN
      connection.nextTimeoutHeight = 0
      set("connections/{identifier}", connection)
    case CLOSED:
      expected = Connection{CLOSETRY, identifier, connection.counterpartyClientIdentifier, connection.clientIdentifier, timeoutHeight}
      assert(verifyMembership(consensusState, proofTimeout, "connections/{counterpartyIdentifier}", expected))
      connection.state = OPEN
      connection.nextTimeoutHeight = 0
      set("connections/{identifier}", connection)
  }
```

#### Freezing by Equivocation

The equivocation detection subprotocol is defined in ICS 2. If a client is frozen by equivocation, all associated connections are immediately frozen as well.

Implementing chains may want to allow applications to register handlers to take action upon discovery of an equivocation. Further discussion is deferred to ICS 12.

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

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
