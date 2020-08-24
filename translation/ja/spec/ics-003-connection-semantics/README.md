---
ics: 3
title: Connection Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 24
required-by: 4, 25
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## 概要

この標準化文書は、IBC *connection* の抽象化について記述します。2つの別々の chain 上の2つの stateful object (*connection ends*)は、それぞれが他方の chain の light client に関連付けられており、cross-chain sub-state の検証と（channel を介した）packet の関連付けを一緒に促進します。2つの chain 間の connection を安全に確立するためのプロトコルが記述されています。

### 動機

IBCプロトコルの中心は、packet の *認可* と *順序付け* の基準を提供します。それぞれ、packet が送信ブロックチェーン上でコミットされたこと（および、エスクロー・トークンなどのような状態遷移に応じて実行されたこと）、特定の順序で正確に一度だけコミットされ、同じ順序で正確に一度だけ配信されることを保証します。この標準規格で規定されている *connection* の抽象化は、[ICS 2](../ics-002-client-semantics) で規定されている *client* の抽象化と合わせて、 IBC の *認可* 基準を定義します。順序付けの基準は、 [ICS 4](../ics-004-channel-and-packet-semantics)で説明されています。

### 定義

Client-related types & functions are as defined in [ICS 2](../ics-002-client-semantics).

Commitment proof related types & functions are defined in [ICS 23](../ics-023-vector-commitments)

`Identifier` and other host state machine requirements are as defined in [ICS 24](../ics-024-host-requirements). The identifier is not necessarily intended to be a human-readable name (and likely should not be, to discourage squatting or racing for identifiers).

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

An *actor*, as referred to in this specification, is an entity capable of executing datagrams who is paying for computation / storage (via gas or a similar mechanism) but is otherwise untrusted. Possible actors include:

- アカウントの鍵で署名するエンドユーザー
- 自律的に、または別のトランザクションに応答して動作する on-chain smart contract
- 別のトランザクションに応答、またはスケジュールされた方法で動作する on-chain module

### 期待される性質

- Implementing blockchains should be able to safely allow untrusted actors to open and update connections.

#### 確立前

connection 確立の前に:

- cross-chain sub-state を検証できないため、それ以上のIBCサブプロトコルは動作すべきではありません。
- 開始中の（connection を作成する）actor は、接続先 chain の初期 consensus state と接続元 chain の初期 consensus state を指定できなければなりません（暗黙的に、例えばトランザクションを送信することによって）。

#### Handshake している間

一度 negotiation handshake が開始されると:

- 適切な handshake datagram のみを順番に実行することができます。
- 3番目の chain は、2つの handshake 中の chain の一つとして成りすますことはできません。

#### 確立後

一度 negotiation handshake が完了すると:

- 両方の chain で作成された connection オブジェクトには、connection を開始する actor が指定した consensus stateが含まれています。
- No other connection objects can be maliciously created on other chains by replaying datagrams.

## 技術仕様

### データ構造

この ICS は `ConnectionState` と `ConnectionEnd` の型を定義しています:

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

- `state` は connection end の現在の状態を表しています。
- `counterpartyConnectionIdentifier` は、この connection の相手 chain 上の connection end を示しています。
- The `counterpartyPrefix` field contains the prefix used for state verification on the counterparty chain associated with this connection. Chains should expose an endpoint to allow relayers to query the connection prefix. If not specified, a default `counterpartyPrefix` of `"ibc"` should be used.
- The `clientIdentifier` field identifies the client associated with this connection.
- The `counterpartyClientIdentifier` field identifies the client on the counterparty chain associated with this connection.
- The `version` field is an opaque string which can be utilised to determine encodings or protocols for channels or packets utilising this connection. If not specified, a default `version` of `""` should be used.

### パスの保存

connection パスは一意の識別子の下に保存されます。

```typescript
function connectionPath(id: Identifier): Path {
    return "connections/{id}"
}
```

A reverse mapping from clients to a set of connections (utilised to look up all connections using a client) is stored under a unique prefix per-client:

```typescript
function clientConnectionsPath(clientIdentifier: Identifier): Path {
    return "clients/{clientIdentifier}/connections"
}
```

### ヘルパー関数

`addConnectionToClient` は、client に関連付けられた一連の connection に connection 識別子を追加するために使用されます。

```typescript
function addConnectionToClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier))
    conns.add(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier), conns)
}
```

Helper functions are defined by the connection to pass the `CommitmentPrefix` associated with the connection to the verification function provided by the client. In the other parts of the specifications, these functions MUST be used for introspecting other chains' state, instead of directly calling the verification functions on the client.

```typescript
function verifyClientConsensusState(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyClientConsensusState(connection, height, connection.counterpartyPrefix, proof, clientIdentifier, consensusStateHeight, consensusState)
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

function verifyPacketData(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes) {
    client = queryClient(connection.clientIdentifier)
    return client.verifyPacketData(connection, height, connection.counterpartyPrefix, proof, portIdentifier, channelIdentifier, data)
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

function getTimestampAtHeight(
  connection: ConnectionEnd,
  height: uint64) {
    client = queryClient(connection.clientIdentifier)
    return client.queryConsensusState(height).getTimestamp()
}
```

### サブプロトコル

本ICSは、opening handshake サブプロコトルを定義する。一度 connection を開くと閉じることはできず、識別子を再割り当てすることもできません (これは packet のリプレイや認可の混乱を防ぐためです)。

Headerの追跡と不正行為の検出は [ICS 2](../ics-002-client-semantics) で定義されています。

![State Machine Diagram](state.png)

#### 識別子の検証

connection は一意の`Identifier`接頭辞の下に保存されます。検証関数`validateConnectionIdentifier`を提供してもよいです。

```typescript
type validateConnectionIdentifier = (id: Identifier) => boolean
```

もし提供されない場合は、デフォルトの `validateClientIdentifier` 関数は常に `true` を返します。

#### バージョニング

handshake プロセスの間に、connection の両端は、その接続に関連付けられたバージョンのバイト文字列に合意します。現時点では、このバージョンバイト文字列の内容はIBCコアプロトコルからは不透明です。将来的には、どのような種類のチャンネルが問題の connection を利用できるか、 または <br> channel 関連の datagram がどのようなエンコーディングフォーマットを 使用するかを示すために使用されるかもしれません。現時点では、host state machineはバージョンデータを利用して、エンコーディング、 優先度、またはIBCの上のカスタムロジックに関連する connection 固有の メタデータを交渉してもよいです。

Host state machineは、バージョンデータを安全に無視するか、空の文字列を指定してもよいです。

実装は、サポートするバージョンのリストを、優先順位の降順でランク付けして返す関数 `getCompatibleVersions` を定義しなければなりません。

```typescript
type getCompatibleVersions = () => []string
```

実装は、相手が提案するバージョンのリストからバージョンを選択するために、関数 `pickVersion` を定義しなければなりません。

```typescript
type pickVersion = ([]string) => string
```

#### Opening Handshake

opening handshake サブプロトコルは、お互いの2つのチェーンの consensus stateを初期化するのに役立ちます。

The opening handshake defines four datagrams: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, and *ConnOpenConfirm*.

正しいプロトコルの実行は以下のような流れになります（すべての呼び出しは ICS 25 に従った module を介して行われることに注意してください）。

Initiator | Datagram | 実行 chain | 事前 state (A, B) | 事後 state (A, B)
--- | --- | --- | --- | ---
Actor | `ConnOpenInit` | A | (none, none) | (INIT, none)
Relayer | `ConnOpenTry` | B | (INIT, none) | (INIT, TRYOPEN)
Relayer | `ConnOpenAck` | A | (INIT, TRYOPEN) | (OPEN, TRYOPEN)
Relayer | `ConnOpenConfirm` | B | (OPEN, TRYOPEN) | (OPEN, OPEN)

サブプロトコルを実装している2つの chain 間の opening handshake の終わりには、以下の特性があります。

- 各 chain は、開始 actor が最初に指定した通りに、お互いの正しい consensus state を持っています。
- 各 chain は、他の chain の識別子を知っており、それに同意しています。

This sub-protocol need not be permissioned, modulo anti-spam measures.

*ConnOpenInit* chain A の connection 試行を初期化します。

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

*ConnOpenTry*は chain Aの connection 試行の通知を chain B に中継します(このコードは chain Bで実行されます)。

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
    abortTransactionUnless(consensusHeight < getCurrentHeight())
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{INIT, desiredIdentifier, getCommitmentPrefix(), counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersions}
    version = pickVersion(counterpartyVersions)
    connection = ConnectionEnd{TRYOPEN, counterpartyConnectionIdentifier, counterpartyPrefix,
                               clientIdentifier, counterpartyClientIdentifier, version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofInit, counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
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
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenAck* は、chain B から chain A への connection 開始試行の受諾を中継します（このコードは chain Aで実行されます）。

```typescript
function connOpenAck(
  identifier: Identifier,
  version: string,
  proofTry: CommitmentProof,
  proofConsensus: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(consensusHeight < getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === INIT || connection.state === TRYOPEN)
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{TRYOPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier, connection.clientIdentifier,
                             version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofTry, connection.counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, connection.counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
    connection.state = OPEN
    abortTransactionUnless(getCompatibleVersions().indexOf(version) !== -1)
    connection.version = version
    provableStore.set(connectionPath(identifier), connection)
}
```

*ConnOpenConfirm* は、chain A から chain B への connection が開かれるのを確認し、その後、両方の chain で connection が開かれます（このコードは chain B で実行されます）。

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

#### Querying

connection は `queryConnection` と識別子で問い合わせることができます。

```typescript
function queryConnection(id: Identifier): ConnectionEnd | void {
    return provableStore.get(connectionPath(id))
}
```

特定の client に関連付けられた connection は、client 識別子で `queryClientConnections` を使用して問い合わせることができます。

```typescript
function queryClientConnections(id: Identifier): Set<Identifier> {
    return privateStore.get(clientConnectionsPath(id))
}
```

### 特性と不変条件

- connection 識別子は先着順です。一度 connection が処理されると、2つの chain 間に一意の識別子ペアが存在します。
- connection の handshake は、他の blockchain のIBCハンドラーが介在することはできません。

## 後方互換性

適用されません。

## 前方互換性

この ICS の将来のバージョンでは、opening handshake にバージョンの交渉が含まれる予定です。connection が確立され、バージョンが交渉されると、将来のバージョン更新は ICS 6 に基づいて交渉できます。

consensus state は、connection 確立時に選択された consensus プロトコルによって定義された`updateConsensusState`関数によってのみ更新することができます。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

本文書の一部は [以前のIBC仕様](https://github.com/cosmos/cosmos-sdk/tree/master/docs/spec/ibc) に触発されました。

2019年3月30日 - 最初のドラフトを提出

2019年5月17日 - ドラフトが確定

2019年7月29日 - client に関連付けられた接続セットを追跡するための改訂

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
