---
ics: 3
title: Connection Semantics
stage: draft
category: ibc-core
requires: 2, 6, 10, 23
required-by: 4
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-03-29
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects on two separate chains, each tracking the consensus state of the other chain, facilitating cross-chain substate verification. Protocols for safely establishing a connection between two chains, verifying relayed updates (headers) to the consensus state tracked by a connection, cleanly closing a connection, and closing a connection due to detected equivocation are described.

## Specification

### Motivation

The core IBC protocol provides *authorization* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction, specified in this standard, defines the *authorization* semantics of IBC (ordering semantics are left to [ICS 4: Channel Semantics](../spec/ics-4-channel-sematics)).

### Desired Properties

- Implementing blockchains should be able to safely allow untrusted users to open and update connections.
- The two connecting blockchains should be able to negotiate a shared connection "version" (agreeing on wire encoding format, accumulator proof format, etc.)

#### Pre-Establishment

Prior to connection establishment:

- No further IBC subprotocols should operate, since cross-chain substates cannot be verified.
- The initiating user (who creates the connection) must be able to specify a root-of-trust for the chain to connect to and a root of trust for the connecting chain (implicitly, e.g. by sending the transaction).

#### During Handshake

Once a negotiation handshake has begun:

- Only the appropriate handshake datagrams can be executed in order.
- No chain can masquerade as one of the handshaking chains (formalize...)

#### Post-Establishment

Once a negotiation handshake has completed:

- The created connection objects on both chains contain the roots of trust specified by the initiating user.
- No other connection objects can be maliciously created on other chains by replaying datagrams.
- The connection should be able to be voluntarily & cleanly closed by both blockchains.
- The connection should be able to be immediately closed upon discovery of a consensus equivocation.

### Technical Specification

#### Definitions

This ICS defines the `Connection` type:

![Datatypes](datatypes.png)

`RootOfTrust`, `Header, and `updateRootOfTrust` are as defined in [ICS 2: Consensus Requirements](../spec/ics-2-consensus-requirements).

`AccumulatorProof` and `verify` are as defined in [ICS 23: Cryptographic Accumulator](../spec/ics-23-cryptographic-accumulator).

`Version` and `checkVersion` are as defined in [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning).

`Identifier` is an opaque value used as the key for a connection object; it must serialize to a bytestring. The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers). The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, so the chains could choose to come to agreement on a common identifier (via `chooseIdentifier` and `checkIdentifier`). Further discussion is deferred to [ICS 10: Chain Naming Convention](../spec/ics-10-chain-naming-convention).

A *user*, as referred to in this specification, is an entity capable of executing datagrams who is paying for computation / storage (via gas or a similar mechanism) but is otherwise untrusted. Possible users include:
- End users signing with an account key
- On-chain smart contracts acting autonomously or in response to another transaction
- On-chain modules acting in response to another transaction or in a scheduled manner

#### Requirements

Connection handlers and subsequent protocols make use of a simple key-value store interface provided by the underlying state machine. This store must provide two functions, which behave in the way you would expect:
- `Get(Key) -> Value | null`
- `Set(Key, Value)`

`Key` and `Value` are assumed to be byte slices; encoding details are left to a later ICS.

Blockchains also need the ability to introspect their own root-of-trust (with `getRootOfTrust`) in order to confirm that the connecting chain has stored the correct one.

#### Subprotocols

Subprotocols are defined as a set of datagram types and a `handleDatagram` function which must be implemented by the state machine of the implementing blockchain. Datagrams must be relayed between chains by an external process. This process is assumed to behave in an arbitrary manner — no safety properties are dependent on its behavior, although progress is generally dependent on the existence of at least one correct relayer process. Further discussion is deferred to [ICS 18: Relayer Algorithms](../spec/ics-18-relayer-algorithms).

IBC subprotocols are reasoned about as interactions between two chains `A` and `B` — there is no prior distinction between these two chains and they are assumed to be executing the same, correct IBC protocol. `A` is simply by convention the chain which goes first in the subprotocol and `B` the chain which goes second. Protocol definitions should generally avoid including `A` and `B` in variable names to avoid confusion (as the chains themselves do not know whether they are `A` or `B` in the protocol).

This ICS defines four subprotocols: opening handshake, header tracking, closing handshake, and closing by equivocation.

##### Opening Handshake

The opening handshake subprotocol serves to initialize roots of trust for two chains on each other and negotiate an agreeable connection version.

This subprotocol need not be permissioned, modulo anti-spam measures.

![Opening Handshake](opening_handshake.png)

At the end of an opening handshake between two chains implementing the subprotocol, the following properties hold:
- Each chain has each other's correct root-of-trust as originally specified by the initiating user.
- The chains have agreed to a shared connection version.
- Each chain has knowledge of and has agreed to its identifier on the other chain.

##### Header Tracking

The header tracking subprotocol serves to update the root of trust for an open connection.

This subprotocol need not be permissioned, modulo anti-spam measures.

![Header Tracking](header_tracking.png)

##### Closing Handshake

The closing handshake protocol serves to cleanly close a connection on two chains.

This subprotocol will likely need to be permissioned to an entity who "owns" the connection on the initiating chain, such as a particular user, smart contract, or governance mechanism.

![Closing Handshake](closing_handshake.png)

##### Closing by Equivocation

The equivocation closing subprotocol serves to immediately close a connection if a consensus equivocation is discovered and thus prevent further packet transmission.

![Closing Equivocation](closing_equivocation.png)

Implementing chains may want to allow applications to register handlers to take action upon discovery of an equivocation. Further discussion is deferred to [ICS 12: Byzantine Recovery Strategies](../ics-12-byzantine-recovery-strategies).

### Backwards Compatibility

Not applicable.

### Forwards Compatibility

Once a connection has been established and a version negotiated, future version updates can be negotiated per [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning). The root of trust can only be updated as allowed by the `updateRootOfTrust` function defined by the consensus protocol chosen when the connection is established.

### Example Implementation

Coming soon.

### Other Implementations

Coming soon.

## History

29 March 2019 - Initial draft version submitted

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
