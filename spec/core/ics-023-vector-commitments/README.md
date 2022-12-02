---
ics: 23
title: Vector Commitments
stage: draft
required-by: 2, 24
category: IBC/TAO
kind: interface
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-08-25
---

## Synopsis

A *vector commitment* is a construction that produces a constant-size, binding commitment to an indexed vector of elements and short membership and/or non-membership proofs for any indices & elements in the vector.
This specification enumerates the functions and properties required of commitment constructions used in the IBC protocol. In particular, commitments utilised in IBC are required to be *positionally binding*: they must be able to prove existence or
nonexistence of values at specific positions (indices).

### Motivation

In order to provide a guarantee of a particular state transition having occurred on one chain which can be verified on another chain, IBC requires an efficient cryptographic construction to prove inclusion or non-inclusion of particular values at particular paths in state.

### Definitions

The *manager* of a vector commitment is the actor with the ability and responsibility to add or remove items from the commitment. Generally this will be the state machine of a blockchain.

The *prover* is the actor responsible for generating proofs of inclusion or non-inclusion of particular elements. Generally this will be a relayer (see [ICS 18](../../relayer/ics-018-relayer-algorithms)).

The *verifier* is the actor who checks proofs in order to verify that the manager of the commitment did or did not add a particular element. Generally this will be an IBC handler (module implementing IBC) running on another chain.

Commitments are instantiated with particular *path* and *value* types, which are assumed to be arbitrary serialisable data.

A *negligible function* is a function that grows more slowly than the reciprocal of every positive polynomial, as defined [here](https://en.wikipedia.org/wiki/Negligible_function).

### Desired Properties

This document only defines desired properties, not a concrete implementation — see "Properties" below.

## Technical Specification

Below we define a behaviour and an overview of datatypes. For data type definition look at [cosmos/ics23](https://github.com/cosmos/ics23/blob/master/proto/cosmos/ics23/v1/proofs.proto) repository.


### Datatypes

A commitment construction MUST specify the following datatypes, which are otherwise opaque (need not be introspected) but MUST be serialisable:

#### Commitment State

A `CommitmentState` is the full state of the commitment, which will be stored by the manager.

```typescript
type CommitmentState = object
```

#### Commitment Root

A `CommitmentRoot` commits to a particular commitment state and should be constant-size.

In certain commitment constructions with constant-size states, `CommitmentState` and `CommitmentRoot` may be the same type.

```typescript
type CommitmentRoot = object
```

#### Commitment Path

A `CommitmentPath` is the path used to verify commitment proofs, which can be an arbitrary structured object (defined by a commitment type). It must be computed by `applyPrefix` (defined below).

```typescript
type CommitmentPath = object
```

#### Prefix

A `CommitmentPrefix` defines a store prefix of the commitment proof. It is applied to the path before the path is passed to the proof verification functions. 

```typescript
type CommitmentPrefix = object
```

The function `applyPrefix` constructs a new commitment path from the arguments. It interprets the path argument in the context of the prefix argument. 

For two `(prefix, path)` tuples, `applyPrefix(prefix, path)` MUST return the same key only if the tuple elements are equal.

`applyPrefix` MUST be implemented per `Path`, as `Path` can have different concrete structures. `applyPrefix` MAY accept multiple `CommitmentPrefix` types.

The `CommitmentPath` returned by `applyPrefix` does not need to be serialisable (e.g. it might be a list of tree node identifiers), but it does need an equality comparison.

```typescript
type applyPrefix = (prefix: CommitmentPrefix, path: Path) => CommitmentPath
```

#### Proof

A `CommitmentProof` demonstrates membership or non-membership for an element or set of elements, verifiable in conjunction with a known commitment root. Proofs should be succinct.

```typescript
type CommitmentProof = object
```

### Required functions

A commitment construction MUST provide the following functions, defined over paths as serialisable objects and values as byte arrays:

```typescript
type Path = string

type Value = []byte
```

#### Initialisation

The `generate` function initialises the state of the commitment from an initial (possibly empty) map of paths to values.

```typescript
type generate = (initial: Map<Path, Value>) => CommitmentState
```

#### Root calculation

The `calculateRoot` function calculates a constant-size commitment to the commitment state which can be used to verify proofs.

```typescript
type calculateRoot = (state: CommitmentState) => CommitmentRoot
```

#### Adding & removing elements

The `set` function sets a path to a value in the commitment.

```typescript
type set = (state: CommitmentState, path: Path, value: Value) => CommitmentState
```

The `remove` function removes a path and associated value from a commitment.

```typescript
type remove = (state: CommitmentState, path: Path) => CommitmentState
```

#### Proof generation

The `createMembershipProof` function generates a proof that a particular commitment path has been set to a particular value in a commitment.

```typescript
type createMembershipProof = (state: CommitmentState, path: CommitmentPath, value: Value) => CommitmentProof
```

The `createNonMembershipProof` function generates a proof that a commitment path has not been set to any value in a commitment.

```typescript
type createNonMembershipProof = (state: CommitmentState, path: CommitmentPath) => CommitmentProof
```

#### Proof verification

The `verifyMembership` function verifies a proof that a path has been set to a particular value in a commitment.

```typescript
type verifyMembership = (root: CommitmentRoot, proof: CommitmentProof, path: CommitmentPath, value: Value) => boolean
```

The `verifyNonMembership` function verifies a proof that a path has not been set to any value in a commitment.

```typescript
type verifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, path: CommitmentPath) => boolean
```

### Optional functions

A commitment construction MAY provide the following functions:

The `batchVerifyMembership` function verifies a proof that many paths have been set to specific values in a commitment.

```typescript
type batchVerifyMembership = (root: CommitmentRoot, proof: CommitmentProof, items: Map<CommitmentPath, Value>) => boolean
```

The `batchVerifyNonMembership` function verifies a proof that many paths have not been set to any value in a commitment.

```typescript
type batchVerifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, paths: Set<CommitmentPath>) => boolean
```

If defined, these functions MUST produce the same result as the conjunctive union of `verifyMembership` and `verifyNonMembership` respectively (efficiency may vary):

```typescript
batchVerifyMembership(root, proof, items) ===
  all(items.map((item) => verifyMembership(root, proof, item.path, item.value)))
```

```typescript
batchVerifyNonMembership(root, proof, items) ===
  all(items.map((item) => verifyNonMembership(root, proof, item.path)))
```

If batch verification is possible and more efficient than individual verification of one proof per element, a commitment construction SHOULD define batch verification functions.

### Properties & Invariants

Commitments MUST be *complete*, *sound*, and *position binding*. These properties are defined with respect to a security parameter `k`, which MUST be agreed upon by the manager, prover, and verifier (and often will be constant for the commitment algorithm).

#### Completeness

Commitment proofs MUST be *complete*: path => value mappings which have been added to the commitment can always be proved to have been included, and paths which have not been included can always be proved to have been excluded, except with probability negligible in `k`.

For any prefix `prefix` and any path `path` last set to a value `value` in the commitment `acc`,

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, applyPrefix(prefix, path), value)
```

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === false) negligible in k
```

For any prefix `prefix` and any path `path` not set in the commitment `acc`, for all values of `proof` and all values of `value`,

```typescript
root = getRoot(acc)
proof = createNonMembershipProof(acc, applyPrefix(prefix, path))
```

```
Probability(verifyNonMembership(root, proof, applyPrefix(prefix, path)) === false) negligible in k
```

#### Soundness

Commitment proofs MUST be *sound*: path => value mappings which have not been added to the commitment cannot be proved to have been included, or paths which have been added to the commitment excluded, except with probability negligible in a configurable security parameter `k`.

For any prefix `prefix` and any path `path` last set to a value `value` in the commitment `acc`, for all values of `proof`,

```
Probability(verifyNonMembership(root, proof, applyPrefix(prefix, path)) === true) negligible in k
```

For any prefix `prefix` and any path `path` not set in the commitment `acc`, for all values of `proof` and all values of `value`,

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === true) negligible in k
```

#### Position binding

Commitment proofs MUST be *position binding*: a given commitment path can only map to one value, and a commitment proof cannot prove that the same path opens to a different value except with probability negligible in k.

For any prefix `prefix` and any path `path` set in the commitment `acc`, there is one `value` for which:

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, applyPrefix(prefix, path), value)
```

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === false) negligible in k
```

For all other values `otherValue` where `value !== otherValue`, for all values of `proof`,

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), otherValue) === true) negligible in k
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

Apr 25, 2019 - Draft submitted

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
