---
ics: 7
title: Tendermint Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-10
modified: 2019-12-19
---

## Synopsis

This specification document describes a client (verification algorithm) for a blockchain using Tendermint consensus.

### Motivation

State machines of various sorts replicated using the Tendermint consensus algorithm might like to interface with other replicated state machines or solo machines over IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../ics-002-client-semantics).

The Tendermint light client uses the generalised Merkle proof format as defined in ICS 8.

`hash` is a generic collision-resistant hash function, and can easily be configured.

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification depends on correct instantiation of the [Tendermint consensus algorithm](https://github.com/tendermint/spec/blob/master/spec/consensus/consensus.md) and [light client algorithm](https://github.com/tendermint/spec/blob/master/spec/consensus/light-client.md).

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

The `Evidence` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
Tendermint client `Evidence` consists of two headers at the same height both of which the light client would have considered valid.

```typescript
interface Evidence {
  fromValidatorSet: List<Pair<Address, uint64>>
  fromHeight: uint64
  h1: Header
  h2: Header
}
```

### Client initialisation

Tendermint client initialisation requires a (subjectively chosen) latest consensus state, including the full validator set.

```typescript
function initialise(consensusState: ConsensusState, validatorSet: List<Pair<Address, uint64>>, height: uint64): ClientState {
  return ClientState{
    validatorSet,
    latestHeight: height,
    pastHeaders: Map.singleton(latestHeight, consensusState)
  }
}
```

The Tendermint client `latestClientHeight` function returns the latest stored height, which is updated every time a new (more recent) header is validated.

```typescript
function latestClientHeight(clientState: ClientState): uint64 {
  return clientState.latestHeight
}
```

### Validity predicate

Tendermint client validity checking uses the bisection algorithm described in the [Tendermint spec](https://github.com/tendermint/spec/blob/master/spec/consensus/light-client.md). If the provided header is valid, the client state is updated & the newly verified commitment written to the store.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    // assert that header is newer than any we know
    assert(header.height < clientState.latestHeight)
    // call the `verify` function
    assert(verify(clientState.validatorSet, clientState.latestHeight, header))
    // update latest height
    clientState.latestHeight = header.height
    // create recorded consensus state, save it
    consensusState = ConsensusState{validatorSet.hash(), header.commitmentRoot}
    set("clients/{identifier}/consensusStates/{header.height}", consensusState)
    // save the client
    set("clients/{identifier}", clientState)
}
```

### Misbehaviour predicate

Tendermint client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    // assert that the heights are the same
    assert(evidence.h1.height === evidence.h2.height)
    // assert that the commitments are different
    assert(evidence.h1.commitmentRoot !== evidence.h2.commitmentRoot)
    // fetch the previously verified commitment root & validator set hash
    consensusState = get("clients/{identifier}/consensusStates/{evidence.fromHeight}")
    // check that the validator set matches
    assert(consensusState.validatorSetHash === evidence.fromValidatorSet.hash())
    // check if the light client "would have been fooled"
    assert(
      verify(evidence.fromValidatorSet, evidence.fromHeight, evidence.h1) &&
      verify(evidence.fromValidatorSet, evidence.fromHeight, evidence.h2)
      )
    // set the frozen height
    clientState.frozenHeight = evidence.h1.height // which is same as h2.height
    // save the client
    set("clients/{identifier}", clientState)
}
```

### State verification functions

Tendermint client state verification functions check a Merkle proof against a previously validated commitment root.

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusState/{consensusStateHeight}")
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided consensus state has been stored
    assert(root.verifyMembership(path, consensusState, proof))
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connections/{connectionIdentifier}")
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided connection end has been stored
    assert(root.verifyMembership(path, connectionEnd, proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided channel end has been stored
    assert(root.verifyMembership(path, channelEnd, proof))
}

function verifyPacketData(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided commitment has been stored
    assert(root.verifyMembership(path, hash(data), proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided acknowledgement has been stored
    assert(root.verifyMembership(path, hash(acknowledgement), proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that no acknowledgement has been stored
    assert(root.verifyNonMembership(path, proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the nextSequenceRecv is as claimed
    assert(root.verifyMembership(path, nextSequenceRecv, proof))
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
December 19th, 2019 - Final first draft

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
