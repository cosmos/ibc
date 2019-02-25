---
ics: 2
title: Chain Naming Convention
status: Proposal
category: IBC
author: Juwoon Yun <joon@tendermint.com>
created: 2019-02-25
---

## Abstract

`ChainID` is a type that a chain uses to identify the other chain. When the chain stores or reads information about the other chain, it access to its storage using the `ChainID`. Other information, such as genesis file, registerer of the other chain, etc., can effect on the issuence of the `ChainID`, but once the `ChainID` is formed, it is persistent and modifying the `ChainID` means that the chain is now recognizing the other chain as newly registered chain, even if the other chain is identical with the original. Same with when the `ChainID` remains the same but the pointing chain is changed, for example the application logic manually override the latest header with another chain's, then now the chain recognizes the new chain as the original chain, making the new chain inherits the previous chain's balance and other chain-level states.

`ChainID` can be subjective on the chains, so there is no protocol level contraint on how the `ChainID` will be formed. The chain's application logic can choose how it will allocate `ChainID` for the other chain. However, the format of `ChainID` is defined in the protocol.
 
`ChainID` is one of the fundemental building blocks of IBC protocol, thus it has to be indepdendent from the enviornments as possible. The format of `ChainID` should be uniform, regardless of the encoding library, operating system. etc..

In this proposal, we propose the **format** of the `ChainID`s, which satisfies the requirements above and is restricted by the protocol. Also we propose the **generation scheme**s of the `ChainID`s, which is not restricted by the protocol but recommended. 

## Format

### Proposal 1

`ChainID` is a type of `[4]byte`. 

### Proposal 2

`ChainID` is a type of `[8]byte`, which is human readable alphanumeric string with constant size.

### Proposal 3

`ChainID` is a type of `[]byte`, which is human readable alphanumeric string with variable size, maximum `256`. The first byte of a `ChainID` should be equal with the length of the rest.

## Generation Scheme

NOTE: multiple generation scheme can be valid

### Proposal 1

`ChainID` is generated from the root of trust commit hash, and optionally with user-provided sugar in order to prevent hash collision.

### Proposal 2

`ChainID` is allocated by the application logic in nondeterministic interactive way. This includes auction and governance.
