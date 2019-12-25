---
ics: '3'
title: 连接语义
stage: 草案
category: IBC / TAO
kind: 实例化
requires: 2、24
required-by: 4、25
author: Christopher Gos <cwgoes@tendermint.com>，Juwoon Yun <joon@tendermint.com>
created: '2019-03-07'
modified: '2019-08-25'
---

## 概要

这个标准文档对 IBC *连接*的抽象进行描述：两条独立链上的两个有状态的对象（ *连接端* ），彼此与另一条链上的轻客户端关联，并共同来促进跨链子状态的验证和数据包的关联（通过通道）。描述了用于在两条链上安全地建立客户端的协议。

### 动机

核心 IBC 协议对数据包提供了*授权*和*排序*语义：一旦数据包分别已经在发送链上被有且仅有一次地按特定的顺序提交（并根据执行的交易的状态，例如通证托管），则可以确保这些数据包会以相同的顺序有且只有一次被递送到接收链。本标准中的*连接*抽象与 [ICS 2](../ics-002-client-semantics) 中定义的*客户端*抽象一同定义了 IBC 的*授权*语义。排序语义在 [ICS 4](../ics-004-channel-and-packet-semantics) 中进行了定义。

### 定义

客户端相关的类型和函数被定义在 [ICS 2](../ics-002-client-semantics) 中。

客户端相关的类型和函数被定义在 [ICS 23](../ics-023-vector-commitments) 中。

标识符和其他状态机主机的要求如 {a0}ICS 24{/a0} 所示。标识符不一定要是人类可读的名称（有可能地，不应阻止对标识符的抢注或争夺）。

开放式握手协议允许每个链验证用于引用另一个链上的连接的标识符，从而使每个链上的模块可以推出另一个链上的引用。

本规范中提到的*参与者*是能够执行数据报的实体，该数据报需要为计算/存储付费（通过 gas 或类似的机制），否则是不被信任的。 可能的参与者包括：

- 最终用户使用帐户密钥签名
- 自主执行或响应另一笔交易的链上智能合约
- 响应其他事务或按计划方式运行的链上模块

### 所需属性

- 区块链实现应该安全地允许不受信的参与者建立或更新连接。

#### 连接建立前阶段

在建立连接之前：

- 不能再使用IBC子协议，因为无法验证跨链子状态。
- 发起方（创建连接方）必须能够为要连接的链和连接的链指定初始共识状态（隐式地，例如通过发送交易）。

#### 握手期间

一旦握手协商开始后：

- 只有相关的握手数据报才可以按顺序被执行。
- 第三条链不可能伪装成正在发生握手的两条链链中的一条

#### 完成握手后阶段

一旦握手协商完成：

- 已经在两条链上建立的连接对象包括由初始参与者指定的共识状态。
- 通过重放数据报，无法在其他链上恶意地建立其他的连接对象。

## 技术指标

### 数据结构

此 ICS 定义了`ConnectionState`和`ConnectionEnd`类型：

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
}
```

```typescript
interface ConnectionEnd {
  state: ConnectionState
  counterpartyConnectionIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string | []string
}
```

- `state`字段描述连接端的当前状态。
- `counterpartyConnectionIdentifier`字段标识了与这个连接相关的对方链的连接端。
- `clientIdentifier`字段标识了与这个连接相关的对方链的连接端。
- `counterpartyClientIdentifier`字段标识与此连接关联的客户端。
- `version`字段是不透明的字符串，可用于确定使用此连接的通道或数据包的编码或协议。

### 储存路径

连接路径存储在唯一标识符下。

```typescript
function connectionPath(id: Identifier): Path {
    return "connections/{id}"
}
```

从客户端到一组连接（用于使用客户端查找所有连接）的反向映射存储在每个客户端的唯一前缀下：

```typescript
function clientConnectionsPath(clientIdentifier: Identifier): Path {
    return "clients/{clientIdentifier}/connections"
}
```

### 辅助函数

`addConnectionToClient`用于将连接标识符添加到与客户端关联的连接集合。

```typescript
function addConnectionToClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier))
    conns.add(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier), conns)
}
```

`removeConnectionFromClient`用于从与客户端关联的连接集合中删除某个连接标识符。

```typescript
function removeConnectionFromClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier))
    conns.remove(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier), conns)
}
```

辅助函数由连接所定义，以将与连接关联的`CommitmentPrefix`传递给客户端提供的验证函数。 在规范的其他部分，这些功能必须用于内省其他链的状态，而不是直接在客户端上调用验证功能。

```typescript
function verifyClientConsensusState(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyClientConsensusState(connection, height, connection.counterpartyPrefix, proof, clientIdentifier, consensusState)
}

function verifyConnectionState(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyConnectionState(connection, height, connection.counterpartyPrefix, proof, connectionIdentifier, connectionEnd)
}

function verifyChannelState(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyChannelState(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, channelEnd)
}

function verifyPacketCommitment(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  commitment: bytes) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketCommitment(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, commitment)
}

function verifyPacketAcknowledgement(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketAcknowledgement(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, acknowledgement)
}

function verifyPacketAcknowledgementAbsence(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketAcknowledgementAbsence(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier)
}

function verifyNextSequenceRecv(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyNextSequenceRecv(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, nextSequenceRecv)
}
```

### 子协议

本 ICS 定义了建立握手子协议。一旦握手建立，连接将不能被关闭，标识符也无法被重新分配（这防止了数据包重放或者授权混乱）。

数据头追踪和不良行为检测在 [ICS 2](../ics-002-client-semantics) 中被定义。

![State Machine Diagram](../../../../spec/ics-003-connection-semantics/state.png)

#### 标识符验证

连接存储在唯一的`Identifier`前缀下。 
可以提供验证函数`validateConnectionIdentifier`。

```typescript
type validateConnectionIdentifier = (id: Identifier) => boolean
```

如果未提供，默认的`validateConnectionIdentifier`函数将始终返回true。

#### 版本控制

在握手过程中，连接的两端需要保持连接相关的版本字节串一致。目前，此版本字节串的内容对于 IBC 核心协议是不透明的。将来，它可能被用于指示哪些类型的通道可以使用特定的连接，或者指示信道相关的数据报将使用哪种编码格式。目前，主机状态机可以利用版本数据来协商与 IBC 之上的自定义逻辑有关的编码、优先级或特定与连接的元数据。

主机状态机还可以安全地忽略版本数据或指定一个空字符串。

该标准的一个实现必须定义一个函数`getCompatibleVersions` ，该函数返回它支持的版本列表，按优先级降序排列。

```typescript
type getCompatibleVersions = () => []string
```

实现必须定义一个函数 `pickVersion` 来从合约双方提议的版本列表中选择一个版本。

```typescript
type pickVersion = ([]string) => string
```

#### 建立握手

建立握手子协议用于初始化两条链彼此的共识状态。

建立握手定义了 4 种数据报： *ConnOpenInit* ， *ConnOpenTry* ， *ConnOpenAck*和*ConnOpenConfirm* 。

一个正确的协议执行流程如下：（注意所有的请求都是按照 ICS 25 来制定的）

发起人 | 数据报 | 作用链 | 先前状态（A，B） | 连接建立后状态（A，B）
--- | --- | --- | --- | ---
参与者 | `ConnOpenInit` | A | (none, none) | （INIT，none）
中继器 | `ConnOpenTry` | B | （INIT，none） | （INIT，TRYOPEN）
中继器 | `ConnOpenAck` | A | （INIT，TRYOPEN） | (OPEN, TRYOPEN)
中继器 | `ConnOpenConfirm` | B | (OPEN, TRYOPEN) | (OPEN, OPEN)

在实现子协议的两个链之间的开放握手结束时，具有以下属性：

- 每条链都具有如同发起方所指定的对方链正确共识状态。
- 每条链都知道且认同另一链上的标识符。

该子协议不需要经过授权，以及反垃圾信息模块化处理。

*ConnOpenInit* 初始化链 A 上的连接尝试。

```typescript
function connOpenInit(
  identifier: Identifier,
  desiredCounterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  clientIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier) {
    abortTransactionUnless(validateConnectionIdentifier(identifier))
    abortTransactionUnless(provableStore.get(connectionPath(identifier)) == null)
    state = INIT
    connection = ConnectionEnd{state, desiredCounterpartyConnectionIdentifier, counterpartyPrefix,
      clientIdentifier, counterpartyClientIdentifier, getCompatibleVersions()}
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenTry* 将链 A 到链 B 的连接尝试通知转发（此代码在链 B 上执行）。

```typescript
function connOpenTry(
  desiredIdentifier: Identifier,
  counterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  counterpartyClientIdentifier: Identifier,
  clientIdentifier: Identifier,
  counterpartyVersions: string[],
  proofInit: CommitmentProof,
  proofConsensus: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(validateConnectionIdentifier(desiredIdentifier))
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{INIT, desiredIdentifier, getCommitmentPrefix(), counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersions}
    version = pickVersion(counterpartyVersions)
    connection = ConnectionEnd{state, counterpartyConnectionIdentifier, counterpartyPrefix,
                               clientIdentifier, counterpartyClientIdentifier, version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofInit, counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(proofHeight, proofConsensus, counterpartyClientIdentifier, expectedConsensusState))
    previous = provableStore.get(connectionPath(desiredIdentifier))
    abortTransactionUnless(
      (previous === null) ||
      (previous.state === INIT &&
        previous.counterpartyConnectionIdentifier === counterpartyConnectionIdentifier &&
        previous.counterpartyPrefix === counterpartyPrefix &&
        previous.clientIdentifier === clientIdentifier &&
        previous.counterpartyClientIdentifier === counterpartyClientIdentifier &&
        previous.version === version))
    identifier = desiredIdentifier
    state = TRYOPEN
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenAck* 对从链 B 返回链 A 的连接打开尝试的确认消息进行中继（此代码在链A上执行）。

```typescript
function connOpenAck(
  identifier: Identifier,
  version: string,
  proofTry: CommitmentProof,
  proofConsensus: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === INIT || connection.state === TRYOPEN)
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{TRYOPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier, connection.clientIdentifier,
                             version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofTry, connection.counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(proofHeight, proofConsensus, connection.counterpartyClientIdentifier, expectedConsensusState))
    connection.state = OPEN
    abortTransactionUnless(getCompatibleVersions().indexOf(version) !== -1)
    connection.version = version
    provableStore.set(connectionPath(identifier), connection)
}
```

*ConnOpenConfirm* 确认链 A 与链 B 的连接的建立，然后在两个链上打开连接（此代码在链 B 上执行）。

```typescript
function connOpenConfirm(
  identifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === TRYOPEN)
    expected = ConnectionEnd{OPEN, identifier, getCommitmentPrefix(), connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofAck, connection.counterpartyConnectionIdentifier, expected))
    connection.state = OPEN
    provableStore.set(connectionPath(identifier), connection)
}
```

#### 查询方式

可以使用`queryConnection`通过标识符来查询连接。

```typescript
function queryConnection(id: Identifier): ConnectionEnd | void {
    return provableStore.get(connectionPath(id))
}
```

可以使用`queryClientConnections`和客户端标识符来查询与特定客户端关联的连接。

```typescript
function queryClientConnections(id: Identifier): Set<Identifier> {
    return privateStore.get(clientConnectionsPath(id))
}
```

### 属性和不变性

- 连接标识符是“先到先得”的：一旦连接被商定，两个链之间就会存在一对唯一的标识符。
- 连接握手不能被另一条链的 IBC 处理程序作为中间人来进行干预。

## 向后兼容性

不适用。

## 向前兼容性

此 ICS 的未来版本将在开放式握手中包括版本协商。建立连接并协商版本后，可以根据 ICS 6 协商将来的版本更新。

只能在建立连接时选择的共识协议定义的`updateConsensusState`函数允许的情况下更新共识状态。

## 示例实现

即将发布。

## 其他实现

即将发布。

## 历史

本文档的某些部分受[以前的 IBC 规范](https://github.com/cosmos/cosmos-sdk/tree/master/docs/spec/ibc)的启发。

2019年3月29日-提交初稿

2019年5月17日-草稿定稿

2019年7月29日-修订版本以跟踪与客户端关联的连接集

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
