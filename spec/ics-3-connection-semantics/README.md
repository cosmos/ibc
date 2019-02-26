---
ics: 3
title: Connection Semantics
stage: Proposal
category: ibc-core
author: Juwoon Yun <joon@tendermint.com> Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-03-06
---

## Abstract

The basis of IBC is the ability to verify in the on-chain consensus ruleset of chain `B` that a data packet received on chain `B` was correctly generated on chain `A`. This establishes a cross-chain linearity guarantee: upon validation of that packet on chain `B` we know that the packet has been executed on chain `A` and any associated logic resolved (such as assets being escrowed), and we can safely perform application logic on chain `B` (such as generating vouchers on chain `B` for the chain `A` assets which can later be redeemed with a packet in the opposite direction).

## Specification

### Motivation

### Desired Properties

`Connection` is `[]Block` where `B` is submitted headers from the other chain and `c` is a map from `PortID` to `Channel`. 

1. Connection can be registered only for an empty `ChainID`
If a `ChainID` is not allocated to any of the connections, new connection can be registered for that. This ensures that once a connection is registered, the `ChainID` is unique to identify only that chain.

2. Connection can be updated if the new block can be verified by any of the already registered block
If a new block is submitted to the chain, it verifies and includes the block.

// XXX: add connection closing
// Should we allow the ChainIDs reusable once the connections are closed?

// XXX: should ChainID be (practically) infinite(e.g. bytes32)?


### Technical Specification

// XXX: add connection handshaking
// XXX: add broadcasting/unidirectional/bidirectional connection

// XXX: add packets for ibc connection module
// XXX: should we send handshake message in packet format?

// XXX: add handshake handling logic

### Example Implementation

### Other Implementations

* Cosmos-SDK: [](https://github.com/cosmos/cosmos-sdk/docs/spec/ibc)

### History

March 6th 2019: Initial ICS 3 draft finished and submitted as a PR
