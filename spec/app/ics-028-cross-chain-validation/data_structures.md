<!-- omit in toc -->
# CCV: Technical Specification - Data Structures
[&uparrow; Back to main document](./README.md)

[&uparrow; Back to technical specification](./technical_specification.md)

<!-- omit in toc -->
## Outline
- [External Data Structures](#external-data-structures)
- [CCV Data Structures](#ccv-data-structures)
- [CCV Packets](#ccv-packets)
- [CCV State](#ccv-state)
  - [State on Provider Chain](#state-on-provider-chain)
  - [State on Consumer Chain](#state-on-consumer-chain)

## External Data Structures
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

The data structures required for creating clients (i.e., `ClientState`, `ConsensusState`) are defined in [ICS 2](../../core/ics-002-client-semantics). 
In the context of CCV, every chain is uniquely defined by their chain ID and the validator set. 
Thus, CCV requires the `ClientState` to contain the chain ID and the `ConsensusState` for a particular height to contain the validator set at that height. 
In addition, the `ClientState` should contain the `UnbondingPeriod`.
For an example, take a look at the `ClientState` and `ConsensusState` defined in [ICS 7](../../client/ics-007-tendermint-client).

## CCV Data Structures
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
    preCCV: Bool
    unbondingPeriod: Duration
    connId: Identifier
    providerClientState: ClientState
    providerConsensusState: ConsensusState
    counterpartyClientId: Identifier
    initialValSet: [ValidatorUpdate]
    transferChannelId: Identifier
  }
  ```
  - `preCCV` is a flag indicating whether the consumer CCV module starts in pre-CCV state. 
    In pre-CCV state the consumer CCV module MUST NOT pass validator updates to the underlying consensus engine.
    If `preCCV == true`, then `connId` must be set.
  - `unbondingPeriod` is the unbonding period on the consumer chain.
  - `connId` is the ID of the connection end on the consumer chain on top of which the CCV channel will be established.
    If `connId == ""`, a new client of the provider chain and a new connection on top of this client are created.
  - `providerClientState` is the client state used to create a new client of the provider chain (as defined in [ICS 2](../../core/ics-002-client-semantics)).
    If `connId != ""`, then `providerClientState` is ignored.
  - `providerConsensusState` is the consensus state used to create a new client of the provider chain (as defined in [ICS 2](../../core/ics-002-client-semantics)).
    If `connId != ""`, then `providerConsensusState` is ignored.
  - `counterpartyClientId` is the ID of the client of the consumer chain on the provider chain. 
    Note that `counterpartyClientId` is only needed to allow the consumer CCV module to initiate the connection opening handshake.
    If `connId != ""`, then `counterpartyClientId` is ignored.
  - `initialValSet` is the first validator set that will start validating on this consumer chain.
  - `transferChannelId` is the ID of a token transfer channel (as defined in [ICS 20](../../app/ics-020-fungible-token-transfer)) used for the Reward Distribution sub-protocol. 
    If `transferChannelId == ""`, a new token transfer channel is created on top of the same connection as the CCV channel.

The provider CCV module handles governance proposals to add new consumer chains and to remove existing consumer chains. 
While the structure of governance proposals is specific to every ABCI application (for an example, see the `Proposal` interface in the [Governance module documentation](https://docs.cosmos.network/v0.45/modules/gov/) of Cosmos SDK),
this specification expects the following fields to be part of the proposals to add new consumer chains (i.e., `ConsumerAdditionProposal`) and to remove existing ones (i.e., `ConsumerRemovalProposal`):
  ```typescript
  interface ConsumerAdditionProposal {
    chainId: string
    spawnTime: Timestamp
    connId: Identifier
    unbondingPeriod: Duration
    transferChannelId: Identifier
    lockUnbondingOnTimeout: Bool
  }
  ```
  - `chainId` is the proposed chain ID of the new consumer chain. It must be different from all other consumer chain IDs of the executing provider chain.
  - `spawnTime` is the time on the provider chain at which the consumer chain genesis is finalized and all validators are responsible to start their consumer chain validator node.
  - `connId` is the ID of the connection end on the provider chain on top of which the CCV channel will be established.
    If `connId == ""`, a new client of the consumer chain and a new connection on top of this client are created.
    Note that a sovereign chain can transition to a consumer chain while maintaining existing IBC channels to other chains by providing a valid `connId`. 
  - `unbondingPeriod` is the unbonding period on the consumer chain.
  - `transferChannelId` is the ID of a token transfer channel (as defined in [ICS 20](../../app/ics-020-fungible-token-transfer)) used for the Reward Distribution sub-protocol. 
    If `transferChannelId == ""`, a new token transfer channel is created on top of the same connection as the CCV channel. 
    Note that `transferChannelId` is the ID of the channel end on the consumer chain.
  - `lockUnbondingOnTimeout` is a boolean value that indicates whether the funds corresponding to the outstanding unbonding operations are to be released in case of a timeout. 
    If `lockUnbondingOnTimeout == true`, a governance proposal to stop the timed out consumer chain would be necessary to release the locked funds. 
  ```typescript
  interface ConsumerRemovalProposal {
    chainId: string
    stopTime: Timestamp
  }
  ```
  - `chainId` is the chain ID of the consumer chain to be removed. It must be the ID of an existing consumer chain of the executing provider chain.
  - `stopTime` is the time on the provider chain at which all validators are responsible to stop their consumer chain validator node.

During the CCV channel opening handshake, the provider chain adds the address of its distribution module account to the channel version as metadata (as described in [ICS 4](../../core/ics-004-channel-and-packet-semantics/README.md#definitions)). 
The metadata structure is described by the following interface:
```typescript
interface CCVHandshakeMetadata {
  providerDistributionAccount: string // the account's address
  version: string
}
```
This specification assumes that the provider CCV module has access to the address of the distribution module account through the `GetDistributionAccountAddress()` method. For an example, take a look at the [auth module](https://docs.cosmos.network/v0.45/modules/auth/) of Cosmos SDK. 

During the CCV channel opening handshake, the provider chain adds the address of its distribution module account to the channel version as metadata (as described in [ICS 4](../../core/ics-004-channel-and-packet-semantics/README.md#definitions)). 
The metadata structure is described by the following interface:
```typescript
interface CCVHandshakeMetadata {
  providerDistributionAccount: string // the account's address
  version: string
}
```
This specification assumes that the provider CCV module has access to the address of the distribution module account through the `GetDistributionAccountAddress()` method. For an example, take a look at the [auth module](https://docs.cosmos.network/v0.45/modules/auth/) of Cosmos SDK. 

## CCV Packets
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
      // downtime slash requests acknowledgements, 
      // i.e., list of validator addresses
      downtimeSlashAcks: [string]
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
      downtime: Bool
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

## CCV State
[&uparrow; Back to Outline](#outline)

This section describes the internal state of the CCV module. For simplicity, the state is described by a set of variables; for each variable, both the type and a brief description is provided. In practice, all the state (except for hardcoded constants, e.g., `ProviderPortId`) is stored in a key/value store (KVS). The host state machine provides a KVS interface with three functions, i.e., `get()`, `set()`, and `delete()` (as defined in [ICS 24](../../core/ics-024-host-requirements)).

- `ccvVersion = "ccv-1"` is the CCV expected version. Both the provider and the consumer chains need to agree on this version.
- `zeroTimeoutHeight = {0,0}` is the `timeoutHeight` (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)) used by CCV for sending packets. Note that CCV uses `ccvTimeoutTimestamp` for sending CCV packets and `transferTimeoutTimestamp` for transferring tokens. 
- `ccvTimeoutTimestamp: uint64` is the `timeoutTimestamp` (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)) for sending CCV packets. The CCV protocol is responsible of setting `ccvTimeoutTimestamp` such that the *Correct Relayer* assumption is feasible.
- `transferTimeoutTimestamp: uint64` is the `timeoutTimestamp` (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)) for transferring tokens. 

### State on Provider Chain
[&uparrow; Back to Outline](#outline)

- `ProviderPortId = "provider"` is the port ID the provider CCV module is expected to bind to.
- `initTimeout: uint64` is the maximum time duration the Channel Initialization subprotocol may execute, 
  i.e., for any consumer chain, if the CCV channel is not established within `initTimeout` since the consumer chain was registered, then the consumer chain is removed. 
- `vscTimeout: uint64` is the maximum time duration between sending any `VSCPacket` to any consumer chain and receiving the corresponding `VSCMaturedPacket`, without timing out the consumer chain and consequently removing it.
- `pendingConsumerAdditionProposals: [ConsumerAdditionProposal]` is a list of pending governance proposals to add new consumer chains. 
- `pendingConsumerRemovalProposals: [ConsumerRemovalProposal]` is a list of pending governance proposals to remove existing consumer chains. 
  Both lists of pending governance proposals expose the following interface: 
```typescript
  interface [Proposal] {
    // append a proposal to the list; the list is modified
    Append(p: Proposal) 

    // remove a proposal from the list; the list is modified
    Remove(p: Proposal)
  }
  ```
- `lockUnbondingOnTimeout: Map<string, Bool>` is a mapping from consumer chain IDs to the boolean values indicating whether the funds corresponding to the in progress unbonding operations are to be released in case of a timeout.
- `chainToClient: Map<string, Identifier>` is a mapping from consumer chain IDs to the associated client IDs.
- `chainToConnection: Map<string, Identifier>` is a mapping from consumer chain IDs to the associated connection IDs.
- `chainToChannel: Map<string, Identifier>` is a mapping from consumer chain IDs to the CCV channel IDs.
- `channelToChain: Map<Identifier, string>` is a mapping from CCV channel IDs to consumer chain IDs.
- `initTimeoutTimestamps: Map<string, uint64>` is a mapping from consumer chain IDs to init timeout timestamps, see `initTimeout`.
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
- `vscSendTimestamps: Map<(string, uint64), uint64>` is a mapping from `(chainId, vscId)` tuples to the timestamps of sending `VSCPacket`s.
- `initialHeights: Map<string, Height>` is a mapping from consumer chain IDs to the heights on the provider chain. 
  For every consumer chain, the mapping stores the height when the CCV channel to that consumer chain is established. 
  Note that the provider validator set at this height matches the validator set at the height when the first VSC is provided to that consumer chain.
  It enables the mapping from consumer heights to provider heights.
- `VSCtoH: Map<uint64, Height>` is a mapping from VSC IDs to heights on the provider chain. It enables the mapping from consumer heights to provider heights, 
  i.e., the voting power at height `VSCtoH[id]` on the provider chain was last updated by the validator updates contained in the VSC with ID `id`.  
- `unbondingOps: Map<uint64, UnbondingOperation>` is a mapping that enables accessing for every unbonding operation the list of consumer chains that are still unbonding. When unbonding operations are initiated, the Staking module calls the `AfterUnbondingInitiated()` [hook](#ccv-pcf-hook-afubopcr1); this leads to the creation of a new `UnbondingOperation`, which is defined as
  ```typescript
  interface UnbondingOperation {
    id: uint64
    // list of consumer chain IDs that are still unbonding
    unbondingChainIds: [string] 
  }
  ```
- `vscToUnbondingOps: Map<(string, uint64), [uint64]>` is a mapping from `(chainId, vscId)` tuples to a list of unbonding operation IDs. 
  It enables the provider CCV module to match a `VSCMaturedPacket{vscId}`, received from a consumer chain with `chainId`, with the corresponding unbonding operations. 
  As a result, `chainId` can be removed from the list of consumer chains that are still unbonding these operations. 
  For more details see how received `VSCMaturedPacket`s [are handled](#ccv-pcf-rcvmat1).
- `maturedUnbondingOps: [uint64]` is a list of IDs of matured unbonding operations (from the perspective of the consumer chains), for which notifications can be sent to the Staking module (see `stakingKeeper.UnbondingCanComplete`). 
  Note that `maturedUnbondingOps` is emptied at the end of each block.
- `downtimeSlashRequests: Map<string, [string]>` is a mapping from `chainId`s to lists of validator addresses, 
  i.e., `downtimeSlashRequests[chainId]` contains all the validator addresses for which the provider chain received slash requests for downtime from the consumer chain with `chainId`.

### State on Consumer Chain
[&uparrow; Back to Outline](#outline)

- `ConsumerPortId = "consumer"` is the port ID the consumer CCV module is expected to bind to.
- `ConsumerUnbondingPeriod: Duration` is the unbonding period on the consumer chain.
- `preCCV: Bool` is a flag indicating whether the consumer CCV module starts in pre-CCV state. 
  In pre-CCV state, the consumer CCV module MUST NOT pass validator updates to the underlying consensus engine.
- `providerClientId: Identifier` identifies the client of the provider chain (on the consumer chain) that the CCV channel is build upon.
- `providerChannel: Identifier` identifies the consumer's channel end of the CCV channel.
- `ccvValidatorSet: <string, ValidatorUpdate>` is a mapping that stores the validators in the validator set of the consumer chain.
- `receivedVSCs: [VSCPacketData]` is a list of data items (i.e., `VSCPacketData`) received in `VSCPacket`s that are not yet applied. 
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
  `outstandingDowntime[valAddr]` is set to false once the consumer chain receives a confirmation that the downtime slash request was received by the provider chain, i.e., a `VSCPacket` that contains `valAddr` in `downtimeSlashAcks`. 
  The mapping enables the consumer CCV module to avoid sending to the provider chain multiple slashing requests for the same downtime infraction.
- `providerDistributionAccount: string` is the address of the distribution module account on the provider chain. It enables the consumer chain to transfer rewards to the provider chain.
- `distributionChannelId: Identifier` is the ID of the distribution token transfer channel used for sending rewards to the provider chain.
- `BlocksPerDistributionTransfer: int64` is the interval (in number of blocks) between two distribution token transfers. 
- `lastDistributionTransferHeight: Height` is the block height of the last distribution token transfer.
- `ccvAccount: string` is the address of the CCV module account where a fraction of the consumer chain rewards are collected before being transferred to the provider chain. 