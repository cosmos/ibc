---
ics: 2
title: Consensus Requirements
stage: Proposal
category: ibc-core
author: Juwoon Yun <joon@tendermint.com> Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-03-05
---

The basis of IBC is the ability to verify in the on-chain consensus ruleset of chain `B` that a data packet received on chain `B` was correctly generated on chain `A`. This establishes a cross-chain linearity guarantee: upon validation of that packet on chain `B` we know that the packet has been executed on chain `A` and any associated logic resolved (such as assets being escrowed), and we can safely perform application logic on chain `B` (such as generating vouchers on chain `B` for the chain `A` assets which can later be redeemed with a packet in the opposite direction). 

In order to verify an incoming packet, the blockchain should be able to check 1. whether the block is valid and 2. whether the packet is included in the block or not. In this proposal, we introduce the generalized concept for blockchains, called `Block`. `Block` requires its consensus algirithm to satisfy some properties, including deterministic safety, lightclient compatability and valid transition of state machine. This makes the linearity of the packets guaranteed.

## Specification

### Motivation

### Desired Propoerties

`Block` is `(p Maybe<Block>, v func(Block) bool, s func(ChainID) Maybe<(Connection, func(PortID) Channel)>)` where `p` is parent block, `v` is lightclient verifier and `s` is state. The other parts of the protocol, such as connections and channels, are defined over `Block`s, so if a blockchain wants to establish an IBC communication with another, it is required to satisfy the properties of `Block`. 

Definitions:

1. Direct children have their parent as `p` and are verifiable with their parent.
2. Height of a block is the step of referring `p` required to get to `nil`

Requirements:

1. Blocks have only one direct child
If a blockchain has deterministic safety(as opposed to probablistic safety in Nakamoto consensus), then there cannot be more than one child for each block. In tendermint, this assumption breakes when +1/3 validators are malicious, producing multiple blocks those are direct child of a single block and all of them are verifiable with the parent. It makes conflicting packets delivered from the failed chain, so for the chains who are receiving from this chain, **fraud proof #10** mechanism should be applied in this case. However he failed chain itself also need to be recovered and reconnected again. It will be covered in **byzantine recovery strategies #6**.

2. Blocks have at least one direct child
There is at least one direct child for all blocks, meaning that the lightclient logic can proceed the blocks one by one even in the worst case. If it not satisfied then there can be a point where the lightclient stops and cannot proceed, which halts IBC connection unexpectedly. This also can be violated when the blocks are restarted out of the consensus, for example in Tendermint, (1/2 < validators < 2/3) forked out the chain. This also will be covered in **byzantine recovery strategies**.

3. If a block verifies another block then it is a descendant of the block
Lightclient should not verify packets which is not in its chain. If the verifier returns true for a block that is not a descendent, it simply means that there is an error in the lightclient logic.

4. Block can have a state only if it is a valid transition from the parent's(see the next paragraph)
Block cannot have a state which is not a transition from its parent. Since the state is only defined for IBC logic, this only means that each block are running IBC-compatible application logic(does not care about the other modules' failure, including the handlers for the packets).

These requirements allows channels work safely without concerning about double spending attack. 
If a block is submitted to the chain(and optionally passed some challange period for fraud proof), then it is assured that the packet is finalized so the application logic can process it.


### Technical Specification

Following functions exist over `Block`s.

* `height : Block -> int`
Returns the height of the block.

* `verify : Block -> Block -> bool`
Verifies the latter block is a descendant of the former block, as defined in **lightclient specification**. Can take additional lightclient proof argument.

* `connection : Block -> ChainID -> Connection`
Returns the connection for `ChainID` as defined in **connection semantics**

* `channel : Block -> ChainID -> PortID -> Channel`
Returns the channel for `(ChainID, PortID)` pair, as defined in **channel semantics**

### Implementations

* Cosmos-SDK: [](https://github.com/cosmos/cosmos-sdk/x/ibc)  

## History 

March 5th 2019: Initial ICS 2 draft finished and submitted as a PR

