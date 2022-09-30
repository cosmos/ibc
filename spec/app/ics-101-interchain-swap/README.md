---
ics: 101
title: Interchain Sawp
stage: draft
category: IBC/APP
kind: instantiation
author: Ping(ping@side.one)
created: (creation date)
modified: 2022-07-27
requires: 24, 25
---

## Synopsis

(high-level description of and rationale for specification)

### Motivation

(rationale for existence of standard)
 - Build a fully decentralised exchange p2p network, that each chain play a role in the exchange network. Assets can be swapped directly between any two blockchain(in where ibcswap is integrated ) in networks
 - Users usually are preferred to one assets to pool than two assets with a percentage splits
 - Reduce impermanent loss for liquidity provider

### Definitions
 - Liquidity
 - Single sided liquidity pool
 - Automatic Marker Maker(AMM)
 - Farming & Rewards
 - Pool Weight, used in AMM, 每次添加流动性或者删除流动性的时候调整相应资产的pool weight
 - Left Side Swap: Input how many coins you want to sell, output an amount you will receive
 - Right Side Swap: Input how many coins you want to buy, output an amount you need to pay

### Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

 - Permissionless: An interchain account may be created by any actor without the approval of a third party (e.g. chain governance). Note: Individual implementations may implement their own permissioning scheme, however the protocol must not require permissioning from a trusted party to be secure.
 - Decentralization: All parameters are managed on its chain,  no one, (even no single blockchain), control all things.
 - Community Driven:

## Technical Specification

(main part of standard document - not all subsections are required)

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Data Structures

```ts
enum MessageType {
    Create,
    Deposit,
    Withdraw,
    LeftSwap,
    RightSwap,
}

// IBCSwapDataPacket is used to wrap message for relayer.
interface IBCSwapDataPacket {
    msgType: MessageType,
    data: Uint8Array, // Bytes
}
```

IBCSwapDataPacket is a wrap which wrap all sepecific messages

### Sub-protocols
IBCSwap implements following sub-protocols:
```protobuf
  rpc DelegateCreatePool(MsgCreatePoolRequest) returns (MsgCreatePoolResponse);
  rpc DelegateSingleDeposit(MsgSingleDepositRequest) returns (MsgSingleDepositResponse);
  rpc DelegateWithdraw(MsgWithdrawRequest) returns (MsgWithdrawResponse);
  rpc DelegateLeftSwap(MsgLeftSwapRequest) returns (MsgSwapResponse);
  rpc DelegateRightSwap(MsgRightSwapRequest) returns (MsgSwapResponse);
```
(sub-protocols, if applicable)

#### structure of sub protocols:

 - Create Pool
``` ts
interface MsgCreatePoolRequest {
    sender: string,
    denoms: string[],
    decimals: [],
    weight: string,
}

interface MsgCreatePoolResponse {}
```

- Single Sided Deposit
```ts
interface MsgDepositRequest {
    sender: string,
    tokens: Coin[],
}
interface MsgSingleDepositResponse {
    pool_token: Coin[];
}
```

- Withdraw
```ts
interface MsgWithdrawRequest {
    sender: string,
    poolCoin: Coin,
    denomOut: string, // optional, if not set, withdraw native coin to sender.
}
interface MsgWithdrawResponse {
   tokens: Coin[];
}
```
 - Left Side Swap
 ```ts
 interface MsgLeftSwapRequest {
    sender: string,
    tokenIn: Coin,
    denomOut: string,
    slippage: number; // max tolerated slippage
    recipient: string, 
}
interface MsgSwapResponse {
   tokens: Coin[];
}
```
 - Right Side Swap
 ```ts
interface MsgRightSwapRequest {
    sender: string,
    denomIn: string,
    tokenOut: Coin,
    slippage: number; // max tolerated slippage 
    recipient: string,
}
interface MsgSwapResponse {
   tokens: Coin[];
}
```

### Port and Channel Setup

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
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // assert that version is "ics20-1" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics101-1" || version === "")
  return "ics101-1", nil
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
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // assert that version is "ics101-1"
  abortTransactionUnless(counterpartyVersion === "ics101-1")
  // return version that this chain will use given the
  // counterparty version
  return "ics101-1", nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  abortTransactionUnless(counterpartyVersion === "ics101-1")
}
```

### Packet Relay

```ts
function SendIBCSwapDelegationDataPacket(
  swapPacket: IBCSwapPacketData,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64) {

    // send packet using the interface defined in ICS4
    handler.sendPacket(
      getCapability("port"),
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      swapPacket
    )
}

```

onRecvPacket:

```go
func (im IBCModule) OnRecvPacket(
	ctx sdk.Context,
	packet channeltypes.Packet,
	relayer sdk.AccAddress,
) ibcexported.Acknowledgement {
	ack := channeltypes.NewResultAcknowledgement([]byte{byte(1)})

	var data types.IBCSwapPacketData
	var ackErr error
	if err := types.ModuleCdc.UnmarshalJSON(packet.GetData(), &data); err != nil {
		ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap packet data")
		ack = channeltypes.NewErrorAcknowledgement(ackErr)
	}

	// only attempt the application logic if the packet data
	// was successfully decoded
	if ack.Success() {

		switch data.Type {
		case types.CREATE_POOL:
			var msg types.MsgCreatePoolRequest
			if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			}
			if res, err := im.keeper.OnCreatePoolReceived(ctx, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			} else if result, errEncode := types.ModuleCdc.Marshal(res); errEncode != nil {
				ack = channeltypes.NewErrorAcknowledgement(errEncode)
			} else {
				ack = channeltypes.NewResultAcknowledgement(result)
			}
			break
		case types.SINGLE_DEPOSIT:
			var msg types.MsgSingleDepositRequest
			if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			}
			if res, err := im.keeper.OnSingleDepositReceived(ctx, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			} else if result, errEncode := types.ModuleCdc.Marshal(res); errEncode != nil {
				ack = channeltypes.NewErrorAcknowledgement(errEncode)
			} else {
				ack = channeltypes.NewResultAcknowledgement(result)
			}
			break
		case types.WITHDRAW:
			var msg types.MsgWithdrawRequest
			if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			}
			if res, err := im.keeper.OnWithdrawReceived(ctx, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			} else if result, errEncode := types.ModuleCdc.Marshal(res); errEncode != nil {
				ack = channeltypes.NewErrorAcknowledgement(errEncode)
			} else {
				ack = channeltypes.NewResultAcknowledgement(result)
			}
			break
		case types.LEFT_SWAP:
			var msg types.MsgLeftSwapRequest
			if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			}
			if res, err := im.keeper.OnLeftSwapReceived(ctx, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			} else if result, errEncode := types.ModuleCdc.Marshal(res); errEncode != nil {
				ack = channeltypes.NewErrorAcknowledgement(errEncode)
			} else {
				ack = channeltypes.NewResultAcknowledgement(result)
			}
			break
		case types.RIGHT_SWAP:
			var msg types.MsgRightSwapRequest
			if err := types.ModuleCdc.Unmarshal(data.Data, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			}
			if res, err := im.keeper.OnRightSwapReceived(ctx, &msg); err != nil {
				ackErr = sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "cannot unmarshal ICS-101 interchain swap message")
				ack = channeltypes.NewErrorAcknowledgement(ackErr)
			} else if result, errEncode := types.ModuleCdc.Marshal(res); errEncode != nil {
				ack = channeltypes.NewErrorAcknowledgement(errEncode)
			} else {
				ack = channeltypes.NewResultAcknowledgement(result)
			}
			break
		}

	}

	// NOTE: acknowledgement will be written synchronously during IBC handler execution.
	return ack
}
```

onAcknowledgePacket

```go

// OnAcknowledgementPacket implements the IBCModule interface
func (im IBCModule) OnAcknowledgementPacket(
	ctx sdk.Context,
	packet channeltypes.Packet,
	acknowledgement []byte,
	relayer sdk.AccAddress,
) error {
	var ack channeltypes.Acknowledgement
	if err := types.ModuleCdc.UnmarshalJSON(acknowledgement, &ack); err != nil {
		return sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "cannot unmarshal ICS-101 ibcswap packet acknowledgement: %v", err)
	}
	var data types.IBCSwapPacketData
	if err := types.ModuleCdc.UnmarshalJSON(packet.GetData(), &data); err != nil {
		return sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "cannot unmarshal ICS-101 ibcswap packet data: %s", err.Error())
	}

	switch data.Type {
	case types.CREATE_POOL:
		var request types.MsgCreatePoolRequest
		if err := types.ModuleCdc.Unmarshal(data.Data, &request); err != nil {
			return err
		}
		var response types.MsgCreatePoolResponse
		if err := types.ModuleCdc.Unmarshal(ack.GetResult(), &response); err != nil {
			return err
		}
		if err := im.keeper.OnCreatePoolAcknowledged(ctx, &request, &response); err != nil {
			return err
		}
		break
	case types.SINGLE_DEPOSIT:
		var request types.MsgSingleDepositRequest
		if err := types.ModuleCdc.Unmarshal(data.Data, &request); err != nil {
			return err
		}
		var response types.MsgSingleDepositResponse
		if err := types.ModuleCdc.Unmarshal(ack.GetResult(), &response); err != nil {
			return err
		}
		if err := im.keeper.OnSingleDepositAcknowledged(ctx, &request, &response); err != nil {
			return err
		}
		break
	case types.WITHDRAW:
		var request types.MsgWithdrawRequest
		if err := types.ModuleCdc.Unmarshal(data.Data, &request); err != nil {
			return err
		}
		var response types.MsgWithdrawResponse
		if err := types.ModuleCdc.Unmarshal(ack.GetResult(), &response); err != nil {
			return err
		}
		if err := im.keeper.OnWithdrawAcknowledged(ctx, &request, &response); err != nil {
			return err
		}
		break
	case types.LEFT_SWAP:
		var request types.MsgLeftSwapRequest
		if err := types.ModuleCdc.Unmarshal(data.Data, &request); err != nil {
			return err
		}
		var response types.MsgSwapResponse
		if err := types.ModuleCdc.Unmarshal(ack.GetResult(), &response); err != nil {
			return err
		}
		if err := im.keeper.OnLeftSwapAcknowledged(ctx, &request, &response); err != nil {
			return err
		}
		break
	case types.RIGHT_SWAP:
		var request types.MsgRightSwapRequest
		if err := types.ModuleCdc.Unmarshal(data.Data, &request); err != nil {
			return err
		}
		var response types.MsgSwapResponse
		if err := types.ModuleCdc.Unmarshal(ack.GetResult(), &response); err != nil {
			return err
		}
		if err := im.keeper.OnRightSwapAcknowledged(ctx, &request, &response); err != nil {
			return err
		}
		break
	}

	return nil
}
```

onTimeoutPacket and OnFailure
```ts
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
}

```

```ts

function refundToken(packet: Packet) {
   let token
   switch packet.type {
    case LeftSwap:
    case RightSwap:
      token = packet.tokenIn
      break;
    case Deposit:
      token = packet.tokens
      break;
    case Withdraw:
      token = packet.pool_token
   }
    escrowAccount = channelEscrowAddresses[packet.srcChannel]
    bank.TransferCoins(escrowAccount, packet.sender, token.denom, token.amount)
}
```


### IBCSwap Relayer Listener



### Properties & Invariants

(properties & invariants maintained by the protocols specified, if applicable)

## Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

## Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

## Example Implementation

(link to or description of concrete example implementation)

## Other Implementations

(links to or descriptions of other implementations)

## History

(changelog and notable inspirations / references)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
