---
ics: 3
title: Connection Semantics & Lifecycle
stage: draft
category: ibc-core
requires: 2
required-by: 4
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-03-29
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects on two separate chains, each tracking the consensus state of the other chain, facilitating cross-chain substate verification. Protocols for establishing a connection between two chains, verifying relayed updates (headers) to the consensus state tracked by a connection, cleanly closing a connection, and closing a connection due to detected equivocation are described.

## Specification

### Motivation

- Connection = cross-chain light client state.
- Between two chains `A` and `B`.
- Permissionless opening / closing / updates.

### Desired Properties

- Permissionless channel opening / channel closing / channel updates.

#### Pre-Establishment

- Guarantees that no packets can be committed on other connections?
- No required a priori root-of-trust knowledge
- Only one connection "between" two chains

#### During Handshake

Once a negotiation handshake has *begun* (defined as the first packet being committed):

- Only the appropriate handshake packets can be committed in order
- No chain can masquerade as one of the handshaking chains (formalize...)

#### Post-Establishment

- Connection provides verification of relayed packets
- No packets other than committed by consensus of *A* / *B* on connection *C* can be relayed

### Technical Specification

#### Definitions

Consensus-related primitives are as defined in [ICS 2: Consensus Requirements](../spec/ics-2-consensus-requirements).

Accumulator-related primitives are as defined in [ICS 23: Cryptographic Accumulator](../spec/ics-23-cryptographic-accumulator).

#### Requirements

Connection handlers and subsequent protocols reference a simple key-value store interface provided by the underlying state machine. This store must provide three functions, which behave in the way you would expect:
- `Get(Key) -> Maybe Value`
- `Set(Key, Value)`
- `Has(Key) -> Bool`

`Key` and `Value` are assumed to be byte slices; encoding details are left to a later ICS.

#### Subprotocols

Subprotocols are defined as a set of datagram types and a `handleDatagram` function operating on the connection state. Datagrams must be relayed between chains by an external process. This process is assumed to behave in an arbitrary manner — no safety properties are dependent on its behavior, although progress is generally dependent on the existence of at least one correct relayer process. Further discussion is deferred to [ICS 18: Off-Chain Relayer Algorithm](../spec/ics-18-offchain-relayer-algorithm).

IBC subprotocols are reasoned about as interactions between two chains `A` and `B` — there is no prior distinction between these two chains and they are assumed to be executing the same, correct IBC protocol. `A` is simply by convention the chain which goes first in the subprotocol and `B` the chain which goes second.

This ICS defines four subprotocols: opening handshake, header tracking, closing handshake, and closing by equivocation.

##### Opening Handshake

The opening handshake subprotocol serves to initialize roots of trust for two chains on each other and negotiate an agreeable connection version.

Generally, this subprotocol need not be permissioned (any user can start the protocol with `CONNOPENINIT`), modulo anti-spam measures.

![Opening Handshake](opening_handshake.png)

##### Header Tracking

The header tracking subprotocol serves to update the root of trust for an open connection.

This subprotocol need not be permissioned, modulo anti-spam measures.

![Tracking Headers](tracking_headers.png)

##### Closing Handshake

The closing handshake protocol serves to cleanly close a connection on two chains.

This subprotocol will likely need to be permissioned to an entity who "owns" the connection on the initiating chain, such as a particular user, smart contract, or governance mechanism.

![Closing Handshake](closing_handshake.png)

##### Closing by Equivocation

![Closing Fraud](closing_fraud.png)

Further discussion is deferred to [ICS 12: Byzantine Recovery Strategies](../ics-12-byzantine-recovery-strategies).

### Backwards Compatibility

Not applicable.

### Forwards Compatibility

Once a connection has been established and a version negotiated, future version updates can be negotiated per [ICS 6: Connection & Channel Versioning](../spec/ics-6-connection-channel-versioning). The root of trust can only be updated per the `updateRootOfTrust` function defined by the consensus protocol chosen when the connection is established.

### Example Implementation

Coming soon.

### Other Implementations

Coming soon.

## History

29 March 2019 - Initial draft version submitted

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
