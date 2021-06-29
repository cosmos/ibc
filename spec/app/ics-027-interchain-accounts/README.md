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

On Ethereum, there are two types of accounts: externally owned accounts, controlled by private keys, and contract accounts, controlled by their contract code ([ref](https://github.com/ethereum/wiki/wiki/White-Paper)). Similar to Ethereum's contract accounts, interchain accounts are controlled by another chain (not a private key) while retaining all the capabilities of a normal account (i.e. stake, send, vote, etc). While an Ethereum CA's contract logic is performed within Ethereum's EVM, interchain accounts are managed by a seperate chain via IBC in a way such that the owner of the account retains full control over how it behaves. ICS27-1 primarily targets the use cases of DAO investing and staking derivatives over IBC.

### Definitions 

- Interchain Account: An account on a host chain. An interchain account has all the capabilites of a normal account. However, rather than signing transactions with a private key, a controller chain will send IBC packets to the host chain which signal what transactions the interchain account should execute 
- Interchain Account Owner: An account on the controller chain. Every interchain account on a host chain has a respective owner account on the controller chain 
- Controller Chain: The chain registering and controlling an account on a host chain. The controller chain sends IBC packets to the host chain in order to control the account
- Host Chain: The chain where the interchain account is registered. The host chain listens for IBC packets from a controller chain which should contain instructions (e.g. cosmos SDK messages) that the interchain account will execute

The IBC handler interface & IBC relayer module interface are as defined in [ICS 25](../ics-025-handler-interface) and [ICS 26](../ics-026-routing-module), respectively.

### Desired Properties

- Permissionless 
- Fault containment: An interchain account must follow rules of its host chain, even in times of Byzantine behavior by the controller chain (the chain that manages the account)
- Sending and receiving transactions will be processed in an ordered channel where packets are delivered exactly in the order in which they were sent
- If a channel closes, the controller chain must be able to regain access to registered interchain accounts by simply opening a new channel
- Each interchain account is owned by a single account on the controller chain. Only the owner account on the controller chain is authorized to control the interchain account

## Technical Specification

A chain can implement one or both parts to the interchain accounts protocol (controlling and hosting). A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its own chain, and vice versa. 

This specification defines the general way to register an interchain account and transfer tx bytes. The host chain is responsible for deserialising and executing the tx bytes, and the controller chain should know how the host chain will handle the tx bytes in advance (Cosmos SDK chains will deserialize using Protobuf). 

### Authentication & Authorization

For the controller chain to register an interchain account on a host chain, first the controlling side must bind a new port to the interchain account module with the id `ics27-1-{owner-address}` where `{owner-address}` is the account address of the interchain account owner. This port will be used to create channels between the controller & host chain for a specific owner/interchain account pair. Only the account with `{owner-address}` matching the bound port will be authorized to send IBC packets over channels created with `SourcePort` `ics27-1-{owner-address}`. The host chain trusts that each controller chain will enforce this port registration and access.


In the case of a channel closing, both the controller and host chain should keep track of an `active-channel` for each registered interchain account. The `active-channel` is set during the channel creation handshake process. If a channel closes, a new channel can be opened with the same source port, allowing access to the interchain account on the host chain. 

An active channel can look like:


```typescript
{
 // Controller Chain
 SourcePortId: `ics-27-1-cosmos1mjk79fjjgpplak5wq838w0yd982gzkyfrk07am`,
 SourceChannelId: `channel-1`,
 // Host Chain
 CounterPartyPortId: `interchain-accounts`,
 CounterPartyChannelId: `channel-2`,
}
````

### Data Structures

```typescript
interface InterchainAccountModule {
  // Controlling side
  initInterchainAccount(owner: string) // binds a new port with id `ics27-1-{owner-address}`
  tryRunTx(owner: string, data: any) // sends an IBC packet containing tx bytes over the active channel for the respective owner
  // Host side
  createInterchainAccount(): Address 
  deserialiseTx(txBytes: Uint8Array): Tx
  runTx(tx: Tx): Result
}
```

**A chain can implement the entire interface, or decide to implement only the sending or receiving parts of the protocol.**


#### Sending Interface
The `initInterchainAccount` method in the `InterchainAccountModule` interface defines how the controller chain requests the creation of an interchain account on a host chain. Calling `initInterchainAccount`  binds a new port with id `ics27-1-{owner-address}` and calls `OpenChanInit` via the IBC module which will initiate the handshake process and emit an event signaling to a relayer to create a new channel between both chains. The host chain can then create the interchain account in the `ChanOpenTry` callback as part of the channel creation handshake process defined in [ics-4](https://github.com/cosmos/ibc/tree/master/spec/core/ics-004-channel-and-packet-semantics). Once the `chanOpenAck` callback is successful the handshake-originating (controller) chain can assume the account registration is succesful.  


The `tryRunTx` method in the`InterchainAccountModule` creates an outgoing IBC packet containing tx bytes (e.g. cosmos SDK messages) that a specifc interchain account should execute.  

#### Recieving Interfacce

`createInterchainAccount`  A newly created interchain account must not conflict with an existing account. `createInterchainAccount` should be called in the `chanOpenTry` callback as part of the channel creation handshake process defined in [ics-4](https://github.com/cosmos/ibc/tree/master/spec/core/ics-004-channel-and-packet-semantics).

`runTx` executes a transaction based on the IBC packet recieved from the controller chain.

### Packet Data
`InterchainAccountPacketData` contains an array of messages that an interchain account can execute and a memo string that is sent to the host chain.  

```typescript
message InterchainAccountPacketData  {
    repeated google.protobuf.Any messages = 1;
    string memo = 2;
}
```

The acknowledgement packet structure is defined as in [ics4](https://github.com/cosmos/cosmos-sdk/blob/v0.42.4/proto/ibc/core/channel/v1/channel.proto#L134-L147). If an error occurs on the host chain the acknowledgement should contain the error message.

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
The interchain account module on a host chain must always bind to a port with the id `interchain_account`. Controller chains will bind to ports dynamically, with each port id set as `ics27-1-{owner-address}`.

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
  // version not used at present
  abortTransactionUnless(version === "ics27-1")
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
  // no action necessary
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

### Packet relay
`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  InterchainAccountPacketData data = packet.data
    const tx = deserialiseTx(packet.data.txBytes)
    abortTransactionUnless(authenticateTx(tx))
    try {
      const result = runTx(tx)

      return Acknowledgement{
        result: result
      }
    } catch (e) {
      // Return ack with error.
      return Acknowledgement{
        error: e.message,
    }
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
    if (acknowledgement.result) {
      onTxSucceeded(packet.sourcePort, packet.sourceChannel, packet.data.data)
    } else {
      onTxFailed(packet.sourcePort, packet.sourceChannel, packet.data.data)
    }
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

