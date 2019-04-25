---
ics: 23
title: Standard Accumulator
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-04-16
---

# Synopsis

An *accumulator*, or *cryptographic accumulator*, is a construction that produces a succinct, binding commitment to a set or indexed vector of elements and short membership and/or non-membership proofs for any element in the set or vector.
This specification enumerates the functions and properties required of accumulator constructions used in the IBC protocol.

# Specification

## Motivation

In order to provide a guarantee of a particular state transition having occurred on one chain which can be verified on another chain, IBC requires an efficient cryptographic construction to prove inclusion or non-inclusion of particular values in state.

## Definitions

The *manager* of an accumulator is the actor with the ability and responsibility to add or remove items from the accumulator. Generally this will be the state machine of a blockchain.

The *prover* is the actor responsible for generating accumulator proofs of inclusion or non-inclusion of particular elements. Generally this will be a relayer (see [ICS 18](../ics-18-relayer-algorithms)).

The *verifier* is the actor who checks accumulator proofs in order to verify that the manager of the accumulator did or did not add a particular element. Generally this will be an IBC handler running on another chain.

## Desired Properties

This document only defines desired properties, not a concrete implementation — see "algorithms" below.

## Technical Specification

An accumulator MUST provide the following functions:

```coffeescript

```

### Algorithms

required

- Gen(S_0) -> a_0
- Add(a_t, x) -> (a_t+1)
- Del(a_t, x) -> (a_t+1)

- MemWitCreate(a_t, x) -> w_xt
- NonMemWitCreate(a_t, x) -> w_xt

- VerMem(a_t, x, w_xt) -> {0, 1}
- VerNonMem(a_t, x, w_xt) -> {0, 1}

must be
- correct: for an element in the accumulator, vermem(element) => 1, vernonmem(element) => 0
- sound: for an element not in the accumulator, vernonmem(element) => 1, vermem(element) => 0

optional

- BatchVerifyMem(a_t, {x}, w_xt) -> {0, 1}
- BatchVerifyNonMem(a_t, {x}, w_xt) -> {0, 1}

(vector commitments)

- Gen() -> v_0
- Com(v_t, m) -> v_t+1
- Update(v_t, m, i) -> v_t+1
- Open(v_t, m, i) -> pi
- Verify(v_t, pi) -> {0, 1}

optional

- BatchOpen
- BatchVerify

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Accumulator algorithms are expected to be fixed. New algorithms can be introduced by versioning connections and channels.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

Security definitions are mostly sourced from these papers:
- [Accumulators with Applications to Anonymity-Preserving Revocation](https://eprint.iacr.org/2017/043.pdf)
- [Batching Techniques for Accumulatorswith Applications to IOPs andStateless Blockchains](https://eprint.iacr.org/2018/1188.pdf)

25 April 2019 - Draft submitted

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
