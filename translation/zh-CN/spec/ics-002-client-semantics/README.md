---
ics: 2
title: 客户端语义
stage: 草案
category: IBC/TAO
kind: 接口
requires: 23, 24
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2020-01-13
---

## 概要

该标准规定了实现区块链间通信协议的状态机的共识算法必须满足的属性。 这些属性对更高层协议抽象中的有效安全的验证而言是必需的。IBC 中用于验证另一台状态机的共识记录及状态子组件的算法称为“合法性判定式”，并将其与验证者认为正确的状态配对形成“轻客户端”（通常简称为“客户端”）。

该标准还规定了如何在典范的 IBC 处理程序中存储、注册和更新轻客户端。 所存储的客户端实例能被第三方参与者进行检视，例如，用户检查链的状态并确定是否发送 IBC 数据包。

### 动机

在 IBC 协议中，参与者（可能是终端用户、链下进程或状态机）需要能够对另一台状态机共识算法认同的状态更新进行验证，以及拒绝其共识算法不认同的任何潜在状态更新。轻客户端就是一个带有能做到上面功能的状态机的算法。该标准规范了轻客户端的模型和要求，因此，只要提供满足所列要求的相关轻客户端算法，IBC 协议就可以轻松地与运行新共识算法的新状态机集成。

除了本规范中描述的属性外，IBC 对状态机的内部操作及其共识算法没有任何要求。一台状态机可能由一个单独的私钥签名进程、多个统一仲裁签名的进程、多个运行拜占庭容错共识算法的进程或其他尚未发明的配置组成——从 IBC 的角度来看，一个状态机是完全由其轻客户端的验证和不良行为检测逻辑来定义的。
客户端通常不包括对状态转换逻辑的验证（因为这将等同于在其他状态机上又简单的执行了一次），但是在特定情况下，客户端可以选择验证部分状态转换。

客户端还可以当作其他客户端的阈值视角。 如果模块利用 IBC 协议与概率最终性（probabilistic-finality）共识算法进行交互，对于不同的应用可能需要不同的最终性阈值，那么可以创建一个只写客户端来跟踪不同区块头，多个具有不同最终性阀值（多少块确认前的状态根被认为是最终的）的只读客户端可以使用相同的状态。

客户端协议还应该支持第三方引荐。 Alice 是一台状态机上的一个模块，希望将 Bob（Alice 认识的第二台状态机上的第二个模块）介绍给 Carol（Alice 认识但 Bob 不认识的第三台状态机上的第三个模块）。Alice 必须利用现有的通道传送给 Bob 用于和 Carol 通信的典范序列化的合法性判定式，然后 Bob 可以与 Carol 建立连接和通道并直接通信。
如有必要，在 Bob 进行连接尝试之前，Alice 还可以向 Carol 传送 Bob 的合法性判定式，使得 Carol 获悉并接受进来的请求。

客户端接口也应该被构建为：只要底层状态机可以提供恰当的计算和存储gas 的计量机制，那么在运行时可以定义一个安全地提供定制验证逻辑的自定义轻客户端。例如，在支持运行 WASM 的状态机主机上，客户端实例创建完后，合法性判定式和不良行为判定式可以作为可执行的 WASM 函数提供。

### 定义

- `get`, `set`, `Path`, 和 `Identifier` 在 [ICS 24](../ics-024-host-requirements) 中被定义.

- `CommitmentRoot` 如同在 [ICS 23](../ics-023-vector-commitments) 中被定义的那样，它必须为下游逻辑提供一种廉价方式去验证键值对是否在特定高度的状态中存在。

- `ConsensusState` 是表示合法性判定式状态的不透明类型。`ConsensusState` 必须能够验证相关共识算法所达成一致的状态更新。 它也必须以典范的方式实现可序列化，以便第三方（例如对方状态机）可以检查特定状态机是否存储了特定的共识状态。 它最终必须能被使用它的状态机检视，必入状态机可以查看某个过去高度的共识状态。

- `ClientState` 是表示一个客户端状态的不透明类型。
    `ClientState` 必须公开查询函数，以验证处于特定高度的状态下键/值对的存在或不存在，并且能够获取当前的共识状态.

### 所需属性

轻客户端必须提供安全的算法使用现有的`ConsensusState`来验证其他链的典范区块头 。然后，更高级别的抽象将能够验证
存储在`ConsensusState`的`CommitmentRoot`的状态的子组件是
确保已由其他链的共识算法提交。

合法性判定式应反映正在运行相应的共识算法的全节点的行为。给定`ConsensusState`和消息列表，如果一个全节点接受由`Commit`生成的新`Header` ，那么轻客户端也必须接受它，如果一个全节点拒绝它，那么轻客户端也必须拒绝它。

轻客户端不是重放整个消息记录，因此在出现共识不良行为的情况下有可能轻客户端的行为和全节点不同。
在这种情况下，一个用来证明合法性判定式和全节点之间的差异的不良行为证明可以被生成将其提交给链，以便链可以安全地停用轻客户端，使过去的状态根无效，并等待更高级别的干预。

## 技术规范

该规范概述了每种*客户端类型*必须定义的内容。客户端类型是一组操作轻客户端所需的数据结构，初始化逻辑，合法性判定式和不良行为判定式的定义。实现 IBC 协议的状态机可以支持任意数量的客户端
类型，并且每种客户端类型都可以使用不同的初始共识状态实例化，以便进行跟踪不同的共识实例。为了在两台机器之间建立连接（请参阅 [ICS 3](../ics-003-connection-semantics) ），
这些机器必须各自支持与另一台机器的共识算法相对应的客户端类型。

特定的客户端类型应在本规范之后的版本中定义，并且该仓库中应存在一个典范的客户端类型列表。
实现了 IBC 协议的机器应遵守这些客户端类型，但他们可以选择仅支持一个子集。

### 数据结构

#### 共识状态

`ConsensusState` 是一个客户端类型定义的不透明数据结构，用来被合法性判定式验证新的提交和状态根。该结构可能包含共识过程产生的最后一次提交，包括签名和验证人集合元数据。

`ConsensusState` 必须由一个 `Consensus`实例生成，该实例为每个 `ConsensusState`分配唯一的高度（这样，每个高度恰好具有一个关联的共识状态）。如果没有一样的加密承诺根，则同一链上的两个`ConsensusState`不应具有相同的高度。此类事件称为“矛盾行为”，必须归类为不良行为。 如果发生这种情况，则应生成并提交证明，以便可以冻结客户端，并根据需要使先前的状态根无效。

链的 `ConsensusState` 必须可以被典范的序列化，以便其他链可以检查存储的共识状态是否与另一个共识状态相等（请参见 [ICS 24](../ics-024-host-requirements) 的键表）。

```typescript
type ConsensusState = bytes
```

`ConsensusState` 必须存储在下面定义的指定的键下，这样其他链可以验证一个特定的共识状态是否已存储。

#### 区块头

`Header` 是由客户端类型定义的不透明数据结构，它提供信息以用来更新`ConsensusState`。可以将区块头提交给关联的客户端以更新存储的`ConsensusState` 。区块头可能包含一个高度、一个证明、一个加密承诺根，还有可能的合法性判定式更新。

```typescript
type Header = bytes
```

#### 共识

`Consensus` 是一个 `Header` 生成函数，它利用之前的
`ConsensusState` 和消息返回结果。

```typescript
type Consensus = (ConsensusState, [Message]) => Header
```

### 区块链

区块链是一个生成有效`Header`的共识算法。它从创世`ConsensusState`开始通过各种消息生成一个唯一的区块头列表。

`区块链` 被定义为

```typescript
interface Blockchain {
  genesis: ConsensusState
  consensus: Consensus
}
```

其中

- `Genesis`是一个创世`ConsensusState`
- `Consensus`是一个区块头生成函数

从`Blockchain`生成的区块头应满足以下条件：

1. 每个`Header`不得超过一个直接的孩子

- 满足，假如：最终性和安全性
- 可能的违规场景：验证人双重签名，链重组（中本聪共识中）

1. 每个`Header`最终必须至少有一个直接的孩子

- 满足，假如：活跃性，轻客户端验证程序连续性
- 可能的违规场景：同步停止，不兼容的硬分叉

1. 每个`Header`必须由`Consensus`生成，以确保有效的状态转换

- 满足，假如：正确的块生成和状态机
- 可能的违规场景：不变量被破坏，超过多数验证人共谋

除非区块链满足以上所有条件，否则 IBC 协议可能无法按预期工作：链可能会收到多个冲突数据包，链无法从超时事件中恢复，链可以窃取用户的资产等

合法性判定式的合法性取决于`Consensus` 的安全模型。例如， `Consensus`可以是受一个被信任的运营商管理的 PoA（proof of authority），或质押价值不足的 PoS（proof of stake）。在这种情况下，安全假设可能被破坏， `Consensus`与合法性判定式的关联就不存在了，并且合法性判定式的行为变的不可定义。此外， `Blockchain`可能不再满足上述要求，这将导致区块链与 IBC 协议不再兼容。在这些导致故障的情况下，一个会不良行为证明可以被生成并提交给包含客户端的区块链以安全的冻结轻客户端，并防止之后的 IBC 数据包被中继。

#### 合法性判定式

合法性判定式是由一种客户端类型定义的一个不透明函数，用与根据当前`ConsensusState`来验证 `Header` 。使用合法性判定式应该比通过父`Header` 和一系列网络消息进行完全共识算法重放的计算效率高很多。

合法性判定式和客户端状态更新逻辑是合并在一个单独的 `checkValidityAndUpdateState`类型中的，它的定义如下：

```typescript
type checkValidityAndUpdateState = (Header) => Void
```

`checkValidityAndUpdateState` 在输入区块头无效的情况下必须抛出一个异常。

如果给定的区块头有效，客户端必须改变内部状态存储立即确认的共识根，以及更新必要的签名权威跟踪（例如对验证人集合的更新）以供后续的合法性判定式调用。

#### 不良行为判定式

一个不良行为判定式是由一种客户端类型定义的不透明函数，用于检查数据是否对共识协议的构成违规。可能是出现两个拥有不同状态根但在同一个区块高度的签名的区块头、一个包含无效状态转换的签名的区块头或其他由共识算法定义的不良行为的证据。

不良行为判定式和客户端状态更新逻辑是合并在一个单独的`checkMisbehaviourAndUpdateState`类型中的，它的定义如下：

```typescript
type checkMisbehaviourAndUpdateState = (bytes) => Void
```

`checkMisbehaviourAndUpdateState` 在给定证据无效的情况下必须抛出一个异常。

如果一个不良行为是有效的，客户端还必须根据不良行为的性质去更改内部状态，来标记先前认为有效的区块高度为无效。

#### 客户端状态

客户端状态是由一种客户端类型定义的不透明数据结构。它或将保留任意的内部状态去追踪已经被验证过的状态根和发生过的不良行为。

轻客户端是一种不透明的表现形式——不同的共识算法可以定义不同的轻客户端更新算法，但是轻客户端必须对 IBC 处理程序公开一组通用的查询函数。

```typescript
type ClientState = bytes
```

客户端类型必须定义一种方法用提供的共识状态初始化客户端状态，并根据情况写入状态。

```typescript
type initialise = (consensusState: ConsensusState) => ClientState
```

客户断类型必须定义一种方法来获取当前高度（最近验证的区块头的高度）。

```typescript
type latestClientHeight = (
  clientState: ClientState)
  => uint64
```

#### 承诺根

`承诺根` 是根据 [ICS 23](../ics-023-vector-commitments) 由一种客户端类型定义的不透明数据结构。它用于验证处于特定最终高度（必须与特定承诺根相关联）的状态中是否存在特定键/值对。

#### 状态验证

状态类型必须定义一系列函数去对客户端追踪的状态机的内部状态进行验证。内部实现细节可能存在差异（例如，一个回环客户端可以直接读取状态信息且不需要提供证明）。

##### 所需函数：

`verifyClientConsensusState` 验证存储在目标机器上的特定客户端的共识状态的证明。

```typescript
type verifyClientConsensusState = (
  clientState: ClientState,
  height: uint64,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState)
  => boolean
```

`verifyConnectionState` 验证存储在目标机器上的特定连接端的连接状态的证明。

```typescript
type verifyConnectionState = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd)
  => boolean
```

`verifyChannelState` 验证在存储在目标机器上上的指定通道端，特定端口下的的通道状态的证明。

```typescript
type verifyChannelState = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd)
  => boolean
```

`verifyPacketData`验证在指定的端口，指定的通道和指定的序列的向外发送的数据包承诺的证明。

```typescript
type verifyPacketData = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes)
  => boolean
```

`verifyPacketAcknowledgement` 在指定的端口、指定的通道和指定的序号的传入数据包的确认的证明。

```typescript
type verifyPacketAcknowledgement = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes)
  => boolean
```

`verifyPacketAcknowledgementAbsence` 验证在指定的端口、指定的通道和指定的序号的未收到传入数据包确认的证明。

```typescript
type verifyPacketAcknowledgementAbsence = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64)
  => boolean
```

`verifyNextSequenceRecv` 验证在指定端口上和指定通道接收的下一个序号的证明。

```typescript
type verifyNextSequenceRecv = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64)
  => boolean
```

##### 实现策略

###### 回环

一个本地机器的回环客户端仅需要读取本地状态，其必须具有访问权限。

###### 简单签名

具有已知公钥的独立机器的客户端检查该本地机器发送的消息的签名，
作为`Proof`参数提供。 `height`参数可以用作重放保护随机数。

这种方式里也可以使用多重签名或门限签名方案。

###### 客户代理

客户代理验证的是目标机器的代理机器的证明。通过包含首先是一个代理机器上客户端状态的证明，然后是目标机器的子状态相对于代理计算机上的客户端状态的证明。这使代理客户端可以避免存储和跟踪目标机器本身的共识状态，但是要以代理机器正确性的安全假设为代价。

###### 莫克尔状态树

对于具有莫克尔状态树的状态机的客户端，可以通过调用`verifyMembership`或`verifyNonMembership`来实现这些功能。使用经过验证的存储在`ClientState`中的莫克尔根，按照 [ICS 23](../ics-023-vector-commitments) 验证处于特定高度的状态中特定键/值对是否存在。

```typescript
type verifyMembership = (ClientState, uint64, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, uint64, CommitmentProof, Path) => boolean
```

### 子协议

IBC 处理程序必须实现以下定义的函数。

#### 标识符验证

客户端存储在唯一的`Identifier`前缀下。 ICS 002 不要求以特定方式生成客户端标识符，仅要求它们是唯一的即可。但是，如果需要，可以限制`Identifier`的空间。可能需要提供下面的验证函数`validateClientIdentifier` 。

```typescript
type validateClientIdentifier = (id: Identifier) => boolean
```

如果没有提供以上函数，默认的`validateClientIdentifier`会永远返回`true` 。

##### 利用过去的状态根

为了避免客户端更新（更改状态根）与握手中携带证明的交易或数据包收据之间的竞争条件，许多 IBC 处理程序允许调用方指定一个之前的状态根作为参考，这类 IBC 处理程序必须确保它们对调用者传入的区块高度执行任何必要的检查，以确保逻辑上的正确性。

#### 创建

通过调用`createClient`附带特定的标识符和初始化共识状态来创建一个客户端。

```typescript
function createClient(
  id: Identifier,
  clientType: ClientType,
  consensusState: ConsensusState) {
    abortTransactionUnless(validateClientIdentifier(id))
    abortTransactionUnless(privateStore.get(clientStatePath(id)) === null)
    abortSystemUnless(provableStore.get(clientTypePath(id)) === null)
    clientType.initialise(consensusState)
    provableStore.set(clientTypePath(id), clientType)
}
```

#### 查询

可以通过标识符查询客户端共识状态和客户端内部状态，但是查询的特定路径由每种客户端类型定义。

#### 更新

客户端的更新是通过提交新的`Header`来完成的。`Identifier`用于指向逻辑将被更新的客户端状态。 当使用`ClientState`的合法性判定式和`ConsensusState`验证新的`Header`时，客户端必须相应地更新其内部状态，还可能更新最终性承诺根和`ConsensusState`中的签名授权逻辑。

```typescript
function updateClient(
  id: Identifier,
  header: Header) {
    clientType = provableStore.get(clientTypePath(id))
    abortTransactionUnless(clientType !== null)
    clientState = privateStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    clientType.checkValidityAndUpdateState(clientState, header)
}
```

#### 不良行为

如果客户端检测到不良行为的证据，则可以向客户端发出警报，可能使先前有效的状态根无效并阻止未来的更新。

```typescript
function submitMisbehaviourToClient(
  id: Identifier,
  evidence: bytes) {
    clientType = provableStore.get(clientTypePath(id))
    abortTransactionUnless(clientType !== null)
    clientState = privateStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    clientType.checkMisbehaviourAndUpdateState(clientState, evidence)
}
```

### 实现示例

一个合法性判定式示例是构建在运行单一运营者的共识算法的区块链上的，其中有效区块由这个运营者进行签名。区块链运行过程中运营者的签名密钥可以被改变。

客户端特定的类型定义如下：

- `ConsensusState` 存储最新的区块高度和最新的公钥
- `Header`包含一个区块高度、一个新的承诺根、一个操作者的签名以及可能还包括一个新的公钥
- `checkValidityAndUpdateState` 检查已经提交的区块高度是否是单调递增的以及签名是否正确，并更改内部状态
- `checkMisbehaviourAndUpdateState` 被用于检查两个相同区块高度但承诺根不同的区块头，并更改内部状态

```typescript
interface ClientState {
  frozen: boolean
  pastPublicKeys: Set<PublicKey>
  verifiedRoots: Map<uint64, CommitmentRoot>
}

interface ConsensusState {
  sequence: uint64
  publicKey: PublicKey
}

interface Header {
  sequence: uint64
  commitmentRoot: CommitmentRoot
  signature: Signature
  newPublicKey: Maybe<PublicKey>
}

interface Evidence {
  h1: Header
  h2: Header
}

// algorithm run by operator to commit a new block
function commit(
  commitmentRoot: CommitmentRoot,
  sequence: uint64,
  newPublicKey: Maybe<PublicKey>): Header {
    signature = privateKey.sign(commitmentRoot, sequence, newPublicKey)
    header = {sequence, commitmentRoot, signature, newPublicKey}
    return header
}

// initialisation function defined by the client type
function initialise(consensusState: ConsensusState): () {
  clientState = {
    frozen: false,
    pastPublicKeys: Set.singleton(consensusState.publicKey),
    verifiedRoots: Map.empty()
  }
  privateStore.set(identifier, clientState)
}

// validity predicate function defined by the client type
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    abortTransactionUnless(consensusState.sequence + 1 === header.sequence)
    abortTransactionUnless(consensusState.publicKey.verify(header.signature))
    if (header.newPublicKey !== null) {
      consensusState.publicKey = header.newPublicKey
      clientState.pastPublicKeys.add(header.newPublicKey)
    }
    consensusState.sequence = header.sequence
    clientState.verifiedRoots[sequence] = header.commitmentRoot
}

function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusStates/{height}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, consensusState, proof)
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connections/{connectionIdentifier}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, connectionEnd, proof)
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
    return clientState.verifiedRoots[sequence].verifyMembership(path, channelEnd, proof)
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
    return clientState.verifiedRoots[sequence].verifyMembership(path, data, proof)
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
    return clientState.verifiedRoots[sequence].verifyMembership(path, acknowledgement, proof)
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
    return clientState.verifiedRoots[sequence].verifyNonMembership(path, proof)
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
    return clientState.verifiedRoots[sequence].verifyMembership(path, nextSequenceRecv, proof)
}

// misbehaviour verification function defined by the client type
// any duplicate signature by a past or current key freezes the client
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    h1 = evidence.h1
    h2 = evidence.h2
    abortTransactionUnless(clientState.pastPublicKeys.contains(h1.publicKey))
    abortTransactionUnless(h1.sequence === h2.sequence)
    abortTransactionUnless(h1.commitmentRoot !== h2.commitmentRoot || h1.publicKey !== h2.publicKey)
    abortTransactionUnless(h1.publicKey.verify(h1.signature))
    abortTransactionUnless(h2.publicKey.verify(h2.signature))
    clientState.frozen = true
}
```

### 属性和不变量

- 客户标识符是不可变的，先到先得。客户端无法删除（如果重复使用标识符，允许删除意味着允许将来重放过去的数据包）。

## 向后兼容性

不适用。

## 向前兼容性

只要新客户端类型符合该接口，就可以随意添建到 IBC 实现中。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

2019年3月5日-初稿已完成并作为PR提交

2019年5月29日-进行了各种修订，尤其是多个承诺根

2019年8月15日-进行大量返工以使客户端界面更加清晰

2020年1月13日-客户端类型分离和路径更改的修订

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
