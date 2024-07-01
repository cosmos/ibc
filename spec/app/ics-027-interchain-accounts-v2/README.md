---
ics: 27v2
title: Interchain Accounts
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
- If a channel closes, the controller chain must be able to regain access to registered interchain accounts by simply opening a new channel.
- Each interchain account is owned by a single account on the controller chain. Only the owner account on the controller chain is authorized to control the interchain account. The controller chain is responsible for enforcing this logic.
- The controller chain must store the account address of any owned interchain accounts registered on host chains. 
- A host chain must have the ability to limit interchain account functionality on its chain as necessary (e.g. a host chain can decide that interchain accounts registered on the host chain cannot take part in staking). This should be achieved with a blacklist mechanisms. 
- The controller chain must be able to set up multiple interchain account(s) on the host chain within a single transaction.
- The controller chain should be able to fund an interchain account in the same registering transaction.  
- The distinct interchain account owner(s) on the same controller chain, controlling interchain accounts on the same host chain, must be able to use the same channel.
- An icaOwnerAccount on the controller chain can manage 1..n hostAccount(s) on the host chain. An hostAccount on the host chain can be managed by 1 and only 1 ownerAccount on the controller chain. 
- A chain can utilize one or both subprotocols (described below) of the interchain account protocol. A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

### General design 

The interchain account protocol defines the relationship and interactions between two types of chains: the controller chain and the host chain. The protocol allows a controller chain to create and manage accounts on a host chain programmatically through IBC transactions, bypassing the need for private key signatures.

This specification defines the general way to send tx bytes from a controller chain, on an already established ica-channel, to be executed on behalf of the owner account on the host chain. The host chain is responsible for deserializing and executing the tx bytes and the controller chain must know how the host chain will handle the tx bytes in advance of sending a packet, thus this must be negotiated during channel creation.

As represented in [image1](https://excalidraw.com/#json=bv5-bwApNyeM9tqAGIMjo,-tTILw_EtAi7t7oAyv28HQ) the specification is presented in three main parts:

*icaBase*: The part of the protocol that must be implemented on the both the chains and serves as the common ground for both kind of chain to communicate. 

*icaControlling*: The part of the protocl that must be implemented on the controlling chain, responsible for sending IBC packets to register and manage interchain accounts on the host chain.

*icaHosting*: The part of the protocol that must be implemented on the host chain, responsible for receiving IBC packets and executing transactions on behalf of the interchain accounts.

For the sake of clarity, we provide first the technical specification of the *icaBase* part. Later on we provide the technical specification of the *icaControlling* and finally we provide details over the *icaHosting*.  

## Technical specification

### **icaBase** 

#### Routing module callbacks

The routing module callbacks are common at the base of the interchain account protocol. Any of the chain (controlling or hosting) can start a channel creation handshake. 

##### Channel lifecycle management

// TODO INCLUDE TX TYPE AND ENCODING CONSIDERATION 

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ica` or `""`.  

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier], // Seen this present in ICS20. Really Needed'?
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ica" or "ica-2" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel 
  //NOTE shall we change ica to ica-1? 
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
  connectionHops: [Identifier], // Seen this present in ICS20. Really Needed'?
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

### Data Structures

// Not sure I need an interface for that. Since it will be stored differently on controller and host, should I just define the hostAccount as part of the module states? 

The `hostAccount` interface contains the `sequenceNumber`, and the `address` that will be generated and returned by the host chain in the acknowledgment, as effect of the account registration procedure. 

```typescript


interface hostAccount{
  sequenceNumber: uint64, 
  address: string
}


```

#### Packet Data

// Is this ok? 

The `InterchainAccountPacketData` can be of two types the `icaRegisterPacketData` and the `icaExecutePacketData`. 

ICS-27 version 2 has two types of packet `REGISTER_TX` and `EXECUTE_TX`.

```typescript
enum icaPacketDataType {
  REGISTER_TX,
  EXECUTE_TX
}
```

Each of the icaPacketDataType has its own packet data structure, 

```typescript
type InterchainAccountPacketData = icaRegisterPacketData | icaExecutePacketData;  
```

// Potentially we can have a single packet data : icaPacketData - Investigate if this separation is Necessary

The `icaRegistrationPacketData` contains the information `icaOwner.address` and the array of `hostAccountSequences` that will be passed by the controller chain to the host chain that will be used in conjunction with the `packet.sourcePort` and `packet.sourceChannel` to generate the addresses on the host chain. 

```typescript
interface icaRegisterPacketData {
  icaOwnerAddress : string, 
  hostAccountSequences: [] uint64 // The hostAccountSequences to be used for registration within a single tx
  // memo: string, // Investigate if we want memo here.       
}
```

The `icaExecutePacketData` contains the information `icaOwner.address`, the array of `hostAccountSequences` , an array of `msgs` and a `memo` that will be passed by the controller chain to the host chain to execute the msgs on the associated account addresses. 

```typescript
interface icaExecutePacketData {
  icaOwnerAddress : string, 
  hostAccountSequences: [] uint64,
  msgs: [] Any, //msg 
  memo: string,  
}
```

The acknowledgement data type can be of three types, namely `icaExecutePacketSuccess`, `icaRegisterPacketSuccess` and `icaPacketError`. Each of them stores different values that are used in a different flows of the interchain account protocol.

```typescript
type icaPacketAcknowledgement = icaExecutePacketSuccess | icaRegisterPacketSuccess | icaPacketError;  
```

For the registration message, in the success case the account address generated on the host chain will be returned.

```typescript
interface icaRegistrationPacketSuccess { 
  hostAccounts: [] hostAccount,  
}
```

While for the execution message, in the success case, an array of bytes `resultData`, containing the ordered return values of each message execution, is returned. 

```typescript
interface icaExecutePacketSuccess {
  resultData: [] bytes,  
}
```

Whether the interchain account actions fails the reason for failure (if any) will be returned. 

```typescript
interface icaPacketError {
  error: string
}
```

### **icaControlling**

We now specify the things related to the controlling subprotocol. 

#### Port & channel setup

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

### Data Structures

```typescript
interface icaOwner{
  address: string
}
```

### Module State 

The interchain account module keeps track of the controlled hostAccounts associated with particular channels in state and the `nextHostAccountSequenceNumber`.  Fields of the `ModuleState` are assumed to be in scope.

The `nextHostAccountSequenceNumber` starts at 0 and it is increased linearly as new account get registered. 

```typescript
interface ModuleState {
  hostAccounts: Map<portId, 
    Map <channelId, 
    Map <icaOwner.address, 
    Map <hostAccount.sequenceNumber,
    hostAccount.address>>>>,
  nextHostAccountSequenceNumber : uint64  
}
```

#### Utility functions

// TODO These functions have to be adapted in case we will use it 

```typescript

function InitInterchainAccountAddress(portId: string, channelId: string, icaOwnerAddress:string, hostAccountSequenceNumber:uint64) returns (error) {
  
  // Abort if hostAccountAddress at the specified keys already exist. 
  if(getInterchainAccountAddress(portId,channelId,icaOwnerAddress,hostAccountSequenceNumber)!=="") {
    error={"intechainAccountKeys already used"}
    return error 
  }
  // Initialize in module mapping to hostAccount Data 
  // Address set to empty in intialization. Will be generated by the host chain and returned in the acknowledgment. 
  hostAccounts[portId][channelId][icaOwnerAddress][hostAccountSequenceNumber].hostAccountAddress=""
  // Depend on 4.1 hostAccounts[icaOwnerAddress][hostAccountSequenceNumber].TxSequence=0

  return nil 
}

// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(portId: string, channelId: string, icaOwnerAccount: string,  hostAccountSequenceNumber: uint64, address: string) returns (string) {

hostAccounts[portId][channelId][icaOwnerAccount][hostAccountSequenceNumber].hostAccountAddress=address

return address 
}

// Retrieves the interchain account from state.
function GetInterchainAccountAddress(portId: string, channelId: string,icaOwnerAddress: string, hostAccountSequenceNumber:uint64) returns (string){

return hostAccounts[portId][channelId][icaOwnerAddress][hostAccountSequenceNumber].hostAccountAddress

}
```

### Interchain Account EntryPoints 

To interact with the interchain account protocol, the user can generate two types of tx, namely `REGISTER_TX` and `EXECUTE_TX`. For each Tx type, we define an icaTxHandler so that we have `icaRegisterTxHandler` and `icaExecuteTxHandler` which serves as the entrypoints for the interchain account module functionalities. 
Both tx handlers must verify, that the signer is the `icaOwnerAddress` and then on the type of message, they must construct the related kind of packet and call the associated functions. 

```typescript
function icaRegisterTxHandler(portId: string, channelId: string, icaOwnerAddress: string, hostAccountNumber: unit64)returns (err) {

  // Esnure the tx has been dispatched to the correct handler
  abortTransactionUnless(Tx.type===REGISTER_TX)
  // verify Tx Signature:: must be signed by icaOwnerAddress
  abortTransactionUnlesss(IsValidAddress(icaOwnerAddress))
  abortTransactionUnless(this.Tx.signer===icaOwnerAddress)
  // Validate functions parameter.. 

  _,err= sendRegisterTx(portId: string, channelId: string, icaOwnerAddress: string, hostAccountNumber: unit64) 
  abortTransactionUnless(err!==nil)
  return err 

}
```

```typescript
function icaExecuteTxHandler(portId: string, channelId: string, icaOwnerAddress: string, hostAccountSequenceNumbers:[] unit64, msgs: []msgs ){
  // Verify Tx Signature:: must be signed by icaOwnerAddress
  // validate functions parameters, 
  // call sendExecuteTx 
}
```

#### Sub-protocols

The sub-protocols described herein must be implemented in a controlling "interchain account" module with access to the IBC routing module. 

##### **RegisterInterchainAccount**

`RegisterInterchainAccount` is the entry point function to register an interchain account. In the case the controller chain wants to register new interchain accounts will send a Tx which type is `REGISTER_TX` including the parameter `hostAccountNumber` that will trigger the `RegisterInterchainAccount` function logic to be executed on the controller chain. The function will be executed using the moduleState `nextHostAccountSequenceNumber` parameter and will initialize the map of `hostAccounts` between the tuple, (portId, channelId, ownerAccount, hostAccountSequence) and the hostAcccount maintained by the controller chain in the moduleState.

//The `RegisterInterchainAccount` Must include the ownerAccount address and the channelIdentifier which is meant to operate on. 

The host chain Must be able to generate and store in state the hostAccount.address, that will be controlled by the ownerAccount, by using the information provided about the hostAccounts passed in the `REGISTER_TX` and must pass back the generated address inside the ack. Once received the ack, the controller chain must store the hostAccount.address generated address in the mapping previously described. 

// TODO 

```typescript
function RegisterInterchainAccount(
  portId: Identifier, // Should be strings instead? 
  channelId: Identifier,
  icaOwnerAddress: string,
  hostAccountNumber: uint64 // The number of accounts the icaOwnerAddress wants to register within a single tx 
  ) 
  returns ([]hostSequenceNumbers,err) {
  
  for i in 0..hostAccountNumber{
    // Use the module state nextHostAccountSequenceNumber to initialize the account 
    err=InitInterchainAccountAddress(portId, channelId, icaOwnerAddress,  
    nextHostAccountSequenceNumber)
    abortTransactionUnless(err!==nil)
    hostSequenceNumbers.push(nextHostAccountSequenceNumber)  
    nextHostAccountSequenceNumber=nextHostAccountSequenceNumber+1  
  }

  return hostSequenceNumbers,nil 
}
```

##### **AuthenticateTx**

In the contrlling part of the protocol, `AuthenticateTx` is meant to be used to verify that the expected signer of the tx is the actual `icaOwnerAddress`. 

// NOT USED FOR NOW 

```typescript
function AuthenticateTx(msg Any, icaOwnerAddress string) returns (error) {
  msgSigner=msg.getSigner() // Verify proper syntax 
  abortTransactionUnless(msgSigner==icaOwnerAddress)
}
```

##### **RecoverInterchainAccount**

// MOVE AT THE END 

Since we are allowing only unordered channels and disallowing channel closure procedures, we don't need a procedure for recovering. Channel cannot be closed or get stucked. 

### Packet relay

`SendRegisterTx` and `SendExecuteTx` must be called by a transaction handler in the controller chain module which performs appropriate signature checks. In particular the transaction handlers must verify that `icaOwnerAddress` is the actual signer of the tx.

Thinking about a smart contract system, then the system should verify that the tx the user generate to call the `SendRegisterTx` and `SendExecuteTx` contract function has been signed by the `icaOwnerAddress`.  

`SendRegisterTx` is used by a controller chain to send an IBC packet containing instructions on the number of host accounts to create on a host chain for a given interchain account owner. 

```typescript
function SendRegisterTx( 
  sourcePort: string,
  sourceChannel: string,
  icaOwnerAddress: string,
  hostAccountNumber: uint64, // Account number for which we are requesting the generation 
  //memo: string,   // Do we want to allow memo to be used in here? 
  
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
  let usedHostAccountSequences : []uint64 = []

  abortTransactionUnless(hostAccountNumber > 0)
  
  // A single registration message enable the registration of n accounts. 
  // A potential limit of the number of accounts to guarantee a non out-of-gas error is to be discussed
  usedHostAccountSequences,err = RegisterInterchainAccount(sourcePort, sourceChannel, icaOwnerAddress,hostAccountNumber)
  
  abortTransactionUnless(err!=nil)     
  
  //SendRegisterTx has been called by a transaction handler in the controller chain module which has performed appropriate signature checks for the icaOwnerAddress such that we can assume the tx has already been validated for being originated by the icaOwnerAddress

  icaPacketData = icaRegistrationPacketData{icaOwnerAccount,usedHostAccountSequences}
  
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

`SendExecuteTx` is used by a controller chain to send an IBC packet containing instructions (messages) and the host accounts on which the tx should be executed on a host chain for a given interchain account owner. 

// TODO 

```typescript
function SendExecuteTx(){
  icaPacketData = IcaExecutePacketData{icaOwnerAccount,accounts,msgs,memo}
  
  // Retrieve all the hostAccounts on which we want to execute Tx on. 
  // Note that here we may have hostAccounts previosly registered so that have the hostAccountAddress set, and hostAccounts which address is still empty, since they have just started the registration procedure. 

  let accounts : [] hostAccount = []
  
  for j in 0..nextHostAccountSequenceNumber {
    accounts.push(GetInterchainAccount(icaOwnerAddress,j))
  }
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

Note that interchain accounts controller modules should not execute any logic upon packet receipt, i.e. the `OnRecvPacket` callback should not be called, and in case it is called, it should simply return an error acknowledgement:

```typescript
// Called on Controller Chain by Relayer
function onRecvPacket(packet Packet) {
  return NewErrorAcknowledgement(ErrInvalidChannelFlow)
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
// Called on Controller Chain by Relayer
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes
) {
  data=packet.Data 
  switch packet.type: 
  case types.REGISTER_TX:
    // TODO CHECK HOW TO USE ACKN BETTER 

    //hostAccountAddress=acknowledgement.hostAccountAddress
    let i :uint64 = 0
    // Store addresses under the right sequence number. 
    for address in acknowledgement.hostAccountAddress{
      // Store in hostAccounts module state the address returned in the ack  
      hostAccounts[packet.destinationPort][packet.destinationChannel][data.icaOwnerAddress][data.hostSequenceNumbers.getElement(i)].hostAccountAddress=address
      // Increment the positional number 
      i=i+1 
    }
  // call underlying app's OnAcknowledgementPacket callback 
  // see ICS-30 middleware for more information
  case types.EXECUTE_TX: 
    //TODO 
  }
```

```typescript
// Called on Controller Chain by Relayer
function onTimeoutPacket(packet: Packet) {
  // call underlying app's OnTimeoutPacket callback 
  // see ICS-30 middleware for more information
}
```

### *icaHosting*

#### Port & channel setup

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

### Data Structres

// TODO 

```typescript
interface blacklistedMessages{
  portId: Identifier, 
  channelId: Identifier, 
  msgs: []Any, // Msg Type?  
}
```

### Module State Host Chain

The interchain account module on host chain tracks the blacklist of msgs associated with particular contoller account in state. Fields of the `ModuleState` are assumed to be in scope.

// NOTE Need to inspect how to TODO blacklist mechanisms. Do the controller chain need to know which msg are blacklisted? Probably not. Thus the blacklist couldbe a module state in host chain that can be modified by an ad hoc 
function 

// Should be the blacklist icaOwnerAccounts specific? 
// Not necessary to maintain the hostAccounts here. 

```typescript
interface ModuleState {
  hostAccounts: Map<portId, Map <channelId, Map <icaOwnerAddress, Map<hostAccountSequenceNumber, hostAccount>>>>,
  //hostAccounts: Map<hostAccount.sequenceNumber, hostAccount.address>,
  //TODO blacklist: [] blacklistedMessages 
}
```

#### Utility functions

// TODO These functions have to be adapted in case we will use it 

```typescript

/* NOT USED, COULD BE USED IF WE WANT SEPARATION 
function GenerateInterchainAccountAddress(portId: string, channelId: string, icaOwnerAddress:string, hostAccountSequenceNumber:uint64) returns (address,error) {
  // Initialize in module mapping to hostAccount Data 
  // Address set to empty in intialization. Will be generated by the host chain and returned in the acknowledgment. 
  hostAccounts[portId][channelId][icaOwnerAddress][hostAccountSequenceNumber].hostAccountAddress=""
  // Depend on 4.1 hostAccounts[icaOwnerAddress][hostAccountSequenceNumber].TxSequence=0

  return nil 
}

*/
// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(portId: string, channelId: string, icaOwnerAccount: string,  hostAccountSequenceNumber: uint64, address: string) returns (string) {

hostAccounts[portId][channelId][icaOwnerAccount][hostAccountSequenceNumber].hostAccountAddress=address

return address 
}

// Retrieves the interchain account from state.
function GetInterchainAccountAddress(icaOwnerAddress: string, hostAccountSequenceNumber:uint64) returns (hostAccount){

return hostAccounts[icaOwnerAddress][hostAccountSequenceNumber]

}
```

#### Sub-protocols

The sub-protocols described herein must be implemented in a hosting "interchain account" module with access to the IBC routing module. 

##### **RegisterInterchainAccount**

`RegisterInterchainAccount` may be called during the `OnReceive` callback when a `MsgRegisterInterchainAccount` is relayed to the host chain and contains the parameter `hostAccountNumber`.

// Complete This. Then Write Send Tx, then Write authenticate Tx, then write callbacks for full flow. 

```typescript
function RegisterInterchainAccount(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortId: Identifier, 
  counterpartyChannelIdentifier: Identifier,
  icaOwnerAccount: string,
  usedSequences: []uint64 
  ) returns ([] string) { // Coudl be a map hostAccounts= map seq -> address 

  // validate port format
  abortTransactionUnless(portIdentifier=="ica") 
  // retrieve channel 
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  // validate that the channel infos
  abortTransactionUnless(isActive(channel))
  abortTransactionUnless(channel.counterpartyPortIdentifier == counterpartyPortIdentifier) 
  abortTransactionUnless(channel.counterpartyChannelIdentifier == counterpartyChannelIdentifier)   

   
  let hostAccounts : [] string = []  
  for seq in usedSequences{
    hostAccounts.push(
      newAddress(channel.counterpartyPortIdentifier, channel.counterpartyChannelIdentifier, icaOwnerAccount, seq))
    
    // newAddress MUST generate deterministically the account and should return Map<uint64,address>
  }
  return hostAccounts
}
```

##### **AuthenticateTx**

In the hosting part of the protocol, `AuthenticateTx` is meant to be used to verify that the expected signer of the msgs is the actual hostAccountAddress. 

`AuthenticateTx` checks that the expected signer of a particular message is the `hostAccountAddress` provided in the `sendTx`. 

```typescript
function AuthenticateTx(msg Any, hostAccountAddress string) returns (error) {
  msgSigner=msg.getExpectedSigner() // Verify proper syntax 
  abortTransactionUnless(msgSigner==hostAccountAddress)
}
```

// SHOULD BE THE SAME STUFF

##### **ExecuteTx**

Executes each message sent by the owner account on the controller chain.

```typescript
function ExecuteTx(hostAccount: string, msg Any) returns (resultString, error) {
  
  // Signature has already been validated in the sendTx 
  // Execute the msg for the given hostAccount 
  return hostAccount.execute(msg) // Review Syntax
  // return result of transaction
}
```

##### **BlacklistMsg** 

// TODO BlackList Function for Host writing into module state blacklisted msgs

### Packet relay

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
// Called on Host Chain by Relayer
function onRecvPacket(packet Packet) {

  switch packet.type { 

  case types.REGISTER_TX:

    (icaOwnerAccount,hostAccountSequenceNumbers,err) = types.DeserializeTx(Packet.Data)

    if err != nil {
      return NewErrorAcknowledgement(err)
    }

    let newAccounts: Map<uint64,address>
    for seq in hostAccountSequenceNumbers {
        hostAccounts[packet.DestinationPort][packet.DestinationChannel][icaOwnerAddress][seq].hostAccountAddress=RegisterInterchainAccount(
        Packet.DestinationPort,Packet.DestinationChannel,Packet.SourcePort,Packet.SourceChannel, icaOwnerAccount,seq)
        newAccounts[seq].hostAccountAddress=hostAccounts[seq].hostAccountAddress
      }
  
  //icaPacketAcknowledgement = icaExecutePacketSuccess | icaRegistrationPacketSuccess | icaPacketError; 
  icaPacketAcknowledgement ack = icaRegistrationPacketSuccess{newAccounts}
  
  //ack = NewResultAcknowledgement.icaRegistrationPacketSuccess{newAccounts}
  return ack

  case types.EXECUTE_TX:
    // TODO - REDO 
      let resultsData : [] bytes = []
  // Kind of inefficient - needs more thinking 
  if (hostAccountNumber!=nil){
    /* Here we should start from last element. We should ensure the array of hostAccounts is ordered for hostAccountSequenceNumber. If so, if new account have been registered by the same icaOwnerAddress then, since the hostAccountSequenceNumber is incremented sequentially, they should be the latest added elements.   
    */


  // ExecuteTx executes each of the messages contained in the packet.Data 
  for msg in msgs{
    // TODO Include check for blacklisted message. In case present in blacklist skip it. No need to abort, no?
    for account in hostAccount{ 
      resultData, err = ExecuteTx(account.hostAccountAddress, msg)
      if err != nil {
      // TODO think about what happens if some msgs get executed and then one msg returns an error. 
      // Will be everything reverted automatically or shall we think about reversion? 
      // NOTE: The error string placed in the acknowledgement must be consistent across all
      // nodes in the network or there will be a fork in the state machine. 
      return NewErrorAcknowledgement(err)
      }
      resultsData.push(resultData)          
    }
  }
  
  //InterchainAccountPacketAcknowledgement ack = InterchainAccountPacketAcknowledgement{resultData, null}
  // return acknowledgement containing all the result of the transactions execution on host chain 
  ack = NewResultAcknowledgement(resultsData,hostAccounts,[]byte{byte(1)})
  return ack

  
  default:
    return NewErrorAcknowledgement(ErrUnknownDataType)
  }
  }
```

### Register & controlling flows

// TODO REDO COMPLETELY. PROVIDE DIAGRAMs FOR FLOWS

[imageUrl](https://excalidraw.com/#json=BfGp0ZbDAhO_LWiNmdGGn,Mgw4FrmorzuGjM0alzLMWw
)

## Example Implementations

// TODO 

- Implementation of ICS 27 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

## Future Improvements

// TODO Mention IBC Lite 

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
    
## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
