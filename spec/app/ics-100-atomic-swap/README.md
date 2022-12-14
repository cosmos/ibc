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

`Order`: An offer to exchange quantity X of token A for quantity Y of token B. Tokens offered are sent to an escrow account (owned by the module).

`Maker`: A user that makes or initiates an order.

`Taker`: Is the counterparty who takes or responds to an order.

`Maker Chain`: The blockchain where a maker makes or initiaties an order.

`Taker Chain`: The blockchain where a taker takes or responds to an order.

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations.
- `Guarantee of exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.
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
interface MakeSwapMsg {
  // the port on which the packet will be sent, specified by the maker when the order is created
  source_port string
  // the channel by which the packet will be sent, specified by the maker when the order is created
  source_channel: string;
  // the tokens to be exchanged
  sell_token : Coin
  buy_token: Coin;
  // the maker's address
  maker_address: string;
  // the maker's address on the taker chain
  maker_receiving_address string;
  // if desired_taker is specified,
  // only the desired_taker is allowed to take this order
  // this is the address on the taker chain
  desired_taker: string;
  create_timestamp: int64;
  expired_timestamp: int64;
  timeout_height: int64,
  timeout_timestamp: int64,
}
```

```typescript
interface TakeSwapMsg {
  order_id: string;
  // the tokens to be sell
  sell_token: Coin;
  // the taker's address
  taker_address: string;
  // the taker's address on the maker chain
  taker_receiving_address: string;
  create_timestamp: int64;
  timeout_height: int64,
  timeout_timestamp: int64,
}
```

```typescript
interface TakeCancelMsg {
  order_id: string;
  maker_address: string;
  timeout_height: int64,
  timeout_timestamp: int64,
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
  maker: MakeSwapMsg;
  status: Status;
  // set onRecieved(), Make sure that the take order can only be sent to the chain the make order came from
  port_id_on_taker_chain: string;
  // set onRecieved(), Make sure that the take order can only be sent to the chain the make order came from
  channel_id_on_taker_chain: string;
  taker: TakeSwap;
  cancel_timestamp: int64;
  complete_timestamp: int64;
  
  createOrder(msg: MakeSwapMsg) OrderBook {
    return {
        id : generateOrderId(msg)
        status: Status.INITIAL
        maker: msg,
    }
  }
}

// Order id is a global unique string
function generateOrderId(msg MakeSwapMsg) {
    cosnt bytes = protobuf.encode(msg)
    return sha265(bytes)
}


```

### Life scope and control flow

#### Making a swap

1. User creates an order on the maker chain with specified parameters (see type `MakeSwap`).  Tokens are sent to the escrow address owned by the module. The order is saved on the maker chain
2. An `AtomicSwapPacketData` is relayed to the taker chain where `onRecvPacket` the order is also saved on the taker chain.  
3. A packet is subsequently relayed back for acknowledgement. A packet timeout or a failure during `onAcknowledgePacket` will result in a refund of the escrowed tokens.

#### Taking a swap

1. A user takes an order on the taker chain by triggering `TakeSwap`.  Tokens are sent to the escrow address owned by the module.  An order cannot be taken if the current time is later than the `expired_timestamp`
2. An `AtomicSwapPacketData` is relayed to the maker chain where `onRecvPacket` the escrowed tokens are sent to the destination address.  
3. A packet is subsequently relayed back for acknowledgement. Upon acknowledgement escrowed tokens on the taker chain is sent to the related destination address.  A packet timeout or a failure during `onAcknowledgePacket` will result in a refund of the escrowed tokens.

#### Cancelling a swap

1.  The maker cancels a previously created order.  Expired orders can also be cancelled.
2.  An `AtomicSwapPacketData` is relayed to the taker chain where `onRecvPacket` the order is cancelled on the taker chain. If the order is in the process of being taken (a packet with `TakeSwapMsg` is being relayed from the taker chain to the maker chain), the cancellation will be rejected.
3.  A packet is relayed back where upon acknowledgement the order on the maker chain is also cancelled.  The refund only occurs if the taker chain confirmed the cancellation request.

### Sub-protocols

The sub-protocols described herein should be implemented in a "Fungible Token Swap" module with access to a bank module and to the IBC routing module.

```ts
function makeSwap(request MakeSwapMsg) {
    const balance = bank_keeper.getBalances(request.make_address)
    abortTransactionUnless(balance.amount > request.sell_token.Amount)
    // gets escrow address by source port and source channel
    const escrowAddr = escrowAddress(request.sourcePort, request.sourceChannel)
    // locks the sell_token to the escrow account
    const err = bankkeeper.sendCoins(request.maker_address, escrowAddr, request.sell_token)
    abortTransactionUnless(err == null)
    // contructs the IBC data packet
    const packet = {
        type: SwapMessageType.TYPE_MSG_MAKE_SWAP,
        data: protobuf.encode(request), // encode the request message to protobuf bytes.
        memo: "",
    }
    sendAtomicSwapPacket(packet, request.source_port, request.source_channel, request.timeout_height, request.timeout_timestamp)
    
    // creates and saves order on the maker chain.
    const order = OrderBook.createOrder(msg)
    //saves order to store
    store.save(order)
}
```

```ts
function takeSwap(request TakeSwapMsg) {
    const order = OrderBook.findOrderById(request.order_id)
    abortTransactionUnless(order != null)
    abortTransactionUnless(order.expired_timestamp < Now().timestamp())
    abortTransactionUnless(order.maker.buy_token.denom === request.sell_token.denom)
    abortTransactionUnless(order.maker.buy_token.amount === request.sell_token.amount)
    abortTransactionUnless(order.taker == null)
    
    const balance = bank_keeper.getBalances(request.taker_address)
    abortTransactionUnless(balance.amount > request.sell_token.Amount)
    // gets the escrow address by source port and source channel
    const escrowAddr = escrowAddress(request.sourcePort, request.sourceChannel)
    // locks the sell_token to the escrow account
    const err = bankkeeper.sendCoins(request.taker_address, escrowAddr, request.sell_token)
    abortTransactionUnless(err == null)
    // constructs the IBC data packet
    const packet = {
        type: SwapMessageType.TYPE_MSG_TAKE_SWAP,
        data: protobuf.encode(request), // encode the request message to protobuf bytes.
        memo: "",
    } 
    sendAtomicSwapPacket(packet, order.port_id_on_taker_chain, order.channel_id_on_taker_chain, request.timeout_height, request.timeout_timestamp)
    
    //update order state
    order.taker = request // mark that the order has been occupied
    store.save(order)
}
```


```ts
function cancelSwap(request TakeCancelMsg) {
    const order = OrderBook.findOrderById(request.order_id)
    // checks if the order exists
    abortTransactionUnless(order != null)
    // make sure the sender is the maker of the order.
    abortTransactionUnless(order.maker.maker_address == request.maker_address)
    abortTransactionUnless(order.status == Status.SYNC || order.status == Status.INITIAL)
    
    // constructs the IBC data packet
    const packet = {
        type: SwapMessageType.TYPE_MSG_CANCEL_SWAP,
        data: protobuf.encode(request), // encode the request message to protobuf bytes.
        memo: "",
    } 
    // the request is sent to the taker chain, and the taker chain decides if the cancel order is accepted or not
    // the cancelation can only be sent to the same chain as the make order.
    sendAtomicSwapPacket(packet, order.maker.source_port_id, order.maker.source_channel_id request.timeout_height, request.timeout_timestamp)
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
  // assert that counterparty selected version is "ics100-1"
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
function sendAtomicSwapPacket(
    swapPacket AtomicSwapPacketData, 
    sourcePort, sourceChannel, timeoutHeight, timeoutTimestamp: int64
) {
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
function onRecvPacket(packet channeltypes.Packet) {
  switch packet.type {
      case TYPE_MSG_MAKE_SWAP:
        const make_msg = protobuf.decode(packet.bytes)
        
        // check if buy_token is native token on the taker chain
        const supply = bank_keeper.getSuppy(make_msg.buy_token.denom)
        abortTransactionUnless(supply > 0)
        
        // create and save order on the taker chain.
        const order = OrderBook.createOrder(msg)
        order.status = Status.SYNC
        order.port_id_on_taker_chain = packet.destinationPort
        order.channel_id_on_taker_chain = packet.destinationChannel
        //saves order to store
        store.save(order)
        break;
      case TYPE_MSG_TAKE_SWAP:
        const take_msg = protobuf.decode(packet.bytes)
        const order = OrderBook.findOrderById(take_msg.order_id)
        abortTransactionUnless(order != null)
        abortTransactionUnless(order.status == Status.SYNC)
        abortTransactionUnless(order.expired_timestamp < Now().timestamp())
        abortTransactionUnless(take_msg.sell_token.denom == order.maker.buy_token.denom)
        abortTransactionUnless(take_msg.sell_token.amount == order.maker.buy_token.amount)
        
        // send maker.sell_token to taker's receiving address
        bank_keeper.sendCoins(escrowAddr, take_msg.taker_receiving_address, order.maker.sell_token)
        
        // update status of order
        order.status = Status.COMPLETE
        order.taker = take_msg
        order.complete_timestamp = take_msg.create_timestamp
        store.save(order)
        break;
      case TYPE_MSG_CANCEL_SWAP:
        const cancel_msg = protobuf.decode(packet.bytes)
        const order = OrderBook.findOrderById(cancel_msg.order_id)
        abortTransactionUnless(order != null)
        abortTransactionUnless(order.status == Status.SYNC || order.status == Status.INITIAL)
        abortTransactionUnless(order.taker != null) // the maker order has not been occupied 
        
        // update status of order
        order.status = Status.CANCEL
        order.cancel_timestamp = cancel_msg.create_timestamp
        store.save(order)
        break;
      default:
        throw new Error("ErrUnknownDataPacket")
  }
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: AtomicSwapPacketData,
  acknowledgement: bytes) {
  // ack is failed
  if (!ack.success) {
    refundToken(packet) 
  } else {
    switch packet.type {
      case TYPE_MSG_MAKE_SWAP:
        const make_msg = protobuf.decode(packet.bytes)
        
        // update order status on the maker chain.
        const order = OrderBook.findOrderById(make_msg)
        order.status = Status.SYNC
        //save order to store
        store.save(order)
        break;
      case TYPE_MSG_TAKE_SWAP:
        const take_msg = protobuf.decode(packet.bytes)
        
        // update order status on the taker chain.
        const order = OrderBook.findOrderById(take_msg.order_id)
        order.status = Status.COMPLETE
        order.taker = take_msg
        order.complete_timestamp = take_msg.create_timestamp
        store.save(order)
        
        //send tokens to maker
        bank_keeper.sendCoins(escrowAddr, order.maker.maker_receiving_address, take_msg.sell_token)
        break;
      case TYPE_MSG_CANCEL_SWAP:
        const cancel_msg = protobuf.decode(packet.bytes)
        
        // update order status on the maker chain.
        const order = OrderBook.findOrderById(cancel_msg.order_id)
        // update state on maker chain
        order.status = Status.CANCEL
        order.cancel_timestamp = cancel_msg.create_timestamp
        store.save(order)
        
        //send tokens back to maker
        bank_keeper.sendCoins(escrowAddr, order.maker.maker_address, order.maker.sell_token)
        break;
      default:
        throw new Error("ErrUnknownDataPacket")
  }
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that the tokens will be refunded).

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet);
}
```

`refundTokens` is called by both `onAcknowledgePacket` on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original owner.

```typescript
function refundTokens(packet: Packet) {
  AtomicSwapPacketData data = packet.data
  //send tokens from module to message sender
  cosnt order_id;
  switch packet.type {
      case TYPE_MSG_MAKE_SWAP:
          const msg = protobuf.decode(data)
          bank_keeper.sendCoins(escrowAddr, msg.maker_address, msg.sell_token)
          order_id = generateOrderId(msg)
          break;
      case TYPE_MSG_TAKE_SWAP:
          const msg = protobuf.decode(data)
          bank_keeper.sendCoins(escrowAddr, msg.taker_address, msg.sell_token)
          order = msg.order_id
      }
  }
  // update order state to cancel
  order = Orderbook.findOrderById(orderId)
  order.status = Status.CANCEL
  store.save(order)
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

Nov 11, 2022 - Draft revised

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
