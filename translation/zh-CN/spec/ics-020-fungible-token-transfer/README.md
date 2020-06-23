---
ics: 20
title: 同质通证转移
stage: 草案
category: IBC/APP
requires: 25, 26
kind: 实例化
author: Christopher Goes <cwgoes@interchain.berlin>
created: 2019-07-15
modified: 2020-02-24
---

## 概览

该标准规定了通过 IBC 通道在各自链上的两个模块之间进行通证转移的数据包的数据结构，状态机处理逻辑以及编码细节。本文所描述的状态机逻辑允许在无许可通道打开的情况下安全的处理多个链的通证。该逻辑通过在节点状态机上的 IBC 路由模块和一个现存的资产跟踪模块之间建立实现了一个同质通证转移的桥接模块。

### 动机

基于 IBC 协议连接的一组链的用户可能希望在一条链上能利用在另一条链上发行的资产来使用该链上的附加功能，例如交易或隐私保护，同时保持发行链上的原始资产的同质性。该应用层标准描述了一个在基于 IBC 连接的链间转移同质通证的协议，该协议保留了资产的同质性和资产所有权，限制了拜占庭错误的影响，并且无需额外许可。

### 定义

[ICS 25](../ics-025-handler-interface) 和 [ICS 26](../ics-026-routing-module) 分别定义了 IBC 处理接口和 IBC 路由模块接口。

### 所需属性

- 保持同质性（双向锚定）。
- 保持供应量不变（在单一源链和模块上保持不变或通胀）。
- 无许可的通证转移，无需将连接（connections）、模块或通证面额加入白名单。
- 对称（所有链实现相同的逻辑，hubs 和 zones 无协议差别）。
- 容错：防止由于链`B`的拜占庭行为造成源自链`A`的通证的拜占庭通货膨胀（尽管任何将通证转移到链`B`上的用户都面临风险）。

## 技术规范

### 数据结构

仅需要一个数据包数据类型`FungibleTokenPacketData`，该类型指定了面额，数量，发送账户，接受账户以及发送链是否为资产的发行链。

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
}
```

确认数据类型描述转账是成功还是失败，以及失败的原因（如果有）。

```typescript
interface FungibleTokenPacketAcknowledgement {
  success: boolean
  error: Maybe<string>
}
```

同质通证转移桥接模块跟踪与状态中指定通道关联的托管地址。假设`ModuleState`的字段在范围内。

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
}
```

### 子协议

本文所述的子协议应该在“同质通证转移桥接”模块中实现，并且可以访问 band 模块和 IBC 路由模块。

#### 端口 & 通道设置

当创建“同质通证转移桥接”模块时（也可能是区块链本身初始化时），必须仅调用一次`setup`函数用于绑定到对应的端口并创建一个托管地址（该地址由模块所有）。

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

调用`setup`函数后，通过在不同链上的同质通证转移模块之间的 IBC 路由模块创建通道。

管理员（具有在节点的状态机上创建连接和通道的权限）负责在本地链与其他链的状态机之间创建连接，在本地链与其他链的该模块（或支持该接口的其他模块）的实例之间创建通道。本规范仅定义了数据包处理语义，模块本身在任意时间点都无需关心连接或通道是否存在。

#### 路由模块回调

##### 通道生命周期管理

机器`A`和机器`B`在当且仅当以下情况下接受来自第三台机器上任何模块的新通道创建请求：

- 第三台机器的模块绑定到“bank”端口。
- 创建的通道是无序的。
- 版本号为空。

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // only allow channels to "bank" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // assert that version is "ics20-1"
  abortTransactionUnless(version === "ics20-1")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
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
  version: string,
  counterpartyVersion: string) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics20-1"
  abortTransactionUnless(version === "ics20-1")
  abortTransactionUnless(counterpartyVersion === "ics20-1")
  // only allow channels to "bank" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // port has already been validated
  // assert that version is "ics20-1"
  abortTransactionUnless(version === "ics20-1")
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
  // no action necessary
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

##### 数据包中继

用简单文字来描述就是，在 `A` 和`B`两个链间：

- 在源 zone 上，桥接模块会在发送链上托管现有的本地资产面额，并在接收链上生成凭证。
- 在目标 zone 上，桥接模块会在发送链上销毁本地凭证，并在接收链上解除对本地资产面额的托管。
- 当数据包超时时，本地资产将解除托管并退还给发送者，或将凭证发回给发送者。
- 确认数据用于处理失败，例如无效面额或无效目标帐户。返回失败的确认比终止交易更可取，因为它更容易使发送链根据失败的本质而采取适当的措施。

模块中对节点状态机上的账户所有者进行签名检查的交易处理程序必须调用`createOutgoingPacket`。

```typescript
function createOutgoingPacket(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  destPort: string,
  destChannel: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: uint64,
  timeoutTimestamp: uint64) {
  // inspect the denomination to determine whether or not we are the source chain
  prefix = "{destPort}/{destChannel}"
  source = denomination.slice(0, len(prefix)) === prefix
  if source {
    // sender is source chain: escrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.sourceChannel]
    // escrow source tokens (assumed to fail if balance insufficient)
    bank.TransferCoins(sender, escrowAccount, denomination.slice(len(prefix)), amount)
  } else {
    // receiver is source chain, burn vouchers
    // construct receiving denomination, check correctness
    prefix = "{sourcePort}/{sourceChannel}"
    abortTransactionUnless(denomination.slice(0, len(prefix)) === prefix)
    // burn vouchers (assumed to fail if balance insufficient)
    bank.BurnCoins(sender, denomination, amount)
  }
  FungibleTokenPacketData data = FungibleTokenPacketData{denomination, amount, sender, receiver}
  handler.sendPacket(Packet{timeoutHeight, timeoutTimestamp, destPort, destChannel, sourcePort, sourceChannel, data}, getCapability("port"))
}
```

当路由模块收到一个数据包后调用`onRecvPacket`。

```typescript
function onRecvPacket(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  // inspect the denomination to determine whether or not we are the source chain
  prefix = "{packet.destPort}/{packet.destChannel}"
  source = denomination.slice(0, len(prefix)) === prefix
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}
  if source {
    // sender was source, mint vouchers to receiver (assumed to fail if balance insufficient)
    err = bank.MintCoins(data.receiver, data.denomination, data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
  } else {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // construct receiving denomination, check correctness
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    if (data.denomination.slice(0, len(prefix)) !== prefix)
      ack = FungibleTokenPacketAcknowledgement{false, "invalid denomination"}
    else {
      // unescrow tokens to receiver (assumed to fail if balance insufficient)
      err = bank.TransferCoins(escrowAccount, data.receiver, data.denomination.slice(len(prefix)), data.amount)
      if (err !== nil)
        ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
    }
  }
  return ack
}
```

当由路由模块发送的数据包被确认后，该模块调用`onAcknowledgePacket`。

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // if the transfer failed, refund the tokens
  if (!ack.success)
    refundTokens(packet)
}
```

当由路由模块发送的数据包超时（例如数据包没有被目标链接收到）后，路由模块调用`onTimeoutPacket`。

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
}
```

`refundTokens`会在两处被调用，失败时的`onAcknowledgePacket` 和`onTimeoutPacket`，用来退还托管的通证给原始发送者。

```typescript
function refundTokens(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  prefix = "{packet.destPort}/{packet.destChannel}"
  source = data.denomination.slice(0, len(prefix)) === prefix
  if source {
    // sender was source chain, unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // construct receiving denomination, check correctness
    // unescrow tokens back to sender
    bank.TransferCoins(escrowAccount, data.sender, data.denomination.slice(len(prefix)), data.amount)
  } else {
    // receiver was source chain, mint vouchers
    // construct receiving denomination, check correctness
    prefix = "{packet.sourcePort}/{packet.sourceChannel}"
    // we abort here because we couldn't have sent this packet
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // mint vouchers back to sender
    bank.MintCoins(data.sender, data.denomination, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // can't happen, only unordered channels allowed
}
```

#### 原理

##### 正确性

该实现保持了同质性和供应量不变。

同质性：如果通证已发送到目标链，则可以以相同面额和数量兑换回源链。

供应量：将供应重新定义为未锁定的通证。所有源链的发送量等于目标链的接受量。源链可以改变通证的供应量。

##### 多链注意事项

此规范不能直接处理“菱形问题”，在该问题中，用户将源自链 A 的通证发送到链 B，然后又发送给链 D，并希望通过 D-> C-> A 归还它，由于此时通证的供应量被认为是由链 B 控制（面额将为“ {portOnD} / {channelOnD} / {portOnB} / {channelOnB} / denom”），链 C 不能充当中介。尚不清楚该场景是否应按协议处理—可能只需要原路返回就可以了（如果在这两个途径上都有频繁的流动性和一定的结余，菱形路径将在大多数情况下适用）。较长的赎回路径引起的复杂性可能导致网络拓扑结构中出现中心链。

为了跟踪沿着各种路径在链网络中移动的所有面额，对于特定的链实现一个注册表将有助于跟踪每个面额的“全局”源链。最终用户服务提供商（例如钱包作者）可能希望集成这样的注册表，或保留自己的典范源链和人类可读名称的映射，以改善 UX。

#### 可选附录

- 每个本地链都可以选择保留一个查找表，以在状态中使用简短，用户友好的本地面额，在发送和接收数据包时，它们会与较长面额进行转换。
- 可能会对与哪些其他机器连接以及建立哪些通道施加其他限制。

## 向后兼容性

不适用。

## 向前兼容性

此初始标准在通道握手中使用版本“ ics20-1”。

该标准的未来版本可以在通道握手中使用其他版本，并安全的更改数据包数据格式和数据包处理程序的语义。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

2019年7月15 - 草案完成

2019年7月29 - 主要修订；整理

2019年8月25 - 主要修订；进一步整理

2020年2月3日-进行修订，以处理对成功和失败的确认

2020年2月24日-用来推断来源字段的修订，包括版本字符串

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
