# Interchain Standards Development

![banner](./assets/interchain-standards-image.jpg)

## Synopsis

This repository is the canonical location for development and documentation of inter-chain standards utilised by the Cosmos network & interchain ecosystem. Initially it will be used to consolidate design documentation for the inter-blockchain communication protocol (IBC), encoding standards for Cosmos chains, and miscellaneous utilities such as off-chain message signing.

## Standardisation

Please see [ICS 1](spec/ics-001-ics-standard) for a description of what a standard entails.

To propose a new standard, [open an issue](https://github.com/cosmos/ics/issues/new). To start a new standardisation document, copy the [template](spec/ics-template.md) and open a PR.

See [PROCESS.md](PROCESS.md) for a description of the standardisation process.

Quick references & interchain standards can be read as [a single PDF](./spec.pdf).

## IBC Quick References

The subject of most initial interchain standards is the inter-blockchain communication protocol, "IBC".

If you are diving in or planning to review specifications, the following are recommended reading:
- [IBC Architecture](./ibc/1_IBC_ARCHITECTURE.md)
- [IBC Design Principles](./ibc/2_IBC_DESIGN_PRINCIPLES.md)
- [IBC Terminology](./ibc/3_IBC_TERMINOLOGY.md)
- [IBC Usecases](./ibc/4_IBC_USECASES.md)
- [IBC Design Patterns](./ibc/5_IBC_DESIGN_PATTERNS.md)
- [IBC specification progress tracking](https://github.com/cosmos/ics/issues/26)

## Interchain Standards

All standards in the "draft" stage are listed here in order of their ICS numbers, sorted by category.

### Meta

| Interchain Standard Number     | Standard Title             | Stage |
| ------------------------------ | -------------------------- | ----- |
| [1](spec/ics-001-ics-standard) | ICS Specification Standard | Draft |

### IBC (Core)

| Interchain Standard Number                          | Standard Title                     | Stage |
| --------------------------------------------------- | ---------------------------------- | ----- |
| [2](spec/ics-002-consensus-verification)            | Consensus Verification             | Draft |
| [3](spec/ics-003-connection-semantics)              | Connection Semantics               | Draft |
| [4](spec/ics-004-channel-and-packet-semantics)      | Channel & Packet Semantics         | Draft |
| [5](spec/ics-005-port-allocation)                   | Port Allocation                    | Draft |
| [18](spec/ics-018-relayer-algorithms)               | Relayer Algorithms                 | Draft |
| [23](spec/ics-023-vector-commitments)               | Vector Commitments                 | Draft |
| [24](spec/ics-024-host-requirements)                | Host Requirements                  | Draft |
| [25](spec/ics-025-handler-interface)                | Handler Interface                  | Draft |
| [26](spec/ics-026-relayer-module)                   | Relayer Module                     | Draft |

### IBC (Application)

| Interchain Standard Number                 | Standard Title          | Stage |
| ------------------------------------------ | ----------------------- | ----- |
| [20](spec/ics-020-fungible-token-transfer) | Fungible Token Transfer | Draft |

## Standard Dependency Visualisation

Directed arrows indicate a dependency relationship (that origin depends on destination).

![deps](assets/deps.png)
