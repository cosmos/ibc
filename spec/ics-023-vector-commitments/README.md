---
ics: 23
title: Vector Commitments
stage: draft
required-by: 2, 3, 4
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-05-11
---

## Synopsis

A *vector commitment* is a construction that produces a succinct, binding commitment to an indexed vector of elements and short membership and/or non-membership proofs for any indices & elements in the vector.
This specification enumerates the functions and properties required of commitment constructions used in the IBC protocol. In particular, commitments utilized in IBC are required to be *positionally binding*: they must be able to prove existence or
nonexistence of values at specific positions (indices).

### Motivation

In order to provide a guarantee of a particular state transition having occurred on one chain which can be verified on another chain, IBC requires an efficient cryptographic construction to prove inclusion or non-inclusion of particular values at particular keys in state.

### Definitions

The *manager* of a vector commitment is the actor with the ability and responsibility to add or remove items from the commitment. Generally this will be the state machine of a blockchain.

The *prover* is the actor responsible for generating proofs of inclusion or non-inclusion of particular elements. Generally this will be a relayer (see [ICS 18](../ics-018-relayer-algorithms)).

The *verifier* is the actor who checks proofs in order to verify that the manager of the commitment did or did not add a particular element. Generally this will be an IBC handler (module implementing IBC) running on another chain.

Commitments are instantiated with particular *key* and *value* types, which are assumed to be arbitrary serializable data.

A *negligible function* is a function that grows more slowly than the reciprocal of every positive polynomial, as defined [here](https://en.wikipedia.org/wiki/Negligible_function).

### Desired Properties

This document only defines desired properties, not a concrete implementation — see "Properties" below.

## Technical Specification

### Datatypes

An commitment construction MUST specify the following datatypes, which are otherwise opaque (need not be introspected) but MUST be serializable:

#### State

An `CommitmentState` is the full state of the commitment, which will be stored by the manager.

```typescript
type CommitmentState = object
```

#### Root

An `CommitmentRoot` commits to a particular commitment state and should be succinct.

In certain commitment constructions with succinct states, `CommitmentState` and `CommitmentRoot` may be the same type.

```typescript
type CommitmentRoot = object
```

#### Proof

An `CommitmentProof` demonstrates membership or non-membership for an element or set of elements, verifiable in conjunction with a known commitment root. Proofs should be succinct.

```typescript
type CommitmentProof = object
```

### Required functions

An commitment construction MUST provide the following functions, defined over keys and values as byte arrays:

```typescript
type Key = string

type Value = string
```

#### Initialization

The `generate` function initializes the state of the commitment from an initial (possibly empty) map of keys to values.

```typescript
type generate = (initial: Map<Key, Value>) => CommitmentState
```

#### Root calculation

The `calculateRoot` function calculates a succinct commitment to the commitment state which can be used to verify proofs.

```typescript
type calculateRoot = (state: CommitmentState) => CommitmentRoot
```

#### Adding & removing elements

The `set` function sets a key to a value in the commitment.

```typescript
type set = (state: CommitmentState, key: Key, value: Value) => CommitmentState
```

The `remove` function removes a key and associated value from an commitment.

```typescript
type remove = (state: CommitmentState, key: Key) => CommitmentState
```

#### Proof generation

The `createMembershipProof` function generates a proof that a particular key has been set to a particular value in an commitment.

```typescript
type createMembershipProof = (state: CommitmentState, key: Key, value: Value) => CommitmentProof
```

The `createNonMembershipProof` function generates a proof that a key has not been set to any value in an commitment.

```typescript
type createNonMembershipProof = (state: CommitmentState, key: Key) => CommitmentProof
```

#### Proof verification

The `verifyMembership` function verifies a proof that a key has been set to a particular value in an commitment.

```typescript
type verifyMembership = (root: CommitmentRoot, proof: CommitmentProof, key: Key, value: Value) => boolean
```

The `verifyNonMembership` function verifies a proof that a key has not been set to any value in an commitment.

```typescript
type verifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, key: Key) => boolean
```

### Optional functions

An commitment construction MAY provide the following functions:

The `batchVerifyMembership` function verifies a proof that many keys have been set to specific values in an commitment.

```typescript
type batchVerifyMembership = (root: CommitmentRoot, proof: CommitmentProof, items: Map<Key, Value>) => boolean
```

The `batchVerifyNonMembership` function verifies a proof that many keys have not been set to any value in an commitment.

```typescript
type batchVerifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, keys: Set<Key>) => boolean
```

If defined, these functions MUST be computationally equivalent to the conjunctive union of `verifyMembership` and `verifyNonMembership` respectively (`proof` may vary):

```typescript
batchVerifyMembership(root, proof, items) ===
  all(items.map((item) => verifyMembership(root, proof, item)))
```

```typescript
batchVerifyNonMembership(root, proof, keys) ===
  all(items.map((item) => verifyNonMembership(root, proof, key)))
```

If batch verification is possible and more efficient than individual verification of one proof per element, an commitment construction SHOULD define batch verification functions.

### Properties

Commitments MUST be *complete*, *sound*, and *position binding*. These properties are defined with respect to a security parameter `λ`, which MUST be agreed upon by the manager, prover, and verifier (and often will be constant for the commitment algorithm).

#### Completeness

Commitment proofs MUST be *complete*: key => value mappings which have been added to the commitment can always be proved to have been included, and keys which have not been included can always be proved to have been excluded, except with probability negligible in `λ`.

For any key `key` last set to a value `value` in the commitment `acc`,

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, key, value)
```

```
Pr(verifyMembership(root, proof, key, value) === false) negligible in λ
```

For any key `key` not set in the commitment `acc`, for all values of `proof` and all values of `value`,

```typescript
root = getRoot(acc)
proof = createNonMembershipProof(acc, key)
```

```
Pr(verifyNonMembership(root, proof, key) === false) negligible in λ
```

#### Soundness

Commitment proofs MUST be *sound*: key => value mappings which have not been added to the commitment cannot be proved to have been included, or keys which have been added to the commitment excluded, except with probability negligible in a configurable security parameter `λ`.

For any key `key` last set to a value `value` in the commitment `acc`, for all values of `proof`,

```
Pr(verifyNonMembership(root, proof, key) === true) negligible in λ
```

For any key `key` not set in the commitment `acc`, for all values of `proof` and all values of `value`,

```
Pr(verifyMembership(root, proof, key, value) === true) negligible in λ
```

#### Position binding

Commitment proofs MUST be *position binding*: a given key can only map to one value, and an commitment proof cannot prove that the same key opens to a different value except with probability negligible in λ.

For any key `key` set in the commitment `acc`, there is one `value` for which:

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, key, value)
```

```
Pr(verifyMembership(root, proof, key, value) === false) negligible in λ
```

For all other values `otherValue` where `value /= otherValue`, for all values of `proof`,

```
Pr(verifyMembership(root, proof, key, otherValue) === true) negligible in λ
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Commitment algorithms are expected to be fixed. New algorithms can be introduced by versioning connections and channels.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Security definitions are mostly sourced from these papers (and simplified somewhat):
- [Vector Commitments and their Applications](https://eprint.iacr.org/2011/495.pdf)
- [Commitments with Applications to Anonymity-Preserving Revocation](https://eprint.iacr.org/2017/043.pdf)
- [Batching Techniques for Commitments with Applications to IOPs and Stateless Blockchains](https://eprint.iacr.org/2018/1188.pdf)

Thanks to Dev Ojha for extensive comments on this specification.

25 April 2019 - Draft submitted

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
