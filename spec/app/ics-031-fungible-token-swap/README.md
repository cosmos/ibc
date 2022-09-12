---
ics: 31
title: Fungible Token Swap
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Ping Liang <18786721@qq.com>
created: 2022-07-27 
modified: 2022-07-27
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe multi-chain denomination handling with permissionless channel opening. This logic constitutes a "fungible token swap module", interfacing between the IBC routing module and an existing asset tracking module on the host state machine.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilise an asset issued on one chain on another chain, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing chain. This application-layer standard describes a protocol for transferring fungible tokens between chains connected with IBC which preserves asset fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

### Definitions

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

### Desired Properties

- Preservation of fungibility (two-way peg).
- Preservation of total supply (constant or inflationary on a single source chain & module).
- Permissionless token transfers, no need to whitelist connections, modules, or denominations.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-inflation of tokens originating on chain `A`, as a result of chain `B`'s Byzantine behaviour (though any users who sent tokens to chain `B` may be at risk).

## Technical Specification

### Data Structures

Only one packet data type is required: `AtomicSwapPacketData`, which specifies the type of swap message, data(protobuf marshalled) and memo.

```typescript
enum SwapMessageType {
  // Default zero value enumeration
  TYPE_UNSPECIFIED = 0,

  TYPE_MSG_MAKE_SWAP = 1, 
  TYPE_MSG_TAKE_SWAP = 2,
  TYPE_MSG_CANCEL_SWAP = 3,
}

// AtomicSwapPacketData is comprised of a raw transaction, type of transaction and optional memo field.
interface AtomicSwapPacketData {
  type: SwapMessageType;
  data: types[];
  memo: string;
}

```

所有的`AtomicSwapPacketData`会根据它的类型转发到相应的Message handler去execute。共有3种类型，他们是：


```typescript
interface MakeSwap {
  // the port on which the packet will be sent
  source_port string
  // the channel by which the packet will be sent
  source_channel: string;
  // the tokens to be sell
  sell_token : Coin
  buy_token: Coin;
  // the sender address
  maker_address: string;
  // the sender's address on the destination chain
  maker_receiving_address string;
  // if desired_taker is specified,
  // only the desired_taker is allowed to take this order
  // this is address on destination chain
  desired_taker: string;
  create_timestamp: int64;
}

```

```typescript
interface TakeSwap {
  order_id: string
  // the tokens to be sell
  sell_token: Coin;
  // the sender address
  taker_address: string;
  // the sender's address on the destination chain
  taker_receiving_address: string;
  create_timestamp: int64
}
```

```typescript
interface CancelSwap {
  order_id: string;
  maker_address: string;
}
```


Both chains(source chain and destination chain) maintain a seperated order book in state, 
```typescript
enum Status {
  INITIAL = 0,
  SYNC = 1,
  CANCEL = 2,
  COMPLETE = 3,
}

enum FillStatus {
  NONE_FILL = 0,
  PARTIAL_FILL = 1,
  COMPLETE_FILL = 2,
}

interface OrderBook {
  id: string
  maker: MakeSwap
  status: Status
  fill_status: FillStatus
  channel_id: string
  takers: TakeSwap[]
  cancel_timestamp: int64
  complete_timestamp: int64
}


```
### Life scope and control flow

<img src="./ibcswap.png"/>

### Sub-protocols

The sub-protocols described herein should be implemented in a "fungible token transfer bridge" module with access to a bank module and to the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("bank", ModuleCallbacks{
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

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the fungible token transfer module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels
to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion
that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics20-1`.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics20-1" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics20-1" || version === "")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
  return "ics20-1", nil
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
  counterpartyVersion: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics20-1"
  abortTransactionUnless(counterpartyVersion === "ics20-1")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
  // return version that this chain will use given the
  // counterparty version
  return "ics20-1", nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  // port has already been validated
  // assert that counterparty selected version is "ics20-1"
  abortTransactionUnless(counterpartyVersion === "ics20-1")
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

##### Packet relay

In plain English, between chains `A` and `B`:

- When acting as the source zone, the bridge module escrows an existing local asset denomination on the sending chain and mints vouchers on the receiving chain.
- When acting as the sink zone, the bridge module burns local vouchers on the sending chains and unescrows the local asset denomination on the receiving chain.
- When a packet times-out, local assets are unescrowed back to the sender or vouchers minted back to the sender appropriately.
- Acknowledgement data is used to handle failures, such as invalid denominations or invalid destination accounts. Returning
  an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending chain
  to take appropriate action based on the nature of the failure.

`sendFungibleTokens` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function sendFungibleTokens(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64) {
    prefix = "{sourcePort}/{sourceChannel}/"
    // we are the source if the denomination is not prefixed
    source = denomination.slice(0, len(prefix)) !== prefix
    if source {
      // determine escrow account
      escrowAccount = channelEscrowAddresses[sourceChannel]
      // escrow source tokens (assumed to fail if balance insufficient)
      bank.TransferCoins(sender, escrowAccount, denomination, amount)
    } else {
      // receiver is source chain, burn vouchers
      bank.BurnCoins(sender, denomination, amount)
    }

    // create FungibleTokenPacket data
    data = FungibleTokenPacketData{denomination, amount, sender, receiver}

    // send packet using the interface defined in ICS4
    handler.sendPacket(
      getCapability("port"),
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      data
    )
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the packets were prefixed by the sending chain
  source = data.denom.slice(0, len(prefix)) === prefix
  if source {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // unescrow tokens to receiver (assumed to fail if balance insufficient)
    err = bank.TransferCoins(escrowAccount, data.receiver, data.denom.slice(len(prefix)), data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
  } else {
    prefix = "{packet.destPort}/{packet.destChannel}/"
    prefixedDenomination = prefix + data.denom
    // sender was source, mint vouchers to receiver (assumed to fail if balance insufficient)
    err = bank.MintCoins(data.receiver, prefixedDenomination, data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
  }
  return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // if the transfer failed, refund the tokens
  if (!ack.success)
    refundTokens(packet)
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
}
```

`refundTokens` is called by both `onAcknowledgePacket`, on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original sender.

```typescript
function refundTokens(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the denomination is not prefixed
  source = data.denom.slice(0, len(prefix)) !== prefix
  if source {
    // sender was source chain, unescrow tokens back to sender
    escrowAccount = channelEscrowAddresses[packet.srcChannel]
    bank.TransferCoins(escrowAccount, data.sender, data.denom, data.amount)
  } else {
    // receiver was source chain, mint vouchers back to sender
    bank.MintCoins(data.sender, data.denom, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // can't happen, only unordered channels allowed
}
```


## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version "ics31-1" in the channel handshake.

A future version of this standard could use a different version in the channel handshake,
and safely alter the packet data format & packet handler semantics.

## Example Implementation

https://github.com/sideprotocol/ibcswap

## Other Implementations

Coming soon.

## History

Aug 15, 2022 - Draft written

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
