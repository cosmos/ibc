---
ics: 28
title: WASM Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Parth Desai <parth@chorus.one>
created: 2020-10-13
modified: 2020-10-13
---

## Synopsis

This specification document describes an interface to a client (verification algorithm) stored as a wasm bytecode for a blockchain.

### Motivation

WASM based light clients are decoupled from SDK source code, which enables one to upgrade existing light client or add support for other blockchain without modifying SDK source code.

### Definitions

Functions & terms are as defined in [ICS 2](../ics-002-client-semantics).

`currentTimestamp` is as defined in [ICS 24](../ics-024-host-requirements).

`WASM Client Code` refers to wasm bytecode stored in the client store, which provides target blockchain specific implementation of [ICS 2](../ics-002-client-semantics).

`WASM Client` refers to a particular instance of `WASM Client Code` defined as a tuple `(WASM Client Code, ClientID)`. 

`WASM VM` refers to a virtual machine capable of executing valid wasm bytecode. 


### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification depends on correctness of the `WASM Client code` in context of consensus algorithm of its target `blockchain`, as well as correct instantiation of `WASM Client`.

### Client state

The wasm client state tracks location of the wasm bytecode and binary data representing current state of the client. Binary data represented by `state` field is opaque and only interpreted by the WASM Client Code.

```typescript
interface ClientState {
  location: string
  state: []byte
}
```

### Consensus state

The WASM consensus state tracks the timestamp (block time), `WASM Client code` specific fields and commitment root for all previously verified consensus states.

```typescript
interface ConsensusState {
  timestamp: uint64
  commitmentRoot: []byte
  .. // Client type specific fields
}
```

### Height

The height of a WASM light client instance consists of two `uint64`s: the epoch number, and the height in the epoch.

```typescript
interface Height {
  epochNumber: uint64
  epochHeight: uint64
}
```

Comparison between heights is implemented as follows:

```typescript
function compare(a: Height, b: Height): Ord {
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

Contents of wasm client headers depend upon `WASM Client Code`.

```typescript
interface Header {
  <Client type specific fields>
}
```

### Misbehaviour
 
The `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
WASM client `Misbehaviour` consists of two headers at the same height both of which the light client would have considered valid.

```typescript
interface Misbehaviour {
  fromHeight: Height
  h1: Header
  h2: Header
}
```

### Client initialisation

WASM client initialisation requires a (subjectively chosen) latest consensus state interpretable by the target WASM Client Code. `wasmCodeId` field is unique identifier for `WASM Client Code`, and `initializationData` refers to opaque data required for initialization of the particular client managed by that WASM Client Code.

```typescript
function initialise(
    wasmCodeId: String,
    initializationData: []byte,
    consensusState: []byte,
  ): ClientState {
    codeHandle = getWASMCode(wasmCodeId)
    assert(codeHandle.isInitializationDataValid(initializationData, consensusState))
    set("clients/{identifier}/consensusStates/{height}", consensusState)
    return codeHandle.initialise(initializationData)
}
```

The `latestClientHeight` function returns the latest stored height, which is updated every time a new (more recent) header is validated.

```typescript
function latestClientHeight(clientState: ClientState): Height {
  codeHandle = clientState.codeHandle();
  codeHandle.latestClientHeight(clientState)
}
```

### Validity predicate

WASM client validity checking uses underlying WASM Client code. If the provided header is valid, the client state is updated & the newly verified commitment written to the store.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  epoch: uint64,
  header: Header) {
    codeHandle = clientState.codeHandle()
    isValid, consensusState = codeHandle.validateHeaderAndCreateConsensusState(clientState, header, epoch)
    set("clients/{identifier}/consensusStates/{header.height}", consensusState)
    // save the client
    set("clients/{identifier}", clientState)
}
```

### Misbehaviour predicate

WASM client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  misbehaviour: Misbehaviour) {
    codeHandle = clientState.codeHandle()
    consensusState = get("clients/{identifier}/consensusStates/{misbehaviour.fromHeight}")
    assert(codeHandle.handleMisbehaviour(clientState, consensusState, misbehaviour))
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
    codeHandle = clientState.codeHandle()
    codeHandle.verifyNewClientState(oldClientState, newClientState, height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided consensus state has been stored
    assert(root.verifyMembership(path, newClientState, proof))
    // update client state
    clientState = newClientState
    set("clients/{identifier}", clientState)
}
```

In case of wasm client, upgrade of `WASM Client Code` is also possible via blockchain specific management functionality.

### State verification functions

WASM client state verification functions check a Merkle proof against a previously validated commitment root.

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
    codeHandle = getCodeHandleFromClientID(clientIdentifier)
    assert(codeHandle.isValidClientState(clientState, height))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.isValidClientState(clientState, height))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.isValidClientState(clientState, height))
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided channel end has been stored
    assert(root.verifyMembership(codeHandle.getProofSpec(clientState), path, channelEnd, proof))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.isValidClientState(clientState, height))
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided commitment has been stored
    assert(root.verifyMembership(codeHandle.getProofSpec(clientState), path, hash(data), proof))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.isValidClientState(clientState, height))
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided acknowledgement has been stored
    assert(root.verifyMembership(codeHandle.getProofSpec(clientState), path, hash(acknowledgement), proof))
}

function verifyPacketReceiptAbsence(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/receipts/{sequence}")
    codeHandle = clientState.codeHandle()
    assert(codeHandle.isValidClientState(clientState, height))
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that no acknowledgement has been stored
    assert(root.verifyNonMembership(codeHandle.getProofSpec(clientState), path, proof))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.isValidClientState(clientState, height))
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the nextSequenceRecv is as claimed
    assert(root.verifyMembership(codeHandle.getProofSpec(clientState), path, nextSequenceRecv, proof))
}
```

### Properties & Invariants

Correctness guarantees as provided by the underlying algorithm implemented by `WASM Client Code`.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

As long as `WASM Client Code` keeps interface consistent with `ICS 02` it should be forward compatible

## Example Implementation

None yet.

## Other Implementations

None at present.

## History


## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
