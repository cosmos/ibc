---
ics: 7
title: Tendermint Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-10
modified: 2019-12-10
---

## Synopsis

Client for Tendermint consensus.

### Motivation

(rationale for existence of standard)

### Definitions

(definitions of any new terms not defined in common documentation)

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

(main part of standard document - not all subsections are required)

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Data Structures

(new data structures, if applicable)

### Sub-protocols

```typescript
interface Header {
  height: uint64
  commitmentRoot: []byte
  signatures: []Signature
}

interface StoredHeader {
  validatorSetHash: []byte
  commitmentRoot: []byte
}

interface ClientState {
  consensusState: ConsensusState
  pastHeaders: Map<uint64, StoredHeader>
  frozenHeight: Maybe<uint64>
}

interface ConsensusState {
  validatorSet: List<Pair<Address, uint64>>
  latestHeight: uint64
  latestHeader: StoredHeader
}

interface Evidence {
  h1: Header
  h2: Header
}

// initialisation function defined by the client type
function initialize(consensusState: ConsensusState): ClientState {
  return {
    consensusState,
    frozenHeight: null,
    pastHeaders: Map.singleton(consensusState.latestHeight, consensusState.latestHeader)
  }
}

// validity predicate function defined by the client type
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    // TODO: check height
    // TODO: call verify function, performing bisection
    // TODO: update latest height
    // TODO: update latest header
    // TODO: store verified header info
}

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

// misbehaviour verification function defined by the client type
// any duplicate signature by a past or current key freezes the client
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    // TODO: check equivocation or "would have been fooled"
    // TODO: set frozen height accordingly
}
```

### Properties & Invariants

Correctness guarantees as provided by the Tendermint light client algorithm.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementation

(link to or description of concrete example implementation)

## Other Implementations

None at present.

## History

December 10th, 2019 - Initial version

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
