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

This standard document specifies the X, Y and Z for token exchange through single-sided liquidity pools over an IBC channel between separate chains.

### Motivation

ICS-101 Interchain Swaps enables chains their own token pricing mechanism and exchange protocol via IBC transactions.  Each chain can thus play a role in a fully decentralised exchange network.

Users might also prefer single asset pools over dual assets pools as it removes the risk of impermanent loss.

### Definitions

`Single-sided liquidity pools`: a liquidity pool that does not require users to deposit both token denominations -- one is enough.

`Left side swap`: a token exchange that specifies the desired quantity to be sold.

`Right side swap`: a token exchange that specifies the desired quantity to be purchased

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations.  Individual implementations may have their own permissioning scheme, however the protocol must not require permissioning from a trusted party to be secure.
- `Decentralization`: all parameters are managed on chain.  Does not require any central authority or entity to function.  Also does not require a single blockchain, acting as a hub, to function.
- `Gaurantee of Exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.

## Technical Specification

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




