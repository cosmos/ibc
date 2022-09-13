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

The sub-protocols described herein should be implemented in a "fungible token Atomic Swap" module with access to a bank module and to the IBC routing module.
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

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the fungible token transfer module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels
to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion
that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics31-1`.

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
  // assert that version is "ics31-1" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics31-1" || version === "")

  return "ics31-1", nil
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
  // assert that version is "ics31-1"
  abortTransactionUnless(counterpartyVersion === "ics31-1")

  return "ics31-1", nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  // port has already been validated
  // assert that counterparty selected version is "ics31-1"
  abortTransactionUnless(counterpartyVersion === "ics31-1")
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

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: AtomicSwapPacketData) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
}
```

`refundTokens` is called by both `onAcknowledgePacket`, on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original sender.

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
