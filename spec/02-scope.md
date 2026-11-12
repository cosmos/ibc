<!-- SPDX-License-Identifier: Community-Spec-1.0 -->

# Scope

The Scope of this Working Group is the development of the Interchain Standards (ICS): the specifications of the Inter-Blockchain Communication protocol (IBC). IBC lets independent state machines, such as blockchains, exchange authenticated and ordered data packets, with each verifying the other's state through an on-chain light client of its counterparty.

## In Scope

The Scope covers what an implementation must do to interoperate with other IBC implementations: protocol behaviour, state machine transitions, data structures, stored state and its commitment paths, message, packet and acknowledgement encodings, and the interfaces between components. It applies to every category defined in [ICS 1](ics-001-ics-standard/README.md):

- **Transport, authentication and ordering (IBC/TAO):** client semantics, connection semantics, channel and packet semantics, port allocation, vector commitments, host requirements, the handler interface, routing and application callbacks, and multi-hop messaging (ICS 2, 3, 4, 5, 23, 24, 25, 26 and 33).
- **Light clients (IBC/TAO):** light client specifications implementing the client semantics of ICS 2. Each defines how a host tracks a counterparty and verifies its headers, misbehaviour and state proofs (ICS 6, 7, 8, 9 and 10).
- **Relaying (IBC/TAO):** the relayer algorithms, and the requirements they place on hosts (ICS 18).
- **Applications (IBC/APP):** packet data formats, acknowledgement formats and processing semantics for applications and middleware built on IBC. These are fungible token transfer (ICS 20), interchain accounts and general message passing (ICS 27), cross-chain validation (ICS 28), relayer fee payment (ICS 29), middleware (ICS 30), cross-chain queries (ICS 31), atomic swap (ICS 100) and non-fungible token transfer (ICS 721).
- **Meta:** the standardisation process, the ICS format and the specification templates (ICS 1, [PROCESS.md](PROCESS.md) and [ics-template.md](ics-template.md)).

This includes IBC v1, IBC v2 and later versions of the protocol. It also includes new Interchain Standards in these categories, at any stage defined in [PROCESS.md](PROCESS.md), and deprecated ones.

## Out of Scope

The following are not in Scope, even where a specification references, depends on or verifies them:

- **Consensus.** The consensus protocols, block formats and validator-set rules of the state machines IBC connects, such as CometBFT, GRANDPA or QBFT. A light client specification covers the verification a host performs, not the counterparty's consensus. The one exception is the validator-set updates between provider and consumer chains that ICS 28 itself specifies.
- **Cryptography.** Cryptographic primitives such as signature schemes and hash functions. This also covers concrete commitment constructions, such as IAVL trees or Merkle Patricia tries, beyond the interface ICS 23 defines.
- **Execution environments.** Virtual machines and smart contract platforms that specifications run in or target, such as the Wasm runtime ICS 8 loads clients into, or the EVM.
- **Host application logic.** Host logic an IBC application invokes but does not specify, such as a contract called through general message passing or the transactions an interchain account executes.
- **Off-chain operation.** Network transport, peer-to-peer networking, and the policies an operator chooses when running a relayer, such as which packets to relay and when, beyond the algorithms ICS 18 specifies.
- **Implementations.** Source code implementing the specifications, including the software in this repository outside `spec/`. Code in a specification is licensed as set out in [04-license.md](04-license.md).
- **External standards.** Interchain Standards maintained outside this repository, such as ICS 32 (Interchain Queries).

Any changes of Scope are not retroactive.
