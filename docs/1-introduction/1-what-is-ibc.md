---
title: "What is IBC"
description: "IBC is a protocol for sending packets between independent chains, where each chain verifies the other chain's writes with an on-chain light client."
---

The Inter-Blockchain Communication (IBC) Protocol is the open standard for secure interoperability between independent systems. It is a battle-tested, extensible, trust-minimized interoperability protocol that connects 115+ public blockchain networks, plus private networks and consortia. In over five years of production, it has processed $50B+ in transaction volume without an exploit of the supported version. IBC is an open standard that lets digital ledgers exchange assets and data directly with other ledgers, including Ethereum and EVM-based frameworks such as Hyperledger Besu, non-Ethereum ledgers, and other systems that meet a small set of requirements.

Like TCP/IP standardized communication across the internet, IBC standardizes communication across blockchains and other distributed ledgers. It enables two systems to exchange data, assets, and arbitrary messages directly, without relying on a centralized or third-party intermediary.

At its core, IBC is a collection of open specifications that define how systems establish connections, verify messages, and transfer information securely. Because IBC is an open standard, it can be implemented on virtually any system capable of generating verifiable proofs, including blockchains, permissioned ledgers, and other distributed systems.

Today, IBC has implementations across major ecosystems, including Cosmos, EVM chains, and Solana, providing out-of-the-box connectivity to hundreds of networks.

IBC provides a complete interoperability stack:

- Protocol standards for secure cross-system interoperability
- Infrastructure such as relayers that transport messages between connected systems
- Reusable applications for common interoperability use cases, including token transfers and General Message Passing (GMP) for cross-chain contract calls
- Flexible verification mechanisms, ranging from trust-minimized light clients to permissioned attestation-based models

IBC is modular and extensible by design, allowing developers to adopt existing applications and verification systems or build custom modules tailored to their needs.

# Why IBC?

### Direct communication and verification

IBC enables systems to communicate directly with one another through self-hosted infrastructure. The connections between systems are peer-to-peer and do not rely on a third-party intermediary; instead, they directly verify the validity of cross-system messages against the counterparty’s state.  In addition, a digital ledger or system using IBC preserves its security model. This simplifies the risk surface and reduces external dependencies.

### An extensible, open standard

IBC is an open, community-governed interoperability standard that any open-source user can implement, extend, and operate independently.

As an open framework, IBC can be adopted and implemented for any digital ledger or blockchain implementation, ensuring interoperability remains future-proof as new ecosystems and networks emerge and connect through the same standard. There are existing IBC implementations for Ethereum, Hyperledger Besu, Solana, and Cosmos.

### Flexible verification

IBC supports multiple trust models to meet different business and regulatory needs.

- Consensus light client verification provides trust-minimized security by verifying messages directly against signed consensus data.
- Attestation-based verification enables organizations to leverage existing governance processes, compliance frameworks, and first-party key infrastructure.

### No vendor lock-in

Because IBC is an open standard rather than a proprietary network, organizations retain control over their infrastructure, contracts, and security mechanisms while remaining interoperable with other IBC-enabled systems. Unlike other interoperability solutions, IBC imposes no protocol-level platform fees and does not require participants to rely on a specific service provider.