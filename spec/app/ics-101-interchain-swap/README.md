---
ics: 101
title: Interchain Swap
stage: draft
category: IBC/APP
kind: instantiation
author: Ping <ping@side.one>, Edward Gunawan <edward@s16.ventures>
created: 2022-10-09
modified: 2022-10-11
requires: 24, 25
---

## Synopsis

This standard document specifies the packet data structure, state machine handling logic, and encoding details for token exchange through single-sided liquidity pools over an IBC channel between separate chains.

### Motivation

ICS-101 Interchain Swaps enables chains their own token pricing mechanism and exchange protocol via IBC transactions.  Each chain can thus play a role in a fully decentralised exchange network.

Users might also prefer single asset pools over dual assets pools as it removes the risk of impermanent loss.

### Definitions

`Interchain swap`: a IBC token swap protocol, built on top of an automated marketing making system, which leverages liquidity pools and incentives.  Each chain that integrates this app becomes part of a decentralized exchange network.

`Automated market makers(AMM)`: are decentralized exchanges that pool liquidity and allow tokens to be traded in a permissionless and automatic way.  Usually uses an invariant for token swapping calculation.  In this interchain standard, the Balancer algorithm is implemented.

`Weighted pools`: liquidity pools characterized by the percentage weight of each token denomination maintained within.

`Single-sided liquidity pools`: a liquidity pool that does not require users to deposit both token denominations -- one is enough.

`Left-side swap`: a token exchange that specifies the desired quantity to be sold.

`Right-side swap`: a token exchange that specifies the desired quantity to be purchased.

`Pool state`: the entire state of a liquidity pool including its invariant value which is derived from its token balances and weights inside.

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations.  Individual implementations may have their own permissioning scheme, however the protocol must not require permissioning from a trusted party to be secure.
- `Decentralization`: all parameters are managed on chain via governance.  Does not require any central authority or entity to function.  Also does not require a single blockchain, acting as a hub, to function.
- `Gaurantee of Exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.
- `Liquidity Incentives`: supports the collection of fees which are distributed to liquidity providers and acts as incentive for liquidity participation.
- `Weighted Math`: allows the configuration of pool weights so users can choose their levels of exposure between the tokens.


## Technical Specification

### Algorithms

#### Invariant

A constant invariant is maintained after trades which takes into consideration token weights and balance.  The value function $V$ is defined as:

$$V = {&Pi;_tB_t^{W_t}}$$ 

Where

- $t$ ranges over the tokens in the pool
- $B_t$ is the balance of the token in the pool
- $W_t$ is the normalized weight of the tokens, such that the sum of all normalized weights is 1. 

#### Spot Price

Spot prices of tokens are defined entirely by the weights and balances of the token pair.  The spot price between any two tokens, $SpotPrice_i^{o}$, or in short $SP_i^o$, is the ratio of the token balances normalized by their weights:

$$SP_i^o = (B_i/W_i)/(B_o/W_o)$$ 

- $B_i$ is the balance of token $i$, the token being sold by the trader which is going into the pool
- $B_o$ is the balance of token $o$, the token being bought by the trader which is going out of the pool
- $W_i$ is the weight of token $i$
- $W_o$ is the weight of token $o$

#### Fees

Traders pay swap fees when they trade with a pool. These fees can be customized with a minimum value of 0.0001% and a maximum value of 10%. 

The fees go to liquidity providers in exchange for depositing their tokens in the pool to facilitate trades. Trade fees are collected at the time of a swap, and goes directly into the pool, increasing the pool balance. For a trade with a given $inputToken$ and $outputToken$, the amount collected by the pool as a fee is 

$$Amount_{fee} = Amount_{inputToken} * swapFee$$ 

As the pool collects fees, liquidity providers automatically collect fees through their proportional ownership of the pool balance.

### Data Structures

Only one packet data type is required: `IBCSwapDataPacket`, which specifies the message type and data(protobuf marshalled).  It is a wrapper for interchain swap messages.

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

### Sub-protocols

Traditional liquidity pools typically maintain its pool state in one location.  

A liquidity pool in the interchain swap protocol maintains its pool state on both its source chain and destination chain.  The pool states mirror each other and are synced through IBC packet relays, which we elaborate on in the following sub-protocols.

IBCSwap implements the following sub-protocols:

```protobuf
  rpc DelegateCreatePool(MsgCreatePoolRequest) returns (MsgCreatePoolResponse);
  rpc DelegateSingleDeposit(MsgSingleDepositRequest) returns (MsgSingleDepositResponse);
  rpc DelegateWithdraw(MsgWithdrawRequest) returns (MsgWithdrawResponse);
  rpc DelegateLeftSwap(MsgLeftSwapRequest) returns (MsgSwapResponse);
  rpc DelegateRightSwap(MsgRightSwapRequest) returns (MsgSwapResponse);
```

#### Interfaces for sub-protocols

``` ts
interface MsgCreatePoolRequest {
    sender: string,
    denoms: string[],
    decimals: [],
    weight: string,
}

interface MsgCreatePoolResponse {}
```
```ts
interface MsgDepositRequest {
    sender: string,
    tokens: Coin[],
}
interface MsgSingleDepositResponse {
    pool_token: Coin[];
}
```
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

### Control Flow And Life Scope

To implement interchain swap, we introduce the `Message Delegator` and `Relay Listener`. The `Message Delegator` will pre-process the request (validate msgs, lock assets, etc), and then forward the transactions to the relayer.

```go
func (k Keeper) DelegateCreatePool(goctx context.Context, msg *types.MsgCreatePoolRequest) (*types.MsgCreatePoolResponse, error) {
	ctx := sdk.UnwrapSDKContext(goctx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	_, err1 := sdk.AccAddressFromBech32(msg.Sender)
	if err1 != nil {
		return nil, err1
	}

	pool := types.NewBalancerLiquidityPool(msg.Denoms, msg.Decimals, msg.Weight)
	if err := pool.Validate(); err != nil {
		return nil, err
	}

	// count native tokens
	count := 0
	for _, denom := range msg.Denoms {
		if k.bankKeeper.HasSupply(ctx, denom) {
			count += 1
			pool.UpdateAssetPoolSide(denom, types.PoolSide_POOL_SIDE_NATIVE_ASSET)
		} else {
			pool.UpdateAssetPoolSide(denom, types.PoolSide_POOL_SIDE_REMOTE_ASSET)
		}
	}
	if count == 0 {
		return nil, types.ErrNoNativeTokenInPool
	}

	msgByte, err0 := types.ModuleCdc.Marshal(msg)
	if err0 != nil {
		return nil, err0
	}

	packet := types.NewIBCSwapPacketData(types.CREATE_POOL, msgByte, nil)
	if err := k.SendIBCSwapDelegationDataPacket(ctx, msg.SourcePort, msg.SourceChannel, msg.TimeoutHeight, msg.TimeoutTimestamp, packet); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitTypedEvents(msg)

	return &types.MsgCreatePoolResponse{}, nil
}

func (k Keeper) DelegateSingleDeposit(goctx context.Context, msg *types.MsgSingleDepositRequest) (*types.MsgSingleDepositResponse, error) {
	ctx := sdk.UnwrapSDKContext(goctx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sender, err1 := sdk.AccAddressFromBech32(msg.Sender)
	if err1 != nil {
		return nil, err1
	}

	// deposit assets to the swap module
	length := len(msg.Tokens)
	var coins = make([]sdk.Coin, length)
	for i := 0; i < length; i++ {
		coins[i] = *msg.Tokens[i]
	}
	k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(coins...))

	msgByte, err0 := types.ModuleCdc.Marshal(msg)
	if err0 != nil {
		return nil, err0
	}

	packet := types.NewIBCSwapPacketData(types.SINGLE_DEPOSIT, msgByte, nil)
	if err := k.SendIBCSwapDelegationDataPacket(ctx, msg.SourcePort, msg.SourceChannel, msg.TimeoutHeight, msg.TimeoutTimestamp, packet); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitTypedEvents(msg)

	return &types.MsgSingleDepositResponse{}, nil
}

func (k Keeper) DelegateWithdraw(ctx2 context.Context, msg *types.MsgWithdrawRequest) (*types.MsgWithdrawResponse, error) {
	ctx := sdk.UnwrapSDKContext(ctx2)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sender, err1 := sdk.AccAddressFromBech32(msg.Sender)
	if err1 != nil {
		return nil, err1
	}

	// deposit assets to the swap module
	k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(*msg.PoolToken))

	msgByte, err0 := types.ModuleCdc.Marshal(msg)
	if err0 != nil {
		return nil, err0
	}

	packet := types.NewIBCSwapPacketData(types.WITHDRAW, msgByte, nil)
	if err := k.SendIBCSwapDelegationDataPacket(ctx, msg.SourcePort, msg.SourceChannel, msg.TimeoutHeight, msg.TimeoutTimestamp, packet); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitTypedEvents(msg)

	return &types.MsgWithdrawResponse{}, nil
}

func (k Keeper) DelegateLeftSwap(goctx context.Context, msg *types.MsgLeftSwapRequest) (*types.MsgSwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(goctx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sender, err1 := sdk.AccAddressFromBech32(msg.Sender)
	if err1 != nil {
		return nil, err1
	}

	// deposit assets to the swap module
	k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(*msg.TokenIn))

	msgByte, err0 := types.ModuleCdc.Marshal(msg)
	if err0 != nil {
		return nil, err0
	}

	packet := types.NewIBCSwapPacketData(types.LEFT_SWAP, msgByte, nil)
	if err := k.SendIBCSwapDelegationDataPacket(ctx, msg.SourcePort, msg.SourceChannel, msg.TimeoutHeight, msg.TimeoutTimestamp, packet); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitTypedEvents(msg)

	return &types.MsgSwapResponse{}, nil
}

func (k Keeper) DelegateRightSwap(goctx context.Context, msg *types.MsgRightSwapRequest) (*types.MsgSwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(goctx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sender, err1 := sdk.AccAddressFromBech32(msg.Sender)
	if err1 != nil {
		return nil, err1
	}

	k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(*msg.TokenIn))

	msgByte, err0 := types.ModuleCdc.Marshal(msg)
	if err0 != nil {
		return nil, err0
	}

	packet := types.NewIBCSwapPacketData(types.RIGHT_SWAP, msgByte, nil)
	if err := k.SendIBCSwapDelegationDataPacket(ctx, msg.SourcePort, msg.SourceChannel, msg.TimeoutHeight, msg.TimeoutTimestamp, packet); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitTypedEvents(msg)

	return &types.MsgSwapResponse{}, nil
}
```

The `Relay Listener` handle all transactions, execute transactions when received, and send the result as an acknowledgement. In this way, packets relayed on the source chain update pool states on the destination chain according to results in the acknowledgement.

```go
func (k Keeper) OnCreatePoolReceived(ctx sdk.Context, msg *types.MsgCreatePoolRequest) (*types.MsgCreatePoolResponse, error) {

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	_, err1 := sdk.AccAddressFromBech32(msg.Sender)
	if err1 != nil {
		return nil, err1
	}

	pool := types.NewBalancerLiquidityPool(msg.Denoms, msg.Decimals, msg.Weight)
	if err := pool.Validate(); err != nil {
		return nil, err
	}

	// count native tokens
	count := 0
	for _, denom := range msg.Denoms {
		if k.bankKeeper.HasSupply(ctx, denom) {
			count += 1
			pool.UpdateAssetPoolSide(denom, types.PoolSide_POOL_SIDE_NATIVE_ASSET)
		} else {
			pool.UpdateAssetPoolSide(denom, types.PoolSide_POOL_SIDE_REMOTE_ASSET)
		}
	}
	if count == 0 {
		return nil, types.ErrNoNativeTokenInPool
	}

	k.SetBalancerPool(ctx, pool)

	return &types.MsgCreatePoolResponse{
		PoolId: pool.Id,
	}, nil

}

func (k Keeper) OnSingleDepositReceived(ctx sdk.Context, msg *types.MsgSingleDepositRequest) (*types.MsgSingleDepositResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	amm, err := k.CreateIBCSwapAMM(ctx, msg.PoolId)
	if err != nil {
		return nil, err
	}

	poolToken, err := amm.Deposit(msg.Tokens)
	if err != nil {
		return nil, err
	}

	k.SetBalancerPool(ctx, *amm.Pool) // update pool states

	return &types.MsgSingleDepositResponse{
		PoolToken: &poolToken,
	}, nil
}

func (k Keeper) OnWithdrawReceived(ctx sdk.Context, msg *types.MsgWithdrawRequest) (*types.MsgWithdrawResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	amm, err := k.CreateIBCSwapAMM(ctx, msg.PoolToken.Denom) // Pool Token denomination is the pool Id
	if err != nil {
		return nil, err
	}

	outToken, err := amm.Withdraw(msg.PoolToken, msg.DenomOut)
	if err != nil {
		return nil, err
	}

	k.SetBalancerPool(ctx, *amm.Pool) // update pool states

	// only output one asset in the pool
	return &types.MsgWithdrawResponse{
		Tokens: []*sdk.Coin{
			&outToken,
		},
	}, nil
}

func (k Keeper) OnLeftSwapReceived(ctx sdk.Context, msg *types.MsgLeftSwapRequest) (*types.MsgSwapResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	poolId := types.GeneratePoolId([]string{msg.TokenIn.Denom, msg.DenomOut})

	amm, err := k.CreateIBCSwapAMM(ctx, poolId) // Pool Token denomination is the pool Id
	if err != nil {
		return nil, err
	}

	outToken, err := amm.LeftSwap(msg.TokenIn, msg.DenomOut)
	if err != nil {
		return nil, err
	}

	k.SetBalancerPool(ctx, *amm.Pool) // update pool states

	// only output one asset in the pool
	return &types.MsgSwapResponse{
		TokenOut: &outToken,
	}, nil
}

func (k Keeper) OnRightSwapReceived(ctx sdk.Context, msg *types.MsgRightSwapRequest) (*types.MsgSwapResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	poolId := types.GeneratePoolId([]string{msg.TokenIn.Denom, msg.TokenOut.Denom})

	amm, err := k.CreateIBCSwapAMM(ctx, poolId) // Pool Token denomination is the pool Id
	if err != nil {
		return nil, err
	}

	outToken, err := amm.RightSwap(msg.TokenIn.Denom, msg.TokenOut)
	if err != nil {
		return nil, err
	}

	k.SetBalancerPool(ctx, *amm.Pool) // update pool states

	// only output one asset in the pool
	return &types.MsgSwapResponse{
		TokenOut: &outToken,
	}, nil
}

func (k Keeper) OnCreatePoolAcknowledged(ctx sdk.Context, request *types.MsgCreatePoolRequest, response *types.MsgCreatePoolResponse) error {

}

func (k Keeper) OnSingleDepositAcknowledged(ctx sdk.Context, request *types.MsgSingleDepositRequest, response *types.MsgSingleDepositResponse) error {

}

func (k Keeper) OnWithdrawAcknowledged(ctx sdk.Context, request *types.MsgWithdrawRequest, response *types.MsgWithdrawResponse) error {

}

func (k Keeper) OnLeftSwapAcknowledged(ctx sdk.Context, request *types.MsgLeftSwapRequest, response *types.MsgSwapResponse) error {

}

func (k Keeper) OnRightSwapAcknowledged(ctx sdk.Context, request *types.MsgRightSwapRequest, response *types.MsgSwapResponse) error {

}
```

#### Port & channel setup

The fungible token swap module on a chain must always bind to a port with the id `interchainswap`

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("interchainswap", ModuleCallbacks{
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

An interchain swap module will accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics101-1`.

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

#### Packet relay

`SendIBCSwapDelegationDataPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

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

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

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

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

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

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that the tokens will be refunded).  Tokens are also refunded on failure.

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

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Coming soon.

## Example Implementation

https://github.com/ibcswap/ibcswap

## Other Implementations

Coming soon.

## History

Oct 9, 2022 - Draft written

Oct 11, 2022 - Draft revised

## References

https://dev.balancer.fi/resources/pool-math/weighted-math#spot-price

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
