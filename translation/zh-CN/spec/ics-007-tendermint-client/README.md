---
ics: 7
title: Tendermint 客户端
stage: 草案
category: IBC/TAO
kind: 实例化
implements: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-12-10
modified: 2019-12-19
---

## 概要

本规范文档描述了使用 Tendermint 共识的区块链客户端（验证算法）。

### 动机

使用 Tendermint 共识算法的各种状态机可能希望与其他使用 IBC 的状态机或单机进行交互。

### 定义

函数和术语如 [ICS 2](../ics-002-client-semantics) 中所定义。

`currentTimestamp`如 [ICS 24](../ics-024-host-requirements) 中所定义。

Tendermint 轻客户端使用 ICS 8 中定义的通用默克尔证明格式。

`hash`是一种通用的抗碰撞哈希函数，可以轻松的配置。

### 所需属性

该规范必须满足 ICS 2 中定义的客户端接口。

## 技术指标

该规范依赖于 [Tendermint 共识算法](https://github.com/tendermint/spec/blob/master/spec/consensus/consensus.md)和[轻客户端算法](https://github.com/tendermint/spec/blob/master/spec/consensus/light-client.md)的正确实例化。

### 客户端状态

Tendermint 客户端状态跟踪当前的验证人集合，信任期，解除绑定期，最新区块高度，最新时间戳（区块时间）以及可能的冻结区块高度。

```typescript
interface ClientState {
  validatorSet: List<Pair<Address, uint64>>
  trustingPeriod: uint64
  unbondingPeriod: uint64
  latestHeight: uint64
  latestTimestamp: uint64
  frozenHeight: Maybe<uint64>
}
```

### 共识状态

Tendermint 客户端会跟踪所有先前已验证的共识状态的时间戳（区块时间），验证人集和和承诺根（在取消绑定期之后可以将其清除，但不应该在之前清除）。

```typescript
interface ConsensusState {
  timestamp: uint64
  validatorSet: List<Pair<Address, uint64>>
  commitmentRoot: []byte
}
```

### 区块头

Tendermint 客户端头包括区块高度，时间戳，承诺根，完整的验证人集合以及提交该块的验证人的签名。

```typescript
interface Header {
  height: uint64
  timestamp: uint64
  commitmentRoot: []byte
  validatorSet: List<Pair<Address, uint64>>
  signatures: []Signature
}
```

### 证据

`Evidence`类型用于检测不良行为并冻结客户端-以防止进一步的数据包流。 Tendermint 客户端的`Evidence`包括两个相同高度并且轻客户端认为都是有效的区块头。

```typescript
interface Evidence {
  fromHeight: uint64
  h1: Header
  h2: Header
}
```

### 客户端初始化

Tendermint 客户初始化要求（主观选择的）最新的共识状态，包括完整的验证人集合。

```typescript
function initialise(
  consensusState: ConsensusState, validatorSet: List<Pair<Address, uint64>>,
  height: uint64, trustingPeriod: uint64, unbondingPeriod: uint64): ClientState {
  assert(trustingPeriod < unbondingPeriod)
    return ClientState{
      validatorSet,
      latestHeight: height,
      latestTimestamp: consensusState.timestamp,
      trustingPeriod,
      unbondingPeriod,
      pastHeaders: Map.singleton(latestHeight, consensusState)
    }
}
```

Tendermint 客户端的`latestClientHeight`函数返回最新存储的高度，该高度在每次验证了新的（较新的）区块头时都会更新。

```typescript
function latestClientHeight(clientState: ClientState): uint64 {
  return clientState.latestHeight
}
```

### 合法性判定式

Tendermint 客户端合法性检查使用 [Tendermint 规范中](https://github.com/tendermint/spec/blob/master/spec/consensus/light-client.md)描述的二分算法。如果提供的区块头有效，那么将更新客户端状态并将新验证的承诺写入存储。

```typescript
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    // assert trusting period has not yet passed
    assert(currentTimestamp() - clientState.latestTimestamp < clientState.trustingPeriod)
    // assert header timestamp is not in the future (& transitively that is not past the trusting period)
    assert(header.timestamp <= currentTimestamp())
    // assert header timestamp is past current timestamp
    assert(header.timestamp > clientState.latestTimestamp)
    // assert header height is newer than any we know
    assert(header.height > clientState.latestHeight)
    // call the `verify` function
    assert(verify(clientState.validatorSet, clientState.latestHeight, header))
    // update latest height
    clientState.latestHeight = header.height
    // create recorded consensus state, save it
    consensusState = ConsensusState{validatorSet, header.commitmentRoot, header.timestamp}
    set("clients/{identifier}/consensusStates/{header.height}", consensusState)
    // save the client
    set("clients/{identifier}", clientState)
}
```

### 不良行为判定式

Tendermint 客户端的不良行为检查决定于在相同高度的两个冲突区块头是否都会通过轻客户端的验证。

```typescript
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    // assert that the heights are the same
    assert(evidence.h1.height === evidence.h2.height)
    // assert that the commitments are different
    assert(evidence.h1.commitmentRoot !== evidence.h2.commitmentRoot)
    // fetch the previously verified commitment root & validator set
    consensusState = get("clients/{identifier}/consensusStates/{evidence.fromHeight}")
    // assert that the timestamp is not from more than an unbonding period ago
    assert(currentTimestamp() - consensusState.timestamp < clientState.unbondingPeriod)
    // check if the light client "would have been fooled"
    assert(
      verify(consensusState.validatorSet, evidence.fromHeight, evidence.h1) &&
      verify(consensusState.validatorSet, evidence.fromHeight, evidence.h2)
      )
    // set the frozen height
    clientState.frozenHeight = min(clientState.frozenHeight, evidence.h1.height) // which is same as h2.height
    // save the client
    set("clients/{identifier}", clientState)
}
```

### 状态验证函数

Tendermint 客户端状态验证函数对照先前已验证的承诺根检查默克尔证明。

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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided consensus state has been stored
    assert(root.verifyMembership(path, consensusState, proof))
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connections/{connectionIdentifier}")
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided connection end has been stored
    assert(root.verifyMembership(path, connectionEnd, proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided channel end has been stored
    assert(root.verifyMembership(path, channelEnd, proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided commitment has been stored
    assert(root.verifyMembership(path, hash(data), proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the provided acknowledgement has been stored
    assert(root.verifyMembership(path, hash(acknowledgement), proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that no acknowledgement has been stored
    assert(root.verifyNonMembership(path, proof))
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
    // check that the client is at a sufficient height
    assert(clientState.latestHeight >= height)
    // check that the client is unfrozen or frozen at a higher height
    assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
    // fetch the previously verified commitment root & verify membership
    root = get("clients/{identifier}/consensusStates/{height}")
    // verify that the nextSequenceRecv is as claimed
    assert(root.verifyMembership(path, nextSequenceRecv, proof))
}
```

### 属性和不变量

正确性保证和 Tendermint 轻客户端算法相同。

## 向后兼容性

不适用。

## 向前兼容性

不适用。更改客户端验证算法将需要新的客户端标准。

## 示例实现

还没有。

## 其他实现

目前没有。

## 历史

2019年12月10日-初始版本 2019年12月19日-最终初稿

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
