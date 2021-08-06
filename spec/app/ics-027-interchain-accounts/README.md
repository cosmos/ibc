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

On Ethereum, there are two types of accounts: externally owned accounts, controlled by private keys, and contract accounts, controlled by their contract code ([ref](https://github.com/ethereum/wiki/wiki/White-Paper)). Similar to Ethereum's contract accounts, interchain accounts are controlled by another chain (not a private key) while retaining all the capabilities of a normal account (i.e. stake, send, vote, etc). While an Ethereum CA's contract logic is performed within Ethereum's EVM, interchain accounts are managed by a separate chain via IBC in a way such that the owner of the account retains full control over how it behaves. ICS27-1 primarily targets the use cases of DAO investing and staking derivatives over IBC.

### Definitions 

- `Interchain Account`: An account on a host chain. An interchain account has all the capabilities of a normal account. However, rather than signing transactions with a private key, a controller chain will send IBC packets to the host chain which signal what transactions the interchain account should execute 
- `Interchain Account Owner`: An account on the controller chain. Every interchain account on a host chain has a respective owner account on the controller chain 
- `Controller Chain`: The chain registering and controlling an account on a host chain. The controller chain sends IBC packets to the host chain to control the account
- `Host Chain`: The chain where the interchain account is registered. The host chain listens for IBC packets from a controller chain which should contain instructions (e.g. cosmos SDK messages) that the interchain account will execute

The IBC handler interface & IBC relayer module interface are as defined in [ICS 25](../ics-025-handler-interface) and [ICS 26](../ics-026-routing-module), respectively.

### Desired Properties

- Permissionless 
- The ordering of transactions sent to an interchain account on a host chain must be maintained. Transactions should be executed by an interchain account in the order in which they are sent by the controller chain
- If a channel closes, the controller chain must be able to regain access to registered interchain accounts by simply opening a new channel
- Each interchain account is owned by a single account on the controller chain. Only the owner account on the controller chain is authorized to control the interchain account
- The controller chain must store the account address of the interchain account

## Technical Specification

### General Design 

A chain can implement one or both parts of the interchain accounts protocol (*controlling* and *hosting*). A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

This specification defines the general way to register an interchain account and transfer tx bytes to control the account. The host chain is responsible for deserializing and executing the tx bytes, and the controller chain should know how the host chain will handle the tx bytes in advance (Cosmos SDK chains will deserialize using Protobuf). 

### Contract

```typescript

// * CONTROLLER CHAIN *

// InitInterchainAccount is the entry point to registering an interchain account.
// It generates a new port identifier using the owner address, connection identifiers
// The port id will look like: ics27-1-{connection-number}-{counterparty-connection-number}-{owner-address}
// and counterparty connection identifier. It will bind to the port identifier and
// call 04-channel 'ChanOpenInit'. An error is returned if the port identifier is already in use
// An `OnChannelOpenInit` event is emitted which can be picked up by an offchain process such as a relayer
function InitInterchainAccount(connectionId: string, counterPartyConnectionId: string, ownerAddress: string) returns (nil){
}

// TrySendTx is used to send an IBC packet containing instructions (messages) to an interchain account on a host chain for a given interchain account owner. 
function TrySendTx(connectionId: string, counterPartyConnectionId: string, ownerAddress: string, messages: []bytes) returns ([]bytes, error){
    // A port id string will be generated from the connectionIds + ownerAddress. This is the port-id that the IBC packet will be sent on
    // A call to GetActiveChannel() checks if there is a currently active channel for this port-id which also implies an interchain account has previously be registered
    // A call to check if the channel status is OPEN (an additional, albeit slightly redundant check)
    // if there are no errors CreateOutgoingPacket() is called and the IBC packet will be sent to the host chain on the active channel
}

// This helper function is required for the controller chain to get the address of a newly registered interchain account on a host chain.
// Because the registration of an interchain account happens during the channel creation handshake, there is no way for the controller chain to know what the address of the interchain account is on the host chain in advance. 
// This function sends an IBC packet to the host chain, on the owner port + active channel with the sole intention of eventually parsing the interchain account address from the Acknowledgement packet on the controller chain side.
// The OnAcknowledgePacket function on the controller chain will handle the parsing + setting the interchain account address in state.
// The controller chain builds the messages (before sending via IBC in the TrySendTx fn) that the host side will eventually execute. Therefore, the interchain account address must be known by the controller chain.
function GetInterchainAccountAddressFromAck(connectionId: string, counterPartyConnectionId: string, ownerAddress: string) returns (nil){
    // Sends a generic IBC packet to the host chain with the intention of parsing the interchain account address associated with this port/connection/channel from the Acknowledgement packet.
}

// Opens a new channel on a particular port given a connection
// This is a helper function to open a new channel 
// This is a safety function in case of a channel closing and the controller chain needs to regain access to an interchain account on the host chain 
function InitChannel(portId: string, connectionId: string) returns (nil){
  // An `OnChannelOpenInit` event is emitted which can be picked up by an offchain process such as a relayer which will finish the channel opening handshake
}

// * HOST CHAIN *

// RegisterInterchainAccount is called on the OnOpenTry step during the channel creation handshake 
function RegisterInterchainAccount(counterPartyPortId: string) returns (error){
   // generates an address for the interchain account 
   // checks to make sure the account has not already been registered
   // creates a new address on chain 
   // calls SetInterchainAccountAddress()
   // returns the address of the newly created account
}

// DeserializeTx is used to deserialize message bytes parsed from an IBC packet into a format that the host chain can recognize & execute i.e. cosmos SDK messages
function DeserializeTx(txBytes []byte) returns ([]Any, error){
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
function SetActiveChannel(portId: string, channelId: string) returns (error){
}

// Returns the id of the active channel if present
function GetActiveChannel(portId: string) returns (string, boolean){
}

// Stores the address of the interchain account in state
function SetInterchainAccountAddress(portId string, address string) returns (string) {
}

// Gets the interchain account from state
function GetInterchainAccountAddress(portId string) returns (string, error){
}
```

### Registering & Controlling flows
**Active Channels**

The controller and host chain should keep track of an `active-channel` for each registered interchain account. The `active-channel` is set during the channel creation handshake process. This is a safety mechanism that allows a controller chain to regain access to an interchain account on a host chain in case of a channel closing. 

An active channel can look like this:


```typescript
{
 // Controller Chain
 SourcePortId: `ics-27-0-0-cosmos1mjk79fjjgpplak5wq838w0yd982gzkyfrk07am`,
 SourceChannelId: `channel-1`,
 // Host Chain
 CounterPartyPortId: `interchain_account`,
 CounterPartyChannelId: `channel-2`,
}
```

**Register Account Flow**

To register an interchain account we require an off-chain process (relayer) to listen for `OnChannelOpenInit` events with the capability to finish a channel creation handshake on a given connection. 

1. The controller chain binds a new IBC port with an id composed of the *source/counterparty connection-ids* & the *interchain account owner address*

The IBC portID will look like this:
```
ics27-1-{connection-number}-{counterparty-connection-number}-{owner-address}
```
This port will be used to create channels between the controller & host chain for a specific owner/interchain account pair. Only the account with `{owner-address}` matching the bound port will be authorized to send IBC packets over channels created with `ics27-1-{connection-number}-{counterparty-connection-number}-{owner-address}`. The host chain trusts that each controller chain will enforce this port registration and access. 

2. The controller chain emits an event signaling to open a new channel on this port given a connection 
3. A relayer listening for `OnChannelOpenInit` events will begin the channel creation handshake
4. During the `OnChanOpenTry` callback on the host chain an interchain account will be registered and a mapping of the interchain account address to the owner account address will be stored in state (this is used for authenticating transactions on the host chain at execution time)
5. During the `OnChanOpenAck` & `OnChanOpenConfirm` callbacks on the controller & host chains respectively, the `active-channel` for this interchain account/owner pair, is set in state


**Controlling Flow**

Once an interchain account is registered on the host chain a controller chain can begin sending instructions (messages) to control this account. 

1. The controller chain calls `GetInterchainAccountAddressFromAck()` to get the address of the interchain account on the host chain and sets the address in state mapped to the respective portID. 
2. The controller chain calls `TrySendTx` and passes message(s) that will be executed on the host side by the associated interchain account (determined by the source port identifer)

Cosmos SDK psuedo code example:

```typescript
interchainAccountAddress := GetInterchainAccountAddress(portId)
msg := &banktypes.MsgSend{FromAddress: interchainAccountAddress, ToAddress: ToAddress, Amount: amount}
// Sends the message to the host chain, where it will eventually be executed 
TrySendTx(ownerAddress, connectionId, counterPartyConnectionId, msg)
```

4. The host chain upon receiving the IBC packet will call `DeserializeTx` and then call `AuthenticateTx` for each message. If either of these steps fails an error will be returned.
5. The host chain will then call `ExecuteTx` for each message and return an acknowledgment



### Packet Data
`InterchainAccountPacketData` contains an array of messages that an interchain account can execute and a memo string that is sent to the host chain.  

```typescript
message InterchainAccountPacketData  {
    repeated google.protobuf.Any messages = 1;
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

The ```InterchainAccountHook``` interface allows the controller chain to receive results of executed transactions on the host chain.
```typescript
interface InterchainAccountHook {
  onTxSucceeded(sourcePort:string, sourceChannel:string, txBytes: Uint8Array)
  onTxFailed(sourcePort:string, sourceChannel:string, txBytes: Uint8Array)
}
```

### Port & channel setup

The interchain account module on a host chain must always bind to a port with the id `interchain-account`. Controller chains will bind to ports dynamically, with each port id set as `ics27-1-{connection-number}-{counterparty-connection-number}-{owner-address}`.

The example below assumes a module is implementing the entire `InterchainAccountModule` interface. The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialized) to bind to the appropriate port.

```typescript
function setup() {
  capability = routingModule.bindPort("interchain_account", ModuleCallbacks{
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
- The version string is "ics27-1"

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
  // only allow channels to "interchain_account" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "interchain-account")
  // version not used at present
  abortTransactionUnless(version === "ics27-1")
  // Only open the channel if there is no active channel already set (with status OPEN)
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

  // assert that version is "ics27-1"
  abortTransactionUnless(version === "ics27-1")
  abortTransactionUnless(counterpartyVersion === "ics27-1")
  // create an interchain account 
  createInterchainAccount(counterpartyPortIdentifier, counterpartyChannelIdentifier)
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // port has already been validated
  // assert that version is "ics27-1"
  abortTransactionUnless(version === "ics27-1")
  // state change to keep track of successfully registered interchain account
  setActiveChannel(SourcePortId)
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  setActiveChannel(CounterPartyPortId)
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // users should not be able to close channels
    return nil
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
   // users should not be able to close channels
   return nil
}
```

### Packet relay
`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function OnRecvPacket(
	packet channeltypes.Packet,
) {
    ack = channeltypes.NewResultAcknowledgement([]byte{byte(1)})

    var data types.IBCAccountPacketData
    if err = UnmarshalJSON(packet.GetData(), &data); err != nil {
	ack = channeltypes.NewErrorAcknowledgement(fmt.Sprintf("cannot unmarshal ICS-27 interchain account packet data: %s", err.Error()))
    }

    // only attempt the application logic if the packet data
    // was successfully decoded
    if ack.Success() {
        err = am.keeper.OnRecvPacket(ctx, packet)
        if err != nil {
            ack = channeltypes.NewErrorAcknowledgement(err.Error())
        }   
        // If the packet sequence is 1 add the interchain account address to the 
        if packet.Seqeunce = 1 {
            var interchainAccountAddress = GetInterchainAccountAddress(packet.CounterPartyPortId)
            ack = channeltypes.NewResultAcknowledgement([]byte{byte(interchainAccountAddress)})
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
    if (acknowledgement.result) {
      onTxSucceeded(packet.sourcePort, packet.sourceChannel, packet.data.data)
      // If the packet sequence is 1 set the interchain account address in state
      if packet.Sequence == 1 {
          setInterchainAccountAddress(sourcePort, String(acknowledgement.result))
      }
    } else {
      onTxFailed(packet.sourcePort, packet.sourceChannel, packet.data.data)
    }
    return
}
```

```typescript
function onTimeoutPacket(packet: Packet) {
  // Receiving chain should handle this event as if the tx in packet has failed
    onTxFailed(packet.sourcePort, packet.sourceChannel, packet.data.data)
    return
}
```

## Example Implementation

Repository for Cosmos-SDK implementation of ICS27: https://github.com/cosmos/interchain-accounts

## History

Aug 1, 2019 - Concept discussed

Sep 24, 2019 - Draft suggested

Nov 8, 2019 - Major revisions

Dec 2, 2019 - Minor revisions (Add more specific description & Add interchain account on Ethereum)

July 14, 2020 - Major revisions

April 27, 2021 - Redesign of ics27 specification

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
