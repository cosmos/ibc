---
ics: 3
title: Connection Semantics & Lifecycle
stage: proposal
category: ibc-core
requires: 2
required-by: 4
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-03-07
---

## Synopsis

This standards document describes the abstraction of an IBC *connection*: two stateful objects on two separate chains, each tracking the consensus state of the other chain, to enable arbitrary cross-chain substate verification. Protocols for establishing a connection between two chains, verifying relayed updates (headers) to the consensus state tracked by a connection, and voluntarily closing a connection are described.

## Specification

### Motivation

- Connection = cross-chain light client state.
- Between two chains `A` and `B`.
- Permissionless opening / closing / updates.

The basis of IBC is the ability to verify in the on-chain consensus ruleset of chain `B` that a data packet received on chain `B` was correctly generated on chain `A`. This establishes a cross-chain linearity guarantee: upon validation of that packet on chain `B` we know that the packet has been executed on chain `A` and any associated logic resolved (such as assets being escrowed), and we can safely perform application logic on chain `B` (such as generating vouchers on chain `B` for the chain `A` assets which can later be redeemed with a packet in the opposite direction).

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

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

#### Definitions


#### Requirements

Consensus-related primitives are as defined in [ICS 2: Consensus Requirements](../spec/ics-2-consensus-requirements).

Accumulator-related primitives are as defined in [ICS 23: Cryptographic Accumulator](../spec/ics-23-cryptographic-accumulator).

#### Subprotocols

##### Opening Handshake

![Opening Handshake](opening_handshake.png)

##### Tracking Headers

![Tracking Headers](tracking_headers.png)

##### Closing Handshake

Connections may elect to voluntarily close a handshake cleanly on both chains via the following protocol:

![Closing Handshake](closing_handshake.png)

Closing a connection may break application invariants and should only be undertaken in extreme circumstances such as Byzantine behavior of the connected chain.

Closure may be permissioned to an on-chain governance system, an identifiable party on the other chain (such as a signer quorum, although this will not work in some Byzantine cases), or any user who submits an application-specific fraud proof. When a connection is closed, application-specific measures may be undertaken to recover assets held on a Byzantine chain. Further discussion is deferred to [ICS 12: Byzantine Recovery Strategies](../ics-12-byzantine-recovery-strategies).

##### Closing by Equivocation

![Closing Fraud](closing_fraud.png)

### Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

### Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

### Example Implementation

(link to or description of concrete example implementation)

### Other Implementations

(links to or descriptions of other implementations)

## History

(changelog and notable inspirations / references)

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
