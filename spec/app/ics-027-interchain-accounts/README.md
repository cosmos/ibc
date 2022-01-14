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
- A host chain must have the ability to limit interchain account functionality on its chain as necessary (e.g. a host chain can decide that interchain accounts registered on the host chain cannot take part in staking).


## Technical specification

### General design 

A chain can utilize one or both parts of the interchain accounts protocol (*controlling* and *hosting*). A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

This specification defines the general way to register an interchain account and transfer tx bytes to control the account. The host chain is responsible for deserializing and executing the tx bytes, and the controller chain must know how the host chain will handle the tx bytes in advance (Cosmos SDK chains will deserialize using Protobuf). 

### Controller chain contract

#### **InitInterchainAccount**

`InitInterchainAccount` is the entry point to registering an interchain account.
It generates a new controller portID using the owner account address and connection identifiers.
It will bind to the controller portID and
call 04-channel `ChanOpenInit`. An error is returned if the controller portID is already in use.
A `ChannelOpenInit` event is emitted which can be picked up by an offchain process such as a relayer.
The account will be registered during the `OnChanOpenTry` step on the host chain.
This function must be called after an `OPEN` connection is already established with the given connection and counterparty connection identifiers.

```typescript
function InitInterchainAccount(connectionId: string, counterpartyConnectionId: string, owner: string) returns (error){
}
```

#### **TrySendTx**

`TrySendTx` is used to send an IBC packet containing instructions (messages) to an interchain account on a host chain for a given interchain account owner.

```typescript
function TrySendTx(channelCapability: ChannelCapability, portId: string, connectionId: string, counterpartyConnectionId: string, icaPacketData: InterchainAccountPacketData) returns (uint64, error){
    // A call to GetActiveChannel() checks if there is a currently active channel for this portId which also implies an interchain account has been registered using this port identifier
    // if there are no errors CreateOutgoingPacket() is called and the IBC packet will be sent to the host chain on the active channel
}
```

#### **InitChannel**

Opens a new channel on a particular port given a connection.
This is a helper function to open a new channel.
This is a safety function in case of a channel closing and the controller chain needs to regain access to an interchain account on the host chain.

```typescript
function InitChannel(portId: string, connectionId: string) returns (nil){
  // A `ChannelOpenInit` event is emitted which can be picked up by an off-chain process such as a relayer which will finish the channel opening handshake
  // The active channel will be set to the newly opened channel on the `OnChanOpenAck` & `OnChanOpenConfirm` steps
}
```

### Host chain contract

#### **RegisterInterchainAccount**

`RegisterInterchainAccount` is called on the `OnChanOpenTry` step during the channel creation handshake.

```typescript
function RegisterInterchainAccount(accAddr: string, counterpartyPortId: string) returns (nil){
   // checks to make sure the account has not already been registered
   // creates a new address on chain 
   // calls SetInterchainAccountAddress()
}
```

#### **AuthenticateTx**

`AuthenticateTx` is called before `ExecuteTx`.
`AuthenticateTx` checks that the signer of a particular message is the interchain account associated with the counterparty portID of the channel that the IBC packet was sent on.

```typescript
function AuthenticateTx(msgs []Any, portId string) error {
    // GetInterchainAccountAddress(portId)
    // interchainAccountAddress != signer.String() return error
}
```

#### **ExecuteTx**

Executes each message sent by the owner account on the controller chain.

```typescript
function ExecuteTx(sourcePort: string, destPort: string, destChannel: string, msgs []Any) error {
  // validate each message
  // verify that interchain account owner is authorized to send each message
  // execute each message
}
```

### Utility functions

```typescript
// Sets the active channel for a given portID.
function SetActiveChannelID(portId: string, channelId: string) returns (error){
}

// Returns the ID of the active channel for a given portID, if present.
function GetActiveChannelID(portId: string) returns (string, boolean){
}

// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(portId string, address string) returns (string) {
}

// Retrieves the interchain account from state.
function GetInterchainAccountAddress(portId string) returns (string, bool){
}

// DeleteActiveChannelID removes the active channel keyed by the provided portID stored in state
function (k Keeper) DeleteActiveChannelID(portId string) {
}
```

### Register & controlling flows

#### Register account flow

To register an interchain account we require an off-chain process (relayer) to listen for `ChannelOpenInit` events with the capability to finish a channel creation handshake on a given connection. 

1. The controller chain binds a new IBC port with the controller portID for a given *source/counterparty connection-ids* and *interchain account owner address*.

This port will be used to create channels between the controller & host chain for a specific owner/interchain account pair. Only the account with `{owner-account-address}` matching the bound port will be authorized to send IBC packets over channels created with the controller portID. It is up to each controller chain to enforce this port registration and access on the controller side. 

2. The controller chain emits an event signaling to open a new channel on this port given a connection. 
3. A relayer listening for `ChannelOpenInit` events will continue the channel creation handshake.
4. During the `OnChanOpenTry` callback on the host chain an interchain account will be registered and a mapping of the interchain account address to the owner account address will be stored in state (this is used for authenticating transactions on the host chain at execution time). 
5. During the `OnChanOpenAck` callback on the controller chain a record of the interchain account address registered on the host chain during `OnChanOpenTry` is set in state with a mapping from portID -> interchain account address. See [version negotiation](#Version-negotiation) section below for how to implement this.
6. During the `OnChanOpenAck` & `OnChanOpenConfirm` callbacks on the controller & host chains respectively, the [active-channel](#Active-channels) for this interchain account/owner pair, is set in state.

#### Active channels

The controller and host chain must keep track of an `active-channel` for each registered interchain account. The `active-channel` is set during the channel creation handshake process. This is a safety mechanism that allows a controller chain to regain access to an interchain account on a host chain in case of a channel closing. 

An example of an active channel on the controller chain can look like this:


```typescript
{
 // Controller Chain
 SourcePortId: `ics27-<version>.<source-connection-id>.<destination-connection-id>.<owner-account-address>`,
 SourceChannelId: `<channel-id>`,
 // Host Chain
 CounterpartyPortId: `interchain-account`,
 CounterpartyChannelId: `<channel-id>`,
}
```

In the event of a channel closing, the active channel must be unset. ICS-27 channels can only be closed in the event of a timeout (if the implementation uses ordered channels) or in the unlikely event of a light client attack. Controller chains must retain the ability to open new ICS-27 channels and reset the active channel for a particular portID (containing `{owner-account-address}`) and associated interchain account. 

#### Version negotiation

ICS-27 takes advantage of [ICS-04 channel version negotiation](../../core/ics-004-channel-and-packet-semantics/README.md#versioning) to store a mapping of the controller chain portID to the newly registered interchain account address, on both the host & controller chains, during the channel creation handshake ([account registration flow](#Register-account-flow)). 

ICS-04 allows for each channel version negotiation to be application-specific. In the case of interchain accounts, the channel version set during the `OnChanOpenInit` step (controller chain) must be `ics27-<version>` & the version set during the host chain `OnChanOpenTry` step will include the interchain account address that will be created ([see summary table below](#Version-negotiation-summary)). Note that the generation of this address is stateless, and can be generated in advance of the account creation. 

Due to how the mechanics of ICS-04 channel version negotiation operate the version passed into the host chain side (`OnChanOpenTry`) must match the counterparty version passed into the controller side (`OnChanOpenAck`) otherwise, there will be a resulting error at the IBC protocol level. 

Combined with the one channel per interchain account approach, this method of version negotiation allows us to pass the address of the interchain account back to the controller chain and create a mapping from controller portID -> interchain account address during the `OnChanOpenAck` callback. As outlined in the [controlling flow](#Controlling-flow), a controller chain will need to know the address of a registered interchain account in order to send transactions to the account on the host chain.

#### Version negotiation summary

`interchain-account-address` is the address of the interchain account registered on the host chain by the controller chain.

| Initiator | Datagram         | Chain acted upon |  Version (Controller, Host) |
| --------- | ---------------- | ---------------- |  ---------------------- | 
| Controller| ChanOpenInit     | Controller       |  (ics27-1, none)        | 
| Relayer   | ChanOpenTry      | Host             |  (ics27-1, ics27-1.{interchain-account-address})        | 
| Relayer   | ChanOpenAck      | Controller       |  (ics27-1, ics27-1.{interchain-account-address})        | 
| Relayer   | ChanOpenConfirm  | Host             |  (ics27-1, ics27-1.{interchain-account-address})        | 

The channel `version` string passed into the `OnChanOpenTry` callback by a relayer will contain the account address of the interchain account that will be registered on the host chain. A relayer will then pass this version into the `OnChanOpenAck` step on the controller chain as the `counterpartyVersion`. The controller chain will then proceed to parse the interchain account address from the `counterpartyVersion` string and store this address in state with a map to the associated portID.  

#### Controlling flow

Once an interchain account is registered on the host chain a controller chain can begin sending instructions (messages) to the host chain to control the account. 

1. The controller chain calls `TrySendTx` and passes message(s) that will be executed on the host side by the associated interchain account (determined by the controller side port identifier)

Cosmos SDK pseudo-code example:

```typescript
interchainAccountAddress := GetInterchainAccountAddress(portId)
msg := &banktypes.MsgSend{FromAddress: interchainAccountAddress, ToAddress: ToAddress, Amount: amount}
icaPacketData = InterchainAccountPacketData{
   Type: types.EXECUTE_TX,
   Data: serialize(msg),
   Memo: "memo",
}

// Sends the message to the host chain, where it will eventually be executed 
TrySendTx(ownerAddress, connectionId, counterpartyConnectionId, data)
```

2. The host chain upon receiving the IBC packet will call `DeserializeTx`. 
    
3. The host chain will then call `AuthenticateTx` and `ExecuteTx` for each message and return an acknowledgment containing a success or error.  

Messages are authenticated on the host chain by taking the controller side port identifier and calling `GetInterchainAccountAddress(controllerPortId)` to get the expected interchain account address for the current controller port. If the signer of this message does not match the expected account address then authentication will fail.

### Packet Data
`InterchainAccountPacketData` contains an array of messages that an interchain account can execute and a memo string that is sent to the host chain as well as the packet `type`. ICS-27 version 1 has only one type `EXECUTE_TX`.

```typescript
message InterchainAccountPacketData  {
    enum type
    bytes data = 1;
    string memo = 2;
}
```

The acknowledgment packet structure is defined as in [ics4](https://github.com/cosmos/ibc-go/blob/main/proto/ibc/core/channel/v1/channel.proto#L135-L148). If an error occurs on the host chain the acknowledgment contains the error message.

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

ICS-27 relies on [ICS-30 middleware architecture](../ics-030-middleware) to provide the option for application developers to apply custom logic on the success or fail of ICS-27 packets. 

Controller chains will wrap `OnAcknowledgementPacket` & `OnTimeoutPacket` to handle the success or fail cases for ICS-27 packets. 

### Port & channel setup

The interchain account module on a host chain must always bind to a port with the id `interchain-account`. Controller chains will bind to ports dynamically, as specified in the identifier format [section](#identifer-formats).

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

- The channel being created is ordered.
- The channel initialization step is being invoked from the controller chain.

```typescript
// Called on Controller Chain by InitInterchainAccount
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
// Called on Host Chain by Relayer
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // validate port ID
  abortTransactionUnless(portIdentifier === "interchain-account")
  // only allow channels to be created on host chain if the counteparty port ID
  // is in the expected controller portID format.
  abortTransactionUnless(validateControllerPortParams(counterpartyPortIdentifier))
  // assert that version is expected format `ics27-1.interchain-account-address`
  abortTransactionUnless(validateVersion(version))
  // assert that the counterparty version is `ics27-1`  
  abortTransactionUnless(counterpartyVersion === "ics27-1")
  // create the interchain account 
  RegisterInterchainAccount(accAddr, counterpartyPortIdentifier)
}
```

```typescript
// Called on Controller Chain by Relayer
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
// Called on Host Chain by Relayer
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // set the active channel for this owner/interchain account pair
  setActiveChannel(portIdentifier)
}
```

### Closing handshake

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) { \
 	// disallow user-initiated channel closing for interchain account channels
  return err
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // unset the active channel for given portID 
    DeleteActiveChannelID(portIdentifier)
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

                        // ExecuteTx calls the AuthenticateTx function defined above 
		        if err = ExecuteTx(ctx, packet.SourcePort, packet.DestinationPort, packet.DestinationChannel, msgs); err != nil {
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

	// NOTE: acknowledgment will be written synchronously during IBC handler execution.
	return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
    // call underlying app's OnAcknowledgementPacket callback 
    // see ICS-30 middleware for more information
}
```

```typescript
function onTimeoutPacket(packet: Packet) {
    // unset the active channel for given portID
    DeleteActiveChannelID(portIdentifier)

    // call underlying app's OnTimeoutPacket callback 
    // see ICS-30 middleware for more information
}
```

### Identifier formats

These are the formats that the port identifiers on each side of an interchain accounts channel must follow to be accepted by a correct interchain accounts module.

Controller Port Identifier: `ics27-1.{connection-id}.{counterparty-connection-id}.{owner-account-address}`

Host Port Identifier: `interchain-account`

## Example Implementation

Repository for Cosmos-SDK implementation of ICS-27: https://github.com/cosmos/ibc-go

## Future Improvements

A future version of interchain accounts may be greatly simplified by the introduction of an IBC channel type that is ORDERED but does not close the channel on timeouts, and instead proceeds to accept and receive the next packet. If such a channel type is made available by core IBC, Interchain accounts could require the use of this channel type and remove all logic and state pertaining to "active channels". The controller port identifier format can also be simplified to remove any reference to the underlying connection identifiers

The "active channel" setting and unsetting is currently necessary to allow interchain account owners to create a new channel in case the current active channel closes during channel timeout. The connection identifiers are part of the portID to ensure that any new channel that gets opened are established on top of the original connection. All of this logic becomes unnecessary once the channel is ordered **and** unclosable, which can only be achieved by the introduction of a new channel type to core IBC.

## History

Aug 1, 2019 - Concept discussed

Sep 24, 2019 - Draft suggested

Nov 8, 2019 - Major revisions

Dec 2, 2019 - Minor revisions (Add more specific description & Add interchain account on Ethereum)

July 14, 2020 - Major revisions

April 27, 2021 - Redesign of ics27 specification

November 11, 2021 - Update with latest changes from implementation

December 14, 2021 - Revisions to spec based on audits and maintainer reviews
    
## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).


