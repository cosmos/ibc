---
ics: 12
title: NEAR Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: none
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

`merklize` is a generic function which can construct a merkle tree from an array which the element in it can be serialized by `borsh`. This function should return the merkle root of the tree at least. (In this document, we assume this function can return a tuple. We use `merklize(...).root` to denote the merkle root of the result tree.)

In NEAR protocol, the block producers are changed by time (about 12 hours). The period is known as `epoch` and the id of an epoch is represented by a `CryptoHash`.

### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification is based on the [NEAR light client specification](https://nomicon.io/ChainSpec/LightClient) and the implementation of [nearcore v1.30.0](https://github.com/near/nearcore/releases/tag/1.30.0) by adding necessary data fields and checking processes.

### Client state

The NEAR client state tracks the following data:

```typescript
interface ClientState {
    trustingPeriod: uint64
    latestHeight: Height
    latestTimestamp: uint64
    frozenHeight: Maybe<uint64>
    upgradeCommitmentPrefix: []byte
    upgradeKey: []bype
}
```

### Consensus state

The NEAR client tracks the block producers of current epoch and header (refer to [Headers section](#headers)) for all previously verified consensus states (these can be pruned after a certain period, but should not be pruned beforehand).

```typescript
interface ValidatorStake {
    accountId: string
    publicKey: PublicKey
    stake: uint128
}

interface ConsensusState {
    currentBps: List<ValidatorStake>
    header: Header
}
```

### Height

The height of a NEAR client is an `uint64` number.

```typescript
type Height = uint64
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

The NEAR client headers include the `LightClientBlock` and previous state root of chunks. The entire block producers for next epoch and approvals for the block after the next are included in `LightClientBlock`.

```typescript
interface BlockHeaderInnerLite {
    height: Height
    epochId: CryptoHash
    nextEpochId: CryptoHash
    prevStateRoot: CryptoHash
    outcomeRoot: CryptoHash
    timestamp: uint64   // in nanoseconds
    nextBpHash: CryptoHash
    blockMerkleRoot: CryptoHash
}

interface LightClientBlock {
    prevBlockHash: CryptoHash
    nextBlockInnerHash: CryptoHash
    innerLite: BlockHeaderInnerLite
    innerRestHash: CryptoHash
    nextBps: Maybe<List<ValidatorStake>>
    approvalsAfterNext: List<Maybe<Signature>>
}

interface Header {
    lightClientBlock: LightClientBlock
    prevStateRootOfChunks: List<CryptoHash>
}
```

The current block hash, next block hash and approval message can be calculated from `LightClientBlock`. The signatures in `approvalsAfterNext` are provided by current block producers by signing the approval message.

```typescript
function (LightClientBlock) currentBlockHash(): CryptoHash {
    return hash(concat(
        hash(concat(
          hash(borsh(self.innerLite)),
          self.innerRestHash,
        )),
        self.prevBlockHash
    ))
}

function (LightClientBlock) nextBlockHash(): CryptoHash {
    return hash(
        concat(self.nextBlockInnerHash,
        self.currentBlockHash()
    ))
}

enum ApprovalInner {
    Endorsement(CryptoHash),
    Skip(uint64)
}

function (LightClientBlock) approvalMessage(): []byte {
    return concat(
        borsh(ApprovalInner::Endorsement(self.nextBlockHash())),
        littleEndian(self.innerLite.height + 2)
    )
}
```

We also define the `CommitmentRoot` of `Header` as:

```typescript
function (Header) commitmentRoot(): CryptoHash {
    return self.lightClientBlock.innerLite.prevStateRoot
}
```

Header implements `ClientMessage` interface.

### Misbehaviour

The `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
The NEAR client `Misbehaviour` consists of two headers at the same height both of which the light client would have considered valid.

```typescript
interface Misbehaviour {
    identifier: string
    header1: Header
    header2: Header
}
```

> As the slashing policy is NOT applicable in NEAR protocol for now, this section is only for references.

### Client initialisation

The NEAR client initialisation requires a latest consensus state (the latest header and the block producers of the epoch of the header).

```typescript
function (Header) getHeight(): Height {
    return self.lightClientBlock.innerLite.height
}

function initialise(
  trustingPeriod: uint64,
  latestTimestamp: uint64,
  upgradeCommitmentPrefix: []byte,
  upgradeKey: []bype,
  consensusState: ConsensusState): ClientState {
    assert(len(consensusState.currentBps) > 0)
    assert(consensusState.header.getHeight() > 0)
    // implementations may define a identifier generation function
    identifier = generateClientIdentifier()
    height = consensusState.header.getHeight()
    provableStore.set("clients/{identifier}/consensusStates/{height}", consensusState)
    return ClientState {
        trustingPeriod
        latestHeight: height
        latestTimestamp
        frozenHeight: null
        upgradeCommitmentPrefix
        upgradeKey
    }
}
```

### Validity Predicate

The NEAR client validity checking uses spec described in the [NEAR light client specification](https://nomicon.io/ChainSpec/LightClient) by adding an extra checking for the previous state root of chunks. If the provided header is valid, the client state is updated and the newly verified header is written to the store.

```typescript
function verifyClientMessage(clientMessage: ClientMessage) {
    switch typeof(clientMessage) {
      case Header:
        verifyHeader(clientMessage)
    }
}
```

Verify validity of regular update to the NEAR client

```typescript
function (Header) getEpochId(): CryptoHash {
    return self.lightClientBlock.innerLite.epochId
}

function (Header) getNextEpochId(): CryptoHash {
    return self.lightClientBlock.innerLite.nextEpochId
}

function (ConsensusState) getBlockProducersOf(epochId: CryptoHash): List<ValidatorStake> {
    if epochId === self.header.getEpochId() {
        return self.currentBps
    } else if epochId === self.header.getNextEpochId() {
        return self.header.lightClientBlock.nextBps
    } else {
        return null
    }
}

function verifyHeader(header: Header) {
    consensusState = provableStore.get("clients/{clientMessage.identifier}/consensusStates/{clientState.latestHeight}", consensusState)

    latestHeader = consensusState.header
    approvalMessage = header.lightClientBlock.approvalMessage()

    // (1) The height of the block is higher than the height of the current head.
    assert(clientState.latestHeight < header.getHeight())

    // (2) The epoch of the block is equal to the epochId or nextEpochId
    //     known for the current head.
    assert(header.getEpochId() in
        [latestHeader.getEpochId(), latestHeader.getNextEpochId()])

    // (3) If the epoch of the block is equal to the nextEpochId of the head,
    //     then nextBps is not null.
    assert(not(header.getEpochId() == latestHeader.getNextEpochId()
        && header.lightClientBlock.nextBps === null))

    // (4) approvalsAfterNext contain valid signatures on approvalMessage
    //     from the block producers of the corresponding epoch.
    // (5) The signatures present in approvalsAfterNext correspond to
    //     more than 2/3 of the total stake.
    totalStake = 0
    approvedStake = 0

    epochBlockProducers = consensusState.getBlockProducersOf(header.getEpochId())
    for maybeSignature, blockProducer in
      zip(header.lightClientBlock.approvalsAfterNext, epochBlockProducers) {
        totalStake += blockProducer.stake

        if maybeSignature === null {
            continue
        }

        approvedStake += blockProducer.stake

        assert(verifySignature(
            public_key: blockProducer.public_key,
            signature: maybeSignature,
            message: approvalMessage
        ))
    }

    assert(approvedStake * 3 > totalStake * 2)

    // (6) If nextBps is not none, hash(borsh(nextBps)) corresponds to the nextBpHash in innerLite
    if header.lightClientBlock.nextBps !== null {
        assert(hash(borsh(header.lightClientBlock.nextBps))
            === header.lightClientBlock.innerLite.nextBpHash)
    }

    // (7) Check the prevStateRoot is the root of merklized prevStateRootOfChunks
    assert(header.commitmentRoot() === merklize(header.prevStateRootOfChunks).root)
}
```

### Misbehaviour Predicate

Function `checkForMisbehaviour` will check if an update contains evidence of Misbehaviour. If the `ClientMessage` is a header we check for implicit evidence of misbehaviour by checking if there already exists a conflicting consensus state in the store or if the header breaks time monotonicity.

```typescript
function (Header) timestamp(): uint64 {
    return self.lightClientBlock.innerLite.timestamp
}

function (ConsensusState) timestamp(): uint64 {
    return self.header.lightClientBlock.innerLite.timestamp
}

function checkForMisbehaviour(
  clientMsg: clientMessage) => bool {
    clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
    switch typeof(clientMsg) {
        case Header:
            // fetch consensusstate at header height if it exists
            consensusState = provableStore.get("clients/{clientMsg.identifier}/consensusStates/{header.getHeight()}")
            // if consensus state exists and conflicts with the header
            // then the header is evidence of misbehaviour
            if consensusState != nil
              && consensusState.header.commitmentRoot() != header.commitmentRoot() {
                return true
            }

            // check for time monotonicity misbehaviour
            // if header is not monotonically increasing with respect to neighboring consensus states
            // then return true
            // NOTE: implementation must have ability to iterate ascending/descending by height
            prevConsState = getPreviousConsensusState(header.getHeight())
            nextConsState = getNextConsensusState(header.getHeight())
            if prevConsState.timestamp() >= header.timestamp() {
                return true
            }
            if nextConsState != nil && nextConsState.timestamp() <= header.timestamp() {
                return true
            }
        case Misbehaviour:
            if (misbehaviour.header1.getHeight() < misbehaviour.header2.getHeight()) {
                return false
            }
            // if heights are equal check that this is valid misbehaviour of a fork
            if (misbehaviour.header1.getHeight() === misbehaviour.header2.getHeight() && misbehaviour.header1.commitmentRoot() !== misbehaviour.header2.commitmentRoot()) {
                return true
            }
            // otherwise if heights are unequal check that this is valid misbehavior of BFT time violation
            if (misbehaviour.header1.timestamp() <= misbehaviour.header2.timestamp()) {
                return true
            }

            return false
    }
}
```

> As the slashing policy is NOT applicable in NEAR protocol for now, this section is only for references.

### Update State

Function `updateState` will perform a regular update for the NEAR client. It will add a consensus state to the client store. If the header is higher than the lastest height on the clientState, then the clientState will be updated.

```typescript
function updateState(clientMessage: clientMessage) {
    clientState = provableStore.get("clients/{clientMessage.identifier}/clientState")
    consensusState = provableStore.get("clients/{clientMessage.identifier}/consensusStates/{clientState.latestHeight}")

    header = Header(clientMessage)
    // only update the clientstate if the header height is higher
    // than clientState latest height
    if clientState.latestHeight < header.getHeight() {
        // update latest height
        clientState.latestHeight = header.getHeight()

        // save the client
        provableStore.set("clients/{clientMessage.identifier}/clientState", clientState)
    }

    currentBps = consensusState.getBlockProducersOf(header.getEpochId())
    // create recorded consensus state, save it
    newConsensusState = ConsensusState { currentBps, header }
    provableStore.set("clients/{clientMessage.identifier}/consensusStates/{header.getHeight()}", newConsensusState)
}
```

### Update State On Misbehaviour

Function `updateStateOnMisbehaviour` will set the frozen height to a non-zero sentinel height to freeze the entire client.

```typescript
function updateStateOnMisbehaviour(clientMsg: clientMessage) {
    clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
    switch typeof(clientMsg) {
        case Header:
            prevConsState = getPreviousConsensusState(header.getHeight())
            clientState.frozenHeight = prevConsState.header.getHeight()
        case Misbehaviour:
            prevConsState = getPreviousConsensusState(misbehaviour.header1.getHeight())
            clientState.frozenHeight = prevConsState.header.getHeight()
    }
    provableStore.set("clients/{clientMsg.identifier}/clientState", clientState)
}
```

> As the slashing policy is NOT applicable in NEAR protocol for now, this section is only for references.

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
    // check that the latest height has been incremented
    assert(newClientState.latestHeight > clientState.latestHeight)
    // check proof of updated client state in state at predetermined commitment prefix and key
    path = applyPrefix(clientState.upgradeCommitmentPrefix, clientState.upgradeKey)
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // fetch the previously verified commitment root & verify membership
    // Implementations may choose how to pass in the identifier
    // ibc-go provides the identifier-prefixed store to this method
    // so that all state reads are for the client in question
    consensusState = provableStore.get("{clientIdentifier}/consensusStates/{height}")
    // verify that the provided client state has been stored
    assert(consensusState.verifyMembership(path, newClientState, proof))
    // update client state
    clientState = newClientState
    provableStore.set("{clientIdentifier}/clientState", clientState)
}
```

### State verification functions

The NEAR client state verification functions check a MPT (Merkle Patricia Tree) proof against a previously validated consensus state. The client should provide both membership verification and non-membership verification.

```typescript
function (ConsensusState) verifyMembership(
  path: []byte,
  value: []byte,
  proof: [][]byte): bool {
    // Check that the root in proof is one of the prevStateRoot of a chunk
    assert(hash(proof[0]) in self.header.prevStateRootOfChunks)
    // Check the value on the path is exactly the given value with proof data
    // based on MPT construction algorithm.
    // Omit pseudocode for the verification in this document.
}

function verifyMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: []byte): Error {
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
    consensusState = provableStore.get("clients/{clientMessage.identifier}/consensusStates/{height}")
    // verify that <path, value> has been stored
    if !consensusState.verifyMembership(path, value, proof) {
        return Error
    }
    return nil
}

function (ConsensusState) verifyNonMembership(
  path: []byte,
  proof: [][]byte): bool {
    // Check that the root in proof is one of the prevStateRoot of a chunk
    assert(hash(proof[0]) in self.header.prevStateRootOfChunks)
    // Check that there is NO value on the path with proof data
    // based on MPT construction algorithm.
    // Omit pseudocode for the verification in this document.
}

function verifyNonMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath): Error {
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
    consensusState = provableStore.get("clients/{clientMessage.identifier}/consensusStates/{height}")
    // verify that nothing has been stored at path
    if !consensusState.verifyNonMembership(path, proof) {
        return Error
    }
    return nil
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
