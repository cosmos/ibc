---
ics: 2
title: Client Semantics
stage: draft
category: IBC/TAO
kind: interface
requires: 23, 24
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2020-01-13
---

## 概要

この規格は、IBC プロトコルを実装する Machine の Consensus アルゴリズムが満たすべき特性を規定します。これらの特性はより上位レベルのプロトコルの抽象化において効率的で安全な検証を行うために必要となります。IBC では、他の Machine の consensus 転写物 と state のサブコンポーネントを検証するために利用されるアルゴリズムを「検証用関数（validity predicate）」と呼び、検証者が正しいと仮定した状態とペアにすることで「light client」（しばしば「client」と短縮される）を形成します。

この規格はまた、light client がどのように正規の IBC handler に格納、登録、更新されるかについても規定します。保存された client インスタンスは、chain の state を検査して IBC packet を送信するかどうかを決定する user のような、第三者の actor によって検査が可能です。

### 動機

IBC プロトコルでは、actor（エンドユーザー、オフチェーンプロセス、または machine）は、他の machine が自らの consensus アルゴリズムによって合意した state の更新を検証し、また合意の無いどのような state の更新も拒否できる必要があります。light client は machine がそのように動作するために用いるアルゴリズムです。この規格では light client モデルと要件を定式化します。それによって IBC プロトコルは、新しい consensus アルゴリズムを実行している新しい machine が 要件を満たした light client アルゴリズムを備えている限り、その machine と容易に統合することができます。

IBC は本仕様で記載される特性を越えた、いかなる machine の内部動作や consensus アルゴリズムに対する要件も課しません。machine は単一のプロセスが秘密鍵を用いて署名操作を行ってもよいし、複数のプロセスが一体となって署名を行ってもよいし、多数のプロセスがビザンチン障害耐性のある consensus アルゴリズムを運用してもよいし、まだ発明されていないその他の構成であってもよいです。 — IBC の観点に立つと、machine は light client 検証と状態不一致を検出するロジックによって完全に定義されています。client は一般的に状態遷移ロジックの検証を含んでいませんが（それは単に他の state machine を実行することと同等であるため）、個別のケースで状態遷移の一部を検証するようにしてもよいかもしれません。

client は 他の client の閾値ビューとしても機能します。IBC プロトコルを利用している module が、異なるアプリケーションに対して異なる finality の敷値を要求する確率的な finality consensus アルゴリズムとやりとりする場合、それらの header を追跡するために1つの書き込み専用 client を作成し、その同一 state を異なる finality 閾値を持つ多数の読み取り専用 client が使用することができます。

client プロトコルは、第三者の紹介もサポートしなければなりません。machine 上の module である Alice は、Alice が知っている（Aliceを知っている）2つ目の machine 上の module であるBobを、Alice は知っているが Bob は知らない3つ目の machine 上の module である Carol に紹介したいとします。Alice は Bob への既存の channel を利用して、Carol の正準的にシリアライズ可能な検証関数を伝える必要があります。これによって Bob は connection と channel を開いて、Carol と直接通信できるようになります。必要に応じて、Alice は Bob の connection 試行に先立ち Carol に Bob の検証用関数を伝えることもできます。これによって Carol は 受信リクエストの承認方法を得ます。

client インタフェースは、下層の state machine が計算やストレージに課金する適切な gas 測定の仕組みを提供できる場合には、実行時にカスタムされた client を定義するカスタム検証ロジックが安全に提供されるように構成されるべきです。例えば WASM 実行をサポートするホスト state machine 上では、検証用関数や不一致検証関数は client インスタンスが作成された際に実行可能な WASM 関数として提供されるでしょう。

### 定義

- `get`、`set`、`Path`、`Identifier` は [ICS 24](../ics-024-host-requirements) で定義されます。

- `CommitmentRoot` は [ICS 23](../ics-023-vector-commitments) で定義されます。下流のロジックが特定の高さの state でキー/値のペアが存在するかどうかを検証するための安価な方法を提供する必要があります。

- `ConsensusState` は検証用関数の state を表現する opaque な型です。`ConsensusState` は関連する consensus アルゴリズムによって合意された state の更新を検証できなければなりません。また取引相手の machine など第三者が、特定の machine が特定の `ConsensusState` を格納していることを確認できるように、正規の方法でシリアライズできなければなりません。それは、state machine が過去のブロック高での自分の `ConsensusState` を調べることができるように、最終的にはそれが対象の state machine によって検査可能でなければなりません。

- `ClientState` は、client の state を表す opaque な型です。特定の高さにある state のキー/値のペアの存在/非存在を検証し、現在の `ConsensusState` を取得するために `ClientState` は、クエリ関数を公開しなければなりません。

### 望ましい特性

Light client は 他の chain の正規の header を、 `ConsensusState`を用いて検証する安全なアルゴリズムを提供しなければなりません。これによって、より上位レベルの抽象化は他の chain の consensus アルゴリズムによってコミットが保証されている `ConsensusState` 内に保存された `CommitmentRoot`を用いて、state のサブコンポーネントを検証することができるようになります。

検証用関数は対応する consensus アルゴリズムを実行しているフルノードの動作を反映することが期待されます。`ConsensusState` とメッセージリストが与えられたとして、フルノードが `Commit` を用いて生成された新しい `Header` を受け入れる場合、light client もそれを受け入れなければなりません。フルノードがそれを拒否した場合、light clientもそれを拒否しなくてはなりません。

light client はメッセージ全体を再生しているわけではないので、consensus が不正動作した場合、light client の振る舞いがフルノードの振る舞いと異なることがあります。この場合、検証用関数とフルノードとの乖離を証明する不正動作証明を生成して chain に提出することで、chain が light client を安全に無効化し、過去の state root を無効化し、より上位レベルの介入を待つことができます。

## 技術仕様

この仕様では、各 *client type*が定義する必要のある内容を概説します。client type とはlight client を動作させるために必要なデータ構造、初期化ロジック、検証用関数、不正動作検証関数を定義したものです。IBC プロトコルを実装する state machine は任意の数の client type をサポートすることができ、各 client type は異なる consensus インスタンスを追跡するために、異なる初期 consensus stateでインスタンス化することができます。2 台の machine 間の connection を確立するためには（[ICS 3](.../ics-003-connection-semantics) を参照）、machine はそれぞれ相手 machine の consensus アルゴリズムに対応する client type をサポートしている必要があります。

特定の client type はこの仕様の後のバージョンで定義されなければならず、その際、正規のリストはこのリポジトリに含まれるでしょう。IBC プロトコルを実装する machine は、これらの client type を尊重することが期待されていますが、 サブセットのみのサポートを選択するかもしれません。

### データ構造

#### ConsensusState

`ConsensusState` は client type で定義される opaque なデータ構造体であり、新しいコミットと state の root を検証するために検証用関数によって用いられます。おそらくこの構造体には、署名や validator セットのメタデータを含む、consensus プロセスによって生成された最新コミットが含まれるでしょう。

`ConsensusState` は `Consensus` インスタンスによって生成される必要があります。Consensus はそれぞれの `ConsensusState` に（ちょうど1つのブロック高に対して1つの consensus state を割り当てるように）ユニークなブロック高を割り当てます。同じ chain 上の2つの `ConsensusState` は commitment rootが等しくない限り同一のブロック高を持つべきではありません。そうした事象は「equivocation」と呼ばれ、不正動作として分類される必要があります。もし発生した場合は、証明を生成して提出しなければなりません。それにより client が凍結され、必要に応じて以前の state root が無効化されます。

`ConsensusState` は正規のシリアライゼーションを持つ必要があり、それによって他の chain は保存された consensus stateが他方と等しいことをチェックできます。（キー空間テーブルについては [ICS 24](../ics-024-host-requirements) を参照してください。）

```typescript
type ConsensusState = bytes
```

`ConsensusState` は、以降で定義されるように個別のキーの下で保存される必要があり、これにより他の chain は特定の consensus state が保存されていることを検証できます。

`ConsensusState` は consensus stateに紐付いたタイムスタンプを返す `getTimestamp()`メソッドを定義する必要があります。

```typescript
type getTimestamp = ConsensusState => uint64
```

#### Header

`Header` は `ConsensusState`を更新するための情報を提供する client type によって定義される opaque なデータ構造です。Header は保存された `ConsensusState` を更新する、関連する client へ提出することができます。ブロック高、証明、commitment root、そして場合によっては検証用関数の更新が含まれています。

```typescript
type Header = bytes
```

#### Consensus

`Consensus` は前の `ConsensusState` をメッセージと共に受け取り、`Header` を生成し、結果を返す関数です。

```typescript
type Consensus = (ConsensusState, [Message]) => Header
```

### Blockchain

ブロックチェーンは、有効な `Header` を生成する consensus アルゴリズムです。所定のメッセージを持つジェネシス `ConsensusState` から始まる header のユニークなリストを生成します。

`Blockchain` は以下のように定義されます。

```typescript
interface Blockchain {
  genesis: ConsensusState
  consensus: Consensus
}
```

ここで

- `Genesis` は 起源となる `ConsensusState`
- `Consensus` は header を生成する関数

`Blockchain` から生成される header は以下を満たすことが期待されます。

1. 各 `Header` は2つ以上の直接の子を持ってはいけません。

- 次の場合に満たされる：finailityと安全性
- 考えられる違反シナリオ：validator の2重署名、chainのリオルグ（Nakamoto consensus）

1. 各 `Header` は、最終的に少なくとも1つ直接の子が必要です。

- 次の場合に満たされる：liveness、light clientの検証機能が継続していること
- 考えられる違反シナリオ：同期の停止、互換性のないハードフォーク

1. 各 `Header` は、有効な状態遷移を保証する `Consensus` によって生成される必要があります。

- 次の場合に満たされる：正しいブロック生成と state machine
- 考えられる違反のシナリオ：不変性が中断されること、超多数の validator カルテル

ブロックチェーンが上記すべてを満たさない限り、IBC プロトコルは意図した通りに動作しない可能性があります。意図しない動作の例として、chain が複数の競合する packet を受信する可能性がある、chain がタイムアウトイベントから回復できない、chain が user の asset を盗む可能性があるなど。

検証用関数の妥当性は `Consensus` のセキュリティモデルに依存します。例えば `Consensus`は、信頼された operator による PoA であったり、ステークの価値が不十分な PoSであったりするかもしれません。このような場合、安全性の仮定が崩れ、`Consensus` と 検証用関数の対応はもはや存在せず、検証用関数の振る舞いは未定義となります。また、`Blockchain` は先に挙げた要件をもはや満たしていない可能性があり、IBC プロトコルとの非互換を引き起こします。原因の責任が判明している障害に関しては、不正動作証明を生成して client を保存している chain に提出することで、light client を安全に無効化しそれ以上の IBC packet 中継を防ぐことができます。

#### 検証用関数

検証用関数は client type によって定義される opaque な関数で、現在の`ConsensusState` に基づいて `Header`<br> を検証します。検証用関数の使用は、与えられた親 `Header` とネットワークメッセージのリストに対して完全な consensus アルゴリズムを再実行するよりもはるかに計算効率が良いはずです。

検証用関数と client state の更新ロジックは、1つの `checkValidityAndUpdateState` 型に統合されており、以下のように定義されています。

```typescript
type checkValidityAndUpdateState = (Header) => Void
```

`checkValidityAndUpdateState` は提供された Header が無効な場合、例外をスローする必要があります。

提供された header が有効であった場合、client は内部 stateを変更して現在確定されている consensus root を保存し、将来の検証用関数の呼び出しのために必要な署名権限の追跡（validator セットの変更など）を更新しなければなりません。

client は時間的制約のある検証用関数を備えていても良いです。一定期間（例えば3週間の bond 解除猶予期間）header が 提供されないような場合に、もはや client を更新できなくするといったものです。この場合、chain のガバナンスシステムや信頼されたマルチシグネチャのような 許可されたエンティティが、無効化された client の凍結を解除して新しく正しい header を提供するために介入することが許可されるかもしれません。

#### 不正行為検証用関数

不正動作検証用関数は client type で定義された opaque な関数で、データが consensus プロトコルに違反しているかどうかをチェックするために使用されます。consensus プロトコル違反には、state root が異なるがブロック高が同じである2つの署名付き header や、無効な状態遷移を含む署名付き header、consensus アルゴリズムで定義されたその他の不正行為の証拠といったものがありえます。

不正動作検証用関数と client state の更新ロジックは、以下のように定義される単一の `checkMisbehaviourAndUpdateState` 型に統合されます。

```typescript
type checkMisbehaviourAndUpdateState = (bytes) => Void
```

`checkMisbehaviourAndUpdateState` は提供された証拠が有効でない場合、 例外をスローする必要があります。

不正動作が有効だった場合、client は不正な動作の性質に応じて、内部 state を変更して、以前は有効と見なされていたブロック高を無効としてマークする必要があります。

不正動作が検出された場合、clientを無効化して、今後の更新を送信できないようにする必要があります。chain ガバナンスシステムや信頼されたマルチ署名などによって許可されたエンティティは、無効化された client の凍結を解除して新しく正しい header を提供するために介入することが許される場合があります。

#### ClientState

`ClientState` は client type によって定義される opaque なデータ構造です。所定の内部 state を保持し、検証された root と過去の不正行為を追跡することもできます。

light client は opaque な表現です — 異なる consensus アルゴリズムは異なる light client 更新アルゴリズムを定義することができます — が、IBC handler にクエリ関数の共通セットを公開する必要があります。

```typescript
type ClientState = bytes
```

client type は提供された consensus state を用いて client stateを初期化し、必要に応じて内部 state に書き込むメソッドを定義する必要があります。

```typescript
type initialise = (consensusState: ConsensusState) => ClientState
```

client type は、現在の高さ（最新の検証済み header の高さ）を取得するメソッドを定義する必要があります。

```typescript
type latestClientHeight = (
  clientState: ClientState)
  => uint64
```

#### CommitmentProof

`CommitmentProof`は [ICS 23](../ics-023-vector-commitments) に従い client type によって定義される opaque なデータ構造です。個別の finality を得たブロック高（必要に応じて特定の commitment root に関連付けられる）での特定のキー/値ペアの状態の有無を確認するために使用されます。

#### stateの検証

client type は、client が追跡する state machine の内部 state を認証する関数を定義する必要があります。内部実装の詳細は異なる場合があります（たとえば、ループバック client は単に state から直接読み取るだけで、証明は必要ありません）。

##### 必要な関数

`verifyClientConsensusState`は対象の machine に保存されている指定された client の consensus stateの証明を検証します。

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

`verifyConnectionState`は、対象の machine に保存されている指定された connection の connection state の証明を検証します。

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

`verifyChannelState`は、対象の machine に保存されている、指定されたポートの下の、指定された channel 終端の channel state の証明を検証します。

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

`verifyPacketData`は、指定されたポート、指定された channel 、および指定されたシーケンスでの発信 packet の commitment の証明を検証します。

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

`verifyPacketAcknowledgement` は、指定されたポート、指定された channel 、および指定されたシーケンスでの着信した packet <br> 応答の証拠を検証します。

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

`verifyPacketAcknowledgementAbsence` は、指定されたポート、指定された channel 、および指定されたシーケンスで着信 packet 応答がないことの証明を検証します。

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

`verifyNextSequenceRecv` は、指定されたポートで指定された channel が次に受信すべきシーケンス番号の証明を検証します。

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

#### クエリインタフェース

##### chain クエリ

これらのクエリエンドポイントは、特定の client に関連付けられた chain のノードによって、HTTPまたは同等のRPC APIを介して公開されると想定されています。

`queryHeader`は、特定の client によって検証される chain によって定義される必要があり、ブロック高による header の取得を許可する必要があります。このエンドポイントは信頼されないものと見なされます。

```typescript
type queryHeader = (height: uint64) => Header
```

`queryChainConsensusState` は 特定の client によって検証される chain で定義される場合があります。これは、新しい client を構成するために用いることができる現在の consensus state の取得を可能にします。この関数が使用される場合、`ConsensusState` は主観的であるため、照会しているエンティティが手動で確認しなくてはなりません。このエンドポイントは信頼されないものと見なされます。`ConsensusState` の正確な性質については client type によって異なっているかもしれません。

```typescript
type queryChainConsensusState = (height: uint64) => ConsensusState
```

（現在の consensus state だけでなく）ブロック高による過去の consensus state の取得は便利ですが、必須ではないことに注意してください。

`queryChainConsensusState`  は client の作成に必要なその他のデータ、例えばある種の PoS セキュリティモデルでの「bond解除準備期間」などを返すかもしれません。こうしたデータもまた、照会側エンティティによって検証される必要があります。

##### チェーン上の state クエリ

この仕様では、識別子によって client の状態を照会する単一の関数を定義します。

```typescript
function queryClientState(identifier: Identifier): ClientState {
  return privateStore.get(clientStatePath(identifier))
}
```

`ClientState` 型は最新の検証済ブロック高（もし望むなら、このブロック高から `queryConsensusState` を用いて consensus state を取得できます）を公開すべきです。

```typescript
type latestHeight = (state: ClientState) => uint64
```

client type は、relayer およびその他のオフチェーンエンティティが標準 API のオンチェーン state と連携できるように、以下の標準化されたクエリ関数を定義すべきです。

`queryConsensusState` は、保存されている consensus stateをブロック高によって取得できます。

```typescript
type queryConsensusState = (
  identifier: Identifier,
  height: uint64
) => ConsensusState
```

##### 証明の構築

各 client type は relayer が client state の検証アルゴリズムで必要となる証明を構築するための関数を定義すべきです。これらの関数は client type によって異なる形式を取るかもしれません。例えば、Tendermint client の証明はストアクエリのキー/値データと共に返されたり、solo client の証明は（user がメッセージに署名する必要があるため）当該の solo machine がインタラクティブに構築したりするかもしれません。これらの関数は、フルノードへの RPC を介した外部クエリや、ローカル計算や検証を構成することもできます。

```typescript
type queryAndProveClientConsensusState = (
  clientIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix,
  consensusStateHeight: uint64) => ConsensusState, Proof

type queryAndProveConnectionState = (
  connectionIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix) => ConnectionEnd, Proof

type queryAndProveChannelState = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix) => ChannelEnd, Proof

type queryAndProvePacketData = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix,
  sequence: uint64) => []byte, Proof

type queryAndProvePacketAcknowledgement = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix,
  sequence: uint64) => []byte, Proof

type queryAndProvePacketAcknowledgementAbsence = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix,
  sequence: uint64) => Proof

type queryAndProveNextSequenceRecv = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: uint64,
  prefix: CommitmentPrefix) => uint64, Proof
```

##### 実装戦略

###### ループバック

ローカル machine のループバック client は、アクセスが必要なローカル state から読み取るだけです。

###### 単純な署名

既知の公開鍵を持つ solo machine の client は local machine によって送信された、`Proof` パラメータとして提供されたメッセージの署名をチェックします。`height` パラメータはリレー防止の nonce として用いられます。

マルチシグネチャまたは閾値によるシグネチャ方式も同様の方法で使用できます。

###### proxy client

proxy client は、別の（proxy）machine による対象 machine の検証を検証します。最初に proxy machine 上の client state の証明をプルーフに含め、次に proxy machine 上の client state に関する、対象 machine のサブ state を2つ目の証明として含めます。これにより、proxy client は対象の machine 自体の consensus  state の保存と追跡を回避できますが、proxy machine の正しさに関するセキュリティ上の仮定が必要となります。

###### マークル化された state ツリー

マークル化された state ツリーを持つ state machine の clientでは、`verfiyMembership` や `verifyNonMembership` を呼び出すことで、`ClientState` に保存された検証済みマークルツリーを用いて、[ICS 23](../ics-023-vector-commitments) に従って特定ブロック高での state に特定のキー/値ペアが存在するかしないかを検証する関数を実装することができます。

```typescript
type verifyMembership = (ClientState, uint64, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, uint64, CommitmentProof, Path) => boolean
```

### サブプロトコル

IBC handler は、以下で定義される関数を実装する必要があります。

#### 識別子の検証

client は一意の `Identifier` プレフィックスの下に保存されます。この ICS では、client 識別子を特定の方法で生成する必要はなく、一意であれば良いです。ただし、必要に応じて `Identifier` の空間領域を制限することは可能です。検証関数 `validateClientIdentifier` が提供される場合があります。

```typescript
type validateClientIdentifier = (id: Identifier) => boolean
```

もし提供されない場合は、初期の `validateClientIdentifier` <br> は常に `true` を返します。

##### 過去のrootの活用

ハンドシェイクや packet 受信における client 更新（state root を変更する）と証明を運ぶトランザクションの間の競合状態を回避するために、多くの IBC handler 関数では、呼び出し元が参照する特定の過去の root を指定することができます。これを行う IBC handler 関数は、論理的な正確さを保証するために、呼び出し元によって渡されたブロック高に対して必要なチェックも確実に行われるようにしなければなりません。

#### 作成

識別子と初期 consensus state を指定して `createClient` を呼び出すと、新しい client が作成されます。

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

#### クエリ

client の consensus state と client の内部 state は識別子によって照会できますが、照会する必要のある個別の path はそれぞれの client type によって定義されます。

#### 更新

client の更新は新しい `Header` を提出することで行われます。`Identifier`は保存されているロジック更新対象の `ClientState` を指すのに用いられます。新しい `Header` が保存されている `ClientState` の検証用関数と `ConsensusState` を用いて検証されると、client はそれに応じて内部 state を更新しなければならず、 場合によっては commitment root を確定し、保存されたconsensus state の署名権限ロジックを更新しなければなりません。

client が更新できなくなった場合（信頼できる期限を経過した場合など）、その client に関連付けられた connection と channel を介して packet を送信したり、処理中（in-flight）の packet をタイムアウトしたりすることができなくなります（宛先 chain のブロック高とタイムスタンプが検証できないためです）。client の state をリセットしたり、connection と channel を別の client に移行したりするには、手動による介入が必要です。これは完全に自動的に安全に行うことはできませんが、IBC を実装する chain は、ガバナンスメカニズムがこれらのアクションを実行できるように選択できます（おそらくマルチシグまたはコントラクトの client/connection/channel ごとに選択することになります）。

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

#### 不正動作

client が不正な動作の証拠を検出した場合、警告を発し、以前に有効だった state の root を無効にし、将来の更新を防ぐことができます。

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

### 実装例

単一オペレータでの consensus アルゴリズムを実行している chain に対する検証用関数の例を構築します。ここで、有効なブロックは operator によって署名されるとします。opearator の署名鍵は、chain の実行中に変更することができます。

client 固有の型は次のように定義されます。

- `ConsensusState` は最新のブロック高と最新の公開鍵を保存します
- `Header` にはブロック高、新しい commitment root、 operator による署名、そして場合によっては新しい公開鍵が含まれます
- `checkValidityAndUpdateState` は送信されたブロック高が単調増加していること、および署名が正しいことを確認して内部 state を変更します
- `checkMisbehaviourAndUpdateState` は高さが同じで commitment root が異なる2つの header をチェックし、内部 state を変更します

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
    return clientState.verifiedRoots[sequence].verifyMembership(path, hash(data), proof)
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
    return clientState.verifiedRoots[sequence].verifyMembership(path, hash(acknowledgement), proof)
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

### プロパティと不変条件

- client の識別子は不変で先着順です。client は削除することができません (仮に削除を許可できるとすると、識別子の再利用によって過去の packet が将来再利用される可能性があるでしょう)。

## 後方互換性

該当しません。

## 前方互換性

新しい client type はこのインタフェースに準拠している限り、自由に IBC 実装で追加することができます。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

2019年3月5日 - 最初のドラフトが終了し、PRとして提出

2019年5月29日 - さまざまな改訂、特に複数の commitment root

2019年8月15日 - client インタフェースをわかりやすくするための大幅な改訂

2020年1月13日 - client type の分離と path の変更に関する改訂

2020年1月26日 - クエリインタフェースの追加

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
