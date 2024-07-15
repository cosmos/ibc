# IBC

![banner](./assets/interchain-standards.png)

## Synopsis

This repository is the canonical location for development and documentation of the inter-blockchain communication protocol (IBC).

It shall be used to consolidate design rationale, protocol semantics, and encoding descriptions for IBC, including both the core transport, authentication, & ordering layer (IBC/TAO) and the application layers describing packet encoding & processing semantics (IBC/APP).

Contributions are welcome. See [CONTRIBUTING.md](meta/CONTRIBUTING.md) for contribution guidelines.

## What is IBC?

<!-- markdown-link-check-disable-next-line -->
For a high-level explanation of what IBC is and how it works, please read [this blog post](https://medium.com/the-interchain-foundation/eli5-what-is-ibc-def44d7b5b4c).

## Interchain Standards

All standards at or past the "Draft" stage are listed here in order of their ICS numbers, sorted by category.

### Meta

| Interchain Standard Number               | Standard Title             | Stage | Maintainer    |
| ---------------------------------------- | -------------------------- | ----- | ------------- |
| [1](spec/ics-001-ics-standard/README.md) | ICS Specification Standard | N/A   | Protocol team |

### Core Transport 

| Interchain Standard Number                                    | Standard Title             | Stage     | Implementations | Maintainer    |
| ------------------------------------------------------------- | -------------------------- | --------- | --------------- | ------------- |
| [2](spec/core/ics-002-client-semantics/README.md)             | Client Semantics           | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [3](spec/core/ics-003-connection-semantics/README.md)         | Connection Semantics       | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [4](spec/core/ics-004-channel-and-packet-semantics/README.md) | Channel & Packet Semantics | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [5](spec/core/ics-005-port-allocation/README.md)              | Port Allocation            | Candidate | [ibc-go](https://github.com/cosmos/ibc-go) | Protocol team |
| [23](spec/core/ics-023-vector-commitments/README.md)          | Vector Commitments         | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [24](spec/core/ics-024-host-requirements/README.md)           | Host Requirements          | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [25](spec/core/ics-025-handler-interface/README.md)           | Handler Interface          | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [26](spec/core/ics-026-routing-module/README.md)              | Routing Module             | Candidate | [ibc-go](https://github.com/cosmos/ibc-go), [ibc-rs](https://github.com/cosmos/ibc-rs) | Protocol team |
| [33](spec/core/ics-033-multi-hop/README.md)                   | Multi-hop Messaging        | Candidate | [ibc-go](https://github.com/cosmos/ibc-go) | Protocol team |

### Light Clients

| Interchain Standard Number                                      | Standard Title             | Stage | Implementations | Maintainer    |
| --------------------------------------------------------------- | -------------------------- | ----- | --------------- | ------------- |
| [6](spec/client/ics-006-solo-machine-client/README.md)          | Solo Machine Client        | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/light-clients/06-solomachine) | Protocol team |
| [7](spec/client/ics-007-tendermint-client/README.md)            | Tendermint Client          | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/light-clients/07-tendermint), [ibc-rs](https://github.com/cosmos/ibc-rs/tree/main/ibc-clients/ics07-tendermint) | Protocol team |
| [8](spec/client/ics-008-wasm-client/README.md)                  | Wasm Client                | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/light-clients/08-wasm), [ibc-rs](https://github.com/cosmos/ibc-rs/tree/main/ibc-clients/ics08-wasm) | Protocol team / [Composable Finance](https://www.composable.finance) |
| [9](spec/client/ics-009-loopback-cilent/README.md)       | Loopback Client            | Draft | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/light-clients/09-localhost) | Protocol team |
| [10](spec/client/ics-010-grandpa-client/README.md)              | GRANDPA Client             | Draft | | [Octopus Network](https://oct.network) |

### Relayer

| Interchain Standard Number                                       | Standard Title             | Stage | Implementations | Maintainer    |
| ---------------------------------------------------------------- | -------------------------- | ----- | --------------- | ------------- |
| [18](spec/relayer/ics-018-relayer-algorithms/README.md)          | Relayer Algorithms         | Finalized | [go-relayer](https://github.com/cosmos/relayer), [rust-relayer](https://github.com/informalsystems/hermes), [ts-relayer](https://github.com/confio/ts-relayer) | Protocol team |

### App

| Interchain Standard Number                               | Version | Standard Title          | Stage     | Implementations                                                                                                           | Maintainer         |
| -------------------------------------------------------- | ------- | ----------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------ |
| [20](spec/app/ics-020-fungible-token-transfer/README.md) | v2      | Fungible Token Transfer | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/ics20-v2-beta.0/modules/apps/transfer)                                     | Protocol team      |
| [20](spec/app/ics-020-fungible-token-transfer/v1/README.md) | v1      | Fungible Token Transfer | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/apps/transfer), [ibc-rs](https://github.com/cosmos/ibc-rs/tree/main/ibc-apps/ics20-transfer) | Protocol team      |
| [27](spec/app/ics-027-interchain-accounts/README.md)     | v1      | Interchain Accounts     | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/apps/27-interchain-accounts)                                  | Protocol team      | 
| [28](spec/app/ics-028-cross-chain-validation/README.md)  | v1      | Cross-Chain Validation  | Candidate | [interchain-security](https://github.com/cosmos/interchain-security/tree/main)                                            | Cosmos Hub team    |
| [29](spec/app/ics-029-fee-payment)                       | v1      | General Relayer Incentivization Mechanism | Candidate | [ibc-go](https://github.com/cosmos/ibc-go/tree/main/modules/apps/29-fee)                                                  | Protocol team      |
| [30](spec/app/ics-030-middleware)                        | v1      | IBC Application Middleware | N/A      | N/A                                                                                                                       | Protocol team      |
| [31](spec/app/ics-031-crosschain-queries)                | v1      | Cross-Chain Queries     | Draft     | N/A                                                                                                                       | Protocol team      |
| [32](https://github.com/strangelove-ventures/async-icq)  | v1      | Interchain Queries      | Candidate | [async-icq](https://github.com/strangelove-ventures/async-icq)                                                            | [Strangelove Ventures](https://strange.love) |
| [100](spec/app/ics-100-atomic-swap)                      | v1      | Interchain Atomic Swap  | Candidate | [ibcswap](https://github.com/ibcswap/ibcswap)                                                                             | [Side Labs](https://side.one) |
| [721](spec/app/ics-721-nft-transfer)                     | v1      | Non-Fungible Token Transfer | Candidate | [nft-transfer](https://github.com/bianjieai/nft-transfer)                                                                 | [IRIS Network](https://www.irisnet.org) |

## Translations

The Interchain Standards are also translated into the following languages:

- [Chinese](https://github.com/octopus-network/ibc-spec-cn)
