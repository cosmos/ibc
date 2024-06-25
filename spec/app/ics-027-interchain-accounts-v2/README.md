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
   
### Features // Appoggio , will be deleted

1. Configuration 
1.1 The host chain should accept all message types by default and maintain a blacklist of message types it does not permit 
// Check blacklist on hosting-authenticateTx

2. Registration 
2.1 The controller of the interchain account must have authority over the account on the host chain to execute messages
2.2	A registered interchain account can be any account type supported by x/accounts

3. Control
3.01	The channel type through which a controller sends transactions to the host should be unordered // Channel setup only Unorderd

3.02	The message execution order should be determined at the packet level	
3.03	Many controllers can send messages to many host accounts through the same channel	
3.04	The controller of the interchain account should be able to receive information about the balance of the interchain account in the acknowledgment after a transaction was executed by the host	
3.05	The user of the controller should be able to receive all the information contained in the acknowledgment without implementing additional middleware on a per-user basis	
// Mmmm
3.06	Callbacks on the packet lifecycle should be supported by default	
3.07	A user can perform module safe queries through a host chain account and return the result in the acknowledgment
4. Host Execution
4.1 It should be possible to ensure a packet lifecycle from a different application completes before a message from a controller is executed
4.2 It should be possible for a controller to authorise a host account to execute specific actions on a host chain without needing a packet round trip each time (e.g. auto-compounding)
5. Performance 
5.1 The number of packet round trips to register an account, load the account with tokens and execute messages on the account should be minimised. 
// NOTE In theory we can achieve this by using a list of msgs that are passed to send Tx.
// TO BE DEFINED if we need to maintain a certain order of msgs for security reasons. 

## Technical specification

### Data Structures

The `hostAccount` interface contains the `portId`, the `channelId`, and the `hostAccountSequenceNumber` parameters that in addition to the `icaOwnerAddress`, are used for the hostAccount generation and identification. The `hostAccountSequenceNumber` starts at 0 and it is increased linearly as new account get registered. 
Additionally it contains the `hostAccountAddress` that will be returned by the host chain in the acknowledgment, as effect of the account registration procedure, and a `txSequenceNumber` which purpose is to guarantee the order of execution of transactions regarding a single hostAccount.   

```typescript
interface icaOwnerAddress: string
```

```typescript
interface hostAccountSequenceNumber: uint64 
```

```typescript
interface hostAccount{
  //portId: Identifier, // Really needed?
  //channelId: Identifier, // Really needed?
 // hostAccountSequenceNumber: uint64, 
  hostAccountAddress: string,
  //txSequenceNumber: uint64 To discuss 4.1 // the struct can be helpfull if need this   
}
```

```typescript
interface icaTxResult{
  resultData: [] bytes, 
  //result: "AQ==",
}
```

// TODO 

```typescript
interface blacklistedMessages{
  portId: Identifier, 
  channelId: Identifier, 
  msgs: []Any, // Msg Type?  
}
```

### Packet Data

`InterchainAccountPacketData` contains an array of messages, an array of hostAccounts that will execute the list of messages and a memo string that is sent to the host chain. 

```typescript
interface icaExecutePacketData {
  icaOwnerAddress : string, 
  hostAccounts: [] hostAccount, 
  msgs: [] Any, //msg 
  memo: string,
  hostAccountNumber:uint64 // Optional Sentinel value. If provided a registration procedure is triggered.   
}
```




The acknowledgement data type describes whether the interchain account actions succeeded or failed, and the reason for failure (if any). Additionally, in the success case, a vector of bytes `resultData`, containing the return values of each message execution, is returned. 

```typescript
type icaPacketAcknowledgement = icaPacketSuccess | icaPacketError;  

interface icaPacketSuccess {
  results : [] icaTxResult, 
  hostAccounts: []hostAccount,  
}

interface icaPacketError {
  error: string
}
```

### Msgs 

The ics27-2 protocol supports two types of messages, namely `MsgRegisterInterchainAccount` which triggers the execution of the registering flow and `MsgSendTx` that triggers the execution of the controlling flow.  

// TODO Reason about encoding  // This must be included in channel handshake 

Note that both the `InterchainAccountPacketData` as well as `InterchainAccountPacketAcknowledgement` must be JSON-encoded (not Protobuf encoded) when they serialized into packet data. Also note that `uint256` is string encoded when converted to JSON, but must be a valid decimal number of the form `[0-9]+`.

### Module State Controller Chain

The interchain account module keeps track of the controlled hostAccounts associated with particular channels in state and the nextHostSequenceNumber.  Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  hostAccounts: Map<portId, Map <channelId, Map <icaOwnerAddress, Map<hostAccountSequenceNumber, hostAccount>>>>,
  nextHostAccountSequenceNumber : uint64  
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
  blacklist: [] blacklistedMessages 
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "interchain account" module with access to the IBC routing module. Alternatively, *hosting* and *controlling* could be implemented in separate smart contracts. 

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port (owned by the module).

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the interchain account module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

We define a different setup function for each of the subprotocols *Controlling* and *Hosting*. 

##### *Controlling*

```typescript
function setup() {
  capability = routingModule.bindPort("icacontroller", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    // TODO Missing Upgrade Callbacks
    // onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
  claimCapability("port", capability)
}
```

##### *Hosting*

```typescript
function setup() {
  capability = routingModule.bindPort("icahost", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    // TODO Missing Upgrade Callbacks
    onRecvPacket,
   // onTimeoutPacket,
   // onAcknowledgePacket,
  })
  claimCapability("port", capability)
}
```

#### Routing module callbacks

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

### General design 

The interchain account protocol is composed of two main subprotocols namely *controlling* and *hosting*. A chain can utilize one or both subprotocols of the interchain account protocol. A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 
  
This specification defines the general way to send tx bytes from a controller chain, on an already established ica-channel, to be executed on behalf of the owner account on the host chain. The host chain is responsible for deserializing and executing the tx bytes and the controller chain must know how the host chain will handle the tx bytes in advance of sending a packet, thus this must be negotiated during channel creation.

#### Controlling 

##### **RegisterInterchainAccount**

`RegisterInterchainAccount` is the entry point function to register an interchain account. In the case the controller chain wants to register new interchain accounts will generate a new msg `MsgRegisterInterchainAccount` including the parameter `hostAccountNumber` that will trigger the `RegisterInterchainAccount` function logic to be executed on the controller chain. The function will be executed using the moduleState `nextHostAccountSequenceNumber` parameter and will initialize the map of `hostAccounts` between the tuple, (portId, channelId, ownerAccount, hostAccountSequence) and the hostAcccount maintained by the controller chain in the moduleState.

//The `RegisterInterchainAccount` Must include the ownerAccount address and the channelIdentifier which is meant to operate on. 

The host chain Must be able to generate and store in state the hostAccount, that will be controlled by the ownerAccount, by using the information provided about the hostAccounts passed in the `ExecuteMsg` message and must pass back the generated address inside the ack. Once received the ack, the controller chain must store the hostAccount generated address in the mapping previously described. 

// TODO 

```typescript
function RegisterInterchainAccount(
  portId: Identifier, // Should be strings instead? 
  channelId: Identifier,
  counterpartyPortId: Identifier,  
  counterpartyChannelId: Identifier, 
  icaOwnerAddress: string,
  hostAccountNumber: uint64 // The number of accounts the icaOwnerAddress wants to register within a single tx 
  ) 
  returns ([]hostSequenceNumber,error) {
  
  // validate port format
  abortTransactionUnless(portIdentifier=="ica") // Eventually icacontroller
  // retrieve channel 
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  // validate that the channel infos
  abortTransactionUnless(isActive(channel))
  abortTransactionUnless(channel.counterpartyPortIdentifier == counterpartyPortIdentifier) 
  abortTransactionUnless(channel.counterpartyChannelIdentifier == counterpartyChannelIdentifier)   
  
  let usedSequences: [] uint64 =[]

  for i in 0..hostAccountNumber{
    // Use nextHostAccountSequenceNumber to initialize the account 
    err=InitInterchainAccountAddress(icaOwnerAddress, portId, channelId, 
    nextHostAccountSequenceNumber)
    abortTransactionUnless(err!==nil)
    usedSequence.push(nextHostAccountSequenceNumber)  
    nextHostAccountSequenceNumber=nextHostAccountSequenceNumber+1  
  }

  return usedSequences,nil 
```



##### **RecoverInterchainAccount**

Since we are allowing only unordered channels and disallowing channel closure procedures, we don't need a procedure for recovering. 

#### Hosting

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

  // Is it necessary to maintain a mapping on the host chain for the generated account? 
  
  let hostAccounts : [] string = []  
  for seq in usedSequences{
    hostAccounts.push(
      newAddress(counterpartyPortIdentifier, counterpartyChannelIdentifier, icaOwnerAccount, hostAccountSequence))
    
    // Since newAddress should generate deterministically the account, calling twice new address with the same exact
    // parameters should return the same address 
    // If we need to keep it in state in the host then we may use logic with setInterchainAccountAddress and 
    // getInterchainAccountAddress
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

#### Utility functions

// TODO These functions have to be adapted in case we will use it 

```typescript
// Sets the active channel for a given portID and connectionID.
function SetActiveChannelID(portId: Identifier, connectionId: Identifier, channelId: Identifier) returns (error){
}

// Returns the ID of the active channel for a given portID and connectionID, if present.
function GetActiveChannelID(portId: Identifier, connectionId: Identifier) returns (Identifier, boolean){
}

function InitInterchainAccountAddress(icaOwnerAddress:string, portId: Identifier, channelId: Identifier, hostAccountSequenceNumber:uint64) returns (error) {
  // Initialize in module mapping to hostAccount Data 
  hostAccounts[icaOwnerAddress][hostAccountSequenceNumber].portId=portId
  hostAccounts[icaOwnerAddress][hostAccountSequenceNumber].channelId=channelId
  hostAccounts[icaOwnerAddress][hostAccountSequenceNumber].TxSequence=0
  // Address set to empty in intialization. Will be generated by the host chain and returned in the acknowledgment. 
  hostAccounts[icaOwnerAddress][hostAccountSequenceNumber].hostAccountAddress="" 

}

// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(icaOwnerAccount: string,  hostAccountSequenceNumber: uint64, address: string) returns (hostAccount) {

hostAccounts[icaOwnerAccount][hostAccountSequenceNumber].hostAccountAddress=address

}

// Retrieves the interchain account from state.
function GetInterchainAccountAddress(icaOwnerAddress: string, hostAccountSequenceNumber:uint64) returns (hostAccount){

return hostAccounts[icaOwnerAddress][hostAccountSequenceNumber]

}
```

### Packet relay

`SendTx` is used by a controller chain to send an IBC packet containing instructions (messages) to an interchain account on a host chain for a given interchain account owner. 

`SendTx` must be called by a transaction handler in the controller chain module which performs appropriate signature checks. In particular it must verify that `icaOwnerAddress` is the actual signer of the tx.

Thinking about a smart contract system, then the system should verify that the tx the user generate to call the `SendTx` contract function has been signed by the `icaOwnerAddress`.  

```typescript
function MsgSendTxHandler(msg: MsgSendTx){
  // MsgSendTx{Msgs: []byte, portId, channelId, ICA_OWNER_ADDR, HOST_ACCOUNT_SEQUENCE_NUM}
  // Verify msg has been signed by icaOwnerAddress 
  // Call SendTx(..fill with parameter from msgSendTx)
}
```

```typescript
function SendTx(
  icaOwnerAddress: string,
  msgs: []msg, // messages array  
  memo: string,   
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64, // in unix nanoseconds
  hostAccountNumber: uint64 // Optional Parameter. If is set a registration procedure is triggered. 
) returns (uint64) {

  // validate icaOwnerAccount
  abortTransactionUnlesss(IsValidAddress(icaOwnerAddress))
  // retrieve channel 
  channel = provableStore.get(channelPath(sourcePort, sourceChannel))
  abortTransactionUnless(channel.version=="ica")
  // validate timeoutTimestamp
  abortTransactionUnless(timeoutTimestamp <= currentTimestamp())
  // validate Height? 

  let err : error = nil
  
  if(hostAccountNumber > 0){
      // A single registration message enable the registration of n accounts. 
      // A potential limit of the number of accounts to guarantee a non out-of-gas error is to be discussed
      err = RegisterInterchainAccount(sourcePort, sourceChannel, channel.counterpartyPortID,channel.counterpartyChannelID, icaOwnerAddress,hostAccountNumber)
      abortTransactionUnless(err!=nil)     
  }
  
  //SendTx has been called by a transaction handler in the controller chain module which has performed appropriate signature checks for the icaOwnerAddress such that we can assume the tx has already been validated for being originated by the icaOwnerAddress

  // Retrieve all the hostAccounts on which we want to execute Tx on. 
  // Note that here we may have hostAccounts previosly registered so that have the hostAccountAddress set, and hostAccounts which address is still empty, since they have just started the registration procedure. 

  let accounts : [] hostAccount = []
  
  for j in 0..nextHostAccountSequenceNumber {
    accounts.push(GetInterchainAccount(icaOwnerAddress,j))
  }
  
  icaPacketData = InterchainAccountPacketData{icaOwnerAccount,accounts,msgs,memo}
  
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

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
// Called on Host Chain by Relayer
function onRecvPacket(packet Packet) {

  //InterchainAccountPacketAcknowledgement ack = InterchainAccountPacketAcknowledgement{true, null}  
  //ack = NewResultAcknowledgement([]byte{byte(1)})

  (icaOwnerAccount,hostAccounts,msgs,memo,hostAccountNumber,err) = types.DeserializeTx(Packet.Data)

  if err != nil {
    return NewErrorAcknowledgement(err)
  }

  let resultsData : [] bytes = []
  // Kind of inefficient - needs more thinking 
  if (hostAccountNumber!=nil){
    /* Here we should start from last element. We should ensure the array of hostAccounts is ordered for hostAccountSequenceNumber. If so, if new account have been registered by the same icaOwnerAddress then, since the hostAccountSequenceNumber is incremented sequentially, they should be the latest added elements.   
    */
    for account in hostAccounts {
      if (account.hostAccountAddress==""){
        hostAccounts[icaOwnerAddress,account.hostAccountSequenceNumber].hostAccountAddress=RegisterInterchainAccount(
        Packet.DestinationPort,Packet.DestinationChannel,Packet.SourcePort,Packet.SourceChannel, icaOwnerAccount,account.hostAccountSequenceNumber)
      }else{
        break 
      }
  }

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
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
// Called on Controller Chain by Relayer
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes
) {
  // call underlying app's OnAcknowledgementPacket callback 
  // see ICS-30 middleware for more information
}
```

```typescript
// Called on Controller Chain by Relayer
function onTimeoutPacket(packet: Packet) {
  // call underlying app's OnTimeoutPacket callback 
  // see ICS-30 middleware for more information
}
```

Note that interchain accounts controller modules should not execute any logic upon packet receipt, i.e. the `OnRecvPacket` callback should not be called, and in case it is called, it should simply return an error acknowledgement:

```typescript
// Called on Controller Chain by Relayer
function onRecvPacket(packet Packet) {
  return NewErrorAcknowledgement(ErrInvalidChannelFlow)
}
```

### Custom logic

ICS-27 relies on [ICS-30 middleware architecture](../ics-030-middleware) to provide the option for application developers to apply custom logic on the success or fail of ICS-27 packets. 

Controller chains will wrap `OnAcknowledgementPacket` & `OnTimeoutPacket` to handle the success or fail cases for ICS-27 packets. 

### Register & controlling flows

// TODO REDO COMPLETELY. PROVIDE DIAGRAMs FOR FLOWS

#### Register account flow

To register an interchain account we require an off-chain process (relayer) to listen for `ChannelOpenInit` events with the capability to finish a channel creation handshake on a given connection. 

1. The controller chain binds a new IBC port with the controller portID for a given *interchain account owner address*.

This port will be used to create channels between the controller & host chain for a specific owner/interchain account pair. Only the account with `{owner-account-address}` matching the bound port will be authorized to send IBC packets over channels created with the controller portID. It is up to each controller chain to enforce this port registration and access on the controller side. 

2. The controller chain emits an event signaling to open a new channel on this port given a connection. 
3. A relayer listening for `ChannelOpenInit` events will continue the channel creation handshake.
4. During the `OnChanOpenTry` callback on the host chain an interchain account will be registered and a mapping of the interchain account address to the owner account address will be stored in state (this is used for authenticating transactions on the host chain at execution time). 
5. During the `OnChanOpenAck` callback on the controller chain a record of the interchain account address registered on the host chain during `OnChanOpenTry` is set in state with a mapping from (controller portID, controller connectionID) -> interchain account address. See [metadata negotiation](#metadata-negotiation) section below for how to implement this.
6. During the `OnChanOpenAck` & `OnChanOpenConfirm` callbacks on the controller & host chains respectively, the [active-channel](#active-channels) for this interchain account/owner pair, is set in state.

#### Controlling flow

Once an interchain account is registered on the host chain a controller chain can begin sending instructions (messages) to the host chain to control the account. 

1. The controller chain calls `SendTx` and passes message(s) that will be executed on the host side by the associated interchain account (determined by the controller side port identifier)

Cosmos SDK pseudo-code example:

```golang
// connectionId is the identifier for the controller connection
interchainAccountAddress := GetInterchainAccountAddress(portId, connectionId)
msg := &banktypes.MsgSend{FromAddress: interchainAccountAddress, ToAddress: ToAddress, Amount: amount}
icaPacketData = InterchainAccountPacketData{
  Type: types.EXECUTE_TX,
  Data: serialize(msg),
  Memo: "memo",
}

// Sends the message to the host chain, where it will eventually be executed 
SendTx(ownerAddress, connectionId, portID, data, timeout)
```

2. The host chain upon receiving the IBC packet will call `DeserializeTx`. 
    
3. The host chain will then call `AuthenticateTx` and `ExecuteTx` for each message and return an acknowledgment containing a success or error.  

Messages are authenticated on the host chain by taking the controller side port identifier and calling `GetInterchainAccountAddress(controllerPortId, hostConnectionId)` to get the expected interchain account address for the current controller port and connection identifier. If the signer of this message does not match the expected account address then authentication will fail.

#### Active channels 

// TODO verify if needed and what's needed

The controller and host chain must keep track of an `active-channel` for each registered interchain account. The `active-channel` is set during the channel creation handshake process. This is a safety mechanism that allows a controller chain to regain access to an interchain account on a host chain in case of a channel closing. 

An example of an active channel on the controller chain can look like this:

```typescript
{
  // Controller Chain
  SourcePortId: `icacontroller-<owner-account-address>`,
  SourceChannelId: `<channel-id>`,
  // Host Chain
  CounterpartyPortId: `icahost`,
  CounterpartyChannelId: `<channel-id>`,
}
```

In the event of a channel closing, the active channel may be replaced by starting a new channel handshake with the same port identifiers on the same underlying connection of the original active channel. ICS-27 channels can only be closed in the event of a timeout (if the implementation uses ordered channels) or in the unlikely event of a light client attack. Controller chains must retain the ability to open new ICS-27 channels and reset the active channel for a particular portID (containing `{owner-account-address}`) and connectionID pair.

The controller and host chains must verify that any new channel maintains the same metadata as the previous active channel to ensure that the parameters of the interchain account remain the same even after replacing the active channel. The `Address` of the metadata should not be verified since it is expected to be empty at the INIT stage, and the host chain will regenerate the exact same address on TRY, because it is expected to generate the interchain account address deterministically from the controller portID and connectionID (both of which must remain the same).

// For example here, we may just use the proof of the channel closure and thus provide a new channel. This may need to update the channel on every interchain record. Should be possible to do it in bulk, to respect block limits. 

#### **Metadata negotiation**

// TODO verify if needed and what's needed

ICS-27 takes advantage of [ICS-04 channel version negotiation](../../core/ics-004-channel-and-packet-semantics/README.md#versioning) to negotiate metadata and channel parameters during the channel handshake. The metadata will contain the encoding format along with the transaction type so that the counterparties can agree on the structure and encoding of the interchain transactions. The metadata sent from the host chain on the TRY step will also contain the interchain account address, so that it can be relayed to the controller chain. At the end of the channel handshake, both the controller and host chains will store a mapping of (controller chain portID, controller/host connectionID) to the newly registered interchain account address ([account registration flow](#register-account-flow)). 

ICS-04 allows for each channel version negotiation to be application-specific. In the case of interchain accounts, the channel version will be a string of a JSON struct containing all the relevant metadata intended to be relayed to the counterparty during the channel handshake step ([see summary below](#metadata-negotiation-summary)).

Combined with the one channel per interchain account approach, this method of metadata negotiation allows us to pass the address of the interchain account back to the controller chain and create a mapping from (controller portID, controller connection ID) -> interchain account address during the `OnChanOpenAck` callback. As outlined in the [controlling flow](#controlling-flow), a controller chain will need to know the address of a registered interchain account in order to send transactions to the account on the host chain.

#### **Metadata negotiation summary**

`interchain-account-address` is the address of the interchain account registered on the host chain by the controller chain.

- **INIT**

Initiator: Controller

Datagram: ChanOpenInit

Chain Acted Upon: Controller

Version: 

```json
{
  "Version": "ics27-1",
  "ControllerConnectionId": "self_connection_id",
  "HostConnectionId": "counterparty_connection_id",
  "Address": "",
  "Encoding": "requested_encoding_type",
  "TxType": "requested_tx_type",
}
```

Comments: The address is left empty since this will be generated and relayed back by the host chain. The connection identifiers must be included to ensure that if a new channel needs to be opened (in case active channel times out), then we can ensure that the new channel is opened on the same connection. This will ensure that the interchain account is always connected to the same counterparty chain.

- **TRY**

Initiator: Relayer

Datagram: ChanOpenTry

Chain Acted Upon: Host

Version: 

```json
{
  "Version": "ics27-1",
  "ControllerConnectionId": "counterparty_connection_id",
  "HostConnectionId": "self_connection_id",
  "Address": "interchain_account_address",
  "Encoding": "negotiated_encoding_type",
  "TxType": "negotiated_tx_type",
}
```

Comments: The ICS-27 application on the host chain is responsible for returning this version given the counterparty version set by the controller chain in INIT. The host chain must agree with the single encoding type and a single tx type that is requested by the controller chain (ie. included in counterparty version). If the requested encoding or tx type is not supported, then the host chain must return an error and abort the handshake.
The host chain must also generate the interchain account address and populate the address field in the version with the interchain account address string.

- **ACK** 

Initiator: Relayer

Datagram: ChanOpenAck

Chain Acted Upon: Controller

CounterpartyVersion: 

```json
{
  "Version": "ics27-1",
  "ControllerConnectionId": "self_connection_id",
  "HostConnectionId": "counterparty_connection_id",
  "Address": "interchain_account_address",
  "Encoding": "negotiated_encoding_type",
  "TxType": "negotiated_tx_type",
}
```

Comments: On the ChanOpenAck step, the ICS27 application on the controller chain must verify the version string chosen by the host chain on ChanOpenTry. The controller chain must verify that it can support the negotiated encoding and tx type selected by the host chain. If either is unsupported, then it must return an error and abort the handshake.
If both are supported, then the controller chain must store a mapping from the channel's portID to the provided interchain account address and return successfully.

### Identifier formats

// TODO verify if needed and what's needed

These are the default formats that the port identifiers on each side of an interchain accounts channel. The controller portID **must** include the owner address so that when a message is sent to the controller module, the sender of the message can be verified against the portID before sending the ICA packet. The controller chain is responsible for proper access control to ensure that the sender of the ICA message has successfully authenticated before the message reaches the controller module.

Controller Port Identifier: optional prefix `icacontroller-` + mandatory `{owner-account-address}`

Host Port Identifier: `icahost`

The `icacontroller-` prefix on the controller port identifier is optional and host chains **must** not enforce that the counterparty port identifier includes it. Controller chains may decide to include it and validate that it is present in their own port identifier.

## Example Implementations

- Implementation of ICS 27 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

## Future Improvements

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
// TO BE REMOVED: 
##### **AuthenticateTx**

In the controlling part of the protocol, `AuthenticateTx` is meant to be used to verify that the tx arriving to the msgServer has been generated and signed by the actual `icaOwnerAddress`. 

// Option 1 
So `AuthenticateTx` checks that the decoded signedHash (signed by `icaOwnerAddress`) of the array of msgs is equal to the hash of the array of msgs provided in the `sendTx`. 

```typescript
function AuthenticateTx(msgs []Any, signedHash bytes, icaOwnerAddress string) returns (error) {
  
  hashMsgsList = hash(msgs)
  decodedHashMsgsList = decodeHash(signedHash,icaOwnerAddress) // Use public key to decode the signedHash
  abortTransactionUnless(hashMsgsList==decodedHashMsgsList)

}
```

// Option 2

So `AuthenticateTx` checks that the decoded signedHash (signed by `icaOwnerAddress`) of the array of msgs is equal to the hash of the array of msgs provided in the `sendTx`. 

```typescript
function AuthenticateTx(msgs []Any, signedHash bytes, icaOwnerAddress string) returns (error) {
  
  hashMsgsList = hash(msgs)
  decodedHashMsgsList = decodeHash(signedHash,icaOwnerAddress) // Use public key to decode the signedHash
  abortTransactionUnless(hashMsgsList==decodedHashMsgsList)

}
```