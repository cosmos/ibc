---
ics: 7
title: Tendermint Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-10
modified: 2019-12-18
---

## Synopsis

This specification document describes a client (verification algorithm) for a blockchain using Tendermint consensus.

### Motivation

State machines of various sorts replicated using the Tendermint consensus algorithm might like to interface with other replicated state machines or solo machines over IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../ics-002-client-semantics).

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

### Client state

The Tendermint client state tracks the current validator set, latest height, and a possible frozen height.

```typescript
interface ClientState {
  validatorSet: List<Pair<Address, uint64>>
  latestHeight: uint64
  frozenHeight: Maybe<uint64>
}
```

### Consensus state

The Tendermint client tracks the validator set hash & commitment root for all previously verified consensus states (these can be pruned after awhile).

```typescript
interface ConsensusState {
  validatorSetHash: []byte
  commitmentRoot: []byte
}
```

### Headers

The Tendermint client headers include a height, the commitment root, the complete validator set, and the signatures by the validators who committed the block.

```typescript
interface Header {
  height: uint64
  commitmentRoot: []byte
  validatorSet: List<Pair<Address, uint64>>
  signatures: []Signature
}
```

### Evidence

Tendermint client `Evidence` consists of two headers at the same height both of which the light client would have considered valid.

```typescript
interface Evidence {
  h1: Header
  h2: Header
}
```

### Client initialisation

```typescript
function initialize(consensusState: ConsensusState): ClientState {
  return {
    consensusState,
    frozenHeight: null,
    pastHeaders: Map.singleton(consensusState.latestHeight, consensusState.latestHeader)
  }
}
```

### Validity predicate

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    // TODO: check height
    // TODO: call verify function, performing bisection
    // TODO: update latest height
    // TODO: update latest header
    // TODO: store verified header info
}
```

### Misbehaviour predicate

```typescript
// misbehaviour verification function defined by the client type
// any duplicate signature by a past or current key freezes the client
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    // TODO: check equivocation or "would have been fooled"
    // TODO: set frozen height accordingly
}
```

### State verification functions

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusState")
    // TODO: check frozen height
    // TODO: return root.verifyMembership
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connection/{connectionIdentifier}")
    // TODO: check frozen height
    // TODO: return root.verifyMembership
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
    // TODO: check frozen height
    // TODO: return root.verifyMembership
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
    // TODO: check frozen height
    // TODO: return root.verifyMembership
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
    // TODO: check frozen height
    // TODO: return root.verifyMembership
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
    // TODO: check frozen height
    // TODO: return root.verifyMembership
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
    // TODO: check frozen height
    // TODO: return root.verifyMembership
}
```

### Properties & Invariants

Correctness guarantees as provided by the Tendermint light client algorithm.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementation

None yet.

## Other Implementations

None at present.

## History

December 10th, 2019 - Initial version
December 18th, 2019 - Final first draft

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
