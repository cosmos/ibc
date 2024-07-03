---
ics: 27
title: Interchain Accounts
version: 2 
stage: Draft
category: IBC/APP
requires: 25, 26
kind: instantiation
version compatibility:
author: Tony Yun <tony@chainapsis.com>, Dogemos <josh@tendermint.com>, Sean King <sean@interchain.io>
created: 2019-08-01
modified: 2020-07-14
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the account management system over an IBC channel between separate chains.

### Motivation

ICS-27 Interchain Accounts outlines a cross-chain account management protocol built upon IBC. ICS-27 enabled chains can programmatically create accounts on other ICS-27 enabled chains & control these accounts via IBC transactions (instead of signing with a private key). Interchain accounts retain all of the capabilities of a normal account (i.e. stake, send, vote) but instead are managed by a separate chain via IBC in a way such that the owner account on the controller chain retains full control over any interchain account(s) it registers on host chain(s). 

#### Version 2 

The ICS-27 version 2 comes to address the [requirements](https://github.com/cosmos/ibc-go/blob/48f69848bb84d9bc396c750eb656f961c7d773ad/docs/requirements/ics27-multiplexed-requirements.md) defined in collaboration with the coummunity. 

### Definitions 

- `Host Chain`: The chain where the interchain account is registered. The host chain listens for IBC packets from a controller chain which contain instructions (e.g. cosmos SDK messages) that the interchain account will execute.
- `Controller Chain`: The chain registering and controlling an account on a host chain. The controller chain sends IBC packets to the host chain to control the account.
- `Interchain Account`: An account on a host chain. An interchain account has all the capabilities of a normal account. However, rather than signing transactions with a private key, a controller chain will send IBC packets to the host chain which signals what transactions the interchain account must execute. 
- `Interchain Account Owner`: An account on the controller chain. Every interchain account on a host chain has a respective owner account on the controller chain. 

The IBC handler interface & IBC relayer module interface are as defined in [ICS-25](../../core/ics-025-handler-interface) and [ICS-26](../../core/ics-026-routing-module), respectively.

### Desired properties

- Permissionless: An interchain account may be created by any actor without the approval of a third party (e.g. chain governance). Note: Individual implementations may implement their own permissioning scheme, however the protocol must not require permissioning from a trusted party to be secure.
- Fault isolation: A controller chain must not be able to control accounts registered by other controller chains. For example, in the case of a fork attack on a controller chain, only the interchain accounts registered by the forked chain will be vulnerable.
- The ordering of transactions sent to an interchain account on a host chain must be maintained. Transactions must be executed by an interchain account in the order in which they are sent by the controller chain. 
- An icaOwnerAccount on the controller chain can manage 1..n hostAccount(s) on the host chain. An hostAccount on the host chain can be managed by 1 and only 1 icaOwnerAccount on the controller chain. 
- The controller chain must store the account address of any owned interchain accounts registered on host chains. 
- A host chain must have the ability to limit interchain account functionality on its chain as necessary (e.g. a host chain can decide that interchain accounts registered on the host chain cannot take part in staking). This should be achieved with a blacklist mechanisms. 
- The controller chain must be able to set up multiple interchain account(s) on the host chain within a single transaction. 
- Many controller accounts should be able to send messages to many host accounts through the same channel.
- The number of packet round trips to register an account, load the account with tokens and execute messages on the account should be minimised.  
- A chain can utilize one or both subprotocols (described below) of the interchain account protocol. A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

### General design 

The interchain account protocol defines the relationship and interactions between two chains with different roles: the controller chain and the host chain. The protocol allows a controller chain to create and manage accounts on a host chain programmatically through IBC transactions, bypassing the need for private key signatures.

This specification defines the general way to send tx bytes from a controller chain, on an already established ica-channel, to an host chain. The host chain is responsible for deserializing and executing the tx bytes and the controller chain must know how the host chain will handle the tx bytes in advance of sending a packet, thus this must be negotiated during channel creation.

The ICS-27 version 2 is composed of two subprotocols, namely `icaControlling` and `icaHosting`. The *icaControlling* must be implemented on the controlling chain and is responsible for sending IBC packets to register and manage interchain accounts on the host chain. The *icaHosting* must be implemented on the host chain and is responsible for receiving IBC packets, generate addresses and executing transactions on behalf of the controller chain.

## Technical specification

### Data Structures

We define two types of interchain account packet data the `icaRegisterPacketData` and the `icaExecutePacketData`. Each of the packet data has its own data structure.

Additionally, we define the `icaPacketDataType` that will be used to distinguish between the two type of packets. 

```typescript
enum icaPacketDataType {
  REGISTER,  
  EXECUTE,         
}
```

The `icaRegistrationPacketData` contains the parameters `icaOwnerAddress` and the array of `hostAccountIds` that will be passed by the controller chain to the host chain that will be used in conjunction with the `packet.sourcePort` and `packet.sourceChannel` to generate the addresses on the host chain. 

```typescript
interface icaRegisterPacketData {
  icaType: icaPacketDataType = REGISTER,
  icaOwnerAddress : string, 
  hostAccountIds: [] uint64 // The hostAccountIds to be used for registration within a single tx
  // memo: string, // Investigate if we want memo here.       
}
```

The `icaExecutePacketData` contains the parameters `icaOwnerAddress`, the array of `hostAccountIds` , an array of `msgs` and a `memo` that will be passed by the controller chain to the host chain to execute the msgs on the associated account addresses. 

```typescript
interface icaExecutePacketData {
  icaType: icaPacketDataType = EXECUTE,
  icaOwnerAddress : string, 
  hostAccountIds: [] uint64,
  msgs: [] Any, //msg 
  memo: string,  
}
```

ICS-27 version 2 defines four acknowledgement data type, namely `icaExecutePacketSuccess`, `icaRegisterPacketSuccess`,`icaRegisterPacketError`, `icaExecutePacketError`. Each of them stores different values that are used in a different flows of the interchain account protocol.

```typescript
// type icaPacketAcknowledgement = icaExecutePacketSuccess | icaRegisterPacketSuccess | icaRegisterPacketError | icaExecutePacketError;  
enum icaPacketAcknowledgementType{
  REGISTER_SUCCESS, 
  EXECUTE_SUCCESS,
  REGISTER_ERROR,
  EXECUTE_ERROR, 
}
```

Whether an interchain account flow fails the reason for failure (if any) will be returned. 

```typescript
interface icaRegisterPacketError {
  type: icaPacketAcknowledgementType = REGISTER_ERROR,  
  error: string
}
```

```typescript
interface icaExecutePacketError {
  type: icaPacketAcknowledgementType = EXECUTE_ERROR,  
  error: string
}
```

The `icaRegisterPacketSuccess` is defined to handle the succesfull registration flow. In the success case the account address generated on the host chain will be stored in the acknowledgement and delivered back to the controller chain.

```typescript
interface icaRegisterPacketSuccess { 
  type: icaPacketAcknowledgementType = REGISTER_SUCCESS,  
  hostAccounts: [] string,  
}
```

The `icaExecutePacketSuccess` is defined to handle the succesfull execution flow. In the success case the an array of bytes `resultData`, containing the ordered return values of each message execution, will be stored in the acknowledgement and delivered back to the controller chain.

```typescript
interface icaExecutePacketSuccess {
  type: icaPacketAcknowledgementType = EXECUTE_SUCCESS,  
  resultData: [] bytes,  
}
```

### Sub-protocols

#### Routing module callbacks

The routing module callbacks associated to the channel management are at the base of the interchain account protocol. Any of the chain (controlling or hosting) can start a channel creation handshake. 

##### Channel lifecycle management

// TODO INCLUDE TX TYPES AND ENCODING CONSIDERATION that should be negotiated during channel creation handshake 

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ica` or `""`.  

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ica" or ""
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel  
  abortTransactionUnless(version === "ica" || version === "")

  if version == "" {
    // default to latest supported version
    return "ica", nil
  }
  // If the version is not empty and is among those supported, we return the version
  return version, nil 
}
```

```typescript
function onChanOpenTry(
  order: ChannelOrder,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ica" or "ica-2" 
  abortTransactionUnless(counterpartyVersion === "ica")

  // return the same version as counterparty version so long as we support it
  return counterpartyVersion, nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  // port has already been validated
  // assert that counterparty selected version is the same as our version
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(counterpartyVersion === channel.version)
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated, version has already been validated
}
```

```typescript
// Channel closure is disabled.
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // always abort transaction
    abortTransactionUnless(FALSE)
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

### Transaction Types 

The ICS-27 version 2 defines two types of transactions `REGISTER_TX` and `EXECUTE_TX`.

```typescript
enum icaTxTypes {
  REGISTER_TX,
  EXECUTE_TX
}
```

#### **icaControlling**

##### Interchain Account EntryPoints 

To interact with the interchain account protocol, the user can generate two types of tx, namely `REGISTER_TX` and `EXECUTE_TX`. For each Tx type, we define an icaTxHandler so that we have `icaRegisterTxHandler` and `icaExecuteTxHandler`. The icaTxHandlers serve as the entrypoints for the interchain account module functionalities. 
Both tx handlers must verify, that the signer is the `icaOwnerAddress` and then, based on the type of message, they must call the associated functions that will construct the related kind of packet. 

```typescript
function icaRegisterTxHandler(portId: string, channelId: string, icaOwnerAddress: string, hostAccountNumber: unit64)returns (uint64) {

  // Ensure the tx has been dispatched to the correct handler
  abortTransactionUnless(this.Tx.type===REGISTER_TX)
  // verify Tx Signature:: must be signed by icaOwnerAddress
  abortTransactionUnlesss(IsValidAddress(icaOwnerAddress))
  abortTransactionUnless(this.Tx.signer===icaOwnerAddress) // CHECK PROPER SYNTAX
  // Validate functions parameter.. 

  // Should compute and pass in timeout related things or this should be done in sendRegisterTx? 
  return sequence = sendRegisterTx(portId, channelId, icaOwnerAddress, hostAccountNumber) 
}
```

```typescript
function icaExecuteTxHandler(portId: string, channelId: string, icaOwnerAddress: string, hostAccountIds:[] unit64, msgs: []msgs, memo:string )returns (uint64){
  
  // Ensure the tx has been dispatched to the correct handler
  abortTransactionUnless(this.Tx.type===EXECUTE_TX)
  // verify Tx Signature:: must be signed by icaOwnerAddress
  abortTransactionUnlesss(IsValidAddress(icaOwnerAddress))
  abortTransactionUnless(this.Tx.signer===icaOwnerAddress)// CHECK PROPER SYNTAX
  // Validate functions parameter.. 

  // call sendExecuteTx 
  // Should compute and pass in timeout related things or this should be done in sendExecuteTx? 
  return sequence= sendExecuteTx(portId, channelId, icaOwnerAddress, hostAccountIds, msgs, memo) 
}
```

##### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port (owned by the module).

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the interchain account module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

```typescript
function setup() {
  capability = routingModule.bindPort("icacontroller", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit, // Force Abort
    onChanCloseConfirm, // Do Nothing
    // TODO Missing Upgrade Callbacks
    onRecvPacket, // Force Abort
    onTimeoutPacket,
    onAcknowledgePacket,
  })
  claimCapability("port", capability)
}
```

##### Module State 

The interchain account module keeps track of the controlled hostAccounts associated with particular channels in state and the `nextHostAccountId`.  Fields of the `ModuleState` are assumed to be in scope.

The `nextHostAccountId` starts at 0 and it is increased linearly as new account get registered. 

```typescript
interface ModuleState {
  hostAccounts: Map<portId, 
    Map <channelId, 
    Map <icaOwnerAddress, 
    Map <hostAccountId,
    hostAccountAddress>>>>,
  nextHostAccountId : uint64,
  unusedHostAccountIds: []uint64  
}
```

##### Utility functions

```typescript

function InitInterchainAccountAddress(portId: string, channelId: string, icaOwnerAddress:string, hostAccountId:uint64) returns (error) {
  
  // Abort if hostAccountAddress at the specified keys already exist. 
  if(getInterchainAccountAddress(portId,channelId,icaOwnerAddress,hostAccountId)!=="") {
    error={"intechainAccountKeys already used"}
    return error 
  }
  // Initialize in module mapping to hostAccount Data 
  // Address set to init during intialization. The address will be generated by the host chain and returned in the acknowledgment. 
  hostAccounts[portId][channelId][icaOwnerAddress][hostAccountId].hostAccountAddress=""
  return nil 
}

// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(portId: string, channelId: string, icaOwnerAccount: string,  hostAccountId: uint64, address: string) returns (string) {

hostAccounts[portId][channelId][icaOwnerAccount][hostAccountId].hostAccountAddress=address

return address 
}

// Retrieves the interchain account address from state.
function GetInterchainAccountAddress(portId: string, channelId: string,icaOwnerAddress: string, hostAccountId: uint64) returns (string){

return hostAccounts[portId][channelId][icaOwnerAddress][hostAccountId].hostAccountAddress

}
```

##### Helper Functions

The helper functions described herein must be implemented in a controlling interchain account module. 

###### **RegisterInterchainAccount**

The `RegisterInterchainAccount` function of the controlling part of the interchain account protocol provides the semantics to initialize the interchain account registration. When a user on the controller chain wants to register new interchain accounts, will send a Tx which type is `REGISTER_TX`, including the parameter `hostAccountNumber` to specify the number of accounts to register, that will trigger the `RegisterInterchainAccount` function logic to be executed on the controller chain. The function will be executed taking in input the `hostAccountId` that will be used for the generation. This `hostAccountId` will be computed or from the list of `unusedHostAccountIDs` or from  `nextHostAccountId` parameter in the moduleState. Thus the function will initialize the mapping of `hostAccounts` between the tuple key  (portId, channelId, ownerAccount, hostAccountId) and the hostAcccountAddress maintained by the controller chain in the moduleState.

The host chain Must be able to generate and store in state the hostAccountAddress, that will be controlled by the icaOwnerAddress by using the information provided about the hostAccounts passed in the `REGISTER_TX` and must pass back the generated address inside the ack. Once received the ack, the controller chain must store the hostAccountAddress generated address in the mapping previously described. In the case of error during the registration, the `usedHostAccountIds` will be added to the `unusedHostAccountIDs` array. 

```typescript
function RegisterInterchainAccount(
  portId: string, 
  channelId: string,
  icaOwnerAddress: string,
  hostAccountNumber: uint64 // The number of accounts the icaOwnerAddress wants to register within a single tx 
  ) 
  returns ([]hostAccountIds,err) {

  for i in 0..hostAccountNumber{
    let hostAccountId: uint64 
    if(unusedHostAccountIds.length > 0) {
      // Use an unused ID if available
      hostAccountId = unusedHostAccountIds.pop();
    } else {
      // Otherwise, use the nextHostAccountId
      hostAccountId = nextHostAccountId;
      nextHostAccountId++;
    }
    // Use hostAccountId to initialize the account 
    err=InitInterchainAccountAddress(portId, channelId, icaOwnerAddress,  
    hostAccountId)
    abortTransactionUnless(err!==nil)
    // Push into hostSequenceNumber array the used nextHostAccountId
    // We will return this array to keep track of the used sequence numbers  
    hostAccountIds.push(hostAccountId)  
  }

  return hostAccountIds,nil 
}
```

##### Packet relay

`SendRegisterTx` and `SendExecuteTx` must be called by a transaction handler in the controller chain module which performs appropriate signature checks. In particular the transaction handlers must verify that `icaOwnerAddress` is the actual signer of the tx.

//TODO CLARIFY WAY BETTER THE CONCEPT  
Thinking about a smart contract system, then the system should verify that the tx the user generate to call the `SendRegisterTx` and `SendExecuteTx` contract function has been signed by the `icaOwnerAddress`.  

`SendRegisterTx` is used by a controller chain to send an IBC packet containing instructions on the number of host accounts to create on a host chain for a given interchain account owner. 

```typescript
function SendRegisterTx( 
  sourcePort: string,
  sourceChannel: string,
  icaOwnerAddress: string,
  hostAccountNumber: uint64, // Account number for which we are requesting the generation 
  //memo: string,   // Do we want to allow memo to be used in here? Probably we should not 
  
) returns (uint64) {

  // Compute
  // timeoutHeight: Height,
  // timeoutTimestamp: uint64, // in unix nanoseconds
 
  // retrieve channel 
  channel = provableStore.get(channelPath(sourcePort, sourceChannel))
  abortTransactionUnless(channel.version=="ica")
  // validate that the channel infos
  abortTransactionUnless(isActive(channel))
  // validate timeoutTimestamp
  abortTransactionUnless(timeoutTimestamp <= currentTimestamp())
  // validate Height? 

  let err : error = nil
  let usedhostAccountIds : []uint64 = []

  abortTransactionUnless(hostAccountNumber > 0)
  
  // A single registration message enable the registration of n accounts. 
  // A potential limit of the number of accounts to guarantee a non out-of-gas error is to be discussed
  usedHostAccountIds,err = RegisterInterchainAccount(sourcePort, sourceChannel, icaOwnerAddress,hostAccountNumber)
  
  abortTransactionUnless(err!=nil)     
  
  //SendRegisterTx has been called by a transaction handler in the controller chain module which has performed appropriate signature checks for the icaOwnerAddress such that we can assume the tx has already been validated for being originated by the icaOwnerAddress

  icaPacketData = icaRegistrationPacketData{icaOwnerAccount,usedHostAccountIds}
  
  // send packet using the interface defined in ICS4
  sequence = handler.sendPacket(
    getCapability("port"),
    sourcePort,
    sourceChannel,
    timeoutHeight,
    timeoutTimestamp,
    protobuf.marshal(icaPacketData) // protobuf-marshalled bytes of packet data
  )
  return sequence
}
```

`SendExecuteTx` is used by a controller chain to send an IBC packet containing instructions (messages) and the host accounts references that should execute the tx on behalf of the interchain account owner. 

// TODO 

```typescript
function SendExecuteTx( 
  portId: string,
  channelId: string,
  icaOwnerAddress: string,
  hostAccountIds: [] uint64,  // TODO Reason about this. Maybe could use addresses directly  
  msgs: []msg, 
  memo: string)
  returns (uint64){
  
  // Verify that the provided hostAccountIds match with an already registered hostAccountAddress
  for seq in hostAccountIds{
    abortTransactionUnless(GetInterchainAccountAddress(portId,channelId,icaOwnerAddress,seq)!=="", "Interchain Account Not Registered")
    }
  
  // It exist at least one message to be executed 
  abortTransactionUnless(msgs.isNotEmpty())

  icaPacketData = IcaExecutePacketData{icaOwnerAddress,hostAccountsSequences,msgs,memo}

  // send packet using the interface defined in ICS4
  sequence = handler.sendPacket(
    getCapability("port"),
    portId, //sourcePort,
    channelId, //sourceChannel,
    timeoutHeight,
    timeoutTimestamp,
    protobuf.marshal(icaPacketData) // protobuf-marshalled bytes of packet data
  )
  return sequence
}

```

Note that interchain accounts controller modules should not execute any logic upon packet receipt, i.e. the `OnRecvPacket` callback should not be called, and in case it is called, it should simply return an error acknowledgement:

```typescript
// Called on Controller Chain by Relayer
function onRecvPacket(packet Packet) {
  return NewErrorAcknowledgement(ErrInvalidChannelFlow)
}
```

// CHECK STUFF IN HERE 
`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
// Called on Controller Chain by Relayer
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) { 
  ack=Deserialize(acknowledgement)
  switch ack.type: 
    case types.REGISTER_SUCCESS:
      
      // We have to store the addresses into our module state mapping 
      let i :uint64 = 0
      // Store addresses under the right sequence number. 
      for address in ack.hostAccountAddress{
        // Store in hostAccounts module state the address returned in the ack
        hostAccounts[packet.sourcePort][packet.sourceChannel][packet.data.icaOwnerAddress][packet.data.hostAccountIds.getElement(i)].hostAccountAddress=address
        // Increment the positional number 
        i=i+1 
    }
    // call underlying app's OnAcknowledgementPacket callback 
    // see ICS-30 middleware for more information
    case types.EXECUTE_SUCCESS: 
    //TODO Verify no op is ok. In theory the user should be able to read the information returned in the ack by reading the state. 
    // No Op 
    case types.EXECUTE_ERROR:
    //TODO Verify no op is ok. In theory no state changes happened on controller chain, so no op required 
    // No Op 
    case types.REGISTER_ERROR:
       // In case of registration errors, populate the unusedHostAccountsIds with the ids provided for this registering Tx. The usage of the unusedHostAccountIds serves to deter potential gaps in the sequence of account IDs 
       for id in packet.data.hostAccountIds {
        unusedHostAccountIds.push(id);
      }
  }
```

```typescript
// Called on Controller Chain by Relayer
function onTimeoutPacket(packet: Packet) {

  switch packet.data.icaType { 
    case types.REGISTER:
        for id in packet.data.hostAccountIds {
        unusedHostAccountIds.push(id);
      }
    case types.EXECUTE:
    // No Op
  }
}
```

#### *icaHosting*

##### Port & channel setup

Here we define the setup function for the *Hosting* subprotocol. 

```typescript
function setup() {
  capability = routingModule.bindPort("icahost", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit, // Force Abort 
    onChanCloseConfirm, // Do nothing
    // TODO Missing Upgrade Callbacks
    onRecvPacket,
   // onTimeoutPacket,
   // onAcknowledgePacket,     
  })
  claimCapability("port", capability)
}
```

##### Module State Host Chain

The interchain account module on host chain must track the hostAccounts and should track the blacklist of msgs associated with particular contoller account in state. Fields of the `ModuleState` are assumed to be in scope.

// TODO 

// NOTE Need to inspect how to TODO blacklist mechanisms. Do the controller chain need to know which msg are blacklisted? Probably not. Thus the blacklist couldbe a module state in host chain that can be modified by an ad hoc 
function 

```typescript
interface blacklistedMessages{
  portId: Identifier, 
  channelId: Identifier, 
  msgs: []Any, // Msg Type?  
}
```

// Should be the blacklist icaOwnerAccounts specific? 

```typescript
interface ModuleState {
  hostAccounts: Map<portId, Map <channelId, Map <icaOwnerAddress, Map<hostAccountId, hostAccount>>>>,
  //hostAccounts: Map<hostAccount.sequenceNumber, hostAccount.address>,
  //TODO blacklist: [] blacklistedMessages 
}
```

##### Utility functions

```typescript

// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(portId: string, channelId: string, icaOwnerAccount: string,  hostAccountId: uint64) returns (string){

// Generate new address 
// newAddress MUST generate deterministically the host account address 
address=newAddress(portId,channelId, icaOwnerAccount, seq)
// Set in the host chain module state the generated address 
hostAccounts[portId][channelId][icaOwnerAccount][hostAccountId].hostAccountAddress=address

return address 
}

// Retrieves the interchain account from state. // Verify if should be move the protocol base part. 
function GetInterchainAccountAddress(portId: string, channelId: string, icaOwnerAddress: string, hostAccountId:uint64) returns (string){

return hostAccounts[portId][channelId][icaOwnerAddress][hostAccountId].hostAccountAddress
}
```

##### Helper Function

The helper functions described herein must be implemented in a hosting interchain account module. 

###### **RegisterInterchainAccount**

`RegisterInterchainAccount` may be called during the `OnReceive` callback when a `REGISTER_TX` tx type is relayed to the host chain and contains the parameter `hostAccountNumber`.
 
```typescript
function RegisterInterchainAccount(
  portId: string,
  channelId: string,
  counterpartyPortId: string, 
  counterpartyChannelId: string,
  icaOwnerAccount: string,
  usedSequences: []uint64 
  ) {  

  // validate port format
  abortTransactionUnless(portId=="ica") 
  // retrieve channel 
  channel = provableStore.get(channelPath(portId, channelId))
  // validate that the channel infos
  abortTransactionUnless(isActive(channel))
  abortTransactionUnless(channel.counterpartyPortId == counterpartyPortId) 
  abortTransactionUnless(channel.counterpartyChannelId == counterpartyChannelId)   

  for seq in usedSequences{
    SetInterchainAccountAddress(portId,channelId,icaOwnerAddress,seq)
  }
  
}
```

##### **ExecuteTx**

Executes each message sent by the owner account on the controller chain.

```typescript
function ExecuteTx(hostAccount: string, msg Any) returns (resultString, error) {
  
  // Signature has already been validated in the sendTx 
  // Execute the msg for the given hostAccount 
  
   // Review Syntax
  return hostAccount.execute(msg), nil  
  // return result of transaction
}
```

##### **BlacklistMsg** 

// TODO BlackList Function for Host writing into module state blacklisted msgs

##### Packet relay

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
// Called on Host Chain by Relayer
function onRecvPacket(packet Packet) {
  // The host chain can receive two interchain account packet types: REGISTER_TX and EXECUTE_TX
  // Thus we must handle both cases in the onRecv
  switch packet.data.icaType { 
    case types.REGISTER:

      (_,icaOwnerAccount,hostAccountIds,err) = types.Deserialize(packet.data)

      if err != nil {
        return NewErrorAcknowledgement(err)
      }

      // We want to return the addresses that will be generated in an array of string placed in the ack.  
      let newAccounts: []string = []
    
      for seq in hostAccountIds {
        RegisterInterchainAccount(Packet.DestinationPort,Packet.DestinationChannel,Packet.SourcePort,Packet.SourceChannel, icaOwnerAccount,seq)
        // We want to return in the ack only the new generated accounts. So we retrieve them from the module state
        newAccounts.push(hostAccounts[seq].hostAccountAddress)
      }
  
      //icaPacketAcknowledgement = icaExecutePacketSuccess | icaRegistrationPacketSuccess | icaPacketError; 
      icaPacketAcknowledgement ack = icaRegisterPacketSuccess{newAccounts}
      return ack

    case types.EXECUTE:

      (_,icaOwnerAccount,hostAccountIds,msgs,memo,err) = types.Deserialize(packet.data)
    
      if err != nil {
        return NewErrorAcknowledgement(err)
      }
   
      // TODO Check Optimizations  
      let resultsData : [] bytes = []
      let executingAddress: string = "" 
      let hostAddressesSet: set(string)

    // Given the hostAccountIds, retrieve the hostAccountsAddress and place them in a set of address. 
    // If I pass directly the addresses retrieved in the controller chain, we may skip this. 
    for seq in hostAccountIds{ 
      temp= getInterchainAccountAddress(icaOwnerAddress,seq)
      abortTransactionUnless(temp!=="", "Requesting Tx For A Non Registered Account")
      hostAddressesSet.add(temp)
      temp=""
      }
  
    for msg in msgs{
      // TODO Include check for blacklisted message.
      executingAddress = msg.expectedSigner()
      // Verify that the expectedSigner is in the set of host addresses provided for this IBC tx.
      // Ok so basically here the idea is just to confirm that the expected signer is part of a set of the hostAccounts set passed in by the icaOwnerAddress. Is this enough? 
      abortTransactionUnless(executingAddress.isIn(hostAddressesSet), "Expected Signer Mismatch")

      // ExecuteTx executes each message individually  
      resultData, err = ExecuteTx(executingAddress, msg)
      if err != nil {
        // In case any of the msg in the for loop fails, everything will be reverted by returning the error ack. 
        return NewErrorAcknowledgement(err)
      }
      // Only push result if no error is detected 
      resultsData.push(resultData)       
    }
    
    InterchainAccountPacketAcknowledgement ack = icaExecutePacketSuccess{resultData}
    
    return ack
  }
  
  default:
    return NewErrorAcknowledgement(ErrUnknownDataType)
  }
```

### Register & controlling flows

// TODO
[DiagramBaseImage](https://excalidraw.com/#json=BfGp0ZbDAhO_LWiNmdGGn,Mgw4FrmorzuGjM0alzLMWw
)

## Considerations

### Interchain Account Recovery

Since we are allowing only unordered channels and disallowing channel closure procedures, we don't need a procedure for recovering. Channel cannot be closed or get stucked. 

### Message Execution Ordering 

Problem:  
Given chain A and chain B, if chain A sends two IBC packets, each one containing an  `EXECUTE_TX` message, the order of execution of the packets, and so of the messages, is not guaranteed because it depends on the relayer delivery order of the packets themself. 

Solution: 
The user who needs a certain order of execution for its messages Must place them in the same IBC packet. When the messages are placed in the same IBC packet, we can guarantee the atomicity and the order of execution. Indeed the messages that are passed in a interchain account `EXECUTE_TX` will be executed by the host chain following a FIFO mechanisms. 

### Account balances post execution on host chain 

In the case the controller chain wants to know the host account balance after certain msgs are executed, it should include a crosschain-query message at the bottom of the msg list. 

## Example Implementations

- Implementation of ICS 27 version 1 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

- Implementation of ICS 27 version 2 in Go COMING SOON. 

## Future Improvements

// TODO  

A future version of interchain accounts may be greatly simplified by the introduction of an IBC channel type that is ORDERED but does not close the channel on timeouts, and instead proceeds to accept and receive the next packet. If such a channel type is made available by core IBC, Interchain accounts could require the use of this channel type and remove all logic and state pertaining to "active channels". The metadata format can also be simplified to remove any reference to the underlying connection identifiers.

The "active channel" setting and unsetting is currently necessary to allow interchain account owners to create a new channel in case the current active channel closes during channel timeout. The connection identifiers are part of the metadata to ensure that any new channel that gets opened are established on top of the original connection. All of this logic becomes unnecessary once the channel is ordered **and** unclosable, which can only be achieved by the introduction of a new channel type to core IBC.

## History

Aug 1, 2019 - Concept discussed

Sep 24, 2019 - Draft suggested

Nov 8, 2019 - Major revisions

Dec 2, 2019 - Minor revisions (Add more specific description & Add interchain account on Ethereum)

July 14, 2020 - Major revisions

April 27, 2021 - Redesign of ics27 specification

November 11, 2021 - Update with latest changes from implementation

December 14, 2021 - Revisions to spec based on audits and maintainer reviews

August 1, 2023 - Implemented channel upgrades callbacks

// TODO
    
## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
