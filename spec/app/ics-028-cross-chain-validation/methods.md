<!-- omit in toc -->
# CCV: Technical Specification - Methods
[&uparrow; Back to main document](./README.md)

[&uparrow; Back to technical specification](./technical_specification.md)

<!-- omit in toc -->
## Outline
- [General Methods](#general-methods)
  - [BeginBlock and EndBlock](#beginblock-and-endblock)
  - [Packet Relay](#packet-relay)
- [Sub-protocols](#sub-protocols)
  - [Initialization](#initialization)
  - [Consumer Chain Removal](#consumer-chain-removal)
  - [Validator Set Update](#validator-set-update)
  - [Consumer Initiated Slashing](#consumer-initiated-slashing)
  - [Reward Distribution](#reward-distribution)

## General Methods
[&uparrow; Back to Outline](#outline)

To express the error conditions, the following specification of the sub-protocols uses the exception system of the host state machine, which is exposed through two functions (as defined in [ICS 24](../../core/ics-024-host-requirements)): `abortTransactionUnless` and `abortSystemUnless`.

### BeginBlock and EndBlock
[&uparrow; Back to Outline](#outline)

The functions `BeginBlock()` and `EndBlock()` (see [Implemented Interfaces](./technical_specification.md#implemented-interfaces)) are split across the CCV sub-protocols.

<!-- omit in toc -->
#### **[CCV-PCF-BBLOCK.1]**
```typescript
// PCF: Provider Chain Function
// implements the AppModule interface
function BeginBlock() {
    BeginBlockInit()
    BeginBlockCCR()
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - `BeginBlockInit()` is invoked (see [[CCV-PCF-BBLOCK-INIT.1]](#ccv-pcf-bblock-init1), i.e., it contains the `BeginBlock()` logic needed for the Initialization sub-protocol).
  - `BeginBlockCCR()` is invoked (see [[CCV-PCF-BBLOCK-CCR.1]](#ccv-pcf-bblock-ccr1), i.e., it contains the `BeginBlock()` logic needed for the Consumer Chain Removal sub-protocol).
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-EBLOCK.1]**
```typescript
// PCF: Provider Chain Function
// implements the AppModule interface
function EndBlock(): [ValidatorUpdate] {
  EndBlockCIS()
  EndBlockVSU()

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
  - `EndBlockCIS()` is invoked (see [[CCV-PCF-EBLOCK-CIS.1]](#ccv-pcf-eblock-cis1), i.e., it contains the `EndBlock()` logic needed for the Consumer Initiated Slashing sub-protocol).
  - `EndBlockVSU()` is invoked (see [[CCV-PCF-EBLOCK-VSU.1]](#ccv-pcf-eblock-vsu1), i.e., it contains the `EndBlock()` logic needed for the Validator Set Update sub-protocol).
- **Error Condition**
  - None.

> **Note**: The provider CCV module expects the provider Staking module to update its view of the validator set before the `EndBlock()` of the provider CCV module is invoked. 
> A solution is for the provider Staking module to update its view during `EndBlock()` and then, the `EndBlock()` of the provider Staking module to be executed before the `EndBlock()` of the provider CCV module.

---

<!-- omit in toc -->
#### **[CCV-CCF-BBLOCK.1]**
```typescript
// CCF: Consumer Chain Function
// implements the AppModule interface
function BeginBlock() {
    BeginBlockCCR()
    BeginBlockCIS()
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - `BeginBlockCCR()` is invoked (see [[CCV-CCF-BBLOCK-CCR.1]](#ccv-ccf-bblock-ccr1), i.e., it contains the `BeginBlock()` logic needed for the Consumer Chain Removal sub-protocol).
  - `BeginBlockCIS()` is invoked (see [[CCV-CCF-BBLOCK-CIS.1]](#ccv-ccf-bblock-cis1), i.e., it contains the `BeginBlock()` logic needed for the Consumer Initiated Slashing sub-protocol).
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-EBLOCK.1]**
```typescript
// CCF: Consumer Chain Function
// implements the AppModule interface
function EndBlock(): [ValidatorUpdate] {
  EndBlockRD()

  // return the validator set updates to the consensus engine
  return EndBlockVSU()
}
```
- **Caller**
  - The ABCI application.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - `EndBlockRD()` is invoked (see [[CCV-PCF-EBLOCK-RD.1]](#ccv-pcf-eblock-rd1), i.e., it contains the `EndBlock()` logic needed for the Reward Distribution sub-protocol).
  - `EndBlockVSU()` is invoked and the return value is returned to the consensus engine (see [[CCV-CCF-EBLOCK-VSU.1]](#ccv-ccf-eblock-vsu1), i.e., it contains the `EndBlock()` logic needed for the Validator Set Update sub-protocol).
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
  - A packet sent on a channel owned by the provider CCV module timed out as a result of either
    - the timeout height or timeout timestamp passing on the consumer chain without the packet being received (see `timeoutPacket` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#sending-end));
    - or the channel being closed without the packet being received (see `timeoutOnClose` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#timing-out-on-close)). 
- **Precondition**
  - The *Correct Relayer* assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
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
  - A packet sent on a channel owned by the consumer CCV module timed out as a result of either
    - the timeout height or timeout timestamp passing on the provider chain without the packet being received (see `timeoutPacket` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#sending-end));
    - or the channel being closed without the packet being received (see `timeoutOnClose` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#timing-out-on-close)). 
- **Precondition**
  - The *Correct Relayer* assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition** 
  - If the timeout is for a `VSCMaturedPacket`, the `onTimeoutVSCMaturedPacket` method is invoked.
  - If the timeout is for a `SlashPacket`, the `onTimeoutSlashPacket` method is invoked.
  - Otherwise, the transaction is aborted.
- **Error Condition**
  - None.

## Sub-protocols

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
  err = portKeeper.bindPort(ProviderPortId)
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
#### **[CCV-PCF-SPCCPROP.1]**
```typescript
// PCF: Provider Chain Function
// implements governance proposal Handler 
function SpawnConsumerChainProposalHandler(p: SpawnConsumerChainProposal) {
  if currentTimestamp() > p.spawnTime {
    CreateConsumerClient(p)
  }
  else {
    // store the proposal as a pending spawn proposal
    pendingSpawnProposals.Append(p)
  }
}
```
- **Caller**
  - `EndBlock()` method of Governance module.
- **Trigger Event**
  - A governance proposal `SpawnConsumerChainProposal` has passed (i.e., it got the necessary votes).
- **Precondition** 
  - True. 
- **Postcondition** 
  - If the spawn time has already passed, `CreateConsumerClient(p)` is invoked, with `p` the `SpawnConsumerChainProposal`. 
  - Otherwise, the proposal is appended to the list of pending spawn proposals, i.e., `pendingSpawnProposals`.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-CRCLIENT.1]**
```typescript
// PCF: Provider Chain Function
// Utility method
function CreateConsumerClient(p: SpawnConsumerChainProposal) {
  // create client state
  clientState = ClientState{
    chainId: p.chainId,
    // get UnbondingPeriod from provider Staking module
    // TODO governance and CCV params
    // see https://github.com/cosmos/ibc/issues/673
    unbondingPeriod: stakingKeeper.UnbondingTime(),
    // the height when the client was last updated
    latestHeight: p.initialHeight,
  }

  // create consensus state;
  // the validator set is the same as the validator set 
  // from own consensus state at current height
  ownConsensusState = getConsensusState(getCurrentHeight())
  consensusState = ConsensusState{
    validatorSet: ownConsensusState.validatorSet,
  }

  // create consumer chain client and store it
  clientId = clientKeeper.CreateClient(clientState, consensusState)
  chainToClient[p.chainId] = clientId

  // store lockUnbondingOnTimeout flag
  lockUnbondingOnTimeout[p.chainId] = p.lockUnbondingOnTimeout
}
```
- **Caller**
  - Either `SpawnConsumerChainProposalHandler` (see [CCV-PCF-SPCCPROP.1](#ccv-pcf-spccprop1)) or `BeginBlockInit()` (see [CCV-PCF-BBLOCK-INIT.1](#ccv-pcf-bblock-init1)).
- **Trigger Event**
  - A governance proposal `SpawnConsumerChainProposal` `p` has passed (i.e., it got the necessary votes).
- **Precondition** 
  - `currentTimestamp() > p.spawnTime`.
- **Postcondition** 
  - A client state is created with `chainId = p.chainId` and `unbondingPeriod` set to the `UnbondingPeriod` obtained from the provider Staking module.
  - A consensus state is created with `validatorSet` set to the validator set the provider chain own consensus state at current height.
  - A client of the consumer chain is created and the client ID is added to `chainToClient`.
  - `lockUnbondingOnTimeout[p.chainId]` is set to `p.lockUnbondingOnTimeout`.
- **Error Condition**
  - None.

> **Note:** Creating a client of a remote chain requires a `ClientState` and a `ConsensusState` (for an example, take a look at [ICS 7](../../client/ics-007-tendermint-client)).
> `ConsensusState` requires setting a validator set of the remote chain. 
> The provider chain uses the fact that the validator set of the consumer chain is the same as its own validator set. 
> The rest of information to create a `ClientState` it receives through the governance proposal.

<!-- omit in toc -->
#### **[CCV-PCF-BBLOCK-INIT.1]**
```typescript
// PCF: Provider Chain Function
function BeginBlockInit() {
  // iterate over the pending spawn proposals and create 
  // the consumer client if the spawn time has passed
  foreach p IN pendingSpawnProposals {
    if currentTimestamp() > p.spawnTime {
      CreateConsumerClient(p)
      pendingSpawnProposals.Remove(p)
    }
  }
}
```
- **Caller**
  - The `BeginBlock()` method.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - For each `SpawnConsumerChainProposal` `p` in the list of pending spawn proposals `pendingSpawnProposals`, if `currentTimestamp() > p.spawnTime`, then
    - `CreateConsumerClient(p)` is invoked;
    - `p` is removed from `pendingSpawnProposals`.
- **Error Condition**
  - None.
  
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
  version: string): string {
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
  counterpartyVersion: string):  string {
    // validate parameters:
    // - only ordered channels allowed
    abortTransactionUnless(order == ORDERED)
    // - require the portIdentifier to be the port ID the CCV module is bound to
    abortTransactionUnless(portIdentifier == ProviderPortId)

    // assert that the counterpartyPortIdentifier matches 
    // the expected consumer port ID
    abortTransactionUnless(counterpartyPortIdentifier == ConsumerPortId)

    // assert that the counterpartyVersion matches the expected version
    abortTransactionUnless(counterpartyVersion == ccvVersion)
    
    // get the client state associated with this client ID in order 
    // to get access to the consumer chain ID
    clientId = getClient(channelIdentifier)
    clientState = clientKeeper.GetClientState(clientId)
    
    // require the CCV channel to be built on top 
    // of the expected client of the consumer chain
    abortTransactionUnless(chainToClient[clientState.chainId] == clientId)

    // require that no other CCV channel exists for this consumer chain
    abortTransactionUnless(clientState.chainId NOTIN chainToChannel.Keys())

    return CCVHandshakeMetadata{
      providerDistributionAccount: GetDistributionAccountAddress(),
      version: ccvVersion
    }
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
    - `counterpartyPortIdentifier != ConsumerPortId`;
    - `counterpartyVersion != ccvVersion`;
    - the channel is not built on top of the client created for this consumer chain;
    - another CCV channel for this consumer chain already exists.
  - A `CCVHandshakeMetadata` is returned, with `providerDistributionAccount` set to the the address of the distribution module account on the provider chain and `version` set to `ccvVersion`.
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
    // set initialHeights for this consumer chain
    initialHeights[chainId] = getCurrentHeight()
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
  - Otherwise, 
    - the channel mappings are set, i.e., `chainToChannel` and `channelToChain`;
    - `initialHeights[chainId]` is set to the current height.
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
  //   the validator set in the providerConsensusState (e.g., ICS 7)
  abortSystemUnless(gs.initialValSet == gs.providerConsensusState.validatorSet)

  // bind to ConsumerPortId port 
  err = portKeeper.bindPort(ConsumerPortId)
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
  - The genesis state contains no valid provider client state, where the validity is defined in the corresponding client specification (e.g., [ICS 7](../../client/ics-007-tendermint-client)).
  - The genesis state contains no valid provider consensus state, where the validity is defined in the corresponding client specification (e.g., [ICS 7](../../client/ics-007-tendermint-client))..
  - The genesis state contains an empty initial validator set.
  - The genesis state contains an initial validator set that does not match the validator set in the provider consensus state.
  - The capability for the port `ConsumerPortId` cannot be claimed.

> **Note**: CCV assumes that all the correct validators in the initial validator set of the consumer chain receive the _same_ consumer chain binary and consumer chain genesis state. 
> Although the mechanism of disseminating the binary and the genesis state is outside the scope of this specification, a possible approach would entail including this information in the governance proposal on the provider chain.

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
  version: string): string {
    // ensure provider channel hasn't already been created
    abortTransactionUnless(providerChannel == "")

    // validate parameters:
    // - only ordered channels allowed
    abortTransactionUnless(order == ORDERED)
    // - require the portIdentifier to be the port ID the CCV module is bound to
    abortTransactionUnless(portIdentifier == ConsumerPortId)
    // - require the version to be the expected version
    abortTransactionUnless(version == "" OR version == ccvVersion)

    // assert that the counterpartyPortIdentifier matches 
    // the expected consumer port ID
    abortTransactionUnless(counterpartyPortIdentifier == ProviderPortId)
   
    // require that the client ID of the client associated 
    // with this channel matches the expected provider client id
    clientId = getClient(channelIdentifier)   
    abortTransactionUnless(providerClient != clientId)

    return ccvVersion
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
    - `version` is set but not to the expected version;
    - `counterpartyPortIdentifier != ProviderPortId`;
    - the client associated with this channel is not the expected provider client.
  - `ccvVersion` is returned.
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
  counterpartyVersion: string):  string {
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

    // the version must be encoded in JSON format (as defined in ICS4)
    md = UnmarshalJSON(counterpartyVersion)

    // assert that the counterpartyVersion matches the expected version
    abortTransactionUnless(md.version == ccvVersion)

    // set the address of the distribution module account on the provider chain
    providerDistributionAccount = md.providerDistributionAccount

    // initiate opening handshake for the distribution token transfer channel
    // over the same connection as the CCV channel
    // i.e., ChanOpenInit (as required by ICS20)
    distributionChannelId = channelKeeper.ChanOpenInit(
      UNORDERED, // order
      channelKeeper.GetConnectionHops(channelIdentifier), // connectionHops: same as the CCV channel
      "transfer", // portIdentifier
      "transfer", // counterpartyPortIdentifier
      "ics20-1" // version
    )
}
```
- **Caller**
  - The consumer IBC routing module.
- **Trigger Event**
  - The consumer IBC routing module receives a `ChanOpenAck` message on a port the consumer CCV module is bounded to.
- **Precondition**
  - True.
- **Postcondition**
  - `counterpartyVersion` is unmarshaled into a `CCVHandshakeMetadata` structure `md`. 
  - The transaction is aborted if any of the following conditions are true:
    - `providerChannel` is already set;
    - `md.version != ccvVersion`.
  - The address of the distribution module account on the provider chain is set to `md.providerDistributionAccount`.
  - The distribution token transfer channel opening handshake is initiated and `distributionChannelId` is set to the resulting channel ID.
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

### Consumer Chain Removal
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-PCF-STCCPROP.1]**
```typescript
// PCF: Provider Chain Function
// implements governance proposal Handler 
function StopConsumerChainProposalHandler(p: StopConsumerChainProposal) {
  if currentTimestamp() > p.stopTime {
    // stop the consumer chain and do not lock the unbonding
    StopConsumerChain(p.chainId, false)
  }
  else {
    // store the proposal as a pending stop proposal
    pendingStopProposals.Append(p)
  }
}
```
- **Caller**
  - `EndBlock()` method of Governance module.
- **Trigger Event**
  - A governance proposal `StopConsumerChainProposal` has passed (i.e., it got the necessary votes).
- **Precondition** 
  - True. 
- **Postcondition** 
  - If the spawn time has already passed, `StopConsumerChain(p.chainId, false)` is invoked, with `p` the `StopConsumerChainProposal`. 
  - Otherwise, the proposal is appended to the list of pending stop proposals, i.e., `pendingStopProposals`.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-STCC.1]**
```typescript
// PCF: Provider Chain Function
function StopConsumerChain(chainId: string, lockUnbonding: Bool) {
  // cleanup state
  chainToClient.Remove(chainId)
  lockUnbondingOnTimeout.Remove(chainId)
  if chainId IN chainToChannel.Keys() {
    // CCV channel is established
    channelToChain.Remove(chainToChannel[chainId])
    channelKeeper.ChanCloseInit(chainToChannel[chainId])
    chainToChannel.Remove(chainId)
  }
  pendingVSCPackets.Remove(chainId)
  initialHeights.Remove(chainId)
  downtimeSlashRequests.Remove(chainId)

  if !lockUnbonding {
    // remove chainId form all outstanding unbonding operations
    foreach id IN vscToUnbondingOps[(chainId, _)] {
      unbondingOps[id].unbondingChainIds.Remove(chainId)
    }
    // clean up vscToUnbondingOps mapping
    vscToUnbondingOps.Remove((chainId, _))
  }
}
```
- **Caller**
  - `StopConsumerChainProposalHandler` (see [CCV-PCF-STCCPROP.1](#ccv-pcf-stccprop1)) 
    or `BeginBlockCCR()` (see [CCV-PCF-BBLOCK-CCR.1](#ccv-pcf-bblock-ccr1)) 
    or `onTimeoutVSCPacket()` (see [CCV-PCF-TOVSC.1](#ccv-pcf-tovsc1)).
- **Trigger Event**
  - Either a governance proposal to stop the consumer chain with `chainId` has passed (i.e., it got the necessary votes) or a packet sent on the CCV channel to the consumer chain with `chainId` has timed out.
- **Precondition**
  - True.
- **Postcondition**
  - The client ID mapped to `chainId` in `chainToClient` is removed.
  - The value mapped to `chainId` in `lockUnbondingOnTimeout` is removed.
  - If the CCV channel to the consumer chain with `chainId` is established, then
    - the chain ID mapped to `chainToChannel[chainId]` in `channelToChain` is removed;
    - the channel closing handshake is initiated for the CCV channel;
    - the channel ID mapped to `chainId` in `chainToChannel` is removed.
  - All the `VSCPacketData` mapped to `chainId` in `pendingVSCPackets` are removed.
  - The height mapped to `chainId` in `initialHeights` is removed.
  - `downtimeSlashRequests[chainId]` is emptied.
  - If `lockUnbonding == false`, then 
    - `chainId` is removed from all outstanding unbonding operations;
    - all the entries with `chainId` are removed from the `vscToUnbondingOps` mapping.
- **Error Condition**
  - None

> **Note**: Invoking `StopConsumerChain(chainId, lockUnbonding)` with `lockUnbonding == FALSE` entails that all outstanding unbonding operations can complete before the `UnbondingPeriod` elapses on the consumer chain with `chainId`. 
> Thus, invoking `StopConsumerChain(chainId, false)` for any `chainId` MAY violate the *Bond-Based Consumer Voting Power* and *Slashable Consumer Misbehavior* properties (see the [System Properties](./system_model_and_properties.md#system-properties) section). 
> 
> `StopConsumerChain(chainId, false)` is invoked in two scenarios (see Trigger Event above).
> - In the first scenario (i.e., a governance proposal to stop the consumer chain with `chainId`), the validators on the provider chain MUST make sure that it is safe to stop the consumer chain. 
> Since a governance proposal needs a majority of the voting power to pass, the safety of invoking `StopConsumerChain(chainId, false)` is ensured by the *Safe Blockchain* assumption (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
> 
> - The second scenario (i.e., a timeout) is only possible if the *Correct Relayer* assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section), 
> which is necessary to guarantee both the *Bond-Based Consumer Voting Power* and *Slashable Consumer Misbehavior* properties (see the [Assumptions](./system_model_and_properties.md#correctness-reasoning) section).

<!-- omit in toc -->
#### **[CCV-PCF-BBLOCK-CCR.1]**
```typescript
// PCF: Provider Chain Function
function BeginBlockCCR() {
  // iterate over the pending stop proposals 
  // and stop the consumer chain
  foreach p IN pendingStopProposals {
    if currentTimestamp() > p.stopTime {
      // stop the consumer chain and do not lock the unbonding
      StopConsumerChain(p.chainId, false)
      pendingStopProposals.Remove(p)
    }
  }
}
```
- **Caller**
  - The `BeginBlock()` method.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - For each `StopConsumerChainProposal` `p` in the list of pending spawn proposals `pendingStopProposals`, if `currentTimestamp() > p.stopTime`, then
    - `StopConsumerChain(p.chainId, false)` is invoked;
    - `p` is removed from `pendingStopProposals`.
- **Error Condition**
  - None.

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
#### **[CCV-CCF-BBLOCK-CCR.1]**
```typescript
// CCF: Consumer Chain Function
function BeginBlockCCR() {
  if providerChannel != "" AND channelKeeper.GetChannelState(providerChannel) == CLOSED {
      // the CCV channel was established, but it was then closed; 
      // the consumer chain is no longer safe

      // cleanup state, e.g., 
      // providerChannel = ""

      // shut down consumer chain
      abortSystemUnless(FALSE)
    } 
  }
}
```
- **Caller**
  - The `BeginBlock()` method.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - If the CCV was established, but then was moved to the `CLOSED` state, then the state of the consumer CCV module is cleaned up, e.g., the `providerChannel` is unset. 
- **Error Condition**
  - If the CCV was established, but then was moved to the `CLOSED` state. 

> **Note**: Once the CCV channel is closed, the provider chain can no longer provider security. As a result, the consumer chain MUST be shut down. 
> For an example of how to do this in practice, see the Cosmos SDK [implementation](https://github.com/cosmos/cosmos-sdk/blob/0c0b4da114cf73ef5ae1ac5268241d69e8595a60/x/upgrade/abci.go#L71). 

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



### Validator Set Update
[&uparrow; Back to Outline](#outline)

The *validator set update* sub-protocol enables the provider chain 
- to update the consumer chain on the voting power granted to validators on the provider chain
- and to ensure the correct completion of unbonding operations for validators that produce blocks on the consumer chain.

<!-- omit in toc -->
#### **[CCV-PCF-EBLOCK-VSU.1]**
```typescript
// PCF: Provider Chain Function
function EndBlockVSU() {
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
        downtimeSlashAcks: downtimeSlashRequests[chainId]
      }
      downtimeSlashRequests.Remove(chainId)

      // add VSCPacket data to the list of pending VSCPackets 
      pendingVSCPackets.Append(chainId, data)
    }

    // check whether there is an established CCV channel to the consumer chain
    if chainId IN chainToChannel.Keys() {
      // get the channel ID for the given consumer chain ID
      channelId = chainToChannel[chainId]

      foreach data IN pendingVSCPackets[chainId] {
        // send data using the interface exposed by ICS-4
        channelKeeper.sendPacket(
          portKeeper.getCapability(portKeeper.portPath(ProviderPortId)),
          ProviderPortId, // source port ID
          channelId, // source channel ID
          zeroTimeoutHeight,
          ccvTimeoutTimestamp,
          data
        )
      }

      // remove pending VSCPackets
      pendingVSCPackets.Remove(chainId)
    }
  }
  // increment VSC ID
  vscId++ 
}
```
- **Caller**
  - The `EndBlock()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - A list of validator updates `valUpdates` is obtained from the provider Staking module.
  - For every consumer chain with `chainId`
    - If either `valUpdates` is not empty or there were unbonding operations initiated during this block, then 
      - a `VSCPacket` data `data` is created such that `data.id = vscId`, `data.updates = valUpdates`, and `data.downtimeSlashAcks = downtimeSlashRequests[chainId]`;
      - `downtimeSlashRequests[chainId]` is emptied;
      - `packetData` is appended to the list of pending `VSCPacket`s associated to `chainId`, i.e., `pendingVSCPackets[chainId]`.
    - If there is an established CCV channel for the the consumer chain with `chainId`, then
      - for each `VSCPacketData` in the list of pending VSCPackets associated to `chainId`
        - a packet with the `VSCPacketData` is sent on the channel associated with the consumer chain with `chainId`;
      - all the pending VSCPackets associated to `chainId` are removed.
  - `vscId` is incremented.
- **Error Condition**
  - None.

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
function onTimeoutVSCPacket(packet: Packet) {
  // cleanup state
  abortTransactionUnless(packet.getDestinationChannel() IN channelToChain.Keys())
  chainId = channelToChain[packet.getDestinationChannel()]
  // stop the consumer chain and use lockUnbondingOnTimeout 
  // to decide whether to lock the unbonding
  StopConsumerChain(chainId, lockUnbondingOnTimeout[chainId])
}
```
- **Caller**
  - The `onTimeoutPacket()` method.
- **Trigger Event**
  - A `VSCPacket` sent on a channel owned by the provider CCV module timed out as a result of either
    - the timeout height or timeout timestamp passing on the consumer chain without the packet being received (see `timeoutPacket` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#sending-end));
    - or the channel being closed without the packet being received (see `timeoutOnClose` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#timing-out-on-close)). 
- **Precondition**
  - The *Correct Relayer* assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition**
  - The transaction is aborted if the ID of the channel on which the packet was sent is not mapped to a chain ID (in `channelToChain`).
  - `StopConsumerChain(chainId, lockUnbondingOnTimeout[chainId])` is invoked, where `chainId = channelToChain[packet.getDestinationChannel()]`.
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
  foreach op in GetUnbondingsFromVSC(chainId, packet.data.id) {
    // remove the consumer chain from 
    // the list of consumer chain that are still unbonding
    op.unbondingChainIds.Remove(chainId)
    // if the unbonding operation has unbonded on all consumer chains
    if op.unbondingChainIds.IsEmpty() {
      // notify the Staking module that the unbonding can complete
      stakingKeeper.UnbondingCanComplete(op.id)
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
  - For each unbonding operation `op` returned by `GetUnbondingsFromVSC(chainId, packet.data.id)`
    - `chainId` is removed from `op.unbondingChainIds`;
    - if `op.unbondingChainIds` is empty,
      - the `UnbondingCanComplete()` method of the Staking module is invoked;
      - the entry `op` is removed from `unbondingOps`.
  - `(chainId, vscId)` is removed from `vscToUnbondingOps`.
  - A successful acknowledgment is returned.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-PCF-GETUBS.1]**
```typescript
// PCF: Provider Chain Function
// Utility method
function GetUnbondingsFromVSC(
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
function AfterUnbondingInitiated(opId: uint64) {
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

  // ask the Staking module to wait for this operation 
  // to reach maturity on the consumer chains
  stakingKeeper.PutUnbondingOnHold(opId)
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
  - `opId` is appended to every list in `vscToUnbondingOps[(chainId, vscId)]`, where `chainId` is an ID of a consumer chains registered with this provider chain and `vscId` is the current VSC ID. 
  - The `PutUnbondingOnHold(opId)` of the Staking module is invoked.
- **Error Condition**
  - None.

---

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
  // TODO governance and CCV params (UnbondingPeriod)
  // see https://github.com/cosmos/ibc/issues/673
  maturityTimestamp = currentTimestamp().Add(UnbondingPeriod)
  maturingVSCs.Add(packet.data.id, maturityTimestamp)

  // reset outstandingDowntime for validators in packet.data.downtimeSlashAcks
  foreach valAddr IN packet.data.downtimeSlashAcks {
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
  // the CCV channel state is changed to CLOSED 
  // by the IBC handler (since the channel is ORDERED)
}
```
- **Caller**
  - The `onTimeoutPacket()` method.
- **Trigger Event**
  - A `VSCMaturedPacket` sent on a channel owned by the consumer CCV module timed out as a result of either
    - the timeout height or timeout timestamp passing on the provider chain without the packet being received (see `timeoutPacket` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#sending-end));
    - or the channel being closed without the packet being received (see `timeoutOnClose` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#timing-out-on-close)).
- **Precondition**
  - The *Correct Relayer* assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
- **Postcondition**
  - The state is not changed.
- **Error Condition**
  - None

<!-- omit in toc -->
#### **[CCV-CCF-EBLOCK-VSU.1]**
```typescript
// CCF: Consumer Chain Function
function EndBlockVSU(): [ValidatorUpdate] {
  // unbond mature packets if the CCV channel is established
  if providerChannel != "" {
    UnbondMaturePackets()
  }

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

  // return the validator set updates
  return changes
}
```
- **Caller**
  - The `EndBlock()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - If `providerChannel != ""`, `UnbondMaturePackets()` is invoked;
  - If `pendingChanges` is empty, the state is not changed.
  - Otherwise,
    - the pending changes are aggregated and stored in `changes`;
    - `pendingChanges` is emptied;
    - `UpdateValidatorSet(changes)` is invoked;
    - `changes` is returned.
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
      // new validator bonded;
      // note that due pendingChanges.Aggregate(), 
      // a validator can be added to the valset and 
      // then removed in the subsequent block, 
      // resulting in update.power == 0 
      if update.power > 0 {
        // add new CrossChainValidator to validator set
        val := CrossChainValidator{
          address: addr,
          power: update.power
        }
        validatorSet[addr] = val
        // call AfterCCValidatorBonded hook
        AfterCCValidatorBonded(addr)
      }
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
    - if the validator is not in the validator set and `update.power > 0`, then 
      - a new `CrossChainValidator` is added to `validatorSet`;
      - the `AfterCCValidatorBonded` hook is called;
    - otherwise, if the validator's new power is `0`, then,
      - the validator is removed from `validatorSet`;
      - the `AfterCCValidatorBeginUnbonding` hook is called;
    - otherwise, the validator's power is updated.
- **Error Condition**
  - None.

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

    // send VSCMaturedPacketData using the interface exposed by ICS-4
    channelKeeper.sendPacket(
      portKeeper.getCapability(portKeeper.portPath(ConsumerPortId)),
      ConsumerPortId, // source port ID
      providerChannel, // source channel ID
      zeroTimeoutHeight,
      ccvTimeoutTimestamp,
      packetData
    )
          
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
#### **[CCV-PCF-EBLOCK-CIS.1]**
```typescript
// PCF: Provider Chain Function
function EndBlockCIS() {
  // set VSCtoH mapping
  VSCtoH[vscId] = getCurrentHeight() + 1
}
```
- **Caller**
  - The `EndBlock()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - `vscId` is mapped to the height of the subsequent block. 
- **Error Condition**
  - None.

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
    infractionHeight = initialHeights[chainId]
  }
  else {
    infractionHeight = VSCtoH[packet.data.vscId]
  }

  // request the Slashing module to slash the validator
  // using the slashFactor set on the provider chain
  slashFactor = slashingKeeper.GetSlashFactor(packet.data.downtime)
  slashingKeeper.Slash(
    packet.data.valAddress, 
    infractionHeight, 
    packet.data.valPower, 
    slashFactor))

  // request the Slashing module to jail the validator
  // using the jailTime set on the provider chain
  jailTime = slashingKeeper.GetJailTime(packet.data.downtime)
  slashingKeeper.JailUntil(packet.data.valAddress, currentTimestamp() + jailTime)

  if packet.data.downtime {
    // add validator to list of downtime slash requests for chainId
    downtimeSlashRequests[chainId].Append(packet.data.valAddress)
  }

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
    - if `packet.data.vscId == 0`, `infractionHeight` is set to `initialHeights[chainId]`, with `chainId = channelToChain[packet.getDestinationChannel()]`, i.e., the height when the CCV channel to this consumer chain is established;
    - otherwise, `infractionHeight` is set to `VSCtoH[packet.data.vscId]`, i.e., the height at which the voting power was last updated by the validator updates in the VSC with ID `packet.data.vscId`;
    - a request is made to the Slashing module to slash `slashFactor` of the tokens bonded at `infractionHeight` by the validator with address `packet.data.valAddress`, where `slashFactor` is the slashing factor set on the provider chain;
    - a request is made to the Slashing module to jail the validator with address `packet.data.valAddress` for a period `jailTime`, where `jailTime` is the jailing time set on the provider chain;
    - if the slash request is for downtime, the validator's address `packet.data.valAddress` is added to the list of downtime slash requests from this `chainId`;
    - a successful acknowledgment is returned.
- **Error Condition**
  - None.

---

<!-- omit in toc -->
#### **[CCV-CCF-BBLOCK-CIS.1]**
```typescript
// CCF: Consumer Chain Function
function BeginBlockCIS() {
  HtoVSC[getCurrentHeight() + 1] = HtoVSC[getCurrentHeight()]
}
```
- **Caller**
  - The `BeginBlock()` method.
- **Trigger Event**
  - A `BeginBlock` message is received from the consensus engine; `BeginBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - `HtoVSC` for the subsequent block height is set to the same VSC ID as the current block height.
- **Error Condition**
  - None. 

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
  // the CCV channel state is changed to CLOSED 
  // by the IBC handler (since the channel is ORDERED)
}
```
- **Caller**
  - The `onTimeoutPacket()` method.
- **Trigger Event**
  - A `SlashPacket` sent on a channel owned by the consumer CCV module timed out as a result of either
    - the timeout height or timeout timestamp passing on the provider chain without the packet being received (see `timeoutPacket` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#sending-end));
    - or the channel being closed without the packet being received (see `timeoutOnClose` defined in [ICS4](../../core/ics-004-channel-and-packet-semantics/README.md#timing-out-on-close)).
- **Precondition**
  - The *Correct Relayer* assumption is violated (see the [Assumptions](./system_model_and_properties.md#assumptions) section).
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

    // create SlashPacket data
    packetData = SlashPacketData{
      valAddress: valAddress,
      valPower: power,
      vscId: HtoVSC[infractionHeight],
      downtime: downtime
    }

    // check whether the CCV channel to the provider chain is established
    if providerChannel != "" {
      // send SlashPacket data using the interface exposed by ICS-4
      channelKeeper.sendPacket(
        portKeeper.getCapability(portKeeper.portPath(ConsumerPortId)),
        ConsumerPortId, // source port ID
        providerChannel, // source channel ID
        zeroTimeoutHeight,
        ccvTimeoutTimestamp,
        packetData
      )

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
    - a `SlashPacket` data `packetData` is created, such that `packetData.vscId = VSCtoH[infractionHeight]`;
    - if the CCV channel to the provider chain is established, then 
      - a packet with the `packetData` is sent to the provider chain;
      - if the request is for downtime, `outstandingDowntime[data.valAddress]` is set to true;
    - otherwise `SlashRequest{data: packetData, downtime: downtime}` is appended to `pendingSlashRequests`.
- **Error Condition**
  - None.

> **Note**: The ABCI application MUST subtract `ValidatorUpdateDelay` from the infraction height before invoking `SendSlashRequest`, 
> where `ValidatorUpdateDelay` is a delay (in blocks) between when validator updates are returned to the consensus-engine and when they are applied. 
> For example, if `ValidatorUpdateDelay = x` and a validator set update is returned with new validators at the end of block `10`, 
> then the new validators are expected to sign blocks beginning at block `11+x`
> (for more details, take a look at the [ABCI specification](https://github.com/tendermint/spec/blob/v0.7.1/spec/abci/abci.md#endblock)).
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
      // send req.data using the interface exposed by ICS-4
      channelKeeper.sendPacket(
        portKeeper.getCapability(portKeeper.portPath(ConsumerPortId)),
        ConsumerPortId, // source port ID
        providerChannel, // source channel ID
        zeroTimeoutHeight,
        ccvTimeoutTimestamp,
        req.data
      )

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

### Reward Distribution
[&uparrow; Back to Outline](#outline)

<!-- omit in toc -->
#### **[CCV-CCF-EBLOCK-RD.1]**
```typescript
// CCF: Consumer Chain Function
function EndBlockRD() {
  if getCurrentHeight() - lastDistributionTransferHeight >= BlocksPerDistributionTransfer {
    DistributeRewards()
  }
}
```
- **Caller**
  - The `EndBlock()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine; `EndBlock` messages are sent once per block.
- **Precondition**
  - True. 
- **Postcondition**
  - If `getCurrentHeight() - lastDistributionTransferHeight >= BlocksPerDistributionTransfer`, the `DistributeRewards()` method is invoked.
- **Error Condition**
  - None.

<!-- omit in toc -->
#### **[CCV-CCF-DISTRREW.1]**
```typescript
// CCF: Consumer Chain Function
function DistributeRewards() {
  // iterate over all different tokens in ccvAccount
  foreach (denomination, amount) IN ccvAccount.GetAllBalances() {
    // transfer token using ICS20
    transferKeeper.TransferToken(
      denomination,
      amount,
      ccvAccount, // sender
      providerDistributionAccount, // receiver
      distributionChannelId, // transfer channel ID
      zeroTimeoutHeight, // timeoutHeight
      transferTimeoutTimestamp // timeoutTimestamp
    )
  }
  lastDistributionTransferHeight = getCurrentHeight()
}
```
- **Caller**
  - The `EndBlockRD()` method.
- **Trigger Event**
  - An `EndBlock` message is received from the consensus engine.
- **Precondition**
  - `getCurrentHeight() - lastDistributionTransferHeight >= BlocksPerDistributionTransfer`
- **Postcondition**
  - For each token type defined as a pair `(denomination, amount)` in `ccvAccount`, a transfer token (as defined in [ICS 20](../ics-020-fungible-token-transfer/README.md)) is initiated. 
  - `lastDistributionTransferHeight` is set to the current height. 
- **Error Condition**
  - None.