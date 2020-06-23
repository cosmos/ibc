---
ics: 6
title: 单机客户端
stage: 草案
category: IBC/TAO
kind: 实例化
implements: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-09
modified: 2019-12-09
---

## 概要

本规范文档描述了具有单个可更新公钥的单机客户端（验证算法），该客户端实现了 [ICS 2](../ics-002-client-semantics) 接口。

### 动机

单机，可能是诸如手机，浏览器或笔记本电脑之类的设备，它们希望与使用 IBC 的其他机器和多副本帐本进行交互，并且可以通过统一的客户端接口来实现。

单机客户端大致类似于“隐式帐户”，可以用来代替账本上的“常规交易”，从而允许所有交易通过 IBC 的统一接口进行。

### 定义

函数和术语如 [ICS 2](../ics-002-client-semantics) 中所定义。

### 所需属性

该规范必须满足 [ICS 2](../ics-002-client-semantics) 中定义的客户端接口。

从概念上讲，我们假设有一个“全局的大签名表”（生成的签名是公开的）并相应的包含了重放保护。

## 技术指标

该规范包含 [ICS 2](../ics-002-client-semantics) 定义的所有函数的实现。

### 客户端状态

单机的`ClientState`就是简单的客户端是否被冻结。

```typescript
interface ClientState {
  frozen: boolean
  consensusState: ConsensusState
}
```

### 共识状态

单机的`ConsensusState`由当前的公钥和序号组成。

```typescript
interface ConsensusState {
  sequence: uint64
  publicKey: PublicKey
}
```

### 区块头

`Header`仅在机器希望更新公钥时才由单机提供。

```typescript
interface Header {
  sequence: uint64
  signature: Signature
  newPublicKey: PublicKey
}
```

### 证据

单机的不良行为的`Evidence`包括一个序号和该序号上不同消息的两个签名。

```typescript
interface SignatureAndData {
  sig: Signature
  data: []byte
}

interface Evidence {
  sequence: uint64
  signatureOne: SignatureAndData
  signatureTwo: SignatureAndData
}
```

### 客户端初始化

单机客户端`initialise`函数以初始共识状态启动一个未冻结的客户端。

```typescript
function initialise(consensusState: ConsensusState): ClientState {
  return {
    frozen: false,
    consensusState
  }
}
```

单机客户端`latestClientHeight`函数返回最新的序号。

```typescript
function latestClientHeight(clientState: ClientState): uint64 {
  return clientState.consensusState.sequence
}
```

### 合法性判定式

单机客户端的`checkValidityAndUpdateState`函数检查当前注册的公共密钥是否对新的公共密钥和正确的序号进行了签名。

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
  assert(header.sequence === clientState.consensusState.sequence)
  assert(checkSignature(header.newPublicKey, header.sequence, header.signature))
  clientState.consensusState.publicKey = header.newPublicKey
  clientState.consensusState.sequence++
}
```

### 不良行为判定式

任何当前公钥在不同消息上的重复签名都会冻结单机客户端。

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    h1 = evidence.h1
    h2 = evidence.h2
    pubkey = clientState.consensusState.publicKey
    assert(evidence.h1.signature.data !== evidence.h2.signature.data)
    assert(checkSignature(pubkey, evidence.sequence, evidence.h1.signature.sig))
    assert(checkSignature(pubkey, evidence.sequence, evidence.h2.signature.sig))
    clientState.frozen = true
}
```

### 状态验证函数

所有单机客户端状态验证函数都仅检查签名，该签名必须由单机提供。

```typescript
function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusState/{consensusStateHeight}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + consensusState
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connection/{connectionIdentifier}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + connectionEnd
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
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
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + channelEnd
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}

function verifyPacketData(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + data
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
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
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + acknowledgement
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
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
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
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
    abortTransactionUnless(!clientState.frozen)
    value = clientState.consensusState.sequence + path + nextSequenceRecv
    assert(checkSignature(clientState.consensusState.pubKey, value, proof))
    clientState.consensusState.sequence++
}
```

### 属性和不变量

实例化 [ICS 2](../ics-002-client-semantics) 中定义的接口。

## 向后兼容性

不适用。

## 向前兼容性

不适用。更改客户端验证算法将需要新的客户端标准。

## 示例实现

还没有。

## 其他实现

目前没有。

## 历史

2019年12月9日-初始版本 2019年12月17日-最终初稿

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
