---
ics: 9
title: Loopback Client
stage: draft
category: IBC/TAO
kind: instantiation
author: Christopher Goes <cwgoes@tendermint.com>
created: 2020-01-17
modified: 2020-01-17
requires: 2
implements: 2
---

## Synopsis

This specification describes a loop-back client, designed to be used for interaction over the IBC interface with modules present on the same ledger.

### Motivation

Loop-back clients may be useful in cases where the calling module does not have prior knowledge of where precisely the destination module lives and would like to use the uniform IBC message-passing interface (similar to `127.0.0.1` in TCP/IP).

### Definitions

Functions & terms are as defined in [ICS 2](../ics-002-client-semantics).

### Desired Properties

Intended client semantics should be preserved, and loop-back abstractions should be negligible cost.

## Technical Specification

### Data structures

No client state, consensus state, headers, or evidence data structures are required for a loopback client.

```typescript
type ClientState object

type ConsensusState object

type Header object

type Evidence object
```

### Client initialisation

No initialisation is necessary for a loopback client; an empty state is returned.

```typescript
function initialise(): ClientState {
  return {}
}
```

### Validity predicate

No validity checking is necessary in a loopback client; the function should never be called.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    assert(false)
}
```

### Misbehaviour predicate

No misbehaviour checking is necessary in a loopback client; the function should never be called.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    return
}
```

### State verification functions

Loop-back client state verification functions simply read the local state. Note that they will need (read-only) access to keys outside the client prefix.

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "consensusStates/{clientIdentifier}")
    assert(get(path) === consensusState)
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connection/{connectionIdentifier}")
    assert(get(path) === connectionEnd)
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
    assert(get(path) === channelEnd)
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
    assert(get(path) === commitment)
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
    assert(get(path) === acknowledgement)
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
    assert(get(path) === nil)
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
    assert(get(path) === nextSequenceRecv)
}
```

### Properties & Invariants

Semantics are as if this were a remote client of the local ledger.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client algorithm will require a new client standard.

## Example Implementation

Coming soon.

## Other Implementations

None at present.

## History

2020-01-17 - Initial version

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
