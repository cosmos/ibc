---
ics: 8
title: Wasm Client
stage: draft
category: IBC/TAO
kind: instantiation
implements: 2
author: Parth Desai <parth@chorus.one>, Mateusz Kaczanowski <mateusz@chorus.one>, Blas Rodriguez Irizar <blas@composable.finance>
created: 2020-10-13
modified: 2022-12-15
---

## Synopsis

This specification document describes an interface to a light client stored as a WASM bytecode for a blockchain.

### Motivation

Currently adding a new client implementation or upgrading an existing one requires a hard upgrade because the client implementations are part of the static chain binary. Any change to the on-chain light client code is dependent on chain governance to approve an upgrade before it can be deployed.

This may be acceptable when adding new client types since the number of unique consensus algorithms that need to be supported is currently small. However, this process will become very tedious when it comes to upgrading light clients.

Without dynamically upgradable light clients, a chain that wishes to upgrade its consensus algorithm (and thus break existing light clients) must wait for all counterparty chains to perform a hard upgrade that adds support for the upgraded light client before it itself can perform an upgrade on its chain. Examples of a consensus-breaking upgrade would be an upgrade from Tendermint v1 to a light-client breaking Tendermint v2 or switching from Tendermint consensus to Honeybadger. Changes to the internal state-machine logic will not affect consensus, e.g. changes to staking module do not require an IBC upgrade.

Requiring all counterparties to statically add new client implementations to their binaries will inevitably slow the pace of upgrades in the IBC network since the deployment of an upgrade on even a very experimental, fast-moving chain will be blocked by an upgrade to a high-value chain that will be inherently more conservative.

Once the IBC network broadly adopts dynamically upgradable clients, a chain may upgrade its consensus algorithm whenever it wishes and relayers may upgrade the client code of all counterparty chains without requiring the counterparty chains to perform an upgrade themselves. This prevents a dependency on counterparty chains when considering upgrading one's consensus algorithm.

Another reason why this interface is beneficial is that it removes the dependency between Light clients and the Go programming language. By using WASM
as a compilation target, light clients can be written in any programming language whose toolchain includes WASM as a compilation target. Examples of
these are Go, Rust, C, and C++.


### Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

`currentTimestamp` is as defined in [ICS 24](../../core/ics-024-host-requirements).

`Wasm Client Code` refers to Wasm bytecode stored in the client store, which provides a target blockchain specific implementation of [ICS 2](../../core/ics-002-client-semantics).

`Wasm Client` refers to a particular instance of `Wasm Client Code` defined as a tuple `(Wasm Client Code, ClientID)`.

`Wasm VM` refers to a virtual machine capable of executing valid Wasm bytecode.


### Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

This specification depends on the correct instantiation of the `WASM client` and is decoupled from any specific implementation of the target `blockchain` consensus algorithm.

### Client state

The Wasm client state tracks the location of the Wasm bytecode via `codeHash`. Binary data represented by `data` field is opaque and only interpreted by the Wasm Client Code. `type` represents client type.
`type` and `codeHash` both are immutable.

```typescript
interface ClientState {
  codeHash: []byte
  data: []byte
  latestHeight: Height
}
```

### Consensus state

The Wasm consensus state tracks the timestamp (block time), `WASM Client` code-specific fields and commitment root for all previously verified consensus states.
`type` and `codeHash` both are immutable.

```typescript
interface ConsensusState {
  codeHash: []byte
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

This is designed to allow the height to reset to `0` while the revision number increases by one to preserve timeouts through zero-height upgrades.

### Headers

Contents of Wasm client headers depend upon `WASM Client` Code.

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
  data: []byte
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

Wasm client validity checking uses underlying Wasm Client code. If the provided header is valid, the client state is updated & the newly verified commitment is written to the store.

```typescript
function CheckSubstituteAndUpdateState(
  substituteClient: ClientState,
  header: Header) {
    store = getStore("clients/{identifier}")

    consensusState, err := GetConsensusState(subjectClientStore, cdc, c.LatestHeight)

    codeHandle = clientState.codeHandle()

    // verify that provided header is valid and state is saved
    assert(codeHandle.validateHeaderAndCreateConsensusState(store, clientState, header))
}
```

### Misbehaviour predicate

Wasm client misbehaviour checking determines whether or not two conflicting headers at the same height would have convinced the light client.

```typescript
function checkForMisbehaviour(
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

### State verification functions

Wasm client state verification functions check a Merkle proof against a previously validated commitment root.

```go
	func (c ClientState) VerifyUpgradeAndUpdateState(
		ctx sdk.Context,
		cdc codec.BinaryCodec,
		store sdk.KVStore,
		newClient ClientState,
		newConsState ConsensusState,
		proofUpgradeClient,
		proofUpgradeConsState []byte,
	) error {
    // check that consensus state is of type wasm consensusstate
    	wasmUpgradeConsState, ok := newConsState.(*ConsensusState)
	if !ok {
		return sdkerrors.Wrapf(clienttypes.ErrInvalidConsensus, "upgraded consensus state must be wasm light consensus state. expected %T, got: %T",
			&ConsensusState{}, wasmUpgradeConsState)
	}
	// last height of current counterparty chain must be client's latest height
	lastHeight := c.LatestHeight
	_, err := GetConsensusState(store, cdc, lastHeight)
	if err != nil {
		return sdkerrors.Wrap(err, "could not retrieve consensus state for lastHeight")
	}
  encodedData := packData(newClient, proofUpgradeClient, proofUpgradeConsState)
  out, err := callContract(c.CodeId, ctx, store, encodedData)
	if err != nil {
		return sdkerrors.Wrapf(ErrUnableToCall, fmt.Sprintf("underlying error: %s", err.Error()))
	}
  return nil
  }


  	func (c ClientState) VerifyMembership(
		ctx sdk.Context,
		clientStore sdk.KVStore,
		cdc codec.BinaryCodec,
		height Height,
		delayTimePeriod uint64,
		delayBlockPeriod uint64,
		proof []byte,
		path Path,
		value []byte,
	) error {
    	const VerifyClientMessage = "verify_membership"
    inner := make(map[string]interface{})
    inner["height"] = height
    inner["delay_time_period"] = delayTimePeriod
    inner["delay_block_period"] = delayBlockPeriod
    inner["proof"] = proof
    inner["path"] = path
    inner["value"] = value
    payload := make(map[string]map[string]interface{})
    payload[VerifyClientMessage] = inner

    _, err := call[contractResult](payload, &c, ctx, clientStore)
    return err
  }

  func (c ClientState) VerifyNonMembership(
	ctx sdk.Context,
	clientStore sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	delayTimePeriod uint64,
	delayBlockPeriod uint64,
	proof []byte,
	path []byte,
) error {
	const VerifyClientMessage = "verify_non_membership"
	inner := make(map[string]interface{})
	inner["height"] = height
	inner["delay_time_period"] = delayTimePeriod
	inner["delay_block_period"] = delayBlockPeriod
	inner["proof"] = proof
	inner["path"] = path
	payload := make(map[string]map[string]interface{})
	payload[VerifyClientMessage] = inner

	_, err := call[contractResult](payload, &c, ctx, clientStore)
	return err
}

func (c ClientState) VerifyClientMessage(ctx sdk.Context, cdc codec.BinaryCodec, clientStore sdk.KVStore, clientMsg exported.ClientMessage) error {
  encodedData := packData(clientMsg, c)
	_, err := call[contractResult](encodedData, &c, ctx, clientStore)
	return err
}

func (c ClientState) CheckForMisbehaviour(ctx sdk.Context, cdc codec.BinaryCodec, clientStore sdk.KVStore, msg exported.ClientMessage) bool {
	wasmMisbehaviour, ok := msg.(*Misbehaviour)
	if !ok {
		return false
	}
  encodedData := packData(wasmMisbehaviour)
	_, err := call[contractResult](encodedData, &c, ctx, clientStore)
	if err != nil {
		panic(err)
	}

	return true
}

// UpdateStateOnMisbehaviour should perform appropriate state changes on a client state given that misbehaviour has been detected and verified
func (c ClientState) UpdateStateOnMisbehaviour(ctx sdk.Context, cdc codec.BinaryCodec, clientStore sdk.KVStore, clientMsg exported.ClientMessage) {
    encodedData := packData(clientMsg, c)
	_, err = callContract(c.CodeId, ctx, clientStore, encodedData)
	if err != nil {
		panic(err)
	}
}

func (c ClientState) UpdateState(ctx sdk.Context, cdc codec.BinaryCodec, clientStore sdk.KVStore, clientMsg exported.ClientMessage) []exported.Height {
  clientMsgConcrete := make(map[string]interface{})
  switch clientMsg := clientMsg.(type) {
    case *Header:
      clientMsgConcrete["header"] = clientMsg
    case *Misbehaviour:
      clientMsgConcrete["misbehaviour"] = clientMsg
  }
  encodedData := packData(clientMsgConcrete)
	output, err := call[contractResult](  encodedData := packData(clientMsgConcrete)
, &c, ctx, clientStore)
	if err != nil {
		panic(err)
	}
  if err := json.Unmarshal(output.Data, &c); err != nil {
  panic(sdkerrors.Wrapf(ErrUnableToUnmarshalPayload, fmt.Sprintf("underlying error: %s", err.Error())))
}
	SetClientState(clientStore, cdc, &c)
	return []exported.Height{c.LatestHeight}
}

```
### Wasm Client Code Interface

#### Interaction between Go and WASM?
When an instruction needs to be executed in WASM code, functions are executed using a `wasmvm`.
The process requires packaging all the arguments to be executed by a specific function (including
pointers to `KVStore`s if needed), pointing to a code hash, and a `sdk.GasMeter` to properly account
for gas usage during the execution of the function.


```go
func (c *CodeHandle) isValidClientState(ctx sdk.Context, clientState ClientState, height u64) (*types.Response, error) {
    clientStateData := json.Serialize(clientState)
    packedData := pack(clientStateData, height)
    // VM specific code to call Wasm contract
    desercost := types.UFraction{Numerator: 1, Denominator: 1}
    return callContract(codeID, ctx, store, packedData)
}
```

```go
func callContract(codeID []byte, ctx sdk.Context, store sdk.KVStore, msg []byte) (*types.Response, error) {
	gasMeter := ctx.GasMeter()
	chainID := ctx.BlockHeader().ChainID
	height := ctx.BlockHeader().Height
	// safety checks before casting below
	if height < 0 {
		panic("Block height must never be negative")
	}
	sec := ctx.BlockTime().Unix()
	if sec < 0 {
		panic("Block (unix) time must never be negative ")
	}
	env := types.Env{
		Block: types.BlockInfo{
			Height:  uint64(height),
			Time:    uint64(sec),
			ChainID: chainID,
		},
		Contract: types.ContractInfo{
			Address: "",
		},
	}

	return callContractWithEnvAndMeter(codeID, ctx, store, env, gasMeter, msg)
}
```

```go
func callContractWithEnvAndMeter(codeID cosmwasm.Checksum, ctx sdk.Context, store sdk.KVStore, env types.Env, gasMeter sdk.GasMeter, msg []byte) (*types.Response, error) {
	msgInfo := types.MessageInfo{}
	desercost := types.UFraction{Numerator: 1, Denominator: 1}
	resp, gasUsed, err := WasmVM.Execute(codeID, env, msgInfo, msg, store, cosmwasm.GoAPI{}, nil, gasMeter, gasMeter.Limit(), desercost)
	if &ctx != nil {
		consumeGas(ctx, gasUsed)
	}
	return resp, err
}
```

#### Wasm Client interface
Every Wasm client code needs to support the ingestion of the below messages to be used as a light client.

```rust
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct MisbehaviourMessage {
    pub client_id: Vec<u8>,
    pub data: Vec<u8>,
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct CreateConsensusMessage {
    pub client_state: Vec<u8>,
    pub height: Height
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct InitializeClientStateMessage {
    pub initialization_data: Vec<u8>,
    pub consensus_state: Vec<u8>
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
    client_state: Vec<u8>,
    height: Height
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct ValidateNewClientStateMessage {
    client_state: Vec<u8>,
    new_client_state: Vec<u8>,
    height: Height
}

#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub struct ValidateInitializationDataMessage {
    init_data: Vec<u8>,
    consensus_state: Vec<u8>
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
   LatestClientHeight(Vec<u8>),
}

```

### Properties & Invariants

Correctness guarantees as provided by the underlying algorithm implemented by `Wasm Client Code`.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

As long as `Wasm Client Code` keeps its interface consistent with `ICS 02` it should be forward compatible

## Example Implementation

None yet.

## Other Implementations

None at present.

## History


## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
