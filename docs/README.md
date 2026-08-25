# IBC docs

## Introduction

- [What is IBC](1-introduction/1-what-is-ibc.md)
  Packets between chains, verified by on-chain light clients.

## How IBC works

The protocol, concept by concept, in reading order.

- [Overview](2-how-ibc-works/1-overview.md)
  The components, and the packet they pass.

- [Packets and applications](2-how-ibc-works/2-packets-and-applications.md)
  What moves, and what gives it meaning.

- [Core: router and store](2-how-ibc-works/3-core-router-and-store.md)
  The entry point, and the provable record.

- [Clients and counterparties](2-how-ibc-works/4-clients-and-counterparties.md)
  How one chain verifies another.

- [Relayer](2-how-ibc-works/5-relayer.md)
  What decides when packets move.

- [Packet lifecycle](2-how-ibc-works/6-packet-lifecycle.md)
  Delivered and acknowledged, or timed out.

## Applications

IBC applications.

- [GMP: how it works](3-applications/1-gmp.md)
  Contract calls on another chain.

- [IFT: how it works](3-applications/2-ift.md)
  A token that burns here and mints there.

## Light clients

How a chain decides what to believe.

- [The attestation light client](4-light-clients/1-attestation-light-client.md)
  Accepts what a quorum of attestors signs.

- [Attestors](4-light-clients/2-attestors.md)
  The services that sign, and the keys trusted.

## IBC-solidity contracts

Contract-level reference.

- [Overview](5-ibc-solidity-contracts/1-overview.md)
  The contracts, and who deploys each.

- [ICS26Router](5-ibc-solidity-contracts/2-ics26-router.md)
  Entry points, storage, events, roles.

- [ICS27GMP and accounts](5-ibc-solidity-contracts/3-ics27-gmp-and-accounts.md)
  GMP and its per-sender accounts.

- [IFT contracts](5-ibc-solidity-contracts/4-ift-contracts.md)
  The base contract and its variants.

- [AttestationLightClient](5-ibc-solidity-contracts/5-attestation-light-client.md)
  What it verifies, and how.

- [Permissions and upgrades](5-ibc-solidity-contracts/6-permissions-and-upgrades.md)
  Roles, proxies, and what is fixed.

## IBC CLI

The `ibc` binary: deploying IBC, and running the relayers and attestors that move packets.

- [Overview](6-ibc-cli/1-overview.md)
  The three parts, and how they run.

**Guides**

- [Deploy IBC and send a token](6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md)
  Two chains, IBC deployed, one token moved.

- [Run a standalone attestor](6-ibc-cli/3-run-a-standalone-attestor.md)
  An attestor in its own process.

- [Run a standalone relayer](6-ibc-cli/4-run-a-standalone-relayer.md)
  Your own relayer on an existing connection.

- [Make a cross-chain GMP call](6-ibc-cli/5-make-a-cross-chain-gmp-call.md)
  A contract call across chains.

**Reference**

- [Configuration](6-ibc-cli/6-configuration.md)
  Every key in `ibc.yml`.

- [CLI commands](6-ibc-cli/7-cli-commands.md)
  Every command and flag.

- [API](6-ibc-cli/8-api.md)
  The gRPC services.

The tables on the last three pages are generated from this repository. [The tooling README](6-ibc-cli/tools/README.md) says what never to edit by hand.
