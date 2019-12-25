---
ics: '2'
title: Client Semantics
stage: draft
category: IBC/TAO
kind: interface
requires: 23, 24
required-by: '3'
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: '2019-02-25'
modified: '2019-08-25'
---

## 概览

该标准规定了实现区块链间通信协议的状态机的共识算法必须满足的特性。 这些属性对高层协议抽象中的有效安全的验证而言是必需的。IBC 中用于验证另一台状态机的共识记录子组件及状态子组件的算法称为“有效性断言”，并将其与验证者认为正确的状态配对形成“轻客户端”（简称为“客户端”）。

该标准还规定了如何在规范的 IBC 处理程序中存储、注册和更新轻客户端。 所存储的客户端实例将由第三方参与者进行内省，例如，用户检查链的状态并确定是否发送 IBC 数据包。

### 动机

在 IBC 协议中，参与者可以是终端用户、链下进程或状态机，这需要能够对另一台状态机共识算法认同的状态更新进行验证，以及拒绝其共识算法不认同的任何潜在状态更新。轻客户端是状态机可以执行的算法。该标准规范了轻客户端的模型和要求，因此，只要提供满足所列要求的相关轻客户端算法，IBC 协议就可以轻松地与运行新共识算法的新状态机集成。

除了本规范中描述的属性外，IBC 对状态机的内部操作及其共识算法没有任何要求。 从 IBC 的角度来看，一台状态机可能包括一个私钥签名流程、统一签名流程仲裁、多个运行拜占庭容错共识算法的流程或其他尚未发明的配置——从 IBC 的角度来看，一个状态机完全由其轻客户端的验证和自相矛盾行为检测逻辑来定义。
客户通常将通常不包括对状态转换逻辑的验证（因为这将与其他状态机无异），但是在特定情况下，客户可以选择验证部分状态转换。

客户端还可以为其他客户端提供阈值视角。 如果模块利用 IBC 协议与概率性最终一致性算法（probabilistic-finality）进行交互，对于不同的应用可能需要不同的最终确定性阈值，那么可以创建一个只允许写入的客户端来跟踪不同区块头，其状态供其他具有不同最终确定性阀值的只读客户端来使用。

客户端协议还应该支持第三方引接。 Alice是一台状态机上的一个模块，希望将 Bob（第二台状态机上的第二个模块，与 Alice 彼此已知）介绍给Carol（第三台状态机上的第三个模块，Alice 知道但 Bob 却不知道）。 Alice 必须利用现有的通道与鲍勃通信，以向 Carol 传递规范串行化的合法性判定式，然后 Bob 可以与之建立连接和通道，以便 Bob 和 Carol 可以直接通信。
如有必要，在 Bob 进行连接尝试之前，Alice 还可以向 Carol 传达 Bob 的合法性判定式，使得 Carol 获悉并接受引接请求。

客户端接口也应该被构建为：只要底层状态机可以提供恰当的计算和存储燃料的计量机制，那么在运行时可以安全地提供定制验证逻辑来自定义一个轻客户端。例如，在支持运行 WASM 的状态机主机上，在创建客户端实例时，可以提供合法性判定式和矛盾行为判定式作为可执行的 WASM 函数。

### 定义

- `get`, `set`, `Path`, 和 `Identifier` 在 [ICS 24](../ics-024-host-requirements) 中被定义.

- `CommitmentRoot` 如同在 [ICS 23](../ics-023-vector-commitments) 中被定义的那样，它必须提供为下游逻辑提供一种廉价方式，去验证键值对是否在特定高度的世界状态中存在。

- `共识状态` 是代表合法性判定式的不透明类型。
    `ConsensusState` 必须能够验证相关共识算法所同意的状态更新。 它也必须以规范的方式实现可序列化，以便第三方（例如对应方的状态）可以检查特定状态机是否存储了特定的共识状态。 它最终必须由它所针对的状态机进行自省，以便状态机可以在过去的高度查找其自己的共识状态。

- `客户端状态` 是代表一个客户端状态的不透明类型。
    `ClientState` 必须公开查询函数，以验证处于特定高度的状态下键/值对的成员身份或非成员身份，并且能够提取当前的共识状态.

### Desired Properties

Light clients must provide a secure algorithm to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level abstractions will then be able to verify
sub-components of the state with the `CommitmentRoot`s stored in the `ConsensusState`, which are
guaranteed to have been committed by the other chain's consensus algorithm.

Validity predicates are expected to reflect the behaviour of the full nodes which are running the
corresponding consensus algorithm. Given a `ConsensusState` and a list of messages, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects it, then the light client MUST also reject it.

Light clients are not replaying the whole message transcript, so it is possible under cases of
consensus misbehaviour that the light clients' behaviour differs from the full nodes'.
In this case, a misbehaviour proof which proves the divergence between the validity predicate
and the full node can be generated and submitted to the chain so that the chain can safely deactivate the
light client, invalidate past state roots, and await higher-level intervention.

## Technical Specification

This specification outlines what each *client type* must define. A client type is a set of definitions
of the data structures, initialisation logic, validity predicate, and misbehaviour predicate required
to operate a light client. State machines implementing the IBC protocol can support any number of client
types, and each client type can be instantiated with different initial consensus states in order to track
different consensus instances. In order to establish a connection between two machines (see [ICS 3](../ics-003-connection-semantics)),
the machines must each support the client type corresponding to the other machine's consensus algorithm.

Specific client types shall be defined in later versions of this specification and a canonical list shall exist in this repository.
Machines implementing the IBC protocol are expected to respect these client types, although they may elect to support only a subset.

### 数据结构

#### 共识状态

`共识状态` 是一个由客户端类型来定义的不透明数据结构，被合法性判定式用来验证新的区块提交和根状态。该结构可能包含共识过程产生的最后一次提交，包括签名和验证者集合元数据。

`共识状态` 必须由一个 `共识`实例生成，该实例为每个 `共识状态`分配唯一的高度（这样，每个高度恰好具有一个关联的共识状态）。如果没有一致的承诺根，则同一链上的两个`共识状态`不应具有相同的高度。此类事件称为“自相矛盾行为”，必须归类为不当行为。 如果发生这种情况，则应生成并提交证明，以便可以冻结客户端，并根据需要使先前的状态根无效。

链的 `共识状态` 必须可以被规范地序列化，以便其他链可以检查存储的共识状态是否与另一个共识状态相等（请参见 [ICS 24](../ics-024-host-requirements) 了解密钥空间表）。

```typescript
type ConsensusState = bytes
```

`共识状态` 必须存储在下面定义的特定密钥下，这样其他链可以验证一个特定的共识状态是否已存储。

#### 区块头

`区块头` 是由客户端类型定义的不透明数据结构，它提供信息以用来更新`共识状态`。可以将区块头提交给关联的客户端以更新存储的`共识状态` 。区块头可能包含高度、证明、承诺根，并可能更新合法性判定式。

```typescript
type Header = bytes
```

#### 共识

`共识` 是一个 `区块头` 生成函数，它利用之前的
`共识状态` 和消息并返回结果。

```typescript
type Consensus = (ConsensusState, [Message]) => Header
```

### 区块链

区块链是一个生成有效`区块头`的共识算法。它由创世文件`共识状态` 生成带有任意消息的唯一的区块头列表。

`区块链` 被定义为

```typescript
interface Blockchain {
  genesis: ConsensusState
  consensus: Consensus
}
```

where

- `Genesis` is the genesis `ConsensusState`
- `Consensus` is the header generating function

The headers generated from a `Blockchain` are expected to satisfy the following:

1. Each `Header` MUST NOT have more than one direct child

- Satisfied if: finality & safety
- Possible violation scenario: validator double signing, chain reorganisation (Nakamoto consensus)

1. Each `Header` MUST eventually have at least one direct child

- Satisfied if: liveness, light-client verifier continuity
- Possible violation scenario: synchronised halt, incompatible hard fork

1. Each `Header`s MUST be generated by `Consensus`, which ensures valid state transitions

- Satisfied if: correct block generation & state machine
- Possible violation scenario: invariant break, super-majority validator cartel

Unless the blockchain satisfies all of the above the IBC protocol
may not work as intended: the chain can receive multiple conflicting
packets, the chain cannot recover from the timeout event, the chain can
steal the user's asset, etc.

The validity of the validity predicate is dependent on the security model of the
`Consensus`. For example, the `Consensus` can be a proof of authority with
a trusted operator, or a proof of stake but with
insufficient value of stake. In such cases, it is possible that the
security assumptions break, the correspondence between `Consensus` and
the validity predicate no longer exists, and the behaviour of the validity predicate becomes
undefined. Also, the `Blockchain` may not longer satisfy
the requirements above, which will cause the chain to be incompatible with the IBC
protocol. In cases of attributable faults, a misbehaviour proof can be generated and submitted to the
chain storing the client to safely freeze the light client and
prevent further IBC packet relay.

#### 合法性判定式

一个合法性判定式是由客户端类型定义的一个不透明函数，用与根据当前`共识状态`来验证 `区块头` 。使用合法性判定式应该比给定父`区块头` 和网络消息列表的完全共识重放算法拥有高得多的计算效率。

合法性判定式和客户端状态更新逻辑是绑定在一个单独的 `checkValidityAndUpdateState`类型中的，它的定义如下：

```typescript
type checkValidityAndUpdateState = (Header) => Void
```

`checkValidityAndUpdateState` 必须在输入非有效区块头的情况下抛出一个异常。

如果给定的区块头有效，客户端必须改变内部状态以存储立即确认的状态根，以及更新必要的签名权限跟踪（例如对验证者集合的更新）以供后续对合法性判定式的调用。

#### 不良行为判定式

一个不良行为判定式是由客户端类型定义的不透明函数，用于检查数据是否对共识协议的构成违规。这可能是出现两个拥有不同状态根但在同一个区块高度的签名的区块头、一个包含无效状态转换签名的区块头或这其他由共识算法定义的不良行为的证据。

不良行为判定式和客户端状态更新逻辑是绑定在一个单独的`checkMisbehaviourAndUpdateState`类型中的，它的定义如下：

```typescript
type checkMisbehaviourAndUpdateState = (bytes) => Void
```

`checkMisbehaviourAndUpdateState` 在给定证据无效的情况下必须抛出一个异常。

如果一个不良行为是有效的，客户端还必须根据不良行为的性质去更改内部状态，来标记先前认为有效的区块高度为无效。

#### 客户端状态

客户端状态是一个由客户端类型定义的不透明数据结构。它或将保留任意的内部状态去追踪已经被验证过的块根和发生过的不良行为。

轻客户端是一种不透明的表现形式——不同的共识算法可以定义不同的轻客户端更新算法，但是轻客户端必须对 IBC 处理程序公开暴露通用查询功能集合。

```typescript
type ClientState = bytes
```

客户端类型必须定义一个方法用提供的共识状态去初始化一个客户端状态：

```typescript
type initialize = (state: ConsensusState) => ClientState
```

#### 承诺根

`承诺根` 是根据 [ICS 23](../ics-023-vector-commitments)
由客户端类型定义的不透明数据结构。它用于验证处于特定最终高度（必须与特定承诺根相关联）的状态中是否存在特定键/值对。

#### 状态验证

状态类型必须定义一系列函数去对客户端追踪的状态机的内部状态进行验证。内部实现细节可能存在差异（例如，一个回路客户端可以直接读取状态信息且不需要提供证明）。

##### 所需函数：

`verifyClientConsensusState` 验证存储在指定状态机上的特定客户端的共识状态的证明。

```typescript
type verifyClientConsensusState = (
  clientState: ClientState,
  height: uint64,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState)
  => boolean
```

`verifyConnectionState` 验证在指定状态机上存储的特定连接端的连接状态证明。

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

`verifyChannelState` 验证在指定端口下存储在目标计算机上的指定通道端的通道状态的证明。

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

`verifyPacketCommitment` 验证在指定端口、指定通道和指定序列上的向外发送的数据包承诺的证明。

```typescript
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

`verifyPacketAcknowledgement` 在指定的端口、指定的通道和指定的序列上验证传入数据包确认的证明。

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

`verifyPacketAcknowledgementAbsence` 验证在指定的端口、指定的通道和指定的序列中是否缺少传入数据包确认的证明。

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

`verifyNextSequenceRecv` 验证在指定端口上要从指定通道接收的下一个序列号的证明。

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

##### Implementation strategies

###### Loopback

A loopback client of a local machine merely reads from the local state, to which it must have access.

###### Simple signatures

A client of a solo machine with a known public key checks signatures on messages sent by that local machine,
which are provided as the `Proof` parameter. The `height` parameter can be used as a replay protection nonce.

Multi-signature or threshold signature schemes can also be used in such a fashion.

###### Proxy clients

Proxy clients verify another (proxy) machine's verification of the target machine, by including in the
proof first a proof of the client state on the proxy machine, and then a secondary proof of the sub-state of
the target machine with respect to the client state on the proxy machine. This allows the proxy client to
avoid storing and tracking the consensus state of the target machine itself, at the cost of adding
security assumptions of proxy machine correctness.

###### Merklized state trees

For clients of state machines with Merklized state trees, these functions can be implemented by calling `verifyMembership` or `verifyNonMembership`, using a verified Merkle
root stored in the `ClientState`, to verify presence or absence of particular key/value pairs in state at particular heights in accordance with [ICS 23](../ics-023-vector-commitments).

```typescript
type verifyMembership = (ClientState, uint64, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, uint64, CommitmentProof, Path) => boolean
```

### 子协议

IBC 处理程序必须实现以下定义的函数。

#### 标识符验证

客户端存储在唯一的`标识符`前缀下。 ICS 02 不需要以特定方式生成客户端标识符，仅要求它们是唯一的即可。但是，如果需要，可以限制`标识符`的空间。可能需要验证函数`validateClientIdentifier` 。

```typescript
type validateClientIdentifier = (id: Identifier) => boolean
```

如果没有提供以上函数，默认的`validateClientIdentifier`会永远返回`true` 。

#### 路径空间

`clientStatePath` 接受一个`标识符`并返回一个存储特定客户端状态的`路径`。

```typescript
function clientStatePath(id: Identifier): Path {
    return "clients/{id}/state"
}
```

`clientTypePath` 接受一个`标识符`并返回一个存储特定类型客户端的`路径` 。

```typescript
function clientTypePath(id: Identifier): Path {
    return "clients/{id}/type"
}
```

共识状态必须分开存储，以便可以独立验证它们。

`ConsensusStatePath`接受一个`标识符`并返回一个`路径`来存储客户端的共识状态。

```typescript
function consensusStatePath(id: Identifier): Path {
    return "clients/{id}/consensusState"
}
```

##### Utilising past roots

To avoid race conditions between client updates (which change the state root) and proof-carrying
transactions in handshakes or packet receipt, many IBC handler functions allow the caller to specify
a particular past root to reference, which is looked up by height. IBC handler functions which do this
must ensure that they also perform any requisite checks on the height passed in by the caller to ensure
logical correctness.

#### 创建

通过特定的标识符和初始化共识状态调用`createClient`来创建一个客户端。

```typescript
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

#### 查询

客户端共识状态和客户端内部状态能够通过标识符来进行查询。返回的客户端状态必须履行一个能够进行成员关系/非成员关系验证的接口。

```typescript
function queryClientConsensusState(id: Identifier): ConsensusState {
    return provableStore.get(consensusStatePath(id))
}
```

```typescript
function queryClient(id: Identifier): ClientState {
    return privateStore.get(clientStatePath(id))
}
```

#### 更新

通过提交新的`区块头`来完成客户端的更新。`标识符`用于指向逻辑将被更新的客户端状态。 当使用存存储`客户端状态`的合法性判定式和`共识状态`验证新的`区块头`时，客户端必须相应地更新其内部状态，可能最终确定承诺根并更新存储的`共识状态`中的签名授权逻辑。

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

如果客户端检测到不当行为的证据，则可以向客户端发出警报，可能使先前有效的状态根无效并阻止将来的更新。

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

一个合法性判定式示例是构建在运行单一运营者的共识算法的区块链上的，其中有效区块由这个运营者进行签名。在该区块链运行过程中可以更改运营者的签名密钥。

客户端特定的类型定义如下：

- `ConsensusState` 存储最新的区块高度和最新的公钥
- `Header`包含一个高度、一个新的承诺根、一个操作者的签名以及可能还包括一个新的公钥
- `checkValidityAndUpdateState` 检查已经提交的区块高度是否是单调递增的以及签名是否正确，并更改内部状态
- `checkMisbehaviourAndUpdateState` 被用于检查两个相同块高但不同承诺根的区块头，并更改内部状态

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

// 运营者执行算法去提交一个新的区块
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

// 合法性判定式函数由客户端类型来定义
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

### Properties & Invariants

- Client identifiers are immutable & first-come-first-serve. Clients cannot be deleted (allowing deletion would potentially allow future replay of past packets if identifiers were re-used).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

New client types can be added by IBC implementations at-will as long as they conform to this interface.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Mar 5, 2019 - Initial draft finished and submitted as a PR

May 29, 2019 - Various revisions, notably multiple commitment-roots

Aug 15, 2019 - Major rework for clarity around client interface

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
