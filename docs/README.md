# IBC docs

## Introduction

- [What is IBC](1-introduction/1-what-is-ibc.md): Packets between chains, verified by on-chain light clients.

## How IBC works

Learn about the protocol.

- [Overview](2-how-ibc-works/1-overview.md): The components, and the packet they pass.
- [Packets and applications](2-how-ibc-works/2-packets-and-applications.md): What moves, and what gives it meaning.
- [Core: router and store](2-how-ibc-works/3-core-router-and-store.md): The entry point, and the provable record.
- [Clients and counterparties](2-how-ibc-works/4-clients-and-counterparties.md): How one chain verifies another.
- [Relayer](2-how-ibc-works/5-relayer.md): What decides when packets move.
- [Packet lifecycle](2-how-ibc-works/6-packet-lifecycle.md): Delivered and acknowledged, or timed out.

## Applications

Learn about IBC applications.

- [GMP: how it works](3-applications/1-gmp.md): Contract calls on another chain.
- [Make a cross-chain GMP call](3-applications/2-make-a-cross-chain-gmp-call.md): Call a contract across chains.
- [IFT: how it works](3-applications/3-ift.md): A token that burns here and mints there.

## Light clients

How a chain decides what to believe.

- [The attestation light client](4-light-clients/1-attestation-light-client.md): Accepts what a quorum of attestors signs.
- [Attestors](4-light-clients/2-attestors.md): The services that sign, and the keys trusted.

## IBC-solidity contracts

Solidity contract references.

- [Overview](5-ibc-solidity-contracts/1-overview.md): The contracts, and who deploys each.
- [ICS26Router](5-ibc-solidity-contracts/2-ics26-router.md): Entry points, storage, events, roles.
- [ICS27GMP and accounts](5-ibc-solidity-contracts/3-ics27-gmp-and-accounts.md): GMP and its per-sender accounts.
- [IFT contracts](5-ibc-solidity-contracts/4-ift-contracts.md): The base contract and its variants.
- [AttestationLightClient](5-ibc-solidity-contracts/5-attestation-light-client.md): What it verifies, and how.
- [Permissions and upgrades](5-ibc-solidity-contracts/6-permissions-and-upgrades.md): Roles, proxies, and what is fixed.

## IBC CLI

IBC CLI is an all-in-one binary for deploying and running an IBC connection. 

- [Overview](6-ibc-cli/1-overview.md): CLI architecture and its parts.

### Guides

- [Deploy IBC and send a token](6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md): Deploy IBC on two chains and send a token.
- [Run a standalone attestor](6-ibc-cli/3-run-a-standalone-attestor.md): An attestor in its own process.
- [Run a standalone relayer](6-ibc-cli/4-run-a-standalone-relayer.md): Connect a relayer to an existing connection.

### Reference

- [Configuration](6-ibc-cli/5-configuration.md): Every key in `ibc.yml`.
- [CLI commands](6-ibc-cli/6-cli-commands.md): Every command and flag.
- [API](6-ibc-cli/7-api.md): The gRPC services.
