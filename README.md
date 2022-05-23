# IBC

![banner](./assets/interchain-standards-image.jpg)

## Synopsis

This repository is the canonical location for development and documentation of the inter-blockchain communication protocol (IBC).

It shall be used to consolidate design rationale, protocol semantics, and encoding descriptions for IBC, including both the core transport, authentication, & ordering layer (IBC/TAO) and the application layers describing packet encoding & processing semantics (IBC/APP).

Contributions are welcome. See [CONTRIBUTING.md](meta/CONTRIBUTING.md) for contribution guidelines.

See [ROADMAP.md](meta/ROADMAP.md) for a public up-to-date version of our roadmap.

## Interchain Standards

All standards at or past the "Draft" stage are listed here in order of their ICS numbers, sorted by category.

### Meta

| Interchain Standard Number               | Standard Title             | Stage |
| ---------------------------------------- | -------------------------- | ----- |
| [1](spec/ics-001-ics-standard/README.md) | ICS Specification Standard | N/A   |

### Core

| Interchain Standard Number                                    | Standard Title             | Stage |
| ------------------------------------------------------------- | -------------------------- | ----- |
| [2](spec/core/ics-002-client-semantics/README.md)             | Client Semantics           | Candidate |
| [3](spec/core/ics-003-connection-semantics/README.md)         | Connection Semantics       | Candidate |
| [4](spec/core/ics-004-channel-and-packet-semantics/README.md) | Channel & Packet Semantics | Candidate |
| [5](spec/core/ics-005-port-allocation/README.md)              | Port Allocation            | Candidate |
| [23](spec/core/ics-023-vector-commitments/README.md)          | Vector Commitments         | Candidate |
| [24](spec/core/ics-024-host-requirements/README.md)           | Host Requirements          | Candidate |
| [25](spec/core/ics-025-handler-interface/README.md)           | Handler Interface          | Candidate |
| [26](spec/core/ics-026-routing-module/README.md)              | Routing Module             | Candidate |

### Client

| Interchain Standard Number                                      | Standard Title             | Stage |
| --------------------------------------------------------------- | -------------------------- | ----- |
| [6](spec/client/ics-006-solo-machine-client/README.md)          | Solo Machine Client        | Candidate |
| [7](spec/client/ics-007-tendermint-client/README.md)            | Tendermint Client          | Candidate |
| [8](spec/client/ics-008-wasm-client/README.md)                  | Wasm Client                | Draft |
| [9](spec/client/ics-009-loopback-client/README.md)              | Loopback Client            | Candidate |
| [10](spec/client/ics-010-grandpa-client/README.md)              | GRANDPA Client             | Draft |

### Relayer

| Interchain Standard Number                                       | Standard Title             | Stage |
| ---------------------------------------------------------------- | -------------------------- | ----- |
| [18](spec/relayer/ics-018-relayer-algorithms/README.md)          | Relayer Algorithms         | Candidate |

### App

| Interchain Standard Number                               | Standard Title          | Stage |
| -------------------------------------------------------- | ----------------------- | ----- |
| [20](spec/app/ics-020-fungible-token-transfer/README.md) | Fungible Token Transfer | Candidate |
| [27](spec/app/ics-027-interchain-accounts/README.md)     | Interchain Accounts     | Draft |
| [29](spec/app/ics-029-fee-payment) | General Relayer Incentivisation Mechanism | Candidate |
| [30](spec/app/ics-030-middleware) | IBC Application Middleware | Candidate |
