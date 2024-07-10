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

ICS-27 Interchain Accounts outlines a cross-chain account management protocol built upon IBC. ICS-27-enabled chains can programmatically create accounts on other ICS-27-enabled chains & control these accounts via IBC transactions (instead of signing with a private key). Interchain accounts retain all of the capabilities of a normal account (i.e. stake, send, vote) but instead are managed by a separate chain via IBC in a way such that the owner account on the controller chain retains full control over any interchain account(s) it registers on host chain(s). 

#### Version 2 

The ICS-27 version 2 comes to address the needs of the community and adheres to the requirements defined [here](https://github.com/cosmos/ibc-go/blob/48f69848bb84d9bc396c750eb656f961c7d773ad/docs/requirements/ics27-multiplexed-requirements.md). 

### Definitions 

- `Host Chain`: The chain where the interchain account is registered. The host chain listens for IBC packets from a controller chain which contain instructions (e.g. cosmos SDK messages) that the interchain account will execute.
- `Controller Chain`: The chain registering and controlling an account on a host chain. The controller chain sends IBC packets to the host chain to control the account.
- `Interchain Account`: An account on a host chain. An interchain account has all the capabilities of a normal account. However, rather than signing transactions with a private key, a controller chain will send IBC packets to the host chain which signals what transactions the interchain account must execute. 
- `Interchain Account Owner`: An account on the controller chain. Every interchain account on a host chain has a respective owner account on the controller chain. 

The IBC handler interface & IBC relayer module interface are as defined in [ICS-25](../../core/ics-025-handler-interface) and [ICS-26](../../core/ics-026-routing-module), respectively.

### Desired properties

- Permissionless: An interchain account may be created by any actor without the approval of a third party (e.g. chain governance). Note: Individual implementations may implement their own permissioning scheme, however, the protocol must not require permissioning from a trusted party to be secure.
- Fault isolation: A controller chain must not be able to control accounts registered by other controller chains. For example, in the case of a fork attack on a controller chain, only the interchain accounts registered by the forked chain will be vulnerable.
- The ordering of transactions sent to an interchain account on a host chain must be maintained. Transactions must be executed by an interchain account in the order in which they are sent by the controller chain. 
- The controller chain must store the account address of any owned interchain accounts registered on host chains. 
- A host chain must have the ability to limit interchain account functionality on its chain as necessary (e.g. a host chain can decide that interchain accounts registered on the host chain cannot take part in staking). This should be achieved with a blacklist mechanisms. 
- An icaOwnerAccount on the controller chain can manage 1..n hostAccount(s) on the host chain. A hostAccount on the host chain can be managed by 1 and only 1 icaOwnerAccount on the controller chain. 
- Many controller accounts on the same controller chain should be able to send messages to many host accounts on the same host chain through the same channel.

### General design 

The interchain account protocol defines the relationship and interactions between two chains with different roles: the controller chain and the host chain. The protocol allows a controller chain to create and manage accounts on a host chain programmatically through IBC transactions, bypassing the need for private key signatures.

This specification defines the general way to send TX bytes from a controller chain, on an already established ica-channel, to a host chain. The host chain is responsible for deserializing and executing the tx bytes and the controller chain must know how the host chain will handle the tx bytes in advance of sending a packet, thus this must be negotiated during channel creation.

The ICS-27 version 2 is composed of two subprotocols, namely `icaControlling` and `icaHosting`. The *icaControlling* must be implemented on the controlling chain and is responsible for sending IBC packets to register and manage interchain accounts on the host chain. The *icaHosting* must be implemented on the host chain and is responsible for receiving IBC packets to generate addresses and execute transactions on behalf of the controller chain.

A chain can utilize one or both subprotocols of the interchain account protocol. A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

## Technical specification

### Data Structures

We define two types of interchain account packet data the `icaRegisterPacketData` and the `icaExecutePacketData`. Each of the packet data has its data structure.

Additionally, we define the `icaPacketDataType` that will be used to distinguish between the two types of packets. 

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

The `icaExecutePacketData` contains the parameters `icaOwnerAddress`, the array of `hostAccountIds`, an array of `msgs` and a `memo` that will be passed by the controller chain to the host chain to execute the msgs on the associated account addresses. 

```typescript
interface icaExecutePacketData {
  icaType: icaPacketDataType = EXECUTE,
  icaOwnerAddress : string, 
  hostAccountIds: [] uint64,
  msgs: [] Any, //msg 
  memo: string,  
}
```

ICS-27 version 2 defines four acknowledgment data types, namely `icaExecutePacketSuccess`, `icaRegisterPacketSuccess`,`icaRegisterPacketError`, `icaExecutePacketError`. Each of them stores different values that are used in a different flow of the interchain account protocol.

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

The `icaRegisterPacketSuccess` is defined to handle the successful registration flow. In the success case, the account address generated on the host chain will be stored in the acknowledgment and delivered back to the controller chain.

```typescript
interface icaRegisterPacketSuccess { 
  type: icaPacketAcknowledgementType = REGISTER_SUCCESS,  
  hostAccounts: [] string,  
}
```

The `icaExecutePacketSuccess` is defined to handle the successful execution flow. In the success case the array of bytes `resultData`, containing the ordered return values of each message execution, will be stored in the acknowledgment and delivered back to the controller chain.

```typescript
interface icaExecutePacketSuccess {
  type: icaPacketAcknowledgementType = EXECUTE_SUCCESS,  
  resultData: [] bytes,  
}
```

#### **Metadata negotiation**

The ICS-04 allows for each channel version negotiation to be application-specific. ICS-27 takes advantage of [ICS-04 channel version negotiation](../../core/ics-004-channel-and-packet-semantics/README.md#versioning) to negotiate metadata and channel parameters during the channel handshake. In the case of interchain accounts, the channel version will be a string of a JSON struct containing all the relevant metadata intended to be relayed to the counterparty during the channel handshake steps. The metadata used for the ICS-27 version 2 will contain the encoding format along with the channel version itself.

```typescript
version: {
  "Version": "ics27-v2", // channel version
  "Encoding": "requested_encoding_type", // Json, protobuf.. 
}
```

// Note. For now, may be ok like this, but eventually, we can make the encoding a channelEnd native parameter.

### Transaction Types 

The ICS-27 version 2 defines two types of transactions `REGISTER_TX` and `EXECUTE_TX`.

```typescript
enum icaTxTypes {
  REGISTER_TX,
  EXECUTE_TX
}
```

### Sub-protocols

#### **icaControlling**

##### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port (owned by the module).

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the interchain account module on separate chains.

An administrator (with the permissions to create connections & channels on the state machine) is responsible for setting up connections to other state machines & creating channels to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only and defines them in such a fashion that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

```typescript
function setup() {
  capability = routingModule.bindPort("ica-controller", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,  // Force Abort 
    onChanOpenAck,
    onChanOpenConfirm, // Force Abort 
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

##### Routing module callbacks

The routing module callbacks associated with the channel management are at the base of the interchain account protocol. By design, only the controller chain can start a channel creation handshake. 

###### Channel lifecycle management

When machine `A`, with the role of controller, starts a new channel handshake, chain `B` with the role of host must accept the new channel if and only if:

- The channel being created is `UNORDERED`.
- The counterpartyMetatada.Version string is `ics27-v2`.  
- The counterpartyPortIdentifier is `ica-controller`.  

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
  abortTransactionUnless(portIdentifier === "ica-controller")
  abortTransactionUnless(counterpartyPortIdentifier === "ica-host")

  if version == "" {
    // default to latest supported version
    metadata = {
      Version: "ics27-v2",
      Encoding: DefaultEncoding, //decide default econding
    }
    version = marshalJSON(metadata)
    return version, nil
  } else{ 
    // If the version is not empty and is among those supported, we return the version
    metadata = UnmarshalJSON(version)
    // assert that version is "ica" 
    abortTransactionUnless(matadata.Version === "ics27-v2")
    // assert the choosed encoding is supported.
    abortTransactionUnless(IsSupportedEncoding(metadata.Encoding))
    return version, nil 
  }
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
  // Always Abort 
  abortTransactionUnless(false, "Invalid channel creation flow: channel handshake must be initiated by controller chain")
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
  counterpartyMetadata = UnmarshalJSON(CounterpartyVersion)
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  channelMetadata=UnmarshalJSON(channel.version)
  abortTransactionUnless(counterpartyMetadata.Version === "ics27-v2")
  // Check if the econding has been agreed 
  abortTransactionUnless(counterpartyMetadata.Encoding === channelMetadata.Encoding)
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // Always abort 
  abortTransactionUnless(false, "Invalid channel creation flow: channel handshake must be initiated by controller chain")
  }
```

```typescript
// Channel closure is disabled.
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // always abort transaction
    abortTransactionUnless(false, "Invalid flow: channel closure is disabled")
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary, this cannot be reached. 
}
```

##### Interchain Account EntryPoints 

To interact with the interchain account protocol, the user can generate two types of tx, namely `REGISTER_TX` and `EXECUTE_TX`. For each Tx type, we define a icaTxHandler so that we have `icaRegisterTxHandler` and `icaExecuteTxHandler`. Both tx handlers must verify, that the signer is the `icaOwnerAddress` and then, based on the `icaTxType`, they must call the associated functions that will construct the related kind of packet. 

```typescript
function icaRegisterTxHandler(portId: string, channelId: string, icaOwnerAddress: string, hostAccountNumber: unit64): uint64 {

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
function icaExecuteTxHandler(portId: string, channelId: string, icaOwnerAddress: string, hostAccountIds:[] unit64, msgs: []msgs, memo:string ) : uint64 {
  
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

##### Module State 

The interchain account module keeps track of the controlled `hostAccounts` associated with particular channels in the state. Additionally, it tracks the `nextHostAccountId` and the `unusedHostAccountsIds` array. Fields of the `ModuleState` are assumed to be in scope.

When the protocol is initiated, the `unusedHostAccountIds` array is empty. So the Ids are initially associated with the `nextHostAccountId` which starts at 0 and is increased linearly as new accounts get registered. When a register message fails, the Ids computed during the sending are recovered and inserted in the `unusedHostAccountIds` array during the acknowlegment. So in the next register message, the Ids in `unusedHostAccountIds` are the first used to generate the new accounts.  
// TODO There is space to easily add a delete account mechanism 

```typescript
interface ModuleState {
  hostAccounts: Map<portId : string, 
    Map <channelId: string, 
    Map <icaOwnerAddress: string, 
    Map <hostAccountId: uint64,
    hostAccountAddress: string>>>>,
  nextHostAccountId : uint64,
  unusedHostAccountIds: []uint64  
}
```

##### Utility functions

```typescript

// Stores the address of the interchain account in state.
function setInterchainAccountAddress(portId: string, channelId: string, icaOwnerAccount: string,  hostAccountId: uint64, address: string) : string {

hostAccounts[portId][channelId][icaOwnerAccount][hostAccountId].hostAccountAddress=address

return address 
}

// Retrieves the interchain account address from state.
function getInterchainAccountAddress(portId: string, channelId: string,icaOwnerAddress: string, hostAccountId: uint64) : string {

return hostAccounts[portId][channelId][icaOwnerAddress][hostAccountId].hostAccountAddress

}
```

##### Helper Functions

The helper functions described herein must be implemented in a controlling interchain account module. 

###### **registerInterchainAccount**

When a user on the controller chain wants to register new interchain accounts, he will send a Tx whose type is `REGISTER_TX`, including the parameter `hostAccountNumber` to specify the number of accounts to register, which will trigger the `registerInterchainAccount` function logic to be executed on the controller chain. The `registerInterchainAccount` function select the `usedHostAccountIds` either from the list of `unusedHostAccountIDs`, if non empty, or from the `nextHostAccountId` parameters in the moduleState. 

The host chain Must be able to generate and store in state the hostAccountAddress, which will be controlled by the icaOwnerAddress by using the information provided about the hostAccounts passed in the `REGISTER_TX` and must pass back the generated address inside the ack. Once received the ack, the controller chain must store the `hostAccountAddress` generated address in its module state. In the case of an error during the registration, the `usedHostAccountIds` will be added to the `unusedHostAccountIDs` array. 

```typescript
function registerInterchainAccount(
  portId: string, 
  channelId: string,
  icaOwnerAddress: string,
  hostAccountNumber: uint64 // The number of accounts the icaOwnerAddress wants to register within a single tx 
  ) 
  : ([]hostAccountIds,err) {

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
    //err=InitInterchainAccountAddress(portId, channelId, icaOwnerAddress, hostAccountId)
    if(getInterchainAccountAddress(portId,channelId,icaOwnerAddress,hostAccountId)!=="") {    
      error={"intechainAccountKeys already used"}
    return ([],error) 
  }
    //abortTransactionUnless(err!==nil)
    // Push into hostSequenceNumber array the used nextHostAccountId
    // We will return this array to keep track of the used sequence numbers  
    hostAccountIds.push(hostAccountId)  
  }

  return hostAccountIds,nil 
}
```

##### Packet relay

`sendRegisterTx` and `sendExecuteTx` must be called by a transaction handler in the controller chain module which performs appropriate signature checks. In particular, the transaction handlers must verify that the `icaOwnerAddress` is the actual signer of the TX.

//TODO CLARIFY WAY BETTER THE CONCEPT  // May be in a different section 
Thinking about a smart contract system, then the system should verify that the tx the user generates to call the `sendRegisterTx` and `sendExecuteTx` contract function has been signed by the `icaOwnerAddress`.  

`sendRegisterTx` is used by a controller chain to send an IBC packet containing instructions on the number of host accounts to create on a host chain for a given interchain account owner. 

```typescript
function sendRegisterTx( 
  sourcePort: string,
  sourceChannel: string,
  icaOwnerAddress: string,
  hostAccountNumber: uint64, // Account number for which we are requesting the generation 
  //memo: string,   // Do we want to allow memo to be used in here? Probably we should not 
  
) : uint64 {

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
  usedHostAccountIds,err = registerInterchainAccount(sourcePort, sourceChannel, icaOwnerAddress,hostAccountNumber)
  
  abortTransactionUnless(err===nil)     
  
  //sendRegisterTx has been called by a transaction handler in the controller chain module which has performed appropriate signature checks for the icaOwnerAddress such that we can assume the tx has already been validated for being originated by the icaOwnerAddress

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

`sendExecuteTx` is used by a controller chain to send an IBC packet containing instructions (messages) and the host accounts references that should execute the tx on behalf of the interchain account owner. 

```typescript
function sendExecuteTx( 
  portId: string,
  channelId: string,
  icaOwnerAddress: string,
  hostAccountIds: [] uint64,  // TODO Reason about this. Maybe could use addresses directly  
  msgs: []msg, 
  memo: string) 
  : uint64 {
  
  // Verify that the provided hostAccountIds match with an already registered hostAccountAddress
  for seq in hostAccountIds{
    abortTransactionUnless(getInterchainAccountAddress(portId,channelId,icaOwnerAddress,seq)!=="", "Interchain Account Not Registered")
    }
  
  // It exist at least one message to be executed 
  abortTransactionUnless(msgs.isNotEmpty())

  icaPacketData = IcaExecutePacketData{icaOwnerAddress,hostAccountsIds,msgs,memo}

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

Note that interchain accounts controller modules should not execute any logic upon packet receipt, i.e. the `onRecvPacket` callback should not be called, and in case it is called, it should simply return an error acknowledgment:

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

Here we define the setup function for the *icaHosting* subprotocol. 

```typescript
function setup() {
  capability = routingModule.bindPort("ica-host", ModuleCallbacks{
    onChanOpenInit, // Force Abort 
    onChanOpenTry,
    onChanOpenAck, // Force Abort 
    onChanOpenConfirm, 
    onChanCloseInit, // Force Abort 
    onChanCloseConfirm, // Do nothing
    // TODO Missing Upgrade Callbacks
    onRecvPacket,
    //onTimeoutPacket, // Force Abort
    //onAcknowledgePacket, // Force Abort      
  })
  claimCapability("port", capability)
}
```

##### Routing module callbacks

The routing module callbacks associated with the channel management are at the base of the interchain account protocol. By design, the host chain can only accept/negotiate a channel creation handshake started by a controller chain. 

###### Channel lifecycle management

When machine `A`, with the role of controller, starts a new channel handshake, chain `B` with the role of host must accept the new channel if and only if:

- The channel being created is `UNORDERED`.
- The counterpartyMetatada.Version string is `ics27-v2`.  
- The counterpartyPortIdentifier is `ica-controller`.  

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
  // Always Abort 
  abortTransactionUnless(false, "Invalid channel creation flow: channel handshake must be initiated by controller chain")
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
  
  abortTransactionUnless(order === UNORDERED)
  // Unmarshal metada from counterpartyVersion
  counterpartyMetadata = UnmarshalJSON(counterpartyVersion)
  
  abortTransactionUnless(portIdentifier === "ica-host")
  abortTransactionUnless(counterpartyPortIdentifier === "ica-controller")
  // assert that version is "ics27-v2" 
  abortTransactionUnless(counterpartyMetadata.Version === "ics27-v2")
  // assert the choosed encoding is supported.
  abortTransactionUnless(IsSupportedEncoding(counterpartyMetadata.Encoding))
  return counterpartyVersion, nil 
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  // Always Abort 
  abortTransactionUnless(false, "Invalid channel creation flow: channel handshake must be initiated by controller chain")
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
  abortTransactionUnless(false, "Invalid flow: channel closure is disabled")
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

##### Module State Host Chain

The interchain account module on the host chain must track the `hostAccounts` and should track the blacklist of msgs associated with a particular controller account in the state. Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  hostAccounts: Map<portId, Map <channelId, Map <icaOwnerAddress, Map<hostAccountId, hostAccount>>>>,
  // Generic msg blacklist. It can be icaOwnerAddress specific, or only portId,channelId specific. 
  msgBlacklist: Map<portId, Map <channelId, Map <icaOwnerAddress, set(msg)>>>,
   
}
```

##### Utility functions

```typescript
// Stores the address of the interchain account in state.
function setInterchainAccountAddress(portId: string, channelId: string, icaOwnerAccount: string,  hostAccountId: uint64) : string {
// While the setInterchainAccountAddress of the controller chain only stores the passed in addresses, in the hosting subprotocol this function has to generate new addresses deterministically based on the passed-in parameters and then store them in the module state in the hostAccounts map.
address=newAddress(portId,channelId, icaOwnerAccount, seq)
// Set in the host chain module state the generated address 
hostAccounts[portId][channelId][icaOwnerAccount][hostAccountId].hostAccountAddress=address

return address 
}

// Retrieves the interchain account from state.
function getInterchainAccountAddress(portId: string, channelId: string, icaOwnerAddress: string, hostAccountId:uint64) : string {

return hostAccounts[portId][channelId][icaOwnerAddress][hostAccountId].hostAccountAddress
}
```

##### Helper Function

The helper functions described herein must be implemented in a hosting interchain account module. 

###### **registerInterchainAccount**

`registerInterchainAccount` may be called during the `OnReceive` callback when a `REGISTER_TX` tx type is relayed to the host chain and contains the parameter `usedIds`.
 
```typescript
function registerInterchainAccount(
  portId: string,
  channelId: string,
  counterpartyPortId: string, 
  counterpartyChannelId: string,
  icaOwnerAccount: string,
  usedIds: []uint64 
  ) {  

  // validate port format
  abortTransactionUnless(portId=="ica") 
  // retrieve channel 
  channel = provableStore.get(channelPath(portId, channelId))
  // validate that the channel infos
  //abortTransactionUnless(isActive(channel))
  abortTransactionUnless(channel.counterpartyPortId == counterpartyPortId) 
  abortTransactionUnless(channel.counterpartyChannelId == counterpartyChannelId)   

  for seq in usedIds{
    setInterchainAccountAddress(portId,channelId,icaOwnerAddress,seq)
  }
  
}
```

##### **executeTx**

Executes each message sent by the owner account on the controller chain.

```typescript
function executeTx(hostAccount: string, msg Any) : (resultString, error) {
  
  // Signature has already been validated in the sendTx 
  // Execute the msg for the given hostAccount 
  
   // Review Syntax
  return hostAccount.execute(msg), nil  
  // return result of transaction
}
```

##### **msgBlacklist** 

The `addMessageToBlacklist` can be called by the host chain to blacklist certain types of msgs. 
// Note, the blacklist can be dangerous, check the consideration section. Inspect

```typescript
function addMessageToBlacklist(portId: string, channelId: string, icaOwnerAddress: string, msgType: string): error {

  if(msgType.isIn(msgBlacklist[portId][channelId][icaOwnerAddress])==false){
    msgBlacklist[portId][channelId][icaOwnerAddress].add(msgType)
    return nil
  } else { 
    return "Message type already blacklisted"
    }
}
```

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
        if getInterchainAccountAddress(portId,channelId,icaOwnerAddress,hostAccountId)!=="") {
          return NewErrorAcknowledgement("hostAccountId already used")
          }
        
        registerInterchainAccount(Packet.DestinationPort,Packet.DestinationChannel,Packet.SourcePort,Packet.SourceChannel, icaOwnerAccount,seq)
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

    // Construct the set of addresses given the hostAccountIds 
    for seq in hostAccountIds{ 
      temp=getInterchainAccountAddress(portId,channelId,icaOwnerAddress,seq)
      if temp=="" {
        return NewErrorAcknowledgement("Requesting Tx For A Non Registered Account")
      }
      hostAddressesSet.add(temp)
      }
  
    for msg in msgs{
      // TODO Include check for blacklisted message.
      if(msg.type.isIn(msgBlackList[portId][channelId][icaOwnerAddress])){
          return NewErrorAcknowledgement("The controller chain is trying to execute a message that has been blacklisted by the host chain.")
        }

      executingAddress = msg.expectedSigner()
      // Verify that the expectedSigner is in the set of host addresses constructed for this IBC tx.
      // Here the idea is that we confirm that the expected signer is part of a set of the hostAccountsAddress set constructed with the hostAccountIds passed in by the icaOwnerAddress. Is this enough? 
        if(executingAddress.isIn(hostAddressesSet)==false){
          return NewErrorAcknowledgement("Expected Signer Mismatch")
        }

      // executeTx executes each message individually  
      resultData, err = executeTx(executingAddress, msg)
      if err != nil {
        // In case any of the msg in the for loop fails, everything will be reverted by returning the error ack providing atomiticy between msgs of the same ica packet. 
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

// TODO: Provide diagrams for each of the flows [DiagramBaseImage](https://excalidraw.com/#json=BfGp0ZbDAhO_LWiNmdGGn,Mgw4FrmorzuGjM0alzLMWw
)

#### Registering Flow 

##### Success Case 

Precondition: The user on the controller chain has created an account. This account will be used as the `icaOwnerAddress`.  

- 0.1 The user creates a `registerTx` Tx1.  
- 0.2 The user sign Tx1 with the `icaOwnerAddress`
- 0.3 The user sends Tx1 to the controller state machine. 

- 1.1 The controller state machine passes the transaction to the proper icaTxHandler.  
- 1.2 The `icaRegisterTxHandler` validates Tx1 and executes signature checks over `icaOwnerAddress` verifying it is the signer of Tx1. 
- 1.3 The `icaRegisterTxHandler` calls `sendRegisterTx`

- 2.1 The `sendRegisterTx` calls the `registerInterchainAccount` controller function. 
- 2.2 The `registerInterchainAccount` computes the `usedHostAccountsIds`
- 2.3 The `sendRegisterTx` construct and sends the packet, via ICS-4 wrapper, with `icaRegisterPacketData` information (containing the `icaOwnerAddress` and the `usedHostAccountsIds`)

- 3.1 The relayer relays the packet to the host state machine. 

- 4.1 The host state machine dispatches the packet to the proper module handler.  
- 4.2 The `onRecvPacket` callback is activated on the host state machine and triggers the `registerInterchainAccount` function. 
- 4.3 The `registerInterchainAccount` function of the host chain verifies that the `usedHostAccountsIds` have not already been used. 
- 4.4 The `registerInterchainAccount` generates the addresses based on the passed-in parameters.
- 4.5 The addresses are stored in the host chain module state. 
- 4.6 Upon completion, the `onRecvPacket` callback writes an acknowledgment containing the newly generated addresses. 

- 5.1 The relayer relays the acknowledgment packet to the controller state machine.

- 6.1 The `onAcknowledgePacket` is activated on the controller state machine 
- 6.2 The addresses contained in the acknowledgment are written into the controller chain module state. 

##### Error Case 

Precondition: The user on the controller chain has created an account. This account will be used as the `icaOwnerAddress`.  

- 0.1 The user creates a `registerTx` Tx1.  
- 0.2 The user sign Tx1 with the `icaOwnerAddress`
- 0.3 The user sends Tx1 to the controller state machine. 

- 1.1 The controller state machine passes the transaction to the proper icaTxHandler.  
- 1.2 The `icaRegisterTxHandler` validates Tx1 and executes signature checks over `icaOwnerAddress` verifying it is the signer of Tx1.  
- 1.3 The `icaRegisterTxHandler` calls `sendRegisterTx`

- 2.1 The `sendRegisterTx` calls the `registerInterchainAccount` controller function. 
- 2.2 The `registerInterchainAccount` computes the `usedHostAccountsIds`
- 2.3 The `sendRegisterTx` construct and sends the packet, via ICS-4 wrapper, with `icaRegisterPacketData` information (containing the `icaOwnerAddress` and the `usedHostAccountsIds`)

- 3.1 The relayer relays the packet to the host state machine. 

- 4.1 The host state machine dispatches the packet to the proper module handler.  
- 4.2 The `onRecvPacket` callbacks are activated on the host state machine. 
- 4.3 The `onRecvPacket` function triggers an error, thus it returns an error acknowledgment. 

- 5.1 The relayer relays the error acknowledgment packet to the controller state machine.

- 6.1 The `onAcknowledgePacket` is activated on the controller state machine 
- 6.2 The `usedHostAccountsIds` are recovered and stored in the `unusedHostAccountsIds` array. 

Note that `onTimeout` a similar logic is triggered with the `usedHostAccountsIds` that get recovered and stored in the `unusedHostAccountsIds` array.

#### Controlling Flow 

##### Success Case 

Precondition: The user on the controller chain has registered an account on the host chain. The message that will be passed in must use an already registered account address.  

- 0.1 The user creates a `executeTx` Tx2.  
- 0.2 The user signs Tx2 with the `icaOwnerAddress`.  
- 0.3 The user sends Tx2 to the controller state machine.  
 
- 1.1 The controller state machine passes the transaction to the proper icaTxHandler.  
- 1.2 The `icaExecuteTxHandler` validates Tx2 and executes signatures checks over the `icaOwnerAddress` verifying this is the signer of Tx2.  
- 1.3 The `icaExecuteTxHandler` calls `sendExecuteTx`.  

- 2.1 The `sendExecuteTx` verifies that the `hostAccountIds` passed in are actually related to an already registered `hostAccountAddress` and that the messages array is not empty.    
- 2.2 The `sendExecuteTx` constructs and sends the packet, via ICS-4 wrapper, with `icaExecutePacketData` information (containing the `icaOwnerAddress` and the `hostAccountsIds` and the `msgs`).  

- 3.1 The relayer relays the packet to the host state machine.

- 4.1 The host state machine dispatches the packet to the proper module handler.    
- 4.2 The `onRecvPacket` callback is activated on the host state machine.   
- 4.3 During the `onRecvPacket` the addresses set is constructed given the `hostAccountIds` (retrieve the addresses from the module state given the `hostAccountId` key).  
- 4.4 For every msg contained in the `msgs` array, the `onRecvPacket` verifies that the `msg.expectedSigner` is contained in constructed the addresses set.  
- 4.5 The msg is executed and the return values are saved in the `resultData`.   
- 4.6 Once all the msgs are executed, the acknowledgment containing `resultData` is returned.   

- 5.1 The relayer relays the acknowledgment packet to the controller state machine.  

- 6.1 The `onAcknowledgePacket` is activated on the controller state machine triggering a noOp.     

##### Error Case 

Precondition: The user on the controller chain has registered an account on the host chain. The message that will be passed in must use an already registered account address. 

- 0.1 The user creates an `executeTx` Tx2.    
- 0.2 The user signs Tx2 with the `icaOwnerAddress`.  
- 0.3 The user sends Tx2 to the controller state machine.   

- 1.1 The controller state machine passes the transaction to the proper icaTxHandler.    
- 1.2 The `icaExecuteTxHandler` validates Tx2 and executes signatures checks over the `icaOwnerAddress` verifying this is the signer of Tx2.  
- 1.3 The `icaExecuteTxHandler` calls `sendExecuteTx`.  

- 2.1 The `sendExecuteTx` verifies that the `hostAccountIds` passed in are actually related to an already registered `hostAccountAddress` and that the messages array is not empty.    
- 2.2 The `sendRegisterTx` construct and sends the packet, via ICS-4 wrapper, with `icaExecutePacketData` information (containing the `icaOwnerAddress` and the `hostAccountsIds` and the `msgs`).  

- 3.1 The relayer relays the packet to the host state machine.   

- 4.1 The host state machine dispatches the packet to the proper module handler.    
- 4.2 The `onRecvPacket` callbacks are activated on the host state machine.   
- 4.3 The `onRecvPacket` function triggers an error, thus it returns an error acknowledgment.   

- 5.1 The relayer relays the error acknowledgment packet to the controller state machine.  

- 6.1 The `onAcknowledgePacket` is activated on the controller state machine triggering a noOp (nothing to revert on the controller chain).  

## Considerations

### Message Execution Ordering 

Problem:  
Given chain A and chain B, if chain A sends two IBC packets, each one containing an  `EXECUTE_TX` message, the order of execution of the packets, and so of the messages, is not guaranteed because it depends on the relayer delivery order of the packets themself. 

Solution: 
The user who needs a certain order of execution for its messages Must place them in the same IBC ica packet. When the messages are placed in the same IBC packet, we can guarantee the atomicity and the order of execution. Indeed if any of the messages fails, everything will be reverted by writing an error acknowledgment, and, additionally, the messages that are passed in an interchain account `EXECUTE_TX` will be executed by the host chain following a FIFO mechanism. 

### Account balances post execution on the host chain 

In the case the controller chain wants to know the host account balance after certain msgs are executed, it should include a cross-chain-query message at the bottom of the msg list. 

### Interchain Account Recovery

Since we are allowing only unordered channels and disallowing channel closure procedures, we don't need a procedure for recovering. The channel cannot be closed or get stuck. 

### MsgBlacklist 

// TODO Should be the blacklist icaOwnerAccounts specific? 
What happens if the controller chain registers and funds a hostAccount and then the host chain blacklist the transfers? Are funds lost? 

## Example Implementations

- Implementation of ICS 27 version 1 in Go can be found in the [ibc-go repository](https://github.com/cosmos/ibc-go).

- Implementation of ICS 27 version 2 in Go COMING SOON. 

## Future Improvements

// TODO  

A future version of interchain accounts may be greatly simplified by the introduction of an IBC channel type that is ORDERED but does not close the channel on timeouts and instead proceeds to accept and receive the next packet. If such a channel type is made available by core IBC, Interchain accounts could require the use of this channel type and remove all logic and state pertaining to "active channels". The metadata format can also be simplified to remove any reference to the underlying connection identifiers.

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

July 4, 2024 - [ICS-27 version 2 draft suggested](https://github.com/cosmos/ibc/pull/1122) 
    
## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
