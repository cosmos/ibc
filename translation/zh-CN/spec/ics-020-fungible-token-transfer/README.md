---
ics: 20
title: 同质通证转移
stage: 草案
category: IBC/APP
requires: 25, 26
kind: 实例化
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-07-15
modified: 2019-08-25
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
  source: boolean
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
  routingModule.bindPort("bank", ModuleCallbacks{
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
  // 只允许无序的通道
  abortTransactionUnless(order === UNORDERED)
  // 只允许对方链使用绑定"bank"端口的通道
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // 目前还未使用版本
  abortTransactionUnless(version === "")
  // 分配托管地址
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
  // 只允许无序的通道
  abortTransactionUnless(order === UNORDERED)
  // 目前还未使用版本
  abortTransactionUnless(version === "")
  abortTransactionUnless(counterpartyVersion === "")
  // 只允许对方链使用绑定"bank"端口的通道
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // 分配托管地址
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // 目前还未使用版本
  abortTransactionUnless(version === "")
  // 完成端口验证
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // 完成端口验证，接受通道确认
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // 无需任何操作
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // 无需任何操作
}
```

##### 数据包中继

用简单文字来描述就是，在 `A` 和`B`两个链间：

- 在源 zone 上，桥接模块会在发送链上托管现有的本地资产面额，并在接收链上生成凭证。
- 在目标 zone 上，桥接模块会在发送链上销毁本地凭证，并在接收链上解除对本地资产面额的托管。
- 当数据包超时时，本地资产将解除托管并退还给发送者，或将凭证发回给发送者。
- 无需数据确认。

模块中对节点状态机上的账户所有者进行签名检查的交易处理程序必须调用`createOutgoingPacket`。

```typescript
function createOutgoingPacket(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  source: boolean) {
  if source {
    // 发送者在源链上: 托管通证
    // 确定托管账户
    escrowAccount = channelEscrowAddresses[packet.sourceChannel]
    // 构造收款面额并做正确性检查
    prefix = "{packet/destPort}/{packet.destChannel}"
    abortTransactionUnless(denomination.slice(0, len(prefix)) === prefix)
    // 托管源链通证（如果余额不足则失败）
    bank.TransferCoins(sender, escrowAccount, denomination.slice(len(prefix)), amount)
  } else {
    // 如果接受者是源链上的账户则销毁付款凭单
    // 构造收款面额并做正确性检查
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(denomination.slice(0, len(prefix)) === prefix)
    // 销毁付款凭单（如果余额不足则失败）
    bank.BurnCoins(sender, denomination, amount)
  }
  FungibleTokenPacketData data = FungibleTokenPacketData{denomination, amount, sender, receiver, source}
  handler.sendPacket(packet)
}
```

当路由模块收到一个数据包后调用`onRecvPacket`。

```typescript
function onRecvPacket(packet: Packet): bytes {
  FungibleTokenPacketData data = packet.data
  if data.source {
    // 发送者是源链上的账户: 创建付款凭单
    // 构造收款面额并做正确性检查
    prefix = "{packet/destPort}/{packet.destChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // 为接受者创建付款凭单（如果余额不足则失败）
    bank.MintCoins(data.receiver, data.denomination, data.amount)
  } else {
    // 接收者是源链上的账户：解除托管通证
    // 获取托管账户
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // 构造收款面额并做正确性检查
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // 解除通证托管并返还给接收者（如果余额不足则失败）
    bank.TransferCoins(escrowAccount, data.receiver, data.denomination.slice(len(prefix)), data.amount)
  }
  return 0x
}
```

当由路由模块发送的数据包被确认后，该模块调用`onAcknowledgePacket`。

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // 可能永远不会被调用，因此是一个空操作
}
```

当由路由模块发送的数据包超时（例如数据包没有被目标链接收到）后，路由模块调用`onTimeoutPacket`。

```typescript
function onTimeoutPacket(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  if data.source {
    // 如果发送者是源链上的账户，解除通证托管
    // 获取托管账户
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // 构造收款面额并做正确性检查
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // 解除通证托管并返还给发送者
    bank.TransferCoins(escrowAccount, data.sender, data.denomination.slice(len(prefix)), data.amount)
  } else {
    // 如果接收者是源链上的账户，创建付款凭单
    // 构造收款面额并做正确性检查
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // 创建付款凭单返回给发送者
    bank.MintCoins(data.sender, data.denomination, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // 不会发生，仅允许无序通道
}
```

#### 原理

##### 正确性

该实现保持了同质性和供应量不变。

同质性：如果通证已发送到目标链，则可以以相同面额和数量兑换回源链。

供应量：将供应重新定义为未锁定的通证。所有源链的发送量等于目标链的接受量。源链可以改变通证的供应量。

##### 多链注意事项

还无法处理“菱形问题”，即用户将在链 A 上发行的通证跨链转移到链 B，然后又转移到链 D，并想通过 D -> C -> A 的路径将通证转移回链 A，由于此时通证的供应量被认为是由链 B 控制，链 C 无法作为中介。目前尚不确定该场景是否应该在协议内处理，可能只需原路返回即可（如果两条路径上都有频繁的流动性和结余，菱形路径会更有效）。长的赎回路径产生的复杂性会导致网络拓扑中中心链的出现。

#### 可选附录

- 每个本地链都可以选择保留一个查找表，以在状态中使用简短，用户友好的本地面额，在发送和接收数据包时，它们会与较长面额进行转换。
- 可能会对与哪些其他机器连接以及建立哪些通道施加其他限制。

## 向后兼容性

不适用。

## 向前兼容性

该标准的未来版本可能使用不同的通道创建版本。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

2019年7月15 - 草案完成

2019年7月29 - 主要修订；整理

2019年8月25 - 主要修订；进一步整理

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
