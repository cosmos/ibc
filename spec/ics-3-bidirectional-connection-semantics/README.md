---
ics: 3
title: Connection Semantics
stage: draft
category: ibc-core
requires: 2, 6, 10, 23
required-by: 4
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-04-30
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects on two separate chains, each tracking the consensus state of the other chain, facilitating cross-chain substate verification. Protocols for safely establishing a connection between two chains, verifying relayed updates (headers) to the consensus state tracked by a connection, cleanly closing a connection, and closing a connection due to detected equivocation are described.

## Specification

### Motivation

The core IBC protocol provides *authorization* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction, specified in this standard, defines the *authorization* semantics of IBC (ordering semantics are left to [ICS 4: Channel Semantics](../spec/ics-4-channel-semantics)).

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

### Technical Specification

#### Definitions

`ConsensusState`, `Header, and `updateConsensusState` are as defined in [ICS 2: Consensus Requirements](../spec/ics-2-consensus-requirements).

`AccumulatorProof` and `verify` are as defined in [ICS 23: Cryptographic Accumulator](../spec/ics-23-cryptographic-accumulator).

`Version` and `checkVersion` are as defined in [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning).

`Identifier` is as defined in [ICS 24](../spec/ics-24-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

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

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

A *actor*, as referred to in this specification, is an entity capable of executing datagrams who is paying for computation / storage (via gas or a similar mechanism) but is otherwise untrusted. Possible actors include:
- End users signing with an account key
- On-chain smart contracts acting autonomously or in response to another transaction
- On-chain modules acting in response to another transaction or in a scheduled manner

#### Requirements

Host state machine requirements are as defined in [ICS 24](../ics-24-host-requirements).

#### Subprotocols

Subprotocols are defined as a set of datagram types and a `handleDatagram` function which must be implemented by the state machine of the implementing blockchain. Datagrams must be relayed between chains by an external process. This process is assumed to behave in an arbitrary manner — no safety properties are dependent on its behavior, although progress is generally dependent on the existence of at least one correct relayer process. Further discussion is deferred to [ICS 18: Relayer Algorithms](../spec/ics-18-relayer-algorithms).

IBC subprotocols are reasoned about as interactions between two chains `A` and `B` — there is no prior distinction between these two chains and they are assumed to be executing the same, correct IBC protocol. `A` is simply by convention the chain which goes first in the subprotocol and `B` the chain which goes second. Protocol definitions should generally avoid including `A` and `B` in variable names to avoid confusion (as the chains themselves do not know whether they are `A` or `B` in the protocol).

This ICS defines four subprotocols: opening handshake, header tracking, closing handshake, and closing by equivocation.

##### Opening Handshake

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
  Identifier lightClientIdentifier
}
```

```coffeescript
function handleConnOpenInit(identifier, desiredVersion, desiredCounterpartyIdentifier, lightClientIdentifier)
  assert(Get(identifier) == null)
  state = INIT
  Set(identifier, (state, desiredVersion, desiredCounterpartyIdentifier, lightClientIdentifier))
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
}
```

```coffeescript
function handleConnOpenTry(desiredIdentifier, counterpartyIdentifier, desiredVersion, counterpartyLightClientIdentifier, lightClientIdentifier, proofInit)
  consensusState = Get(lightClientIdentifier)
  expectedConsensusState = getConsensusState()
  assert(verify(consensusState, proofInit,
    (counterpartyIdentifier, (INIT, desiredVersion, desiredIdentifier, counterpartyLightClientIdentifier))))
  assert(verify(consensusState, proofInit,
    (counterpartyLightClientIdentifier, expectedConsensusState)))
  assert(get(desiredIdentifier) == nil)
  identifier = desiredIdentifier
  version = chooseVersion(desiredVersion)
  state = OPENTRY
  Set(identifier, (state, version, counterpartyIdentifier, consensusState))
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
}
```

```coffeescript
function handleConnOpenAck(identifier, agreedVersion, proofTry)
  (state, desiredVersion, desiredCounterpartyIdentifier, lightClientIdentifier) = Get(identifier)
  assert(state == INIT)
  consensusState = Get(lightClientIdentifier)
  expectedConsensusState = getConsensusState()
  assert(verify(consensusState, proofTry,
    (desiredCounterpartyIdentifier, (OPENTRY, agreedVersion, identifier, counterpartyLightClientIdentifier))))
  assert(verify(consensusState, proofTry,
    (counterpartyLightClientIdentifier, expectedConsensusState)))
  assert(checkVersion(desiredVersion, agreedVersion))
  state = OPEN
  Set(identifier, (state, agreedVersion, desiredCounterpartyIdentifier, lightClientIdentifier))
```

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains.

```golang
type ConnOpenConfirm struct {
  // Identifier for connection on chain B
  Identifier        identifier
  // Proof of stored OPEN state on chain A
  AccumulatorProof  proofAck
}
```

```coffeescript
function handleConnOpenConfirm(identifier, proofAck)
  (state, version, counterpartyIdentifier, lightClientIdentifier) = Get(identifier)
  assert(state == OPENTRY)
  consensusState = Get(lightClientIdentifier)
  expectedConsensusState = getConsensusState()
  assert(verify(consensusState, proofAck,
    (counterpartyIdentifier, (OPEN, version, identifier, counterpartyLightClientIdentifier))))
  state = OPEN
  Set(identifier, (state, version, counterpartyIdentifier, consensusState))
```

##### Header Tracking

Headers are tracked at the light client level. See [ICS 2](../ics-2-consensus-requirements).

##### Closing Handshake

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
  Identifier identifier
  Identifier identifierCounterparty
}
```

```coffeescript
function handleConnCloseInit(identifier, identifierCounterparty)
  (state, version, counterpartyIdentifier, consensusState) = Get(identifier)
  assert(state == OPEN)
  assert(identifierCounterparty == counterpartyIdentifier)
  state = TRYCLOSE
  Set(identifier, (state, version, counterpartyIdentifier, consensusState))
```

*ConnCloseTry* relays the intent to close a connection from chain A to chain B.

```golang
type ConnCloseTry struct {
  Identifier identifier
  Identifier identifierCounterparty
  AccumulatorProof proofInit
}
```

```coffeescript
function handleConnCloseTry(identifier, identifierCounterparty, proofInit)
  (state, version, counterpartyIdentifier, consensusState) = Get(identifier)
  assert(state == OPEN)
  assert(identifierCounterparty == counterpartyIdentifier)
  assert(verify(consensusState, proofInit, (counterpartyIdentifier, TRYCLOSE)))
  state = CLOSED
  Set(identifier, (state, version, counterpartyIdentifier, consensusState))
```

*ConnCloseAck* acknowledges a connection closure on chain B.

```golang
type ConnCloseAck struct {
  Identifier identifier
  AccumulatorProof proofTry
}
```

```coffeescript
function handleConnCloseAck(identifier, proofTry)
  (state, version, counterpartyIdentifier, consensusState) = Get(identifier)
  assert(state == TRYCLOSE)
  assert(verify(consensusState, proofTry, (counterpartyIdentifier, CLOSED)))
  state = CLOSED
  Set(identifier, (state, version, counterpartyIdentifier, consensusState))
```

##### Closing by Equivocation

The equivocation closing subprotocol is defined in ICS 2. If a client is closed by equivocation, all associated connections are immediately closed as well.

Implementing chains may want to allow applications to register handlers to take action upon discovery of an equivocation. Further discussion is deferred to [ICS 12: Byzantine Recovery Strategies](../ics-12-byzantine-recovery-strategies).

### Backwards Compatibility

Not applicable.

### Forwards Compatibility

Once a connection has been established and a version negotiated, future version updates can be negotiated per [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning). The consensus state can only be updated as allowed by the `updateConsensusState` function defined by the consensus protocol chosen when the connection is established.

### Example Implementation

Coming soon.

### Other Implementations

Coming soon.

## History

Parts of this document were inspired by the [previous IBC specification](https://github.com/cosmos/cosmos-sdk/tree/master/docs/spec/ibc).

29 March 2019 - Initial draft version submitted
30 April 2019 - Draft finalized

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
