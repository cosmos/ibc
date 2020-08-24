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

client 関連の型と機能は [ICS 2](../ics-002-client-semantics) で定義されているとおりです。

Commitment proof 関連の型と機能は [ICS 23](../ics-023-vector-commitments) で定義されているとおりです。

`Identifier` および他の host state machine の要件は、[ICS 24](../ics-024-host-requirements) で定義されている通りです。識別子は、必ずしも人間が読める名前であることを意図しているわけではありません（そして、識別子の占有や競合を防止するためとも限りません）。

opening handshake プロトコルは、各 chain が他の chain 上の connection を参照するために用いる識別子を検証できるようにし、各 chain 上の module が他の chain 上の参照について判断できるようにします。

本仕様で言及されている *actor* とは、datagram を実行できる entity で、計算 / ストレージ (gas または同様の機構を介して) を負担していますが、それ以外は信頼されていません。可能な actor は以下の通りです。

- アカウントの鍵で署名するエンドユーザー
- 自律的に、または別のトランザクションに応答して動作する on-chain smart contract
- 別のトランザクションに応答、またはスケジュールされた方法で動作する on-chain module

### 期待される性質

- blockchain を実装することで、信頼されていない actor が connection を開いたり更新したりすることを安全に許可できるようにしなければなりません。

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
- datagram を再生することで、他の chain 上に悪意を持って他の connection オブジェクトを作成することはできません。

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
- `counterpartyPrefix` には、この connection の相手 chain の状態検証に使用される prefix が含まれます。chain は、relayer が connection prefix を照会できるようにエンドポイントを公開する必要があります。指定されていない場合は、`「ibc」` のデフォルトの `counterpartyPrefix` が使用されるべきです。
- `clientIdentifier` は、この connection に関連付けられた client を示しています。
- `counterpartyClientIdentifier` は、この connection の<br>相手 chain 上の client を示しています。
- `version` は opaque string で、この connection を利用する channel または packet のエンコーディングまたはプロトコルを決定するために利用できます。指定されていない場合は、デフォルトの`version` の `""` が使用されるべきです。

### パスの保存

connection パスは一意の識別子の下に保存されます。

```typescript
function connectionPath(id: Identifier): Path {
    return "connections/{id}"
}
```

client から一連の connetion への逆マッピング (clientを使用したすべての connetion を検索するために利用される) は、client 毎に一意の prefix の下に保存されます。

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

ヘルパー関数は、client が提供する検証関数に connection に関連する `CommitmentPrefix` を渡すための connection によって定義されます。この仕様の他の部分では、これらの関数は、client 上の検証関数を直接呼び出すのではなく、他の chain の状態を確認するために使用されなければなりません。

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

opening handshake は4つの datagram を定義します: *ConnOpenInit*、*ConnOpenTry*、*ConnOpenAck*、*ConnOpenConfirm*。

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

このサブプロトコルは、スパム防止策等を除いて、許可制である必要はありません。

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
