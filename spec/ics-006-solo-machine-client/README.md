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

This specification document describes a client (verification algorithm) for a solo machine with a single updateable public key which implements the [ICS 2](../ics-002-client-semantics) interface.

### Motivation

Solo machines — which might be devices such as phones, browsers, or laptops — might like to interface with other machines & replicated ledgers which speak IBC, and they can do so through the uniform client interface.

### Definitions

Functions & terms are as defined in [ICS 2](../ics-002-client-semantics).

### Desired Properties

This specification must satisfy the client interface defined in [ICS 2](../ics-002-client-semantics).

Conceptually, we assume "big table of signatures in the universe" - that signatures produced are public - and incorporate replay protection accordingly.

## Technical Specification

This specification contains implementations for all of the functions defined by [ICS 2](../ics-002-client-semantics).

### Client state

The `ClientState` of a solo machine is simply whether or not the client is frozen.

```typescript
interface ClientState {
  frozen: boolean
}
```

### Consensus state

The `ConsensusState` of a solo machine consists of the current public key & sequence number.

```typescript
interface ConsensusState {
  sequence: uint64
  publicKey: PublicKey
}
```

### Headers

`Header`s must only be provided by a solo machine when the machine wishes to update the public key.

```typescript
interface Header {
  sequence: uint64
  signature: Signature
  newPublicKey: PublicKey
}
```

### Evidence

`Evidence` of solo machine misbehaviour consists of a sequence and two signatures over different messages at that sequence.

```typescript
interface Evidence {
  sequence: uint64
  signatureOne: Signature
  signatureTwo: Signature
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

### Validity predicate

The solo machine client `checkValidityAndUpdateState` function checks that the currently registered public key has signed over the new public key with the correct sequence.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
  assert(sequence === clientState.consensusState.sequence)
  assert(checkSignature(header.newPublicKey, header.sequence, header.signature))
  clientState.consensusState.publicKey = header.newPublicKey
  clientState.consensusState.sequence++
}
```

### Misbehaviour predicate

Any duplicate signature on different messages by the current public key freezes a solo machine client.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    h1 = evidence.h1
    h2 = evidence.h2
    pubkey = clientState.consensusState.publicKey
    assert(evidence.h1.signature.data !== evidence.h2.signature.data)
    assert(checkSignature(pubkey, evidence.sequence, evidence.h1.signature))
    assert(checkSignature(pubkey, evidence.sequence, evidence.h2.signature))
    clientState.frozen = true
}
```

### State verification functions

All solo machine client state verification functions simply check a signature, which must be provided by the solo machine.

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusState")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + consensusState
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connection/{connectionIdentifier}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + connectionEnd
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyChannelState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + channelEnd
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyPacketCommitment(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  commitment: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + commitment
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyPacketAcknowledgement(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + acknowledgement
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyPacketAcknowledgementAbsence(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyNextSequenceRecv(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/nextSequenceRecv")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + nextSequenceRecv
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}
```

### Properties & Invariants

Instantiates the interface defined in [ICS 2](../ics-002-client-semantics).

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

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
