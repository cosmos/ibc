---
ics: 27
title: Interchain Accounts
stage: Draft
category: IBC/TAO
requires: 25, 26
kind: instantiation
author: Tony Yun <tony@chainapsis.com>, Dogemos <josh@tendermint.com>, Sean King <sean@interchain.io>
created: 2019-08-01
modified: 2020-07-14
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the account management system over an IBC channel between separate chains.

### Motivation

ICS-27 Interchain Accounts outlines a cross-chain account management protocol built upon IBC. ICS-27 enabled chains can programmatically create accounts on other ICS-27 enabled chains & control these accounts via IBC transactions (instead of signing with a private key). Interchain accounts retain all of the capabilities of a normal account (i.e. stake, send, vote) but instead are managed by a separate chain via IBC in a way such that the owner of the accounts retain full control over how the accounts behave. 

### Definitions 

- `Interchain Account`: An account on a host chain. An interchain account has all the capabilities of a normal account. However, rather than signing transactions with a private key, a controller chain will send IBC packets to the host chain which signal what transactions the interchain account should execute 
- `Interchain Account Owner`: An account on the controller chain. Every interchain account on a host chain has a respective owner account on the controller chain 
- `Controller Chain`: The chain registering and controlling an account on a host chain. The controller chain sends IBC packets to the host chain to control the account
- `Host Chain`: The chain where the interchain account is registered. The host chain listens for IBC packets from a controller chain which should contain instructions (e.g. cosmos SDK messages) that the interchain account will execute

The IBC handler interface & IBC relayer module interface are as defined in [ICS 25](../ics-025-handler-interface) and [ICS 26](../ics-026-routing-module), respectively.

### Desired Properties

- Permissionless 
- Fault tolerance: A controller chain must not be able to control accounts registered by other controller chains. For example, in the case of a fork attack on a controller chain, only the interchain accounts registered by the forked chain will be vulnerable
- The ordering of transactions sent to an interchain account on a host chain must be maintained. Transactions should be executed by an interchain account in the order in which they are sent by the controller chain
- If a channel closes, the controller chain must be able to regain access to registered interchain accounts by simply opening a new channel
- Each interchain account is owned by a single account on the controller chain. Only the owner account on the controller chain is authorized to control the interchain account. The controller chain is responsible for enforcing this logic.
- The controller chain must store the account address of any owned interchain account's registered on host chains
- A host chain must have the ability to limit interchain account functionality on its chain as necessary (e.g. a host chain can decide that interchain accounts registered on the host chain cannot take part in staking)


## Technical Specification

### General Design 

A chain can utilize one or both parts of the interchain accounts protocol (*controlling* and *hosting*). A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

This specification defines the general way to register an interchain account and transfer tx bytes to control the account. The host chain is responsible for deserializing and executing the tx bytes, and the controller chain should know how the host chain will handle the tx bytes in advance (Cosmos SDK chains will deserialize using Protobuf). 

### Contract

```typescript

// * CONTROLLER CHAIN *

// InitInterchainAccount is the entry point to registering an interchain account 
// It generates a new port identifier using the owner address and connection identifiers
// The port id will look like: ics27-1.{connection-id}.{counterparty-connection-id}.{owner-address}
// It will bind to the port identifier and
// call 04-channel 'ChanOpenInit'. An error is returned if the port identifier is already in use
// An `OnChannelOpenInit` event is emitted which can be picked up by an offchain process such as a relayer
// The account will be registered during the OnChanOpenTry step on the host chain
function InitInterchainAccount(connectionId: string, counterPartyConnectionId: string, owner: string) returns (error){
}

// TrySendTx is used to send an IBC packet containing instructions (messages) to an interchain account on a host chain for a given interchain account owner 
function TrySendTx(channelCapability: ChannelCapability, portID: string, connectionId: string, counterPartyConnectionId: string, icaPacketData: InterchainAccountPacketData) returns (uint64, error){
    // A call to GetActiveChannel() checks if there is a currently active channel for this port-id which also implies an interchain account has been registered using this port identifier
    // if there are no errors CreateOutgoingPacket() is called and the IBC packet will be sent to the host chain on the active channel
}


// Opens a new channel on a particular port given a connection
// This is a helper function to open a new channel 
// This is a safety function in case of a channel closing and the controller chain needs to regain access to an interchain account on the host chain 
function InitChannel(portId: string, connectionId: string) returns (nil){
  // An `OnChannelOpenInit` event is emitted which can be picked up by an off-chain process such as a relayer which will finish the channel opening handshake
  // The active channel will be set to the newly opened channel on the OnChanOpenAck & OnChanOpenConfirm steps
}

// * HOST CHAIN *

// RegisterInterchainAccount is called on the OnChanOpenTry step during the channel creation handshake 
function RegisterInterchainAccount(accAddr: string, counterPartyPortId: string) returns (nil){
   // checks to make sure the account has not already been registered
   // creates a new address on chain 
   // calls SetInterchainAccountAddress()
}

// AuthenticateTx is called before ExecuteTx.
// AuthenticateTx checks that the signer of a particular message is the interchain account associated with the counteryParty portId of the channel that the IBC packet was sent on.
function AuthenticateTx(msgs []Any, portId string) error {
    // GetInterchainAccountAddress(portId)
    // interchainAccountAddress != signer.String() return error
}

// Executes each message sent by the owner on the Controller chain
function ExecuteTx(sourcePort: string, destPort: string, destChannel: string, msgs []Any) error {
  // validates each message
  // executes each message
}

// * UTILITY FUNCTIONS *

// Sets the active channel
function SetActiveChannelID(portId: string, channelId: string) returns (error){
}

// Returns the id of the active channel if present
function GetActiveChannelID(portId: string) returns (string, boolean){
}

// Stores the address of the interchain account in state
function SetInterchainAccountAddress(portId string, address string) returns (string) {
}

// Gets the interchain account from state
function GetInterchainAccountAddress(portId string) returns (string, bool){
}
```

### Register & Controlling flows

#### Register Account Flow

To register an interchain account we require an off-chain process (relayer) to listen for `OnChannelOpenInit` events with the capability to finish a channel creation handshake on a given connection. 

1. The controller chain binds a new IBC port with an id composed of the *source/counterparty connection-ids* & the *interchain account owner address*

The IBC port identifier will look like this:
```
ics27-1.{connection-id}.{counterparty-connection-id}.{owner-address}
```

This port will be used to create channels between the controller & host chain for a specific owner/interchain account pair. Only the account with `{owner-address}` matching the bound port will be authorized to send IBC packets over channels created with `ics27-1.{connection-id}.{counterparty-connection-id}.{owner-address}`. It is up to each controller chain to enforce this port registration and access on the controller side. 

2. The controller chain emits an event signaling to open a new channel on this port given a connection 
3. A relayer listening for `OnChannelOpenInit` events will begin the channel creation handshake
4. During the `OnChanOpenTry` callback on the host chain an interchain account will be registered and a mapping of the interchain account address to the owner account address will be stored in state (this is used for authenticating transactions on the host chain at execution time). 
5. During the `OnChanOpenAck` callback on the controller chain a record of the interchain account address registered on the host chain during `OnChanOpenTry` is set in state with a mapping from portID -> interchain account address. See [version negotiation](#Version-Negotiation) section below for how to implement this
6. During the `OnChanOpenAck` & `OnChanOpenConfirm` callbacks on the controller & host chains respectively, the [active-channel](#Active-Channels) for this interchain account/owner pair, is set in state

#### Active Channels

The controller and host chain should keep track of an `active-channel` for each registered interchain account. The `active-channel` is set during the channel creation handshake process. This is a safety mechanism that allows a controller chain to regain access to an interchain account on a host chain in case of a channel closing. 

An example of an active channel on the controller chain can look like this:


```typescript
{
 // Controller Chain
 SourcePortId: `ics27-1.0.0.<owner-id>`,
 SourceChannelId: `channel-1`,
 // Host Chain
 CounterPartyPortId: `interchain-account`,
 CounterPartyChannelId: `channel-2`,
}
```

In the event of a channel closing, the active channel should be unset. ICS-27 channels should only be closed in the event of a timeout (if the implementation uses ordered channels) or in the unlikely event of a light client attack. It is critical that controller chains retain the ability to open new ICS-27 channels and reset the active channel for a particular port id (owner) and associated interchain account. 

#### Version Negotiation

ICS-27 takes advantage of [ISC-04 channel version negotiation](https://github.com/cosmos/ibc/tree/master/spec/core/ics-004-channel-and-packet-semantics#versioning) to store a mapping of the controller chain port ID to the newly registered interchain account address, on both the host chain & controller chains, during the channel creation handshake ([account registration flow](#Register-Account-Flow)). 

ICS-004 allows for each channel version negotiation to be application-specific. In the case of interchain accounts, the channel version set during the `OnChanOpenInit` step (controller chain) must be `ics27-<version>` & the version set during the host chain `OnChanOpenTry` step will include the interchain account address that will be created. Note that the generation of this address is stateless, and can be generated in advance of the account creation. An example implementation of this can be viewed here: 

Due to how the mechanics of ICS-004 channel version negotiation operate the version passed into the host chain side (OnChanOpenTry) must match the counterparty version passed into the controller side (OnChanOpenAck) otherwise there will be a resulting error at the IBC protocol level. 

Combined with the one channel per interchain account approach, this method of version negotiation allows us to pass the address of the interchain account back to the controller chain and create a mapping from controller port ID -> interchain account address during the `OnChanOpenAck` callback. As outlined in the [controlling flow](#Controlling-Flow), a controller chain will need to know the address of a registered interchain account in order to send transactions to the account on the host chain.

#### Version negotiation summary

| Initiator | Datagram         | Chain acted upon |  Version (Controller, Host) |
| --------- | ---------------- | ---------------- |  ---------------------- | 
| Controller| ChanOpenInit     | Controller       |  (ics27-1, none)        | 
| Relayer   | ChanOpenTry      | Host             |  (ics27-1, ics27-1.{interchain-account-address})        | 
| Relayer   | ChanOpenAck      | Controller       |  (ics27-1, ics27-1.{interchain-account-address})        | 
| Relayer   | ChanOpenConfirm  | Host             |  (ics27-1, ics27-1.{interchain-account-address})        | 

#### Controlling Flow

Once an interchain account is registered on the host chain a controller chain can begin sending instructions (messages) to the host chain to control the account. 

1. The controller chain calls `TrySendTx` and passes message(s) that will be executed on the host side by the associated interchain account (determined by the controller side port identifier)

Cosmos SDK psuedo code example:

```typescript
interchainAccountAddress := GetInterchainAccountAddress(portId)
msg := &banktypes.MsgSend{FromAddress: interchainAccountAddress, ToAddress: ToAddress, Amount: amount}
icaPacketData = InterchainAccountPacketData{
   Type: types.EXECUTE_TX,
   Data: serialize(msg),
   Memo: "memo",
}

// Sends the message to the host chain, where it will eventually be executed 
TrySendTx(ownerAddress, connectionId, counterPartyConnectionId, data)
```

4. The host chain upon receiving the IBC packet will call `DeserializeTx` and then call `AuthenticateTx` for each message. If either of these steps fails an error will be returned
    
Messages are authenticated on the host chain by taking the controller side port identifier and calling `GetInterchainAccountAddress(controllerPortId)` to get the expected interchain account address for the controller port (owner). If the signer of this message does not match the expected account address then authentication will fail. An example implementation for the cosmos SDK can be seen here:
    
5. The host chain will then call `ExecuteTx` for each message and return an acknowledgment

### Packet Data
`InterchainAccountPacketData` contains an array of messages that an interchain account can execute and a memo string that is sent to the host chain as well as the packet `type`. ICS-27 version 1 has only one type `EXECUTE_TX`.

```typescript
message InterchainAccountPacketData  {
    enum type
    bytes data = 1;
    string memo = 2;
}
```

The acknowledgment packet structure is defined as in [ics4](https://github.com/cosmos/cosmos-sdk/blob/v0.42.4/proto/ibc/core/channel/v1/channel.proto#L134-L147). If an error occurs on the host chain the acknowledgment should contain the error message.

```typescript
message Acknowledgement {
  // response contains either a result or an error and must be non-empty
  oneof response {
    bytes  result = 21;
    string error  = 22;
  }
}
```

### Custom logic

ICS-27 relies on [ICS-30 middleware architecture](https://github.com/cosmos/ibc/tree/master/spec/app/ics-030-middleware) to provide the option for application developers to apply custom logic on the success or fail of ICS-27 packets. 

Controller chains will wrap `OnAcknowledgement` & `OnTimeoutPacket` in order to handle the success or fail cases for ICS-27 packets. 

### Port & channel setup

The interchain account module on a host chain must always bind to a port with the id `interchain-account`. Controller chains will bind to ports dynamically, with each port id set as `ics27-1.{connection-id}.{counterparty-connection-id}.{owner-address}`.

The example below assumes a module is implementing the entire `InterchainAccountModule` interface. The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialized) to bind to the appropriate port.

```typescript
function setup() {
  capability = routingModule.bindPort("interchain-account", ModuleCallbacks{
      onChanOpenInit,
      onChanOpenTry,
      onChanOpenAck,
      onChanOpenConfirm,
      onChanCloseInit,
      onChanCloseConfirm,
      onRecvPacket,
      onTimeoutPacket,
      onAcknowledgePacket,
      onTimeoutPacketClose
    })
    claimCapability("port", capability)
}
```

Once the `setup` function has been called, channels can be created via the IBC routing module.

### Channel lifecycle management

An interchain account module will accept new channels from any module on another machine, if and only if:

- The channel being created is ordered
- The channel initialization step is being invoked from the controller chain

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // validate port format
  abortTransactionUnless(validateControllerPortParams(portIdentifier))
  // only allow channels to be created on the "interchain-account" port on the counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "interchain-account")
  // validate controller side channel version
  abortTransactionUnless(version === "ics27-1")
  // only open the channel if there is no active channel already set (with status OPEN)
  abortTransactionUnless(activeChannel === nil)
}
```

```typescript
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
  // only unordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // validate port format
  abortTransactionUnless(validateControllerPortParams(portIdentifier))
  // assert that version is expected format `ics27-1.interchain-account-address`
  abortTransactionUnless(validateVersion(version))
  // assert that the counterparty version is `ics27-1`  
  abortTransactionUnless(counterpartyVersion === "ics27-1")
  // create the interchain account 
  RegisterInterchainAccount(accAddr, counterpartyPortIdentifier)
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // validate that the counterparty version is expected format `ics27-1.interchain-account-address`      
  abortTransactionUnless(validateVersion(version))
  // state change to keep track of successfully registered interchain account
  SetInterchainAccountAddress(portID, accAddr)
  // set the active channel for this owner/interchain account pair
  setActiveChannel(SourcePortId)
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // set the active channel for this owner/interchain account pair
  setActiveChannel(portIdentifier)
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) { \
    // unset the active channel for this owner/interchain account pair
    DeleteActiveChannel(portIdentifier, channelIdentifier)
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // unset the active channel for this owner/interchain account pair
    DeleteActiveChannel(portIdentifier, channelIdentifier)
}
```

### Packet relay
`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function OnRecvPacket(packet Packet) {
    	ack := NewResultAcknowledgement([]byte{byte(1)})

	// only attempt the application logic if the packet data
	// was successfully decoded
	if ack.Success() {
		switch data.Type {
	          case types.EXECUTE_TX:
		        msgs, err := types.DeserializeTx(data.Data)
		        if err != nil {
			      return err
		        }

		        if err = executeTx(ctx, packet.SourcePort, packet.DestinationPort, packet.DestinationChannel, msgs); err != nil {
			      return err
		        }

		        return nil
	          default:
		        return ErrUnknownDataType
	        }
	        if err != nil {
                    ack = NewErrorAcknowledgement(err.Error())
                }
	}

	// NOTE: acknowledgement will be written synchronously during IBC handler execution.
	return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
    // call underlying app's OnAcknowledgementPacket callback 
    // see ICS30 middleware for more information
}
```

```typescript
function onTimeoutPacket(packet: Packet) {
    // call underlying app's OnTimeoutPacket callback 
    // see ICS30 middleware for more information
}
```

## Example Implementation

Repository for Cosmos-SDK implementation of ICS-27: https://github.com/cosmos/ibc-go

## History

Aug 1, 2019 - Concept discussed

Sep 24, 2019 - Draft suggested

Nov 8, 2019 - Major revisions

Dec 2, 2019 - Minor revisions (Add more specific description & Add interchain account on Ethereum)

July 14, 2020 - Major revisions

April 27, 2021 - Redesign of ics27 specification

November 11, 2021 - Update with latest changes from implementation
    
## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).

