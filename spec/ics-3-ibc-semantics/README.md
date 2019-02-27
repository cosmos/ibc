# ICS3

---
ics: 3
title: IBC Connection Semantics
status: Proposal
category: IBC
author: Juwoon Yun <joon@tendermint.com>
created: 2019-02-25
---

## Abstract

In this proposal, we introduce a concept of abstract blockchain that IBC protocol can work on. Cosmos-SDK based blockchains will automatically satisfy the requirements with the help of IBC module. Tendermint based blockchains will satisfy most of the `Block` requirements, but have to implement a state machine for the protocol. The concrete implementation on the SDK for the state machine will be documented in another proposal. 

`Block` describes a block forming a blockchain. 

## Block

`Block` is `(p Maybe<Block>, f func(Block) bool, s State)` where `p` is parent block, `f` is lightclient verifier and `s` is state. The other parts of the protocol, such as connections and channels, are defined over `Block`s, so if a blockchain wants to establish an IBC communication with another, it is required to satisfy the properties of `Block`.

Definitions:

1. If a block verifies another block and the other block has the block as its parent, the other block is the child of the block.
2. Descendants are either direct child or descendants of direct child
3. Ancestors are either direct parent or ancestors of direct parent

`Block` satisfies the followings:

1. Blocks have one direct child
If a blockchain has liveness, then every block in the blockchain should have its own child. This means the blockchain is an infinite stream of blocks, starting from its genesis. When the blockchain looses its liveness, this assumption is no longer satisfied thus the packet sent to this chain will be lost. 
// timeout is not for this case, since the expiration is defined by the height of receiving chain, so if the receiving chain does not produce more block then timeout does not work. 
 
2. Blocks have only one direct child
If a blockchain has deterministic safety(as opposed to probablistic safety in Nakamoto consensus), then there cannot be more than one child for each block. In tendermint, this assumption breakes when +1/3 validators are malicious, producing multiple blocks those are direct child of a single block and all of them are verifiable with the parent. It makes conflicting packets delivered from the failed chain, so for the chains who are receiving from this chain, **fraud proof** mechanism should be applied in this case. 

3. If a block verifies another block then it is a descendant of the block
Lightclient should not verify packets which is not in its chain. If the verifier returns true for a block that is not a descendent, it simply means that there is an error in the lightclient logic.

4. Blocks cannot validate descentants from a block that they cannot validate
// TODO: is this needed & is this true?

5. Block can have a state only if it is a valid transition from the parent's(see the next paragraph)
Block cannot have a state which is not a transition from its parent. Since the state is only defined for IBC logic, this only means that each block are running IBC-compatible application logic(does not care about the other modules' failure, including the handlers for the packets).

// TODO: define subcategory of verifiers that returns false for every blocks after unbonding period

We can detect and recover from the event where these assumptions are broken from the other chain(throught the timeout and fraud proof logic), but the failed chain itself also need to be recovered and run again. It will be covered in **byzantine recovery strategies**.
 
## State
 
`State` is `c func(ChainID) Maybe<Connection>`. `c` is a map from `ChainID`s to connections. For sake of simplicity, we omit every logic excepts those are directly associated with the connection & channel.

## Connection

// TODO1: extend `b Block` to `b []Block` so we can access on the previous versions
// it is needed if we want to prune the packets before it is received on the other chain(e.g. peggy, udp-style comm)
`Connection` is `(b Block, c func(PortID) Channel)` where `B` is the last known header from the other chain and `c` is a map from `PortID` to `Channel`.  

Requirements:
1. Connection can be registered only for an empty `ChainID`
If a `ChainID` is not allocated to any of the connections, new connection can be registered for that. This ensures that once a connection is registered, the `ChainID` is unique to identify that chain.

2. Connection can be updated if the new block is verifiable with the current referred block
// TODO: allow update on arbitrary height, see TODO1
When a new packet is pushed in the state, the receiving chain should update its connection to verify it. 

// TODO: add connection closing
// Should we allow the ChainIDs reusable once the connections are closed?

// TODO: chains with connection referral & parent referral forms an DAG, so there should not be a loop
// this will enable to add semantics for ordering without introducing global time

## Appendix

### Block Definitions

1. `forall b, b' in B : b.f(b') && b=b'.p <=> child(b, b')`
2. `forall b, b', b'' in B : child(b, b'') || child(b, b') && descendant(b', b'') <=> descendant(b, b'')`
3. `forall b, b', b'' in B : b=b''.p || b=b'.p && ancestor(b'.p, b''.p) <=> ancestor(b, b'')`

### Block Requirements

1. `forall b in B : exists b' in B : child(b, b')`
2. `forall b, b', b'' in B : child(b, b') && child(b, b'') => b'=b''`
3. `forall b, b' in B : b.f(b') => descendant(b, b')`
4. `forall b, b' in B : child(b, b') => transition(b.s, b'.s)`

### Connection Requirements
1. `forall s in S, id in ChainID, b in B : s(id) = nil => transition(s, {id => (b, {_ => nil}), _ => s(_)})`
2. `forall s in S, id in ChainID, b in B : s(id).b.f(b) => transition(s, {id => (b, s(id).q), _ => s(_)})`
