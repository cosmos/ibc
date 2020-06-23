---
ics: 24
title: 主机状态机要求
stage: 草案
category: IBC/TAO
kind: 接口
requires: 23
required-by: 2, 3, 4, 5, 18
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-08-25
---

## 概要

该规范定义了必须提供的最小接口集和满足运行区块链间链通信协议的状态机必须实现的属性。

### 动机

IBC 被设计为通用标准，将由各种区块链和状态机运行，并且必须明确定义主机的要求。

### 定义

### 所需属性

IBC 应该要求底层状态机提供尽可能简单的接口，以最大程度的简化正确的实现。

## 技术指标

### 模块系统

主机状态机必须支持一个模块系统，这样，独立的，可能相互不信任的代码包就可以安全的在同一帐本上执行，控制它们如何以及何时允许其他模块与其通信，并由“主模块”或执行环境识别这些代码。

IBC/TAO 规范定义了两个模块的实现：核心“ IBC 处理程序”模块和“ IBC 中继器”模块。 IBC/APP 规范还为特定的数据包处理应用程序逻辑定义了其他模块。 IBC 要求可以使用“主模块”或执行环境来授予主机状态机上的其他模块对 IBC 处理程序模块和/或 IBC 路由模块的访问权限，但不对处于同个状态机上的任何其他模块的功能或通信能力施加任何要求。

### 路径，标识符，分隔符

`Identifier`是一个字节字符串，用作状态存储的对象（例如连接，通道或轻客户端）的键。

标识符必须为非空（正整数长度）。

标识符只能由以下类别之一中的字符组成：

- 字母数字
- `.`, `_`, `+`, `-`, `#`
- `[`, `]`, `<`, `>`

`Path`是用作状态存储对象的键的字节串。路径必须仅包含标识符，常量字符串和分隔符`"/"` 。

标识符并不意图成为有价值的资源，以防止名字抢注，可能需要实现最小长度要求或伪随机生成，但本规范未施加特别限制。

分隔符`"/"`用于分隔和连接两个标识符或一个标识符和一个常量字节串。标识符不得包含`"/"`字符，以防止歧义。

在整个说明书中，大括号表示的变量插值，用作定义路径格式的简写，例如`client/{clientIdentifier}/consensusState` 。

除非另有说明，否则本规范中列出的所有标识符和所有字符串都必须编码为 ASCII。

### 键/值存储

主机状态机必须提供键/值存储接口，具有以标准方式运行的三个函数：

```typescript
type get = (path: Path) => Value | void
```

```typescript
type set = (path: Path, value: Value) => void
```

```typescript
type delete = (path: Path) => void
```

`Path`如上所述。 `Value`是特定数据结构的任意字节串编码。编码细节留给单独的 ICS。

这些函数必须仅能由 IBC 处理程序模块（在单独的标准中描述了其实现）使用，因此只有 IBC 处理程序模块才能`set`或`delete` `get`可以读取的路径。这可以实现为整个状态机中的一个大型的键/值存储的子存储（前缀键空间）。

主机状态机务必提供此接口的两个实例- 一个`provableStore`用于供其他链读取的存储和主机的本地存储`privateStore`，在这`get` ， `set`和`delete`可以被调用，例如`provableStore.set('some/path', 'value')` 。

对于`provableStore` ：

- 写入键/值存储的数据必须可以使用 [ICS 23](../ics-023-vector-commitments) 中定义的向量承诺从外部证明。
- 必须使用这些规范中提供的，如 proto3 文件中的典范数据结构编码。

对于`privateStore` ：

- 可以支持外部证明，但不是必须的-IBC 处理程序将永远不会向其写入需要证明的数据。
- 可以使用典范的 proto3 数据结构，但不是必须的-它可以使用应用程序环境首选的任何格式。

> 注意：任何提供这些方法和属性的键/值存储接口都足以满足 IBC 的要求。主机状态机可以使用路径和值映射来实现“代理存储”，这些映射不直接匹配通过存储接口设置和检索的路径和值对-路径可以分组在存储桶结构中，值可以分组存储在页结构中，这样就可以是用一个单独的承诺来证明，可以以某种双射的方式非连续的重新映射路径空间，等等—只要`get` ， `set`和`delete`行为符合预期，并且其他机器可以在可证明的存储中验证路径和值对的承诺证明（或它们的不存在）。如果适用，存储必须对外公开此映射，以便客户端（包括中继器）可以确定存储的布局以及如何构造证明。使用这种代理存储的机器的客户端也必须了解映射，因此它将需要新的客户端类型或参数化的客户端。

> 注意：此接口不需要任何特定的存储后端或后端数据布局。状态机可以选择使用根据其需求配置的存储后端，只要顶层的存储满足指定的接口并提供承诺证明即可。

### 路径空间

目前，IBC/TAO 为`provableStore`和`privateStore`建议以下路径前缀。

协议的未来版本中可能会使用未来的路径，因此可证明存储中的整个键空间必须为 IBC 处理程序保留。

可证明存储中使用的键可以安全的根据每个客户端类型而变化，只要在这里定义的键格式以及在机器实现中实际使用的定义之间存在双向映射 。

只要 IBC 处理程序对所需的特定键具有独占访问权，私有存储的某些部分就可以安全的用于其他目的。 只要在此定义的键格式和实际私有存储实现中格式之间存在双向映射，私有存储中使用的键就可以安全的变化。

请注意，下面列出的与客户端相关的路径反映了 [ICS 7](../ics-007-tendermint-client) 中定义的 Tendermint 客户端，对于其他客户端类型可能有所不同。

存储 | 路径格式 | 值格式 | 定义在
--- | --- | --- | ---
provableStore | "clients/{identifier}/clientType" | ClientType | [ICS 2](../ics-002-client-semantics)
privateStore | "clients/{identifier}/clientState" | ClientState | [ICS 2](../ics-007-tendermint-client)
provableStore | "clients/{identifier}/consensusState/{height}" | ConsensusState | [ICS 7](../ics-007-tendermint-client)
privateStore | "clients/{identifier}/connections | []Identifier | [ICS 3](../ics-003-connection-semantics)
provableStore | "connections/{identifier}" | ConnectionEnd | [ICS 3](../ics-003-connection-semantics)
privateStore | "ports/{identifier}" | CapabilityKey | [ICS 5](../ics-005-port-allocation)
provableStore | "channelEnds/ports/{identifier}/channels/{identifier}" | ChannelEnd | [ICS 4](../ics-004-channel-and-packet-semantics)
provableStore | "seqSends/ports/{identifier}/channels/{identifier}/nextSequenceSend" | uint64 | [ICS 4](../ics-004-channel-and-packet-semantics)
provableStore | "seqRecvs/ports/{identifier}/channels/{identifier}/nextSequenceRecv" | uint64 | [ICS 4](../ics-004-channel-and-packet-semantics)
provableStore | "seqAcks/ports/{identifier}/channels/{identifier}/nextSequenceAck" | uint64 | [ICS 4](../ics-004-channel-and-packet-semantics)
provableStore | "commitments/ports/{identifier}/channels/{identifier}/packets/{sequence}" | bytes | [ICS 4](../ics-004-channel-and-packet-semantics)
provableStore | "acks/ports/{identifier}/channels/{identifier}/acknowledgements/{sequence}" | bytes | [ICS 4](../ics-004-channel-and-packet-semantics)

### 模块布局

以空间表示，模块的布局及其在主机状态机上包含的规范如下所示（Aardvark，Betazoid 和 Cephalopod 是任意模块）：

```
+----------------------------------------------------------------------------------+
|                                                                                  |
| Host State Machine                                                               |
|                                                                                  |
| +-------------------+       +--------------------+      +----------------------+ |
| | Module Aardvark   | <-->  | IBC Routing Module |      | IBC Handler Module   | |
| +-------------------+       |                    |      |                      | |
|                             | Implements ICS 26. |      | Implements ICS 2, 3, | |
|                             |                    |      | 4, 5 internally.     | |
| +-------------------+       |                    |      |                      | |
| | Module Betazoid   | <-->  |                    | -->  | Exposes interface    | |
| +-------------------+       |                    |      | defined in ICS 25.   | |
|                             |                    |      |                      | |
| +-------------------+       |                    |      |                      | |
| | Module Cephalopod | <-->  |                    |      |                      | |
| +-------------------+       +--------------------+      +----------------------+ |
|                                                                                  |
+----------------------------------------------------------------------------------+
```

### 共识状态检视

主机状态机必须提供使用`getCurrentHeight`检视其当前高度的功能：

```
type getCurrentHeight = () => uint64
```

主机状态机必须使用典范的二进制序列化定义满足 [ICS 2](../ics-002-client-semantics) 要求的一个唯一`ConsensusState`类型。

主机状态机必须提供使用`getConsensusState`检视其共识状态的能力：

```typescript
type getConsensusState = (height: uint64) => ConsensusState
```

`getConsensusState`必须至少返回连续`n`个最近的高度的共识状态，其中`n`对于主机状态机是恒定的。大于`n`高度可能会被安全的删除掉（之后对这些高度的调用会失败）。

主机状态机必须提供使用`getStoredRecentConsensusStateCount`检视最近`n`个共识状态的能力 ：

```typescript
type getStoredRecentConsensusStateCount = () => uint64
```

### 承诺路径检视

主机链必须提供通过`getCommitmentPrefix`检视其承诺路径的能力：

```typescript
type getCommitmentPrefix = () => CommitmentPrefix
```

结果`CommitmentPrefix`是主机状态机的键值存储使用的前缀。 使用主机状态机的`CommitmentRoot root`和`CommitmentState state` ，必须保留以下属性：

```typescript
if provableStore.get(path) === value {
  prefixedPath = applyPrefix(getCommitmentPrefix(), path)
  if value !== nil {
    proof = createMembershipProof(state, prefixedPath, value)
    assert(verifyMembership(root, proof, prefixedPath, value))
  } else {
    proof = createNonMembershipProof(state, prefixedPath)
    assert(verifyNonMembership(root, proof, prefixedPath))
  }
}
```

对于主机状态机， `getCommitmentPrefix`的返回值必须是恒定的。

### 时间戳访问

主机链必须提供当前的 Unix 时间戳，可通过`currentTimestamp()`访问：

```typescript
type currentTimestamp = () => uint64
```

为了在超时机制中安全使用时间戳，后续区块头中的时间戳必须不能是递减的。

### 端口系统

主机状态机必须实现一个端口系统，其中 IBC 处理程序可以允许主机状态机中的不同模块绑定到唯一命名的端口。端口使用`Identifier`标示 。

主机状态机必须实现与 IBC 处理程序的权限交互，以便：

- 模块绑定到端口后，其他模块将无法使用该端口，直到该模块释放它
- 单个模块可以绑定到多个端口
- 端口的分配是先到先得的，已知模块的“预留”端口可以在状态机第一次启动的时候绑定。

可以通过每个端口的唯一引用（对象能力）（例如 Cosmos SDK），源身份验证（例如以太坊）或某种其他访问控制方法（在任何情况下，由主机状态机实施）来实现此许可。 详细信息，请参见 [ICS 5](../ics-005-port-allocation) 。

希望利用特定 IBC 特性的模块可以实现某些处理程序功能，例如，向和另一个状态机上的相关模块的通道握手过程添加其他逻辑。

### 数据报提交

实现路由模块的主机状态机可以定义一个`submitDatagram`函数，以将[数据报](../../ibc/1_IBC_TERMINOLOGY.md) （将包含在交易中）直接提交给路由模块（在 [ICS 26](../ics-026-routing-module) 中定义）：

```typescript
type submitDatagram = (datagram: Datagram) => void
```

`submitDatagram`允许中继器进程将 IBC 数据报直接提交到主机状态机上的路由模块。主机状态机可能要求提交数据报的中继器进程有一个帐户来支付交易费用，在更大的交易结构中对数据报进行签名，等等— `submitDatagram`必须定义并构造任何打包所需的结构。

### 异常系统

主机状态机务必支持异常系统，借以使交易可以中止执行并回滚以前进行的状态更改（包括同一交易中发生的其他模块中的状态更改），但不包括耗费的 gas 和费用，和违反系统不变量导致状态机挂起的行为。

这个异常系统必须暴露两个函数： `abortTransactionUnless`和`abortSystemUnless` ，其中前者回滚交易，后者使状态机挂起。

```typescript
type abortTransactionUnless = (bool) => void
```

如果传递给`abortTransactionUnless`的布尔值为`true` ，则主机状态机无需执行任何操作。如果传递给`abortTransactionUnless`的布尔值为`false` ，则主机状态机务必中止交易并回滚任何之前进行的状态更改，但不包括消耗的 gas 和费用。

```typescript
type abortSystemUnless = (bool) => void
```

如果传递给`abortSystemUnless`的布尔值为`true` ，则主机状态机无需执行任何操作。如果传递给`abortSystemUnless`的布尔值为`false` ，则主机状态机务必挂起。

### 数据可用性

为了达到发送或超时的安全（deliver-or-timeout safety）保证，主机状态机务必具有最终的数据可用性，以便中继器最终可以获取状态中的任何键/值对。而对于仅一次安全（exactly-once safety），不需要数据可用性。

对于数据包中继的活性，主机状态机必须具有交易活性（并因此必须具有共识活性），以便在一个高度范围内确认进入的交易（具体就是，小于分配给数据包的超时高度）。

IBC 数据包数据，以及未直接存储在状态向量中但中继器依赖的其他数据，必须可供中继器进程使用并高效地计算。

具有特定共识算法的轻客户端可能具有不同和/或更严格的数据可用性要求。

### 事件日志系统

主机状态机必须提供一个事件日志系统，借此可以在交易执行过程中记录任意数据，这些数据可以存储，索引并随后由执行状态机的进程查询。中继器利用这些事件日志读取 IBC 数据包数据和超时，这些数据和超时未直接存储在链上状态中（因为链上存储被认为是昂贵的），而是提交简洁的加密承诺（仅存储该承诺）。

该系统期望具有至少一个函数用于发出日志条目，和一个函数用于查询过去的日志，大概如下。

状态机可以在交易执行期间调用函数`emitLogEntry`来写入日志条目：

```typescript
type emitLogEntry = (topic: string, data: []byte) => void
```

`queryByTopic`函数可以被外部进程（例如中继器）调用，以检索在给定高度执行的交易写入的与查询主题关联的所有日志条目。

```typescript
type queryByTopic = (height: uint64, topic: string) => []byte[]
```

也可以支持更复杂的查询功能，并且可以允许更高效的中继器进程查询，但不是必需的。

## 向后兼容性

不适用。

## 向前兼容性

键/值存储功能和共识状态类型在单个主机状态机的操作期间不太可能更改。

因为中继器应该能够更新其进程，所以`submitDatagram`会随着时间而变化。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

2019年4月29日-初稿

2019年5月11日-将“ RootOfTrust”重命名为“ ConsensusState”

2019年6月25日-使用“端口”代替模块名称

2019年8月18日-修订模块系统，定义

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
