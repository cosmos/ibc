# Synopsis / 概览

该标准规定了实现区块链间通信协议的机器的共识算法必须满足的特性。 这些属性对于高层协议抽象中的有效和安全验证是必需的。 IBC 中用于验证另一台机器的共有记录和状态子组件的算法称为“有效性断言”，并将其与验证者认为正确的状态配对形成“轻客户端”（简称为“客户端”）。

该标准还规定了如何在规范的 IBC 处理程序中存储、注册和更新轻客户端。 所存储的客户端实例将由第三方参与者进行内省，例如，用户检查链的状态并确定是否发送 IBC 数据包。

# Definitions / 定义

- `get`, `set`, `Path`, 和 `Identifier` 在 [ICS 24](https://github.com/cosmos/ics/blob/master/spec/ics-024-host-requirements) 中被定义.
- `CommitmentRoot` 如同在 [ICS 23](https://github.com/cosmos/ics/blob/master/spec/ics-023-vector-commitments) 中被定义的那样，它必须提供为下游逻辑提供一种廉价方式，去验证键值对是否在特定高度的世界状态中存在。
- `ConsensusState`/ 共识状态 是代表有效性述词的不透明类型。`ConsensusState` 必须能够验证相关共识算法所同意的状态更新。 它也必须以规范的方式实现可序列化，以便第三方（例如对应方的机器）可以检查特定机器是否存储了特定的共识状态。 它最终必须由它所针对的状态机进行自省，以便状态机可以在过去的高度查找其自己的共识状态。
- `ClientState`/ 客户端状态 是代表一个客户端状态的不透明类型。`ClientState` 必须公开查询函数，以验证处于特定高度的状态下键/值对的成员身份或非成员身份，并且能够提取当前的共识状态.

# Data Structure / 数据结构

## ConsensusState / 共识状态

`共识状态`是一个由客户端类型来定义的不透明数据结构，被有效性谓词用来验证新的区块提交和根状态。该结构可能包含共识过程产生的最后一次提交，包括签名和验证者集合元数据。

`共识状态` 必须由一个`共识`实例生成，该实例为每个`共识状态`分配唯一的高度（这样，每个高度恰好具有一个关联的共识状态）。如果没有一致的承诺根，则同一链上的两个`共识状态`不应具有相同的高度。此类事件称为“存疑行为”，必须归类为不当行为。 如果发生这种情况，则应生成并提交证明，以便可以冻结客户端，并根据需要使先前的状态根无效。

链的`共识状态`必须可以被规范地序列化，以便其他链可以检查存储的共识状态是否与另一个共识状态相等（请参见 ICS 24 了解密钥空间表）。

```
type ConsensusState = bytes
```

`共识状态` 必须存储在下面定义的特定密钥下，这样其他链可以验证一个特定的共识状态是否已存储。

## **Header / 报头**

`报头`是由客户端类型定义的不透明数据结构，它提供信息以用来更新`共识状态`。可以将报头提交给关联的客户端以更新存储的`共识状态`。 报头可能包含高度、证明、承诺根，并可能更新有效性谓词。

```
type Header = bytes
```

## **Consensus / 共识**

`共识` 是一个`报头` 生成函数，它利用之前的`共识状态` 和消息并返回结果。

```
type Consensus = (ConsensusState, [Message]) => Header
```

## **ClientState / 客户端状态**

客户端状态是一个由客户端类型定义的不透明数据结构。它或将保留任意的内部状态去追踪已经被验证过的块根和发生过的不良行为。

轻客户端是一种不透明的表现形式——不同的共识算法可以定义不同的轻客户端更新算法，但是轻客户端必须对 IBC 处理程序公开暴露通用查询功能集合。

```
type ClientState = bytes
```

客户端类型必须定义一个方法用提供的*共识状态*去初始化一个*客户端状态*：

```
type initialize = (state: ConsensusState) => ClientState
```

## **CommitmentProof / 承诺根**

`承诺根`是根据 ICS 23 由客户端类型定义的不透明数据结构。它用于验证处于特定最终高度（必须与特定承诺根相关联）的状态中是否存在特定键/值对。

# Blockchain / 区块链

`区块链`是一个生成有效`标头`的共识算法。它由创世文件`共识状态`生成带有任意消息的唯一的标头列表。

`区块链` 被定义为：

```
interface Blockchain {
  genesis: ConsensusState 
  consensus: Consensus
}
```

# Validity predicate / 有效性谓词

一个有效性谓词是由客户端类型定义的一个不透明函数，用与根据当前`共识状态`来验证 `标头` 。使用有效性谓词应该比给定父`标头` 和网络消息列表的完全共识重放算法拥有高得多的计算效率。

有效性谓词和客户端状态更新逻辑是绑定在一个单独的 `checkValidityAndUpdateState` 类型中的，它的定义如下：

```
type checkValidityAndUpdateState = (Header) => Void
```

`checkValidityAndUpdateState` 必须在输入非有效标头的情况下抛出一个异常。如果给定的标头有效，客户端必须改变内部状态以存储立即确认的状态根，以及更新必要的签名权限跟踪（例如对验证者集合的更新）以供后续对有效性谓词的调用。

## Misbehaviour predicate

一个非有效性谓词是由客户端类型定义的不透明函数，用于检查数据是否对共识协议的构成违规。这可能是出现两个拥有不同状态根但在同一个区块高度的签名的标头、一个包含无效状态转换签名的标头或这其他由共识算法定义的不良行为的证据。

非有效性谓词和客户端状态更新逻辑是绑定在一个单独的`checkMisbehaviourAndUpdateState`类型中的，它的定义如下：

```
type checkMisbehaviourAndUpdateState = (bytes) => Void
```

`checkMisbehaviourAndUpdateState` 在给定证据无效的情况下必须抛出一个异常。

如果一个不良行为是有效的，客户端还必须根据不良行为的性质去更改内部状态，来标记先前认为有效的区块高度为无效。

# State Verification / 状态验证

状态类型必须定义一系列函数去对客户端追踪的状态机的内部状态进行验证。内部实现细节可能存在差异（例如，一个回路客户端可以直接读取状态信息且不需要提供证明）。

**所需函数：**

- `verifyClientConsensusState` 验证存储在指定状态机上的特定客户端的共识状态的证明。

  ```
    type verifyClientConsensusState = (
      clientState: ClientState,
      height: uint64,
      proof: CommitmentProof,
      clientIdentifier: Identifier,
      consensusState: ConsensusState)
      => boolean
    
    # Identifier 在 ICS 24 中被定义
  ```

- `verifyConnectionState`验证在指定状态机上存储的特定连接端的连接状态证明。

  ```
    type verifyConnectionState = (
      clientState: ClientState,
      height: uint64,
      prefix: CommitmentPrefix,
      proof: CommitmentProof,
      connectionIdentifier: Identifier,
      connectionEnd: ConnectionEnd)
      => boolean
  ```

- `verifyChannelState` 验证在指定端口下存储在目标计算机上的指定通道端的通道状态的证明。

  ```
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

- `verifyPacketCommitment`验证在指定端口、指定通道和指定序列上的向外发送的数据包承诺的证明。

  ```
    type verifyPacketCommitment = (
      clientState: ClientState,
      height: uint64,
      prefix: CommitmentPrefix,
      proof: CommitmentProof,
      portIdentifier: Identifier,
      channelIdentifier: Identifier,
      sequence: uint64,
      commitment: bytes)
      => boolean
  ```

- `verifyPacketAcknowledgement`在指定的端口、指定的通道和指定的序列上验证传入数据包确认的证明。

  ```
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

- `verifyPacketAcknowledgementAbsence`验证在指定的端口、指定的通道和指定的序列中是否缺少传入数据包确认的证明。

  ```
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

- `verifyNextSequenceRecv`验证在指定端口上要从指定通道接收的下一个序列号的证明。

  ```
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

# Sub-protocols / 子协议

IBC 处理程序必须实现以下定义的函数。

## Identifier validation / 标识符验证

客户端存储在唯一的`标识符`前缀下。 ICS 02 不需要以特定方式生成客户端标识符，仅要求它们是唯一的即可。但是，如果需要，可以限制`标识符`的空间。可能需要验证函数`validateClientIdentifier` 。

```
type validateClientIdentifier = (id: Identifier) => boolean
```

如果没有提供以上函数，默认的`validateClientIdentifier` 会永远返回`true` 。

### Path-space / 路径空间

`clientStatePath` 接受一个`标识符`并返回一个存储特定客户端状态的`路径`。

```
function clientStatePath(id: Identifier): Path {
    return "clients/{id}/state"
}
```

`clientTypePath` 接受一个`标识符`并返回一个存储特定类型客户端的`路径` 。

```
function clientTypePath(id: Identifier): Path {
    return "clients/{id}/type"
}
```

共识状态必须分开存储，以便可以独立验证它们。

`ConsensusStatePath`接受一个`标识符`并返回一个`路径`来存储客户端的共识状态。

```go
function consensusStatePath(id: Identifier): Path {
    return "clients/{id}/consensusState"
}
```

### **Create / 创建**

通过特定的标识符和初始化共识状态调用`createClient`来创建一个客户端。

```go
function createClient(
  id: Identifier,
  clientType: ClientType,
  consensusState: ConsensusState) {
    abortTransactionUnless(validateClientIdentifier(id))
    abortTransactionUnless(privateStore.get(clientStatePath(id)) === null)
    abortSystemUnless(provableStore.get(clientTypePath(id)) === null)
    clientState = clientType.initialize(consensusState)
    privateStore.set(clientStatePath(id), clientState)
    provableStore.set(clientTypePath(id), clientType)
}
```

### Query / 查询

客户端共识状态和客户端内部状态能够通过标识符来进行查询。返回的客户端状态必须履行一个能够进行成员关系/非成员关系验证的接口。

```go
function queryClientConsensusState(id: Identifier): ConsensusState {
    return provableStore.get(consensusStatePath(id))
}
function queryClient(id: Identifier): ClientState {
    return privateStore.get(clientStatePath(id))
}
```

### Update / 更新

通过提交新的`标头`来完成客户端的更新。`标识符`用于指向逻辑将被更新的客户端状态。 当使用存存储`客户端状态`的有效性谓词和`共识状态`验证新的`报头`时，客户端必须相应地更新其内部状态，可能最终确定承诺根并更新存储的共识状态中的签名授权逻辑。

```go
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

### Misbehaviour / 不良行为

如果客户端检测到不当行为的证据，则可以向客户端发出警报，可能使先前有效的状态根无效并阻止将来的更新。

```go
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

# Example Implementation / 实现示例

一个有效性谓词示例是构建在运行单一运营者的共识算法的区块链上的，其中有效区块由这个运营者进行签名。在该区块链运行过程中可以更改运营者的签名密钥。

客户端特定的类型定义如下：

- `ConsensusState` 存储最新的区块高度和最新的公钥
- `Header` 包含一个高度、一个新的承诺根、一个操作者的签名以及可能还包括一个新的公钥
- `checkValidityAndUpdateState` 检查已经提交的区块高度是否是单调递增的以及签名是否正确，并更改内部状态
- `checkMisbehaviourAndUpdateState` 被用于检查两个相同块高但不同承诺根的标头，并更改内部状态

```go
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

//运营者执行算法去提交一个新的区块
function commit(
  commitmentRoot: CommitmentRoot,
  sequence: uint64,
  newPublicKey: Maybe<PublicKey>): Header {
    signature = privateKey.sign(commitmentRoot, sequence, newPublicKey)
    header = {sequence, commitmentRoot, signature, newPublicKey}
    return header
}

// 初始化函数由客户端类型来定义
function initialize(consensusState: ConsensusState): ClientState {
  return {
    frozen: false,
    pastPublicKeys: Set.singleton(consensusState.publicKey),
    verifiedRoots: Map.empty()
  }
}

// 有效性谓词函数由客户端类型来定义
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
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusState")
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
    path = applyPrefix(prefix, "connection/{connectionIdentifier}")
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
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, commitment, proof)
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

// 不良行为验证函数由客户端类型来定义
// 任何过去或现有的冗余签名会令客户端被冻结
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
