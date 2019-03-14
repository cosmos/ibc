---
ics: 2
title: Consensus Requirements
stage: proposal
category: ibc-core
author: Juwoon Yun <joon@tendermint.com> Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-03-05
---

The IBC protocol creates a mechanism by which two replicated fault-tolerant state machines may pass messages to each other. These messages provide a base layer for the creation of communicating blockchain architecture that overcomes challenges in the scalability and extensibility of computing blockchain environments.

The IBC protocol assumes that multiple applications are running on their own blockchain with their own state and own logic. Communication is achieved over an ordered message queue primitive, allowing the creation of complex inter-chain processes without trusted third parties.

The message packets are not signed by one psuedonymous account, or even multiple, as in multi-signature sidechain implementations. Rather, IBC assigns authorization of the packets to the source blockchain's consensus algorithm, performing light-client style verification on the destination chain. The Byzantine-fault-tolerant properties of the underlying blockchains are preserved: a user transferring assets between two chains using IBC must trust only the consensus algorithms of both chains.

In order to achive this, a blockchain should be able to check 1. whether the block is valid and 2. whether the packet is included in the block or not. In this proposal, we introduce an abstract concept of blockchains, called `Block`. `Block` requires its consensus algorithm to satisfy some properties, including deterministic safety, lightclient compatibility and valid transition of the state machine.

## Specification

### Motivation

`Block` defines required properties of the blockchain on the network. The implementors can check whether the consensus that they are using is qualified to be connected to the network or not. If not, they can modify the algorithm or wrap it with additional logic to make it compatible with the specification.

### Desired Properties

`Block` is `(p : Maybe<Block>, v : Block -> bool, s : ChainID -> Maybe<(Connection, PortID -> Channel))` where `p` is parent block, `v` is lightclient verifier and `s` is state. The other parts of the protocol, such as connections and channels, are defined over `Block`s, so if a blockchain wants to establish an IBC communication with another, it is required to satisfy the properties of `Block`. 

Definitions:

1. Direct children have their parent as `p` and are verifiable with their parent.
2. The height of a block is the step of referring `p` required to get to `nil`

Requirements:

1. Blocks have only one direct child
If a blockchain has deterministic safety(as opposed to probabilistic safety in Nakamoto consensus), then there cannot be more than one child for each block. In tendermint, this assumption breaks when +1/3 validators are malicious, producing multiple blocks those are direct children of a single block and all of them are verifiable with the parent. It makes conflicting packets delivered from the failed chain, so for the chains who are receiving from this chain, **fraud proof #10** mechanism should be applied in this case. However, he failed chain itself also need to be recovered and reconnected again. It will be covered in **byzantine recovery strategies #6**.

2. Blocks have at least one direct child
There is at least one direct child for all blocks, meaning that the lightclient logic can proceed the blocks one by one even in the worst case. If it not satisfied then there can be a point where the lightclient stops and cannot proceed, which halts IBC connection unexpectedly. This also can be violated when the blocks are restarted out of the consensus, for example in Tendermint, (1/2 < validators < 2/3) forked out the chain. This also will be covered in **byzantine recovery strategies**.

3. If a block verifies another block then it is a descendant of the block
Lightclient should not verify packets which are not in its chain. If the verifier returns true for a block that is not a descendant, it simply means that there is an error in the lightclient logic.

4. Block can have a state only if it is a valid transition from the parent's(see the next paragraph)
Block cannot have a state which is not a transition from its parent. Since the state is only defined for IBC logic, this only means that each block are running IBC-compatible application logic(does not care about the other modules' failure, including the handlers for the packets).

These requirements allow channels to work safely without concerning about double spending attack. 
If a block is submitted to the chain(and optionally passed some challenge period for fraud proof), then it is assured that the packet is finalized so the application logic can process it.

### Technical Specification

In a real world, `Block`s do not have to contain the full state data; it can contain an aggregated data set identifier, for example a merkle root, where the actual data latter given can be proven that it is a member of that set. If we don't consider about the data availability problem, this partial blocks, which is be called `Header`s, still satisfies `Block`.

Following functions exist over `Header`s.

* `height : Header -> int`
Returns the height of the block.

* `verify : Header -> Header -> Maybe<LightClientProof> -> bool`
Verifies the latter block is a descendant of the former block, as defined in **lightclient specification**. Can take an additional lightclient proof argument.

* `connection : Header -> ChainID -> Maybe<MemberProof> -> Connection`
Returns the connection for `ChainID` as defined in **connection semantics**. Can take an additional set member proof argument.

* `channel : Header -> ChainID -> PortID -> Channel`
Returns the channel for `(ChainID, PortID)` pair, as defined in **channel semantics**. Can take an additional set member proof argument.

### Example Implementation

An example blockchain `B` is run by a single operator. If a block is signed by the operator, then it is valid. `B` contains `KVStore`, which is a mapping from `[byte]` to  `[byte]`. 

```
type B = (Maybe<B>, Height, OperatorPubKey, Sig, KVStore)
height(B) = B.Height
verify(B1, B2) = B1.OperatorPubKey == B2.OperatorPubKey && B2.OperatorPubKey.Verify(B2.Sig, B2.Hash())
connection(B, ChainID) = Con(B.KVStore.Prefix([0x00] + ChainID))
channel(B, ChainID, PortID) = Ch(B.KVStore.Prefix([0x01] + ChainID + PotID))
```

Where `Con` and `Ch` satisfies `Connection` and `Channel`.

### Other Implementations

* Cosmos-SDK: [](https://github.com/cosmos/cosmos-sdk/x/ibc)  

## History 

March 5th 2019: Initial ICS 2 draft finished and submitted as a PR
