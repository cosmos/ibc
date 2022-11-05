---
ics: 100
title: Atomic Swap
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Ping Liang <ping@side.one>, Edward Gunawan <edward@s16.ventures>
created: 2022-07-27
modified: 2022-10-07
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the atomic swap of fungible tokens over an IBC channel between two modules on separate chains.

### Motivation

Users may wish to exchange tokens without transfering tokens away from its native chain. ICS-100 enabled chains can facilitate atomic swaps between users and their tokens located on the different chains. This is useful for exchanges between specific users at specific prices, and opens opportunities for new application designs.

### Definitions

`Atomic Swap`: An exchange of tokens from separate chains without transfering tokens from one blockchain to another.

`Order`: an offer to exchange quantity X of token A for quantity Y of token B. Tokens offered are sent to an escrow account (owned by the module)

`Maker`: A user that makes or initiates an order.

`Taker`: Is the counterparty who takes or responds to an order.

`Maker Chain`: The blockchain where a maker makes or initiaties an order.

`Taker Chain`: The blockchain where a taker takes or responds to an order.

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations.
- `Gaurantee of exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.
- `Escrow enabled`: an account owned by the module will hold tokens and facilitate exchange.
- `Refundable`: tokens are refunded by escrow when an order is cancelled
- `Basic orderbook`: a store of orders functioning as an orderbook system

## Technical Specification

### General Design

<img src="./ibcswap.png"/>

A maker offers token A in exchange for token B by making an order. The order specifies the quantity and price of exchange, and sends the offered token A to the maker chain's escrow account.

Any taker on a different chain with   token B can accept the offer by taking the order. The taker sends the desired amount of token B to the taker chain's escrow account.

The escrow account on each respective chain transfers the corresponding token amounts to each user's receiving address, without requiring the usual ibc transfer.

### Data Structures

Only one packet data type is required: `AtomicSwapPacketData`, which specifies the swap message type, data(protobuf marshalled) and a memo field.

```typescript
enum SwapMessageType {
  // Default zero value enumeration
  TYPE_UNSPECIFIED = 0,
  TYPE_MSG_MAKE_SWAP = 1,
  TYPE_MSG_TAKE_SWAP = 2,
  TYPE_MSG_CANCEL_SWAP = 3,
}

// AtomicSwapPacketData is comprised of a swap message type, raw transaction and optional memo field.
interface AtomicSwapPacketData {
  type: SwapMessageType;
  data: types[];
  memo: string;
}
```

All `AtomicSwapPacketData` will be forwarded to the corresponding message handler to execute according to its type. There are 3 types:

```typescript
interface MakeSwap {
  // the port on which the packet will be sent
  source_port string
  // the channel by which the packet will be sent
  source_channel: string;
  // the tokens to be exchanged
  sell_token : Coin
  buy_token: Coin;
  // the sender address
  maker_address: string;
  // the sender's address on the taker chain
  maker_receiving_address string;
  // if desired_taker is specified,
  // only the desired_taker is allowed to take this order
  // this is address on the taker chain
  desired_taker: string;
  create_timestamp: int64;
}
```

```typescript
interface TakeSwap {
  order_id: string;
  // the tokens to be sell
  sell_token: Coin;
  // the sender address
  taker_address: string;
  // the sender's address on the taker chain
  taker_receiving_address: string;
  create_timestamp: int64;
}
```

```typescript
interface CancelSwap {
  order_id: string;
  maker_address: string;
}
```

Both the maker chain and taker chain maintain separate orderbooks. Orders are saved in both maker chain and taker chain.

```typescript
enum Status {
  INITIAL = 0,
  SYNC = 1,
  CANCEL = 2,
  COMPLETE = 3,
}

interface OrderBook {
  id: string;
  maker: MakeSwap;
  status: Status;
  channel_id: string;
  takers: TakeSwap[];
  cancel_timestamp: int64;
  complete_timestamp: int64;
}
```

### Life scope and control flow

#### Making a swap

1. User creates an order on the maker chain with specified parameters (see type `MakeSwap`).  Tokens are sent to the escrow address owned by the module. The order is saved on the maker chain
2. An `AtomicSwapPacketData` is relayed to the taker chain where `onRecvPacket` the order is also saved on the taker chain.  
3. A packet is subsequently relayed back for acknowledgement. A packet timeout or a failure during `onAcknowledgePacket` will result in a refund of the escrowed tokens.

#### Taking a swap

1. A user takes an order on the taker chain by triggering `TakeSwap`.  Tokens are sent to the escrow address owned by the module.
2. An `AtomicSwapPacketData` is relayed to the maker chain where `onRecvPacket` the escrowed tokens are sent to the destination address.  
3. A packet is subsequently relayed back for acknowledgement. Upon acknowledgement escrowed tokens on the taker chain is sent to the related destination address.  A packet timeout or a failure during `onAcknowledgePacket` will result in a refund of the escrowed tokens.

#### Cancelling a swap

1.  The taker cancels a previously created order.
2.  An `AtomicSwapPacketData` is relayed to the maker chain where `onRecvPacket` the order is cancelled on the taker chain.
3.  A packet is relayed back where upon acknowledgement the order on the maker chain is also cancelled.

### Sub-protocols

The sub-protocols described herein should be implemented in a "Fungible Token Swap" module with access to a bank module and to the IBC routing module.

```ts
function createSwap(request MakeSwap) {

}
```

```ts
function fillSwap(request TakeSwap) {

}
```

```ts
function cancelSwap(request CancelSwap) {

}
```

#### Port & channel setup

The fungible token swap module on a chain must always bind to a port with the id `atomicswap`

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("atomicswap", ModuleCallbacks{
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

Once the setup function has been called, channels can be created via the IBC routing module.

#### Channel lifecycle management

An fungible token swap module will accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics100-1`.

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
  // assert that version is "ics100-1" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics100-1" || version === "")

  return "ics100-1", nil
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
  // assert that version is "ics100-1"
  abortTransactionUnless(counterpartyVersion === "ics100-1")

  return "ics100-1", nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string
) {
  // port has already been validated
  // assert that counterparty selected version is "ics31-1"
  abortTransactionUnless(counterpartyVersion === "ics100-1");
}
```

```typescript
function onChanOpenConfirm(portIdentifier: Identifier, channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated, version has already been validated
}
```

```typescript
function onChanCloseInit(portIdentifier: Identifier, channelIdentifier: Identifier) {
  // always abort transaction
  abortTransactionUnless(FALSE);
}
```

```typescript
function onChanCloseConfirm(portIdentifier: Identifier, channelIdentifier: Identifier) {
  // no action necessary
}
```

#### Packet relay

`sendAtomicSwapPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function sendAtomicSwapPacket(swapPacket AtomicSwapPacketData) {

    // send packet using the interface defined in ICS4
    handler.sendPacket(
      getCapability("port"),
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      swapPacket.getBytes(), // Should be proto marshalled bytes.
    )
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(data: AtomicSwapPacketData) {
  switch data.Type {
  case TYPE_MSG_MAKE_SWAP:
    var msg types.MsgMakeSwapRequest

    if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
      return err
    }
    if err := k.executeMakeSwap(ctx, packet, &msg); err != nil {
      return err
    }

    return nil

  case TYPE_MSG_TAKE_SWAP:
    var msg types.MsgTakeSwapRequest

    if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
      return err
    }
    if err2 := k.executeTakeSwap(ctx, packet, &msg); err2 != nil {
      return err2
    } else {
      return nil
    }

  case TYPE_MSG_CANCEL_SWAP:
    var msg types.MsgCancelSwapRequest

    if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
      return err
    }
    if err2 := k.executeCancelSwap(ctx, packet, &msg); err2 != nil {
      return err2
    } else {
      return nil
    }

  default:
    return types.ErrUnknownDataPacket
  }

  ctx.EventManager().EmitTypedEvents(&data)

  return nil
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: AtomicSwapPacketData,
  acknowledgement: bytes) {
switch ack.Response.(type) {
  case *channeltypes.Acknowledgement_Error:
    return k.refundPacketToken(ctx, packet, data)
  default:
    switch data.Type {
    case TYPE_MSG_TAKE_SWAP:
      var msg types.MsgTakeSwapRequest

      if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
        return err
      }
      // check order status
      if order, ok := k.GetLimitOrder(ctx, msg.OrderId); ok {
        k.executeTakeSwap(ctx, order, &msg, StepAcknowledgement)
      } else {
        return types.ErrOrderDoesNotExists
      }
      break

    case TYPE_MSG_CANCEL_SWAP:
      var msg types.MsgCancelSwapRequest

      if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
        return err
      }
      if err2 := k.executeCancel(ctx, &msg, StepAcknowledgement); err2 != nil {
        return err2
      } else {
        return nil
      }
      break
    }
  }
  return nil
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that the tokens will be refunded).

```typescript
function onTimeoutPacket(packet: AtomicSwapPacketData) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet);
}
```

`refundTokens` is called by both `onAcknowledgePacket` on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original owner.

```typescript
function refundTokens(packet: AtomicSwapPacketData) {
  FungibleTokenPacketData data = packet.data
  //send tokens from module to message sender
}
```

```typescript
function onTimeoutPacketClose(packet: AtomicSwapPacketData) {
  // can't happen, only unordered channels allowed
}
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version "ics100-1" in the channel handshake.

A future version of this standard could use a different version in the channel handshake,
and safely alter the packet data format & packet handler semantics.

## Example Implementation

https://github.com/ibcswap/ibcswap

## Other Implementations

Coming soon.

## History

Aug 15, 2022 - Draft written

Oct 6, 2022 - Draft revised

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
