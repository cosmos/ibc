---
ics: 3
title: Connection Semantics
stage: draft
category: ibc-core
requires: 2, 6, 10, 23
required-by: 4
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-05-13
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects on two separate chains, each tracking the consensus state of the other chain, facilitating cross-chain substate verification. Protocols for safely establishing a connection between two chains, verifying relayed updates (headers) to the consensus state tracked by a connection, cleanly closing a connection, and closing a connection due to detected equivocation are described.

### Motivation

The core IBC protocol provides *authorization* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction, specified in this standard, defines the *authorization* semantics of IBC (ordering semantics are left to [ICS 4: Channel Semantics](../spec/ics-4-channel-semantics)).

### Definitions

`ConsensusState`, `Header, and `updateConsensusState` are as defined in [ICS 2: Consensus Requirements](../spec/ics-2-consensus-requirements).

`AccumulatorProof` and `verify` are as defined in [ICS 23: Cryptographic Accumulator](../spec/ics-23-cryptographic-accumulator).

`Version` and `checkVersion` are as defined in [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning).

`Identifier` is as defined in [ICS 24](../spec/ics-24-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

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
- No chain can masquerade as one of the handshaking chains (formalize...)

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
  Version version
  Identifier counterpartyIdentifier
  Identifier lightClientIdentifier
}
```

### Requirements

Host state machine requirements are as defined in [ICS 24](../ics-24-host-requirements).

### Subprotocols

Subprotocols are defined as a set of datagram types and a `handleDatagram` function which must be implemented by the state machine of the implementing blockchain. Datagrams must be relayed between chains by an external process. This process is assumed to behave in an arbitrary manner — no safety properties are dependent on its behavior, although progress is generally dependent on the existence of at least one correct relayer process. Further discussion is deferred to [ICS 18: Relayer Algorithms](../spec/ics-18-relayer-algorithms).

IBC subprotocols are reasoned about as interactions between two chains `A` and `B` — there is no prior distinction between these two chains and they are assumed to be executing the same, correct IBC protocol. `A` is simply by convention the chain which goes first in the subprotocol and `B` the chain which goes second. Protocol definitions should generally avoid including `A` and `B` in variable names to avoid confusion (as the chains themselves do not know whether they are `A` or `B` in the protocol).

This ICS defines four subprotocols: opening handshake, header tracking, closing handshake, and closing by equivocation.

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
  Identifier  lightClientIdentifier
  // Height for timeout of ConnOpenTry datagram
  uint64      nextTimeoutHeight
}
```

```coffeescript
function handleConnOpenInit(identifier, desiredVersion, desiredCounterpartyIdentifier, lightClientIdentifier, nextTimeoutHeight)
  assert(get("connections/{identifier}") == null)
  state = INIT
  set("connections/{identifier}",
    (state, desiredVersion, desiredCounterpartyIdentifier, lightClientIdentifier, nextTimeoutHeight))
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
  Identifier        lightClientIdentifier
  // Proof of stored INIT state on chain A
  AccumulatorProof  proofInit
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
  // Height after which the ConnOpenAck datagram can no longer be executed
  uint64            nextTimeoutHeight
}
```

```coffeescript
function handleConnOpenTry(desiredIdentifier, counterpartyIdentifier, desiredVersion, counterpartyLightClientIdentifier, lightClientIdentifier, proofInit, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  consensusState = get("clients/{lightClientIdentifier}")
  expectedConsensusState = getConsensusState()
  assert(verify(consensusState, proofInit,
    ("connections/{counterpartyIdentifier}", (INIT, desiredVersion, desiredIdentifier, counterpartyLightClientIdentifier, timeoutHeight))))
  assert(verify(consensusState, proofInit,
    ("clients/{counterpartyLightClientIdentifier}", expectedConsensusState)))
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
  AccumulatorProof  proofTry
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
  // Height after which the ConnOpenConfirm datagram can no longer be executed
  uint64            nextTimeoutHeight
}
```

```coffeescript
function handleConnOpenAck(identifier, agreedVersion, proofTry, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, desiredVersion, desiredCounterpartyIdentifier, lightClientIdentifier, _) = get("connections/{identifier}")
  assert(state == INIT)
  consensusState = get("clients/{lightClientIdentifier}")
  expectedConsensusState = getConsensusState()
  assert(verify(consensusState, proofTry,
    ("connections/{desiredCounterpartyIdentifier}", (OPENTRY, agreedVersion, identifier, counterpartyLightClientIdentifier, timeoutHeight))))
  assert(verify(consensusState, proofTry,
    (counterpartyLightClientIdentifier, expectedConsensusState)))
  assert(checkVersion(desiredVersion, agreedVersion))
  state = OPEN
  set("connections/{identifier}",
    (state, agreedVersion, desiredCounterpartyIdentifier, lightClientIdentifier, nextTimeoutHeight))
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains.

```golang
type ConnOpenConfirm struct {
  // Identifier for connection on chain B
  Identifier        identifier
  // Proof of stored OPEN state on chain A
  AccumulatorProof  proofAck
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
}
```

```coffeescript
function handleConnOpenConfirm(identifier, proofAck, timeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, version, counterpartyIdentifier, lightClientIdentifier, _) = get("connections/{identifier}")
  assert(state == OPENTRY)
  consensusState = get("clients/{lightClientIdentifier}")
  expectedConsensusState = getConsensusState()
  assert(verify(consensusState, proofAck,
    ("connections/{counterpartyIdentifier}", (OPEN, version, identifier, counterpartyLightClientIdentifier, timeoutHeight))))
  state = OPEN
  set("connections/{identifier}", (state, version, counterpartyIdentifier, consensusState, 0))
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
  (state, version, counterpartyIdentifier, consensusState, _) = get("connections/{identifier}")
  assert(state == OPEN)
  assert(identifierCounterparty == counterpartyIdentifier)
  state = TRYCLOSE
  set("connections/{identifier}", (state, version, counterpartyIdentifier, consensusState, nextTimeoutHeight))
```

*ConnCloseTry* relays the intent to close a connection from chain A to chain B.

```golang
type ConnCloseTry struct {
  // Identifier of connection
  Identifier        identifier
  // Identifier of connection on counterparty chain
  Identifier        identifierCounterparty
  // Proof of intermediary state on counterparty chain
  AccumulatorProof  proofInit
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
  // Height after which the ConnCloseAck datagram can no longer be executed
  uint64            nextTimeoutHeight
}
```

```coffeescript
function handleConnCloseTry(identifier, identifierCounterparty, proofInit, timeoutHeight, nextTimeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, version, counterpartyIdentifier, consensusState) = get("connections/{identifier}")
  assert(state == OPEN)
  assert(identifierCounterparty == counterpartyIdentifier)
  assert(verify(consensusState, proofInit, ("connections/{counterpartyIdentifier}", TRYCLOSE)))
  state = CLOSED
  set("connections/{identifier}",
    (state, version, counterpartyIdentifier, consensusState, nextTimeoutHeight))
```

*ConnCloseAck* acknowledges a connection closure on chain B.

```golang
type ConnCloseAck struct {
  // Identifier of connection
  Identifier        identifier
  // Proof of intermediary state on counterparty chain
  AccumulatorProof  proofTry
  // Height after which this datagram can no longer be executed
  uint64            timeoutHeight
}
```

```coffeescript
function handleConnCloseAck(identifier, proofTry, timeoutHeight)
  assert(getConsensusState().getHeight() <= timeoutHeight)
  (state, version, counterpartyIdentifier, consensusState, _) = get("connections/{identifier}")
  assert(state == TRYCLOSE)
  assert(verify(consensusState, proofTry, ("connections/{counterpartyIdentifier}", CLOSED)))
  state = CLOSED
  set("connections/{identifier}", (state, version, counterpartyIdentifier, consensusState, 0))
```

#### Closing by Equivocation

The equivocation closing subprotocol is defined in ICS 2. If a client is closed by equivocation, all associated connections are immediately closed as well.

Implementing chains may want to allow applications to register handlers to take action upon discovery of an equivocation. Further discussion is deferred to [ICS 12: Byzantine Recovery Strategies](../ics-12-byzantine-recovery-strategies).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Once a connection has been established and a version negotiated, future version updates can be negotiated per [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning). The consensus state can only be updated as allowed by the `updateConsensusState` function defined by the consensus protocol chosen when the connection is established.

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
