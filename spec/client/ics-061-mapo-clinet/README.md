---
ics: 61
title: MAPO Client
stage: draft
category: IBC/TAO
kind: instantiation
author: Jason <mapdev33@protonmail.com>
created: 2022-12-22
implements: 2
---

## Synopsis

This specification document describes a client (verification algorithm) for a MAPO client which implements the [ICS 2](../../core/ics-002-client-semantics) interface.

### Motivation

MAP Protocol provides a provably secure full-chain infrastructure based on light client and zero-knowledge technology, and has been connected to Polygon, NEAR, and BNB smart chains for cross-chain transactions and communications in December 2022; Ethereum 2.0 and Klaytn is being tested and will be launched on the mainnet in January 2023, and will continue to access more mainchains to expand a greater cross-chain ecology and diversity.

So we hope that the blockchain using the MAPO protocol can be connected with other cosmos ecological blockchains through IBC.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

### Desired Properties

This specification must satisfy the client interface defined in [ICS 2](../../core/ics-002-client-semantics).

## Technical Specification

This specification depends on correct instantiation of the [mapo-relay-chain](https://docs.mapprotocol.io/learn/overiew/protocollayer/mapo-relay-chain/verification) and its [light client](https://docs.mapprotocol.io/develop/light-client/mapo-light-client/evm) algorithm.

### Client state

The `ClientState` of a MAPO client tracks latest epoch and whether or not the client is frozen.

```typescript
interface ClientState {
  frozen:   boolean
  latestEpoch: uint256
  epochSize:  uint256
  clientIdentifier: Identifier
}
```

### Consensus state

The `ConsensusState` of a MAPO client consists of the current validator set in the epoch, and current epoch number.

The pairKeys of G1Pubkey used to verify the aggregated signature by bn256 algorithm.

```typescript
type G2Pubkey []byte            // 128 bytes

interface ValidatorSet {
  pairKeys:   List<Pair<Address, G2Pubkey>>
  weights:    list<uint>
}

```

```typescript
interface ConsensusState {
  epoch:            uint256
  validators:       ValidatorSet
  commitmentRoot:   []byte
}
```

### Headers

The MAPO header contains basic information such as the height, the timestamp, the state root,the hash of the transactions, and the IstanbulExtra proposed separately and the aggregated public key of the block.

```typescript
interface MapoSignedHeader {
  height: uint256
  parentHash: Hash
  root:       Hash
  txRoot:     Hash
  receiptRoot:  Hash
  timestamp: uint256
  gasLimit: uint256
  gasUsed: uint256
  nonce:   uint256
  bloom: []byte
  extraData: []byte
  mixDigest:  Hash
  baseFee:   uint256
}
```

```typescript
interface Header {
  MapoSignedHeader
  commitmentRoot: []byte
  identifier: string
}

function signature(header: Header): []byte{
  return copy(toIstanbulExtra(header.extraData).aggregatedSeal.signature)
}
```
```typescript
function HeightToEpoch(height: uint256,epochSize: uint256): uint256 {
  if (height % epochSize == 0)  {
    return height / epochSize
  } else {
    return height / epochSize + 1
  }
}
function toIstanbulExtra(data: []byte): IstanbulExtra {
  return makeIstanbulExtraFromBytes(data)
}
```

### IstanbulExtra

Committee change information corresponds to extraData in block `MapSignedHeader`

```typescript
interface IstanbulExtra {
  //Addresses of added committee members
  validators: []Address
  //The public key(G2) of the added committee member
  addedPubKey: []byte
  //The public key(G1) of the added committee member
  addedG1PubKey: []byte
  //Members removed from the previous committee are removed by bit 1 after binary encoding
  removeList:   uint256
  //The signature of the previous committee on the current header
  //Reference for specific signature and encoding rules
  //https://docs.maplabs.io/develop/map-relay-chain/consensus/epoch-and-block/aggregatedseal#calculate-the-hash-of-the-block-header
  seal:   []byte
  //Information on current committees
  aggregatedSeal: IstanbulAggregatedSeal
  //Information on the previous committee
  parentAggregatedSeal: IstanbulAggregatedSeal
}

interface IstanbulAggregatedSeal {
  bitmap: uint256
  signature: []byte
  round: uint256
}
```

Header implements `ClientMessage` interface.


### Misbehaviour 

MAPO client misbehaviour checking determines whether or not two conflicting headers at the same epoch would have convinced the light client.

```typescript
interface Misbehaviour {
  h1: Header
  h2: Header
}
```

### Client initialisation

The MAPO client `initialise` function starts an unfrozen client with the initial consensus state.

```typescript
function initialise(identifier: Identifier, epoch: uint64, consensusState: ConsensusState): ClientState {
    set("clients/{identifier}/consensusStates/{epoch}", consensusState)
    return ClientState{
      frozen:  false,
      latestEpoch: epoch,
      clientIdentifier: identifier,
      consensusState,
    }
}
```

The MAPO client `latestClientHeight` function returns the latest stored epoch, which is updated by every new epoch.

```typescript
function latestClientHeight(clientState: ClientState): uint256 {
  return clientState.latestEpoch
}
```

### Validity predicate

The MAPO client `checkValidityAndUpdateState` function checks that a header is signed by the current validators set and verifies signature to determine if there is a expected change to the validators set with the epoch number. If the provided header is valid, the client state and the consensus state is updated.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
  
  epoch = HeightToEpoch(header.height,clientState.epochSize)
  assert(epoch == clientState.latestEpoch + 1)
  assert(header.identifier == string(clientState.clientIdentifier))
  consensusState = get("clients/{header.identifier}/consensusStates/{clientState.latestEpoch}")
  assert(checkBlsSignature(consensusState, header))
  clientState.latestEpoch = epoch
  // update the consensusState for the epoch and save it
  consensusState.epoch = epoch
  consensusState.commitmentRoot = header.commitmentRoot
  consensusState.validators = parseValidatorSetFromHeader(header)
  set("clients/{header.identifier}/consensusStates/{epoch}", consensusState)
  // save the client
  set("clients/{header.identifier}", clientState)
}

function checkBlsSignature(consensusState: ConsensusState,header: Header): boolean {
  // return true must be more than 2/3 validator agree it
  return verifyAggregatedSignature(message(header),signature(header),consensusState.validators)
}
```

### Misbehaviour predicate

MAPO client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  misbehaviour: Misbehaviour) {
    h1 = misbehaviour.h1
    h2 = misbehaviour.h2

    assert(h1.identifier == h2.identifier)
    assert(h1.identifier == string(clientState.clientIdentifier))
    assert(h1.height == h2.height)
    epoch1 = HeightToEpoch(h1.height,clientState.epochSize)
    epoch2 = HeightToEpoch(h2.height,clientState.epochSize)
    assert(epoch1 == clientState.latestEpoch + 1)
    assert(epoch2 == clientState.latestEpoch + 1)
    // assert that signature data is different
    assert(signature(h1) !== signature(h2))
    // fetch current consensus state
    consensusState = get("clients/{h1.identifier}/consensusStates/{clientState.latestEpoch}")
    // assert that the signatures validate
    assert(checkBlsSignature(consensusState, h1))
    assert(checkBlsSignature(consensusState, h2))
    // freeze the client
    clientState.frozen = true
}
```

### UpdateState

UpdateState will perform a regular update for the MAPO client. It will add a consensus state to the client store. If the header is higher than the latestEpoch on the clientState, then the clientState will be updated.

```typescript
function updateState(
  clientMsg: clientMessage) {
    header = Header(clientMsg)
    clientState = get("clients/{header.identifier}/clientState")
    assert(header.identifier == string(clientState.clientIdentifier))
    epoch = HeightToEpoch(header.height,clientState.epochSize)
    // only update the clientstate if the header epoch is higher
    // than clientState latest epoch
    if clientState.latestEpoch < epoch {
      // update latest epoch
      clientState.latestEpoch = epoch

      // save the client
      set("clients/{header.identifier}/clientState", clientState)
    }

    // create recorded consensus state, save it
    consensusState.epoch = epoch
    consensusState.commitmentRoot = header.commitmentRoot
    consensusState.validators = parseValidatorSetFromHeader(header)
    set("clients/{header.identifier}/consensusStates/{epoch}", consensusState)
}
```

### UpdateStateOnMisbehaviour

UpdateStateOnMisbehaviour will set the frozen was true and save it.

```typescript
function updateStateOnMisbehaviour(clientMsg: clientMessage) {
    header = Header(clientMsg)
    clientState = get("clients/{header.identifier}/clientState")
    clientState.frozen = true
    set("clients/{header.identifier}/clientState", clientState)
}
```

### State verification functions

MAPO client state verification functions check a Merkle proof against a previously validated commitment root.

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState) {
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusState/{epoch}")
    // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientIdentifier}/consensusStates/{epoch}")
    // verify that the provided connection end has been stored
    assert(state.verifyMembership(path, consensusState, proof))
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "connections/{connectionIdentifier}")
    // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientState.clientIdentifier}/consensusStates/{epoch}")
    // verify that the provided connection end has been stored
    assert(state.verifyMembership(path, connectionEnd, proof))
}

function verifyChannelState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}")
    // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientState.clientIdentifier}/consensusStates/{epoch}")
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
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
    // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientState.clientIdentifier}/consensusStates/{epoch}")
    // verify that the provided commitment has been stored
    assert(state.verifyMembership(path, hash(data), proof))
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
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientState.clientIdentifier}/consensusStates/{epoch}")
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
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/receipts/{sequence}")
   // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientState.clientIdentifier}/consensusStates/{epoch}")
    // verify that no acknowledgement has been stored
    assert(state.verifyNonMembership(path, proof))
}

function verifyNextSequenceRecv(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    epoch = HeightToEpoch(height,clientState.epochSize)
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/nextSequenceRecv")
   // check that the client is at a sufficient epoch
    assert(clientState.latestEpoch >= epoch)
    // check that the client is unfrozen at a epoch
    assert(clientState.frozen === false)
    // fetch the previously verified commitment root & verify membership
    state = get("clients/{clientState.clientIdentifier}/consensusStates/{epoch}")
    // verify that the nextSequenceRecv is as claimed
    assert(state.verifyMembership(path, nextSequenceRecv, proof))
}
```

### Properties & Invariants

Instantiates the interface defined in [ICS 2](../../core/ics-002-client-semantics).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementation

None yet.

## Other Implementations

None at present.

## History

December 22th, 2022 - Initial version

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).

