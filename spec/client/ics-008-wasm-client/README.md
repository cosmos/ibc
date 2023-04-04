---
ics: 8
title: Wasm Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Parth Desai <parth@chorus.one>, Mateusz Kaczanowski <mateusz@chorus.one>, Blas Rodriguez Irizar <blas@composable.finance>, Steve Miskovetz <steve@strange.love>
created: 2020-10-13
modified: 2023-03-28
---

## Synopsis

This specification document describes an interface to a light client stored as a WASM bytecode for a blockchain.
Changes to `ClientState` interface and associated handler to align with changes in [02-client-refactor ADR](https://github.com/cosmos/ibc-go/pull/1871).

### Motivation

Currently, adding a new client implementation or upgrading an existing one requires a hard upgrade because the client implementations are part of the static chain binary. Any change to the on-chain light client code depends on chain governance approving an upgrade before it can be deployed.

This may be acceptable when adding new client types since the number of unique consensus algorithms that need to be supported is currently small. However, this process will become very tedious when it comes to upgrading light clients.

Without dynamically upgradable light clients, a chain that wishes to upgrade its consensus algorithm (and thus break existing light clients) must wait for all counterparty chains to perform a hard upgrade that adds support for the upgraded light client before it can perform an upgrade on its chain. Examples of a consensus-breaking upgrade would be an upgrade from Tendermint v1 to a light-client breaking Tendermint v2 or switching from Tendermint consensus to Honeybadger. Changes to the internal state-machine logic will not affect consensus. E.g., changes to the staking module do not require an IBC upgrade.

Requiring all counterparties to add statically new client implementations to their binaries will inevitably slow the pace of upgrades in the IBC network since the deployment of an upgrade on even a very experimental, fast-moving chain will be blocked by an upgrade to a high-value chain that will be inherently more conservative.

Once the IBC network broadly adopts dynamically upgradable clients, a chain may upgrade its consensus algorithm whenever it wishes, and relayers may upgrade the client code of all counterparty chains without requiring the counterparty chains to perform an upgrade themselves. This prevents a dependency on counterparty chains when considering upgrading one's consensus algorithm.

Another reason why this interface is beneficial is that it removes the dependency between Light clients and the Go programming language. Using WASM as a compilation target, light clients can be written in any programming language whose toolchain includes WASM as a compilation target. Examples of these are Go, Rust, C, and C++.

### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

`currentTimestamp` is as defined in [ICS 24](../../core/ics-024-host-requirements).

`Wasm Contract` refers to Wasm bytecode stored in the `08-wasm` store, which provides a target blockchain specific implementation of [ICS 2](../../core/ics-002-client-semantics).

`Wasm Client` refers to a particular instance of `Wasm Contract` defined as a tuple `(Wasm Contract, ClientID)`.

`Wasm VM` refers to a virtual machine capable of executing valid Wasm bytecode.

### Desired properties

This specification must satisfy the client interface defined in [ICS 2.](../../core/ics-002-client-semantics).

## Technical specification

This specification depends on the correct instantiation of the Wasm client and is decoupled from any specific implementation of the target `blockchain` consensus algorithm.

### Storage management

Light client operations defined in the `02-client` can be stateful; they may modify the
state kept in storage. For that, there is a need to allow the underlying light client implementation
to access client and consensus data structures and, after performing certain computations, to
update the storage with the new versions of them.

The current implementation chooses to share the Wasm client store between the `02-client` module (for reading), `08-wasm` module (for instantiation), and Wasm contract.
Other than instantiation, the Wasm contract is responsible for updating state.

### Wasm VM

The purpose of this module is to delegate light client logic to a module written in Wasm. For that,
the `08-wasm` module needs a reference (or a handler) to a Wasm VM and can directly call the [wasmvm](https://github.com/CosmWasm/wasmvm)
to interact with the VM with less overhead, fewer dependencies, and finer grain control over the Wasm client store than if using `wasmd` instead.

### Gas costs

`wasmd` has thoroughly benchmarked gas adjustments for CosmWasm with which the same values are being used in `08-wasm`'s Wasm VM.

```typescript
const (
  DefaultGasMultiplier uint64 = 140_000_000
  DefaultInstanceCost uint64 = 60_000
  DefaultCompileCost uint64 = 3
  DefaultContractMessageDataCost uint64 = 0
  DefaultDeserializationCostPerByte = 1
)
```

### Client state

The Wasm client state tracks the location of the Wasm bytecode via `codeId`. Binary data represented by the `data` field is opaque and only interpreted by the Wasm contract.

```typescript
interface ClientState {
  data: []byte
  codeId: []byte
  latestHeight: Height
}
```

### Consensus state

The Wasm consensus state tracks the timestamp (block time). Binary data represented by the `data` field is opaque and only interpreted by the Wasm contract.

```typescript
interface ConsensusState {
  data: []byte
  timestamp: uint64
}
```

### Height

The height of a Wasm light client instance consists of two `uint64`s: the revision number and the height in the revision.

```typescript
interface Height {
  revisionNumber: uint64
  revisionHeight: uint64
}
```

### Headers

Contents of Wasm client headers depend upon Wasm contract. Binary data represented by the `data` field is opaque and only interpreted by the Wasm contract.

```typescript
interface Header {
  data: []byte
  height: Height
}
```

### Misbehaviour

If applicable, the `Misbehaviour` type is used for detecting misbehaviour and freezing the client - to prevent further packet flow. 
Binary data represented by the `data` field is opaque and only interpreted by the Wasm contract, but will consist of two conflicting headers, both of which the Wasm contract would have considered valid.

```typescript
interface Misbehaviour {
  data: []byte
}
```

### Client initialization

Wasm client initialization requires a (subjectively chosen) latest consensus state and corresponding client state, interpretable by the Wasm contract. 

```typescript
function initialize(
  identifier: Identifier,
  data: []byte,
  codeId: []byte,
  consensusState: ConsensusState,
  height: Height
): ClientState {
  provableStore.set("clients/{identifier}/consensusStates/{height}", consensusState)

  // retrieve client identifier-prefixed store
  clientStore = provableStore.prefixStore("clients/{identifier}")

  // initialize wasm contract for a previously stored contract identified by codeId
  initContract(codeId, clientStore)

  return ClientState {
    data,
    codeId,
    latestHeight: height
  }
}
```

### Validity predicate

Wasm client validity checking uses underlying Wasm contract. If the provided client message is valid, the client state will proceed to checking for misbehaviour (call to `checkForMisbehaviour`) and updating state (call to either `updateStateOnMisbehaviour` or `updateState` depending on whether misbehaviour was detected in the client message). 

```typescript
function verifyClientMessage(clientMsg: ClientMessage) {
  innerPayload = Map<string, any>()
  switch typeof(clientMsg) {
    case Header:
      innerPayload["header"] = clientMsg
    case Misbehaviour:
      innerPayload["misbehavior"] = clientMsg
  }

  payload = Map<string, any>()
  payload["verify_client_message"] = Map<string, any>([
    ["client_message", innerPayload]
  ])

  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  // retrieve client identifier-prefixed store
  clientStore = provableStore.prefixStore("clients/{clientMsg.identifier}")

  // use underlying wasm contract to verify client message
  assert(callContract(marshalJSON(payload), clientState, clientStore))
}
```

### Misbehaviour predicate

Function `checkForMisbehaviour` will check if an update contains evidence of misbehaviour. Wasm client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkForMisbehaviour(clientMsg: ClientMessage): boolean	{
  innerPayload = Map<string, any>()
  switch typeof(clientMsg) {
    case Header:
      innerPayload["header"] = clientMsg
    case Misbehaviour:
      innerPayload["misbehavior"] = clientMsg
  }

  payload = Map<string, any>()
  payload["check_for_misbehaviour"] = Map<string, any>([
    ["client_message", innerPayload]
  ])

  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  // retrieve client identifier-prefixed store
  clientStore = provableStore.prefixStore("clients/{clientMsg.identifier}")

  // use underlying wasm contract to check for misbehaviour
  result = callContract(marshalJSON(payload), clientState, clientStore)
  return result.foundMisbehaviour
}
```

### State update

Function `updateState` will perform a regular update for the Wasm client. It will add a consensus state to the client store. If the header is higher than the lastest height on the `clientState`, then the `clientState` will be updated.

```typescript
function updateState(clientMsg: ClientMessage) {
  innerPayload = Map<string, any>()([
    ["header", clientMsg]
  ])

  payload = Map<string, any>()
  payload["update_state"] = Map<string, any>([
    ["client_message", innerPayload]
  ])

  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  // retrieve client identifier-prefixed store
  clientStore = provableStore.prefixStore("clients/{clientMsg.identifier}")

  // use underlying wasm contract to update client state and store new consensus state
  callContract(marshalJSON(payload), clientState, clientStore)
}
```

### State update on misbehaviour

Function `updateStateOnMisbehaviour` will set the frozen height to a non-zero height to freeze the entire client.

```typescript
function updateStateOnMisbehaviour(clientMsg: clientMessage) {
  innerPayload = Map<string, any>()
  switch typeof(clientMsg) {
    case Header:
      innerPayload["header"] = clientMsg
    case Misbehaviour:
      innerPayload["misbehavior"] = clientMsg
  }

  payload = Map<string, any>()
  payload["update_state_on_misbehaviour"] = Map<string, any>([
    ["client_message", innerPayload]
  ])

  clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
  // retrieve client identifier-prefixed store
  clientStore = provableStore.prefixStore("clients/{clientMsg.identifier}")

  // use underlying wasm contract to update client state
  callContract(marshalJSON(payload), clientState, clientStore)
}
```

### Upgrades

The chain which this light client is tracking can elect to write a special pre-determined key in state to allow the light client to update its client state (e.g. with a new chain ID or revision) in preparation for an upgrade.

As the client state change will be performed immediately, once the new client state information is written to the pre-determined key, the client will no longer be able to follow blocks on the old chain, so it must upgrade promptly.

```typescript
function upgradeClientState(
  clientState: ClientState,
  newClientState: ClientState,
  height: Height,
  proof: CommitmentProof
) {
  // Use the underlying wasm contract to verify the upgrade and
  // update the client state. The contract is passed a 
  // client identifier-prefixed store so that all state read/write 
  // operations are for the client in question.
}
```

### Proposals

Specific Wasm client params such as latest height, frozen height, trusting period (if applicable), and chain ID can be updated via a governance proposal and executed after approval.

```typescript
function checkSubstituteAndUpdateState(
  subjectClientState: ClientState, 
  substituteClientState: ClientState
) {
  // Use the underlying wasm contract to update the subject 
  // client with the state of the substitute. The contract is 
  // passed a client identifier-prefixed store so that all 
  // state read/write operations are for the client in question.
}
```

### State verification functions

Wasm client state verification functions check a proof against a previously validated commitment root.

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
  // assert that enough time has elapsed
  assert(currentTimestamp() >= processedTime + delayPeriodTime)
  // assert that enough blocks have elapsed
  assert(currentHeight() >= processedHeight + delayPeriodBlocks)

  payload = Map<string, any>()
  payload["verify_membership"] = Map<string, any>([
    ["height", height],
    ["proof", proof],
    ["path", path],
    ["value", value]
  ])

  // retrieve client identifier-prefixed store
	clientStore = provableStore.prefixStore("clients/{clientIdentifier}")

  // use underlying wasm contract to verify that <path, value> has been stored
  result = callContract(marshalJSON(payload), clientState, clientStore)
  return result.error
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
  // assert that enough time has elapsed
  assert(currentTimestamp() >= processedTime + delayPeriodTime)
  // assert that enough blocks have elapsed
  assert(currentHeight() >= processedHeight + delayPeriodBlocks)

  payload = Map<string, any>()
  payload["verify_non_membership"] = Map<string, any>([
    ["height", height],
    ["proof", proof],
    ["path", path]
  ])

  // retrieve client identifier-prefixed store
	clientStore = provableStore.prefixStore("clients/{clientIdentifier}")

  // use underlying wasm contract to verify that nothing has been stored at path
  result = callContract(marshalJSON(payload), clientState, clientStore)
  return result.error
}
```
### Wasm Contract Interface

#### Interaction between Go and WASM

When an instruction needs to be executed in WASM code, functions are executed using a `wasmvm`.
The process requires packaging all the arguments to be executed by a specific function (including
pointers to `KVStore`s if needed), pointing to a code hash, and a `sdk.GasMeter` to properly account
for gas usage during the execution of the function.

#### Contract instantiation

Instantiation of a contract is minimal. No data is passed in the message for the contract call, but the
Wasm client store is passed. This allows for a Wasm contract to initialize any metadata that they
need such as processed height and/or processed time.

#### Contract query

Every Wasm contract must to support these query messages:

```rust
#[cw_serde]
pub struct StatusMsg {}

#[cw_serde]
pub struct ExportMetadataMsg {}
```

The response for queries is as follows:

```rust
#[cw_serde]
pub struct QueryResponse {
  pub status: String, // Status query response
  #[serde(skip_serializing_if = "Option::is_none")]
  pub genesis_metadata: Option<Vec<GenesisMetadata>>, // metadata KV pairs
}

#[cw_serde]
pub struct GenesisMetadata {
  pub key: Vec<u8>,
  pub value: Vec<u8>,
}
```

#### Contract execute

Every Wasm contract must support these execute messages:

```rust
#[cw_serde]
pub struct VerifyMembershipMsg {
  #[schemars(with = "String")]
  #[serde(with = "Base64", default)]
  pub proof: Bytes,
  pub path: MerklePath,
  #[schemars(with = "String")]
  #[serde(with = "Base64", default)]
  pub value: Bytes,
  pub height: Height,
  pub delay_block_period: u64,
  pub delay_time_period: u64,
}

#[cw_serde]
pub struct VerifyNonMembershipMsg {
  #[schemars(with = "String")]
  #[serde(with = "Base64", default)]
  pub proof: Bytes,
  pub path: MerklePath,
  pub height: HeightRaw,
  pub delay_block_period: u64,
  pub delay_time_period: u64,
}

#[cw_serde]
pub struct VerifyClientMessage {
  pub client_message: ClientMessage,
}

#[cw_serde]
pub enum ClientMessage {
  Header(WasmHeader),
  Misbehaviour(WasmMisbehaviour),
}

#[cw_serde]
pub struct CheckForMisbehaviourMsg {
  pub client_message: ClientMessage,
}

#[cw_serde]
pub struct UpdateStateOnMisbehaviourMsg {
  pub client_message: ClientMessage,
}

#[cw_serde]
pub struct UpdateStateMsg {
  pub client_message: ClientMessage,
}

#[cw_serde]
pub struct CheckSubstituteAndUpdateStateMsg {
  substitute_client_msg: Vec<u8>,
}

#[cw_serde]
pub struct VerifyUpgradeAndUpdateStateMsg {
  pub upgrade_client_state: WasmClientState,
  pub upgrade_consensus_state: WasmConsensusState,
  #[schemars(with = "String")]
  #[serde(with = "Base64", default)]
  pub proof_upgrade_client: Vec<u8>,
  #[schemars(with = "String")]
  #[serde(with = "Base64", default)]
  pub proof_upgrade_consensus_state: Vec<u8>,
}
```

The response for execute is as follows:

```rust
#[cw_serde]
pub struct ContractResult {
  pub is_valid: bool, // Execute was successful
  pub error_msg: String, // Error message
  #[serde(skip_serializing_if = "Option::is_none")]
  pub data: Option<Vec<u8>>, // Optional data
  pub found_misbehaviour: bool, // Misbehaviour was found
}
```

### Properties & Invariants

Correctness guarantees as provided by the underlying algorithm implemented by Wasm contract.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

As long as Wasm contract keeps its interface consistent with `ICS 02` it should be forward compatible

## Example Implementation

Implementation of ICS 08 in Go can be found in [ibc-go PR](https://github.com/cosmos/ibc-go/pull/3355).

## Other Implementations

None at present.

## History

Oct 8, 2021 - Final first draft

Mar 15th, 2022 - Update for 02-client refactor 

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
