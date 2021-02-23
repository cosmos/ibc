# IBC

![banner](./assets/interchain-standards-image.jpg)

## Synopsis

This repository is the canonical location for development and documentation of the inter-blockchain communication protocol (IBC).

It shall be used to consolidate design rationale, protocol semantics, and encoding descriptions for IBC, including both the core transport, authentication, & ordering layer (IBC/TAO) and the application layers describing packet encoding & processing semantics (IBC/APP).

Contributions are welcome.

## Standardisation

Please see [ICS 1](spec/ics-001-ics-standard) for a description of what a standard entails.

To propose a new standard, [open an issue](https://github.com/cosmos/ics/issues/new).

To start a new standardisation document, copy the [template](spec/ics-template.md) and open a PR.

See [PROCESS.md](meta/PROCESS.md) for a description of the standardisation process.

See [STANDARDS_COMMITTEE.md](meta/STANDARDS_COMMITTEE.md) for the membership of the core standardisation committee.

See [CONTRIBUTING.md](meta/CONTRIBUTING.md) for contribution guidelines.

## Interchain Standards

All standards at or past the "Draft" stage are listed here in order of their ICS numbers, sorted by category.

### Meta

| Interchain Standard Number     | Standard Title             | Stage |
| ------------------------------ | -------------------------- | ----- |
| [1](spec/ics-001-ics-standard) | ICS Specification Standard | N/A   |

### Core

| Interchain Standard Number     | Standard Title             | Stage |
| ------------------------------ | -------------------------- | ----- |
| [2](spec/core/ics-002-client-semantics)             | Client Semantics           | Candidate |
| [3](spec/core/ics-003-connection-semantics)         | Connection Semantics       | Candidate |
| [4](spec/core/ics-004-channel-and-packet-semantics) | Channel & Packet Semantics | Candidate |
| [5](spec/core/ics-005-port-allocation)              | Port Allocation            | Candidate |
| [23](spec/core/ics-023-vector-commitments)          | Vector Commitments         | Candidate |
| [24](spec/core/ics-024-host-requirements)           | Host Requirements          | Candidate |
| [25](spec/core/ics-025-handler-interface)           | Handler Interface          | Candidate |
| [26](spec/core/ics-026-routing-module)              | Routing Module             | Candidate |

### Client

| Interchain Standard Number                     | Standard Title             | Stage |
| ---------------------------------------------- | -------------------------- | ----- |
| [6](spec/ics-006-solo-machine-client)          | Solo Machine Client        | Candidate |
| [7](spec/ics-007-tendermint-client)            | Tendermint Client          | Candidate |
| [9](spec/ics-009-loopback-client)              | Loopback Client            | Candidate |
| [10](spec/ics-010-grandpa-client)              | GRANDPA Client             | Draft |

### Relayer

| Interchain Standard Number                     | Standard Title             | Stage |
| ---------------------------------------------- | -------------------------- | ----- |
| [18](spec/ics-018-relayer-algorithms)          | Relayer Algorithms         | Candidate |

### App

| Interchain Standard Number                 | Standard Title          | Stage |
| ------------------------------------------ | ----------------------- | ----- |
| [20](spec/ics-020-fungible-token-transfer) | Fungible Token Transfer | Candidate |
| [27](spec/ics-027-interchain-accounts)     | Interchain Accounts     | Draft |
