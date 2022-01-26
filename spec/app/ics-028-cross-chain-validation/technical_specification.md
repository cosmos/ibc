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

## Placing CCV within an ABCI Application
[&uparrow; Back to Outline](#outline)

Before describing the data structures and sub-protocols of the CCV protocol, we provide a short overview of the interfaces the CCV module implements and the interactions with the other ABCI application modules.

<!-- omit in toc -->
### Implemented Interfaces

- CCV is an **ABCI application module**, which means it MUST implement the logic to handle some of the messages received from the consensus engine via ABCI, e.g., `InitChain`, `EndBlock` 
  (for more details, take a look at the [ABCI documentation](https://docs.tendermint.com/v0.34/spec/abci/abci.html)). 
  The following methods handle messages that are of particular interest to the CCV protocol:
  - `InitGenesis()` -- Called when the chain is first started, on receiving an `InitChain` message from the consensus engine. 
  - `EndBlock()` -- Contains logic that is automatically triggered at the end of each block. 
    This is also where the module can inform the underlying consensus engine of changes in the validator set.

> TODO: `ExportGenesis()` and handling hard forks. 

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
  For an example of how governance proposals work, take a look at the [Governance module documentation](https://docs.cosmos.network/master/modules/gov/) of Cosmos SDK. 

- For the [Validator Set Update sub-protocol](#validator-set-update), the provider CCV module interacts with a Staking module on the provider chain. 
  For an example of how staking works, take a look at the [Staking module documentation](https://docs.cosmos.network/master/modules/staking/) of Cosmos SDK. 
  The interaction is defined by the following interface:
  ```typescript 
  interface StakingKeeper {
    // get UnbondingPeriod from the provider Staking module 
    UnbondingTime(): Duration

    // get validator updates from the provider Staking module
    GetValidatorUpdates(chainID: string): [ValidatorUpdate]


    // notify the Staking module of unboding operations that
    // have matured from the consumer chain's perspective 
    CompleteStoppedUnbonding(id: uint64)
  }
  ```

- In addition, the following hooks enable the provider CCV module to register operations to be execute when certain events occur within the Staking module:
  ```typescript
  // invoked by the Staking module after 
  // initiating an unbonding operation
  function AfterUnbondingOpInitiated(opId: uint64);

  // invoked by the Staking module before 
  // completing an unbonding operation
  function BeforeUnbondingOpCompleted(opId: uint64): Bool;
  ```

## Data Structures

### External Data Structures
[&uparrow; Back to Outline](#outline)

This section describes external data structures used by the CCV module.

The CCV module uses the ABCI `ValidatorUpdate` data structure, which consists of a validator address (i.e., the hash of its public key) and its power, i.e.,
```typescript
interface ValidatorUpdate {
  address: string
  power: int64
}
```
The provider chain sends to the consumer chain a list of `ValidatorUpdate`s, containing an entry for every validator that had its power updated. 

The data structures required for creating clients (i.e., `ClientState`, `ConsensusState`) are defined in [ICS 2](../../core/ics-002-client-semantics). 
Specifically for Tendermint clients, the data structures are defined in [ICS 7](../../client/ics-007-tendermint-client).

### CCV Data Structures
[&uparrow; Back to Outline](#outline)

The CCV channel state is indicated by `ChannelStatus`, which is defined as 
```typescript
enum ChannelStatus {
  UNINITIALIZED   // default state
  INITIALIZING    // the channel is in handshake process
  VALIDATING      // the channel is open and validating
  INVALID         // the channel is invalid and can no longer process packets
}
```

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
    status: ChannelStatus
  }
  ```
- On the consumer chain, the genesis state is described by the following interface:
  ```typescript
  interface ConsumerGenesisState {
    providerClientState: ClientState
    providerConsensusState: ConsensusState
  }
  ```

The provider CCV module handles governance proposals to spawn new consumer chains. The structure of these proposals is defined by the `Proposal` interface in the [Governance module documentation](https://docs.cosmos.network/master/modules/gov/). The content of these proposals is described by the following interface (we omit typical fields such as title and description):
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

    // the hash of the genesis state for the consumer chain
    genesisHash: [byte]
  }
  ```
  Note that `Height` is defined in [ICS 7](../../client/ics-007-tendermint-client).

### CCV Packets
[&uparrow; Back to Outline](#outline)

The structure of the packets sent through the CCV channel is defined by the `Packet` interface in [ICS 4](../../core/ics-004-channel-and-packet-semantics). Packets are acknowledged by the remote side by sending back an `Acknowledgement` that contains either a result, created with `NewResultAcknowledgement()`, or an error, created with `NewErrorAcknowledgement()`. 

The following packet data types are required by the CCV module:
- `VSCPacketData` contains a list of validator updates, i.e., 
    ```typescript
    interface VSCPacketData {
      id: uint64 // the id of this VSC
      updates: [ValidatorUpdate]
    }
    ```
> Note that for brevity we use e.g., `VSCPacket` to refer to a packet with `VSCPacketData` as its data.

### CCV State
[&uparrow; Back to Outline](#outline)

This section describes the internal state of the CCV module. For simplicity, the state is described by a set of variables; for each variable, both the type and a brief description is provided. In practice, all the state (except for hardcoded constants, e.g., `ProviderPortId`) is stored in a key/value store (KVS). The host state machine provides a KVS interface with three functions, i.e., `get()`, `set()`, and `delete()` (as defined in [ICS 24](../../core/ics-024-host-requirements)).

- `[ValidatorUpdate]` is a list of `ValidatorUpdate`s. It exposes the following interface:
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

<!-- omit in toc -->
#### State on the provider chain

- `ProviderPortId = "provider"` is the port ID the provider CCV module is expected to bind to.
- `pendingClient: Map<(Timestamp, string), Height>` is a mapping from `(timestamp, chainId)` tuples to the initial height of pending clients, i.e., belonging to consumer chains that were not yet spawned, but for which a `CreateConsumerChainProposal` was received.
- `chainToClient: Map<string, Identifier>` is a mapping from consumer chain IDs to the associated client IDs.
- `chainToChannel: Map<string, Identifier>` is a mapping from consumer chain IDs to the CCV channel IDs.
- `channelToChain: Map<Identifier, string>` is a mapping from CCV channel IDs to consumer chain IDs.
- `channelStatus: Map<Identifier, ChannelStatus>` is a mapping from CCV channel IDs to CCV channel state, as indicated by `ChannelStatus`.
- `pendingUpdates: Map<string, [ValidatorUpdate]>` is a mapping from consumer chain IDs to a list of pending `ValidatorUpdate`s that must be sent to the consumer chain once the CCV channel is established.
- `vscId: uint64` is a monotonic strictly increasing ID that is used to uniquely identify the VSCs sent to the consumer chains.
- `unbondingOps: Map<uint64, UnbondingOperation>` is a mapping that enables accessing for every unbonding operation the list of consumer chains that are still unbonding. When unbonding operations are initiated, the Staking module calls the `AfterUnbondingOpInitiated()` [hook](#ccv-pcf-shook-afubopcr1); this leads to the creation of a new `UnbondingOperation`, which is defined as
  ```typescript
  interface UnbondingOperation {
    id: uint64
    // list of consumer chain IDs that are still unbonding
    unbondingChainIds: [string] 
  }
  ```
- `vscToUnbondingOps: Map<(Identifier, uint64), [uint64]>` is a mapping from `(chainId, vscId)` tuples to a list of unbonding operation IDs. It enables the provider CCV module to match an acknowledgement of a `VSCPacket`, received from a consumer chain with `chainId`, with the corresponding unbonding operations. As a result, `chainId` can be removed from the list of consumer chains that are still unbonding these operations. For more details see how `VSCPacket` [acknowledgements are handled](#ccv-pcf-ackvsc1).

<!-- omit in toc -->
#### State on the consumer chain
- `ConsumerPortId = "consumer"` is the port ID the consumer CCV module is expected to bind to.
- `providerClient: Identifier` identifies the client of the provider chain (on the consumer chain) that the CCV channel is build upon.
- `providerChannel: Identifier` identifies the consumer's channel end of the CCV channel.
- `channelStatus: ChannelStatus` is the status of the CCV channel.
- `pendingChanges: [ValidatorUpdate]` is a list of `ValidatorUpdate`s received, but not yet applied to the validator set. It is emptied on every `EndBlock()`. 
- `unbondingPackets: [(Packet, Time)]>` is a list of `(packet, unbondingTime)` tuples, where `packet` is a received `VSCPacket` and `unbondingTime` is the packet's unbonding time. The list is used to keep track of when unbonding operations are matured on the consumer chain. It exposes the following interface:
  ```typescript
  interface [(Packet, Time)]> {
    // add a packet with its unbonding time to the list;
    // the list is modified
    Add(packet: Packet, unbondingTime: Time)

    // return the list sorted by the unbonding time;
    // the original list is not modified
    SortedByUnbondingTime(): [(Packet, Time)]>

    // remove (packet, unbondingTime) from the list;
    // the list is modified
    Remove(packet: Packet, unbondingTime: Time)
  }
  ```
 
## Sub-protocols

> TODO: What about `EmitEvent()`? There are some events emitted throughout the code, but none of them are specified. 

To express the error conditions, the following specification of the sub-protocols uses the exception system of the host state machine, which is exposed through two functions (as defined in [ICS 24](../../core/ics-024-host-requirements)): `abortTransactionUnless` and `abortSystemUnless`.

### Initialization
[&uparrow; Back to Outline](#outline)

The *initialization* sub-protocol enables a provider chain and a consumer chain to create a CCV channel -- a unique, ordered IBC channel for exchanging packets. As a prerequisite, the initialization sub-protocol MUST create two IBC clients, one on the provider chain to the consumer chain and one on the consumer chain to the provider chain. This is necessary to verify the identity of the two chains (as long as the clients are trusted). 

<!-- omit in toc -->
#### **[CCV-PCF-INITG.1]**
```typescript
// PCF: Provider Chain Function
// implements the AppModule interface
function InitGenesis(state: ProviderGenesisState) {
  // bind to ProviderPortId port 
  err = portKeeper.BindPort(ProviderPortId)
  // check whether the capability for the port can be claimed
  abortSystemUnless(err == nil)

  foreach cs in state.clientStates {
    chainToChannel[cs.chainId] = cs.channelId
    channelToChain[cs.channelId] = cc.chainId
    channelStatus[cs.channelId] = cc.status
  }
}
```
- Initiator:
  - ABCI.
- Expected precondition: 
  - An `InitChain` message is received from the consensus engine; the `InitChain` message is sent when the provider chain is first started. 
- Expected postcondition:
  - The capability for the port `ProviderPortId` is claimed.
  - For each consumer state in the `ProviderGenesisState`, the initial state is set, i.e., the following mappings `chainToChannel`, `channelToChain`, `channelStatus` are set.
- Error condition:
  - The capability for the port `ProviderPortId` cannot be claimed.

<!-- omit in toc -->
#### **[CCV-PCF-CCPROP.1]**
```typescript
// PCF: Provider Chain Function
// implements governance proposal Handler 
function CreateConsumerChainProposal(p: CreateConsumerChainProposal) {
  if currentTimestamp() > p.spawnTime {
    // get UnbondingPeriod from provider Staking module
    unbondingTime = stakingKeeper.UnbondingTime()

    // create client state as defined in ICS 2
    clientState = ClientState{
      chainId: p.chainId,
      trustLevel: DefaultTrustLevel, // 1/3
      trustingPeriod: unbondingTime/2,
      unbondingPeriod: unbondingTime,
      latestHeight: p.initialHeight,
    }

    // create consensus state as defined in ICS 2;
    // SentinelRoot is used as a stand-in root value for 
    // the consensus state set at the upgrade height
    consensusState = ConsensusState{
      timestamp: currentTimestamp(),
      root: SentinelRoot,
      nextValidatorsHash: BlockHeader().NextValidatorsHash,
    }

    // create consumer chain client and store it
    clientId = clientKeeper.CreateClient(clientState, consensusState)
    consumerClient[p.chainId] = clientId
  }
  else {
    // store the client as a pending client
    pendingClient[(p.spawnTime, p.chainId)] = p.initialHeight
  }
}
```
- Initiator: 
  - `EndBlock()` method of Governance module.
- Expected precondition: 
  - A governance proposal with `CreateConsumerChainProposal` as content has passed (i.e., it got the necessary votes). 
- Expected postcondition: 
  - If the spawn time has already passed,
    - `UnbondingPeriod` is retrieved from the provider Staking module;
    - a client state is created;
    - a consensus state is created;
    - a client of the consumer chain is created and the client ID is added to `consumerClient`.
  - Otherwise, the client is stored in `pendingClient` as a pending client.
- Error condition:
  - None.

> **Note:** Creating a client of a remote chain requires a `ClientState` and a `ConsensusState` (as defined in [ICS 2](../../core/ics-002-client-semantics)). For Tendermint clients, creating a `ConsensusState` requires setting a validator set of the remote chain (see [ICS 7](../../client/ics-007-tendermint-client/README.md#consensus-state)). The provider chain uses the fact that the validator set of the consumer chain is the same as its own validator set. The rest of information to create a `ClientState` it receives through a governance proposal.

<!-- omit in toc -->
#### **[CCV-PCF-COINIT.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
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
- Initiator: 
  - The IBC module on the provider chain.
- Expected precondition:
  - The IBC module on the provider chain received a `ChanOpenInit` message on a port the provider CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - Invoked on the provider chain.

<!-- omit in toc -->
#### **[CCV-PCF-COTRY.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
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

    // assert that the counterpartyVersion matches the local version
    abortTransactionUnless(counterpartyVersion == version)

    // set the CCV channel status to INITIALIZING
    channelStatus[channelIdentifier] = INITIALIZING
    
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
- Initiator: 
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a `ChanOpenTry` message on a port the provider CCV module is bounded to.
- Expected postcondition:
  - The status of the CCV channel with ID `channelIdentifier` is set to `INITIALIZING`.
- Error condition:
  - The channel is not ordered.
  - `portIdentifier != ProviderPortId`.
  - `version` is not the expected version.
  - `counterpartyVersion != version`.
  - The channel is not built on top of the client created for this consumer chain.
  - Another CCV channel for this consumer chain already exists.

<!-- omit in toc -->
#### **[CCV-PCF-COACK.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string) {
    // the channel handshake MUST be initiated by consumer chain
    abortTransactionUnless(FALSE)
}
```
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a `ChanOpenAck` message on a port the provider CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - Invoked on the provider chain.

<!-- omit in toc -->
#### **[CCV-PCF-COCONFIRM.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
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
      channelStatus[channelIdentifier] = INVALID
      channelKeeper.ChanCloseInit(channelIdentifier)
      abortTransactionUnless(FALSE)
    }

    // set channel mappings
    chainToChannel[clientState.chainId] = channelIdentifier
    channelToChain[channelIdentifier] = clientState.chainId

    // set CCV channel status to VALIDATING
    channelStatus[channelIdentifier] = VALIDATING
}
```
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a `ChanOpenConfirm` message on a port the provider CCV module is bounded to.
- Expected postcondition:
  - If a CCV channel for this consumer chain already exists, then the channel is invalidated and closed.
  - Otherwise, the channel mappings are set and the CCV channel status is set to `VALIDATING`.
- Error condition:
  - A CCV channel for this consumer chain already exists.

---

<!-- omit in toc -->
#### **[CCV-CCF-INITG.1]**
```typescript
// CCF: Consumer Chain Function
// implements the AppModule interface
function InitGenesis(state: ConsumerGenesisState) {
  // bind to ConsumerPortId port 
  err = portKeeper.BindPort(ConsumerPortId)
  // check whether the capability for the port can be claimed
  abortSystemUnless(err == nil)

  // create client of the provider chain
  clientId = clientKeeper.CreateClient(state.providerClientState, state.providerConsensusState)

  // store the ID of the client of the provider chain
  providerClient = clientId
}
```
- Initiator:
  - ABCI.
- Expected precondition: 
  - An `InitChain` message is received from the consensus engine; the `InitChain` message is sent when the consumer chain is first started. 
- Expected postcondition:
  - The capability for the port `ConsumerPortId` is claimed.
  - A client of the provider chain is created and the client ID is stored into `providerClient`.
- Error condition:
  - The capability for the port `ConsumerPortId` cannot be claimed.

> **Note**: CCV assumes that `ConsumerGenesisState` is disseminated to all validators. To ensure that validators receive the correct genesis state, the governance proposal also contains a hash of the `ConsumerGenesisState` (see `CreateConsumerChainProposal`). 

<!-- omit in toc -->
#### **[CCV-CCF-COINIT.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
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

    // set the CCV channel status to INITIALIZING
    channelStatus[channelIdentifier] = INITIALIZING
   
    // require that the client ID of the client associated 
    // with this channel matches the expected provider client id
    clientId = getClient(channelIdentifier)   
    abortTransactionUnless(providerClient != clientId)
}
```
- Initiator: 
  - The IBC module on the consumer chain.
- Expected precondition:
  - The IBC module on the consumer chain received a `ChanOpenInit` message on a port the consumer CCV module is bounded to.
- Expected postcondition: 
  - The status of the CCV channel with ID `channelIdentifier` is set to `INITIALIZING`.
- Error condition:
  - `providerChannel` is already set.
  - `portIdentifier != ConsumerPortId`.
  - `version` is not the expected version.
  - The client associated with this channel is not the expected provider client.

<!-- omit in toc -->
#### **[CCV-CCF-COTRY.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
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
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a `ChanOpenTry` message on a port the consumer CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - Invoked on the consumer chain.

<!-- omit in toc -->
#### **[CCV-CCF-COACK.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
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
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a `ChanOpenAck` message on a port the consumer CCV module is bounded to.
- Expected postcondition:
  - The state is not changed.
- Error condition:
  - `providerChannel` is already set.
  - `counterpartyVersion != version`.

> **Note:** The initialization sub-protocol on the consumer chain finalizes on receiving the first `VSCPacket` and setting `providerChannel` to the ID of the channel on which it receives the packet (see `onRecvVSCPacket` method).

<!-- omit in toc -->
#### **[CCV-CCF-COCONFIRM.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // the channel handshake MUST be initiated by consumer chain
    abortTransactionUnless(FALSE)
}
```
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a `ChanOpenConfirm` message on a port the consumer CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - Invoked on the consumer chain.

### Channel Closing Handshake
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-PCF-CCINIT.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // Disallow user-initiated channel closing for provider channels
    abortTransactionUnless(FALSE)
}
```
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a `ChanCloseInit` message on a port the provider CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - Invoked on the provider chain.

<!-- omit in toc -->
#### **[CCV-PCF-CCCONFIRM.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // do nothing
}
```
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a `ChanCloseConfirm` message on a port the provider CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-CCINIT.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
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
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a `ChanCloseInit` message on a port the consumer CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - `providerChannel` is not set or `providerChannel` matches the ID of the channel the `ChanCloseInit` message was received on.

<!-- omit in toc -->
#### **[CCV-CCF-CCCONFIRM.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // do nothing
}
```
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a `ChanCloseConfirm` message on a port the consumer CCV module is bounded to.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - None.

### Packet Relay
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-PCF-RCVP.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
function onRecvPacket(packet: Packet): Packet {
  switch typeof(packet.data) {
    // the provider chain receives no packets
    default:
      // unexpected packet type
      return NewErrorAcknowledgement()
  }    
}
```
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a packet on a channel owned by the provider CCV module.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - The packet type is unexpected.

<!-- omit in toc -->
#### **[CCV-PCF-ACKP.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
function onAcknowledgePacket(packet: Packet) {
  switch typeof(packet.data) {
    case VSCPacketData:
      onAcknowledgeVSCPacket(packet)
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE)
  }
}
```
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received an acknowledgement on a channel owned by the provider CCV module.
- Expected postcondition: 
  - If the acknowledgement is for a `VSCPacket`, the `onAcknowledgeVSCPacket` method is invoked.
- Error condition:
  - The acknowledgement is for an unexpected packet.

<!-- omit in toc -->
#### **[CCV-PCF-TOP.1]**
```typescript
// PCF: Provider Chain Function
// implements the ICS26 interface
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
- Initiator:
  - The IBC module on the provider chain.
- Expected precondition: 
  - The IBC module on the provider chain received a timeout on a channel owned by the provider CCV module.
  - The Correct Relayer assumption is violated.
- Expected postcondition: 
  - If the timeout is for a `VSCPacket`, the `onTimeoutVSCPacket` method is invoked.
- Error condition:
  - The timeout is for an unexpected packet.

---

<!-- omit in toc -->
#### **[CCV-CCF-RCVP.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
function onRecvPacket(packet: Packet): Packet {
  switch typeof(packet.data) {
    case VSCPacketData:
      return onRecvVSCPacket(packet)
    default:
      // unexpected packet type
      return NewErrorAcknowledgement()
  }
}
```
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a packet on a channel owned by the consumer CCV module.
- Expected postcondition: 
  - If the packet is a `VSCPacket`, the `onRecvVSCPacket` method is invoked.
- Error condition:
  - The packet type is unexpected.

<!-- omit in toc -->
#### **[CCV-CCF-ACKP.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
function onAcknowledgePacket(packet: Packet) {
  switch typeof(packet.data) {
    // the consumer chain sends no packets
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE)
  }
}
```
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received an acknowledgement on a channel owned by the consumer CCV module.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - The acknowledgement is for an unexpected packet.

<!-- omit in toc -->
#### **[CCV-CCF-TOP.1]**
```typescript
// CCF: Consumer Chain Function
// implements the ICS26 interface
function onTimeoutPacket(packet Packet) {
  switch typeof(packet.data) {
    // the consumer chain sends no packets
    default:
      // unexpected packet type
      abortTransactionUnless(FALSE) 
  }
}
```
- Initiator:
  - The IBC module on the consumer chain.
- Expected precondition: 
  - The IBC module on the consumer chain received a timeout on a channel owned by the consumer CCV module.
  - The Correct Relayer assumption is violated.
- Expected postcondition: 
  - The state is not changed.
- Error condition:
  - The timeout is for an unexpected packet.

### Validator Set Update
[&uparrow; Back to Outline](#outline)

The *validator set update* sub-protocol enables the provider chain 
- to update the consumer chain on the voting power granted to validators on the provider chain
- and to ensure the correct completion of unbonding operations for validators that produce blocks on the consumer chain.

<!-- omit in toc -->
#### **[CCV-PCF-EBLOCK.1]**
```typescript
// PCF: Provider Chain Function
// implements the AppModule interface
function EndBlock(): [ValidatorUpdate] {
  // iterate over all consumer chains registered with this provider chain
  foreach chainId in chainToClient.Keys() {
    // get list of validator updates from the provider Staking module
    valUpdates = stakingKeeper.GetValidatorUpdates(chainId)

    // add validator updates to the list of pending updates 
    pendingUpdates[chainId] = pendingUpdates[chainId].Aggregate(valUpdates)

    // TODO pendingUpdates should be a list of VSCs; 
    // once the channel is established, all the VSCs are sent, 
    // so that we can receive ACKs for the unbonding
    
    // check whether 
    //  - there are pending updates; 
    //  - and there is an established CCV channel to the consumer chain
    if len(pendingUpdates[chainId]) != 0 AND chainId IN chainToChannel.Keys() {
      // the CCV channel should be in VALIDATING state
      abortSystemUnless(channelStatus[chainId] == VALIDATING)

      // create packet data
      packetData = VSCPacketData{id: vscId, updates: pendingUpdates[chainId]}

      // gets the channel ID for the given consumer chain ID
      channelId = chainToChannel[chainId]

      // create packet and send it using the interface exposed by ICS-4
      packet = Packet{data: packetData, destChannel: channelId}
      channelKeeper.SendPacket(packet)
    }
  }
  // increment VSC ID
  vscId++ 

  // do not return anything to the consensus engine
  return []   
}
```
- Initiator: 
  - ABCI.
- Expected precondition:
  - An `EndBlock` message is received from the consensus engine. 
  - The provider Staking module has an up-to-date list of validator updates for every consumer chain registered.
- Expected postcondition:
  - For every consumer chain with `chainId`
    - A list of validator updates is retrieved from the provider Staking module and aggregated to the list of pending updates for the consumer chain with `chainId`, i.e., `pendingUpdates[chainId]`.
    - If the list of pending updates is not empty and there is a CCV channel with status `VALIDATING` for the the consumer chain with `chainId`, then
      - a `VSCPacketData` is created, with `id = vscId` and `updates = valUpdates`;
      - a packet with the created `VSCPacketData` is sent on the channel associated with the consumer chain with `chainId`.
  - `vscId` is incremented.
- Error condition:
  - A CCV channel for the consumer chain with `chainId` exists and its status is not set to `VALIDATING`.

> **Note**: The expected precondition implies that the provider Staking module MUST update its view of the validator sets for each consumer chain before `EndBlock()` in the provider CCV module is invoked. A solution is for the provider Staking module to update its view during `EndBlock()` and then, the `EndBlock()` of the provider Staking module MUST be executed before the `EndBlock()` of the provider CCV module.

<!-- omit in toc -->
#### **[CCV-PCF-ACKVSC.1]**
```typescript
// PCF: Provider Chain Function
function onAcknowledgeVSCPacket(packet: Packet) {
  // get the channel ID of the CCV channel the packet was sent on
  channelId = packet.getDestinationChannel()
  
  // get the ID of the consumer chain mapped to this channel ID
  abortTransactionUnless(channelId IN channelToChain.Keys())
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
}
```
- Initiator: 
  - The `onAcknowledgePacket()` method.
- Expected precondition:
  - The IBC module on the provider chain received an acknowledgement of a `VSCPacket` on a channel owned by the provider CCV module.
- Expected postcondition:
  - For each unbonding operation `op` returned by `GetUnbondingOpsFromVSC(chainId, packet.data.id)`
    - `chainId` is removed from `op.unbondingChainIds`;
    - if `op.unbondingChainIds` is empty,
      - the `CompleteStoppedUnbonding()` method of the Staking module is invoked;
      - the entry `op` is removed from `unbondingOps`.
  - `(chainId, vscId)` is removed from `vscToUnbondingOps`.
- Error condition:
  - The ID of the channel on which the `VSCPacket` was sent is not mapped to a chain ID (in `channelToChain`).

<!-- omit in toc -->
#### **[CCV-PCF-GETUBDES.1]**
```typescript
// PCF: Provider Chain Function
// Utility method
function GetUnbondingOpsFromVSC(
  chainId: Identifier, 
  _vscId: uint64): [UnbondingOperation] {
    ids = vscToUnbondingOps[(chainId, _vscId)]
    if ids == nil {
      // cannot find the list of unbonding operation IDs
      // for this chainId and _vscId
      return nil
    }
    // get all unbonding operations associated with
    // this chainId and _vscId
    ops = []
    foreach id in ids {
      // get the unbonding operation with this ID
      op = unbondingOps[id]
      // if cannot find UnbondingOperation according to vscToUnbondingOps,
      // then vscToUnbondingOps was probably not correctly updated;
      // programming error
      abortSystemUnless(op != nil)
      // append the operation to the list of operations to be returned
      ops.Append(op)
    }
    return ops
}
```
- **Initiator:** 
  - The provider CCV module when receiving an acknowledgement for a `VSCPacket`.
- **Expected precondition:**
  - None. 
- **Expected postcondition:**
  - If there is a list of unbonding operation IDs mapped to `(chainId, _vscId)`, then return the list of unbonding operations mapped to these IDs. 
  - Otherwise, return `nil`.
- **Error condition:**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-TOVSC.1]**
```typescript
// PCF: Provider Chain Function
function onTimeoutVSCPacket(packet Packet) {
  channelStatus = INVALID

  // TODO: Unbonding everything?
}
```
- Initiator: 
  - The `onTimeoutPacket()` method.
- Expected precondition:
  - The IBC module on the provider chain received a timeout of a `VSCPacket` on a channel owned by the provider CCV module.
- Expected postcondition:
  - `channelStatus` is set to `INVALID`
- Error condition:
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-SHOOK-AFUBOPCR.1]**
```typescript
// PCF: Provider Chain Function
// implements a Staking module hook
function AfterUnbondingOpInitiated(opId: uint64) {
  // get the IDs of all consumer chains registered with this provider chain
  chainIds = chainToChannel.Keys()
  
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
- **Initiator:** 
  - The Staking module.
- **Expected precondition:**
  - An unbonding operation with id `opId` is initiated.
- **Expected postcondition:**
  - An unbonding operations is created and added to `unbondingOps`.
  - The ID of the created unbonding operation is appended to every list in `vscToUnbondingOps[(chainId, vscId)]`, where `chainId` is an ID of a consumer chains registered with this provider chain and `vscId` is the current VSC ID. 
- **Error condition:**
  - None.


<!-- omit in toc -->
#### **[CCV-PCF-SHOOK-BFUBOPCO.1]**
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
- **Initiator:** 
  - The Staking module.
- **Expected precondition:**
  - An unbonding operation has matured on the provider chain.
- **Expected postcondition:**
  - If there is an unboding operation with ID `opId`, then true is returned.
  - Otherwise, false is returned.
- **Error condition:**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-RECVVSC.1]**
```typescript
// CCF: Consumer Chain Function
function onRecvVSCPacket(packet: Packet): Packet {
  channelId = packet.getDestinationChannel()
  // check whether the packet was sent on the CCV channel
  if providerChannel != "" && providerChannel != channelId {
    // packet sent on a channel other than the established provider channel;
    // close channel and return error acknowledgement
    channelKeeper.ChanCloseInit(channelId)
    return NewErrorAcknowledgement()
  }
  
  // check whether the status of the CCV channel is VALIDATING
  if (channelStatus != VALIDATING) {
    // set status to VALIDATING
    channelStatus = VALIDATING

    // set the channel as the provider channel
    providerChannel = channelId
  }

  // store the list of updates from the packet
  pendingChanges.Append(packet.data.updates)

  // calculate and store the unbonding time for the packet
  unbondingTime = currentTimestamp().Add(UnbondingPeriod)
  unbondingPackets.Add(packet, unbondingTime)

  // ack will be sent asynchronously
  return nil
}
```
- Initiator: 
  - The `onRecvPacket()` method.
- Expected precondition:
  - The IBC module on the consumer chain received a `VSCPacket` on a channel owned by the consumer CCV module.
- Expected postcondition:
  - If `providerChannel` is set and does not match the channel with ID `channelId` on which the packet was sent, then 
    - the closing handshake for the channel with ID `channelId` is initiated;
    - an error acknowledgement is returned.
  - Otherwise,  
    - if the CCV channel status is not `VALIDATING`, then it is set to `VALIDATING` and the channel ID is set as the provider channel;
    - `packet.data.updates` are appended to `pendingChanges`;
    - `(packet, unbondingTime)` is added to `unbondingPackets`, where `unbondingTime = currentTimestamp() + UnbondingPeriod`;
    - a nil acknowledgement is returned, i.e., the acknowledgement will be sent asynchronously.
- Error condition:
  - None.

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

  // unbond mature packets
  UnbondMaturePackets()

  // return the validator set updates to the consensus engine
  return changes
}
```
- Initiator: 
  - ABCI.
- Expected precondition:
  - An `EndBlock` message is received from the consensus engine. 
- Expected postcondition:
  - If `pendingChanges` is empty, the state is not changed.
  - Otherwise,
    - the pending changes are aggregated and returned to the consensus engine;
    - `pendingChanges` is emptied;
    - `UnbondMaturePackets()` is invoked.
- Error condition:
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-UMP.1]**
```typescript
// CCF: Consumer Chain Function
function UnbondMaturePackets() {
  // check if the provider channel is set
  if providerChannel != "" {
    foreach (packet, unbondingTime) in unbondingPackets.SortedByUnbondingTime() {
      if currentTimestamp() >= unbondingTime {
        // send acknowledgement to the provider chain
        channelKeeper.WriteAcknowledgement(providerChannel, packet, NewResultAcknowledgement())
              
        // remove entry from the list
        unbondingPackets.Remove(packet, unbondingTime)
      } 
      else {
        // stop loop
        break
      }
    }
  }
}
```
- Initiator: 
  - The `EndBlock()` method.
- Expected precondition:
  - None.
- Expected postcondition:
  - If the provider channel is set, for each `(packet, unbondingTime)` in the list of unbonding packet sorted by unbonding times
    - if `currentTimestamp() >= unbondingTime`, the packet is acknowledged (i.e., `channelKeeper.WriteAcknowledgement()` is invoked) and the tuple is removed from `unbondingPackets`;
    - otherwise, stop the loop.
  - Otherwise, the state is not changed.
- Error condition:
  - None.