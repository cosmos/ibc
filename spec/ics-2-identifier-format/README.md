---
ics: 2
title: Identifier Format
status: Proposal
category: IBC
author: Juwoon Yun <joon@tendermint.com>
created: 2019-02-25
---

## Abstract

`ChainID` is a type that a chain uses to identify the other chain. When the chain stores or reads information about the other chain, it access to its storage using the `ChainID`. Other information, such as genesis file, registerer of the other chain, etc., can effect on the issuence of the `ChainID`, but once the `ChainID` is formed, it is persistent and modifying the `ChainID` means that the chain is now recognizing the other chain as newly registered chain, even if the other chain is identical with the original. Same with when the `ChainID` remains the same but the pointing chain is changed, for example the application logic manually override the latest header with another chain's, then now the chain recognizes the new chain as the original chain, making the new chain inherits the previous chain's balance and other chain-level states.

`ChainID` can be subjective on the chains, so there is no protocol level contraint on how the `ChainID` will be generated. The chain's application logic can choose how it will allocate `ChainID` for the other chain. However, the syntactic format of `ChainID` is defined in the protocol.
 
`PortID` is a type that an entity on a chain(including EOAs, contracts, modules) can occupy in order to send packets with a separate sequence from others. A queue, where the packets are actually stored in order, are defined as a pair `(ChainID, PortID)`. This means each entities can access to the queues only whose `PortID` is occupied by them. 

`PortID` should be able to be instanciated in runtime. For example, a contract newly deployed can claim a port to have independent queues. The format should be able to handle multiple `PortID`s, where the number is increasing over time and the collision happens as less as possible.

In this proposal, we propose the **format** of the `ChainID` and `PortID`, which satisfies the requirements above and is restricted by the protocol. Also we propose the **generation scheme**s of the `ChainID`s, which is not restricted by the protocol but recommended. 

## ChainID Format

### Proposal 1

`ChainID` is a type of `[4]byte`. 

### Proposal 2

`ChainID` is a type of `[8]byte`, which is human readable alphanumeric string with constant size.

### Proposal 3

`ChainID` is a type of `[]byte`, which is human readable alphanumeric string with variable size, maximum `256`. The first byte of a `ChainID` should be equal with the length of the rest.

## ChainID Generation Scheme

NOTE: multiple generation scheme can be valid

### Proposal 1

`ChainID` is generated from the root of trust commit hash, and optionally with user-provided sugar in order to prevent hash collision.

## PortID Format

### Proposal 1

`PortID` is a type of `[2/8]byte`. When the first byte of the `PortID` is other then `0xFF`. it has length `2`. When the first byte of the `PortID` is `0xFF`, it has length `8`.
