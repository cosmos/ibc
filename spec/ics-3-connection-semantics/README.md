---
ics: 3
title: IBC Connection Semantics
status: Proposal
category: IBC
author: Juwoon Yun <joon@tendermint.com> Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
---

## Abstract

The basis of IBC is the ability to verify in the on-chain consensus ruleset of chain `B` that a data packet received on chain `B` was correctly generated on chain `A`. This establishes a cross-chain linearity guarantee: upon validation of that packet on chain `B` we know that the packet has been executed on chain `A` and any associated logic resolved (such as assets being escrowed), and we can safely perform application logic on chain `B` (such as generating vouchers on chain `B` for the chain `A` assets which can later be redeemed with a packet in the opposite direction).

In this proposal, we introduce the requirements of IBC for blockchains. Cosmos-SDK based blockchains will automatically satisfy the requirements with the help of IBC module. Tendermint based blockchains will satisfy most of the `Block` requirements, but have to implement a state machine for the protocol. The concrete implementation on the SDK for the state machine will be documented in another proposal. 

## Definitions

* Chain `S` is the source blockchain from which the IBC packet is sent
* Chain `D` is the destination blockchain on which the IBC packet is received
* `B_h` is the signed block(header) of chain `H` at height `h`

## Block

`Block` is `(p Maybe<Block>, v func(Block) bool, c func(ChainID) Maybe<Connection>)` where `p` is parent block, `v` is lightclient verifier and `c` is registered connections. The other parts of the protocol, such as connections and channels, are defined over `Block`s, so if a blockchain wants to establish an IBC communication with another, it is required to satisfy the properties of `Block`.

Definitions:

1. Direct children have their parent as `p` and parents verifies direct children.
2. Descendants are either direct child or descendants of direct child
3. Ancestors are either direct parent or ancestors of direct parent
4. Height of a block is the step of referring `p` required to get to `nil`

Requirements:

1. Blocks have only one direct child
If a blockchain has deterministic safety(as opposed to probablistic safety in Nakamoto consensus), then there cannot be more than one child for each block. In tendermint, this assumption breakes when +1/3 validators are malicious, producing multiple blocks those are direct child of a single block and all of them are verifiable with the parent. It makes conflicting packets delivered from the failed chain, so for the chains who are receiving from this chain, **fraud proof** mechanism should be applied in this case. However he failed chain itself also need to be recovered and reconnected again. It will be covered in **byzantine recovery strategies**.

2. Blocks have one direct child
There is at least one direct child for all blocks, meaning that the lightclient logic can proceed the blocks one by one even in the worst case. If it not satisfied then there can be a point where the lightclient stops and cannot proceed, which halts IBC connection unexpectedly.

3. If a block verifies another block then it is a descendant of the block
Lightclient should not verify packets which is not in its chain. If the verifier returns true for a block that is not a descendent, it simply means that there is an error in the lightclient logic.

4. Block can have a state only if it is a valid transition from the parent's(see the next paragraph)
Block cannot have a state which is not a transition from its parent. Since the state is only defined for IBC logic, this only means that each block are running IBC-compatible application logic(does not care about the other modules' failure, including the handlers for the packets).

These requirements allows channels work safely without concerning about double spending attack. 
If a block is submitted to the chain(and optionally passed some challange period for fraud proof), then it is assured that the packet is finalized so the application logic can process it.

## Verifier

Verifiers will be covered datailed in **lightclient specification**

### Tendermint

For Tendermint consensus algorithm, each of the blocks has additional parameter `C` which is a subset of the consensus ruleset signed on the block. Verifiers returns true if the difference between the `C_current` and `C_verified` are less then `1/3`.
 
### Tendermint + Cosmos BPoS

Cosmos BPoS defines an unbonding period, where the validators/delegators can be slashed after they declared to unbond. For this, or any other algorithms who requires explicit real-time limit that the lightclient proof cannot skip over, we can extend the verifier to have additional constant `P` which makes the returns false for every block(excepts for the direct child) generated after `P`. This also means that `Block` have to be extended to include timestamp.
// XXX: is it true? does unbonding period works for all blocks, or all blocks except for the direct child?

### Nakamoto + Finality Gadget

Nakamoto chain prefers liveness over safety, so the blocks does not satisfy the first requirement. It can be solved by introducing finality gadgets, which checkpoints some blocks and guarantees that the blocks before it have been finalized. However it needs additional semantics for the difference between the generation and the finalization of a block, so a block cannot be verified unless the block has generated and also finalized.
// XXX: do we need to distinguish generation and finalization for the blocks? I think we don't, since the requirement 1 already implies the blocks are finalized.

## Connection

`Connection` is `(b []Block, c func(PortID) Channel)` where `B` is submitted headers from the other chain and `c` is a map from `PortID` to `Channel`. 

Requirements:
1. Connection can be registered only for an empty `ChainID`
If a `ChainID` is not allocated to any of the connections, new connection can be registered for that. This ensures that once a connection is registered, the `ChainID` is unique to identify only that chain.

2. Connection can be updated if there exists a block registered who verifies the new block
If a new block is submitted to the chain, it verifies and includes the block.

3. Connection cannot refer a block that is referring it or its descendant, directly nor indirectly
This guarantees that there is not referring loop. In other words, future blocks cannot be registered

4. Transition can happen multiple times in a block

// TODO: add connection closing
// Should we allow the ChainIDs reusable once the connections are closed?

These requirements simply describes how connection works in the state. The most important one is the first, because `ChainID`s are unique and immutable, application logics can identify the sender/receiver of the packet with the `ChainID`. The second implies that header updating follows the lightclient logic.

## Implementation

