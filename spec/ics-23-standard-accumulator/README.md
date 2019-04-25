---
ics: 23
title: Standard Accumulator
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-04-25
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

Accumulators are instantiated with a particular *element* type, which is assumed to be arbitrary serializable data.

## Desired Properties

This document only defines desired properties, not a concrete implementation — see "algorithms" below.

## Technical Specification

### Datatypes

An accumulator construction MUST specify the following datatypes, which are otherwise opaque (need not be introspected) but must be serializable:

#### State

```golang
type AccumulatorState struct
```

#### Root

```golang
type AccumulatorRoot struct
```

#### Proof

```golang
type AccumulatorProof struct
```

### Required functions

An accumulator construction MUST provide the following functions:

#### Initialization

```coffeescript
generate(Set<Element> initial) -> AccumulatorState
```

#### Root calculation

```coffeescript
calculateRoot(AccumulatorState state) -> AccumulatorRoot
```

#### Adding & removing elements

```coffeescript
add(AccumulatorState state, Element elem) -> AccumulatorState
```

```coffeescript
remove(AccumulatorState state, Element elem) -> AccumulatorState
```

#### Proof generation

```coffeescript
createMembershipWitness(AccumulatorState state, Element elem) -> AccumulatorProof
```

```coffeescript
createNonMembersipWitness(AccumulatorState state, Element elem) -> AccumulatorProof
```

#### Proof verification

```coffeescript
verifyMembership(AccumulatorRoot root, AccumulatorProof proof, Element elem) -> boolean
```

```coffeescript
verifyNonMembership(AccumulatorRoot root, AccumulatorProof proof, Element elem) -> boolean
```

### Optional functions

An accumulator construction MAY provide the following functions:

```coffeescript
batchVerifyMembership(AccumulatorRoot root, AccumulatorProof proof, Set<Element> elems) -> boolean
```

```coffeescript
batchVerifyNonMembership(AccumulatorRoot root, AccumulatorProof proof, Set<Element> elems) -> boolean
```

If defined, these functions MUST be computationally equivalent to the conjunctive union of `verifyMembership` and `verifyNonMembership` respectively (`proof` may vary):

```coffeescript
batchVerifyMembership(root, proof, elems) ==
  verifyMembership(root, proof, elems[0]) &&
  verifyMembership(root, proof, elems[1]) && ...
```

```coffeescript
batchVerifyMembership(root, proof, elems) ==
  verifyNonMembership(root, proof, elems[0]) &&
  verifyNonMembership(root, proof, elems[1]) && ...
```

If batch verification is possible and more efficient than individual verification of one proof per element, an accumulator construction SHOULD define batch verification functions.

### Properties

Accumulators must be *correct* and *sound*. In practice, violations of these properties by computationally-bounded adversaries may be negligible in some security parameter, which is sufficient for use in IBC.

#### Correctness

Accumulator proofs must be *correct*: elements which have been added to the accumulator can always be proved to have been included, but cannot be proved to be excluded.

For an element `elem` in the accumulator `acc`,

```coffeescript
root = getRoot(acc)
proof = createMembershipWitness(acc, elem)
verifyMembership(root, proof, elem) == true
```

and, likewise, for all values of `proof`,

```coffeescript
verifyNonMembership(root, proof, elem) == false
```

#### Soundness

Accumulator proofs must be *sound*: elements which have not been added to the accumulator can never be proved to have been included, but can always be proved to have been excluded.

For an element `elem` not in the accumulator `acc`, for all values of `proof`,

```coffeescript
verifyMembership(root, proof, elem) == false
```

and, likewise, non-membership can be verified,

```coffeescript
root = getRoot(acc)
proof = createNonMembershipWitness(acc, elem)
verifyNonMembership(root, proof, elem) == true
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Accumulator algorithms are expected to be fixed. New algorithms can be introduced by versioning connections and channels.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

Security definitions are mostly sourced from these papers (and simplified somewhat):
- [Accumulators with Applications to Anonymity-Preserving Revocation](https://eprint.iacr.org/2017/043.pdf)
- [Batching Techniques for Accumulators with Applications to IOPs and Stateless Blockchains](https://eprint.iacr.org/2018/1188.pdf)

25 April 2019 - Draft submitted

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
