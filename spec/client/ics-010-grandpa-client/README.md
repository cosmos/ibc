---
ics: 10
title: GRANDPA Client
stage: draft
category: IBC/TAO
kind: instantiation
author: Yuanchao Sun <ys@cdot.network>, John Wu <john@cdot.network>
created: 2020-03-15
implements: 2
---

## Synopsis

This specification document describes a client (verification algorithm) for a blockchain using GRANDPA.

GRANDPA (GHOST-based Recursive Ancestor Deriving Prefix Agreement) is a finality gadget that will be used by the Polkadot relay chain. It now has a Rust implementation and is part of the Substrate, so likely blockchains built using Substrate will use GRANDPA as its finality gadget.

### Motivation

Blockchains using GRANDPA finality gadget might like to interface with other replicated state machines or solo machines over IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification depends on correct instantiation of the [GRANDPA finality gadget](https://github.com/w3f/consensus/blob/master/pdf/grandpa.pdf) and its light client algorithm.

### Client state

The GRANDPA client state tracks latest height and a possible frozen height.

```typescript
interface ClientState {
  latestHeight: uint64
  frozenHeight: Maybe<uint64>
}
```

### Authority set

A set of authorities for GRANDPA.

```typescript
interface AuthoritySet {
  // this is incremented every time the set changes
  setId: uint64
  authorities: List<Pair<AuthorityId, AuthorityWeight>>
}
```

### Consensus state

The GRANDPA client tracks authority set and commitment root for all previously verified consensus states.

```typescript
interface ConsensusState {
  authoritySet: AuthoritySet
  commitmentRoot: []byte
}
```

### Headers

The GRANDPA client headers include the height, the commitment root, a justification of block and authority set.
(In fact, here is a proof of authority set rather than the authority set itself, but we can use a fixed key to verify
the proof and extract the real set, the details are ignored here)

```typescript
interface Header {
  height: uint64
  commitmentRoot: []byte
  justification: Justification
  authoritySet: AuthoritySet
}
```

### Justification

A GRANDPA justification for block finality, it includes a commit message and an ancestry proof including all headers routing all precommit target blocks to the commit target block.
For example, the latest blocks are A - B - C - D - E - F, where A is the last finalised block, F is the point where a majority for vote (they may on B, C, D, E, F) can be collected. Then the proof needs to include all headers from F back to A.

```typescript
interface Justification {
  round: uint64
  commit: Commit
  votesAncestries: []Header
}
```

### Commit

A commit message which is an aggregate of signed precommits.

```typescript
interface Commit {
  precommits: []SignedPrecommit
}

interface SignedPrecommit {
  targetHash: Hash
  signature: Signature
  id: AuthorityId
}
```

### Misbehaviour

The `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
GRANDPA client `Misbehaviour` consists of two headers at the same height both of which the light client would have considered valid.

```typescript
interface Misbehaviour {
  fromHeight: uint64
  h1: Header
  h2: Header
}
```

### Client initialisation

GRANDPA client initialisation requires a (subjectively chosen) latest consensus state, including the full authority set.

```typescript
function initialise(identifier: Identifier, height: uint64, consensusState: ConsensusState): ClientState {
    set("clients/{identifier}/consensusStates/{height}", consensusState)
    return ClientState{
      latestHeight: height,
      frozenHeight: null,
    }
}
```

The GRANDPA client `latestClientHeight` function returns the latest stored height, which is updated every time a new (more recent) header is validated.

```typescript
function latestClientHeight(clientState: ClientState): uint64 {
  return clientState.latestHeight
}
```

### Validity predicate

GRANDPA client validity checking verifies a header is signed by the current authority set and verifies the authority set proof to determine if there is an expected change to the authority set. If the provided header is valid, the client state is updated & the newly verified commitment written to the store.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    // assert header height is newer than any we know
    assert(header.height > clientState.latestHeight)
    consensusState = get("clients/{identifier}/consensusStates/{clientState.latestHeight}")
    // verify that the provided header is valid
    assert(verify(consensusState.authoritySet, header))
    // update latest height
    clientState.latestHeight = header.height
    // create recorded consensus state, save it
    consensusState = ConsensusState{header.authoritySet, header.commitmentRoot}
    set("clients/{identifier}/consensusStates/{header.height}", consensusState)
    // save the client
    set("clients/{identifier}", clientState)
}

function verify(
  authoritySet: AuthoritySet,
  header: Header): boolean {
  let visitedHashes: Hash[]
  for (const signedPrecommit of Header.justification.commit.precommits) {
    if (checkSignature(authoritySet, signedPrecommit)) {
      visitedHashes.push(signedPrecommit.targetHash)
    }
  }
  return visitedHashes.equals(Header.justification.votesAncestries.map(hash))
}
```

### Misbehaviour predicate

GRANDPA client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  misbehaviour: Misbehaviour) {
    // assert that the heights are the same
    assert(misbehaviour.h1.height === misbehaviour.h2.height)
    // assert that the commitments are different
    assert(misbehaviour.h1.commitmentRoot !== misbehaviour.h2.commitmentRoot)
    // fetch the previously verified commitment root & authority set
    consensusState = get("clients/{identifier}/consensusStates/{misbehaviour.fromHeight}")
    // check if the light client "would have been fooled"
    assert(
      verify(consensusState.authoritySet, misbehaviour.h1) &&
      verify(consensusState.authoritySet, misbehaviour.h2)
      )
    // set the frozen height
    clientState.frozenHeight = min(clientState.frozenHeight, misbehaviour.h1.height) // which is same as h2.height
    // save the client
    set("clients/{identifier}", clientState)
}
```

### State verification functions

GRANDPA client state verification functions check a Merkle proof against a previously validated commitment root.

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

function verifyPacketReceiptAbsence(
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

Correctness guarantees as provided by the GRANDPA light client algorithm.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementations

None yet.

## History

March 15, 2020 - Initial version

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
