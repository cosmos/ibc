---
ics: 8
title: Wasm Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Parth Desai <parth@chorus.one>, Mateusz Kaczanowski <mateusz@chorus.one>
created: 2020-10-13
modified: 2020-10-13
---

## Synopsis

This specification document describes an interface to a client (verification algorithm) stored as a Wasm bytecode for a blockchain.

### Motivation

Wasm based light clients are decoupled from SDK source code, which enables one to upgrade existing light client or add support for other blockchain without modifying SDK source code.

### Definitions

Functions & terms are as defined in [ICS 2](../ics-002-client-semantics).

`currentTimestamp` is as defined in [ICS 24](../ics-024-host-requirements).

`Wasm Client Code` refers to Wasm bytecode stored in the client store, which provides a target blockchain specific implementation of [ICS 2](../ics-002-client-semantics).

`Wasm Client` refers to a particular instance of `Wasm Client Code` defined as a tuple `(Wasm Client Code, ClientID)`.

`Wasm VM` refers to a virtual machine capable of executing valid Wasm bytecode.


### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification depends on the correct instantiation of the `Wasm client` and is decoupled from any specific implementation of the target `blockchain` consensus algorithm.

### Client state

The Wasm client state tracks location of the Wasm bytecode via `codeId`. Binary data represented by `data` field is opaque and only interpreted by the Wasm Client Code. `type` represents client type.
`type` and `codeId` both are immutable.

```typescript
interface ClientState {
  codeId: []byte
  data: []byte
  latestHeight: Height
  proofSpecs: []ProofSpec
}
```

### Consensus state

The Wasm consensus state tracks the timestamp (block time), `Wasm Client code` specific fields and commitment root for all previously verified consensus states.
`type` and `codeId` both are immutable.

```typescript
interface ConsensusState {
  codeId: []byte
  data: []byte
  timestamp: uint64
}
```

### Height

The height of a Wasm light client instance consists of two `uint64`s: the revision number, and the height in the revision.

```typescript
interface Height {
  revisionNumber: uint64
  revisionHeight: uint64
}
```

Comparison between heights is implemented as follows:

```typescript
function compare(a: Height, b: Height): Ord {
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

Contents of Wasm client headers depend upon `Wasm Client Code`.

```typescript
interface Header {
  data: []byte
  height: Height
}
```

### Misbehaviour

The `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow - if applicable.
Wasm client `Misbehaviour` consists of two conflicting headers both of which the light client would have considered valid.

```typescript
interface Misbehaviour {
  clientId: string
  h1: Header
  h2: Header
}
```

### Client initialisation

Wasm client initialisation requires a (subjectively chosen) latest consensus state interpretable by the target Wasm Client Code. `wasmCodeId` field is unique identifier for `Wasm Client Code`, and `initializationData` refers to opaque data required for initialization of the particular client managed by that Wasm Client Code.

```typescript
function initialise(
    wasmCodeId: String,
    initializationData: []byte,
    consensusState: []byte,
  ): ClientState {
    codeHandle = getWasmCode(wasmCodeId)
    assert(codeHandle.isInitializationDataValid(initializationData, consensusState))

    store = getStore("clients/{identifier}")
    return codeHandle.initialise(store, initializationData, consensusState)
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

Wasm client validity checking uses underlying Wasm Client code. If the provided header is valid, the client state is updated & the newly verified commitment written to the store.

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  revision: uint64,
  header: Header) {
    store = getStore("clients/{identifier}")
    codeHandle = clientState.codeHandle()

    // verify that provided header is valid and state is saved
    assert(codeHandle.validateHeaderAndCreateConsensusState(store, clientState, header, revision))
}
```

### Misbehaviour predicate

Wasm client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  misbehaviour: Misbehaviour) {
    store = getStore("clients/{identifier}")
    codeHandle = clientState.codeHandle()
    assert(codeHandle.handleMisbehaviour(store, clientState, misbehaviour))
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
  proof: CommitmentPrefix) {
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyNewClientState(clientState, newClientState, height, proof))

    // update client state
    clientState = newClientState
    set("clients/{identifier}", clientState)
}
```

In case of Wasm client, upgrade of `Wasm Client Code` is also possible via blockchain specific management functionality.

### State verification functions

Wasm client state verification functions check a Merkle proof against a previously validated commitment root.

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: Height,
  consensusState: ConsensusState) {
    codeHandle = getCodeHandleFromClientID(clientIdentifier)
    assert(codeHandle.isValidClientState(clientState, height, prefix, clientIdentifier, proof, consensusStateHeight, consensusState))
}

function verifyConnectionState(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyConnectionState(clientState, height, prefix, proof, connectionIdentifier, connectionEnd))
}

function verifyChannelState(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyChannelState(clientState, height, prefix, proof, porttIdentifier, channelIdentifier, channelEnd))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyPacketData(clientState, height, prefix, proof, portportIdentifier, channelIdentifier, sequence, data))
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
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyPacketAcknowledgement(clientState, height, prefix, proof, portportIdentifier, channelIdentifier, sequence, acknowledgement))
}

function verifyPacketReceiptAbsence(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyPacketReceiptAbsence(clientState, height, prefix, proof, portIdentifier, channelIdentifier, sequence))
}

function verifyNextSequenceRecv(
  clientState: ClientState,
  height: Height,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    codeHandle = clientState.codeHandle()
    assert(codeHandle.verifyNextSequenceRecv(clientState, height, prefix, proof, portIdentifier, channelIdentifier, nextSequenceRecv))
}
```

### Wasm Client Code Interface

#### What is code handle?
Code handle is an object that facilitates interaction between Wasm code and go code. For example, consider the method `isValidClientState` which could be implemented like this:

```go
func (c *CodeHandle) isValidClientState(clientState ClientState, height u64) {
    clientStateData := json.Serialize(clientState)
    packedData := pack(clientStateData, height)
    // VM specific code to call Wasm contract
}
```

#### Wasm Client interface
Every Wasm client code need to support ingestion of below messages in order to be used as light client.

```rust
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct MisbehaviourMessage {
    pub client_state: Vec<byte>,
    pub consensus_state: Vec<byte>,
    pub height: u64,
    pub header1: Vec<byte>,
    pub header2: Vec<byte>,
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct CreateConsensusMessage {
    pub client_state: Vec<byte>,
    pub revision: u64,
    pub height: u64
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct InitializeClientStateMessage {
    pub initialization_data: Vec<byte>,
    pub consensus_state: Vec<byte>
}


#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub enum HandleMsg {
    HandleMisbehaviour(MisbehaviourMessage),
    TryCreateConsensusState(CreateConsensusMessage),
    InitializeClientState(InitializeClientStateMessage)
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct ValidateClientStateMessage {
    client_state: Vec<byte>,
    height: u64
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct ValidateNewClientStateMessage {
    client_state: Vec<byte>,
    new_client_state: Vec<byte>,
    height: u64
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct ValidateInitializationDataMessage {
    init_data: Vec<byte>,
    consensus_state: Vec<byte>
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub enum ValidityPredicate {
    ClientState(ValidateClientStateMessage),
    NewClientState(ValidateNewClientStateMessage),
    InitializationData(ValidateInitializationDataMessage),
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub enum QueryMsg {
   IsValid(ValidityPredicate),
   LatestClientHeight(Vec<byte>),
   ProofSpec(Vec<byte>)
}

```

### Properties & Invariants

Correctness guarantees as provided by the underlying algorithm implemented by `Wasm Client Code`.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

As long as `Wasm Client Code` keeps interface consistent with `ICS 02` it should be forward compatible

## Example Implementation

None yet.

## Other Implementations

None at present.

## History


## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
