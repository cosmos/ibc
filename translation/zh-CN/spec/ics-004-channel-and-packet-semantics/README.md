---
ics: 4
title: 通道和数据包语义
stage: 草案
category: IBC/TAO
kind: 实例化
requires: 2, 3, 5, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## 概要

“通道”抽象为区块链间通信协议提供消息传递语义，分为三类：排序、有且仅有一次传递和模块许可。通道充当数据包在一条链上的模块与另一条链上的模块之间传递的通道，从而确保数据包仅被执行一次，并按照其发送顺序进行传递（如有必要），并仅传递给拥有目标链上通道的另一端的相应的模块。每个通道都与一个特定的连接关联，并且一个连接可以具有任意数量的关联通道，从而允许使用公共标识符，并利用连接和轻客户端在所有通道上分摊区块头验证的成本。

通道不关心其中传递的内容。发送和接收 IBC 数据包的模块决定如何构造数据包数据，以及如何对传入的数据包数据进行操作，并且必须利用其自身的应用程序逻辑来根据数据包中的数据来确定要应用的状态转换。

### 动机

区块链间通信协议使用跨链消息传递模型。 外部中继器进程将 IBC *数据包*从一条链中继到另一条链。链`A`和链`B`独立的确认新的块，并且从一个链到另一个链的数据包可能会被任意延迟、审查或重新排序。数据包对于中继器是可见的，并且可以被任何中继器进程读取，然后被提交给任何其他链。

> IBC 协议必须保证顺序（对于有序通道）和有且仅有一次传递，以允许应用程序探讨两条链上已连接模块的组合状态。例如，一个应用程序可能希望允许单个通证化的资产在多个区块链之间转移并保留在多个区块链上，同时保留同质化和供应量。当特定的 IBC 数据包提交到链`B`时，应用程序可以在链`B`上铸造资产凭据，并要求链`A`将等额的资产托管在链`A`上，直到以后以相反的 IBC 数据包将凭证兑换回链`A`为止。这种顺序保证配合正确的应用逻辑，可以确保两个链上的资产总量不变，并且在链`B`上铸造的任何资产凭证都可以之后兑换回链`A`上。

为了向应用层提供所需的排序、有且只有一次传递和模块许可语义，区块链间通信协议必须实现一种抽象以强制执行这些语义——通道就是这种抽象。

### 定义

`ConsensusState` 在 [ICS 2](../ics-002-client-semantics) 中被定义.

`Connection` 在 [ICS 3](../ics-003-connection-semantics) 中被定义.

`Port`和`authenticate`在 [ICS 5](../ics-005-port-allocation) 中被定义。

`hash`是一种通用的抗碰撞哈希函数，其细节必须由使用通道的模块商定。 `hash`在不同的链可以有不同的定义。

`Identifier` ， `get` ， `set` ， `delete` ， `getCurrentHeight`和模块系统相关的原语在 [ICS 24](../ics-024-host-requirements) 中被定义。

*通道*是用于在单独的区块链上的特定模块之间进行有且仅有一次数据包传递的管道，该模块至少具备数据包发送端和数据包接收端。

*双向*通道是数据包可以在两个方向上流动的通道：从`A`到`B`和从`B`到`A`

*单向*通道是指数据包只能沿一个方向流动的通道：从`A`到`B` （或从`B`到`A` ，命名的顺序是任意的）。

*有序*通道是指完全按照发送顺序传送数据包的通道。

*无序*通道是指可以以任何顺序传送数据包的通道，该顺序可能与数据包的发送顺序不同。

```typescript
enum ChannelOrder {
  ORDERED,
  UNORDERED,
}
```

方向和顺序是无关的，因此可以说双向无序通道，单向有序通道等。

所有通道均提供有且仅有一次的数据包传送，这意味着在通道的一端发送的数据包最终将不多于且不少于一次地传送到另一端。

该规范仅涉及*双向*通道。*单向*通道可以使用几乎完全相同的协议，并将在以后的ICS中进行概述。

通道端是一条链上存储通道元数据的数据结构：

```typescript
interface ChannelEnd {
  state: ChannelState
  ordering: ChannelOrder
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  connectionHops: [Identifier]
  version: string
}
```

- `state`是通道端的当前状态。
- `ordering`字段指示通道是有序的还是无序的。
- `counterpartyPortIdentifier`标识通道另一端的对应链上的端口号。
- `counterpartyChannelIdentifier`标识对应链的通道端。
- `nextSequenceSend`是单独存储的，追踪下一个要发送的数据包的序号。
- `nextSequenceRecv`是单独存储的，追踪要接收的下一个数据包的序号。
- `connectionHops`按顺序的存储在此通道上发送的数据包将途径的连接标识符列表。目前，此列表的长度必须为 1。将来可能会支持多跳通道。
- `version`字符串存储一个不透明的通道版本号，该版本号在握手期间已达成共识。这可以确定模块级别的配置，例如通道使用哪种数据包编码。核心 IBC 协议不会使用该版本号。

通道端具有以下*状态* ：

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  CLOSED,
}
```

- 处于`INIT`状态的通道端，表示刚刚开始了握手的建立。
- 处于`TRYOPEN`状态的通道端表示已确认对方链的握手。
- 处于`OPEN`状态的通道端，表示已完成握手，并为发送和接收数据包作好了准备。
- 处于`CLOSED`状态的通道端，表示通道已关闭，不能再用于发送或接收数据包。

区块链间通信协议中的`Packet`是如下定义的特定接口：

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: uint64
  sourcePort: Identifier
  sourceChannel: Identifier
  destPort: Identifier
  destChannel: Identifier
  data: bytes
}
```

- `sequence`对应于发送和接收的顺序，其中序号靠前的数据包必须比序号靠后的数据包先发送或接收。
- `timeoutHeight`指示目标链上的一个共识高度，此高度后不再处理数据包，而是计为已超时。
- `sourcePort`标识发送链上的端口。
- `sourceChannel`标识发送链上的通道端。
- `destPort`标识接收链上的端口。
- `destChannel`标识接收链上的通道端。
- `data`是不透明的值，可以由关联模块的应用程序逻辑定义。

请注意，`Packet`永远不会直接序列化。而是在某些函数调用中使用的中间结构，可能需要由调用 IBC 处理程序的模块来创建或处理该中间结构。

`OpaquePacket`是一个数据包，但是被主机状态机掩盖为一种模糊的数据类型，因此，除了将其传递给 IBC 处理程序之外，模块无法对其进行任何操作。IBC 处理程序可以将`Packet`转换为`OpaquePacket` ，或反过来。

```typescript
type OpaquePacket = object
```

### 所需属性

#### 效率

- 数据包传输和确认的速度应仅受底层链速度的限制。证明应尽可能是批量化的。

#### 有且仅有一次传递

- 在通道的一端发送的 IBC 数据包应仅一次的传递到另一端。
- 对于有且仅有一次安全性，不需要网络同步假设。如果其中一条链或两条链都挂起了，则数据包最多传递不超过一次，并且一旦链恢复，数据包就应该能够再次流转。

#### 定购

- 在有序通道上，应按相同的顺序发送和接收数据包：如果数据包*x*在链`A`通道端的数据包*y*之前发送，则数据包*x*必须在相应的链`B`通道端的数据包*y*之前收到。
- 在无序通道上，可以以任何顺序发送和接收数据包。像有序数据包一样，无序数据包具有单独的根据目标链高度指定的超时高度。

#### 许可

- 通道应该在握手期间被通道的两端都允许，并且此后不可变更（更高级别的逻辑可以通过标记端口的所有权来标记通道所有权）。只有与通道端关联的模块才能在其上发送或接收数据包。

## 技术指标

### 数据流可视化

客户端、连接、通道和数据包的体系结构：

![Dataflow Visualisation](../../../../spec/ics-004-channel-and-packet-semantics/dataflow.png)

### 预备知识

#### 存储路径

通道的结构存储在一个结合了端口标识符和通道标识符的唯一存储路径前缀下：

```typescript
function channelPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

与通道关联的能力键存储在`channelCapabilityPath` ：

```typescript
function channelCapabilityPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
  return "{channelPath(portIdentifier, channelIdentifier)}/key"
}
```

无符号整数计数器`nextSequenceSend`和`nextSequenceRecv`是分开存储的，因此可以单独证明它们：

```typescript
function nextSequenceSendPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceSend"
}

function nextSequenceRecvPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceRecv"
}
```

固定大小的加密承诺数据包数据字段存储在数据包序号下：

```typescript
function packetCommitmentPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/packets/" + sequence
}
```

存储区中缺失的路径相当于占用零位。

数据包确认数据存储在`packetAcknowledgementPath` ：

```typescript
function packetAcknowledgementPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/acknowledgements/" + sequence
}
```

无序通道必须始终向该路径写入确认信息（甚至是空的），使得此类确认信息的缺失可以用作超时证明。有序通道也可以写一个确认信息，但不是必须的。

### 版本控制

在握手过程中，通道的两端在与该通道关联的版本字节串上达成一致。 此版本字节串的内容对于 IBC 核心协议保持也应该保持不透明。
状态机主机可以利用版本数据来标示其支持的 IBC / APP 协议，确认数据包编码格式，或在协商与 IBC 协议之上自定义逻辑有关的其他通道元数据。

状态机主机可以安全地忽略版本数据或指定一个空字符串。

### 子协议

> 注意：如果主机状态机正在使用对象能力认证（请参阅 [ICS 005](../ics-005-port-allocation) ），则所有使用端口的功能都将带有附加能力参数。

#### 标识符验证

通道存储在唯一的`(portIdentifier, channelIdentifier)`前缀下。
可以提供验证函数`validatePortIdentifier` 。

```typescript
type validateChannelIdentifier = (portIdentifier: Identifier, channelIdentifier: Identifier) => boolean
```

如果未提供，默认的`validateChannelIdentifier`函数将始终返回`true` 。

#### 通道生命周期管理

![Channel State Machine](../../../../spec/ics-004-channel-and-packet-semantics/channel-state-machine.png)

发起人 | 数据报 | 作用链 | 先前状态 (A, B) | 作用后状态 (A, B)
--- | --- | --- | --- | ---
参与者 | ChanOpenInit | A | (none, none) | (INIT, none)
中继器进程 | ChanOpenTry | B | (INIT, none) | (INIT, TRYOPEN)
中继器进程 | ChanOpenAck | A | (INIT, TRYOPEN) | (OPEN, TRYOPEN)
中继器进程 | ChanOpenConfirm | B | (OPEN, TRYOPEN) | (OPEN, OPEN)

发起人 | 数据报 | 作用链 | 先前状态 (A, B) | 作用后状态 (A, B)
--- | --- | --- | --- | ---
参与者 | ChanCloseInit | A | (OPEN, OPEN) | (CLOSED, OPEN)
中继器进程 | ChanCloseConfirm | B | (CLOSED, OPEN) | (CLOSED, CLOSED)

##### 建立握手

与另一个链上的模块发起通道建立握手的模块调用`chanOpenInit`函数。

建立通道必须提供本地通道标识符、本地端口、远程端口和远程通道标识符的标识符。

当建立握手完成后，发起握手的模块将拥有在账本上已创建通道的一端，而对应的另一条链的模块将拥有通道的另一端。创建通道后，所有权就无法更改（尽管更高级别的抽象可以实现并提供此功能）。

```typescript
function chanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string): CapabilityKey {
    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))

    abortTransactionUnless(connectionHops.length === 1) // for v1 of the IBC protocol

    abortTransactionUnless(provableStore.get(channelPath(portIdentifier, channelIdentifier)) === null)
    connection = provableStore.get(connectionPath(connectionHops[0]))

    // optimistic channel handshakes are allowed
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state !== CLOSED)
    abortTransactionUnless(authenticate(privateStore.get(portPath(portIdentifier))))
    channel = ChannelEnd{INIT, order, counterpartyPortIdentifier,
                         counterpartyChannelIdentifier, connectionHops, version}
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    key = generate()
    provableStore.set(channelCapabilityPath(portIdentifier, channelIdentifier), key)
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    return key
}
```

模块调用`chanOpenTry`函数，以接受由另一条链上的模块发起的通道建立握手的第一步。

```typescript
function chanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string,
  proofInit: CommitmentProof,
  proofHeight: uint64): CapabilityKey {
    abortTransactionUnless(validateChannelIdentifier(portIdentifier, channelIdentifier))
    abortTransactionUnless(connectionHops.length === 1) // for v1 of the IBC protocol
    previous = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(
      (previous === null) ||
      (previous.state === INIT &&
       previous.order === order &&
       previous.counterpartyPortIdentifier === counterpartyPortIdentifier &&
       previous.counterpartyChannelIdentifier === counterpartyChannelIdentifier &&
       previous.connectionHops === connectionHops &&
       previous.version === version)
      )
    abortTransactionUnless(authenticate(privateStore.get(portPath(portIdentifier))))
    connection = provableStore.get(connectionPath(connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{INIT, order, portIdentifier,
                          channelIdentifier, connectionHops.reverse(), counterpartyVersion}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofInit,
      counterpartyPortIdentifier,
      counterpartyChannelIdentifier,
      expected
    ))
    channel = ChannelEnd{TRYOPEN, order, counterpartyPortIdentifier,
                         counterpartyChannelIdentifier, connectionHops, version}
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    key = generate()
    provableStore.set(channelCapabilityPath(portIdentifier, channelIdentifier), key)
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    return key
}
```

握手发起模块调用`chanOpenAck` ，以确认对方链的模块已接受发起的请求。

```typescript
function chanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string,
  proofTry: CommitmentProof,
  proofHeight: uint64) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state === INIT || channel.state === TRYOPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{TRYOPEN, channel.order, portIdentifier,
                          channelIdentifier, channel.connectionHops.reverse(), counterpartyVersion}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofTry,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))
    channel.state = OPEN
    channel.version = counterpartyVersion
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

握手接受模块调用`chanOpenConfirm`函数以确认
在另一条链上进行握手发起模块的确认信息，并完成通道创建握手。

```typescript
function chanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === TRYOPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{OPEN, channel.order, portIdentifier,
                          channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofAck,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))
    channel.state = OPEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

##### 关闭握手

两个模块中的任意一个通过调用`chanCloseInit`函数来关闭其通道端。一旦一端关闭，通道将无法重新打开。

调用模块可以在调用`chanCloseInit`时原子性的执行适当的应用程序逻辑。

通道关闭后，任何传递中的数据包都会超时。

```typescript
function chanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    channel.state = CLOSED
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

一旦一端已经关闭，对方模块调用`chanCloseConfirm`函数以关闭其通道端。

在调用`chanCloseConfirm`的同时，模块可以原子性的执行其他恰当的应用逻辑。

一旦通道关闭，通道将无法重新打开。

```typescript
function chanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofInit: CommitmentProof,
  proofHeight: uint64) {
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(portIdentifier, channelIdentifier))))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{CLOSED, channel.order, portIdentifier,
                          channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofInit,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))
    channel.state = CLOSED
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
}
```

#### 数据包流和处理

![Packet State Machine](../../../../spec/ics-004-channel-and-packet-semantics/packet-state-machine.png)

##### 一个数据包的日常

以下的步骤发生在一个数据包从机器 *A* 上的模块 *1* 发送到机器 *B* 上的模块 *2*，从头开始。

该模块可以通过 [ICS 25](../ics-025-handler-interface) 或 [ICS 26](../ics-026-routing-module) 接入 IBC 处理程序。

1. 以任何顺序初始客户端和端口设置
    1. 在 *A* 上为 *B* 创建客户端（请参阅 [ICS 2](../ics-002-client-semantics) ）
    2. 在 *B* 上为 *A* 创建客户端（请参阅 [ICS 2](../ics-002-client-semantics) ）
    3. 模块 *1* 绑定到端口（请参阅 [ICS 5](../ics-005-port-allocation) ）
    4. 模块 *2* 绑定到端口（请参阅 [ICS 5](../ics-005-port-allocation) ），该端口以带外方式（out-of-band）传输到模块 *1*
2. 建立连接和通道，按顺序乐观发送（optimistic send）
    1. 模块 *1* 自 *A* 向 *B* 创建连接握手（请参见 [ICS 3](../ics-003-connection-semantics) ）
    2. 使用新创建的连接（此 ICS），自 *1* 向 *2* 开始创建通道握手
    3. 通过新创建的通道自 *1* 向 *2* 发送的数据包（此 ICS）
3. 握手成功完成（如果任一握手失败，则连接/通道可以关闭且数据包超时）
    1. 连接握手成功完成（请参阅 [ICS 3](../ics-003-connection-semantics) ）（这需要中继器进程参与）
    2. 通道握手成功完成（此 ICS）（这需要中继器进程的参与）
4. 在状态机 *B* 的模块 *2* 上确认数据包（如果超过超时区块高度，则确认数据包超时）（这将需要中继器进程参与）
5. 确认消息从状态机 *B* 上的模块 *2* 被中继回状态机 *A* 上的模块 *1*

从空间上表示，两台机器之间的数据包传输可以表示如下：

![Packet Transit](../../../../spec/ics-004-channel-and-packet-semantics/packet-transit.png)

##### 发送数据包

`sendPacket`函数由模块调用，以便在调用模块的通道端将 IBC 数据包发送到另一条链上的相应模块。

在调用`sendPacket`的同时，调用模块必须同时原子性的执行应用逻辑。

IBC 处理程序按顺序执行以下步骤：

- 检查用于发送数据包的通道和连接是否打开
- 检查调用模块是否拥有发送端口
- 检查数据包元数据与通道以及连接信息是否匹配
- 检查目标链尚未达到指定的超时区块高度
- 递增通道关联的发送序号
- 存储对数据包数据和数据包超时信息的固定大小加密承诺

请注意，完整的数据包不会存储在链的状态中——仅仅存储数据和超时信息的简短哈希加密承诺。数据包数据可以从交易的执行中计算得出，并可能作为中继器可以索引的日志输出出来。

```typescript
function sendPacket(packet: Packet) {
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))

    // optimistic sends are permitted once the handshake has started
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))

    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state !== CLOSED)

    // sanity-check that the timeout height hasn't already passed in our local client tracking the receiving chain
    latestClientHeight = provableStore.get(clientPath(connection.clientIdentifier)).latestClientHeight()
    abortTransactionUnless(latestClientHeight < packet.timeoutHeight)

    nextSequenceSend = provableStore.get(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(packet.sequence === nextSequenceSend)

    // all assertions passed, we can alter state

    nextSequenceSend = nextSequenceSend + 1
    provableStore.set(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel), nextSequenceSend)
    provableStore.set(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence), hash(packet.data, packet.timeout))

    // log that a packet has been sent
    emitLogEntry("sendPacket", {sequence: packet.sequence, data: packet.data, timeout: packet.timeout})
}
```

#### 接受数据包

模块调用`recvPacket`函数以接收和处理在对应的链的通道端发送的 IBC 数据包。

在调用`recvPacket`函数的同时，调用模块必须原子性的执行应用逻辑，可能需要事先计算出数据包确认消息的值。

IBC 处理程序按顺序执行以下步骤：

- 检查接收数据包的通道和连接是否打开
- 检查调用模块是否拥有接收端口
- 检查数据包元数据与通道及连接信息是否匹配
- 检查数据包序号是通道端期望接收的（对于有序通道而言）
- 检查尚未打到超时高度
- 在传出链的状态下检查数据包数据的加密承诺包含证明
- 在数据包唯一的存储路径上设置一个不透明确认值（如果确认信息为非空或是无序通道）
- 递增与通道端关联的数据包接收序号（仅限有序通道）

```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  acknowledgement: bytes): Packet {

    channel = provableStore.get(channelPath(packet.destPort, packet.destChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.destPort, packet.destChannel))))
    abortTransactionUnless(packet.sourcePort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.sourceChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    abortTransactionUnless(getConsensusHeight() < packet.timeoutHeight)

    abortTransactionUnless(connection.verifyPacketData(
      proofHeight,
      proof,
      packet.sourcePort,
      packet.sourceChannel,
      packet.sequence,
      concat(packet.data, packet.timeout)
    ))

    // all assertions passed (except sequence check), we can alter state

    if (acknowledgement.length > 0 || channel.order === UNORDERED)
      provableStore.set(
        packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence),
        hash(acknowledgement)
      )

    if (channel.order === ORDERED) {
      nextSequenceRecv = provableStore.get(nextSequenceRecvPath(packet.destPort, packet.destChannel))
      abortTransactionUnless(packet.sequence === nextSequenceRecv)
      nextSequenceRecv = nextSequenceRecv + 1
      provableStore.set(nextSequenceRecvPath(packet.destPort, packet.destChannel), nextSequenceRecv)
    }

    // log that a packet has been received & acknowledged
    emitLogEntry("recvPacket", {sequence: packet.sequence, timeout: packet.timeout, data: packet.data, acknowledgement})

    // return transparent packet
    return packet
}
```

#### 确认

模块会调用`acknowledgePacket`函数来处理先前由对方链上对方模块的通道上的调用模块发送的数据包的确认。
`acknowledgePacket`还会清除数据包的加密承诺，因为数据包已经收到并处理，所以这个不再需要。

在调用`acknowledgePacket`的同时，调用模块可以原子性的执行适当的应用程序确认处理逻辑。

```typescript
function acknowledgePacket(
  packet: OpaquePacket,
  acknowledgement: bytes,
  proof: CommitmentProof,
  proofHeight: uint64): Packet {

    // abort transaction unless that channel is open, calling module owns the associated port, and the packet fields match
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // verify we sent the packet and haven't cleared it out yet
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeout))

    // abort transaction unless correct acknowledgement on counterparty chain
    abortTransactionUnless(connection.verifyPacketAcknowledgement(
      proofHeight,
      proof,
      packet.destPort,
      packet.destChannel,
      packet.sequence,
      acknowledgement
    ))

    // all assertions passed, we can alter state

    // delete our commitment so we can't "acknowledge" again
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

#### 超时

应用程序语义可能需要定义超时：超时是链在将一笔交易视作错误之前将等待多长时间的上限。由于这两个链本地时间的不同，因此这是一个明显的双花攻击的方向——攻击者可能延迟发送确认消息或在超时时间后发送数据包——因此应用程序本身无法安全的实现的思考不充分的超时逻辑。

请注意，为了避免任何可能的“双花”攻击，超时算法要求目标链正在运行并且是可访问的。一个人不能在一个网络完全分区（network parition）的情况下证明任何事情，并且必须等待连接。必须在接收者链上证明已经超时，而不仅仅是以发送链没有收到响应作为判断。

##### 发送端

`timeoutPacket`函数由最初尝试将数据包发送到对方链的模块调用，
如果在没有提交数据包的情况下对方链达到超时区块高度，证明该数据包无法再执行，并允许调用模块安全的执行适当的状态转换。

在调用`timeoutPacket`的同时，调用模块可以原子性的执行适当的应用超时处理逻辑。

在有序通道的情况下，`timeoutPacket`检查接收通道端的`recvSequence` ，如果数据包已超时，则关闭通道。

在无序通道的情况下， `timeoutPacket`检查是否存在确认（如果接收到数据包，则该确认将被写入）。面对超时的数据包，无序通道预期会继续工作。

如果后续数据包的超时高度之间有依赖关系，则可以执行所有数据包的安全批量超时而不是使用超时数据包。该规范暂时省略了细节。

```typescript
function timeoutPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)

    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // 注意：连接可能已经关闭了
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // 检查对端已经超过了超时高度
    abortTransactionUnless(proofHeight >= packet.timeoutHeight)

    // 检查数据包还没被收到
    abortTransactionUnless(nextSequenceRecv < packet.sequence)

    // 验证确实发送了数据包，检查存储
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeout))

    if channel.order === ORDERED
      // 有序通道：检查接收序号和声称的一样
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
    else
      // 无序通道：验证数据包索引的确认信息是缺失的
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.sourcePort,
        packet.sourceChannel,
        packet.sequence
      ))

    // 所有断言通过，我们可以改变状态

    // 删除我们的加密承诺
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    if channel.order === ORDERED {
      // 有序通道：关闭通道
      channel.state = CLOSED
      provableStore.set(channelPath(packet.sourcePort, packet.sourceChannel), channel)
    }

    // 返回透明数据包
    return packet
}
```

##### 关闭时超时

该模块调用`timeoutOnClose`函数以证明未被收到的数据包的地址的通道已经关闭，因此永远不会收到该数据包（即使尚未达到`timeoutHeight` ）。

```typescript
function timeoutOnClose(
  packet: Packet,
  proof: CommitmentProof,
  proofClosed: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    // 注意：通道可能已经关闭了
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // 注意：连接可能已经关闭了
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // 验证我们确实发送了数据包，检查存储
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeout))

    // 检查对方通道端已经关闭
    expected = ChannelEnd{CLOSED, channel.order, channel.portIdentifier,
                          channel.channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofClosed,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))

    if channel.order === ORDERED
      // 有序通道：检查接受序号和声称的一致
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
    else
      // 无序通道：验证数据包索引的确认信息是缺失的
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.sourcePort,
        packet.sourceChannel,
        packet.sequence
      ))

    // 通过了所有断言，我们可以修改状态

    // 删除我们的加密承诺
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // 返回透明数据包
    return packet
}
```

##### 清理状态

模块调用`cleanupPacket`以从状态中删除收到的数据包承诺。接收端必须已经处理过该数据包（无论是正常处理还是超时）。

在有序通道的情况下， `cleanupPacket`通过证明已在另一端收到数据包来清理有序通道上的数据包。

在无序通道的情况下， `cleanupPacket`通过证明已写入关联的确认信息清理无序通道上的数据包。

```typescript
function cleanupPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecvOrAcknowledgement: Either<uint64, bytes>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticate(privateStore.get(channelCapabilityPath(packet.sourcePort, packet.sourceChannel))))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // 注意：连接可能已经关闭
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // 终止交易除非对端已经收到数据包
    abortTransactionUnless(nextSequenceRecv > packet.sequence)

    // 验证我们确实发送了数据包，检查存储
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
               === hash(packet.data, packet.timeout))

    if channel.order === ORDERED
      // 检查接收序号和声称的一致
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecvOrAcknowledgement
      ))
    else
      // 终止交易除非对端有确认信息
      abortTransactionUnless(connection.verifyPacketAcknowledgement(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence,
        nextSequenceRecvOrAcknowledgement
      ))

    // 通过了所有断言，我们可以改变状态

    // 清理存储
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // 返回透明数据包
    return packet
}
```

#### 关于竞争条件的探讨

##### 同时发生握手尝试

如果两台状态机同时彼此发起通道创建握手，或尝试使用相同的标识符，则两者都会失败，必须使用新的标识符。

##### 标识符分配

在目标链上分配标识符存在不可避免的竞争条件。最好建议模块使用伪随机，无价值的标识符。设法声明另一个模块希望使用的标识符，但是，尽管令人烦恼，但不能在握手期间中间人攻击，因为接收模块必须已经拥有握手所针对的端口。

##### 超时/数据包确认

数据包超时和数据包确认之间没有竞争条件，因为数据包只能在接收之前检查超过或没超过超时区块高度。

##### 握手期间的中间人攻击

跨链状态的验证可防止连接握手和通道握手的中间人攻击，因为模块已知道所有信息（源客户端、目标客户端、通道等），该信息将启动握手之前进行确认完成。

##### 有正在传输数据包时的连接/通道关闭

如果在传输数据包时关闭了连接或通道，则数据包将不再被目标链接收，并且在源链上超时。

#### 查询通道

可以使用`queryChannel`函数查询通道：

```typescript
function queryChannel(connId: Identifier, chanId: Identifier): ChannelEnd | void {
    return provableStore.get(channelPath(connId, chanId))
}
```

### 属性和不变性

- 通道和端口标识符的唯一组合是先到先服务的：分配了一对标示符后，只有拥有相应端口的模块才能在该通道上发送或接收。
- 假设链在超时窗口后依然有活性，则数据包只传送一次，并且在超时的情况下，只在发送链上超时一次。
- 通道握手不能受到区块链上的另一个模块或另一个区块链的 IBC 处理程序的中间人攻击。

## 向后兼容性

不适用。

## 向前兼容性

数据结构和编码可以在连接或通道级别进行版本控制。通道逻辑完全不依赖于数据包数据格式，可以由模块在任何时候以自己喜欢的方式对其进行更改。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 发布历史

2019年6月5日-提交草案

2019年7月4日-修改无序通道和确认

2019年7月16日-更改“多跳”路由未来的兼容性

2019年7月29日-修改以处理连接关闭后的超时

2019年8月13日-各种修改

2019年8月25日-清理

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
