---
ics: 6
title: Solo Machine Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-09
modified: 2019-12-09
---

## Synopsis

This specification document describes a client (verification algorithm) for a solo machine with a single updateable public key which implements the [ICS 2](../../core/ics-002-client-semantics) interface.

### Motivation

Solo machines — which might be devices such as phones, browsers, or laptops — might like to interface with other machines & replicated ledgers which speak IBC, and they can do so through the uniform client interface.

Solo machine clients are roughly analogous to "implicit accounts" and can be used in lieu of "regular transactions" on a ledger, allowing all transactions to work through the unified interface of IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

### Desired Properties

This specification must satisfy the client interface defined in [ICS 2](../../core/ics-002-client-semantics).

Conceptually, we assume "big table of signatures in the universe" - that signatures produced are public - and incorporate replay protection accordingly.

## Technical Specification

This specification contains implementations for all of the functions defined by [ICS 2](../../core/ics-002-client-semantics).

### Client state

The `ClientState` of a solo machine is simply whether or not the client is frozen.

```typescript
interface ClientState {
  frozen: boolean
  consensusState: ConsensusState
}
```

### Consensus state

The `ConsensusState` of a solo machine consists of the current public key, current diversifier, sequence number, and timestamp.

The diversifier is an arbitrary string, chosen when the client is created, designed to allow the same public key to be re-used across different solo machine clients (potentially on different chains) without being considered misbehaviour.

```typescript
interface ConsensusState {
  sequence: uint64
  publicKey: PublicKey
  diversifier: string
  timestamp: uint64
}
```

### Height

The `Height` of a solo machine is just a `uint64`, with the usual comparison operations.

### Headers

`Header`s must only be provided by a solo machine when the machine wishes to update the public key or diversifier.

```typescript
interface Header {
  sequence: uint64
  timestamp: uint64
  signature: Signature
  newPublicKey: PublicKey
  newDiversifier: string
}
```

`Header` implements the ClientMessage interface.

### Signature Verification

The solomachine public key must sign over the following struct:

```typescript
interface SignBytes {
  sequence: uint64
  timestamp: uint64  
  diversifier: string
  path: []byte
  data: []byte
}
```

### Misbehaviour 

`Misbehaviour` for solo machines consists of a sequence and two signatures over different messages at that sequence.

```typescript
interface SignatureAndData {
  sig: Signature
  path: Path
  data: []byte
  timestamp: Timestamop
}

interface Misbehaviour {
  sequence: uint64
  signatureOne: SignatureAndData
  signatureTwo: SignatureAndData
}
```

`Misbehaviour` implements the ClientState interface.

### Signatures

Signatures are provided in the `Proof` field of client state verification functions. They include data & a timestamp, which must also be signed over.

```typescript
interface Signature {
  data: []byte
  timestamp: uint64
}
```

### Client initialisation

The solo machine client `initialise` function starts an unfrozen client with the initial consensus state.

```typescript
function initialise(consensusState: ConsensusState): ClientState {
  return {
    frozen: false,
    consensusState
  }
}
```

The solo machine client `latestClientHeight` function returns the latest sequence.

```typescript
function latestClientHeight(clientState: ClientState): uint64 {
  return clientState.consensusState.sequence
}
```

### ClientState Methods

All of the functions defined below are methods on the `ClientState` interface. Thus, the solomachine clientstate is always in scope for these functions.

### Validity predicate

The solo machine client `verifyClientMessage` function checks that the currently registered public key and diversifier signed over the client message at the expected sequence. If the client message is an update, then it must be the current sequence. If the client message is misbehaviour then it must be the sequence of the misbehaviour.

```typescript
function  verifyClientMessage(clientMsg: ClientMessage) {
  switch typeof(ClientMessage) {
    case Header:
      verifyHeader(clientMessage)
    // misbehaviour only suppported for current public key and diversifier on solomachine
    case Misbehaviour:
      verifyMisbehaviour(clientMessage)
  }
}

function verifyHeader(header: header) {
    assert(header.timestamp >= clientstate.consensusState.timestamp)
    headerData = {
      NewPublicKey: header.newPublicKey,
      NewDiversifier: header.newDiversifier,
    }
    sigBytes = SignBytes(
      Sequence: clientState.consensusState.sequence,
      Timestamp: header.timestamp,
      Diversifier: clientState.consensusState.diversifier,
      Path: []byte{"solomachine:header"}
      Value: marshal(headerData)
    )
    assert(checkSignature(cs.consensusState.publicKey, sigBytes, header.signature))
}

function verifyMisbehaviour(misbehaviour: Misbehaviour) {
    s1 = misbehaviour.signatureOne
    s2 = misbehaviour.signatureTwo
    pubkey = clientState.consensusState.publicKey
    diversifier = clientState.consensusState.diversifier
    timestamp = clientState.consensusState.timestamp
    // assert that timestamp could have fooled the light client
    assert(misbehaviour.s1.timestamp >= timestamp)
    assert(misbehaviour.s2.timestamp >= timestamp)
    // assert that the signatures validate and that they are different
    sigBytes1 = SignBytes(
      Sequence: misbehaviour.sequence,
      Timestamp: s1.timestamp,
      Diversifier: diversifier,
      Path: s1.path,
      Data: s1.data
    )
    sigBytes2 = SignBytes(
      Sequence: misbehaviour.sequence,
      Timestamp: s2.timestamp,
      Diversifier: diversifier,
      Path: s2.path,
      Data: s2.data
    )
    assert(sigBytes1 != sigBytes2)
    assert(checkSignature(pubkey, sigBytes1, clientState.consensusState.publicKey))
    assert(checkSignature(pubkey, sigBytes2, clientState.consensusState.publicKey))
}
```

### Misbehaviour predicate

Since misbehaviour is checked in `verifyClientMessage`, if the client message is of type `Misbehaviour` then we return true

```typescript
function checkForMisbehaviour(clientMessage: ClientMessage) => bool {
    switch typeof(ClientMessage) {
    case Misbehaviour:
      return true
    }
    return false
}
```

### Update Functions

`UpdateState` updates the function for a regular update:

```typescript
function updateState(clientMessage: ClientMessage) {
    clientState.consensusState.publicKey = header.newPublicKey
    clientState.consensusState.diversifier = header.newDiversifier
    clientState.consensusState.timestamp = header.timestamp
    clientState.consensusState.sequence++
    set("clients/{identifier}/clientState", clientState)
}
```

`UpdateStateOnMisbehaviour` updates the function after receving valid misbehaviour:

```typescript
function updateStateOnMisbehaviour(clientMessage: ClientMessage) {
    // freeze the client
    clientState.frozen = true
    set("clients/{identifier}/clientState", clientState)
}
```

### State verification functions

All solo machine client state verification functions simply check a signature, which must be provided by the solo machine.

Note that value concatenation should be implemented in a state-machine-specific escaped fashion.

```typescript
function verifyMembership(
  // provided height is unnecessary for solomachine
  // since clientState maintains the expected sequence
  height: uint64,
  // delayPeriod is unsupported on solomachines
  // thus these fields are ignored
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: []byte): boolean {
    // the expected sequence used in the signature
    abortTransactionUnless(!clientState.frozen)
    abortTransactionUnless(proof.timestamp >= clientState.consensusState.timestamp)
    sigBytes = SignBytes(
      Sequence: clientState.consensusState.sequence,
      Timestamp: proof.timestamp,
      Diversifier: clientState.consensusState.diversifier,
      path: path.String(),
      data: value,
    )
    proven = checkSignature(clientState.consensusState.pubKey, sigBytes, proof.sig)
    if !proven {
      return false
    }

    // increment sequence on each verification to provide
    // replay protection
    clientState.consensusState.sequence++
    clientState.consensusState.timestamp = proof.timestamp
    return true
}

function verifyNonMembership(
  // provided height is unnecessary for solomachine
  // since clientState maintains the expected sequence
  height: uint64,
  // delayPeriod is unsupported on solomachines
  // thus these fields are ignored
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath): boolean {
    abortTransactionUnless(!clientState.frozen)
    abortTransactionUnless(proof.timestamp >= clientState.consensusState.timestamp)
    sigBytes = SignBytes(
      Sequence: clientState.consensusState.sequence,
      Timestamp: proof.timestamp,
      Diversifier: clientState.consensusState.diversifier,
      path: path.String(),
      data: nil,
    )
    proven = checkSignature(clientState.consensusState.pubKey, value, proof.sig)
    if !proven {
      return false
    }

    // increment sequence on each verification to provide
    // replay protection
    clientState.consensusState.sequence++
    clientState.consensusState.timestamp = proof.timestamp
    return true
}
```

### Properties & Invariants

Instantiates the interface defined in [ICS 2](../../core/ics-002-client-semantics).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementation

None yet.

## Other Implementations

None at present.

## History

December 9th, 2019 - Initial version
December 17th, 2019 - Final first draft
August 15th, 2022 - Changes to align with 02-client-refactor in [\#813](https://github.com/cosmos/ibc/pull/813)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
