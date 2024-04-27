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

<p style="text-align:center;border-style:solid">This specification has been reviewed by the Spec Committee, but it is not responsible for maintaining it.</p>

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the atomic swap of fungible tokens over an IBC channel between two modules on separate chains.

### Motivation

Users may wish to exchange tokens without transferring tokens away from their native chain. ICS-100 enabled chains can facilitate atomic swaps between users and their tokens located on the different chains. This is useful for exchanges between specific users at specific prices, and opens opportunities for new application designs.

For example, a token exchange would require only one transaction from a user, compared to multiple transactions when using ICS-20.  Additionally, users can minimize trade slippage compared to using a liquidity pool, given there is a willing counter-party.

### Definitions

`Atomic Swap`: An exchange of tokens from separate chains without transferring tokens from one blockchain to another.  The exchange either happens or it doesn't -- there is no other alternative.

`Order`: An offer to exchange quantity X of token A for quantity Y of token B. Tokens offered are sent to an escrow account (owned by the module).

`Maker`: A user that makes or initiates an order.

`Taker`: The counterparty who takes or responds to an order.

`Maker Chain`: The blockchain where a maker makes or initiates an order.

`Taker Chain`: The blockchain where a taker takes or responds to an order.

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations.
- `Guarantee of exchange`: no occurrence of a user receiving tokens without the equivalent promised exchange.
- `Escrow enabled`: an account owned by the module will hold tokens and facilitate exchange.
- `Refundable`: tokens are refunded by escrow when a timeout occurs, or when an order is cancelled.
- `Order cancellation`: orders without takers can be cancelled.
- `Basic orderbook`: a store of orders functioning as an orderbook system.
- `Atomicity`: an exchange of one token for another where it is either a total success or a total failure.

## Technical Specification

### General Design

<img src="./ibcswap.png"/>

A maker offers token A in exchange for token B by making an order. The order specifies the quantity and price of exchange, and sends the offered token A to the maker chain's escrow account. Any taker on a different chain with token B can accept the offer by taking the order. The taker sends the desired amount of token B to the taker chain's escrow account. The escrow account on each respective chain transfers the corresponding token amounts to each user's receiving address, without requiring the usual IBC transfer.

An order without takers can be cancelled.  This enables users to rectify mistakes, such as inputting an incorrect price or taker address.  Upon cancellation escrowed tokens will be refunded. 

When making or taking an order, a timeout window is specified in the relayed data packet.  A timeout will result in escrowed tokens refunded back.  This timeout window is customizable.

### Data Structures

Only one packet data type is required: `AtomicSwapPacketData`, which specifies the swap message type, data (protobuf marshalled) and a memo field.

```typescript
enum SwapMessageType {
  // Default zero value enumeration
  TYPE_UNSPECIFIED = 0,
  TYPE_MSG_MAKE_SWAP = 1,
  TYPE_MSG_TAKE_SWAP = 2,
  TYPE_MSG_CANCEL_SWAP = 3,
}
```

```typescript
// AtomicSwapPacketData is comprised of a swap message type, raw transaction and optional memo field.
interface AtomicSwapPacketData {
  type: SwapMessageType
  data: bytes
  memo: string
}
```

```typescript
type AtomicSwapPacketAcknowledgement = AtomicSwapPacketSuccess | AtomicSwapPacketError
interface AtomicSwapPacketSuccess {
  // This is binary 0x01 base64 encoded
  result: "AQ=="
}

interface AtomicSwapPacketError {
  error: string
}
```

All `AtomicSwapPacketData` will be forwarded to the corresponding message handler to execute according to its type. There are 3 types:

```typescript
interface MakeSwapMsg {
  // the port on which the packet will be sent, specified by the maker when the message is created
  sourcePort: string
  // the channel on which the packet will be sent, specified by the maker when the message is created
  sourceChannel: string
  // the tokens to be exchanged
  sellToken: Coin
  buyToken: Coin
  // the maker's address
  makerAddress: string
  // the maker's address on the taker chain
  makerReceivingAddress string
  // if desiredTaker is specified,
  // only the desiredTaker is allowed to take this order
  // this is the address on the taker chain
  desiredTaker: string
  creationTimestamp: uint64
  expirationTimestamp: uint64
  timeoutHeight: Height
  timeoutTimestamp: uint64
}
```

```typescript
interface TakeSwapMsg {
  orderId: string
  // the tokens to be sold
  sellToken: Coin
  // the taker's address
  takerAddress: string
  // the taker's address on the maker chain
  takerReceivingAddress: string
  creationTimestamp: uint64
  timeoutHeight: Height
  timeoutTimestamp: uint64
}
```

```typescript
interface CancelSwapMsg {
  orderId: string
  makerAddress: string
  timeoutHeight: Height
  timeoutTimestamp: uint64
}
```

where `Coin` is an interface that consists of an amount and a denomination:

```ts
interface Coin {
  amount: int64
  denom: string
}
```

Both the maker chain and taker chain maintain separate order store. Orders are saved in both maker chain and taker chain.

```typescript
// Status of order 
enum Status {
  INITIAL = 0, // initialed on maker chain
  SYNC = 1, // synced to the taker chain
  CANCEL = 2, // canceled 
  COMPLETE = 3, // completed
}

interface Order {
  id: string
  maker: MakeSwapMsg
  status: Status
  // an IBC path, define channel and port on both Maker Chain and Taker Chain 
  path: string
  taker: TakeSwapMsg
  cancelTimestamp: uint64
  completeTimestamp: uint64
}

function createOrder(msg: MakeSwapMsg, packet: channelType.Packet): Order {
    const path = orderPath(packet)
    return Order{
      id : generateOrderId(packet),
      status: Status.INITIAL,
      path: path,
      maker: msg,
    }
  }

function orderPath(packet: channelType.Packet): string {
    return `channel/${packet.sourceChannel}/port/${packet.sourcePort}/channel/${packet.destChannel}/port/${packet.destPort}/sequence/${packet.sequence}`

}
// Order id is a global unique string, since packet contains sourceChannel, SourcePort, distChannel, distPort, sequence and msg data
function generateOrderId(packet: channelType.Packet) : string {
  const bytes = protobuf.encode(packet)
  return sha265(bytes)
}

function extractSourceChannelForTakerMsg(path: string) : string {
  return path.split('/')[5]
}
function extractSourcePortForTakerMsg(path: string) : string{
  return path.split('/')[7]
}
```

### Life scope and control flow

#### Making a swap

1. A maker creates an order on the maker chain with specified parameters (see type `MakeSwap`).  The maker's sell tokens are sent to the escrow address owned by the module. The order is saved on the maker chain.
2. An `AtomicSwapPacketData` is relayed to the taker chain where in `onRecvPacket` the order is also saved on the taker chain.  
3. A packet is subsequently relayed back for acknowledgement. A packet timeout or a failure during `onAcknowledgePacket` will result in a refund of the escrowed tokens.

#### Taking a swap

1. A taker takes an order on the taker chain by triggering `TakeSwap`.  The taker's sell tokens are sent to the escrow address owned by the module.  An order cannot be taken if the current time is later than the `expirationTimestamp`.
2. An `AtomicSwapPacketData` is relayed to the maker chain where in `onRecvPacket` the escrowed tokens are sent to the taker address on the maker chain.
3. A packet is subsequently relayed back for acknowledgement. Upon acknowledgement escrowed tokens on the taker chain are sent to the maker address on the taker chain. A packet timeout or a failure during `onAcknowledgePacket` will result in a refund of the escrowed tokens.

#### Cancelling a swap

1. A maker cancels a previously created order. Expired orders can also be cancelled.
2. An `AtomicSwapPacketData` is relayed to the taker chain where in `onRecvPacket` the order is cancelled on the taker chain. If the order is in the process of being taken (a packet with `TakeSwapMsg` is being relayed from the taker chain to the maker chain), the cancellation will be rejected.
3. A packet is relayed back where upon acknowledgement the order on the maker chain is also cancelled.  The refund only occurs if the taker chain confirmed the cancellation request.

### Sub-protocols

The sub-protocols described herein should be implemented in a "Fungible Token Swap" module with access to a bank module and to the IBC routing module.

```ts
function makeSwap(request: MakeSwapMsg, packet: channelTypes.Packet) {
  const balance = bank.getBalances(request.makerAddress, request.sellToken.denom)
  abortTransactionUnless(balance.amount >= request.sellToken.Amount)
  // gets escrow address by source port and source channel
  const escrowAddr = escrowAddress(request.sourcePort, request.sourceChannel)
  // locks the sellToken to the escrow account
  const err = bank.sendCoins(request.makerAddress, escrowAddr, request.sellToken.amount, request.sellToken.denom)
  abortTransactionUnless(err === null)
  // constructs the IBC data packet
  const packet = {
    type: SwapMessageType.TYPE_MSG_MAKE_SWAP,
    data: protobuf.encode(request), // encode the request message to protobuf bytes
    memo: ""
  }
  sendAtomicSwapPacket(packet, request.sourcePort, request.sourceChannel, request.timeoutHeight, request.timeoutTimestamp)
    
  // creates and saves order on the maker chain.
  const order = createOrder(request, packet)
  // saves order to store
  privateStore.set(order.id, order)
}
```

```ts
function takeSwap(request: TakeSwapMsg) {
  const order = privateStore.get(request.orderId)
  abortTransactionUnless(order !== null)
  abortTransactionUnless(order.expirationTimestamp < currentTimestamp())
  abortTransactionUnless(order.maker.buyToken.denom === request.sellToken.denom)
  abortTransactionUnless(order.maker.buyToken.amount === request.sellToken.amount)
  abortTransactionUnless(order.taker === null)
  // if `desiredTaker` is set, only the desiredTaker can accept the order.
  abortTransactionUnless(order.maker.desiredTaker !== null && order.maker.desiredTaker !== request.takerAddress)
    
  const balance = bank.getBalances(request.takerAddress, request.sellToken.denom)
  abortTransactionUnless(balance.amount >= request.sellToken.amount)
  
  const sourceChannel = extractSourceChannelForTakerMsg(order.path)
  const sourcePort = extractSourcePortForTakerMsg(order.path)
  
  // gets the escrow address by source port and source channel
  const escrowAddr = escrowAddress(sourcePort, sourceChannel)
  // locks the sellToken to the escrow account
  const err = bank.sendCoins(request.takerAddress, escrowAddr, request.sellToken.amount, request.sellToken.denom)
  abortTransactionUnless(err === null)
  // constructs the IBC data packet
  const packet = {
    type: SwapMessageType.TYPE_MSG_TAKE_SWAP,
    data: protobuf.encode(request), // encode the request message to protobuf bytes.
    memo: ""
  } 
    
  sendAtomicSwapPacket(packet, sourcePort, sourceChannel, request.timeoutHeight, request.timeoutTimestamp)
    
  // update order state
  order.taker = request // mark that the order has been occupied
  privateStore.set(order.orderId, order)
}
```

```ts
function cancelSwap(request: CancelSwapMsg) {
  const order = privateStore.get(request.orderId)
  // checks if the order exists
  abortTransactionUnless(order !== null)
  // make sure the sender is the maker of the order.
  abortTransactionUnless(order.maker.makerAddress == request.makerAddress)
  abortTransactionUnless(order.status == Status.SYNC || order.status == Status.INITIAL)
      
  // constructs the IBC data packet
  const packet = {
    type: SwapMessageType.TYPE_MSG_CANCEL_SWAP,
    data: protobuf.encode(request), // encode the request message to protobuf bytes.
    memo: "",
  } 
  // the request is sent to the taker chain, and the taker chain decides if the cancel order is accepted or not
  // the cancellation can only be sent to the same chain as the make order.
  sendAtomicSwapPacket(packet, order.maker.sourcePort, order.maker.sourceChannel request.timeoutHeight, request.timeoutTimestamp)
}
```

#### Port & channel setup

The fungible token swap module on a chain must always bind to a port with the id `atomicswap`.

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
  version: string): (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics100-1" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics100-1" || version === "")
  
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
  
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
  counterpartyVersion: string): (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics100-1"
  abortTransactionUnless(counterpartyVersion === "ics100-1")
  
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
  
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
  abortTransactionUnless(counterpartyVersion === "ics100-1")
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
  abortTransactionUnless(FALSE)
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
  swapPacket: AtomicSwapPacketData, 
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64
) {
  // send packet using the interface defined in ICS4
  handler.sendPacket(
    getCapability("port"),
    sourcePort,
    sourceChannel,
    timeoutHeight,
    timeoutTimestamp,
    protobuf.marshal(swapPacket) // protobuf-marshalled bytes of packet data
  )
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received. 

```typescript
function onRecvPacket(packet channeltypes.Packet) {
  const swapPacket: AtomicSwapPacketData = packet.data
  
  AtomicSwapPacketAcknowledgement ack = AtomicSwapPacketAcknowledgement{true, null}
  
  switch swapPacket.type {
    case TYPE_MSG_MAKE_SWAP:
      const makeMsg = protobuf.decode(swapPacket.data)
        
      // check if buyToken is a valid token on the taker chain, could be either native or ibc token
      const supply = bank.getSupply(makeMsg.buyToken.denom)
      if (supply <= 0) {
        ack = AtomicSwapPacketAcknowledgement{false, "sell token does not exist on the taker chain"}
        break;
      }
        
      // create and save order on the taker chain.
      const order = createOrder(makeMsg, packet)
      order.status = Status.SYNC
      // saves order to store
      const err = privateStore.set(order.orderId, order)
      if (err != null) {
        ack = AtomicSwapPacketAcknowledgement{false, "failed to save the order on taker chain"}
      }
      break;
    case TYPE_MSG_TAKE_SWAP:
      const takeMsg = protobuf.decode(swapPacket.data)
      const order = privateStore.get(takeMsg.orderId)
      abortTransactionUnless(order !== null)
      abortTransactionUnless(order.status === Status.SYNC) 
      abortTransactionUnless(order.expiredTimestamp < currentTimestamp())
      abortTransactionUnless(takeMsg.sellToken.denom === order.maker.buyToken.denom)
      abortTransactionUnless(takeMsg.sellToken.amount === order.maker.buyToken.amount)
      // if `desiredTaker` is set, only the desiredTaker can accept the order.
      abortTransactionUnless(order.maker.desiredTaker !== null && order.maker.desiredTaker !== takeMsg.takerAddress)
  
      const escrowAddr = escrowAddress(order.maker.sourcePort, order.maker.sourceChannel)
      // send maker.sellToken to taker's receiving address
      const err = bank.sendCoins(escrowAddr, takeMsg.takerReceivingAddress, order.maker.sellToken.amount, order.maker.sellToken.denom)
      if (err != null) {
        ack = AtomicSwapPacketAcknowledgement{false, "transfer coins failed"}
        break;
      }
        
      // update status of order
      order.status = Status.COMPLETE
      order.taker = takeMsg
      order.completeTimestamp = takeMsg.creationTimestamp
      privateStore.set(takeMsg.orderId, order)
      break;
    case TYPE_MSG_CANCEL_SWAP:
      const cancelMsg = protobuf.decode(swapPacket.data)
      const order = privateStore.get(cancelMsg.orderId)
      abortTransactionUnless(order !== null)
      abortTransactionUnless(order.status === Status.SYNC || order.status == Status.INITIAL)
      abortTransactionUnless(order.taker !== null) // the maker order has not been occupied 
        
      // update status of order
      order.status = Status.CANCEL
      order.cancelTimestamp = cancelMsg.creationTimestamp 
      const err = privateStore.set(cancelMsg.orderId, order)
      if (err != null) {
        ack = AtomicSwapPacketAcknowledgement{false, "failed to cancel order on taker chain"}
      }
      break;
    default:
      ack = AtomicSwapPacketAcknowledgement{false, "unknown data packet"} 
  }
  
  return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: channeltypes.Packet
  acknowledgement: bytes) {
  // ack is failed
  if (!acknowledgement.success) {
    refundTokens(packet) 
  } else {
    
    const swapPacket: AtomicSwapPacketData = protobuf.decode(packet.data)
    const escrowAddr = escrowAddress(packet.sourcePort, packet.sourceChannel)
    
    switch swapPacket.type {
      case TYPE_MSG_MAKE_SWAP:
        const makeMsg = protobuf.decode(swapPacket.data)
        
        // update order status on the maker chain.
        const order = privateStore.get(generateOrderId(packet))
        order.status = Status.SYNC
        // save order to store
        privateStore.set(order.id, order)
        break;
      case TYPE_MSG_TAKE_SWAP:
        const takeMsg = protobuf.decode(swapPacket.data)
        
        // update order status on the taker chain.
        const order = privateStore.get(takeMsg.orderId)
        
        // send tokens to maker
        bank.sendCoins(escrowAddr, order.maker.makerReceivingAddress, takeMsg.sellToken.amount, takeMsg.sellToken.denom)
        
        order.status = Status.COMPLETE
        order.taker = takeMsg
        order.completeTimestamp = takeMsg.creationTimestamp
        privateStore.set(order.id, order)
        
        break;
      case TYPE_MSG_CANCEL_SWAP:
        const cancelMsg = protobuf.decode(swapPacket.data)
        
        // update order status on the maker chain.
        const order = privateStore.get( cancelMsg.orderId )
        
        // send tokens back to maker
        bank.sendCoins(escrowAddr, order.maker.makerAddress, order.maker.sellToken.amount, order.maker.sellToken.denom)
        
        // update state on maker chain
        order.status = Status.CANCEL
        order.cancelTimestamp = cancelMsg.creationTimestamp
        privateStore.set(order.id, order)
        
        break;
      default:
        throw new Error("ErrUnknownDataPacket")
    }
  }
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that the tokens will be refunded).

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
}
```

`refundTokens` is called by both `onAcknowledgePacket` on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original owner.

```typescript
function refundTokens(packet: Packet) {
  const swapPacket: AtomicSwapPacketData = protobuf.decode(packet.data)
  const escrowAddr = escrowAddress(packet.sourcePort, packet.sourceChannel)
  
  // send tokens from module to message sender
  switch swapPacket.type {
    case TYPE_MSG_MAKE_SWAP:
      const msg = protobuf.decode(swapPacket.data)
      bank.sendCoins(escrowAddr, msg.makerAddress, msg.sellToken.amount, msg.sellToken.denom)
      const orderId = generateOrderId(packet)
      const order = privateStore.get(order.orderId)
      order.status = Status.CANCEL
      privateStore.set(order.orderId, order)
      break;
    case TYPE_MSG_TAKE_SWAP:
      const msg = protobuf.decode(swapPacket.data)
      bank.sendCoins(escrowAddr, msg.takerAddress, msg.sellToken.amount, msg.sellToken.denom)
      const order = privateStore.get(msg.orderId)
      order.taker = null // release the occupation
      privateStore.set(order.orderId, order)
      break;
    case TYPE_MSG_CANCEL_SWAP:
      // do nothing, only send tokens back when cancel msg is acknowledged.
  }
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

## Example Implementations

- Implementation of ICS 100 in Go can be found in [ibcswap repository](https://github.com/ibcswap/ibcswap).

## History

Aug 15, 2022 - Draft written

Oct 6, 2022 - Draft revised

Nov 11, 2022 - Draft revised

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
