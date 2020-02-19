---
ics: 27
title: 链间账户
stage: 草案
category: IBC/TAO
requires: 25, 26
kind: 实例化
author: Tony Yun <yunjh1994@everett.zone>, Dogemos <josh@tendermint.com>
created: 2019-08-01
modified: 2019-12-02
---

## 概要

该标准文档指定了不同链之间 IBC 通道之上的帐户管理系统的数据包数据结构，状态机处理逻辑和编码详细信息。

### 动机

在以太坊上，有两种类型的账户：受私钥控制的外部拥有账户和受其合约代码（ [ref](https://github.com/ethereum/wiki/wiki/White-Paper) ）控制的合约账户。与以太坊的 CA（合约账户）相似，链间账户由另一条链管理，同时保留普通账户的所有功能（例如，质押，发送，投票等）。以太坊 CA 的合约逻辑是在以太坊的 EVM 中执行的，而链间账户由另一条链通过 IBC 进行管理，从而使账户所有者可以完全控制其行为。

### 定义

IBC 处理程序接口和 IBC 路由模块接口分别在 [ICS 25](../ics-025-handler-interface) 和 [ICS 26](../ics-026-routing-module) 中所定义。

### 所需属性

- 无需许可
- 故障容忍：链间帐户必须遵循其宿主链的规则，即使在对方链（管理帐户的链）出现拜占庭行为时也是如此
- 控制帐户的链必须按照链的逻辑异步处理结果。如果交易成功，则结果应为 0x0，如果交易失败，则结果应为 0x0 以外的错误代码。
- 发送和接收交易将在有序通道中进行处理，在该通道中，数据包将按照其发送的顺序传递。

## 技术指标

链间账户的实现是不对称的。这意味着每个链可以具有不同的方式来生成链间帐户并反序列化交易字节和它们可以执行的不同交易集。例如，使用 Cosmos SDK 的链将使用 Amino 对 tx 字节进行反序列化，但是如果对方链是以太坊上的智能合约，则它可能会通过 ABI 对 tx 字节进行反序列化，这是智能合约的最小序列化算法。 链间帐户规范定义了注册链间帐户和传输 tx 字节的一般方法。对方链负责反序列化和执行 tx 字节，并且发送链应事先知道对方链将如何处理 tx 字节。

每个链必须满足以下功能才能创建链间帐户：

- 新的链间帐户不得与现有帐户冲突。
- 每个链必须跟踪是哪个对方链创建了新的链间账户。

同样，每个链必须知道对方链如何序列化/反序列化交易字节，以便通过 IBC 发送交易。对方链必须通过验证交易签名人的权限来安全执行 IBC 交易。

在以下情况下，链必须拒绝交易并且不进行状态转换：

- IBC 交易无法反序列化。
- IBC 交易期望的是对方链创建跨链账户以外的签名者。

不限制你如何区分一个签名者是不是对方链的。但是最常见的方法是在注册链间帐户时记录帐户在状态中，并验证签名者是否是记录的链间帐户。

### 数据结构

每个链必须实现以下接口以支持链间帐户。 `IBCAccountModule`接口的`createOutgoingPacket`方法定义了创建特定类型的传出数据包的方式。类型指示应如何为主机链构建和序列化 IBC 帐户交易。通常，类型指示主机链的构建框架。 `generateAddress`定义了如何使用标识符和盐（salt）确定帐户地址的方法。建议使用盐生成地址，但不是必需的。如果该链不支持用确定性的方式来生成带有盐的地址，则可以以其自己的方式来生成。 `createAccount`使用生成的地址创建帐户。新的链间帐户不得与现有帐户冲突，并且链应跟踪是哪个对方链创建的新的链间帐户，以验证`authenticateTx`中交易签名人的权限。 `authenticateTx`验证交易并检查交易中的签名者是否具有正确的权限。成功通过身份认证后， `runTx`执行交易。

```typescript
type Tx = object

interface IBCAccountModule {
  createOutgoingPacket(chainType: Uint8Array, data: any)
  createAccount(address: Uint8Array)
  generateAddress(identifier: Identifier, salt: Uint8Array): Uint8Array
  deserialiseTx(txBytes: Uint8Array): Tx
  authenticateTx(tx: Tx): boolean
  runTx(tx: Tx): uint32
}
```

对方链使用`RegisterIBCAccountPacketData`注册帐户。使用通道标识符和盐可以确定性的定义链间帐户的地址。 `generateAccount`方法用于生成新的链间帐户的地址。建议通过`hash(identifier+salt)`生成地址，但是也可以使用其他方法。此函数必须通过标识符和盐生成唯一的确定性地址。

```typescript
interface RegisterIBCAccountPacketData {
  salt: Uint8Array
}
```

`RunTxPacketData`用于在链间帐户上执行交易。交易字节包含交易本身，并以适合于目标链的方式进行序列化。

```typescript
interface RunTxPacketData {
  txBytes: Uint8Array
}
```

`IBCAccountHandler`接口允许源链接收在链间帐户上执行交易的结果。

```typescript
interface InterchainTxHandler {
  onAccountCreated(identifier: Identifier, address: Address)
  onTxSucceeded(identifier: Identifier, txBytes: Uint8Array)
  onTxFailed(identifier: Identifier, txBytes: Uint8Array, errorCode: Uint8Array)
}
```

### 子协议

本文所述的子协议应在“链间帐户桥”模块中实现，并可以访问应用的路由和编解码器（解码器或解组器），和访问 IBC 路由模块。

### 端口和通道设置

创建模块时（可能是在初始化区块链本身时），必须调用一次`setup`函数，以绑定到适当的端口并创建托管地址（为模块拥有）。

```typescript
function setup() {
  relayerModule.bindPort("interchain-account", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onSendPacket,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
}
```

调用`setup`功能后，即可通过 IBC 路由模块在独立链上的链间帐户模块实例之间创建通道。

管理员（具有在主机状态机上创建连接和通道的权限）负责建立与其他状态机的连接，并创建与其他链上该模块（或支持此接口的另一个模块）的其他实例的通道。该规范仅定义了数据包处理语义，并以这样一种方式定义它们：模块本身无需担心在任何时间点可能存在或不存在哪些连接或通道。

### 路由模块回调

### 通道生命周期管理

当且仅当以下情况时，机器`A`和`B`接受另一台机器上任何模块的新通道：

- 另一个模块绑定到“链间帐户”端口。
- 正在创建的通道是有序的。
- 版本字符串为空。

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // only allow channels to "interchain-account" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "interchain-account")
  // version not used at present
  abortTransactionUnless(version === "")
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
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // version not used at present
  abortTransactionUnless(version === "")
  abortTransactionUnless(counterpartyVersion === "")
  // only allow channels to "interchain-account" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "interchain-account")
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // version not used at present
  abortTransactionUnless(version === "")
  // port has already been validated
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated
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

### 数据包中继

用简单的文字描述就是`A`和`B`之间，链 A 想要在链 B 上注册一个链间帐户并控制它。同样，也可以反过来。

```typescript
function onRecvPacket(packet: Packet): bytes {
  if (packet.data is RunTxPacketData) {
    const tx = deserialiseTx(packet.data.txBytes)
    abortTransactionUnless(authenticateTx(tx))
    return runTx(tx)
  }

  if (packet.data is RegisterIBCAccountPacketData) {
    RegisterIBCAccountPacketData data = packet.data
    identifier = "{packet/sourcePort}/{packet.sourceChannel}"
    const address = generateAddress(identifier, packet.salt)
    createAccount(address)
    // Return generated address.
    return address
  }

  return 0x
}
```

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  if (packet.data is RegisterIBCAccountPacketData)
    if (acknowledgement !== 0x) {
      identifier = "{packet/sourcePort}/{packet.sourceChannel}"
      onAccountCreated(identifier, acknowledgement)
    }
  if (packet.data is RunTxPacketData) {
    identifier = "{packet/destPort}/{packet.destChannel}"
    if (acknowledgement === 0x)
        onTxSucceeded(identifier: Identifier, packet.data.txBytes)
    else
        onTxFailed(identifier: Identifier, packet.data.txBytes, acknowledgement)
  }
}
```

```typescript
function onTimeoutPacket(packet: Packet) {
  // Receiving chain should handle this event as if the tx in packet has failed
  if (packet.data is RunTxPacketData) {
    identifier = "{packet/destPort}/{packet.destChannel}"
    // 0x99 error code means timeout.
    onTxFailed(identifier: Identifier, packet.data.txBytes, 0x99)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // nothing is necessary
}
```

## 向后兼容性

不适用。

## 向前兼容性

不适用。

## 示例实现

cosmos-sdk 的伪代码：https://github.com/everett-protocol/everett-hackathon/tree/master/x/interchain-account 以太坊上的链间账户的 POC：https://github.com/everett-protocol/ethereum-interchain-account

## 其他实现

（其他实现的链接或描述）

## 历史

2019年8月1日-讨论了概念

2019年9月24日-建议草案

2019年11月8日-重大修订

2019年12月2日-较小修订（在以太坊上添加更多具体描述并添加链间账户）

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
