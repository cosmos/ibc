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

An *accumulator*, or *cryptographic accumulator*, is a construction that produces a succinct, binding commitment to an indexed vector of elements and short membership and/or non-membership proofs for any indicies & elements in the vector.
This specification enumerates the functions and properties required of accumulator constructions used in the IBC protocol. In particular, accumulators utilized in IBC are required to be *vector commitments*, which can prove existence or
nonexistence of values at specific positions (indices).

# Specification

## Motivation

In order to provide a guarantee of a particular state transition having occurred on one chain which can be verified on another chain, IBC requires an efficient cryptographic construction to prove inclusion or non-inclusion of particular values at particular keys in state.

## Definitions

The *manager* of an accumulator is the actor with the ability and responsibility to add or remove items from the accumulator. Generally this will be the state machine of a blockchain.

The *prover* is the actor responsible for generating accumulator proofs of inclusion or non-inclusion of particular elements. Generally this will be a relayer (see [ICS 18](../ics-18-relayer-algorithms)).

The *verifier* is the actor who checks accumulator proofs in order to verify that the manager of the accumulator did or did not add a particular element. Generally this will be an IBC handler running on another chain.

Accumulators are instantiated with a particular *element* type, which is assumed to be arbitrary serializable data.

## Desired Properties

This document only defines desired properties, not a concrete implementation — see "Properties" below.

## Technical Specification

### Datatypes

An accumulator construction MUST specify the following datatypes, which are otherwise opaque (need not be introspected) but must be serializable:

#### State

An `AccumulatorState` is the full state of the accumulator, which will be stored by the manager.

```golang
type AccumulatorState struct
```

#### Root

An `AccumulatorRoot` commits to a particular accumulator state and should be succinct.

In certain accumulator constructions with succinct states, `AccumulatorState` and `AccumulatorRoot` may be the same type.

```golang
type AccumulatorRoot struct
```

#### Proof

An `AccumulatorProof` demonstrates membership or non-membership for an element or set of elements, verifiable in conjunction with a known accumulator root. Proofs should be succinct.

```golang
type AccumulatorProof struct
```

### Required functions

An accumulator construction MUST provide the following functions:

#### Initialization

The `generate` function initializes the state of the accumulator from an initial (possibly empty) map of keys to values.

```coffeescript
generate(Map<Key, Value> initial) -> AccumulatorState
```

#### Root calculation

The `calculateRoot` function calculates a succinct commitment to the accumulator state which can be used to verify proofs.

```coffeescript
calculateRoot(AccumulatorState state) -> AccumulatorRoot
```

#### Adding & removing elements

The `set` function sets a key to a value in the accumulator.

```coffeescript
set(AccumulatorState state, Key key, Value value) -> AccumulatorState
```

The `remove` function removes a key and associated value from an accumulator.

```coffeescript
remove(AccumulatorState state, Key key) -> AccumulatorState
```

#### Proof generation

The `createMembershipProof` function generates a proof that a particular key has been set to a particular value in an accumulator.

```coffeescript
createMembershipProof(AccumulatorState state, Key key, Value value) -> AccumulatorProof
```

The `createNonMembershipProof` function generates a proof that a key has not been set to any value in an accumulator.

```coffeescript
createNonMembersipProof(AccumulatorState state, Key key) -> AccumulatorProof
```

#### Proof verification

The `verifyMembership` function verifies a proof that a key has been set to a particular value in an accumulator.

```coffeescript
verifyMembership(AccumulatorRoot root, AccumulatorProof proof, Key key, Value value) -> boolean
```

The `verifyNonMembership` function verifies a proof that a key has not been set to any value in an accumulator.

```coffeescript
verifyNonMembership(AccumulatorRoot root, AccumulatorProof proof, Key key) -> boolean
```

### Optional functions

An accumulator construction MAY provide the following functions:

The `batchVerifyMembership` function verifies a proof that many keys have been set to specific values in an accumulator.

```coffeescript
batchVerifyMembership(AccumulatorRoot root, AccumulatorProof proof, Map<Key, Value> items) -> boolean
```

The `batchVerifyNonMembership` function verifies a proof that many keys have not been set to any value in an accumulator.

```coffeescript
batchVerifyNonMembership(AccumulatorRoot root, AccumulatorProof proof, Set<Key> keys) -> boolean
```

If defined, these functions MUST be computationally equivalent to the conjunctive union of `verifyMembership` and `verifyNonMembership` respectively (`proof` may vary):

```coffeescript
batchVerifyMembership(root, proof, items) ==
  verifyMembership(root, proof, items[0].Key, items[0].Value) &&
  verifyMembership(root, proof, items[1].Key, items[1].Value) && ...
```

```coffeescript
batchVerifyMembership(root, proof, keys) ==
  verifyNonMembership(root, proof, keys[0]) &&
  verifyNonMembership(root, proof, keys[1]) && ...
```

If batch verification is possible and more efficient than individual verification of one proof per element, an accumulator construction SHOULD define batch verification functions.

### Properties

Accumulators must be *correct* and *sound*. In practice, violations of these properties by computationally-bounded adversaries may be negligible in some security parameter, which is sufficient for use in IBC.

#### Correctness

Accumulator proofs must be *correct*: elements which have been added to the accumulator can always be proved to have been included, but cannot be proved to be excluded.

For a key `key` set to a value `value` in the accumulator `acc`,

```coffeescript
root = getRoot(acc)
proof = createMembershipWitness(acc, key, value)
verifyMembership(root, proof, key, value) == true
```

and, likewise, for all values of `proof`,

```coffeescript
verifyNonMembership(root, proof, key) == false
```

#### Soundness

Accumulator proofs must be *sound*: elements which have not been added to the accumulator can never be proved to have been included, but can always be proved to have been excluded.

For an key `key` not set in the accumulator `acc`, for all values of `proof` and all values of `value`,

```coffeescript
verifyMembership(root, proof, key, value) == false
```

and, likewise, non-membership can be verified,

```coffeescript
root = getRoot(acc)
proof = createNonMembershipWitness(acc, key)
verifyNonMembership(root, proof, key) == true
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
- [Vector Commitments and their Applications](https://eprint.iacr.org/2011/495.pdf)
- [Accumulators with Applications to Anonymity-Preserving Revocation](https://eprint.iacr.org/2017/043.pdf)
- [Batching Techniques for Accumulators with Applications to IOPs and Stateless Blockchains](https://eprint.iacr.org/2018/1188.pdf)

25 April 2019 - Draft submitted

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
