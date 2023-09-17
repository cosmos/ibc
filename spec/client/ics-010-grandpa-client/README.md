---
ics: 10
title: GRANDPA Client
stage: draft
category: IBC/TAO
kind: instantiation
author: Julian Sun <julian@oct.network>, Rivers Yang <rivers@oct.network>
created: 2020-03-15
modified: 2022-11-11
implements: 2
---

## Synopsis

This specification document describes a client (verification algorithm) for a blockchain using GRANDPA finality gadget with BEEFY protocol.

GRANDPA (GHOST-based Recursive ANcestor Deriving Prefix Agreement) is the finality gadget that is implemented for the Polkadot Relay Chain. It now has a Rust implementation and is part of the Substrate. Solo chains built using Substrate may also use GRANDPA as its finality gadget.

[The BEEFY protocol](https://github.com/paritytech/grandpa-bridge-gadget/blob/master/docs/beefy.md) is a gadget that runs alongside GRANDPA finality gadget. It is designed to build a light client for GRANDPA that can be efficiently verified. Refer to [the Octopus appchain template](https://github.com/octopus-network/barnacle/blob/44ad1f12a838b98c9592e9aba4b2c8d0407df34f/runtime/src/lib.rs#L493) to enable BEEFY in Substrate.

### Motivation

State machines of various sorts replicated using the GRANDPA finality gadget might like to interface with other replicated state machines or solo machines over IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification depends on the correct instantiation of the BEEFY protocol.
This specification also depends on the correct implementation of the following algorithms:
* `secp256k1Recover` to recover the public key from a secp256k1 signature.
* `verifyMerkleProof` to verify a Merkle proof.
* `verifyMMRProof` to verify an MMR proof. Refer to [Merkle Mountain Ranges](https://github.com/opentimestamps/opentimestamps-server/blob/master/doc/merkle-mountain-range.md).

### Client state

The GRANDPA client state tracks the latest MMR root, the latest height that the MMR root is updated, a possible frozen height, the current and the next validator set.

```typescript
interface ClientState {
  latestMMRRoot: []byte
  latestHeight: uint64
  frozenHeight: Maybe<uint64>
  currentValidatorSet: ValidatorSet
  nextValidatorSet: ValidatorSet
}
```

### Consensus state

The GRANDPA client tracks the commitment root for all previously verified consensus states (these can be pruned after the unbonding period has passed, but should not be pruned beforehand).

```typescript
interface ConsensusState {
  commitmentRoot: []byte
}
```

### Validator set

The validator set of a GRANDPA client consists of the set id, a Merkle root of validator addresses and the number of validators in the set.

```typescript
interface ValidatorSet {
  id: uint64
  root: []byte
  length: uint32
}
```

### MMRLeaf

The MMR leaf contains:
1. Block number and parent block hash.
2. Merkle Tree Root Hash of next BEEFY validator set.
3. Arbitrary extra leaf data to be used by downstream pallets to include custom data.

```typescript
interface MMRLeaf {
  version: uint8
  parentNumber: uint32
  parentHash: []byte
  beefyNextAuthoritySet: ValidatorSet
  leafExtra: []byte
}
```

### SignedMMRRoot

An MMR root signed by GRANDPA validators as part of the BEEFY protocol.
The MMR root also contains the height of the MMR root stored and the related validator set id.

```typescript

type Signature = [65]byte

interface SignedMMRRoot {
  root: []byte
  blockNumber: uint64
  validatorSetId: uint64
  signatures: []Maybe<Signature>
}
```

### MMRRoot

```typescript
interface MMRRoot {
  signedMMRRoot: SignedMMRRoot
  validatorMerkleProofs: []MerkleProof
}
```

MMRRoot implements `ClientMessage` interface.

### Headers

The GRANDPA headers include the height, the commitment root and the MMR proof of the header.

```typescript
interface Header {
  height: uint64
  commitmentRoot: []byte
  leaf: MMRLeaf
  proof: MMRProof
}
```

Header implements `ClientMessage` interface.

### Misbehaviour
 
The `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
GRANDPA client `Misbehaviour` consists of two signed MMR roots at the same height both of which the light client would have considered valid.

```typescript
interface Misbehaviour {
  fromHeight: uint64
  signedMMRRoot1: SignedMMRRoot
  validatorMerkleProofs1: []MerkleProof
  signedMMRRoot2: SignedMMRRoot
  validatorMerkleProofs2: []MerkleProof
}
```

Misbehaviour implements `ClientMessage` interface.

### Client initialisation

GRANDPA client initialisation requires a (subjectively chosen) latest height, including the full validator set at that height and the next validator set.

```typescript
function initialise(height: uint64, initialValidatorSet: ValidatorSet, nextValidatorSet: ValidatorSet): ClientState {
    assert(height > 0)
    return ClientState{
      latestMMRRoot: nil,
      latestHeight: height,
      frozenHeight: null,
      currentValidatorSet: initialValidatorSet,
      nextValidatorSet 
    }
}
```

The GRANDPA client `latestClientHeight` function returns the latest stored height, which is updated every time a new (more recent) MMR root is validated.

```typescript
function latestClientHeight(clientState: ClientState): Height {
  return clientState.latestHeight
}
```

### Validity predicate

GRANDPA client validity checking verifies that an MMR root is signed by the current validator set. If the provided MMR root is valid, the client state is updated & the newly verified MMR root is written to the store. The validity also checking a provided header can be validated by the MMR root. If the provided header is valid, the consensus state is updated. We use MMR root to advance the client state and relay headers on demand.

```typescript
function verifyClientMessage(
  clientMsg: ClientMessage) {
    switch typeof(clientMsg) {
      case MMRRoot:
        verifyMMR(clientMsg.signedMMRRoot, clientMsg.validatorMerkleProofs)
      case Header:
        verifyCommitment(clientMsg.commitmentRoot, clientMsg.leaf, clientMsg.proof)
      case Misbehaviour:
        verifyMMR(clientMsg.signedMMRRoot1, clientMsg.validatorMerkleProofs1)
        verifyMMR(clientMsg.signedMMRRoot2, clientMsg.validatorMerkleProofs2)
    }
}
```

```typescript
function verifyMMR(signedMMRRoot: SignedMMRRoot, validatorMerkleProofs: []MerkleProof) {
    clientState = get("clients/{header.identifier}/clientState")
    // the block height of the new MMR root must be greater than the latest height
    assert(signedMMRRoot.blockNumber > clientState.latestHeight)
    for _, signature := range signedMMRRoot.signatures {
      validatorAddress = secp256k1Recover(signature)
      assert(validatorAddress != null)
      found = false
      for _, proof := range validatorMerkleProofs {
        if proof.leaf == validatorAddress {
          found = true
          assert(verifyMerkleProof(clientState.currentValidatorSet.root, proof.leaf, proof.proof))
        }
      }
      assert(found)
    }
}
```

```typescript
function verifyCommitment(height: uint64, root: []byte, leaf: MMRLeaf, proof: MMRProof) {
    clientState = get("clients/{header.identifier}/clientState")
    // the block height of the header must be less than the latest height to verify by the latest MMR root.
    assert(height < clientState.latestHeight)
    assert(verifyMMRProof(root, leaf, proof))
}
```

### Misbehaviour predicate

GRANDPA client misbehaviour checking determines whether or not two conflicting signed MMR roots at the same height would have convinced the light client.

```typescript
function checkForMisbehaviour(
  clientMsg: clientMessage) => bool {
    switch typeof(clientMsg) {
      case MMRRoot:
      case Header:
      case Misbehaviour:
        // assert that the MMR roots are different
        assert(misbehaviour.signedMMRRoot1.root !== misbehaviour.signedMMRRoot2.root)
    }
}
```

### UpdateState

UpdateState will perform a regular update for the GRANDPA client. If the height of the new MMR root is higher than the latest height in the client state, then the client state will be updated. And commitment roots validated by MMR root will be stored in consensus state.

```typescript
function updateState(
  clientMsg: clientMessage) {
    clientState = get("clients/{clientMsg.identifier}/clientState")
    switch typeof(clientMsg) {
      case MMRRoot:
        signedMMRRoot = MMRRoot(clientMessage)
        // only update the client state if the MMR height is higher than latest height
        if clientState.height < signedMMRRoot.blockNumber {
          // update latest height
          clientState.latestHeight = signedMMRRoot.blockNumber
          clientState.latestMMRRoot = signedMMRRoot.root
          // save the client
          set("clients/{clientMsg.identifier}/clientState", clientState)
        }
      case Header:
        header = MMRRoot(clientMessage)
        // create recorded consensus state, save it
        consensusState = ConsensusState{header.commitmentRoot}
        set("clients/{clientMsg.identifier}/consensusStates/{header.height}", consensusState)
        // update the next validator set if it changes
        if clientState.currentValidatorSet.id + 1 == header.leaf.beefyNextAuthoritySet.id {
          clientState.currentValidatorSet = header.leaf.beefyNextAuthoritySet
        }
        // save the client
        set("clients/{clientMsg.identifier}/clientState", clientState)
      case Misbehaviour:
    }
}
```

### UpdateStateOnMisbehaviour

UpdateStateOnMisbehaviour will set the frozen height to a non-zero sentinel height to freeze the entire client.

```typescript
function updateStateOnMisbehaviour(clientMsg: clientMessage) {
    clientState = get("clients/{clientMsg.identifier}/clientState")
    misbehaviour = Misbehaviour(clientMessage)
    clientState.frozenHeight = misbehaviour.fromHeight
    set("clients/{clientMsg.identifier}/clientState", clientState)
}
```

### State verification functions

GRANDPA client state verification functions check a Merkle proof against a previously validated commitment root.

These functions utilise the `proofSpecs` with which the client was initialised.

```typescript
function verifyMembership(
  clientState: ClientState,
  height: Height,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: []byte) {
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    // Implementations may choose how to pass in the identifier
    // ibc-go provides the identifier-prefixed store to this method
    // so that all state reads are for the client in question
    root = get("clients/{clientIdentifier}/consensusStates/{height}")
    // verify that <path, value> has been stored
    assert(verifyMembership(root, proof, path, value))
}

function verifyNonMembership(
  clientState: ClientState,
  height: Height,
  proof: CommitmentProof,
  path: CommitmentPath) {
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    // Implementations may choose how to pass in the identifier
    // ibc-go provides the identifier-prefixed store to this method
    // so that all state reads are for the client in question
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that nothing has been stored at path
    assert(verifyMembership(root, proof, path))
}
```

### Properties & Invariants

Correctness guarantees as provided by the BEEFY protocol.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementations

None yet.

## History

March 15, 2020 - Initial version

November 11, 2022 - Update based on BEEFY protocol

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
