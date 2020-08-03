---
ics: 27
title: Interchain Accounts
stage: Draft
category: IBC/TAO
requires: 25, 26
kind: instantiation
author: Tony Yun <tony@chainapsis.com>, Dogemos <josh@tendermint.com>
created: 2019-08-01
modified: 2020-07-14
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the account management system over an IBC channel between separate chains.

### Motivation

On Ethereum, there are two types of accounts: externally owned accounts, controlled by private keys, and contract accounts, controlled by their contract code ([ref](https://github.com/ethereum/wiki/wiki/White-Paper)). Similar to Ethereum's CA (contract accounts), interchain accounts are managed by another chain while retaining all the capabilities of a normal account (i.e. stake, send, vote, etc). While an Ethereum CA's contract logic is performed within Ethereum's EVM, interchain accounts are managed by another chain via IBC in a way such that the owner of the account retains full control over how it behaves.

### Definitions

The IBC handler interface & IBC relayer module interface are as defined in [ICS 25](../ics-025-handler-interface) and [ICS 26](../ics-026-routing-module), respectively.

### Desired Properties

- Permissionless
- Fault containment: Interchain account must follow rules of its host chain, even in times of Byzantine behaviour by the counterparty chain (the chain that manages the account)
- The chain that controls the account must process the results asynchronously and according to the chain's logic. The result's code should be 0 if the transaction was successful and an error code other than 0 if the transaction failed.
- Sending and receiving transactions will be processed in an ordered channel where packets are delivered exactly in the order which they were sent.

## Technical Specification

The implementation of interchain account is non-symmetric. This means that each chain can have a different way to generate an interchain account and deserialise the transaction bytes and a different set of transactions that they can execute. For example, chains that use the Cosmos SDK will deserialise tx bytes using Amino or Protobuf, but if the counterparty chain is a smart contract on Ethereum, it may deserialise tx bytes by an ABI that is a minimal serialisation algorithm for the smart contract.
The interchain account specification defines the general way to register an interchain account and transfer tx bytes. The counterparty chain is responsible for deserialising and executing the tx bytes, and the sending chain should know how counterparty chain will handle the tx bytes in advance.

Each chain must satisfy following features to create a interchain account:

- New interchain accounts must not conflict with existing ones.
- Each chain must keep track of which counterparty chain created each new interchain account.

Also, each chain must know how the counterparty chains serialise/deserialise transaction bytes in order to send transactions via IBC. And the counterparty chain must implement the process of safely executing IBC transactions by verifying the authority of the transaction's signers.

The chain must reject the transaction and must not make a state transition in the following cases:

- The IBC transaction fails to be deserialised.
- The IBC transaction expects signers other than the interchain accounts made by the counterparty chain.

It does not restrict how you can distinguish signers that was not made by the counterparty chain. But the most common way would be to record the account in state when the interchain account is registered and to verify that signers are recorded interchain account.

### Data Structures

Each chain must implement the interfaces as defined in this section in order to support interchain accounts.

`tryRegisterIBCAccount` method in `IBCAccountModule` interface defines the way to request the creation of an IBC account on the host chain (or counterparty chain that the IBC account lives on). The host chain creates an IBC account using its account creation logic, which may use data within the packet (such as destination port, destination channel, etc) and other additional data for the process. The origin chain can receive the address of the account that was created from the acknowledge packet.

`tryRunTx` method in `IBCAccountModule` interface defines the way to create an outgoing packet for a specific `type`. `Type` indicates how the IBC account transaction should be constructed and serialised for the host chain. Generally, `type` indicates what blockchain framework the host chain was built on.

`createAccount` defines the way to determine the account's address by using the packet. If the host chain doesn't support a deterministic way to generate an address with data, it can be generated using the internal logic of the host chain. A newly created interchain account must not conflict with an existing account. Therefore, the host chain (on that the interchain account lives in) must keep track of which blockchains have created an interchain account within the host chain in order to verify the transaction signing authority in `authenticateTx`.

`authenticateTx` validates the transaction and checks that the signers in the transaction have the right permissions. `

`runTx` executes a transaction after the transaction has been successfully authenticated.

```typescript
type Tx = object

interface IBCAccountModule {
  tryRegisterIBCAccount(data: Uint8Array)
  tryRunTx(chainType: Uint8Array, data: any)
  createAccount(packet: Packet, data: Uint8Array): Address
  deserialiseTx(txBytes: Uint8Array): Tx
  authenticateTx(tx: Tx): boolean
  runTx(tx: Tx): Result
}
```

`IBCAccountPacketData` specifies the type of packet. The `IBCAccountPacketData.data` is arbitrary and can be used for different purposes depending on the value of `IBCAccountPacketData.type`.

When the value of `type` is `REGISTER`, `IBCAccountPacketData.data` can be used as information by which the account is created on the host chain. When the value of `type` is `RUNTX`, `IBCAccountPacketData.data` contains the tx bytes.

```typescript
// `Type` enumerator defines the packet type.
enum Type {
  REGISTER,
  RUNTX
}

interface IBCAccountPacketData {
  type: Type
  data: Uint8Array
}
```

The acknowledgement data type describes the type of packet data and chain id and whether the result code of processing and the data of result if it is needed, and the reason for failure (if any).

```typescript
interface IBCAccountPacketAcknowledgement {
    type: Type
    code: uint32
    data: Uint8Array
    error: string
}
```

The ```IBCAccountHook``` interface allows the source chain to receive results of executing transactions on an interchain account.

```typescript
interface IBCAccountHook {
  onAccountCreated(sourcePort:string, sourceChannel:string, address: Address)
  onTxSucceeded(sourcePort:string, sourceChannel:string, txBytes: Uint8Array)
  onTxFailed(sourcePort:string, sourceChannel:string, txBytes: Uint8Array)
}
```

### Subprotocols

The subprotocols described herein should be implemented in an "interchain-account-bridge" module with access to a router and codec (decoder or unmarshaller) for the application and access to the IBC relayer module.

### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port.

```typescript
function setup() {
  capability = routingModule.bindPort("ibcaccount", ModuleCallbacks{
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

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the ibc account module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels
to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion
that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

### Routing module callbacks

### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The other module is bound to the "ibcaccount" port.
- The channel being created is ordered.
- The version string is "ics27-1".

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
  // only allow channels to "ibcaccount" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "ibcaccount")
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
  // only allow channels to "ibcaccount" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "ibcaccount")
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
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated
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

In plain English, between chains `A` and `B`. It will describe only the case that chain A wants to register an Interchain account on chain B and control it. Moreover, this system can also be applied the other way around.

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  IBCAccountPacketData data = packet.data

  switch (data.type) {
  case Type.REGISTER:
    try {
      // Create an account by using the packet's data (destination port, destination channel, etc) and packet data's data.
      const address = createAccount(packet, data.data)

      // Return ack with generated address.
      return IBCAccountPacketAcknowledgement{
        type: Type.REGISTER,
        code: 0,
        data: address,
        error: "",
      }
    } catch (e) {
      // Return ack with error.
      return IBCAccountPacketAcknowledgement{
        type: Type.REGISTER,
        code: 1,
        data: [],
        error: e.message,
      }
    }
  case Type.RUNTX:
    const tx = deserialiseTx(packet.data.txBytes)
    abortTransactionUnless(authenticateTx(tx))
    try {
      const result = runTx(tx)

      return IBCAccountPacketAcknowledgement{
        type: Type.RUNTX,
        code: 0,
        data: result.data,
        error: "",
      }
    } catch (e) {
      // Return ack with error.
      return IBCAccountPacketAcknowledgement{
        type: Type.RUNTX,
        code: e.code || 1,
        data: [],
        error: e.message,
      }
    }
  }
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  switch (ack.type) {
  case Type.REGISTER:
    if (ack.code === 0) {
      onAccountCreated(packet.sourcePort, packet.sourceChannel, ack.data)
    }
    return
  }
  case Type.RUNTX:
    if (ack.code === 0) {
      onTxSucceeded(packet.sourcePort, packet.sourceChannel, packet.data.data)
    } else {
      onTxFailed(packet.sourcePort, packet.sourceChannel, packet.data.data)
    }
    return
}
```

```typescript
function onTimeoutPacket(packet: Packet) {
  // Receiving chain should handle this event as if the tx in packet has failed
  switch (ack.type) {
  case Type.RUNTX:
    onTxFailed(packet.sourcePort, packet.sourceChannel, packet.data.data)
    return
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // nothing is necessary
}
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable.

## Example Implementation

Repository for Cosmos-SDK implementation of ICS27: https://github.com/chainapsis/cosmos-sdk-interchain-account
Pseudocode for cosmos-sdk: https://github.com/everett-protocol/everett-hackathon/tree/master/x/interchain-account
POC for Interchain account on Ethereum: https://github.com/everett-protocol/ethereum-interchain-account

## Other Implementations

(links to or descriptions of other implementations)

## History

Aug 1, 2019 - Concept discussed

Sep 24, 2019 - Draft suggested

Nov 8, 2019 - Major revisions

Dec 2, 2019 - Minor revisions (Add more specific description & Add interchain account on Ethereum)

July 14, 2020 - Major revisions

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).