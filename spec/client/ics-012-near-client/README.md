---
ics: 12
title: NEAR Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 1
author: Rivers Yang <rivers@oct.network>
created: 2023-1-12
---

## Synopsis

This specification document describes a client (verification algorithm) for NEAR protocol.

### Motivation

State machine of NEAR protocol might like to interface with other replicated state machines or solo machines over IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

`hash` is a generic collision-resistant hash function. In NEAR protocol, it is `sha2::sha256` hash function. We define a new type for the result of hash function as:

```typescript
type CryptoHash = [32]byte
```

We also defines the types for public key and signature. In NEAR protocol, they are based on `ed25519` signature algorithm:

```typescript
type PublicKey = ED25519PublicKey
type Signature = ED25519Signature
```

`borsh` is a generic serialization function which follows the [Borsh serialization format](https://borsh.io/).

`merklize` is a generic function which can construct a merkle tree from an array which the element in it can be serialized by `borsh`. This function should return the merkle root of the tree at least.

In NEAR protocol, the block producers are changed by time (about 12 hours). The period is known as `epoch` and the id of a epoch is represented by a `CryptoHash`.

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification is based on the [NEAR light client specification](https://nomicon.io/ChainSpec/LightClient) and the implementation of [nearcore v1.30.0](https://github.com/near/nearcore/releases/tag/1.30.0) by adding necessary data fields and checking processes.

### Client state

The NEAR client state tracks the latest height and cached heights.

```typescript
interface ClientState {
    trustingPeriod: uint64
    latestHeight: Height
    latestTimestamp: uint64
    upgradeCommitmentPrefix: []byte
    upgradeKey: []bype
}
```

### Consensus state

The NEAR client tracks the block producers of current epoch and header (refer to [Headers section](#headers)) for all previously verified consensus states (these can be pruned after the unbonding period has passed, but should not be pruned beforehand).

```typescript
interface ValidatorStakeView {
    accountId: String
    publicKey: PublicKey
    stake: uint128
}

interface ConsensusState {
    currentBps: List<ValidatorStakeView>
    header: Header
}
```

### Height

The height of a NEAR client is an `uint64` number.

```typescript
interface Height {
    height: uint64
}
```

Comparison between heights is implemented as follows:

```typescript
function compare(a: Height, b: Height): Ord {
    if (a.height < b.height)
        return LT
    else if (a.height === b.height)
        return EQ
    return GT
}
```

### Headers

The NEAR client headers include the `LightClientBlockView` and previou state root of chunks. The entire block producers for next epoch and approvals for the block after the next are included in `LightClientBlockView`.

```typescript
interface BlockHeaderInnerLiteView {
    height: Height
    epochId: CryptoHash
    nextEpochId: CryptoHash
    prevStateRoot: CryptoHash
    outcomeRoot: CryptoHash
    timestampNanosec: uint64
    nextBpHash: CryptoHash
    blockMerkleRoot: CryptoHash
}

interface LightClientBlockView {
    prevBlockHash: CryptoHash
    nextBlockInnerHash: CryptoHash,
    innerLite: BlockHeaderInnerLiteView
    innerRestHash: CryptoHash
    nextBps: Maybe<List<ValidatorStakeView>>
    approvalsAfterNext: List<Maybe<Signature>>
}

interface Header {
    lightClientBlockView: LightClientBlockView
    prevStateRootOfChunks: List<CryptoHash>
}
```

The current block hash, next block hash and approval message can be calcuated from `LightClientBlockView`.

```typescript
function (LightClientBlockView) currentBlockHash(): CryptoHash {
    return hash(concat(
        hash(concat(
          hash(borsh(self.innerLite)),
          self.innerRestHash,
        )),
        self.prevBlockHash
    ))
}

function (LightClientBlockView) nextBlockHash(): CryptoHash {
    return hash(
        concat(self.nextBlockInnerHash,
        self.currentBlockHash()
    ))
}

enum ApprovalInner {
    Endorsement(CryptoHash),
    Skip(uint64)
}

function (LightClientBlockView) approvalMessage(): []byte {
    return concat(
        borsh(ApprovalInner::Endorsement(self.nextBlockHash())),
        littleEndian(self.innerLite.height + 2)
    )
}
```

Header implements `ClientMessage` interface.

### Misbehaviour

TBD (currently not applicable in NEAR protocol)

### Client initialisation

The NEAR client initialisation requires a latest consensus state (the latest header and the block producers of the epoch of the header).

```typescript
function initialise(
  trustingPeriod: uint64,
  latestTimestamp: uint64,
  upgradeCommitmentPrefix: []byte,
  upgradeKey: []bype,
  consensusState: ConsensusState): ClientState {
    assert(consensusState.currentBps.len > 0)
    assert(consensusState.header.getHeight() > 0)
    // implementations may define a identifier generation function
    identifier = generateClientIdentifier()
    set("clients/{identifier}/consensusStates/{consensusState.header.getHeight()}", consensusState)
    height = Height {
        height: consensusState.header.getHeight()
    }
    return ClientState {
        trustingPeriod
        latestHeight: height
        latestTimestamp
        upgradeCommitmentPrefix
        upgradeKey
    }
}
```

### Validity Predicate

The NEAR client validity checking uses spec described in the [NEAR light client specification](https://nomicon.io/ChainSpec/LightClient) by adding an extra checking for the previous state root of chunks. If the provided header is valid, the client state is updated and the newly verified header is written to the store.

```typescript
function verifyClientMessage(
  clientMsg: ClientMessage) {
    switch typeof(clientMsg) {
      case Header:
        verifyHeader(clientMsg)
    }
}
```

Verify validity of regular update to the NEAR client

```typescript
function (Header) getHeight(): Height {
    return self.lightClientBlockView.innerLite.height
}

function (Header) getEpochId(): CryptoHash {
    return self.lightClientBlockView.innerLite.epochId
}

function (Header) getNextEpochId(): CryptoHash {
    return self.lightClientBlockView.innerLite.nextEpochId
}

function (ClientState) getBlockProducersOf(epochId: CryptoHash): List<ValidatorStakeView> {
    consensusState = get("clients/{clientMsg.identifier}/consensusStates/{self.latestHeight}")
    if epochId === consensusState.header.getEpochId() {
        return consensusState.currentBps
    } else if epochId === consensusState.header.getNextEpochId() {
        return consensusState.header.lightClientBlockView.nextBps
    } else {
        return null
    }
}

function verifyHeader(header: Header) {
    clientState = get("clients/{header.identifier}/clientState")

    latestHeader = clientState.getLatestHeader()
    approvalMessage = header.lightClientBlockView.approvalMessage()

    // (1) The height of the block is higher than the height of the current head.
    assert(clientState.latestHeight < header.getHeight())

    // (2) The epoch of the block is equal to the epochId or nextEpochId known for the current head.
    assert(header.getEpochId() in
        [latestHeader.getEpochId(), latestHeader.getNextEpochId()])

    // (3) If the epoch of the block is equal to the nextEpochId of the head, then nextBps is not null.
    assert(not(header.getEpochId() == latestHeader.getNextEpochId()
        && header.lightClientBlockView.nextBps === null))

    // (4) approvalsAfterNext contain valid signatures on approvalMessage from the block producers of the corresponding epoch
    // (5) The signatures present in approvalsAfterNext correspond to more than 2/3 of the total stake
    totalStake = 0
    approvedStake = 0

    epochBlockProducers = clientState.getBlockProducersOf(header.getEpochId())
    for maybeSignature, blockProducer in zip(header.lightClientBlockView.approvalsAfterNext, epochBlockProducers) {
        totalStake += blockProducer.stake

        if maybeSignature === null {
            continue
        }

        approvedStake += blockProducer.stake

        assert(verify_signature(
            public_key: blockProducer.public_key,
            signature: maybeSignature,
            message: approvalMessage
        ))
    }

    threshold = totalStake * 2 / 3
    assert(approvedStake > threshold)

    // (6) If nextBps is not none, hash(borsh(nextBps)) corresponds to the nextBpHash in innerLite
    if header.lightClientBlockView.nextBps !== null {
        assert(hash(borsh(header.lightClientBlockView.nextBps)) === header.lightClientBlockView.innerLite.nextBpHash)
    }

    // (7) Check the prevStateRoot is the root of merklized prevStateRootOfChunks
    assert(header.lightClientBlockView.innerLite.prevStateRoot === merklize(header.prevStateRootOfChunks).root)
}
```

### Misbehaviour Predicate

TBD (currently not applicable in NEAR protocol)

### UpdateState

UpdateState will perform a regular update for the NEAR client. It will add a consensus state to the client store. If the header is higher than the lastest height on the clientState, then the clientState will be updated.

```typescript
function updateState(clientMessage: clientMessage) {
    clientState = get("clients/{clientMsg.identifier}/clientState)
    header = Header(clientMessage)
    // only update the clientstate if the header height is higher
    // than clientState latest height
    if clientState.height < header.getHeight() {
        // update latest height
        clientState.latestHeight = header.getHeight()

        // save the client
        set("clients/{clientMsg.identifier}/clientState", clientState)
    }

    currentBps = clientState.getBlockProducersOf(header.getEpochId())
    // create recorded consensus state, save it
    consensusState = ConsensusState { currentBps, header }
    set("clients/{clientMsg.identifier}/consensusStates/{header.getHeight()}", consensusState)
}
```

### UpdateStateOnMisbehaviour

TBD (currently not applicable in NEAR protocol)

### Upgrades

The chain which this light client is tracking can elect to write a special pre-determined key in state to allow the light client to update its client state in preparation for an upgrade.

As the client state change will be performed immediately, once the new client state information is written to the predetermined key, the client will no longer be able to follow blocks on the old chain, so it must upgrade promptly.

```typescript
function upgradeClientState(
  clientState: ClientState,
  newClientState: ClientState,
  height: Height,
  proof: CommitmentProof) {
    // assert trusting period has not yet passed
    assert(currentTimestamp() - clientState.latestTimestamp < clientState.trustingPeriod)
    // check that the revision has been incremented
    assert(newClientState.latestHeight > clientState.latestHeight)
    // check proof of updated client state in state at predetermined commitment prefix and key
    path = applyPrefix(clientState.upgradeCommitmentPrefix, clientState.upgradeKey)
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{clientMsg.identifier}/consensusStates/{height}")
    // verify that the provided consensus state has been stored
    assert(root.verifyMembership(path, newClientState, proof))
    // update client state
    clientState = newClientState
    set("clients/{clientMsg.identifier}/clientState", clientState)
}
```

### State verification functions

The NEAR client state verification functions check a MPT (Merkle Patricia Tree) proof against a previously validated consensus state.

```typescript
function (ConsensusState) verifyMembership(
  path: []byte,
  value: []byte,
  proof: [][]byte): bool {
    // Check that the root in proof is one of the prevStateRoot of a chunk
    assert(hash(proof[0]) in self.header.prevStateRootOfChunks)
    // Check the value on the path is exactly the given value with proof data
    // based on MPT construction algorithm
}

function verifyMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: []byte) {
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
    root = get("clients/{clientIdentifier}/consensusStates/{height}")
    // verify that <path, value> has been stored
    assert(root.verifyMembership(path, value, proof))
}

function (ConsensusState) verifyNonMembership(
  path: []byte,
  proof: [][]byte): bool {
    // Check that the root in proof is one of the prevStateRoot of a chunk
    assert(hash(proof[0]) in self.header.prevStateRootOfChunks)
    // Check that there is NO value on the path with proof data
    // based on MPT construction algorithm
}

function verifyNonMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath) {
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
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that nothing has been stored at path
    assert(root.verifyNonMembership(path, proof))
}
```

### Properties & Invariants

Correctness guarantees as provided by the NEAR light client algorithm.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementation

None yet.

## Other Implementations

None at present.

## History

January 12th, 2023 - Initial version

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
