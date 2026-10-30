# IBC docs

The published documentation site. `ibc/` is written here by hand; `ibc/spec/` is
generated from `spec/` at the repo root, and `ibc-go/` is imported. See
[AGENTS.md](AGENTS.md) before editing.

## Introduction

- [What is IBC](ibc/01-introduction/01-what-is-ibc.mdx): A neutral, open source interoperability standard, its three layers, and why to use it.

## How IBC works

Learn about the protocol.

- [Overview](ibc/02-how-ibc-works/01-overview.mdx): The components, and the packet they pass.
- [Packets and applications](ibc/02-how-ibc-works/02-packets-and-applications.mdx): What moves, and what gives it meaning.
- [Core: router and store](ibc/02-how-ibc-works/03-core-router-and-store.mdx): The entry point, and the provable record.
- [Clients and counterparties](ibc/02-how-ibc-works/04-clients-and-counterparties.mdx): How one chain verifies another.
- [Relayer](ibc/02-how-ibc-works/05-relayer.mdx): What decides when packets move.
- [Packet lifecycle](ibc/02-how-ibc-works/06-packet-lifecycle.mdx): Delivered and acknowledged, or timed out.

## Applications

Learn about IBC applications.

- [GMP: how it works](ibc/03-applications/01-gmp.mdx): Contract calls on another chain.
- [Make a cross-chain GMP call](ibc/05-ibc-cli/03-guides/03-make-a-cross-chain-gmp-call.mdx): Call a contract across chains.
- [IFT: how it works](ibc/03-applications/02-ift.mdx): A token that burns here and mints there.

## Light clients

How a chain decides what to believe.

- [The attestation light client](ibc/04-light-clients/01-attestation-light-client.mdx): Accepts what a quorum of attestors signs.
- [Attestors](ibc/04-light-clients/02-attestors.mdx): The services that sign, and the keys trusted.
- [The Besu QBFT and IBFT 2.0 light client](ibc/04-light-clients/03-besu-light-client.mdx): Verify sealed Besu headers and storage proofs.

## Security

- [Security audits](ibc/07-security/01-audits.mdx): Every independent audit, across the protocol and its Go, Solidity, and Solana implementations.

## IBC-solidity contracts

Solidity contract references.

- [Overview](ibc/06-ibc-solidity-contracts/01-overview.mdx): The contracts, and who deploys each.
- [ICS26Router](ibc/06-ibc-solidity-contracts/02-ics26-router.mdx): Entry points, storage, events, roles.
- [ICS27GMP and accounts](ibc/06-ibc-solidity-contracts/03-ics27-gmp-and-accounts.mdx): GMP and its per-sender accounts.
- [IFT contracts](ibc/06-ibc-solidity-contracts/04-ift-contracts.mdx): The base contract and its variants.
- [AttestationLightClient](ibc/06-ibc-solidity-contracts/05-attestation-light-client.mdx): What it verifies, and how.
- [BesuLightClient](ibc/06-ibc-solidity-contracts/07-besu-light-client.mdx): What it verifies, and how.
- [Permissions and upgrades](ibc/06-ibc-solidity-contracts/06-permissions-and-upgrades.mdx): Roles, proxies, and what is fixed.

## IBC CLI

IBC CLI is an all-in-one binary for deploying and running an IBC connection.

- [Overview](ibc/05-ibc-cli/01-overview.mdx): CLI architecture and its parts.

### Tutorial

- [Deploy IBC and send a token](ibc/05-ibc-cli/02-tutorial/01-deploy-ibc-and-send-a-token.mdx): Deploy IBC on two Besu chains and send a token.

### Run the services

- [Run a standalone attestor](ibc/05-ibc-cli/03-guides/01-run-a-standalone-attestor.mdx): An attestor in its own process.
- [Run a standalone relayer](ibc/05-ibc-cli/03-guides/02-run-a-standalone-relayer.mdx): Connect a relayer to an existing connection.

### Guides

- [Make a cross-chain GMP call](ibc/05-ibc-cli/03-guides/03-make-a-cross-chain-gmp-call.mdx): Call a contract across chains.
- [Upgrade an IBC contract](ibc/05-ibc-cli/03-guides/04-upgrade-the-router-contract.mdx): Contract upgrade procedure using the router as an example.
- [Grant, change, and revoke a role](ibc/05-ibc-cli/03-guides/05-grant-change-and-revoke-a-role.mdx): Restrict relaying, rotate the signer, and revoke the old key's role.
- [Rotate an attestor set](ibc/05-ibc-cli/03-guides/06-rotate-an-attestor-set.mdx): Replace an attestor by deploying a new client and migrating.

### Reference

- [Configuration](ibc/05-ibc-cli/04-reference/01-configuration.mdx): Every key in `ibc.yml`.
- [CLI commands](ibc/05-ibc-cli/04-reference/02-cli-commands.mdx): Every command and flag.
- [API](ibc/05-ibc-cli/04-reference/03-api.mdx): The gRPC services.
- [Observability and metrics](ibc/05-ibc-cli/04-reference/04-observability-and-metrics.mdx): Metric modes, names, labels, and semantics.

## Generated and imported

Not edited here. Both are published on the site alongside the pages above.

- [`ibc/spec/`](ibc/spec): 42 pages generated from `spec/` at the repo root by `scripts/specgen.py`. Edit the source, then regenerate.
- [`ibc-go/`](ibc-go): 236 pages imported for the Go implementation, in `latest/` and the frozen `v10.1.x/`.
