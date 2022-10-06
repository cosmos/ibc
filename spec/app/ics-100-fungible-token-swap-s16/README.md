---
ics: 100
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

This standard document specifies packet data structure, state machine handling logic, and encoding details for the atomic swap of fungible tokens over an IBC channel between two modules on separate chains.

### Motivation

Users may wish to exchange tokens without transfering tokens away from its native chain. ICS-100 enabled chains can facilitate atomic swaps between users and their tokens located on the different chains. This is useful for exchanges between specific users at specific prices, and opens opportunities for new application designs.

### Definitions

`Atomic Swap`: An exchange of tokens from separate chains without transfering tokens away from its source chain.

`Order`: an offer to exchange quantity X of token A for quantity Y of token B. Tokens offered are sent to an escrow account (owned by the module)

`Maker`: A user that makes or initiates an order.

`Taker`: Is the counterparty who takes or responds to an order.

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations.
- `Gaurantee of exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.
- `Escrow enabled`: an account owned by the module will hold tokens and facilitate exchange.
- `Refundable`: tokens are refunded by escrow when an orders is cancelled
- `Basic orderbook`: a store of orders functioning as an orderbook system
- `Partial filled orders`: allows takers to partially fill an order by a maker

## Technical Specification

### General Design

A user offers tokens for exchange by making an order. The order specifies the quantity and price of exchange, and sends the offered tokens to the chain's escrow account.

Any user on a different chain with the correct token denomination can accept the offer by taking the order. The taker sends the desired amount of tokens to the chain's escrow account.

The escrow account on each respective chain transfers the corresponding token amounts to each user's receiving address, without requiring the usual ibc transfer.

### Data Structures

Only one packet data type is required: AtomicSwapPacketData, which specifies the swap message type, data(protobuf marshalled) and a memo field.

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
	order_id: string;
	// the tokens to be sell
	sell_token: Coin;
	// the sender address
	taker_address: string;
	// the sender's address on the destination chain
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

Both the source chain and destination chain maintain separate orderbooks. Orders are saved in both source chain and destination chain.

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
	id: string;
	maker: MakeSwap;
	status: Status;
	fill_status: FillStatus;
	channel_id: string;
	takers: TakeSwap[];
	cancel_timestamp: int64;
	complete_timestamp: int64;
}
```

### Life scope and control flow

The following illustrates the flow:
<img src="./ibcswap.png"/>

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
