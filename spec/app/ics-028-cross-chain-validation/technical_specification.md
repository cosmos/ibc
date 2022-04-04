<!-- omit in toc -->
# CCV: Technical Specification
[&uparrow; Back to main document](./README.md)

<!-- omit in toc -->
## Outline
- [Placing CCV within an ABCI Application](#placing-ccv-within-an-abci-application)
- [Data Structures](#data-structures)
  - [External Data Structures](#external-data-structures)
  - [CCV Data Structures](#ccv-data-structures)
  - [CCV Packets](#ccv-packets)
  - [CCV State](#ccv-state)
- [Sub-protocols](#sub-protocols)
  - [Initialization](#initialization)
  - [Channel Closing Handshake](#channel-closing-handshake)
  - [Packet Relay](#packet-relay)
  - [Validator Set Update](#validator-set-update)
  - [Consumer Initiated Slashing](#consumer-initiated-slashing)

## Placing CCV within an ABCI Application
[&uparrow; Back to Outline](#outline)

Before describing the data structures and sub-protocols of the CCV protocol, we provide a short overview of the interfaces the CCV module implements and the interactions with the other ABCI application modules.

<!-- omit in toc -->
### Implemented Interfaces

- CCV is an **ABCI application module**, which means it MUST implement the logic to handle some of the messages received from the consensus engine via ABCI, e.g., `InitChain`, `EndBlock` 
  (for more details, take a look at the [ABCI specification](https://github.com/tendermint/spec/tree/v0.7.1/spec/abci)). 
  In this specification we define the following methods that handle messages that are of particular interest to the CCV protocol:
  - `InitGenesis()` -- Called when the chain is first started, on receiving an `InitChain` message from the consensus engine. 
    This is also where the application can inform the underlying consensus engine of the initial validator set.
  - `BeginBlock()` -- Contains logic that is automatically triggered at the beginning of each block. 
  - `EndBlock()` -- Contains logic that is automatically triggered at the end of each block. 
    This is also where the application can inform the underlying consensus engine of changes in the validator set.

- CCV is an **IBC module**, which means it MUST implement the module callbacks interface defined in [ICS 26](../../core/ics-026-routing-module/README.md#module-callback-interface). The interface consists of a set of callbacks for 
  - channel opening handshake, which we describe in the [Initialization](#initialization) section;
  - channel closing handshake, which we describe in the [Channel Closing Handshake](#channel-closing-handshake) section;
  - and packet relay, which we describe in the [Packet Relay](#packet-relay) section.

<!-- omit in toc -->
### Interfacing Other Modules

- As an ABCI application module, the CCV module interacts with the underlying consensus engine through ABCI:
  - On the provider chain,
    - it initializes the application (e.g., binds to the expected IBC port) in the `InitGenesis()` method.
  - On the consumer chain,
    - it initializes the application (e.g., binds to the expected IBC port, creates a client of the provider chain) in the `InitGenesis()` method;
    - it provides the validator updates in the `EndBlock()` method.

- As an IBC module, the CCV module interacts with Core IBC for functionalities regarding
  - port allocation ([ICS 5](../../core/ics-005-port-allocation)) via `portKeeper`;
  - channels and packet semantics ([ICS 4](../../core/ics-004-channel-and-packet-semantics)) via `channelKeeper`;
  - connection semantics ([ICS 3](../../core/ics-003-connection-semantics)) via `connectionKeeper`;
  - client semantics ([ICS 2](../../core/ics-002-client-semantics)) via `clientKeeper`.

- For the [Initialization sub-protocol](#initialization), the provider CCV module interacts with a Governance module by handling governance proposals to spawn new consumer chains. 
  If such proposals pass, then all validators on the provider chain MUST validate the consumer chain at spawn time; 
  otherwise they get slashed. 
  For an example of how governance proposals work, take a look at the [Governance module documentation](https://docs.cosmos.network/v0.44/modules/gov/) of Cosmos SDK. 

- The provider CCV module interacts with a Staking module on the provider chain. 
  For an example of how staking works, take a look at the [Staking module documentation](https://docs.cosmos.network/v0.44/modules/staking/) of Cosmos SDK. 
  The interaction is defined by the following interface:
  ```typescript 
  interface StakingKeeper {
    // get UnbondingPeriod from the provider Staking module 
    UnbondingTime(): Duration

    // get validator updates from the provider Staking module
    GetValidatorUpdates(): [ValidatorUpdate]

    // notify the Staking module of unboding operations that
    // have matured from the consumer chain's perspective 
    CompleteStoppedUnbonding(id: uint64)
  }
  ```

- The provider CCV modules interact with a Slashing module on the provider chain. 
  For an example of how slashing works, take a look at the [Slashing module documentation](https://docs.cosmos.network/v0.44/modules/slashing/) of Cosmos SDK. 
  The interaction is defined by the following interface:
  ```typescript 
  interface SlashingKeeper {
    // request the Staking module to slash a validator
    Slash(valAddress: string, 
          infractionHeight: int64, 
          power: int64, 
          slashFactor: int64)

    // request the Slashing module to jail a validator until time
    JailUntil(valAddress: string, time: uint64)
  }
  ```

- The following hooks enable the provider CCV module to register operations to be execute when certain events occur within the provider Staking module:
  ```typescript
  // invoked by the Staking module after 
  // initiating an unbonding operation
  function AfterUnbondingOpInitiated(opId: uint64);

  // invoked by the Staking module before 
  // completing an unbonding operation
  function BeforeUnbondingOpCompleted(opId: uint64): Bool;
  ```

- The consumer CCV module defines the following hooks that enable other modules to register operations to execute when certain events have occurred within CCV:
  ```typescript
  // invoked after a new validator is added to the validator set
  function AfterCCValidatorBonded(valAddress: string);

  // invoked after a validator is removed from the validator set
  function AfterCCValidatorBeginUnbonding(valAddress: string);
  ```

## Data Structures

### External Data Structures
[&uparrow; Back to Outline](#outline)

This section describes external data structures used by the CCV module.

The CCV module uses the ABCI `ValidatorUpdate` data structure, which consists of a validator and its power (for more details, take a look at the [ABCI specification](https://github.com/tendermint/spec/blob/v0.7.1/spec/abci/abci.md#data-types)), i.e.,
```typescript
interface ValidatorUpdate {
  pubKey: PublicKey
  power: int64
}
```
The provider chain sends to the consumer chain a list of `ValidatorUpdate`s, containing an entry for every validator that had its power updated. 

The data structures required for creating clients (i.e., `ClientState`, `ConsensusState`) are defined in [ICS 7](../../client/ics-007-tendermint-client).

### CCV Data Structures
[&uparrow; Back to Outline](#outline)

The CCV module is initialized through the `InitGenesis` method when the chain is first started. The initialization is done from a genesis state. This is the case for both provider and consumer chains:
- On the provider chain, the genesis state is described by the following interface:
  ```typescript
  interface ProviderGenesisState {
    // a list of existing consumer chains
    consumerStates: [ConsumerState]
  }
  ```
  with `ConsumerState` defined as
  ```typescript
  interface ConsumerState {
    chainId: string
    channelId: Identifier
  }
  ```
- On the consumer chain, the genesis state is described by the following interface:
  ```typescript
  interface ConsumerGenesisState {
    providerClientState: ClientState
    providerConsensusState: ConsensusState
    initialValSet: [ValidatorUpdate]
  }
  ```

The provider CCV module handles governance proposals to spawn new consumer chains. The structure of these proposals is defined by the `Proposal` interface in the [Governance module documentation](https://docs.cosmos.network/v0.44/modules/gov/). The content of these proposals is described by the following interface (we omit typical fields such as title and description):
  ```typescript
  interface CreateConsumerChainProposal {
    // The proposed chain ID of the new consumer chain.
    // Must be different from all other consumer chain IDs 
    // of the executing proposer chain.
    chainId: string

    // The proposed initial height of new consumer chain.
    // For a completely new chain, this will be {0,1}; 
    // however, it may be different if this is a chain 
    // that is converting to a consumer chain.
    initialHeight: Height

    // Spawn time is the time on the provider chain at which 
    // the consumer chain genesis is finalized and all validators
    // will be responsible for starting their consumer chain 
    // validator node.
    spawnTime: Timestamp

    // the hash of the consumer chain pre-CCV genesis state, i.e.,
    // the genesis state except the consumer CCV module genesis state;  
    // the pre-CCV genesis state MAY be disseminated off-chain
    genesisHash: [byte]
  }
  ```
  Note that `Height` is defined in [ICS 7](../../client/ics-007-tendermint-client).

### CCV Packets
[&uparrow; Back to Outline](#outline)

The structure of the packets sent through the CCV channel is defined by the `Packet` interface in [ICS 4](../../core/ics-004-channel-and-packet-semantics). 
The following packet data types are required by the CCV module:
- `VSCPacketData` contains a list of validator updates, i.e., 
    ```typescript
    interface VSCPacketData {
      // the id of this VSC
      id: uint64 
      // validator updates
      updates: [ValidatorUpdate]
      // slash requests acknowledgements, 
      // i.e., list of validator addresses
      slashAcks: [string]
    }
    ```
- `VSCMaturedPacketData` contains the ID of the VSC that reached maturity, i.e., 
    ```typescript
    interface VSCMaturedPacketData {
      id: uint64 // the id of the VSC that reached maturity
    }
    ```
- `SlashPacketData` contains a request to slash a validator, i.e.,
  ```typescript
    interface SlashPacketData {
      valAddress: string // validator address, i.e., the hash of its public key
      valPower: int64
      vscId: uint64
      slashFactor: int64
      jailTime: uint64
    }
    ```
> Note that for brevity we use e.g., `VSCPacket` to refer to a packet with `VSCPacketData` as its data.

Packets are acknowledged by the remote side by sending back an `Acknowledgement` that contains either a result (in case of success) or an error (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics/README.md#acknowledgement-envelope)). 
The following acknowledgement types are required by the CCV module:
```typescript
type VSCPacketAcknowledgement = VSCPacketSuccess | VSCPacketError;
type VSCMaturedPacketAcknowledgement = VSCMaturedPacketSuccess | VSCMaturedPacketError;
type SlashPacketAcknowledgement = SlashPacketSuccess | SlashPacketError;
type PacketAcknowledgement = PacketSuccess | PacketError; // general ack
```

### CCV State
[&uparrow; Back to Outline](#outline)

This section describes the internal state of the CCV module. For simplicity, the state is described by a set of variables; for each variable, both the type and a brief description is provided. In practice, all the state (except for hardcoded constants, e.g., `ProviderPortId`) is stored in a key/value store (KVS). The host state machine provides a KVS interface with three functions, i.e., `get()`, `set()`, and `delete()` (as defined in [ICS 24](../../core/ics-024-host-requirements)).

<!-- omit in toc -->
#### State on the provider chain

- `ProviderPortId = "provider"` is the port ID the provider CCV module is expected to bind to.
- `pendingClient: Map<(Timestamp, string), Height>` is a mapping from `(timestamp, chainId)` tuples to the initial height of pending clients, i.e., belonging to consumer chains that were not yet spawned, but for which a `CreateConsumerChainProposal` was received.
- `chainToClient: Map<string, Identifier>` is a mapping from consumer chain IDs to the associated client IDs.
- `chainToChannel: Map<string, Identifier>` is a mapping from consumer chain IDs to the CCV channel IDs.
- `channelToChain: Map<Identifier, string>` is a mapping from CCV channel IDs to consumer chain IDs.
- `pendingVSCPackets: Map<string, [VSCPacketData]>` is a mapping from consumer chain IDs to a list of pending `VSCPacketData`s that must be sent to the consumer chain once the CCV channel is established. The map exposes the following interface: 
  ```typescript
  interface Map<string, [VSCPacketData]> {
    // append a VSCPacketData to the list mapped to chainId;
    // the list is modified
    Append(chainId: string, data: VSCPacketData) 

    // remove all the VSCPacketData mapped to chainId;
    // the list is modified
    Remove(chainId: string)
  }
- `vscId: uint64` is a monotonic strictly increasing and positive ID that is used to uniquely identify the VSCs sent to the consumer chains. 
  Note that `0` is used as a special ID for the mapping from consumer heights to provider heights.
- `initH: Map<string, Height>` is a mapping from consumer chain IDs to the heights on the provider chain. 
  For every consumer chain, the mapping stores the height when the first VSC was provided to that consumer chain. 
  It enables the mapping from consumer heights to provider heights.
- `VSCtoH: Map<uint64, Height>` is a mapping from VSC IDs to heights on the provider chain. It enables the mapping from consumer heights to provider heights, 
  i.e., the voting power at height `VSCtoH[id]` on the provider chain was last updated by the validator updates contained in the VSC with ID `id`.  
- `unbondingOps: Map<uint64, UnbondingOperation>` is a mapping that enables accessing for every unbonding operation the list of consumer chains that are still unbonding. When unbonding operations are initiated, the Staking module calls the `AfterUnbondingOpInitiated()` [hook](#ccv-pcf-hook-afubopcr1); this leads to the creation of a new `UnbondingOperation`, which is defined as
  ```typescript
  interface UnbondingOperation {
    id: uint64
    // list of consumer chain IDs that are still unbonding
    unbondingChainIds: [string] 
  }
  ```
- `vscToUnbondingOps: Map<(Identifier, uint64), [uint64]>` is a mapping from `(chainId, vscId)` tuples to a list of unbonding operation IDs. 
  It enables the provider CCV module to match a `VSCMaturedPacket{vscId}`, received from a consumer chain with `chainId`, with the corresponding unbonding operations. 
  As a result, `chainId` can be removed from the list of consumer chains that are still unbonding these operations. 
  For more details see how received `VSCMaturedPacket`s [are handled](#ccv-pcf-rcvmat1).
- `slashRequests: Map<(string, [string])>` is a mapping from `chainId`s to lists of validator addresses, 
  i.e., `slashRequests[chainId]` contains all the validator addresses for which the provider chain received slash requests from the consumer chain with `chainId`.

<!-- omit in toc -->
#### State on the consumer chain
- `ConsumerPortId = "consumer"` is the port ID the consumer CCV module is expected to bind to.
- `providerClient: Identifier` identifies the client of the provider chain (on the consumer chain) that the CCV channel is build upon.
- `providerChannel: Identifier` identifies the consumer's channel end of the CCV channel.
- `validatorSet: <string, CrossChainValidator>` is a mapping that stores the validators in the validator set of the consumer chain. Each validator is described by a `CrossChainValidator` data structure, which is defined as
  ```typescript
  interface CrossChainValidator {
    address: string // validator address, i.e., the hash of its public key
    power: int64
  }
  ```
- `pendingChanges: [ValidatorUpdate]` is a list of `ValidatorUpdate`s received, but not yet applied to the validator set. 
  It is emptied on every `EndBlock()`. The list exposes the following interface:
  ```typescript
  interface [ValidatorUpdate] {
    // append updates to the list;
    // the list is modified
    Append(updates: [ValidatorUpdate]) 

    // return an aggregated list of updates, i.e., 
    // keep only the latest update per validator;
    // the original list is not modified
    Aggregate(): [ValidatorUpdate]

    // remove all the updates from the list;
    // the list is modified
    RemoveAll()
  }
- `HtoVSC: Map<Height, uint64>` is a mapping from consumer chain heights to VSC IDs. It enables the mapping from consumer heights to provider heights., i.e.,
  - if `HtoVSC[h] == 0`, then the voting power on the consumer chain at height `h` was setup at genesis during Channel Initialization;
  - otherwise, the voting power on the consumer chain at height `h` was updated by the VSC with ID `HtoVSC[h]`.
- `maturingVSCs: [(uint64, uint64)]` is a list of `(id, ts)` tuples, where `id` is the ID of a VSC received via a `VSCPacket` and `ts` is the timestamp at which the VSC reaches maturity on the consumer chain. 
  The list is used to keep track of when unbonding operations are matured on the consumer chain. It exposes the following interface:
  ```typescript
  interface [(uint64, uint64)] {
    // add a VSC id with its maturity timestamp to the list;
    // the list is modified
    Add(id: uint64, ts: uint64)

    // return the list sorted by the maturity timestamps;
    // the original list is not modified
    SortedByMaturityTime(): [(uint64, uint64)]

    // remove (id, ts) from the list;
    // the list is modified
    Remove(id: uint64, ts: uint64)
  }
  ```
- `pendingSlashRequests: [SlashRequest]` is a list of pending `SlashRequest`s that must be sent to the provider chain once the CCV channel is established. A `SlashRequest` consist of a `SlashPacketData` and a flag indicating whether the request is for downtime slashing. The list exposes the following interface: 
  ```typescript
  interface SlashRequest {
    data: SlashPacketData
    downtime: Bool
  }
  interface [SlashRequest] {
    // append a SlashRequest to the list;
    // the list is modified
    Append(data: SlashRequest) 

    // return the reverse list, i.e., latest SlashRequest first;
    // the original list is not modified
    Reverse(): [SlashRequest]

    // remove all the SlashRequest;
    // the list is modified
    RemoveAll()
  }
  ```
- `outstandingDowntime: <string, Bool>` is a mapping from validator addresses to boolean values. 
  `outstandingDowntime[valAddr] == TRUE` entails that the consumer chain sent a request to slash for downtime the validator with address `valAddr`. 
  `outstandingDowntime[valAddr]` is set to false once the consumer chain receives a confirmation that the slash request was received by the provider chain, i.e., a `VSCPacket` that contains `valAddr` in `slashAcks`. 
  The mapping enables the consumer CCV module to avoid sending to the provider chain multiple slashing requests for the same downtime infraction.
 
## Sub-protocols

To express the error conditions, the following specification of the sub-protocols uses the exception system of the host state machine, which is exposed through two functions (as defined in [ICS 24](../../core/ics-024-host-requirements)): `abortTransactionUnless` and `abortSystemUnless`.

### Initialization
[&uparrow; Back to Outline](#outline)

The *initialization* sub-protocol enables a provider chain and a consumer chain to create a CCV channel -- a unique, ordered IBC channel for exchanging packets. As a prerequisite, the initialization sub-protocol MUST create two IBC clients, one on the provider chain to the consumer chain and one on the consumer chain to the provider chain. This is necessary to verify the identity of the two chains (as long as the clients are trusted). 

<!-- omit in toc -->
#### **[CCV-PCF-INITG.1]**
```typescript
// PCF: Provider Chain Function
// implements the AppModule interface
function InitGenesis(state: ProviderGenesisState): [ValidatorUpdate] {
  // bind to ProviderPortId port 
  err = portKeeper.BindPort(ProviderPortId)
  // check whether the capability for the port can be claimed
  abortSystemUnless(err == nil)

  foreach cs in state.consumerStates {
    abortSystemUnless(validateChannelIdentifier(cs.channelId))
    chainToChannel[cs.chainId] = cs.channelId
    channelToChain[cs.channelId] = cc.chainId
  }

  // do not return anything to the consensus engine 
  return []
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - An `InitChain` message is received from the consensus engine; the `InitChain` message is sent when the provider chain is first started. 
- **Precondition**
  - The provider CCV module is in the initial state. 
- **Postcondition**
  - The capability for the port `ProviderPortId` is claimed.
  - For each consumer state in the `ProviderGenesisState`, the initial state is set, i.e., the following mappings `chainToChannel`, `channelToChain` are set.
- **Error Condition**
  - The capability for the port `ProviderPortId` cannot be claimed.
  - For any consumer state in the `ProviderGenesisState`, the channel ID is not valid (cf. the validation function defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)).

<!-- omit in toc -->
#### **[CCV-PCF-CCPROP.1]**
```typescript
// PCF: Provider Chain Function
// implements governance proposal Handler 
function CreateConsumerChainProposal(p: CreateConsumerChainProposal) {
  if currentTimestamp() > p.spawnTime {
    // get UnbondingPeriod from provider Staking module
    unbondingTime = stakingKeeper.UnbondingTime()

    // create client state as defined in ICS 7
    clientState = ClientState{
      chainId: p.chainId,
      trustLevel: DefaultTrustLevel, // 1/3
      trustingPeriod: unbondingTime/2,
      unbondingPeriod: unbondingTime,
      latestHeight: p.initialHeight,
    }

    // create consensus state as defined in ICS 7;
    // SentinelRoot is used as a stand-in root value for 
    // the consensus state set at the upgrade height;
    // the validator set is the same as the validator set 
    // from own consensus state at current height
    ownConsensusState = getConsensusState(getCurrentHeight())
    consensusState = ConsensusState{
      timestamp: currentTimestamp(),
      commitmentRoot: SentinelRoot,
      validatorSet: ownConsensusState.validatorSet,
    }

    // create consumer chain client and store it
    clientId = clientKeeper.CreateClient(clientState, consensusState)
    chainToClient[p.chainId] = clientId
  }
  else {
    // store the client as a pending client
    pendingClient[(p.spawnTime, p.chainId)] = p.initialHeight
  }
}
```
- **Caller**
  - `EndBlock()` method of Governance module.
- **Trigger Event**
  - A governance proposal with `CreateConsumerChainProposal` as content has passed (i.e., it got the necessary votes).
- **Precondition** 
  - True. 
- **Postcondition** 
  - If the spawn time has already passed,
    - `UnbondingPeriod` is retrieved from the provider Staking module;
    - a client state is created;
    - a consensus state is created;
    - a client of the consumer chain is created and the client ID is added to `chainToClient`.
  - Otherwise, the client is stored in `pendingClient` as a pending client.
- **Error Condition**
  - None.

> **Note:** Creating a client of a remote chain requires a `ClientState` and a `ConsensusState` (as defined in [ICS 7](../../core/ics-002-client-semantics)).
> `ConsensusState` requires setting a validator set of the remote chain. 
> The provider chain uses the fact that the validator set of the consumer chain is the same as its own validator set. 
> The rest of information to create a `ClientState` it receives through a governance proposal.

<!-- omit in toc -->
#### **[CCV-PCF-COINIT.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
    // the channel handshake MUST be initiated by consumer chain
    abortTransactionUnless(FALSE)
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a `ChanOpenInit` message on a port the provider CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The transaction is always aborted; hence, the state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-COTRY.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
    // validate parameters:
    // - only ordered channels allowed
    abortTransactionUnless(order == ORDERED)
    // - require the portIdentifier to be the port ID the CCV module is bound to
    abortTransactionUnless(portIdentifier == ProviderPortId)
    // - require the version to be the expected version
    abortTransactionUnless(version == "1")

    // assert that the counterpartyPortIdentifier matches 
    // the expected consumer port ID
    abortTransactionUnless(counterpartyPortIdentifier == ConsumerPortId)

    // assert that the counterpartyVersion matches the local version
    abortTransactionUnless(counterpartyVersion == version)
    
    // get the client state associated with this client ID in order 
    // to get access to the consumer chain ID
    clientId = getClient(channelIdentifier)
    clientState = clientKeeper.GetClientState(clientId)
    
    // require the CCV channel to be built on top 
    // of the expected client of the consumer chain
    abortTransactionUnless(chainToClient[clientState.chainId] == clientId)

    // require that no other CCV channel exists for this consumer chain
    abortTransactionUnless(clientState.chainId NOTIN chainToChannel.Keys())
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a `ChanOpenTry` message on a port the provider CCV module is bounded to.
- **Precondition** 
  - True.
- **Postcondition**
  - The transaction is aborted if any of the following conditions are true:
    - the channel is not ordered;
    - `portIdentifier != ProviderPortId`;
    - `version` is not the expected version;
    - `counterpartyPortIdentifier != ConsumerPortId`;
    - `counterpartyVersion != version`;
    - the channel is not built on top of the client created for this consumer chain;
    - another CCV channel for this consumer chain already exists.
  - The state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-COACK.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string) {
    // the channel handshake MUST be initiated by consumer chain
    abortTransactionUnless(FALSE)
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a `ChanOpenAck` message on a port the provider CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The transaction is always aborted; hence, the state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-COCONFIRM.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // get the client state associated with this client ID in order 
    // to get access to the consumer chain ID
    clientId = getClient(channelIdentifier)
    clientState = clientKeeper.GetClientState(clientId)

    // Verify that there isn't already a CCV channel for the consumer chain
    // If there is, then close the channel.
    if clientState.chainId IN chainToChannel {
      channelKeeper.ChanCloseInit(channelIdentifier)
      abortTransactionUnless(FALSE)
    }

    // set channel mappings
    chainToChannel[clientState.chainId] = channelIdentifier
    channelToChain[channelIdentifier] = clientState.chainId
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a `ChanOpenConfirm` message on a port the provider CCV module is bounded to.
- **Precondition** 
  - True.
- **Postcondition**
  - If a CCV channel for this consumer chain already exists, then 
    - the channel closing handshake is initiated for the underlying channel;
    - the transaction is aborted.
  - Otherwise, the channel mappings are set, i.e., `chainToChannel` and `channelToChain`.
- **Error Condition**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-INITG.1]**
```typescript
// CCF: Consumer Chain Function
// implements the AppModule interface
function InitGenesis(gs: ConsumerGenesisState): [ValidatorUpdate] {
  // ValidateGenesis
  // - contains a valid providerClientState  
  abortSystemUnless(gs.providerClientState != nil AND gs.providerClientState.Valid())
  // - contains a valid providerConsensusState
  abortSystemUnless(gs.providerConsensusState != nil AND gs.providerConsensusState.Valid())
  // - contains a non-empty initial validator set
  abortSystemUnless(gs.initialValSet NOT empty)
  // - contains an initial validator set that matches 
  //   the validator set in the providerConsensusState (see ICS 7)
  abortSystemUnless(gs.initialValSet == gs.providerConsensusState.validatorSet)

  // bind to ConsumerPortId port 
  err = portKeeper.BindPort(ConsumerPortId)
  // check whether the capability for the port can be claimed
  abortSystemUnless(err == nil)

  // create client of the provider chain 
  clientId = clientKeeper.CreateClient(gs.providerClientState, gs.providerConsensusState)

  // store the ID of the client of the provider chain
  providerClient = clientId

  // set default value for HtoVSC
  HtoVSC[getCurrentHeight()] = 0

  // set the initial validator set for the consumer chain
  foreach val IN gs.initialValSet {
    ccVal := CrossChainValidator{
      address: hash(val.pubKey),
      power: val.power
    }
    validatorSet[ccVal.address] = ccVal
  }

  return gs.initialValSet
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - An `InitChain` message is received from the consensus engine; the `InitChain` message is sent when the consumer chain is first started. 
- **Precondition**
  - The consumer CCV module is in the initial state.  
- **Postcondition**
  - The capability for the port `ConsumerPortId` is claimed.
  - A client of the provider chain is created and the client ID is stored into `providerClient`.
  - `HtoVSC` for the current block is set to `0`.
  - The `validatorSet` mapping is populated with the initial validator set.
  - The initial validator set is returned to the consensus engine.
- **Error Condition**
  - The genesis state contains no valid provider client state, where the validity is defined as in [ICS 7](../../client/ics-007-tendermint-client).
  - The genesis state contains no valid provider consensus state, where the validity is defined as in [ICS 7](../../client/ics-007-tendermint-client).
  - The genesis state contains an empty initial validator set.
  - The genesis state contains an initial validator set that does not match the validator set in the provider consensus state.
  - The capability for the port `ConsumerPortId` cannot be claimed.

> **Note**: CCV assumes that the _same_ consumer chain genesis state is disseminated to all the correct validators in the initial validator set of the consumer chain. 
> Although the mechanism of disseminating the genesis state is outside the scope of this specification, a possible approach would entail the following steps:
> - the process `P` creating a governance proposal `Prop` to spawn the new consumer chain creates the genesis state `S` of the entire consumer ABCI application without the genesis state of the consumer CCV module, i.e., without `ConsumerGenesisState`; 
> - `P` adds a hash of `S` to the proposal `Prop` (see `CreateConsumerChainProposal`);
> - `P` disseminates `S` via the gossip network;
> - when handling `Prop`, the provider chain creates and store in its state the `ConsumerGenesisState` using the information that the validator set of the consumer chain matches the validator set of the provider chain;
> - finally, each validator in the initial validator set of the consumer chain obtains the remainder of the genesis state (i.e., `ConsumerGenesisState`) by querying the provider chain.

<!-- omit in toc -->
#### **[CCV-CCF-COINIT.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
    // ensure provider channel hasn't already been created
    abortTransactionUnless(providerChannel == "")

    // validate parameters:
    // - only ordered channels allowed
    abortTransactionUnless(order == ORDERED)
    // - require the portIdentifier to be the port ID the CCV module is bound to
    abortTransactionUnless(portIdentifier == ConsumerPortId)
    // - require the version to be the expected version
    abortTransactionUnless(version == "1")

    // assert that the counterpartyPortIdentifier matches 
    // the expected consumer port ID
    abortTransactionUnless(counterpartyPortIdentifier == ProviderPortId)
   
    // require that the client ID of the client associated 
    // with this channel matches the expected provider client id
    clientId = getClient(channelIdentifier)   
    abortTransactionUnless(providerClient != clientId)
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanOpenInit` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The transaction is aborted if any of the following conditions are true:
    - `providerChannel` is already set;
    - `portIdentifier != ConsumerPortId`;
    - `version` is not the expected version;
    - `counterpartyPortIdentifier != ProviderPortId`;
    - the client associated with this channel is not the expected provider client.
  - The state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-COTRY.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
    // the channel handshake MUST be initiated by consumer chain
    abortTransactionUnless(FALSE)
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanOpenTry` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The transaction is always aborted; hence, the state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-COACK.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string) {
    // ensure provider channel hasn't already been created
    abortTransactionUnless(providerChannel == "")

    // assert that the counterpartyVersion matches the local version
    abortTransactionUnless(counterpartyVersion == version)
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanOpenAck` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition**
  - The transaction is aborted if any of the following conditions are true:
    - `providerChannel` is already set;
    - `counterpartyVersion != version`.
  - The state is not changed.
- **Error Condition**
  - None.

> **Note:** The initialization sub-protocol on the consumer chain finalizes on receiving the first `VSCPacket` and setting `providerChannel` to the ID of the channel on which it receives the packet (see `onRecvVSCPacket` method).

<!-- omit in toc -->
#### **[CCV-CCF-COCONFIRM.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // the channel handshake MUST be initiated by consumer chain
    abortTransactionUnless(FALSE)
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanOpenConfirm` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The transaction is always aborted; hence, the state is not changed.
- **Error Condition**
  - None.

### Channel Closing Handshake
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-PCF-CCINIT.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // Disallow user-initiated channel closing
    abortTransactionUnless(FALSE)
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a `ChanCloseInit` message on a port the provider CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The transaction is always aborted; hence, the state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-CCCONFIRM.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // do nothing
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a `ChanCloseConfirm` message on a port the provider CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The state is not changed.
- **Error Condition**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-CCINIT.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // allow relayers to close duplicate OPEN channels, 
    // if the provider channel has already been established
    if providerChannel == "" || providerChannel == channelIdentifier {
      // user cannot close channel
      abortTransactionUnless(FALSE)
    }
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanCloseInit` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - If `providerChannel` is not set or `providerChannel` matches the ID of the channel the `ChanCloseInit` message was received on, then the transaction is aborted. 
  - The state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-CCCONFIRM.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // do nothing
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanCloseConfirm` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition** 
  - The state is not changed.
- **Error Condition**
  - None.

### Packet Relay
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-PCF-RCVP.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onRecvPacket(packet: Packet): bytes {
  switch typeof(packet.data) {
    case VSCMaturedPacketData:
      return onRecvVSCMaturedPacket(packet)
    case SlashPacketData:
      return onRecvSlashPacket(packet)
    default:
      // unexpected packet type
      return PacketError
  }    
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a packet on a channel owned by the provider CCV module.
- **Precondition**
  - True.
- **Postcondition** 
  - If the packet is a `VSCMaturedPacket`, the acknowledgement obtained from invoking the `onRecvVSCMaturedPacket` method is returned.
  - If the packet is a `SlashPacket`, the acknowledgement obtained from invoking the `onRecvSlashPacket` method is returned.
  - Otherwise, an error acknowledgement is returned.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-ACKP.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onAcknowledgePacket(packet: Packet, ack: bytes) {
  switch typeof(packet.data) {
    case VSCPacketData:
      onAcknowledgeVSCPacket(packet, ack)
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE)
  }
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives an acknowledgement on a channel owned by the provider CCV module.
- **Precondition**
  - True.
- **Postcondition** 
  - If the acknowledgement is for a `VSCPacket`, the `onAcknowledgeVSCPacket` method is invoked.
  - Otherwise, the transaction is aborted.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-TOP.1]**
```typescript
// PCF: Provider Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onTimeoutPacket(packet Packet) {
  switch typeof(packet.data) {
    case VSCPacketData:
      onTimeoutVSCPacket(packet)
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE) 
  }
}
```
- **Caller**
  - The provider IBC routing module.
- **Trigger Event**
  - The provider IBC routing module receives a timeout on a channel owned by the provider CCV module.
- **Precondition**
  - The Correct Relayer assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition** 
  - If the timeout is for a `VSCPacket`, the `onTimeoutVSCPacket` method is invoked.
  - Otherwise, the transaction is aborted.
- **Error Condition**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-RCVP.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onRecvPacket(packet: Packet): bytes {
  switch typeof(packet.data) {
    case VSCPacketData:
      return onRecvVSCPacket(packet)
    default:
      // unexpected packet type
      return PacketError
  }
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a packet on a channel owned by the consumer CCV module.
- **Precondition**
  - True.
- **Postcondition** 
  - If the packet is a `VSCPacket`, the acknowledgement obtained from invoking the `onRecvVSCPacket` method is returned.
  - Otherwise, an error acknowledgement is returned.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-ACKP.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onAcknowledgePacket(packet: Packet, ack: bytes) {
  switch typeof(packet.data) {
    case VSCMaturedPacketData:
      onAcknowledgeVSCMaturedPacket(packet, ack)
    case SlashPacketData:
      onAcknowledgeSlashPacket(packet, ack)
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE)
  }
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives an acknowledgement on a channel owned by the consumer CCV module.
- **Precondition**
  - True.
- **Postcondition** 
  - If the acknowledgement is for a `VSCMaturedPacket`, the `onAcknowledgeVSCMaturedPacket` method is invoked.
  - If the acknowledgement is for a `SlashPacket`, the `onAcknowledgeSlashPacket` method is invoked.
  - Otherwise, the transaction is aborted.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-TOP.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ModuleCallbacks interface defined in ICS26
function onTimeoutPacket(packet Packet) {
  switch typeof(packet.data) {
    case VSCMaturedPacketData:
      onTimeoutVSCMaturedPacket(packet)
    case SlashPacketData:
      onTimeoutSlashPacket(packet)
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE) 
  }
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a timeout on a channel owned by the consumer CCV module.
- **Precondition**
  - The Correct Relayer assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition** 
  - If the timeout is for a `VSCMaturedPacket`, the `onTimeoutVSCMaturedPacket` method is invoked.
  - If the timeout is for a `SlashPacket`, the `onTimeoutSlashPacket` method is invoked.
  - Otherwise, the transaction is aborted.
- **Error Condition**
  - None.

### Validator Set Update
[&uparrow; Back to Outline](#outline)

The *validator set update* sub-protocol enables the provider chain 
- to update the consumer chain on the voting power granted to validators on the provider chain
- and to ensure the correct completion of unbonding operations for validators that produce blocks on the consumer chain.

<!-- omit in toc -->
#### **[CCV-PCF-BBLOCK.1]**
```typescript
// CCF: Provider Chain Function
// implements the AppModule interface
function BeginBlock() {
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-EBLOCK.1]**
```typescript
// PCF: Provider Chain Function
// implements the AppModule interface
function EndBlock(): [ValidatorUpdate] {
  // get list of validator updates from the provider Staking module
  valUpdates = stakingKeeper.GetValidatorUpdates()

  // iterate over all consumer chains registered with this provider chain
  foreach chainId in chainToClient.Keys() {
    // check whether there are changes in the validator set;
    // note that this also entails unbonding operations 
    // w/o changes in the voting power of the validators in the validator set
    if len(valUpdates) != 0 OR len(vscToUnbondingOps[(chainId, vscId)]) != 0 {
      // create VSCPacket data
      data = VSCPacketData{
        id: vscId, 
        updates: valUpdates,
        slashAcks: slashRequests[chainId]
      }
      slashRequests.Remove(chainId)

      // add VSCPacket data to the list of pending VSCPackets 
      pendingVSCPackets.Append(chainId, data)
    }

    // check whether there is an established CCV channel to the consumer chain
    if chainId IN chainToChannel.Keys() {
      // set initH for this consumer chain (if not done already)
      if chainId NOT IN initH.Keys() {
        initH[chainId] = getCurrentHeight()
      }

      // get the channel ID for the given consumer chain ID
      channelId = chainToChannel[chainId]

      foreach data IN pendingVSCPackets[chainId] {
        // create packet and send it using the interface exposed by ICS-4
        packet = Packet{data: data, destChannel: channelId}
        channelKeeper.SendPacket(packet)
      }

      // remove pending VSCPackets
      pendingVSCPackets.Remove(chainId)
    }
  }
  // set VSCtoH mapping
  VSCtoH[vscId] = getCurrentHeight() + 1
  // increment VSC ID
  vscId++ 

  // do not return anything to the consensus engine
  return []   
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - A list of validator updates `valUpdates` is obtained from the provider Staking module.
  - For every consumer chain with `chainId`
    - If either `valUpdates` is not empty or there were unbonding operations initiated during this block, then 
      - a `VSCPacket` data `data` is created such that `data.id = vscId`, `data.updates = valUpdates`, and `data.slashAcks = slashRequests[chainId]`;
      - `slashRequests[chainId]` is emptied;
      - `packetData` is appended to the list of pending `VSCPacket`s associated to `chainId`, i.e., `pendingVSCPackets[chainId]`.
    - If there is an established CCV channel for the the consumer chain with `chainId`, then
      - if `initH[chainId]` is not already set, then `initH[chainId]` is set to the current height;
      - for each `VSCPacketData` in the list of pending VSCPackets associated to `chainId`
        - a packet with the `VSCPacketData` is sent on the channel associated with the consumer chain with `chainId`;
      - all the pending VSCPackets associated to `chainId` are removed.
  - `vscId` is mapped to the height of the subsequent block. 
  - `vscId` is incremented.
- **Error Condition**
  - None.

> **Note**: The provider CCV module expects the provider Staking module to update its view of the validator set before the `EndBlock()` of the provider CCV module is invoked. 
> A solution is for the provider Staking module to update its view during `EndBlock()` and then, the `EndBlock()` of the provider Staking module to be executed before the `EndBlock()` of the provider CCV module.

<!-- omit in toc -->
#### **[CCV-PCF-ACKVSC.1]**
```typescript
// PCF: Provider Chain Function
function onAcknowledgeVSCPacket(packet: Packet, ack: bytes) {
  // providing the VSC with id packet.data.id can fail, 
  // i.e., ack == VSCPacketError, 
  // only if the VSCPacket was sent on a channel 
  // other than the established CCV channel;
  // that should never happen, see EndBlock()
  abortSystemUnless(ack != VSCPacketError)
}
```
- **Caller**
  - The `onAcknowledgePacket()` method.
- **Trigger Event**
  - The provider IBC routing module receives an acknowledgement of a `VSCPacket` on a channel owned by the provider CCV module.
- **Precondition**
  - True.
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - The acknowledgement is `VSCPacketError`.

<!-- omit in toc -->
#### **[CCV-PCF-TOVSC.1]**
```typescript
// PCF: Provider Chain Function
function onTimeoutVSCPacket(packet Packet) {
  // cleanup state
  abortTransactionUnless(packet.getDestinationChannel() IN channelToChain.Keys())
  chainId = channelToChain[packet.getDestinationChannel()]
  channelToChain.Remove(packet.getDestinationChannel())
  chainToChannel.Remove(chainId)

  // TODO: cleanup, e.g., complete all outstanding unbonding ops
  // see https://github.com/cosmos/ibc/issues/669
}
```
- **Caller**
  - The `onTimeoutPacket()` method.
- **Trigger Event**
  - The provider IBC routing module receives a timeout of a `VSCPacket` on a channel owned by the provider CCV module.
- **Precondition**
  - The Correct Relayer assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition**
  - The transaction is aborted if the ID of the channel on which the packet was sent is not mapped to a chain ID (in `channelToChain`).
  - The chain ID mapped to `packet.getDestinationChannel()` in `channelToChain` is removed.
  - The channel ID mapped to `chainId` in `chainToChannel` is removed, where `chainId = channelToChain[packet.getDestinationChannel()]`.
- **Error Condition**
  - None

<!-- omit in toc -->
#### **[CCV-PCF-RCVMAT.1]**
```typescript
// PCF: Provider Chain Function
function onRecvVSCMaturedPacket(packet: Packet): bytes {
  // get the ID of the consumer chain mapped to this channel ID
  abortTransactionUnless(packet.getDestinationChannel() IN channelToChain.Keys())
  chainId = channelToChain[packet.getDestinationChannel()]

  // iterate over the unbonding operations mapped to
  // this chainId and vscId (i.e., packet.data.id)
  foreach op in GetUnbondingOpsFromVSC(chainId, packet.data.id) {
    // remove the consumer chain from 
    // the list of consumer chain that are still unbonding
    op.unbondingChainIds.Remove(chainId)
    // if the unbonding operation has unbonded on all consumer chains
    if op.unbondingChainIds.IsEmpty() {
      // attempt to complete unbonding in Staking module
      stakingKeeper.CompleteStoppedUnbonding(op.id)
      // remove unbonding operation
      unbondingOps.Remove(op.id)
    }
  }
  // clean up vscToUnbondingOps mapping
  vscToUnbondingOps.Remove((chainId, vscId))

  return VSCMaturedPacketSuccess
}
```
- **Caller**
  - The `onRecvPacket()` method.
- **Trigger Event**
  - The provider IBC routing module receives a `VSCMaturedPacket` on a channel owned by the provider CCV module.
- **Precondition**
  - True.
- **Postcondition**
  - The transaction is aborted if the channel on which the packet was received is not an established CCV channel (i.e., not in `channelToChain`).
  - `chainId` is set to the ID of the consumer chain mapped to the channel on which the packet was received. 
  - For each unbonding operation `op` returned by `GetUnbondingOpsFromVSC(chainId, packet.data.id)`
    - `chainId` is removed from `op.unbondingChainIds`;
    - if `op.unbondingChainIds` is empty,
      - the `CompleteStoppedUnbonding()` method of the Staking module is invoked;
      - the entry `op` is removed from `unbondingOps`.
  - `(chainId, vscId)` is removed from `vscToUnbondingOps`.
  - A successful acknowledgment is returned.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-GETUBOPS.1]**
```typescript
// PCF: Provider Chain Function
// Utility method
function GetUnbondingOpsFromVSC(
  chainId: Identifier, 
  _vscId: uint64): [UnbondingOperation] {
    // get all unbonding operations associated with (chainId, _vscId)
    ops = []
    foreach id in vscToUnbondingOps[(chainId, _vscId)] {
      // get the unbonding operation with this ID
      op = unbondingOps[id]
      // append the operation to the list of operations to be returned
      ops.Append(op)
    }
    return ops
}
```
- **Caller**
  - The `onRecvVSCMaturedPacket()` method.
- **Trigger Event**
  - The provider IBC routing module receives a `VSCMaturedPacket` on a channel owned by the provider CCV module.
- **Precondition**
  - The provider CCV module received a `VSCMaturedPacket` `P` from a consumer chain with ID `chainId`, such that `P.data.id == _vscId`.
- **Postcondition**
  - Return the list of unbonding operations mapped to `(chainId, _vscId)`. 
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-HOOK-AFUBOPCR.1]**
```typescript
// PCF: Provider Chain Function
// implements a Staking module hook
function AfterUnbondingOpInitiated(opId: uint64) {
  // get the IDs of all consumer chains registered with this provider chain
  chainIds = chainToClient.Keys()
  // create and store a new unbonding operation
  unbondingOps[opId] = UnbondingOperation{
    id: opId,
    unbondingChainIds: chainIds
  }
  // add the unbonding operation id to vscToUnbondingOps
  foreach chainId in chainIds {
    vscToUnbondingOps[(chainId, vscId)].Append(opId)
  }
}
```
- **Caller**
  - The Staking module.
- **Trigger Event**
  - An unbonding operation with id `opId` is initiated.
- **Precondition**
  - True.
- **Postcondition**
  - An `UnbondingOperation` `op` is created and added to `unbondingOps`, such that `op.id = opId` and `op.unbondingChainIds` is the list of all consumer chains registered with this provider chain, i.e., `chainToClient.Keys()`.
  - The ID of the created unbonding operation is appended to every list in `vscToUnbondingOps[(chainId, vscId)]`, where `chainId` is an ID of a consumer chains registered with this provider chain and `vscId` is the current VSC ID. 
- **Error Condition**
  - None.


<!-- omit in toc -->
#### **[CCV-PCF-HOOK-BFUBOPCO.1]**
```typescript
// PCF: Provider Chain Function
// implements a Staking module hook
function BeforeUnbondingOpCompleted(opId: uint64): Bool {
  if opId in unbondingOps.Keys() {
    // the unbonding operation is still unbonding 
    // on at least one consumer chain
    return true
  }
  return false
}
```
- **Caller**
  - The Staking module.
- **Trigger Event**
  - An unbonding operation with ID `opId` has matured on the provider chain.
- **Precondition**
  - True.
- **Postcondition**
  - If there is an unboding operation with ID `opId` in `unbondingOps`, then true is returned.
  - Otherwise, false is returned.
- **Error Condition**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-BBLOCK.1]**
```typescript
// CCF: Consumer Chain Function
// implements the AppModule interface
function BeginBlock() {
  HtoVSC[getCurrentHeight() + 1] = HtoVSC[getCurrentHeight()]
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - `HtoVSC` for the subsequent block height is set to the same VSC ID as the current block height.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-RCVVSC.1]**
```typescript
// CCF: Consumer Chain Function
function onRecvVSCPacket(packet: Packet): bytes {
  // check whether the packet was sent on the CCV channel
  if providerChannel != "" && providerChannel != packet.getDestinationChannel() {
    // packet sent on a channel other than the established provider channel;
    // close channel and return error acknowledgement
    channelKeeper.ChanCloseInit(packet.getDestinationChannel())
    return VSCPacketError
  }

  // set HtoVSC mapping
  HtoVSC[getCurrentHeight() + 1] = packet.data.id
  
  // check whether the CCV channel is established
  if providerChannel == "" {
    // set the channel as the provider channel
    providerChannel = packet.getDestinationChannel()
    // send pending slash requests
    SendPendingSlashRequests()
  }

  // store the list of updates from the packet
  pendingChanges.Append(packet.data.updates)

  // calculate and store the maturity timestamp for the VSC
  maturityTimestamp = currentTimestamp().Add(UnbondingPeriod)
  maturingVSCs.Add(packet.data.id, maturityTimestamp)

  // reset outstandingDowntime for validators in packet.data.slashAcks
  foreach valAddr IN packet.data.slashAcks {
    outstandingDowntime[valAddr] = FALSE
  }

  return VSCPacketSuccess
}
```
- **Caller**
  - The `onRecvPacket()` method.
- **Trigger Event**
  - The consumer IBC routing module receives a `VSCPacket` on a channel owned by the consumer CCV module.
- **Precondition**
  - True.
- **Postcondition**
  - If `providerChannel` is set and does not match the channel (with ID `packet.getDestinationChannel()`) on which the packet was received, then 
    - the closing handshake for the channel with ID `packet.getDestinationChannel()` is initiated;
    - an error acknowledgement is returned.
  - Otherwise,
    - the height of the subsequent block is mapped to `packet.data.id` (i.e., the `HtoVSC` mapping) ;  
    - if `providerChannel` is not set, then 
      - the CCV channel is marked as established, i.e., `providerChannel = packet.getDestinationChannel()`;
      - the pending slash requests are sent to the provider chain (see [CCV-CCF-SNDPESLASH.1](#ccv-ccf-sndpeslash1));
    - `packet.data.updates` are appended to `pendingChanges`;
    - `(packet.data.id, maturityTimestamp)` is added to `maturingVSCs`, where `maturityTimestamp = currentTimestamp() + UnbondingPeriod`;
    - for each `valAddr` in the slash acknowledgments received from the provider chain, `outstandingDowntime[valAddr]` is set to false;
    - a successful acknowledgement is returned.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-ACKMAT.1]**
```typescript
// CCF: Consumer Chain Function
function onAcknowledgeVSCMaturedPacket(packet: Packet, ack: bytes) {
  // notifications of VSC maturity cannot fail by construction
  abortSystemUnless(ack != VSCMaturedPacketError)
}
```
- **Caller**
  - The `onAcknowledgePacket()` method.
- **Trigger Event**
  - The consumer IBC routing module receives an acknowledgement of a `VSCMaturedPacket` on a channel owned by the consumer CCV module.
- **Precondition**
  - True.
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - The acknowledgement is `VSCMaturedPacketError`.

<!-- omit in toc -->
#### **[CCV-CCF-TOMAT.1]**
```typescript
// CCF: Consumer Chain Function
function onTimeoutVSCMaturedPacket(packet Packet) {
  // TODO What do we do here? 
  // Do we need to notify the provider to close the channel?
  // What happens w/ the consumer chain once the CCV channel gets closed?
  // see https://github.com/cosmos/ibc/issues/669
}
```
- **Caller**
  - The `onTimeoutPacket()` method.
- **Trigger Event**
  - The consumer IBC routing module receives a timeout of a `VSCPacket` on a channel owned by the consumer CCV module.
- **Precondition**
  - The Correct Relayer assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - None

<!-- omit in toc -->
#### **[CCV-CCF-EBLOCK.1]**
```typescript
// CCF: Consumer Chain Function
// implements the AppModule interface
function EndBlock(): [ValidatorUpdate] {
  if pendingChanges.IsEmpty() {
    // do nothing
    return []
  }
  // aggregate the pending changes
  changes = pendingChanges.Aggregate()
  // Note: in the implementation, the aggregation is done directly 
  // when receiving a VSCPacket via the AccumulateChanges method.

  // remove all pending changes
  pendingChanges.RemoveAll()

  // update validatorSet
  UpdateValidatorSet(changes)

  // unbond mature packets if the CCV channel is established
  if providerChannel != "" {
    UnbondMaturePackets()
  }

  // return the validator set updates to the consensus engine
  return changes
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - If `pendingChanges` is empty, the state is not changed.
  - Otherwise,
    - the pending changes are aggregated and stored in `changes`;
    - `pendingChanges` is emptied;
    - `UpdateValidatorSet(changes)` is invoked;
    - if `providerChannel != ""`, `UnbondMaturePackets()` is invoked;
    - `changes` is returned to the consensus engine.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-UPVALS.1]**
```typescript
// CCF: Consumer Chain Function
function UpdateValidatorSet(changes: [ValidatorUpdate]) {
  foreach update IN changes {
    addr := hash(update.pubKey)
    if addr NOT IN validatorSet.Keys() {
      // new validator bonded
      abortSystemUnless(update.power > 0)
      // add new CrossChainValidator to validator set
      val := CrossChainValidator{
        address: addr,
        power: update.power
      }
      validatorSet[addr] = val
      // call AfterCCValidatorBonded hook
      AfterCCValidatorBonded(addr)
    }
    else if update.power == 0 {
      // existing validator begins unbonding
      validatorSet.Remove(addr)
      // call AfterCCValidatorBeginUnbonding hook
      AfterCCValidatorBeginUnbonding(addr)
    }
    else {
      validatorSet[addr].power = update.power
    }
  }
}
```
- **Caller**
  - The `EndBlock()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine.
- **Precondition**
  - `changes` contains the aggregated validator updates from `pendingChanges` before it was emptied. 
- **Postcondition**
  - For each validator `update` in `changes`,
    - if the validator is not in the validator set, then 
      - a new `CrossChainValidator` is added to `validatorSet`;
      - the `AfterCCValidatorBonded` hook is called;
    - otherwise, if the validator's new power is `0`, then,
      - the validator is removed from `validatorSet`;
      - the `AfterCCValidatorBeginUnbonding` hook is called;
    - otherwise, the validator's power is updated.
- **Error Condition**
  - Receiving a validator `update`, such that the validator is not in the validator set and `update.power = 0`.

<!-- omit in toc -->
#### **[CCV-CCF-UMP.1]**
```typescript
// CCF: Consumer Chain Function
function UnbondMaturePackets() {
  foreach (id, ts) in maturingVSCs.SortedByMaturityTime() {
    if currentTimestamp() < ts {
      break // stop loop
    }
    // create VSCMaturedPacketData
    packetData = VSCMaturedPacketData{id: id}

    // create packet and send it using the interface exposed by ICS-4
    packet = Packet{data: packetData, destChannel: providerChannel}
    channelKeeper.SendPacket(packet)
          
    // remove entry from the list
    maturingVSCs.Remove(id, ts)
  }
}
```
- **Caller**
  - The `EndBlock()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine.
- **Precondition**
  - The CCV channel to the provider chain is established, i.e., `providerChannel != ""`.
- **Postcondition**
  - For each `(id, ts)` in the list of maturing VSCs sorted by maturity timestamps
    - if `currentTimestamp() < ts`, the loop is stopped;
    - a `VSCMaturedPacketData` packet data is created;
    - a packet with the created `VSCMaturedPacketData` is sent to the provider chain;
    - the tuple `(id, ts)` is removed from `maturingVSCs`.
- **Error Condition**
  - None.

### Consumer Initiated Slashing
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-PCF-RCVSLASH.1]**
```typescript
// PCF: Provider Chain Function
function onRecvSlashPacket(packet: Packet): bytes {
  // check whether the packet was received on an established CCV channel
  if packet.getDestinationChannel() NOT IN channelToChain.Keys() {
    // packet received on a non-established channel; incorrect behavior
    channelKeeper.ChanCloseInit(packet.getDestinationChannel())
    return SlashPacketError
  }

  // get the height that maps to the VSC ID in the packet data
  if packet.data.vscId == 0 {
    // the infraction happened before sending any VSC to this chain
    chainId = channelToChain[packet.getDestinationChannel()]
    infractionHeight = initH[chainId]
  }
  else {
    infractionHeight = VSCtoH[packet.data.vscId]
  }

  // request the Slashing module to slash the validator
  slashingKeeper.Slash(
    packet.data.valAddress, 
    infractionHeight, 
    packet.data.valPower, 
    packet.data.slashFactor))

  // request the Slashing module to jail the validator
  timestamp = currentTimestamp() + data.jailTime
  slashingKeeper.JailUntil(packet.data.valAddress, timestamp)

  // add validator to list of slash requests for chainId
  slashRequests[chainId].Append(packet.data.valAddress)

  return SlashPacketSuccess
}
```
- **Caller**
  - The `onRecvPacket()` method.
- **Trigger Event**
  - The provider IBC routing module receives a `SlashPacket` on a channel owned by the provider CCV module.
- **Precondition**
  - True.
- **Postcondition**
  - If the channel the packet was received on is not an established CCV channel, then
    - the channel closing handshake is initiated;
    - an error acknowledgment is returned.
  - Otherwise,
    - if `packet.data.vscId == 0`, `infractionHeight` is set to `initH[chainId]`, with `chainId = channelToChain[packet.getDestinationChannel()]`, i.e., the height when the first VSC was provided to this consumer chain;
    - otherwise, `infractionHeight` is set to `VSCtoH[packet.data.vscId]`, i.e., the height at which the voting power was last updated by the validator updates in the VSC with ID `packet.data.vscId`;
    - a request is made to the Slashing module to slash the validator with address `packet.data.valAddress` for misbehaving at height `infractionHeight`;
    - a request is made to the Slashing module to jail the validator with address `packet.data.valAddress` for a period `data.jailTime`;
    - the validator's address `packet.data.valAddress` is added to the list of slash requests from this `chainId`;
    - a successful acknowledgment is returned.
- **Error Condition**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-ACKSLASH.1]**
```typescript
// CCF: Consumer Chain Function
function onAcknowledgeSlashPacket(packet: Packet, ack: bytes) {
  // slash request fail, i.e., ack == SlashPacketError, 
  // only if the SlashPacket was sent on a channel 
  // other than the established CCV channel;
  // that should never happen,
  // see SendSlashRequest() and SendPendingSlashRequests()
  abortSystemUnless(ack != SlashPacketError)
}
```
- **Caller**
  - The `onAcknowledgePacket()` method.
- **Trigger Event**
  - The consumer IBC routing module receives an acknowledgement of a `SlashPacket` on a channel owned by the consumer CCV module.
- **Precondition**
  - True.
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - The acknowledgement is `SlashPacketError`.

<!-- omit in toc -->
#### **[CCV-CCF-TOSLASH.1]**
```typescript
// CCF: Consumer Chain Function
function onTimeoutSlashPacket(packet Packet) {
  // TODO What do we do here? 
  // Do we need to notify the provider to close the channel?
  // What happens w/ the consumer chain once the CCV channel gets closed?
  // see https://github.com/cosmos/ibc/issues/669
}
```
- **Caller**
  - The `onTimeoutPacket()` method.
- **Trigger Event**
  - The consumer IBC routing module receives a timeout of a `SlashPacket` on a channel owned by the consumer CCV module.
- **Precondition**
  - The Correct Relayer assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - None

<!-- omit in toc -->
#### **[CCV-CCF-SNDSLASH.1]**
```typescript
// CCF: Consumer Chain Function
// Enables consumer initiated slashing
function SendSlashRequest(
  valAddress: string, 
  power: int64, 
  infractionHeight: Height,
  downtime: Bool) {
    if downtime AND outstandingDowntime[data.valAddress] {
      // do not send multiple requests for the same downtime
      return
    }

    // TODO governance and CCV params
    // see https://github.com/cosmos/ibc/issues/673
    slashFactor = TBA
    jailTime = TBA

    // create SlashPacket data
    packetData = SlashPacketData{
      valAddress: valAddress,
      valPower: power,
      vscId: HtoVSC[infractionHeight],
      slashFactor: slashFactor,
      jailTime: jailTime
    }

    // check whether the CCV channel to the provider chain is established
    if providerChannel != "" {
      // create packet and send it using the interface exposed by ICS-4
      packet = Packet{data: packetData, destChannel: providerChannel}
      channelKeeper.SendPacket(packet)

      if downtime {
        // set outstandingDowntime for this validator
        outstandingDowntime[data.valAddress] = TRUE
      }
    }
    else {
      // add SlashPacket data to the list of pending SlashPackets 
      req := SlashRequest{data: packetData, downtime: downtime}
      pendingSlashRequests.Append(req)
    }
}
```
- **Caller**
  - The ABCI application (e.g., the Slashing module).
- **Trigger Event**
  - Evidence of misbehavior for a validator with address `valAddress` was received.
- **Precondition**
  - True.
- **Postcondition**
  - If the request is for downtime and there is an outstanding request to slash this validator for downtime, then the state is not changed.
  - Otherwise, 
    - both `slashFactor` and `jailTime` parameters are set;
    - a `SlashPacket` data `packetData` is created, such that `packetData.vscId = VSCtoH[infractionHeight]`;
    - if the CCV channel to the provider chain is established, then 
      - a packet with the `packetData` is sent to the provider chain;
      - if the request is for downtime, `outstandingDowntime[data.valAddress]` is set to true;
    - otherwise `SlashRequest{data: packetData, downtime: downtime}` is appended to `pendingSlashRequests`.
- **Error Condition**
  - None.

> **Note**: The ABCI application MUST subtract `ValidatorUpdateDelay` from the infraction height before invoking `SendSlashRequest`, 
> where `ValidatorUpdateDelay` is a delay (in blocks) between when validator updates are returned to the consensus-engine and when they are applied 
> (for more details, take a look at the [ABCI specification](https://github.com/tendermint/spec/blob/v0.7.1/spec/abci/abci.md#endblock)). 
> For example, if `ValidatorUpdateDelay = x` and a validator set update is returned with new validators at the end of block `10`, 
> then the new validators are expected to sign blocks beginning at block `11+x`. 
> 
> Consequently, the consumer CCV module expects the `infractionHeight` parameter of the `SendSlashRequest()` to be set accordingly.

> **Note**: In the context of single-chain validation, slashing for downtime is an **_atomic operation_**, i.e., once the downtime is detected, the misbehaving validator is slashed and jailed immediately. 
> Consequently, once a validator is punished for downtime, it is removed from the validator set and cannot be punished again for downtime. 
> Since validators are not automatically added back to the validator set, it entails that the validator is aware of the punishment before it can rejoin and be potentially punished again.
> 
> In the context of CCV, slashing for downtime is no longer atomic, i.e., downtime is detected on the consumer chain, but the jailing happens on the provider chain. 
> To avoid sending multiple slash requests for the same downtime infraction, the consumer CCV module uses an `outstandingDowntime` flag per validator. 
> CCV assumes that the consumer ABCI application (e.g., the slashing module) is not including the downtime of a validator with `outstandingDowntime == TRUE` in the evidence for downtime.

<!-- omit in toc -->
#### **[CCV-CCF-SNDPESLASH.1]**
```typescript
// CCF: Consumer Chain Function
// Utility method
function SendPendingSlashRequests() {
  // iterate over every pending SlashRequest in reverse order
  foreach req IN pendingSlashRequests.Reverse() {
    if !req.downtime OR !outstandingDowntime[req.data.valAddress] {
      // create packet and send it using the interface exposed by ICS-4
      packet = Packet{data: req.data, destChannel: providerChannel}
      channelKeeper.SendPacket(packet)

      if req.downtime {
        // set outstandingDowntime for this validator
        outstandingDowntime[req.data.valAddress] = TRUE
      }
    }
  }
  // remove pending SlashRequest
  pendingSlashRequests.RemoveAll()
}
```
- **Caller**
  - The `onRecvVSCPacket()` method (see [CCV-CCF-RCVVSC.1](#ccv-ccf-rcvvsc1)).
- **Trigger Event**
  - The first `VSCPacket` is received from the provider chain.
- **Precondition**
  - `providerChannel != ""`.
- **Postcondition**
  - For each slash request `req` in `pendingSlashRequests` in reverse order, such that either the slash request is not for downtime or there is no outstanding slash request for downtime,
    - a packet with the data `req.data` is sent to the provider chain;
    - if the request is for downtime, `outstandingDowntime[req.data.valAddress]` is set to true.
  - All the pending `SlashRequest`s are removed.
- **Error Condition**
  - None.

> **Note**: Iterating over pending `SlashRequest`s in reverse order ensures that validators that are down for multiple blocks during channel initialization will be slashed for the latest downtime evidence.  
