<!-- SPDX-License-Identifier: Community-Spec-1.0 -->

# Scope

This file is the `Scope.md` named in section 9.13 of the [Community Specification License 1.0](01-community-specification-license-v1.md).

The Scope of this Working Group is the Interchain Standards (ICS) listed below: specifications of the Inter-Blockchain Communication protocol (IBC), which lets independent state machines, such as blockchains, exchange authenticated and ordered data packets, with each verifying the other's state through an on-chain light client of its counterparty.

## In Scope

The Scope is the normative requirements stated in the text of the documents below, which are kept in this repository under `spec/`, and of later revisions to those documents. A requirement is in Scope only to the extent the text of one of these documents states it. Each entry covers every document in its directory.

- **IBC v1**
  - **Core**
    - [ICS 2 Client Semantics](core/ics-002-client-semantics/README.md) (`core/ics-002-client-semantics/`)
    - [ICS 3 Connection Semantics](core/ics-003-connection-semantics/README.md) (`core/ics-003-connection-semantics/`)
    - [ICS 4 Channel and Packet Semantics](core/ics-004-channel-and-packet-semantics/README.md), including channel upgrades (`core/ics-004-channel-and-packet-semantics/`)
    - [ICS 5 Port Allocation](core/ics-005-port-allocation/README.md) (`core/ics-005-port-allocation/`)
    - [ICS 23 Vector Commitments](core/ics-023-vector-commitments/README.md) (`core/ics-023-vector-commitments/`)
    - [ICS 24 Host State Machine Requirements](core/ics-024-host-requirements/README.md) (`core/ics-024-host-requirements/`)
    - [ICS 25 Handler Interface](core/ics-025-handler-interface/README.md) (`core/ics-025-handler-interface/`)
    - [ICS 26 Routing Module](core/ics-026-routing-module/README.md), including application upgrade callbacks (`core/ics-026-routing-module/`)
    - [ICS 33 Multi-hop Channel](core/ics-033-multi-hop/README.md) (`core/ics-033-multi-hop/`)
  - **Light clients**
    - [ICS 6 Solo Machine Client](client/ics-006-solo-machine-client/README.md) (`client/ics-006-solo-machine-client/`)
    - [ICS 7 Tendermint Client](client/ics-007-tendermint-client/README.md) (`client/ics-007-tendermint-client/`)
    - [ICS 8 Wasm Client](client/ics-008-wasm-client/README.md) (`client/ics-008-wasm-client/`)
    - [ICS 9 Loopback Client](client/ics-009-loopback-cilent/README.md) (`client/ics-009-loopback-cilent/`)
    - [ICS 10 GRANDPA Client](client/ics-010-grandpa-client/README.md) (`client/ics-010-grandpa-client/`)
  - **Relayer**
    - [ICS 18 Relayer Algorithms](relayer/ics-018-relayer-algorithms/README.md) (`relayer/ics-018-relayer-algorithms/`)
  - **Applications**
    - [ICS 20 Fungible Token Transfer](app/ics-020-fungible-token-transfer/README.md), including the deprecated version 2 (`app/ics-020-fungible-token-transfer/`)
    - [ICS 27 Interchain Accounts](app/ics-027-interchain-accounts/README.md) (`app/ics-027-interchain-accounts/`)
    - [ICS 28 Cross-Chain Validation](app/ics-028-cross-chain-validation/README.md) (`app/ics-028-cross-chain-validation/`)
    - [ICS 29 General Fee Payment](app/ics-029-fee-payment/README.md) (`app/ics-029-fee-payment/`)
    - [ICS 30 IBC Middleware](app/ics-030-middleware/README.md) (`app/ics-030-middleware/`)
    - [ICS 31 Cross-Chain Queries](app/ics-031-crosschain-queries/README.md) (`app/ics-031-crosschain-queries/`)
    - [ICS 100 Atomic Swap](app/ics-100-atomic-swap/README.md) (`app/ics-100-atomic-swap/`)
    - [ICS 721 Non-Fungible Token Transfer](app/ics-721-nft-transfer/README.md) (`app/ics-721-nft-transfer/`)
- **IBC v2**
  - **Core**
    - [ICS 2 Client Semantics](IBC_V2/core/ics-002-client-semantics/README.md) (`IBC_V2/core/ics-002-client-semantics/`)
    - [ICS 4 Packet Semantics](IBC_V2/core/ics-004-packet-semantics/PACKET.md) (`IBC_V2/core/ics-004-packet-semantics/`)
    - [ICS 5 Port Allocation](IBC_V2/core/ics-005-port-allocation/README.md) (`IBC_V2/core/ics-005-port-allocation/`)
    - [ICS 24 Host State Machine Requirements](IBC_V2/core/ics-024-host-requirements/README.md) (`IBC_V2/core/ics-024-host-requirements/`)
    - [ICS 26 Application Callbacks](IBC_V2/core/ics-026-application-callbacks/README.md) (`IBC_V2/core/ics-026-application-callbacks/`)
  - **Applications**
    - [ICS 27 General Message Passing](IBC_V2/app/ics-027-gmp/README.md) (`IBC_V2/app/ics-027-gmp/`)

## Out of Scope

Everything not stated in the text of the documents above is out of Scope, including:

- **Material incorporated by reference.** Algorithms, formats and specifications that a document references, depends on or links to but does not itself state, such as the Tendermint (CometBFT) consensus and light client algorithms ICS 7 relies on, GRANDPA, the `cosmos/ics23` proof format, the CosmWasm contract platform and the Solidity ABI encoding.
- **Consensus, cryptography and execution environments.** The consensus protocols of the state machines IBC connects, cryptographic primitives such as signature schemes and hash functions, state commitment structures such as IAVL trees or Merkle Patricia tries, and virtual machines and smart contract platforms, except as the text of a document above states a requirement about them.
- **Host and off-chain behaviour.** The internal implementation of host modules an application invokes, the effects of the messages an interchain account executes or a general message passing call makes, network transport, and the policies an operator chooses when running a relayer, except as the text of a document above states a requirement about them.
- **Other documents in `spec/`.** The overview [`IBC_V2/README.md`](IBC_V2/README.md), the guide [`client/Rollup-Integration-guide.md`](client/Rollup-Integration-guide.md), [ICS 1](ics-001-ics-standard/README.md) and the process, contribution and template documents ([`PROCESS.md`](PROCESS.md), [`CONTRIBUTING.md`](CONTRIBUTING.md), [`STANDARDS_COMMITTEE.md`](STANDARDS_COMMITTEE.md), [`ics-template.md`](ics-template.md) and `00` to `08`: [`00-contributor-license-agreement.md`](00-contributor-license-agreement.md), [`01-community-specification-license-v1.md`](01-community-specification-license-v1.md), [`02-scope.md`](02-scope.md), [`03-notices.md`](03-notices.md), [`04-license.md`](04-license.md), [`05-governance.md`](05-governance.md), [`06-contributing.md`](06-contributing.md), [`07-spec-template.md`](07-spec-template.md) and [`08-code-of-conduct.md`](08-code-of-conduct.md)). They remain under the Community Specification License, but add no requirement to the Scope.
- **New documents and standards.** A specification or document not listed above, including new Interchain Standards, new protocol versions and standards whose text is not in this repository, such as ICS 32 Interchain Queries. Adding one requires a change of Scope, approved as set out in [05-governance.md](05-governance.md).
- **Implementations.** Source code implementing the specifications, including the software in this repository outside `spec/`.

Any changes of Scope are not retroactive.
