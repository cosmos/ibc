---
ics: 23
title: Vector Commitments
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-04-25
---

# Synopsis

A *vector commitment* is a construction that produces a succinct, binding commitment to an indexed vector of elements and short membership and/or non-membership proofs for any indicies & elements in the vector.
This specification enumerates the functions and properties required of commitment constructions used in the IBC protocol. In particular, commitments utilized in IBC are required to be *positionally binding*: they must be able to prove existence or
nonexistence of values at specific positions (indices).

# Specification

## Motivation

In order to provide a guarantee of a particular state transition having occurred on one chain which can be verified on another chain, IBC requires an efficient cryptographic construction to prove inclusion or non-inclusion of particular values at particular keys in state.

## Definitions

The *manager* of a vector commitment is the actor with the ability and responsibility to add or remove items from the commitment. Generally this will be the state machine of a blockchain.

The *prover* is the actor responsible for generating proofs of inclusion or non-inclusion of particular elements. Generally this will be a relayer (see [ICS 18](../ics-18-relayer-algorithms)).

The *verifier* is the actor who checks proofs in order to verify that the manager of the commitment did or did not add a particular element. Generally this will be an IBC handler running on another chain.

Commitments are instantiated with particular *key* and *value* types, which is assumed to be arbitrary serializable data.

## Desired Properties

This document only defines desired properties, not a concrete implementation — see "Properties" below.

## Technical Specification

### Datatypes

An commitment construction MUST specify the following datatypes, which are otherwise opaque (need not be introspected) but must be serializable:

#### State

An `CommitmentState` is the full state of the commitment, which will be stored by the manager.

```golang
type CommitmentState struct
```

#### Root

An `CommitmentRoot` commits to a particular commitment state and should be succinct.

In certain commitment constructions with succinct states, `CommitmentState` and `CommitmentRoot` may be the same type.

```golang
type CommitmentRoot struct
```

#### Proof

An `CommitmentProof` demonstrates membership or non-membership for an element or set of elements, verifiable in conjunction with a known commitment root. Proofs should be succinct.

```golang
type CommitmentProof struct
```

### Required functions

An commitment construction MUST provide the following functions:

#### Initialization

The `generate` function initializes the state of the commitment from an initial (possibly empty) map of keys to values.

```coffeescript
generate(Map<Key, Value> initial) -> CommitmentState
```

#### Root calculation

The `calculateRoot` function calculates a succinct commitment to the commitment state which can be used to verify proofs.

```coffeescript
calculateRoot(CommitmentState state) -> CommitmentRoot
```

#### Adding & removing elements

The `set` function sets a key to a value in the commitment.

```coffeescript
set(CommitmentState state, Key key, Value value) -> CommitmentState
```

The `remove` function removes a key and associated value from an commitment.

```coffeescript
remove(CommitmentState state, Key key) -> CommitmentState
```

#### Proof generation

The `createMembershipProof` function generates a proof that a particular key has been set to a particular value in an commitment.

```coffeescript
createMembershipProof(CommitmentState state, Key key, Value value) -> CommitmentProof
```

The `createNonMembershipProof` function generates a proof that a key has not been set to any value in an commitment.

```coffeescript
createNonMembersipProof(CommitmentState state, Key key) -> CommitmentProof
```

#### Proof verification

The `verifyMembership` function verifies a proof that a key has been set to a particular value in an commitment.

```coffeescript
verifyMembership(CommitmentRoot root, CommitmentProof proof, Key key, Value value) -> boolean
```

The `verifyNonMembership` function verifies a proof that a key has not been set to any value in an commitment.

```coffeescript
verifyNonMembership(CommitmentRoot root, CommitmentProof proof, Key key) -> boolean
```

### Optional functions

An commitment construction MAY provide the following functions:

The `batchVerifyMembership` function verifies a proof that many keys have been set to specific values in an commitment.

```coffeescript
batchVerifyMembership(CommitmentRoot root, CommitmentProof proof, Map<Key, Value> items) -> boolean
```

The `batchVerifyNonMembership` function verifies a proof that many keys have not been set to any value in an commitment.

```coffeescript
batchVerifyNonMembership(CommitmentRoot root, CommitmentProof proof, Set<Key> keys) -> boolean
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

If batch verification is possible and more efficient than individual verification of one proof per element, an commitment construction SHOULD define batch verification functions.

### Properties

Commitments must be *complete*, *sound*, and *position binding*. These properties are defined with respect to a security parameter `λ`, which must be agreed upon by the manager, prover, and verifier (and often will be constant for the commitment algorithm).

#### Completeness

Commitment proofs must be *complete*: key => value mappings which have been added to the commitment can always be proved to have been included, and keys which have not been included can always be proved to have been excluded, except with probability negligible in `λ`.

For any key `key` last set to a value `value` in the commitment `acc`,

```coffeescript
root = getRoot(acc)
proof = createMembershipProof(acc, key, value)
P(verifyMembership(root, proof, key, value) == false) negligible in λ
```

For any key `key` not set in the commitment `acc`, for all values of `proof` and all values of `value`,

```coffeescript
root = getRoot(acc)
proof = createNonMembershipProof(acc, key)
P(verifyNonMembership(root, proof, key) == false) negligible in λ
```

#### Soundness

Commitment proofs must be *sound*: key => value mappings which have not been added to the commitment cannot be proved to have been included, or keys which have been added to the commitment excluded, except with probability negligible in a configurable security parameter `λ`.

For any key `key` last set to a value `value` in the commitment `acc`, for all values of `proof`,

```coffeescript
P(verifyNonMembership(root, proof, key) == true) negligible in λ
```

For any key `key` not set in the commitment `acc`, for all values of `proof` and all values of `value`,

```coffeescript
P(verifyMembership(root, proof, key, value) == true) negligible in λ
```

#### Position binding

Commitment proofs must be *position binding*: a given key can only map to one value, and an commitment proof cannot prove that the same key opens to a different value except with probability negligible in λ.

For any key `key` set in the commitment `acc`, there is one `value` for which:

```coffeescript
root = getRoot(acc)
proof = createMembershipProof(acc, key, value)
verifyMembership(root, proof, key, value) == true
```

For all other values `otherValue` where `value /= otherValue`, for all values of `proof`,

```coffeescript
P(verifyMembership(root, proof, key, otherValue) == true) negligible in λ
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Commitment algorithms are expected to be fixed. New algorithms can be introduced by versioning connections and channels.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

Security definitions are mostly sourced from these papers (and simplified somewhat):
- [Vector Commitments and their Applications](https://eprint.iacr.org/2011/495.pdf)
- [commitments with Applications to Anonymity-Preserving Revocation](https://eprint.iacr.org/2017/043.pdf)
- [Batching Techniques for Commitments with Applications to IOPs and Stateless Blockchains](https://eprint.iacr.org/2018/1188.pdf)

25 April 2019 - Draft submitted

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
