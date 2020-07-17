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

`currentTimestamp` is as defined in [ICS 24](../ics-024-host-requirements).

The Tendermint light client uses the generalised Merkle proof format as defined in ICS 8.

`hash` is a generic collision-resistant hash function, and can easily be configured.

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

#### Note on "would-have-been-fooled logic

The basic idea of "would-have-been-fooled" detection is that it allows us to be a bit more conservative, and freeze our light client when we know that another light client somewhere else on the network with a slightly different update pattern could have been fooled, even though we weren't.

Consider a topology of three chains - `A`, `B`, and `C`, and two clients for chain `A`, `A_1` and `A_2`, running on chains `B` and `C` respectively. The following sequence of events occurs:

- Chain `A` produces a block at height `h_0` (correctly).
- Clients `A_1` and `A_2` are updated to the block at height `h_0`.
- Chain `A` produces a block at height `h_0 + n` (correctly).
- Client `A_1` is updated to the block at height `h_0 + n` (client `A_2` is not yet updated).
- Chain `A` produces a second (equivocating) block at height `h_0 + k`, where `k <= n`.

*Without* "would-have-been-fooled", it will be possible to freeze client `A_2` (since there are two valid blocks at height `h_0 + k` which are newer than the latest header `A_2` knows), but it will *not*  be possible to freeze `A_1`, since `A_1` has already progressed beyond `h_0 + k`.

Arguably, this is disadvantageous, since `A_1` was just "lucky" in having been updated when `A_2` was not, and clearly some Byzantine fault has happened that should probably be deal with by human or governance system intervention. The idea of "would-have-been-fooled" is to allow this to be detected by having `A_1` start from a configurable past header to detect misbehaviour (so in this case, `A_1` would be able to start from `h_0` and would also be frozen).

There is a free parameter here - namely, how far back is `A_1` willing to go (how big can `n` be where `A_1` will still be willing to look up `h_0`, having been updated to `h_0 + n`)? There is also a countervailing concern, in and of that double-signing is presumed to be costless after the unbonding period has passed, and we don't want to open up a denial-of-service vector for IBC clients.

The necessary condition is thus that `A_1` should be willing to look up headers as old as it has stored, but should also enforce the "unbonding period" check on the evidence, and avoid freezing the client if the evidence is older than the unbonding period (relative to the client's local timestamp). If there are concerns about clock skew a slight delta could be added.

## Technical Specification

This specification depends on correct instantiation of the [Tendermint consensus algorithm](https://github.com/tendermint/spec/blob/master/spec/consensus/consensus.md) and [light client algorithm](https://github.com/tendermint/spec/blob/master/spec/consensus/light-client.md).

### Client state

The Tendermint client state tracks the current epoch, current validator set, trusting period, unbonding period, latest height, latest timestamp (block time), and a possible frozen height.

```typescript
interface ClientState {
  chainID: string
  validatorSet: List<Pair<Address, uint64>>
  trustLevel: Rational
  trustingPeriod: uint64
  unbondingPeriod: uint64
  latestHeight: Height
  latestTimestamp: uint64
  frozenHeight: Maybe<uint64>
  upgradeCommitmentPrefix: CommitmentPrefix
  upgradeKey: []byte
  maxClockDrift: uint64
  proofSpecs: []ProofSpec
}
```

### Consensus state

The Tendermint client tracks the timestamp (block time), validator set, and commitment root for all previously verified consensus states (these can be pruned after the unbonding period has passed, but should not be pruned beforehand).

```typescript
interface ConsensusState {
  timestamp: uint64
  validatorSet: List<Pair<Address, uint64>>
  commitmentRoot: []byte
}
```

### Height

The height of a Tendermint client consists of two `uint64`s: the epoch number, and the height in the epoch.

```typescript
interface Height {
  epochNumber: uint64
  epochHeight: uint64
}
```

Comparison between heights is implemented as follows:

```typescript
function compare(a: TendermintHeight, b: TendermintHeight): Ord {
  if (a.epochNumber < b.epochNumber)
    return LT
  else if (a.epochNumber === b.epochNumber)
    if (a.epochHeight < b.epochHeight)
      return LT
    else if (a.epochHeight === b.epochHeight)
      return EQ
  return GT
}
```

This is designed to allow the height to reset to `0` while the epoch number increases by one in order to preserve timeouts through zero-height upgrades.

### Headers

The Tendermint client headers include the height, the timestamp, the commitment root, the complete validator set, and the signatures by the validators who committed the block.

```typescript
interface Header {
  height: uint64
  timestamp: uint64
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
  fromHeight: Height
  h1: Header
  h2: Header
}
```

### Client initialisation

Tendermint client initialisation requires a (subjectively chosen) latest consensus state, including the full validator set.

```typescript
function initialise(
  chainID: string, consensusState: ConsensusState,
  validatorSet: List<Pair<Address, uint64>>, trustLevel: Fraction,
  height: Height, trustingPeriod: uint64, unbondingPeriod: uint64,
  upgradeCommitmentPrefix: CommitmentPrefix, upgradeKey: []byte,
  maxClockDrift: uint64, proofSpecs: []ProofSpec): ClientState {
    assert(trustingPeriod < unbondingPeriod)
    assert(height > 0)
    assert(trustLevel > 0 && trustLevel < 1)
    set("clients/{identifier}/consensusStates/{height}", consensusState)
    return ClientState{
      chainID,
      validatorSet,
      trustLevel,
      latestHeight: height,
      latestTimestamp: consensusState.timestamp,
      trustingPeriod,
      unbondingPeriod,
      frozenHeight: null,
      upgradeCommitmentPrefix,
      upgradeKey,
      maxClockDrift,
      proofSpecs
    }
}
```

The Tendermint client `latestClientHeight` function returns the latest stored height, which is updated every time a new (more recent) header is validated.

```typescript
function latestClientHeight(clientState: ClientState): Height {
  return clientState.latestHeight
}
```

### Validity predicate

Tendermint client validity checking uses the bisection algorithm described in the [Tendermint spec](https://github.com/tendermint/spec/tree/master/spec/consensus/light-client). If the provided header is valid, the client state is updated & the newly verified commitment written to the store.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  epoch: uint64,
  header: Header) {
    // assert epoch is correct
    assert(epoch === clientState.currentHeight.epoch)
    // check that epoch is encoded correctly in chain ID
    assert(epoch === clientState.chainID.regex('[a-z]*-(0)'))
    // assert trusting period has not yet passed. This should fatally terminate a connection.
    assert(currentTimestamp() - clientState.latestTimestamp < clientState.trustingPeriod)
    // assert header timestamp is less than trust period in the future. This should be resolved with an intermediate header.
    assert(header.timestamp - clientState.latestTimeStamp < trustingPeriod)
    // assert header timestamp is past current timestamp
    assert(header.timestamp > clientState.latestTimestamp)
    // assert header height is newer than any we know
    assert(header.height > clientState.latestHeight)
    // call the `verify` function
    assert(verify(clientState.validatorSet, clientState.latestHeight, clientState.trustingPeriod, maxClockDrift, header))
    // update validator set
    clientState.validatorSet = header.validatorSet
    // update latest height
    clientState.latestHeight = header.height
    // update latest timestamp
    clientState.latestTimestamp = header.timestamp
    // create recorded consensus state, save it
    consensusState = ConsensusState{header.validatorSet, header.commitmentRoot, header.timestamp}
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
    // fetch the previously verified commitment root & validator set
    consensusState = get("clients/{identifier}/consensusStates/{evidence.fromHeight}")
    // assert that the timestamp is not from more than an unbonding period ago
    assert(currentTimestamp() - evidence.timestamp < clientState.unbondingPeriod)
    // check if the light client "would have been fooled"
    assert(
      verify(consensusState.validatorSet, evidence.fromHeight, evidence.h1) &&
      verify(consensusState.validatorSet, evidence.fromHeight, evidence.h2)
      )
    // set the frozen height
    clientState.frozenHeight = min(clientState.frozenHeight, evidence.h1.height) // which is same as h2.height
    // save the client
    set("clients/{identifier}", clientState)
}
```

### Upgrades

The chain which this light client is tracking can elect to write a special pre-determined key in state to allow the light client to update its client state (e.g. with a new chain ID or epoch) in preparation for an upgrade.

As the client state change will be performed immediately, once the new client state information is written to the predetermined key, the client will no longer be able to follow blocks on the old chain, so it must upgrade promptly.

```typescript
function upgradeClientState(
  clientState: ClientState,
  newClientState: ClientState,
  height: Height,
  proof: CommitmentPrefix) {
    // check proof of updated client state in state at predetermined commitment prefix and key
    path = applyPrefix(clientState.upgradeCommitmentPrefix, clientState.upgradeKey)
     // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided consensus state has been stored
    assert(root.verifyMembership(path, newClientState, proof))
    // update client state
    clientState = newClientState
    set("clients/{identifier}", clientState)
}
```

### State verification functions

Tendermint client state verification functions check a Merkle proof against a previously validated commitment root.

These functions utilise the `proofSpecs` with which the client was initialised.

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: Height,
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
  height: Height,
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
  height: Height,
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
    assert(root.verifyMembership(clientState.proofSpecs, path, channelEnd, proof))
}

function verifyPacketData(
  clientState: ClientState,
  height: Height,
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
    assert(root.verifyMembership(clientState.proofSpecs, path, hash(data), proof))
}

function verifyPacketAcknowledgement(
  clientState: ClientState,
  height: Height,
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
    assert(root.verifyMembership(clientState.proofSpecs, path, hash(acknowledgement), proof))
}

function verifyPacketAcknowledgementAbsence(
  clientState: ClientState,
  height: Height,
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
    assert(root.verifyNonMembership(clientState.proofSpecs, path, proof))
}

function verifyNextSequenceRecv(
  clientState: ClientState,
  height: Height,
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
    assert(root.verifyMembership(clientState.proofSpecs, path, nextSequenceRecv, proof))
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
