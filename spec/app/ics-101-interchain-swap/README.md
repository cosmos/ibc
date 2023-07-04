---
ics: 101
title: Interchain Swap
stage: draft
category: IBC/APP
kind: instantiation
author: Ping <ping@side.one>, Edward Gunawan <edward@s16.ventures>, Marian <marian@side.one>
created: 2022-10-09
modified: 2023-04-06
requires: 24, 25
---

## Synopsis

This standard document specifies the packet data structure, state machine handling logic, and encoding details for token exchange through single-sided liquidity pools over an IBC channel between separate chains.

### Motivation

ICS-101 Interchain Swaps enables chains their own token pricing mechanism and exchange protocol via IBC transactions. Each chain can thus play a role in a fully decentralised exchange network.

Users might also prefer single asset pools over dual assets pools as it removes the risk of impermanent loss.

### Definitions

`Interchain swap`: a IBC token swap protocol, built on top of an automated marketing making system, which leverages liquidity pools and incentives. Each chain that integrates this app becomes part of a decentralized exchange network.

`Automated market makers(AMM)`: are decentralized exchanges that pool liquidity and allow tokens to be traded in a permissionless and automatic way. Usually uses an invariant for token swapping calculation. In this interchain standard, the Balancer algorithm is implemented.

`Weighted pools`: liquidity pools characterized by the percentage weight of each token denomination maintained within.

`Single-sided liquidity pools`: a liquidity pool that does not require users to deposit both token denominations -- one is enough.

`Left-side swap`: a token exchange that specifies the desired quantity to be sold.

`Right-side swap`: a token exchange that specifies the desired quantity to be purchased.

`Pool state`: the entire state of a liquidity pool including its invariant value which is derived from its token balances and weights inside.

### Desired Properties

- `Permissionless`: no need to whitelist connections, modules, or denominations. Individual implementations may have their own permissioning scheme, however the protocol must not require permissioning from a trusted party to be secure.
- `Decentralization`: all parameters are managed on chain via governance. Does not require any central authority or entity to function. Also does not require a single blockchain, acting as a hub, to function.
- `Gaurantee of Exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.
- `Liquidity Incentives`: supports the collection of fees which are distributed to liquidity providers and acts as incentive for liquidity participation.
- `Weighted Math`: allows the configuration of pool weights so users can choose their levels of exposure between the tokens.

## Technical Specification

### Algorithms

#### Invariant

A constant invariant is maintained after trades which takes into consideration token weights and balance. The value function $V$ is defined as:

$$V = {&Pi;_tB_t^{W_t}}$$

Where

- $t$ ranges over the tokens in the pool
- $B_t$ is the balance of the token in the pool
- $W_t$ is the normalized weight of the tokens, such that the sum of all normalized weights is 1.

#### Spot Price

Spot prices of tokens are defined entirely by the weights and balances of the token pair. The spot price between any two tokens, $SpotPrice_i^{o}$, or in short $SP_i^o$, is the ratio of the token balances normalized by their weights:

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

#### Pool Structure

```ts
interface Coin {
  amount: int32;
  denom: string;
}
```

```ts
enum PoolAssetSide {
  Source = 1;
  Destination = 2;
}
```

```ts
// PoolStatus defines if the pool is ready for trading
enum PoolStatus {
  INITIALIZED = 0;
  ACTIVE = 1;
}
```

```ts
interface PoolAsset {
  side: PoolAssetSide;
  balance: Coin;
  // percentage
  weight: int32;
  decimal: int32;
}
```

```ts
interface InterchainLiquidityPool {
  id: string;
  sourceCreator: string;
  destinationCreator: string;
  assets: []PoolAsset;
  swapFee: int32;
  // the issued amount of pool token in the pool. the denom is pool id
  supply: Coin;
  status: PoolStatus;
  encounterPartyPort: string;
  encounterPartyChannel: string;
  constructor(denoms: []string, decimals: []number, weights: []number,swapFee: number, portId string, channelId string) {

    this.id = generatePoolId(denoms)
    this.supply = {
       amount: 0,
       denom: this.id
    }
    this.status = PoolStatus.POOL_STATUS_INITIAL
    this.encounterPartyPort = portId
    this.encounterPartyChannel = channelId
    this.swapFee = swapFee
    // construct assets
    if(denoms.length === decimals.lenght && denoms.length === weight.length) {
        for(let i=0; i < denoms.lenght; i++) {
            this.assets.push({
               side: store.hasSupply(denom[i]) ? PoolAssetSide.Source: PoolAssetSide.Destination,
               balance: {
                 amount: 0,
                 denom: denom[i],
               },
               weight: weights[i],
               decimal: decimals[i],
            })
        }
    }
  }
}
```

```ts
function generatePoolId(denoms: string[]) {
  return "pool" + sha256(denoms.sort().join(""));
}

function generatePoolId(sourceChainId: string, destinationChainId: string, denoms: string[]): string {
  const connectionId: string = getConnectID([sourceChainId, destinationChainId]);
  denoms.sort();

  const poolIdHash = createHash("sha256");
  denoms.push(connectionId);
  poolIdHash.update(denoms.join(""));

  const poolId = "pool" + poolIdHash.digest("hex");
  return poolId;
}

function getConnectID(chainIds: string[]): string {
  // Generate poolId
  chainIds.sort();
  return chainIds.join("/");
}
```

#### IBC Market Maker

```ts
class InterchainMarketMaker {
    pool :InterchainLiquidityPool
    static initialize(pool: InterchainLiquidityPool) : InterchainMarketMaker {
        return {
            pool: pool
        }
    }

    // MarketPrice Bi / Wi / (Bo / Wo)
    function marketPrice(denomIn, denomOut string): float64 {
        const tokenIn = this.Pool.findAssetByDenom(denomIn)
        const tokenOut = this.Pool.findAssetByDenom(denomOut)
        const balanceIn = tokenIn.balance.amount
        const balanceOut = tokenOut.balance.amount
        const weightIn := tokenIn.weight
        const weightOut := tokenOut.weight

        return balanceIn / weightIn / (balanceOut / weightOut)
    }

    // P_issued = P_supply * ((1 + At/Bt) ** Wt -1)
    function depositSingleAsset(token: Coin): Coin {
        const asset = this.pool.findAssetByDenom(token.denom)
        const amount = token.amount
        const supply = this.pool.supply.amount
        const weight = asset.weight / 100
        const issueAmount = supply * (math.pow(1+amount/asset.balance, weight) - 1)

        asset.balance.amount += token.amount // update balance of the asset

        return {
            amount: issueAmount,
            denom: this.pool.supply.denom
        }
    }

    // P_issued = P_supply * Wt * Dt/Bt
    function depositMultiAsset(tokens: Coin[]): Coin[] {
    const outTokens: Coin[] = [];
    for (const token of tokens) {
      const asset = imm.Pool.FindAssetByDenom(token.Denom);
      if (!asset) {
        throw new Error("Asset not found");
      }

      let issueAmount: Int;

      if (imm.Pool.Status === PoolStatus_INITIALIZED) {
        let totalAssetAmount = new Int(0);
        for (const asset of imm.Pool.Assets) {
          totalAssetAmount = totalAssetAmount.Add(asset.Balance.Amount);
        }
        issueAmount = totalAssetAmount.Mul(new Int(asset.Weight)).Quo(new Int(100));
      } else {
        const decToken = new DecCoinFromCoin(token);
        const decAsset = new DecCoinFromCoin(asset.Balance);
        const decSupply = new DecCoinFromCoin(imm.Pool.Supply);

        const ratio = decToken.Amount.Quo(decAsset.Amount).Mul(new Dec(Multiplier));
        issueAmount = decSupply.Amount.Mul(new Dec(asset.Weight)).Mul(ratio).Quo(new Dec(100)).Quo(new Dec(Multiplier)).RoundInt();
      }

      const outputToken: Coin = {
        Amount: issueAmount,
        Denom: imm.Pool.Supply.Denom,
      };
      outTokens.push(outputToken);
    }

    return outTokens;
  }

    // input the supply token, output the expected token.
    // At = Bt * (P_redeemed / P_supply)/Wt
    multiAssetWithdraw(redeem: Coin): Coin[] {
    const outs: Coin[] = [];

    if (redeem.Amount.GT(imm.Pool.Supply.Amount)) {
      throw new Error("Overflow amount");
    }

    for (const asset of imm.Pool.Assets) {
      const out = asset.Balance.Amount.Mul(redeem.Amount).Quo(imm.Pool.Supply.Amount);
      const outputCoin: Coin = {
        Denom: asset.Balance.Denom,
        Amount: out,
      };
      outs.push(outputCoin);
    }

    return outs;
  }

    // LeftSwap implements OutGivenIn
    // Input how many coins you want to sell, output an amount you will receive
    // Ao = Bo * (1 -(Bi / (Bi + Ai)) ** Wi/Wo)
    function leftSwap(amountIn: Coin, denomOut: string): Coin {

        const assetIn = this.pool.findAssetByDenom(amountIn.denom)
        abortTransactionUnless(assetIn != null)

        const assetOut = this.pool.findAssetByDenom(denomOut)
        abortTransactionUnless(assetOut != null)

        // redeem.weight is percentage
        const balanceOut = assetOut.balance.amount
        const balanceIn = assetIn.balance.amount
        const weightIn = assetIn.weight / 100
        const weightOut = assetOut.weight / 100
        const amount = this.minusFees(amountIn.amount)

        const amountOut := balanceOut * (1- (balanceIn / (balanceIn + amount)) ** (weightIn/weightOut))

        return {
            amount: amountOut,
            denom:denomOut
        }
    }

    // RightSwap implements InGivenOut
    // Input how many coins you want to buy, output an amount you need to pay
    // Ai = Bi * ((Bo/(Bo - Ao)) ** Wo/Wi -1)
    function rightSwap(amountIn: Coin, amountOut: Coin) Coin {

        const assetIn = this.pool.findAssetByDenom(amountIn.denom)
        abortTransactionUnless(assetIn != null)
        const AssetOut = this.pool.findAssetByDenom(amountOut.denom)
        abortTransactionUnless(assetOut != null)

        const balanceIn = assetIn.balance.amount
        const balanceOut = assetOut.balance.amount
        const weightIn = assetIn.weight / 100
        const weightOut = assetOut.weight / 100

        const amount = balanceIn * ((balanceOut/(balanceOut - amountOut.amount) ** (weightOut/weightIn) - 1)

        abortTransactionUnless(amountIn.amount > amount)

        return {
            amount,
            denom: amountIn.denom
        }
    }

    // amount - amount * feeRate / 10000
    function minusFees(amount sdk.Int) sdk.Int {
        return amount * (1 - this.pool.feeRate / 10000))
    }
}
```

#### Data packets

Only one packet data type is required: `IBCSwapDataPacket`, which specifies the message type and data(protobuf marshalled). It is a wrapper for interchain swap messages.

```ts
enum MessageType {
  MakePool,
  TakePool,
  MakeMultiAssetDeposit
  TakeMultiAssetDeposit
  MultiAssetWithdraw,
  LeftSwap,
  RightSwap,
}

interface StateChange {
  in: Coin[];
  out: Out[];
  poolTokens: Coin[];
  poolId: string;
  multiDepositOrderId: string;
  sourceChainId: string;
}
```

```ts
// IBCSwapDataPacket is used to wrap message for relayer.
interface IBCSwapDataPacket {
    type: MessageType,
    data: []byte, // Bytes
    stateChange: StateChange
}
```

```typescript
type IBCSwapDataAcknowledgement = IBCSwapDataPacketSuccess | IBCSwapDataPacketError;

interface IBCSwapDataPacketSuccess {
  // This is binary 0x01 base64 encoded
  result: "AQ==";
}

interface IBCSwapDataPacketError {
  error: string;
}
```

### Sub-protocols

Traditional liquidity pools typically maintain its pool state in one location.

A liquidity pool in the interchain swap protocol maintains its pool state on both its source chain and destination chain. The pool states mirror each other and are synced through IBC packet relays, which we elaborate on in the following sub-protocols.

IBCSwap implements the following sub-protocols:

```protobuf
  rpc MakePool (MsgMakePoolRequest) returns (MsgMakePoolResponse);
  rpc TakePool (MsgTakePoolRequest) returns (MsgTakePoolResponse);
  rpc SingleAssetDeposit    (MsgSingleAssetDepositRequest   ) returns (MsgSingleAssetDepositResponse   );
  rpc MakeMultiAssetDeposit    (MsgMakeMultiAssetDepositRequest   ) return (MsgMultiAssetDepositResponse   );
  rpc TakeMultiAssetDeposit    (MsgTakeMultiAssetDepositRequest   ) returns (MsgMultiAssetDepositResponse   );
  rpc MultiAssetWithdraw   (MsgMultiAssetWithdrawRequest  ) returns (MsgMultiAssetWithdrawResponse  );
  rpc Swap       (MsgSwapRequest             ) returns (MsgSwapResponse      );
```

#### Interfaces for sub-protocols

```ts
interface MsgMakePoolRequest {
  sourcePort: string;
  sourceChannel: string;
  creator: string;
  counterPartyCreator: string;
  liquidity: PoolAsset[];
  sender: string;
  denoms: string[];
  decimals: int32[];
  swapFee: int32;
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}

interface MsgMakePoolResponse {
  poolId: string;
}
```

```ts
interface MsgTakePoolRequest {
  creator: string;
  poolId: string;
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}

interface MsgTakePoolResponse {
  poolId: string;
}
```

```ts
interface MsgSingleAssetDepositRequest {
  poolId: string;
  sender: string;
  token: Coin; // only one element for now, might have two in the feature
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}
interface MsgSingleDepositResponse {
  poolToken: Coin;
}
```

```ts
interface DepositAsset {
  sender: string;
  balance: Coin;
}

interface MsgMakeMultiAssetDepositRequest {
  poolId: string;
  deposits: DepositAsset[];
  token: Coin; // only one element for now, might have two in the feature
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}

interface MsgTakeMultiAssetDepositRequest {
  sender: string;
  poolId: string;
  orderId: uint64;
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}

interface MsgMultiAssetDepositResponse {
  poolToken: Coin;
}
```

```ts
interface MsgMultiAssetWithdrawRequest {
  poolId: string;
  receiver: string;
  counterPartyReceiver: string;
  poolToken: Coin;
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}

interface MsgMultiAssetWithdrawResponse {
  tokens: Coin[];
}
```

```ts
interface MsgSwapRequest {
  swap_type: SwapMsgType;
  sender: string;
  poolId: string;
  tokenIn: Coin;
  tokenOut: Coin;
  slippage: uint64;
  recipient: string;
  timeHeight: TimeHeight;
  timeoutTimeStamp: uint64;
}

interface MsgSwapResponse {
  swap_type: SwapMsgType;
  tokens: Coin[];
}
```

### Control Flow And Life Scope

To implement interchain swap, we introduce the `Message Delegator` and `Relay Listener`. The `Message Delegator` will pre-process the request (validate msgs, lock assets, etc), and then forward the transactions to the relayer.

```ts
  function makePool(msg: MsgMakePoolRequest): Promise<MsgMakePoolResponse> {

    const { counterPartyChainId, connected } = await store.GetCounterPartyChainID(msg.sourcePort, msg.sourceChannel);

    abortTransactionUnless(connected)

    const denoms: string[] = [];
    for (const liquidity of msg.liquidity) {
      denoms.push(liquidity.balance.denom);
    }

    const poolId = getPoolId(store.chainID(), counterPartyChainId, denoms);

    const found = await k.getInterchainLiquidityPool(poolId);

    abortTransactionUnless(found)

    // Validate message
    const portValidationErr = host.PortIdentifierValidator(msg.SourcePort);

    abortTransactionUnless(portValidationErr === undefined)

    const channelValidationErr = host.ChannelIdentifierValidator(msg.SourceChannel);

    abortTransactionUnless(channelValidationErr === undefined)

    const validationErr = msg.ValidateBasic();

    abortTransactionUnless(validationErr === undefined)

    abortTransactionUnless(store.hasSupply(msg.liquidity[0].balance.denom))


    const sourceLiquidity = store.GetBalance(msg.creator, msg.liquidity[0].balance.denom);

    abortTransactionUnless(sourceLiquidity.amount > msg.liquidity[0].balance.amount)


    const lockErr = store.lockTokens(msg.sourcePort, msg.sourceChannel, senderAddress, msg.liquidity[0].balance);

    abortTransactionUnless(lockErr === undefined)

    const packet: IBCSwapPacketData = {
      type: "MAKE_POOL",
      data: protobuf.encode(msg),
      stateChange: {
        poolId: poolId,
        sourceChainId: store.ChainID(),
      },
    };

    const sendPacketErr = await store.sendIBCSwapPacket(msg.sourcePort, msg.sourceChannel, timeoutHeight, timeoutStamp, packet);

    abortTransactionUnless(sendPacketErr === undefined)
    return {
      poolId
    };
  }

  function takePool(msg: MsgTakePoolRequest): MsgTakePoolResponse {

    const { pool, found } = await store.getInterchainLiquidityPool(msg.PoolId);
    abortTransactionUnless(found)

    abortTransactionUnless(pool.SourceChainId !== store.ChainID())
    abortTransactionUnless(pool.DestinationCreator === msg.Creator)

    const creatorAddr = sdk.MustAccAddressFromBech32(msg.Creator);

    const asset = pool.FindAssetBySide("SOURCE");
    abortTransactionUnless(asset)

    const liquidity = store.GetBalance(creatorAddr, asset.denom);
    abortTransactionUnless(liquidity.amount > 0)


    const lockErr = store.LockTokens(pool.counterPartyPort, pool.counterPartyChannel, creatorAddr, asset);
    abortTransactionUnless(lockErr === undefined)

    const packet: IBCSwapPacketData = {
      type: "TAKE_POOL",
      data: protobuf.encode(msg),
    };


    const sendPacketErr = await store.SendIBCSwapPacket(pool.counterPartyPort, pool.counterPartyChannel, timeoutHeight, timeoutStamp, packet);
    abortTransactionUnless(sendPacketErr === undefined)

    return {
      poolId: msg.PoolId,
    };
  }

  function makeMultiAssetDeposit(msg: MsgMakeMultiAssetDepositRequest): MsgMultiAssetDepositResponse {

    const { pool, found } = await store.getInterchainLiquidityPool(msg.poolId);

    abortTransactionUnless(found)

    // Check initial deposit condition
    abortTransactionUnless(pool.status === "ACTIVE")

    // Check input ratio of tokens
    const sourceAsset = pool.findAssetBySide("SOURCE");
    abortTransactionUnless(sourceAsset)

    const destinationAsset = pool.findAssetBySide("DESTINATION");
    abortTransactionUnless(destinationAsset)

    const currentRatio = sourceAsset.amount.Mul(sdk.NewInt(1e18)).Quo(destinationAsset.amount);
    const inputRatio = msg.deposits[0].balance.amount.Mul(sdk.NewInt(1e18)).Quo(msg.deposits[1].balance.amount);

    const slippageErr = checkSlippage(currentRatio, inputRatio, 10);
    abortTransactionUnless(slippageErr)

    // Create escrow module account here
    const lockErr = store.lockTokens(pool.counterPartyPort, pool.counterPartyChannel, sdk.msg.deposits[0].sender, msg.deposits[0].balance);
    abortTransactionUnless(lockErr)

    const amm = new InterchainMarketMaker(pool);

    const poolTokens = await amm.depositMultiAsset([
      msg.deposits[0].balance,
      msg.deposits[1].balance,
    ]);

    // create order
    const order: MultiAssetDepositOrder = {
      poolId: msg.poolId;
      chainId: store.chainID(),
      sourceMaker: msg.deposits[0].sender,
      destinationTaker: msg.deposits[1].sender,
      deposits: getCoinsFromDepositAssets(msg.deposits),
      status: "PENDING";
      createdAt: store.blockHeight(),
    };

    // save order in source chain
    store.appendMultiDepositOrder(pool.Id, order);

    const packet: IBCSwapPacketData = {
      type: "MAKE_MULTI_DEPOSIT",
      data: protobuf.encode(msg),
      stateChange: { poolTokens: poolTokens },
    };

    const sendPacketErr = await store.sendIBCSwapPacket(pool.counterPartyPort, pool.counterPartyChannel, timeoutHeight, timeoutStamp, packet);
    abortTransactionUnless(sendPacketErr === undefined)

    return { poolTokens };
  }


  function takeMultiAssetDeposit(msg: MsgTakeMultiAssetDepositRequest): MsgMultiAssetDepositResponse {

  // check pool exist or not
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found)

  // check order exist or not
  const { order, found } = store.getMultiDepositOrder(msg.poolId, msg.orderId);
  abortTransactionUnless(found)

  abortTransactionUnless(order.chainId !== store.chainID())
  abortTransactionUnless(msg.sender === order.destinationTaker)
  abortTransactionUnless(order.status !== "COMPLETE")


  // estimate pool token
  const amm = new InterchainMarketMaker(pool);
  const poolTokens = await amm.depositMultiAsset(order.deposits);

  // check asset owned status
  const asset = order.deposits[1];
  const balance = store.getBalance(msg.sender, asset.denom);
  abortTransactionUnless(balance.amount < asset.amount)


  // Create escrow module account here
  const lockErr = store.lockTokens(pool.counterPartyPort, pool.counterPartyChannel,msg.sender, asset);
  abortTransactionUnless(lockErr === undefined)

  const packet: IBCSwapPacketData = {
    type: "TAKE_MULTI_DEPOSIT",
    data: protobuf.encode(msg),
    stateChange: { poolTokens },
  };


  const sendPacketErr = await store.sendIBCSwapPacket(pool.counterPartyPort, pool.counterPartyChannel, timeoutHeight, timeoutStamp, packet);
  abortTransactionUnless(sendPacketErr === undefined)

  return {};
}

function singleAssetDeposit(msg: MsgSingleAssetDepositRequest): MsgSingleAssetDepositResponse {
  // Validate message
  const validationErr = msg.validateBasic();
  abortTransactionUnless(validationErr === undefined);

  // Check if pool exists
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  // Deposit token to escrow account
  const balance = store.getBalance(msg.sender, msg.token.denom);
  abortTransactionUnless(balance.amount.gt(sdk.NewInt(0)));

  // Check pool status
  abortTransactionUnless(pool.status === "ACTIVE");

  // Lock tokens in escrow account
  const lockErr = store.lockTokens(pool.counterPartyPort, pool.counterPartyChannel, msg.sender, sdk.NewCoins(msg.token));
  abortTransactionUnless(lockErr === undefined);

  const amm = new InterchainMarketMaker(pool);

  const poolToken = await amm.depositSingleAsset(msg.token);
  if (poolToken === undefined) {
    throw new Error("Failed to deposit single asset.");
  }

  const packet: IBCSwapPacketData = {
    type: "SINGLE_DEPOSIT",
    data: protobuf.encode(msg);,
    stateChange: { poolTokens: [poolToken] },
  };

  const sendPacketErr = await store.sendIBCSwapPacket(pool.counterPartyPort, pool.counterPartyChannel, timeoutHeight, timeoutStamp, packet);
  abortTransactionUnless(sendPacketErr === undefined);

  return { poolToken: pool.supply };
}


function multiAssetWithdraw(msg: MsgMultiAssetWithdrawRequest): MsgMultiAssetWithdrawResponse {
  // Validate message
  const validationErr = msg.validateBasic();
  abortTransactionUnless(validationErr === undefined);

  // Check if pool token denom exists
  const poolTokenDenom = msg.poolToken.denom;
  abortTransactionUnless(store.bankKeeper.hasSupply(poolTokenDenom));

  // Get the liquidity pool
  const { pool, found } = k.getInterchainLiquidityPool(ctx, poolTokenDenom);
  abortTransactionUnless(found);

  const amm = new InterchainMarketMaker(pool);

  const outs = await amm.multiAssetWithdraw(msg.poolToken);
  abortTransactionUnless(outs === undefined);

  const packet: IBCSwapPacketData = {
    type: "MULTI_WITHDRAW",
    data: protobuf.encode(msg),
    stateChange: {
      out: outs,
      poolTokens: [msg.poolToken],
    },
  };

  const sendPacketErr = await k.sendIBCSwapPacket(ctx, pool.counterPartyPort, pool.counterPartyChannel, timeoutHeight, timeoutStamp, packet);
  abortTransactionUnless(sendPacketErr === undefined);

  return {};
}


function swap(msg: MsgSwapRequest): MsgSwapResponse {
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  abortTransactionUnless(pool.status === "ACTIVE");

  const lockErr = store.lockTokens(pool.counterPartyPort, pool.counterPartyChannel, msg.sender, msg.tokenIn);
  abortTransactionUnless(lockErr === undefined);

  const amm = new InterchainMarketMaker(pool);

  let tokenOut: sdk.Coin | undefined;
  let msgType: SwapMessageType;

  switch (msg.swapType) {
    case "LEFT":
      msgType = "LEFT_SWAP";
      tokenOut = amm.leftSwap(msg.tokenIn, msg.tokenOut.denom);
      break;
    case "RIGHT":
      msgType = "RIGHT_SWAP";
      tokenOut = amm.rightSwap(msg.tokenIn, msg.tokenOut);
      break;
    default:
       abortTransactionUnless(false);
  }


  abortTransactionUnless(tokenOut?.amount? <= 0);

  const factor = MaximumSlippage - msg.slippage;
  const expected = msg.tokenOut.amount
    .mul(sdk.NewIntFromUint64(factor))
    .quo(sdk.NewIntFromUint64(MaximumSlippage));

  abortTransactionUnless(tokenOut?.amount?.gte(expected));

  const packet: IBCSwapPacketData = {
    type: msgType,
    data: protobuf.encode(msg),
    stateChange: { out: [tokenOut] },
  };

  const sendPacketErr = store.sendIBCSwapPacket(
    pool.counterPartyPort,
    pool.counterPartyChannel,
    timeoutHeight,
    timeoutTimestamp,
    packet
  );
  abortTransactionUnless(sendPacketErr === undefined);

  return {
    swapType: msg.swapType,
    tokens: [msg.tokenIn, msg.tokenOut],
  };
}
```

The `Relay Listener` handle all transactions, execute transactions when received, and send the result as an acknowledgement. In this way, packets relayed on the source chain update pool states on the destination chain according to results in the acknowledgement.

```ts
function onMakePoolReceived(msg: MsgMakePoolRequest, poolID: string, sourceChainId: string): string {
  abortTransactionUnless(msg.validateBasic() === undefined);
  const { pool, found } = store.getInterchainLiquidityPool(poolID);
  abortTransactionUnless(msg.validateBasic() === undefined);

  const liquidityBalance = msg.liquidity[1].balance;
  if (!store.bankKeeper.hasSupply(liquidityBalance.denom)) {
    throw new Error(`Invalid decimal pair: ${types.ErrFailedOnDepositReceived}`);
  }

  const interchainLiquidityPool = new InterchainLiquidityPool(
    poolID,
    msg.creator,
    msg.counterPartyCreator,
    store.bankKeeper,
    msg.liquidity,
    msg.swapFee,
    msg.sourcePort,
    msg.sourceChannel
  );
  interchainLiquidityPool.sourceChainId = sourceChainId;

  const interchainMarketMaker = new InterchainMarketMaker(interchainLiquidityPool);
  interchainLiquidityPool.poolPrice = interchainMarketMaker.lpPrice();

  store.setInterchainLiquidityPool(interchainLiquidityPool);
  return poolID;
}

function onTakePoolReceived(msg: MsgTakePoolRequest): string {
  abortTransactionUnless(msg.validateBasic() === undefined);
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  pool.status = "ACTIVE";
  const asset = pool.findPoolAssetBySide("DESTINATION");
  abortTransactionUnless(asset === undefined);

  const totalAmount = pool.sumOfPoolAssets();
  const mintAmount = totalAmount.mul(sdk.NewInt(asset.weight)).quo(sdk.NewInt(100));

  store.mintTokens(pool.sourceCreator, new sdk.Coin(pool.supply.denom, mintAmount));
  store.setInterchainLiquidityPool(pool);
  return pool.id;
}

function onSingleAssetDepositReceived(
  msg: MsgSingleAssetDepositRequest,
  stateChange: StateChange
): MsgSingleAssetDepositResponse {
  abortTransactionUnless(msg.validateBasic() === undefined);
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  pool.addPoolSupply(stateChange.poolTokens[0]);
  pool.addAsset(msg.token);

  store.setInterchainLiquidityPool(pool);

  return {
    poolToken: stateChange.poolTokens[0],
  };
}

function onMakeMultiAssetDepositReceived(
  msg: MsgMakeMultiAssetDepositRequest,
  stateChange: StateChange
): MsgMultiAssetDepositResponse {
  abortTransactionUnless(msg.validateBasic() === undefined);
  const [senderPrefix, , err] = bech32.decode(msg.deposits[1].sender);
  abortTransactionUnless(store.getConfig().getBech32AccountAddrPrefix() !== senderPrefix);

  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  const order: MultiAssetDepositOrder = {
    poolId: msg.poolId,
    chainId: pool.sourceChainId,
    sourceMaker: msg.deposits[0].sender,
    destinationTaker: msg.deposits[1].sender,
    deposits: getCoinsFromDepositAssets(msg.deposits),
    status: "PENDING",
    createdAt: store.blockHeight(),
  };

  store.appendMultiDepositOrder(msg.poolId, order);

  return {
    poolTokens: stateChange.poolTokens,
  };
}

function onTakeMultiAssetDepositReceived(
  msg: MsgTakeMultiAssetDepositRequest,
  stateChange: StateChange
): MsgMultiAssetDepositResponse {
  abortTransactionUnless(msg.validateBasic() === undefined);

  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  const { order, found: orderFound } = store.getMultiDepositOrder(msg.poolId, msg.orderId);
  abortTransactionUnless(orderFound);
  order.status = "COMPLETE";

  for (const supply of stateChange.poolTokens) {
    pool.addPoolSupply(supply);
  }

  for (const asset of order.deposits) {
    pool.addAsset(asset);
  }

  const totalPoolToken = sdk.NewCoin(msg.poolId, sdk.NewInt(0));
  for (const poolToken of stateChange.poolTokens) {
    totalPoolToken.amount = totalPoolToken.amount.add(poolToken.amount);
  }

  store.mintTokens(order.sourceMaker, totalPoolToken);

  store.setInterchainLiquidityPool(pool);
  store.setMultiDepositOrder(pool.id, order);

  return {};
}

function onMultiAssetWithdrawReceived(
  msg: MsgMultiAssetWithdrawRequest,
  stateChange: StateChange
): MsgMultiAssetWithdrawResponse {
  abortTransactionUnless(msg.validateBasic() === undefined);
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolToken.denom);
  abortTransactionUnless(found);

  for (const poolAsset of stateChange.out) {
    pool.subtractAsset(poolAsset);
  }

  for (const poolToken of stateChange.poolTokens) {
    pool.subtractPoolSupply(poolToken);
  }

  store.unlockTokens(
    pool.counterPartyPort,
    pool.counterPartyChannel,
    msg.counterPartyReceiver,
    sdk.NewCoins(stateChange.out[1])
  );

  if (pool.supply.amount.isZero()) {
    store.removeInterchainLiquidityPool(pool.id);
  } else {
    store.setInterchainLiquidityPool(pool);
  }

  return { tokens: stateChange.out };
}

function onSwapReceived(msg: MsgSwapRequest, stateChange: StateChange): MsgSwapResponse {
  abortTransactionUnless(msg.validateBasic() === undefined);

  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);
  store.unlockTokens(pool.counterPartyPort, pool.counterPartyChannel, msg.recipient, sdk.NewCoins(stateChange.out[0]));

  pool.subtractAsset(stateChange.out[0]);
  pool.addAsset(msg.tokenIn);

  store.setInterchainLiquidityPool(pool);

  return { tokens: stateChange.out };
}

function onMakePoolAcknowledged(msg: MsgMakePoolRequest, poolId: string): void {
  const pool = new InterchainLiquidityPool(
    ctx,
    msg.creator,
    msg.counterPartyCreator,
    k.bankKeeper,
    poolId,
    msg.liquidity,
    msg.swapFee,
    msg.sourcePort,
    msg.sourceChannel
  );

  pool.sourceChainId = store.chainID();

  const totalAmount = sdk.NewInt(0);
  for (const asset of msg.liquidity) {
    totalAmount = totalAmount.add(asset.balance.amount);
  }

  store.mintTokens(msg.creator, {
    denom: pool.supply.denom,
    amount: totalAmount.mul(msg.liquidity[0].weight).quo(100),
  });

  const amm = new InterchainMarketMaker(pool);
  pool.poolPrice = amm.lpPrice();

  store.setInterchainLiquidityPool(pool);
}

function onTakePoolAcknowledged(msg: MsgTakePoolRequest): void {
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);

  const amm = new InterchainMarketMaker(pool);
  pool.poolPrice = amm.lpPrice();
  pool.status = "ACTIVE";

  store.setInterchainLiquidityPool(pool);
}

function onSingleAssetDepositAcknowledged(req: MsgSingleAssetDepositRequest, res: MsgSingleAssetDepositResponse): void {
  const { pool, found } = store.getInterchainLiquidityPool(req.poolId);
  abortTransactionUnless(found);

  store.mintTokens(req.sender, res.poolToken);

  pool.addAsset(req.token);
  pool.addPoolSupply(res.poolToken);

  store.setInterchainLiquidityPool(pool);
}

function onMakeMultiAssetDepositAcknowledged(
  req: MsgMakeMultiAssetDepositRequest,
  res: MsgMultiAssetDepositResponse
): void {
  const { pool, found } = k.getInterchainLiquidityPool(ctx, req.poolId);
  abortTransactionUnless(found);
  store.setInterchainLiquidityPool(pool);
}

function onTakeMultiAssetDepositAcknowledged(req: MsgTakeMultiAssetDepositRequest, stateChange: StateChange): void {
  const { pool, found } = store.getInterchainLiquidityPool(req.poolId);
  abortTransactionUnless(found);

  const order = store.getMultiDepositOrder(req.poolId, req.orderId);
  abortTransactionUnless(order.found);

  for (const poolToken of stateChange.poolTokens) {
    pool.addPoolSupply(poolToken);
  }

  for (const deposit of order.deposits) {
    pool.addAsset(deposit);
  }

  order.status = "COMPLETE";

  store.setInterchainLiquidityPool(pool);
  store.setMultiDepositOrder(pool.id, order);
}

function onMakePoolAcknowledged(msg: MsgMakePoolRequest, poolId: string): void {
  const pool = new InterchainLiquidityPool(
    ctx,
    msg.creator,
    msg.counterPartyCreator,
    k.bankKeeper,
    poolId,
    msg.liquidity,
    msg.swapFee,
    msg.sourcePort,
    msg.sourceChannel
  );

  pool.sourceChainId = store.chainID();

  const totalAmount = sdk.NewInt(0);
  for (const asset of msg.liquidity) {
    totalAmount = totalAmount.add(asset.balance.amount);
  }

  store.mintTokens(ctx, msg.creator, {
    denom: pool.supply.denom,
    amount: totalAmount.mul(sdk.NewInt(Number(msg.liquidity[0].weight))).quo(sdk.NewInt(100)),
  });

  const amm = new InterchainMarketMaker(pool);
  pool.poolPrice = amm.lpPrice();

  store.setInterchainLiquidityPool(pool);
}

function onTakePoolAcknowledged(msg: MsgTakePoolRequest): void {
  const { pool, found } = store.getInterchainLiquidityPool(msg.poolId);
  abortTransactionUnless(found);
  const amm = new InterchainMarketMaker(pool);
  pool.poolPrice = amm.lpPrice();
  pool.status = "ACTIVE";
  store.setInterchainLiquidityPool(pool);
}

function onSingleAssetDepositAcknowledged(req: MsgSingleAssetDepositRequest, res: MsgSingleAssetDepositResponse): void {
  const { pool, found } = store.getInterchainLiquidityPool(req.poolId);
  abortTransactionUnless(found);

  store.mintTokens(req.sender, res.poolToken);

  pool.addAsset(req.token);
  pool.addPoolSupply(res.poolToken);

  store.setInterchainLiquidityPool(pool);
}

function onMakeMultiAssetDepositAcknowledged(
  req: MsgMakeMultiAssetDepositRequest,
  res: MsgMultiAssetDepositResponse
): void {
  const { pool, found } = store.getInterchainLiquidityPool(req.poolId);
  abortTransactionUnless(found);
  store.setInterchainLiquidityPool(pool);
}

function onTakeMultiAssetDepositAcknowledged(req: MsgTakeMultiAssetDepositRequest, stateChange: StateChange): void {
  const { pool, found } = store.getInterchainLiquidityPool(req.poolId);
  abortTransactionUnless(found);
  const order,
    found = store.getMultiDepositOrder(req.poolId, req.orderId);
  abortTransactionUnless(found);
  for (const poolToken of stateChange.poolTokens) {
    pool.addPoolSupply(poolToken);
  }

  for (const deposit of order.deposits) {
    pool.addAsset(deposit);
  }

  order.status = "COMPLETE";

  store.setInterchainLiquidityPool(pool);
  store.setMultiDepositOrder(pool.id, order);
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
  counterpartyVersion: string
) {
  abortTransactionUnless(counterpartyVersion === "ics101-1");
}
```

#### Packet relay

`sendInterchainIBCSwapDataPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```ts
function sendInterchainIBCSwapDataPacket(
  swapPacket: IBCSwapPacketData,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64
) {
  // send packet using the interface defined in ICS4
  handler.sendPacket(getCapability("port"), sourcePort, sourceChannel, timeoutHeight, timeoutTimestamp, swapPacket);
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```ts
function onRecvPacket(packet: Packet) {

    IBCSwapPacketData swapPacket = packet.data
    // construct default acknowledgement of success
    const ack: IBCSwapDataAcknowledgement = new IBCSwapDataPacketSuccess()

    try{
        switch swapPacket.type {
        case CREATE_POOL:
            var msg: MsgCreatePoolRequest = protobuf.decode(swapPacket.data)
            onCreatePoolReceived(msg, packet.destPortId, packet.destChannelId)
            break
        case SINGLE_DEPOSIT:
            var msg: MsgSingleDepositRequest = protobuf.decode(swapPacket.data)
            onSingleDepositReceived(msg)
            break
        case WITHDRAW:
            var msg: MsgWithdrawRequest = protobuf.decode(swapPacket.data)
            onWithdrawReceived(msg)
            break
        case LEFT_SWAP:
            var msg: MsgLeftSwapRequest = protobuf.decode(swapPacket.data)
            onLeftswapReceived(msg)
            break
        case RIGHT_SWAP:
            var msg: MsgRightSwapRequest = protobuf.decode(swapPacket.data)
            onRightReceived(msg)
            break
        }
    } catch {
        ack = new IBCSwapDataPacketError()
    }

    // NOTE: acknowledgement will be written synchronously during IBC handler execution.
    return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```ts

// OnAcknowledgementPacket implements the IBCModule interface
function OnAcknowledgementPacket(
	packet: channeltypes.Packet,
	ack channeltypes.Acknowledgement,
)  {

    var ack channeltypes.Acknowledgement
    if (!ack.success()) {
        refund(packet)
    } else {
        const swapPacket = protobuf.decode(packet.data)
        switch swapPacket.type {
        case CREATE_POOL:
            onCreatePoolAcknowledged(msg)
            break;
        case SINGLE_DEPOSIT:
            onSingleDepositAcknowledged(msg)
            break;
        case WITHDRAW:
            onWithdrawAcknowledged(msg)
            break;
        case LEFT_SWAP:
            onLeftSwapAcknowledged(msg)
            break;
        case RIGHT_SWAP:
            onRightSwapAcknowledged(msg)
        }
    }

    return nil
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that the tokens will be refunded). Tokens are also refunded on failure.

```ts
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet);
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
