---
ics: 3
title: Connection Semantics
stage: draft
category: ibc-core
requires: 2, 6, 10, 23
required-by: 4, 25, 26
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-05-13
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects on two separate chains, each associated with a light client of the other chain, which faciliate cross-chain substate verification and datagram association. Protocols for safely establishing a connection between two chains, cleanly closing a connection, and closing a connection due to detected equivocation are described.

### Motivation

The core IBC protocol provides *authorization* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction specified in this standard, in conjunction with the *client* abstraction specified in [ICS 2](../spec/ics-2-consensus-verification), defines the *authorization* semantics of IBC. Ordering semantics are described in [ICS 4](../spec/ics-4-channel-packet-semantics)).

### Definitions

`ConsensusState`, `Header, and `updateConsensusState` are as defined in [ICS 2: Consensus Verification](../spec/ics-2-consensus-verification).

`CommitmentProof`, `verifyMembership`, and `verifyNonMembership` are as defined in [ICS 23: Vector Commitments](../spec/ics-23-vector-commitments).

`Version` and `checkVersion` are as defined in [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning).

`Identifier` and other host state machine requirements are as defined in [ICS 24](../spec/ics-24-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

A *actor*, as referred to in this specification, is an entity capable of executing datagrams who is paying for computation / storage (via gas or a similar mechanism) but is otherwise untrusted. Possible actors include:
- End users signing with an account key
- On-chain smart contracts acting autonomously or in response to another transaction
- On-chain modules acting in response to another transaction or in a scheduled manner

### Desired Properties

- Implementing blockchains should be able to safely allow untrusted actors to open and update connections.
- The two connecting blockchains should be able to negotiate a shared connection "version". The version consists of metadata about encoding formats that the chain is required to understand, including wire encoding format, accumulator proof format, etc.

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
  TRYCLOSE
  OPEN
  CLOSED
}
```

```golang
type Connection struct {
  ConnectionState state
  Version         version
  Identifier      counterpartyIdentifier
  Identifier      clientIdentifier
  uint64          nextTimeoutHeight
}
```

### Subprotocols

This ICS defines three subprotocols: opening handshake, header tracking, closing handshake. Datagrams defined herein are handled as external messages by the IBC relayer module defined in [ICS 26](../spec/ics-26-relayer-module).

![State Machine Diagram](state.png)

#### Opening Handshake

The opening handshake subprotocol serves to initialize consensus states for two chains on each other and negotiate an agreeable connection version.

The opening handshake defines four datagrams: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, and *ConnOpenConfirm*.

A correct protocol execution flows as follows:

| Initiator | Datagram          | Chain |
| --------- | ----------------- | ----- |
| Actor     | `ConnOpenInit`    | A     |
| Relayer   | `ConnOpenTry`     | B     |
| Relayer   | `ConnOpenAck`     | A     |
| Relayer   | `ConnOpenConfirm` | B     |

At the end of an opening handshake between two chains implementing the subprotocol, the following properties hold:
- Each chain has each other's correct consensus state as originally specified by the initiating actor.
- The chains have agreed to a shared connection version.
- Each chain has knowledge of and has agreed to its identifier on the other chain.

This subprotocol need not be permissioned, modulo anti-spam measures.

*ConnOpenInit* initializes a connection attempt on chain A.

```golang
type ConnOpenInit struct {
  // Identifier to use for connection on chain A
  Identifier  identifier
  // Desired identifier to use for connection on chain B
  Identifier  desiredCounterpartyIdentifier
  // Desired version for connection
  Version     desiredVersion
  // Light client identifier for chain B
  Identifier  clientIdentifier
  // Height for timeout of ConnOpenTry datagram
  uint64      nextTimeoutHeight
}
```

```coffeescript
function handleConnOpenInit(identifier, desiredVersion, desiredCounterpartyIdentifier, clientIdentifier, nextTimeoutHeight)
  assert(get("connections/{identifier}") == null)
  state = INIT
  set("connections/{identifier}",
    (state, desiredVersion, desiredCounterpartyIdentifier, clientIdentifier, nextTimeoutHeight))
```

*ConnOpenTry* relays notice of a connection attempt on chain A to chain B.

```golang
type ConnOpenTry struct {
  // Desired identifier to use for connection on chain B
  Identifier        desiredIdentifier
  // Identifier for connection on chain A
  Identifier        counterpartyIdentifier
  // Desired version for connection
  Version           desiredVersion
  // Light client identifier for chain B on A
  Identifier        counterpartyLightClientIdentifier
  // Light client identifier for chain A on B
  Identifier        clientIdentifier
  // Proof of stored INIT state on chain A
  CommitmentProof   proofInit
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
  // Height after which the ConnOpenAck datagram can no longer be executed
  uint64            nextTimeoutHeight
}
```

```coffeescript
function handleConnOpenTry(desiredIdentifier, counterpartyIdentifier, desiredVersion, counterpartyLightClientIdentifier, clientIdentifier, proofInit, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  consensusState = get("clients/{clientIdentifier}")
  expectedConsensusState = getConsensusState()
  assert(verifyMembership(consensusState.getRoot(), proofInit,
    "connections/{counterpartyIdentifier}", (INIT, desiredVersion, desiredIdentifier, counterpartyLightClientIdentifier, timeoutHeight)))
  assert(verifyMembership(consensusState.getRoot(), proofInit,
    "clients/{counterpartyLightClientIdentifier}", expectedConsensusState))
  assert(get("connections/{desiredIdentifier}") == nil)
  identifier = desiredIdentifier
  version = chooseVersion(desiredVersion)
  state = OPENTRY
  set("connections/{identifier}",
    (state, version, counterpartyIdentifier, consensusState, nextTimeoutHeight))
```

*ConnOpenAck* relays acceptance of a connection open attempt from chain B back to chain A.

```golang
type ConnOpenAck struct {
  // Identifier for connection on chain A
  Identifier        identifier
  // Agreed version for connection
  Version           agreedVersion
  // Proof of stored TRY state on chain B
  CommitmentProof   proofTry
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
  // Height after which the ConnOpenConfirm datagram can no longer be executed
  uint64            nextTimeoutHeight
}
```

```coffeescript
function handleConnOpenAck(identifier, agreedVersion, proofTry, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, desiredVersion, desiredCounterpartyIdentifier, clientIdentifier, _) = get("connections/{identifier}")
  assert(state == INIT)
  consensusState = get("clients/{clientIdentifier}")
  expectedConsensusState = getConsensusState()
  assert(verifyMembership(consensusState, proofTry,
    "connections/{desiredCounterpartyIdentifier}", (OPENTRY, agreedVersion, identifier, counterpartyLightClientIdentifier, timeoutHeight)))
  assert(verifyMembership(consensusState, proofTry,
    counterpartyLightClientIdentifier, expectedConsensusState))
  assert(checkVersion(desiredVersion, agreedVersion))
  state = OPEN
  set("connections/{identifier}",
    (state, agreedVersion, desiredCounterpartyIdentifier, clientIdentifier, nextTimeoutHeight))
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains.

```golang
type ConnOpenConfirm struct {
  // Identifier for connection on chain B
  Identifier        identifier
  // Proof of stored OPEN state on chain A
  CommitmentProof   proofAck
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
}
```

```coffeescript
function handleConnOpenConfirm(identifier, proofAck, timeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, version, counterpartyIdentifier, clientIdentifier, _) = get("connections/{identifier}")
  assert(state == OPENTRY)
  consensusState = get("clients/{clientIdentifier}")
  expectedConsensusState = getConsensusState()
  assert(verifyMembership(consensusState, proofAck,
    "connections/{counterpartyIdentifier}", (OPEN, version, identifier, counterpartyLightClientIdentifier, timeoutHeight)))
  state = OPEN
  set("connections/{identifier}", (state, version, counterpartyIdentifier, consensusState, 0))
```

*ConnOpenTimeout* aborts a connection opening attempt due to a timeout on the other side.

```golang
type ConnOpenTimeout struct {
  // Identifier for connection on this chain
  Identifier      identifier
  // Proof of not-progressed state past the timeout height
  CommitmentProof proofTimeout
}
```

```coffeescript
function handleConnOpenTimeout(identifier, proofTimeout)
  (state, version, counterpartyIdentifier, clientIdentifier, timeoutHeight) = get("connections/{identifier}")
  consensusState = get("clients/{clientIdentifier}")
  assert(consensusState.getHeight() > timeoutHeight)
  switch state {
    case INIT:
      assert(verifyNonMembership(consensusState, proofTimeout,
        "connections/{counterpartyIdentifier}"))
    case TRYOPEN:
      assert(verifyMembership(consensusState, proofTimeout,
        "connections/{counterpartyIdentifier}", (INIT, version, identifier, _, _)))
    case OPEN:
      assert(verifyMembership(consensusState, proofTimeout,
        "connections/{counterpartyIdentifier}", (TRYOPEN, version, identifier, _, _)))
  }
  delete("connections/{identifier}")
```

#### Header Tracking

Headers are tracked at the light client level. See [ICS 2](../ics-2-consensus-requirements).

#### Closing Handshake

The closing handshake protocol serves to cleanly close a connection on two chains.

This subprotocol will likely need to be permissioned to an entity who "owns" the connection on the initiating chain, such as a particular end user, smart contract, or governance mechanism.

The closing handshake subprotocol defines three datagrams: *ConnCloseInit*, *ConnCloseTry*, and *ConnCloseAck*.

A correct protocol execution flows as follows:

| Initiator | Datagram          | Chain |
| --------- | ----------------- | ----- |
| Actor     | `ConnCloseInit`   | A     |
| Relayer   | `ConnCloseTry`    | B     |
| Relayer   | `ConnCloseAck`    | A     |

*ConnCloseInit* initializes a close attempt on chain A.

```golang
type ConnCloseInit struct {
  // Identifier of connection
  Identifier identifier
  // Identifier of connection on counterparty chain
  Identifier identifierCounterparty
  // Timeout height for ConnCloseTry datagram
  uint64     nextTimeoutHeight
}
```

```coffeescript
function handleConnCloseInit(identifier, identifierCounterparty, nextTimeoutHeight)
  (state, version, counterpartyIdentifier, clientIdentifier, _) = get("connections/{identifier}")
  assert(state == OPEN)
  assert(identifierCounterparty == counterpartyIdentifier)
  state = TRYCLOSE
  set("connections/{identifier}", (state, version, counterpartyIdentifier, clientIdentifier, nextTimeoutHeight))
```

*ConnCloseTry* relays the intent to close a connection from chain A to chain B.

```golang
type ConnCloseTry struct {
  // Identifier of connection
  Identifier        identifier
  // Identifier of connection on counterparty chain
  Identifier        identifierCounterparty
  // Proof of intermediary state on counterparty chain
  CommitmentProof   proofInit
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
  // Height after which the ConnCloseAck datagram can no longer be executed
  uint64            nextTimeoutHeight
}
```

```coffeescript
function handleConnCloseTry(identifier, identifierCounterparty, proofInit, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, version, counterpartyIdentifier, clientIdentifier, _) = get("connections/{identifier}")
  assert(state == OPEN)
  assert(identifierCounterparty == counterpartyIdentifier)
  consensusState = get("clients/{clientIdentifier}")
  assert(verifyMembership(consensusState, proofInit, "connections/{counterpartyIdentifier}",
    (TRYCLOSE, version, identifier, counterpartyLightClientIdentifier, timeoutHeight)))
  state = CLOSED
  set("connections/{identifier}",
    (state, version, counterpartyIdentifier, clientIdentifier, nextTimeoutHeight))
```

*ConnCloseAck* acknowledges a connection closure on chain B.

```golang
type ConnCloseAck struct {
  // Identifier of connection
  Identifier        identifier
  // Proof of intermediary state on counterparty chain
  CommitmentProof   proofTry
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
}
```

```coffeescript
function handleConnCloseAck(identifier, proofTry, timeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, version, counterpartyIdentifier, clientIdentifier, _) = get("connections/{identifier}")
  assert(state == TRYCLOSE)
  consensusState = get("clients/{clientIdentifier}")
  assert(verifyMembership(consensusState, proofTry, "connections/{counterpartyIdentifier}",
    (CLOSED, version, identifier, counterpartyLightClientIdentifier, timeoutHeight)))
  state = CLOSED
  set("connections/{identifier}", (state, version, counterpartyIdentifier, consensusState, 0))
```

*ConnCloseTimeout* aborts a connection closing attempt due to a timeout on the other side and reopens the connection.

```golang
type ConnCloseTimeout struct {
  // Identifier for connection on this chain
  Identifier      identifier
  // Proof of not-progressed state past the timeout height
  CommitmentProof proofTimeout
}
```

```coffeescript
function handleConnOpenTimeout(identifier, proofTimeout)
  (state, version, counterpartyIdentifier, clientIdentifier, timeoutHeight) = get("connections/{identifier}")
  consensusState = get("clients/{clientIdentifier}")
  assert(consensusState.getHeight() > timeoutHeight)
  switch state {
    case TRYCLOSE:
      assert(verifyMembership(consensusState, proofTimeout,
        "connections/{counterpartyIdentifier}", (OPEN, version, identifier, _, _)))
      set("connections/{identifier}", (OPEN, version, counterpartyIdentifier, clientIdentifier, 0))
    case CLOSED:
      assert(verifyMembership(consensusState, proofTimeout,
        "connections/{counterpartyIdentifier}", (TRYCLOSE, version, identifier, _, _)))
      set("connections/{identifier}", (OPEN, version, counterpartyIdentifier, clientIdentifier, 0))
  }
```

#### Freezing by Equivocation

The equivocation detection subprotocol is defined in ICS 2. If a client is frozen by equivocation, all associated connections are immediately frozen as well.

Implementing chains may want to allow applications to register handlers to take action upon discovery of an equivocation. Further discussion is deferred to [ICS 12: Byzantine Recovery Strategies](../ics-12-byzantine-recovery-strategies).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Once a connection has been established and a version negotiated, future version updates can be negotiated per [ICS 6](../spec/ics-6-connection-channel-versioning).
The consensus state can only be updated as allowed by the `updateConsensusState` function defined by the consensus protocol chosen when the connection is established.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Parts of this document were inspired by the [previous IBC specification](https://github.com/cosmos/cosmos-sdk/tree/master/docs/spec/ibc).

29 March 2019 - Initial draft version submitted
13 May 2019 - Draft finalized

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
