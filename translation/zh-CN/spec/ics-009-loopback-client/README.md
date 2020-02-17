---
ics: 9
title: 回环客户端
stage: 草案
category: IBC/TAO
kind: 实例化
author: Christopher Goes <cwgoes@tendermint.com>
created: 2020-01-17
modified: 2020-01-17
requires: 2
implements: 2
---

## 概要

本规范描述了一种回环客户端，该客户端旨在通过 IBC 接口与同一帐本中存在的模块进行交互。

### 动机

如果调用模块不了解目标模块的确切位置，并且希望使用统一的 IBC 消息传递接口（类似于 TCP/IP 中的 `127.0.0.1` ），则回环客户端可能很有用。

### 定义

函数和术语如 [ICS 2](../ics-002-client-semantics) 中所定义。

### 所需属性

应保留预期的客户端语义，而且回环抽象的成本应可忽略不计。

## 技术指标

### 数据结构

回环客户端不需要客户端状态，共识状态，区块头或证据数据结构。

```typescript
type ClientState object

type ConsensusState object

type Header object

type Evidence object
```

### 客户端初始化

回环客户端不需要初始化。将返回一个空状态。

```typescript
function initialise(): ClientState {
  return {}
}
```

### 合法性判定式

在回环客户端中，无需进行合法性检查；该函数永远不应该被调用。

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    assert(false)
}
```

### 不良行为判定式

在回环客户端中无需进行任何不良行为检查；该函数永远不应该被调用。

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    return
}
```

### 状态验证函数

回环客户端状态验证函数仅读取本地状态。请注意，他们将需要（只读）访问客户端前缀之外的键。

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "consensusStates/{clientIdentifier}")
    assert(get(path) === consensusState)
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connection/{connectionIdentifier}")
    assert(get(path) === connectionEnd)
}

function verifyChannelState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}")
    assert(get(path) === channelEnd)
}

function verifyPacketCommitment(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  commitment: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
    assert(get(path) === commitment)
}

function verifyPacketAcknowledgement(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    assert(get(path) === acknowledgement)
}

function verifyPacketAcknowledgementAbsence(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    assert(get(path) === nil)
}

function verifyNextSequenceRecv(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/nextSequenceRecv")
    assert(get(path) === nextSequenceRecv)
}
```

### 属性和不变量

语义上类似一个本地帐本的远程客户端。

## 向后兼容性

不适用。

## 向前兼容性

不适用。更改客户端算法将需要新的客户端标准。

## 示例实现

即将到来。

## 其他实现

目前没有。

## 历史

2020-01-17-初始版本

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
