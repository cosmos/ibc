# IBC docs 

## Introduction

- [What is IBC](1-introduction/what-is-ibc.md) 
  IBC is a protocol for sending packets between independent chains, where each chain verifies the other chain's writes with an on-chain light client.

## How IBC works

The protocol, concept by concept. Listed alphabetically after the overview, not in reading order.

- [Overview](2-how-ibc-works/overview.md)  
  IBC is a handful of components passing one provable packet between two chains.

- [Packets and applications](2-how-ibc-works/packets-and-applications.md)  
  A packet is what IBC moves between two chains, and an application is what gives its content meaning.

- [Core: router and store](2-how-ibc-works/core-router-and-store.md)  
  The router is the single entry point every packet operation goes through, and the store it holds is the provable record the other chain verifies against.

- [Clients and counterparties](2-how-ibc-works/clients-and-counterparties.md)  
  A client is one chain's verifier for one other chain, and a mirrored pair of clients, each recording the other as its counterparty, is what connects two chains.
  
- [Relayer](2-how-ibc-works/relayer.md)  
  The relayer decides when IBC packets move. The attestors and the light client decide what counts as true.

- [Packet lifecycle](2-how-ibc-works/packet-lifecycle.md)  
  A packet reaches at most one of two provable endings: delivered and acknowledged, or proven timed out.

## Applications

The two applications in scope.

- [GMP: how it works](3-applications/gmp.md)  
  GMP runs arbitrary contract calls on another chain from a deterministic address unique to the caller, and reports back success, failure, or timeout.

- [IFT: how it works](3-applications/ift.md)  
  A fungible token that moves between chains by burning it on one and minting it on the other, controlled by its issuer on every chain.

## Light clients

How a chain decides what to believe about its counterparty.

- [The attestation light client](4-light-clients/attestation-light-client.md)  
  The attestation light client accepts a claim about the counterparty chain once a quorum of a fixed attestor set has signed it.

- [Attestors](4-light-clients/attestors.md)  
  An attestor is a stateless off-chain service that signs statements about one chain's state, and an attestation light client trusts exactly the keys that sign.

## IBC-solidity contracts

Contract-level reference for the same material.

- [Overview](5-ibc-solidity-contracts/overview.md)  
  The Solidity contracts that make up IBC on a chain, what each one is for, and who deploys it.

- [ICS26Router](5-ibc-solidity-contracts/ics26-router.md)  
  Reference for the packet router: its entry points, registries, commitment store, events, errors, and role gates

- [ICS27GMP and accounts](5-ibc-solidity-contracts/ics27-gmp-and-accounts.md)  
  The General Message Passing application, the per-sender account contracts it deploys, and the surface a sender calls.

- [IFT contracts](5-ibc-solidity-contracts/ift-contracts.md)  
  The IFT base contract, its two deployable variants, and the send-call constructors that build a counterparty's mint call.

- [AttestationLightClient](5-ibc-solidity-contracts/attestation-light-client.md)  
  Contract reference for the attestation light client: what it verifies, what the constructor fixes, its functions, wire formats, roles, and errors.

- [Permissions and upgrades](5-ibc-solidity-contracts/permissions-and-upgrades.md)  
  Which roles gate the IBC-solidity contracts, who holds them on a deployment, which contracts sit behind proxies, and which inputs are fixed at construction.
## IBC CLI

The `ibc` binary: deploying IBC onto a chain, and running the relayers and attestors that keep packets moving. Listed in reading order, tutorial first, reference last.

- [Overview](6-ibc-cli/1-overview.md)  
  One tool for deploying IBC and for running the processes that carry packets across it, built from three parts that share one binary and one config file.

- [Deploy IBC and send a token](6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md)  
  Bring up two chains, deploy IBC on both, and move an Interchain Fungible Token from one to the other.

- [Run a standalone attestor](6-ibc-cli/3-run-a-standalone-attestor.md)  
  Move an attestor into a process of its own, against a deployment that already exists, and point a relayer at it.

- [Run a standalone relayer](6-ibc-cli/4-run-a-standalone-relayer.md)  
  Run your own relayer against a connection someone else deployed, including one that finishes packets a stopped relayer left behind.

- [Make a cross-chain GMP call](6-ibc-cli/5-make-a-cross-chain-gmp-call.md)  
  Call a contract on another chain over IBC, and receive the result back as an acknowledgement.

- [Configuration](6-ibc-cli/6-configuration.md)  
  Every key in `ibc.yml`: chains, connections, attestors, signers, and storage.

- [CLI commands](6-ibc-cli/7-cli-commands.md)  
  Every command and flag the binary accepts, grouped the way the binary groups them.

- [API](6-ibc-cli/8-api.md)  
  The relayer and attestor gRPC services, their methods, and their messages.

The tables on the last three pages are generated from this repository. [The tooling README](6-ibc-cli/tools/README.md) says what never to edit by hand.
