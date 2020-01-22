# Interchain Standards

![banner](./assets/interchain-standards-image.jpg)

## Synopsis

This repository is the canonical location for development and documentation of inter-chain standards utilised by the Cosmos network & interchain ecosystem.

It shall be used to consolidate design rationale, protocol semantics, and encoding descriptions for the inter-blockchain protocol (IBC), including both the core transport, authentication, & ordering layer (IBC/TAO) and the application layers describing packet encoding & processing semantics (IBC/APP).

Contributions are welcome.

The rendered, ordered set of all interchain standards written so far can be read as [a single PDF](./spec.pdf).

## Ecosystem

For a list of IBC implementations, IBC-supporting blockchains, and special IBC bridges, see [ECOSYSTEM.md](./ECOSYSTEM.md).

To add your project to this list, submit a pull request.

Learn more about the [IBC Ecosystem Working Group](./ecosystem/README.md).

Check out a list of [frequently asked questions](./ibc/6_IBC_FAQ.md).

## Standardisation

Please see [ICS 1](spec/ics-001-ics-standard) for a description of what a standard entails.

To propose a new standard, [open an issue](https://github.com/cosmos/ics/issues/new).

To start a new standardisation document, copy the [template](spec/ics-template.md) and open a PR.

See [PROCESS.md](PROCESS.md) for a description of the standardisation process.

Learn more about the [IBC Steering Committee](./org/steering/README.md).

## IBC Quick References

If you are planning to review inter-blockchain communication protocol specifications, the following are required reading:

-   [IBC Terminology](./ibc/1_IBC_TERMINOLOGY.md)
-   [IBC Architecture](./ibc/2_IBC_ARCHITECTURE.md)
-   [IBC Design Principles](./ibc/3_IBC_DESIGN_PRINCIPLES.md)
-   [IBC Usecases](./ibc/4_IBC_USECASES.md)
-   [IBC Design Patterns](./ibc/5_IBC_DESIGN_PATTERNS.md)

Translated versions of some of these documents can be found in the [translation](./translation) folder.

## Interchain Standards

All standards at or past the "Draft" stage are listed here in order of their ICS numbers, sorted by category.

### Meta

| Interchain Standard Number     | Kind | Standard Title             | Stage |
| ------------------------------ | ---- | -------------------------- | ----- |
| [1](spec/ics-001-ics-standard) | Meta | ICS Specification Standard | Draft |

### IBC/TAO

| Interchain Standard Number                     | Kind           | Standard Title             | Stage |
| ---------------------------------------------- | -------------- | -------------------------- | ----- |
| [2](spec/ics-002-client-semantics)             | Interface      | Client Semantics           | Draft |
| [3](spec/ics-003-connection-semantics)         | Instantiation  | Connection Semantics       | Draft |
| [4](spec/ics-004-channel-and-packet-semantics) | Instantiation  | Channel & Packet Semantics | Draft |
| [5](spec/ics-005-port-allocation)              | Interface      | Port Allocation            | Draft |
| [6](spec/ics-006-solo-machine-client)          | Instantiation  | Solo Machine Client        | Draft |
| [7](spec/ics-007-tendermint-client)            | Instantiation  | Tendermint Client          | Draft |
| [9](spec/ics-009-loopback-client)              | Instantiation  | Loopback Client            | Draft |
| [18](spec/ics-018-relayer-algorithms)          | Interface      | Relayer Algorithms         | Draft |
| [23](spec/ics-023-vector-commitments)          | Interface      | Vector Commitments         | Draft |
| [24](spec/ics-024-host-requirements)           | Interface      | Host Requirements          | Draft |
| [25](spec/ics-025-handler-interface)           | Interface      | Handler Interface          | Draft |
| [26](spec/ics-026-routing-module)              | Interface      | Routing Module             | Draft |

### IBC/APP

| Interchain Standard Number                 | Kind           | Standard Title          | Stage |
| ------------------------------------------ | -------------- |------------------------ | ----- |
| [20](spec/ics-020-fungible-token-transfer) | Instantiation  | Fungible Token Transfer | Draft |
| [27](spec/ics-027-interchain-accounts)     | Instantiation  | Interchain Accounts     | Draft |

## Standard Dependency Visualisation

Directed arrows indicate a dependency relationship (that origin depends on destination).

![deps](assets/deps.png)
