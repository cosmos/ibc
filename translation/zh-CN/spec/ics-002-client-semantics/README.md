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

该标准规定了实现区块链间通信协议的机器的共识算法必须满足的特性。 这些属性对于高层协议抽象中的有效和安全验证是必需的。 IBC 中用于验证另一台机器的共有记录和状态子组件的算法称为“有效性断言”，并将其与验证者认为正确的状态配对形成“轻客户端”（简称为“客户端”）。

该标准还规定了如何在规范的 IBC 处理程序中存储、注册和更新轻客户端。 所存储的客户端实例将由第三方参与者进行内省，例如，用户检查链的状态并确定是否发送 IBC 数据包。

### Motivation

In the IBC protocol, an actor, which may be an end user, an off-chain process, or a machine,
needs to be able to verify updates to the state of another machine
which the other machine's consensus algorithm has agreed upon, and reject any possible updates
which the other machine's consensus algorithm has not agreed upon. A light client is the algorithm
with which a machine can do so. This standard formalises the light client model and requirements,
so that the IBC protocol can easily integrate with new machines which are running new consensus algorithms
as long as associated light client algorithms fulfilling the listed requirements are provided.

Beyond the properties described in this specification, IBC does not impose any requirements on
the internal operation of machines and their consensus algorithms. A machine may consist of a
single process signing operations with a private key, a quorum of processes signing in unison,
many processes operating a Byzantine fault-tolerant consensus algorithm, or other configurations yet to be invented
— from the perspective of IBC, a machine is defined entirely by its light client validation & equivocation detection logic.
Clients will generally not include validation of the state transition logic in general
(as that would be equivalent to simply executing the other state machine), but may
elect to validate parts of state transitions in particular cases.

Clients could also act as thresholding views of other clients. In the case where
modules utilising the IBC protocol to interact with probabilistic-finality consensus algorithms
which might require different finality thresholds for different applications, one write-only
client could be created to track headers and many read-only clients with different finality
thresholds (confirmation depths after which state roots are considered final) could use that same state.

The client protocol should also support third-party introduction. Alice, a module on a machine,
wants to introduce Bob, a second module on a second machine who Alice knows (and who knows Alice),
to Carol, a third module on a third machine, who Alice knows but Bob does not. Alice must utilise
an existing channel to Bob to communicate the canonically-serialisable validity predicate for
Carol, with which Bob can then open a connection and channel so that Bob and Carol can talk directly.
If necessary, Alice may also communicate to Carol the validity predicate for Bob, prior to Bob's
connection attempt, so that Carol knows to accept the incoming request.

Client interfaces should also be constructed so that custom validation logic can be provided safely
to define a custom client at runtime, as long as the underlying state machine can provide an
appropriate gas metering mechanism to charge for compute and storage. On a host state machine
which supports WASM execution, for example, the validity predicate and equivocation predicate
could be provided as executable WASM functions when the client instance is created.

### 定义

- `get`, `set`, `Path`, 和 `Identifier` 在 [ICS 24](../ics-024-host-requirements)中被定义.

- `CommitmentRoot` 如同在 [ICS 23](../ics-023-vector-commitments)中被定义的那样，它必须提供为下游逻辑提供一种廉价方式，去验证键值对是否在特定高度的世界状态中存在。

- `共识状态` 是代表有效性述词的不透明类型。
    `ConsensusState` 必须能够验证相关共识算法所同意的状态更新。 它也必须以规范的方式实现可序列化，以便第三方（例如对应方的机器）可以检查特定机器是否存储了特定的共识状态。 它最终必须由它所针对的状态机进行自省，以便状态机可以在过去的高度查找其自己的共识状态。

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

`共识状态` 是一个由客户端类型来定义的不透明数据结构，被有效性谓词用来验证新的区块提交和根状态。该结构可能包含共识过程产生的最后一次提交，包括签名和验证者集合元数据。

`共识状态` 必须由一个 `共识`实例生成，该实例为每个 `共识状态`分配唯一的高度（这样，每个高度恰好具有一个关联的共识状态）。如果没有一致的承诺根，则同一链上的两个`共识状态`不应具有相同的高度。此类事件称为“存疑行为”，必须归类为不当行为。 如果发生这种情况，则应生成并提交证明，以便可以冻结客户端，并根据需要使先前的状态根无效。

链的 `共识状态` 必须可以被规范地序列化，以便其他链可以检查存储的共识状态是否与另一个共识状态相等（请参见 [ICS 24](../ics-024-host-requirements) 了解密钥空间表）。

```typescript
type ConsensusState = bytes
```

`共识状态` 必须存储在下面定义的特定密钥下，这样其他链可以验证一个特定的共识状态是否已存储。

#### 报头

`报头` 是由客户端类型定义的不透明数据结构，它提供信息以用来更新`共识状态`.
。可以将报头提交给关联的客户端以更新存储的`共识状态`. 。 报头可能包含高度、证明、承诺根，并可能更新有效性谓词。

```typescript
type Header = bytes
```

#### 共识

`共识` 是一个 `报头` 生成函数，它利用之前的
`共识状态` 和消息并返回结果。

```typescript
type Consensus = (ConsensusState, [Message]) => Header
```

### 区块链

区块链是一个生成有效`标头`的共识算法。它由创世文件`共识状态` 生成带有任意消息的唯一的标头列表。

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

#### 有效性谓词

一个有效性谓词是由客户端类型定义的一个不透明函数，用与根据当前`共识状态`来验证 `标头` 。使用有效性谓词应该比给定父`标头` 和网络消息列表的完全共识重放算法拥有高得多的计算效率。

有效性谓词和客户端状态更新逻辑是绑定在一个单独的 `checkValidityAndUpdateState`类型中的，它的定义如下：

```typescript
type checkValidityAndUpdateState = (Header) => Void
```

`checkValidityAndUpdateState` 必须在输入非有效标头的情况下抛出一个异常。

如果给定的标头有效，客户端必须改变内部状态以存储立即确认的状态根，以及更新必要的签名权限跟踪（例如对验证者集合的更新）以供后续对有效性谓词的调用。

#### Misbehaviour predicate

一个非有效性谓词是由客户端类型定义的不透明函数，用于检查数据是否对共识协议的构成违规。这可能是出现两个拥有不同状态根但在同一个区块高度的签名的标头、一个包含无效状态转换签名的标头或这其他由共识算法定义的不良行为的证据。

非有效性谓词和客户端状态更新逻辑是绑定在一个单独的`checkMisbehaviourAndUpdateState`类型中的，它的定义如下：

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

通过提交新的`标头`来完成客户端的更新。`标识符`用于指向逻辑将被更新的客户端状态。 当使用存存储`客户端状态`的有效性谓词和`共识状态`验证新的`报头`时，客户端必须相应地更新其内部状态，可能最终确定承诺根并更新存储的`共识状态`中的签名授权逻辑。

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

一个有效性谓词示例是构建在运行单一运营者的共识算法的区块链上的，其中有效区块由这个运营者进行签名。在该区块链运行过程中可以更改运营者的签名密钥。

客户端特定的类型定义如下：

- `ConsensusState` 存储最新的区块高度和最新的公钥
- `Header`包含一个高度、一个新的承诺根、一个操作者的签名以及可能还包括一个新的公钥
- `checkValidityAndUpdateState` 检查已经提交的区块高度是否是单调递增的以及签名是否正确，并更改内部状态
- `checkMisbehaviourAndUpdateState` 被用于检查两个相同块高但不同承诺根的标头，并更改内部状态

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
