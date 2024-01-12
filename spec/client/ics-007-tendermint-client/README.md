---
ics: 7
title: Tendermint Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
version compatibility: ibc-go v7.0.0
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-10
modified: 2019-12-19
---

## Synopsis

This specification document describes a client (verification algorithm) for a blockchain using Tendermint consensus.

### Motivation

State machines of various sorts replicated using the Tendermint consensus algorithm might like to interface with other replicated state machines or solo machines over IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

`currentTimestamp` is as defined in [ICS 24](../../core/ics-024-host-requirements).

The Tendermint light client uses the generalised Merkle proof format as defined in [ICS 23](../../core/ics-023-vector-commitments).

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

Arguably, this is disadvantageous, since `A_1` was just "lucky" in having been updated when `A_2` was not, and clearly some Byzantine fault has happened that should probably be dealt with by human or governance system intervention. The idea of "would-have-been-fooled" is to allow this to be detected by having `A_1` start from a configurable past header to detect misbehaviour (so in this case, `A_1` would be able to start from `h_0` and would also be frozen).

There is a free parameter here - namely, how far back is `A_1` willing to go (how big can `n` be where `A_1` will still be willing to look up `h_0`, having been updated to `h_0 + n`)? There is also a countervailing concern, in and of that double-signing is presumed to be costless after the unbonding period has passed, and we don't want to open up a denial-of-service vector for IBC clients.

The necessary condition is thus that `A_1` should be willing to look up headers as old as it has stored, but should also enforce the "unbonding period" check on the misbehaviour, and avoid freezing the client if the misbehaviour is older than the unbonding period (relative to the client's local timestamp). If there are concerns about clock skew a slight delta could be added.

## Technical Specification

This specification depends on correct instantiation of the [Tendermint consensus algorithm](https://github.com/tendermint/spec/blob/master/spec/consensus/consensus.md) and [light client algorithm](https://github.com/tendermint/spec/blob/master/spec/light-client).

### Client state

The Tendermint client state tracks the current revision, current validator set, trusting period, unbonding period, latest height, latest timestamp (block time), and a possible frozen height.

```typescript
interface ClientState {
  chainID: string
  trustLevel: Rational
  trustingPeriod: uint64
  unbondingPeriod: uint64
  latestHeight: Height
  frozenHeight: Maybe<uint64>
  upgradePath: []string
  maxClockDrift: uint64
  proofSpecs: []ProofSpec
}
```

### Consensus state

The Tendermint client tracks the timestamp (block time), the hash of the next validator set, and commitment root for all previously verified consensus states (these can be pruned after the unbonding period has passed, but should not be pruned beforehand).

```typescript
interface ConsensusState {
  timestamp: uint64
  nextValidatorsHash: []byte
  commitmentRoot: []byte
}
```

### Height

The height of a Tendermint client consists of two `uint64`s: the revision number, and the height in the revision.

```typescript
interface Height {
  revisionNumber: uint64
  revisionHeight: uint64
}
```

Comparison between heights is implemented as follows:

```typescript
function compare(a: TendermintHeight, b: TendermintHeight): Ord {
  if (a.revisionNumber < b.revisionNumber)
    return LT
  else if (a.revisionNumber === b.revisionNumber)
    if (a.revisionHeight < b.revisionHeight)
      return LT
    else if (a.revisionHeight === b.revisionHeight)
      return EQ
  return GT
}
```

This is designed to allow the height to reset to `0` while the revision number increases by one in order to preserve timeouts through zero-height upgrades.

### Headers

The Tendermint headers include the height, the timestamp, the commitment root, the hash of the validator set, the hash of the next validator set, and the signatures by the validators who committed the block. The header submitted to the on-chain client also includes the entire validator set, and a trusted height and validator set to update from. This reduces the amount of state maintained by the on-chain client and prevents race conditions on relayer updates.

```typescript
interface TendermintSignedHeader {
  height: uint64
  timestamp: uint64
  commitmentRoot: []byte
  validatorsHash: []byte
  nextValidatorsHash: []byte
  signatures: []Signature
}
```

```typescript
interface Header {
  TendermintSignedHeader
  identifier: string
  validatorSet: List<Pair<Address, uint64>>
  trustedHeight: Height
  trustedValidatorSet: List<Pair<Address, uint64>>
}

// GetHeight will return the header Height in the IBC ClientHeight
// format.
// Implementations may use the revision number to increment the height
// across height-resetting upgrades. See ibc-go for an example
func (Header) GetHeight() {
  return Height{0, height}
}
```

Header implements `ClientMessage` interface.

### `Misbehaviour`
 
The `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
Tendermint client `Misbehaviour` consists of two headers at the same height both of which the light client would have considered valid.

```typescript
interface Misbehaviour {
  identifier: string
  h1: Header
  h2: Header
}
```

Misbehaviour implements `ClientMessage` interface.

### Client initialisation

Tendermint client initialisation requires a (subjectively chosen) latest consensus state, including the full validator set.

```typescript
function initialise(
  identifier: Identifier, 
  clientState: ClientState, 
  consensusState: ConsensusState
) {
  assert(clientState.trustingPeriod < clientState.unbondingPeriod)
  assert(clientState.height > 0)
  assert(clientState.trustLevel >= 1/3 && clientState.trustLevel <= 1)

  provableStore.set("clients/{identifier}/clientState", clientState)
  provableStore.set("clients/{identifier}/consensusStates/{height}", consensusState)
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
function verifyClientMessage(clientMsg: ClientMessage) {
  switch typeof(clientMsg) {
    case Header:
      verifyHeader(clientMsg)
    case Misbehaviour:
      verifyHeader(clientMsg.h1)
      verifyHeader(clientMsg.h2)
  }
}
```

Verify validity of regular update to the Tendermint client

```typescript
function verifyHeader(header: Header) {
  clientState = provableStore.get("clients/{header.identifier}/clientState")
  // assert trusting period has not yet passed
  assert(currentTimestamp() - clientState.latestTimestamp < clientState.trustingPeriod)
  // assert header timestamp is less than trust period in the future. This should be resolved with an intermediate header.
  assert(header.timestamp - clientState.latestTimeStamp < clientState.trustingPeriod)
  // trusted height revision must be the same as header revision
  // if revisions are different, use upgrade client instead
  // trusted height must be less than header height
  assert(header.GetHeight().revisionNumber == header.trustedHeight.revisionNumber)
  assert(header.GetHeight().revisionHeight > header.trustedHeight.revisionHeight)
  // fetch the consensus state at the trusted height
  consensusState = provableStore.get("clients/{header.identifier}/consensusStates/{header.trustedHeight}")
  // assert that header's trusted validator set hashes to consensus state's validator hash
  assert(hash(header.trustedValidatorSet) == consensusState.nextValidatorsHash)

  // call the tendermint client's `verify` function
  assert(tmClient.verify(
    header.trustedValidatorSet,
    clientState.latestHeight,
    clientState.trustingPeriod,
    clientState.maxClockDrift,
    header.TendermintSignedHeader,
  ))
}
```

### Retrieve Client Status 

Return the Status of the Tendermint client. Status can be either Active, Expired, Unknown or Frozen. 

```typescript
// Returns the status of a client given its store.
function Status (client: clientState) {
  if (client.FrozenHeight !== 0) {
    return Frozen
  }
  // Get latest consensus state from clientStore to check for expiry
  consState, err := client.latestClientHeight()
  if err (!== nil) {
    return Unknown
  }
  // Check if Expired
  let expirationTime := consState.Timestamp + client.TrustingPeriod
  if (expirationTime <== now){
    return Expired 
  }

  return Active
} 
```

### Misbehaviour predicate

Function `checkForMisbehaviour` will check if an update contains evidence of Misbehaviour. If the ClientMessage is a header we check for implicit evidence of misbehaviour by checking if there already exists a conflicting consensus state in the store or if the header breaks time monotonicity.

```typescript
function checkForMisbehaviour(clientMsg: clientMessage): boolean {
  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  switch typeof(clientMsg) {
    case Header:
      // fetch consensusstate at header height if it exists
      consensusState = provableStore.get("clients/{clientMsg.identifier}/consensusStates/{header.GetHeight()}")
      // if consensus state exists and conflicts with the header
      // then the header is evidence of misbehaviour
      if consensusState != nil && 
          !(
          consensusState.timestamp == header.timestamp &&
          consensusState.commitmentRoot == header.commitmentRoot &&
          consensusState.nextValidatorsHash == header.nextValidatorsHash
          ) {
        return true
      }

      // check for time monotonicity misbehaviour
      // if header is not monotonically increasing with respect to neighboring consensus states
      // then return true
      // NOTE: implementation must have ability to iterate ascending/descending by height
      prevConsState = getPreviousConsensusState(header.GetHeight())
      nextConsState = getNextConsensusState(header.GetHeight())
      if prevConsState.timestamp >= header.timestamp {
        return true
      }
      if nextConsState != nil && nextConsState.timestamp <= header.timestamp {
        return true
      }
    case Misbehaviour:
      if (misbehaviour.h1.height < misbehaviour.h2.height) {
        return false
      }
      // if heights are equal check that this is valid misbehaviour of a fork
      if (misbehaviour.h1.height === misbehaviour.h2.height && misbehaviour.h1.commitmentRoot !== misbehaviour.h2.commitmentRoot) {
        return true
      }
      // otherwise if heights are unequal check that this is valid misbehavior of BFT time violation
      if (misbehaviour.h1.timestamp <= misbehaviour.h2.timestamp) {
        return true
      }

      return false
  }
}
```

### Update state

Function `updateState` will perform a regular update for the Tendermint client. It will add a consensus state to the client store. If the header is higher than the lastest height on the `clientState`, then the `clientState` will be updated.

```typescript
function updateState(clientMsg: clientMessage) {
  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  header = Header(clientMessage)
  // only update the clientstate if the header height is higher
  // than clientState latest height
  if clientState.height < header.GetHeight() {
    // update latest height
    clientState.latestHeight = header.GetHeight()

    // save the client
    provableStore.set("clients/{clientMsg.identifier}/clientState", clientState)
  }

  // create recorded consensus state, save it
  consensusState = ConsensusState{header.timestamp, header.nextValidatorsHash, header.commitmentRoot}
  provableStore.set("clients/{clientMsg.identifier}/consensusStates/{header.GetHeight()}", consensusState)

  // these may be stored as private metadata within the client in order to verify
  // that the delay period has passed in proof verification
  provableStore.set("clients/{clientMsg.identifier}/processedTimes/{header.GetHeight()}", currentTimestamp())
  provableStore.set("clients/{clientMsg.identifier}/processedHeights/{header.GetHeight()}", currentHeight())
}
```

### Update state on misbehaviour

Function `updateStateOnMisbehaviour` will set the frozen height to a non-zero sentinel height to freeze the entire client.

```typescript
function updateStateOnMisbehaviour(clientMsg: clientMessage) {
  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  clientState.frozenHeight = Height{0, 1}
  provableStore.set("clients/{clientMsg.identifier}/clientState", clientState)
}
```

### Upgrades

The chain which this light client is tracking can elect to write a special pre-determined key in state to allow the light client to update its client state (e.g. with a new chain ID or revision) in preparation for an upgrade.

As the client state change will be performed immediately, once the new client state information is written to the predetermined key, the client will no longer be able to follow blocks on the old chain, so it must upgrade promptly.

```typescript
function upgradeClientState(
  clientState: ClientState,
  newClientState: ClientState,
  height: Height,
  proof: CommitmentProof
) {
  // assert trusting period has not yet passed
  assert(currentTimestamp() - clientState.latestTimestamp < clientState.trustingPeriod)
  // check that the revision has been incremented
  assert(newClientState.latestHeight.revisionNumber > clientState.latestHeight.revisionNumber)
  // check proof of updated client state in state at predetermined commitment prefix and key
  path = applyPrefix(clientState.upgradeCommitmentPrefix, clientState.upgradeKey)
  // check that the client is at a sufficient height
  assert(clientState.latestHeight >= height)
  // check that the client is unfrozen or frozen at a higher height
  assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
  // fetch the previously verified commitment root & verify membership
  // Implementations may choose how to pass in the identifier
  // ibc-go provides the identifier-prefixed store to this method
  // so that all state reads are for the client in question
  consensusState = provableStore.get("clients/{clientIdentifier}/consensusStates/{height}")
  // verify that the provided consensus state has been stored
  assert(verifyMembership(consensusState.commitmentRoot, proof, path, newClientState))
  // update client state
  clientState = newClientState
  provableStore.set("clients/{clientIdentifier}/clientState", clientState)
}
```

### State verification functions

Tendermint client state verification functions check a Merkle proof against a previously validated commitment root.

These functions utilise the `proofSpecs` with which the client was initialised.

```typescript
function verifyMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: []byte
): Error {
  // check that the client is at a sufficient height
  assert(clientState.latestHeight >= height)
  // check that the client is unfrozen or frozen at a higher height
  assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
  // assert that enough time has elapsed
  assert(currentTimestamp() >= processedTime + delayPeriodTime)
  // assert that enough blocks have elapsed
  assert(currentHeight() >= processedHeight + delayPeriodBlocks)
  // fetch the previously verified commitment root & verify membership
  // Implementations may choose how to pass in the identifier
  // ibc-go provides the identifier-prefixed store to this method
  // so that all state reads are for the client in question
  consensusState = provableStore.get("clients/{clientIdentifier}/consensusStates/{height}")
  // verify that <path, value> has been stored
  if !verifyMembership(consensusState.commitmentRoot, proof, path, value) {
    return error
  }
  return nil
}

function verifyNonMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath
): Error {
  // check that the client is at a sufficient height
  assert(clientState.latestHeight >= height)
  // check that the client is unfrozen or frozen at a higher height
  assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
  // assert that enough time has elapsed
  assert(currentTimestamp() >= processedTime + delayPeriodTime)
  // assert that enough blocks have elapsed
  assert(currentHeight() >= processedHeight + delayPeriodBlocks)
  // fetch the previously verified commitment root & verify membership
  // Implementations may choose how to pass in the identifier
  // ibc-go provides the identifier-prefixed store to this method
  // so that all state reads are for the client in question
  consensusState = provableStore.get("clients/{clientIdentifier}/consensusStates/{height}")
  // verify that nothing has been stored at path
  if !verifyNonMembership(consensusState.commitmentRoot, proof, path) {
    return error
  }
  return nil
}
```

### Properties & Invariants

Correctness guarantees as provided by the Tendermint light client algorithm.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementations

- Implementation of ICS 07 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).
- Implementation of ICS 07 in Rust can be found in [ibc-rs repository](https://github.com/cosmos/ibc-rs).

## History

December 10th, 2019 - Initial version
December 19th, 2019 - Final first draft

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
