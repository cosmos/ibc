---
ics: 4
title: Channel & Packet Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 3, 5, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## 概要

「channel」の抽象化は、interblockchain 通信プロトコルにメッセージ配信のセマンティクスを提供します。channel は、ある chain 上の module と別の chain 上の module の間を通過する packet の導管として機能し、packet が一度だけ実行され、（必要に応じて）送信された順に配信され、宛先 chain 上 の channel 端に対応する module にのみ配信されることを保証します。各 channel は特定の connection に関連付けられます。また connection はいくつでも関連付けられた channel を持つことができ、共通の識別子を使用することができ、1つの connection と light client を利用するすべての channel で header 検証のコストを削減することができます。

channel はペイロードに依存しません。IBC packet を送受信する module は、packet データをどのように構築し、どのように着信 packet データに対応するかを決定し、packet に含まれるデータに応じてどの state トランザクションを適用するかを決定するために、独自のアプリケーションロジックを利用しなければなりません。

### モチベーション

IBC プロトコルは、チェーン間のメッセージパッシングモデルを使用します。IBC *packet* は、外部の relayer プロセスによって一方のブロックチェーンから他方のブロックチェーンに中継されます。chain `A` と chain `B` は独立して新しいブロックを確認し、一方の chain から他方の chain への packet は遅延、検閲、または任意の再順序付けが可能です。packet は relayer から見えるようになっており、任意の relayer プロセスによってブロックチェーンから読み取られ、他のブロックチェーンに送信されることができます。

> IBCプロトコルは、アプリケーションが2つのchain上で接続されたmoduleの結合された状態について推論できるように、順序付け（順序付けられた channel 向け）と正確に1回だけの配信保証を提供しなければなりません。例えば、あるアプリケーションは、単一のトークン化されたアセットを複数の blockchain 間で転送し、複数の blockchain 上で保持できるようにしながら、供給可能性と供給の保全を維持したい場合があります。アプリケーションは、特定の IBC packet が chain  `B` にコミットされたときに、chain `B` 上でアセットバウチャーを作成でき、chain  `A` 上でその packet の送信を要求して、バウチャーが後で逆方向の IBC packet で chain `A` に引き戻されるまで、chain `A` 上で同量のアセットをエスクローすることができます。この順序保証と正しいアプリケーションロジックは、両方の chain での全体供給量が維持され、chain `B` で作成されたバウチャーが後で chain `A` に償還されることを保証することができます。

アプリケーション層に望ましい順序付け、正確な一度限りの配信、module の許可セマンティクスを提供するために、IBC プロトコルはこれらのセマンティクスを強制するための抽象化を実装しなければなりません — channel はこの抽象化のことです。

### 定義

`ConsensusState` は [ICS 2](../ics-002-client-semantics) で定義されます。

`Connection` は [ICS 3](../ics-003-connection-semantics) で定義されます。

`Port` と `authenticateCapability` は [ICS 5](../ics-005-port-allocation) で定義されます。

`hash`は一般的な衝突耐性のあるハッシュ関数で、その仕様は channel を利用する module によって合意されなければなりません。`hash` は異なる chain によって異なる定義をすることができます。

`identifier`、`get`、`set`、`delete`、`getCurrentHeight`、module システムに関連した基本要素は [ICS 24](../ics-024-host-requirements) で定義されます。

*channel *は、別個のブロックチェーン上の特定の module 間で正確に1回の packet 配信を行うためのパイプラインであり、packet を送信することができる最低1つ以上の端と、packet を受信することができる最低1つ以上の端を有しています。

*双方向* channel は、`A` から `B` へ、`B` から`A` へといった風に、packet が両方向に流れる channelです。

*単方向* channel は、packet が一方向にしか流れない channel です。`A` から `B` へ（または `B` から `A` へ。命名順序は任意です)。

*順序付けられた* channel とは、packet が送信された順番通りに配信される channel のことです。

*順序付けられない* channel とは、packet が送信された順序とは異なる任意の順序で配信される channel のことです。

```typescript
enum ChannelOrder {
  ORDERED,
  UNORDERED,
}
```

方向性と順序は独立しているので、双方向の順序付けられない channel、一方向の順序付けられた channel などと表現することができます。

すべての channel は正確に1度だけの packet 配信を提供します。channel の一方の端で送信された packet が他方の端に届くのは1回だけ、それ以上でもそれ以下でもないことを意味します。

この仕様では、*双方向* channel のみを対象とします。*単方向* channel は、ほとんど同じプロトコルを使用することができ、将来の ICS で概説される予定です。

channe の終端は、channel のメタデータを格納する1つの chain 上のデータ構造体です。

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

- `state` は channel 終端の現在の state です。
- `ordering` フィールドは channel が順序付けられているかどうかを示します。
- `counterpartyPortIdentifier` は channel の他方の終端を所有している相手側 chain の port を示します。
- `counterpartyChannelIdentifier` は相手側 chain の channel 終端を示します。
- `nextSequenceSend` は、別で保管され、次に送付される packet のシーケンス番号を追跡します。
- `nextSequenceRecv`は、別で保管され、次に受け取るべき packet のシーケンス番号を追跡します。
- `nextSequenceAck`は、別で保管され、次に通知されるべき packet のシーケンス番号を追跡します。
- `connectionHops` はこの channel で送信される packet が移動する connection 識別子のリストを順に保管します。現時点ではこのリストの長さは1でなければなりません。将来的にはマルチホップ channel がサポートされる可能性があります。
- `version` 文字列はハンドシェイク時に合意された opaque な channel バージョンを保管します。これはどの packet エンコーディングが channel で使用されるかといった module レベルでの設定を決定します。コアのIBC プロトコルはこのバージョンを用いません。

Channel の終端は *state* を持ちます。

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  CLOSED,
}
```

- `INIT` state にいる channel 終端は開始に伴うハンドシェイクをちょうど開始したところです。
- `TRYOPEN` state にいる channel 終端は相手側の chain へ通知したところです。
- `OPEN` state にいる channel 終端はハンドシェイクを完了し、packet を送受信する用意が完了しています。
- `CLOSED` state にいる channel 終端は閉じており、もはや packet の送受信が行えません。

`packet` は IBC では次のような特定のインタフェースになります。

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: uint64
  timeoutTimestamp: uint64
  sourcePort: Identifier
  sourceChannel: Identifier
  destPort: Identifier
  destChannel: Identifier
  data: bytes
}
```

- `sequence` 番号は送受信の順序に対応しており、より早いシーケンス番号を持つ packet はより遅いシーケンス番号を持つ packet よりも前に送受信される必要があります。
- `timeoutHeight` は 宛先 chain 上の consensus ブロックの高さを示し、これ以降では packet が処理されずに、タイムアウトがあったとみなされます。
- `timeoutTimestamp` は宛先 chain上のタイムスタンプで、これ以降では packet が処理されずにタイムアウトがあったとみなされます。
- `sourcePort` は送信元 chain の port を示します。
- `sourceChannel` は送信元 chain の channel 終端を示します。
- `destPort` は受信先 chain の port を示します。
- `destChannel` は受信先 chain の channel 終端を示します。
- `data` は関連付けられた module のアプリケーションロジックによって定義される opaque な値です。

`packet` は直接シリアライズされることはありません。むしろ、IBC ハンドラを呼び出す module が作成されたり処理されたりする必要があるかもしれない、ある種の関数呼び出しで使用される中間構造体です。

`OpaquePacket` は packet ですが、ホストの state machine によってデータ型が隠蔽されているため、module は IBC ハンドラに渡す以外のことはできません。IBC ハンドラは `packet` を `OpaquePacket` と相互にキャストすることが可能です。

```typescript
type OpaquePacket = object
```

### 望ましい特性

#### 効率性

- packet 送信と確認の速度は、下層の chain の速度によってのみ制限されるべきです。proof は可能であれば一括して行うことができるようにすべきです。

#### 正確に1度の送付

- channel の一方の終端で送信された IBC packet は、他方の終端に一度だけ正確に配信されなければなりません。
- 正確に一度だけの安全性を確保するためには、ネットワークの同期を仮定する必要はありません。chain の片方または両方が停止した場合 packet は一度も配信されず、chain が再開すると packet は再び流れるようになるはずです。

#### 順序

- 順序付けられた channel では、packet は同じ順序で送受信されるべきです。もし chain `A` の channel 終端によって packet *x* が packet *y* の前に送信されたならば、packet *x* は packet *y* より前に、対応する chain `B` の channel 終端で受信されなければなりません。
- 順序付けられていない channel では、packet は任意の順序で送受信されます。順序付けられていない packet は順序付けられた packet と同様に、宛先 chain のブロック高さで指定された個別のタイムアウトを持ちます。

#### 許可

- channel は、各終端にある1つの module に許可され、ハンドシェイク中に決定され、その後は不変であるべきです。 (上位レベルのロジックは、port の所有権をトークン化することで、channel の所有権をトークン化しても良いでしょう)。channel 終端に関連付けられた module のみが、channel 上で送受信できるようにするべきです。

## 技術仕様

### データフローの視覚化

client、connection、channel、および packet のアーキテクチャ

![Dataflow Visualisation](dataflow.png)

### 準備

#### Store パス

channel 構造体は、port 識別子と channel 識別子の組み合わせに固有の store パス接頭辞を付けて格納されます。

```typescript
function channelPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

channel に関連付けられたケイパビリティキーは、`channelCapabilityPath`の下に格納されます。

```typescript
function channelCapabilityPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
  return "{channelPath(portIdentifier, channelIdentifier)}/key"
}
```

`nextSequenceSend`、`nextSequenceRecv`、`nextSequenceAck` は符号なし整数型のカウンタで、独立して証明できるように別個に保管されます。

```typescript
function nextSequenceSendPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceSend"
}

function nextSequenceRecvPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceRecv"
}

function nextSequenceAckPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/nextSequenceAck"
}
```

packet データフィールドに対する定数サイズの commitment は、packet シーケンス番号の下に格納されます。

```typescript
function packetCommitmentPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/packets/" + sequence
}
```

store 内のパスが存在しないことは、ゼロビットと同等です。

ack packet データは `packetAcknowledgementPath` の下に保管されます。

```typescript
function packetAcknowledgementPath(portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "{channelPath(portIdentifier, channelIdentifier)}/acknowledgements/" + sequence
}
```

順序付けられていない channel は常にこのパスに通知を書き込まなくてはなりません。これによって、パスが存在しないことをタイムアウトの proof として用いることができます。順序付けられた channel は通知を書き込むことができますが、必須ではありません。

### バージョニング

ハンドシェイクプロセスの間に、channel の両端は、その channel に関連付けられたバージョンの byte 文字列に合意します。このバージョン byte 文字列の内容は、IBC のコアプロトコルには opaque なままです。host  state machine はバージョンデータを利用して、サポートされている IBC/APP プロトコルを示したり、packet エンコーディング形式に合意したり、IBC 上のカスタムロジックに関連する他の channel 関連のメタデータをネゴシエートしたりしてもよいです。

host state machine は、バージョンデータを安全に無視するか、空の文字列を指定してもよいです。

### サブプロトコル

> 注意：host state machineがobject capability認証を利用している場合（[ICS 005](../ics-005-port-allocation)参照）、port を利用するすべての機能は追加のcapability引数を取ります。

#### 識別子の検証

Channels はユニークな `(portIdentifier, channelIdentifier)` <br> 接頭辞の下に保管されます。検証関数 `validatePortIdentifier` が提供されてもよいです。

```typescript
type validateChannelIdentifier = (portIdentifier: Identifier, channelIdentifier: Identifier) => boolean
```

もし提供されない場合は、初期の `validateClientIdentifier` <br> は常に `true` を返します。

#### channel のライフサイクル管理

![Channel State Machine](../../../channel-state-machine.png)

開始者 | Datagram | 実行される chain | 事前 state (A, B) | 事後 state (A, B)
--- | --- | --- | --- | ---
Actor | ChanOpenInit | A | (none, none) | (INIT, none)
Relayer | ChanOpenTry | B | (INIT, none) | (INIT, TRYOPEN)
Relayer | ChanOpenAck | A | (INIT, TRYOPEN) | (OPEN, TRYOPEN)
Relayer | ChanOpenConfirm | B | (OPEN, TRYOPEN) | (OPEN, OPEN)

開始者 | Datagram | 実行される chain | 事前 state (A, B) | 事後 state (A, B)
--- | --- | --- | --- | ---
Actor | ChanCloseInit | A | (OPEN, OPEN) | (CLOSED, OPEN)
Relayer | ChanCloseConfirm | B | (CLOSED, OPEN) | (CLOSED, CLOSED)

##### ハンドシェイクの開始

`chanOpenInit` 関数は module が 他の chain 上の module との channel 開始ハンドシェイク を始めるために呼び出します。

開始される channel は、ローカル channel 、ローカル port、リモート port、およびリモート channel の識別子を提供しなければなりません。

開始ハンドシェイクが完了すると、ハンドシェイクを開始した module は、ホスト台帳上に作成された channel の端を所有し、それが指定した相手側 module は、相手側 chain 上に作成された channel の端を所有することになります。一度 channel が作成されると、所有権は変更できません（これを提供するために、より上位レベルの抽象化が実装される可能性はあります）。

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

    // 楽観的な channel ハンドシェイクは許可されています
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(authenticateCapability(portPath(portIdentifier), portCapability))
    channel = ChannelEnd{INIT, order, counterpartyPortIdentifier,
                         counterpartyChannelIdentifier, connectionHops, version}
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    channelCapability = newCapability(channelCapabilityPath(portIdentifier, channelIdentifier))
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceAckPath(portIdentifier, channelIdentifier), 1)
    return channelCapability
}
```

`chanOpenTry` 関数は module によって呼び出され、他の chain 上の module によって開始された channel 開始ハンドシェイクの最初のステップに同意します。

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
    abortTransactionUnless(authenticateCapability(portPath(portIdentifier), portCapability))
    connection = provableStore.get(connectionPath(connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{INIT, order, portIdentifier,
                          channelIdentifier, [connection.counterpartyConnectionIdentifier], counterpartyVersion}
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
    channelCapability = newCapability(channelCapabilityPath(portIdentifier, channelIdentifier))
    provableStore.set(nextSequenceSendPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceAckPath(portIdentifier, channelIdentifier), 1)
    return channelCapability
}
```

`chanOpenAck` は、ハンドシェイク発信 module によって呼び出され、相手側 chain 上の module による最初のリクエスト承認を確認応答します。

```typescript
function chanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string,
  proofTry: CommitmentProof,
  proofHeight: uint64) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state === INIT || channel.state === TRYOPEN)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{TRYOPEN, channel.order, portIdentifier,
                          channelIdentifier, [connection.counterpartyConnectionIdentifier], counterpartyVersion}
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

`chanOpenConfirm` 関数は、ハンドシェイクを受け入れる module によって呼び出され、他方の chain 上のハンドシェイク発信 module の確認応答を確認して、channel 開始ハンドシェイクを終了します。

```typescript
function chanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === TRYOPEN)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{OPEN, channel.order, portIdentifier,
                          channelIdentifier, [connection.counterpartyConnectionIdentifier], channel.version}
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

##### ハンドシェイクの閉鎖

`chanCloseInit` 関数は channel 終端を閉じる module によって呼び出されます。一度閉じられると、channel は再開できません。

module の呼び出しは、`chanCloseInit` の呼び出しと連動して、適切なアプリケーションロジックをアトミックに実行してもよいです。

移動中の packet は、channel が閉じられるとすぐにタイムアウトすることができます。

```typescript
function chanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
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

`chanCloseConfirm` 関数は相手側の module によって呼び出され、一方の channel 終端が閉じられてから、両終端を閉じます。

module 呼び出しは、`chanCloseConfirm` の呼び出しと連動して適切なアプリケーションロジックをアトミックに実行してもよいです。

一度閉じてしまうと、channel を再開することはできず、識別子を再利用することもできません。識別子の再利用は、以前に送信された packet の再利用の可能性を防ぎたいために行われます。再利用の問題は、light client アルゴリズムがメッセージ（IBC packet）に "署名"している場合を除いて、署名付きメッセージでシーケンス番号を使用することに類似しており、再利用防止シーケンスはport識別子、channel識別子、packet識別子の組み合わせです。もし、特定の最大高さ/時間のタイムアウトが義務化され、追跡されていれば、安全に識別子を再利用することが可能になり、将来の仕様ではこの機能が含まれるかもしれません。

```typescript
function chanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofInit: CommitmentProof,
  proofHeight: uint64) {
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability))
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    expected = ChannelEnd{CLOSED, channel.order, portIdentifier,
                          channelIdentifier, [connection.counterpartyConnectionIdentifier], channel.version}
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

#### packet フローと処理

![Packet State Machine](packet-state-machine.png)

##### packet のとある1日

machine *A* 上の module *1* から machine *B* 上の module *2* に packet を送信するには、最初から始めると以下の手順が必要です。

module は [ICS 25](.../ics-025-handler-interface) または [ICS 26](.../ics-026-routing-module) を介してIBC handler とやりとりできます。

1. client と port の初期設定、任意の順番で良い
    1. *A* 上で *B* のために作成された client（[ICS 2](.../ics-002-client-semantics) を参照）
    2. *B* 上で *A* のために作成された client（[ICS 2](.../ics-002-client-semantics) を参照）
    3. module *1* を port に割り当て（[ICS 5](../ics-005-port-allocation) を参照）
    4. module *2* を port に割り当て（[ICS 5](../ics-005-port-allocation) を参照）、この port は module *1* と帯域外で通信します
2. 順番通りに connection と channel を確立し、楽観的に送信する
    1. module *1* によって *A* から *B* への connection 開始ハンドシェイクが始まります（[ICS 3](../ics-003-connection-semantics)を参照）。
    2. 新たに作成した connection を使用して *1* から *2* へのchannel開始ハンドシェイクが始まります。(本ICS)
    3. packetは新たに作成されたchannelを経由して *1* から *2* に送信されます（本 ICS)
3. ハンドシェイクの正常終了（どちらかのハンドシェイクに失敗した場合、connection/channelを閉じてpacketをタイムアウトさせることができます。）
    1. connection 開始のハンドシェイクが正常に完了します（[ICS 3](../ics-003-connection-semantics)参照）（これにはrelayerプロセスの参加が必要です）
    2. channel開始のハンドシェイクが正常に完了します。(本 ICS) (これにはrelayerプロセスの参加が必要です)
4. マシン*B*、module *2*でのpacket確認(タイムアウトの高さを過ぎた場合はpacketタイムアウト) (これにはrelayerプロセスの参加が必要です)
5. 確認応答はマシン*B*のmodule *2*からマシン*A*のmodule*1*に中継されます（可能性があります）

空間的に表現すると、2台のマシン間の packet 転送は次のように表すことができます。

![Packet Transit](packet-transit.png)

##### packetの送信

`sendPacket` 関数は、呼び出し側の module が所有するchannel end 上の IBC packet を相手 chain 上の対応する module に送信するために、module によって呼び出されます。

呼び出し側のモジュールは、`sendPacket` の呼び出しと連動して、アプリケーションロジックをア トミックに実行しなければなりません。

IBC ハンドラーは以下の手順を順に実行します。:

- packetを送信するために、channel と connection が開いているかどうかを確認します。
- 呼び出し側の module が送信 port を所有しているかどうかを確認します。
- packet のメタデータが channel および connection 情報と一致しているかどうかを確認します。
- 指定したタイムアウトの高さが、宛先 chain でまだ経過していないことを確認します。
- channel に関連付けられた送信シーケンスカウンタを増やします。
- packet データと packet タイムアウトに対する一定サイズの commitment を保管します。

完全な packet は chain の状態には保存されないことに注意してください - データとタイムアウト値に対する短い hash-commitment に過ぎません。packet データはトランザクション実行から計算され、relayer がインデックスを作成できるログ出力として返される可能性があります。

```typescript
function sendPacket(packet: Packet) {
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))

    // 楽観的な送信はハンドシェイクが開始された後に許可されます。
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))

    abortTransactionUnless(connection !== null)

    // 我々のlocal client が受信chainを追跡する際に、タイムアウトの高さが既に通過していないことを簡易に確認してください
    latestClientHeight = provableStore.get(clientPath(connection.clientIdentifier)).latestClientHeight()
    abortTransactionUnless(packet.timeoutHeight === 0 || latestClientHeight < packet.timeoutHeight)

    nextSequenceSend = provableStore.get(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(packet.sequence === nextSequenceSend)

    // すべてのアサーションが問題ない場合、状態を変更することができます。

    nextSequenceSend = nextSequenceSend + 1
    provableStore.set(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel), nextSequenceSend)
    provableStore.set(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence),
                      hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // packet が送信されたことを記録します
    emitLogEntry("sendPacket", {sequence: packet.sequence, data: packet.data, timeoutHeight: packet.timeoutHeight, timeoutTimestamp: packet.timeoutTimestamp})
}
```

#### packetの受信

`recvPacket` 関数は、相手 chain 上の対応する channel end で送信された IBC packet を受信して処理するために、module によって呼び出されます。

呼び出し module は、おそらく事前に確認応答値を計算するために、`recvPacket`の呼び出しと連動してアプリケーションロジックをアトミックに実行しなければなりません。

IBC ハンドラーは以下の手順を順に実行します。:

- channel と connection が packet を受信するために開いているかどうかを確認します。
- 呼び出し側の module が受信 port を所有しているかどうかを確認します。
- packet のメタデータが channel および connection 情報と一致しているかどうかを確認します。
- packet シーケンスが、channel end が受信すると期待している次のシーケンスであるかどうかをチェックします（順序付けられた channelの場合）。
- タイムアウトの高さをまだ過ぎていないことを確認します。
- 送信 chain の状態で packet data commitment の包含証明を確認します。
- opaque な確認応答の値をpacket 固有の保存パスに設定します (確認応答が空でないか、チャネルが順不同の場合)。
- channel endに関連付けられたpacket受信シーケンスを増やします（順序付けられたchannelのみ）。

```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  acknowledgement: bytes): Packet {

    channel = provableStore.get(channelPath(packet.destPort, packet.destChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.destPort, packet.destChannel), capability))
    abortTransactionUnless(packet.sourcePort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.sourceChannel === channel.counterpartyChannelIdentifier)

    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    abortTransactionUnless(packet.timeoutHeight === 0 || getConsensusHeight() < packet.timeoutHeight)
    abortTransactionUnless(packet.timeoutTimestamp === 0 || currentTimestamp() < packet.timeoutTimestamp)

    abortTransactionUnless(connection.verifyPacketData(
      proofHeight,
      proof,
      packet.sourcePort,
      packet.sourceChannel,
      packet.sequence,
      concat(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)
    ))

    // (シーケンスチェックを除く) すべてのアサーションが通過した場合、状態を変更することができます。

    // 確認応答は常に相手側で確認できるように設定します。
    provableStore.set(
      packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence),
      hash(acknowledgement)
    )

    if (channel.order === ORDERED) {
      nextSequenceRecv = provableStore.get(nextSequenceRecvPath(packet.destPort, packet.destChannel))
      abortTransactionUnless(packet.sequence === nextSequenceRecv)
      nextSequenceRecv = nextSequenceRecv + 1
      provableStore.set(nextSequenceRecvPath(packet.destPort, packet.destChannel), nextSequenceRecv)
    } else
      abortTransactionUnless(provableStore.get(packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence) === null))

    // packet を受信し、承認したことを記録します
    emitLogEntry("recvPacket", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight,
                                timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, acknowledgement})

    // return transparent packet
    return packet
}
```

#### 確認応答

`acknowledgePacket`関数は、呼び出し側の module が channel 上で相手 chain上の module に以前に送信した packet の確認応答を処理するために module から呼び出されます。`acknowledgePacket`は packet commitment を削除しますが、これは packetを受信して処理されたので必要ではなくなったからです。

呼び出し module は、`acknowledgePacket`の呼び出しと連動して、適切なアプリケーションの確認応答処理ロジックをアトミックに実行してもよいです。

```typescript
function acknowledgePacket(
  packet: OpaquePacket,
  acknowledgement: bytes,
  proof: CommitmentProof,
  proofHeight: uint64): Packet {

    // そのchannelが開いていなければトランザクションを中止し、呼び出し moduleは関連するportを所有し, そしてその packet フィールドは一致します
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // packetを送信し、まだそれをクリアしていないことを確認します
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // 相手chainの正しい確認応答がない場合はトランザクションを中止します
    abortTransactionUnless(connection.verifyPacketAcknowledgement(
      proofHeight,
      proof,
      packet.destPort,
      packet.destChannel,
      packet.sequence,
      acknowledgement
    ))

    // 確認応答が順番に処理されていなければトランザクションを中止します
    if (channel.order === ORDERED) {
      nextSequenceAck = provableStore.get(nextSequenceAckPath(packet.sourcePort, packet.sourceChannel))
      abortTransactionUnless(packet.sequence === nextSequenceAck)
      nextSequenceAck = nextSequenceAck + 1
      provableStore.set(nextSequenceAckPath(packet.sourcePort, packet.sourceChannel), nextSequenceAck)
    }

    // すべてのアサーションに問題がなかったので、状態を変更できます。

    // 再度確認応答ができないように、commitmentを削除します
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

#### タイムアウト

アプリケーションのセマンティクスでは、ある程度のタイムアウトが必要になる場合があります。これは、 chain がトランザクションをエラーとみなす前に、処理されるのをどれだけの時間待つかの上限のことです。2つのchainは異なるローカルクロックを持っているので、これは明らかに二重支払いの攻撃ベクトルとなります - 攻撃者はレシートの中継を遅らせたり、タイムアウト直後まで packet の送信を待ったりする可能性があります。そのため、アプリケーションは単純なタイムアウトロジックを自分自身で安全に実装することができません。

「二重支払い」攻撃の可能性を避けるために、タイムアウトアルゴリズムは宛先 chain が動作していて到達可能であることを必要とすることに注意してください。完全なネットワークパーティションでは何も証明できず、接続するのを待たなければなりません。タイムアウトは受信側の chain で証明されなければならず、単に送信側の chain で応答がないだけではありません。

##### 送信終了

`timeoutPacket`関数は、最初に packet を 相手module に送信しようとした module によって呼び出されます。そこでは、タイムアウトの高さまたはタイムアウトのタイムスタンプは、packet がコミットされずに相手 chain で渡され、packet が実行できなくなったことを証明し、呼び出し module が適切な状態遷移を安全に実行できるようにします。

呼び出し module は、`timeoutPacket`の呼び出しと連動して、適切なアプリケーションのタイムアウト処理ロジックをアトミックに実行してもよいです。

順序付けされたchannelの場合、`timeoutPacket` は受信 channel 端の`recvSequence`を確認し、packet がタイムアウトした場合は channel を閉じます。

順不同の channel の場合、`timeoutPacket`は、（packetを受信した場合には書き込まれていたであろう）確認応答がないことを確認します。順不同の channel は、タイムアウトした packet に直面しても継続することが期待されます。

後続 packet のタイムアウトの高さに関係がある場合、タイムアウト packet の前のすべての packet の安全な一括タイムアウトを実行することができます。本仕様では、今のところ詳細は省略します。

```typescript
function timeoutPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)

    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // 注意: connection が閉じられているかもしれません。
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // 反対側でタイムアウトの高さまたはタイムアウトタイムスタンプが経過したことを確認します
    abortTransactionUnless(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && connection.getTimestampAtHeight(proofHeight) > packet.timeoutTimestamp))

    // このpacketが実際に送信されたことを確認し、ストアを確認します
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    if channel.order === ORDERED {
      // 順序付けられた channel: packetを受信していないことを確認します
      abortTransactionUnless(nextSequenceRecv <= packet.sequence)
      // 順序付けられた channel: recvシーケンスが要求どおりであることを確認します
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
    } else
      // 順不同の channel: packet indexで確認応答がないことを確認します
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence
      ))

    // すべてのアサーションに問題がないので、状態を変更できます

    // commitmentを削除します
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    if channel.order === ORDERED {
      // 順序付けられた channel: channelを閉じる
      channel.state = CLOSED
      provableStore.set(channelPath(packet.sourcePort, packet.sourceChannel), channel)
    }

    // return transparent packet
    return packet
}
```

##### 閉じる際のタイムアウト

`timeoutOnClose` 関数は、未受信の packet が宛先とした channel が閉じられたことを証明するために、module によって呼び出され、packet は決して受信されません (たとえ  `timeoutHeight` や `timeoutTimestamp` がまだ到達していなくても)。

```typescript
function timeoutOnClose(
  packet: Packet,
  proof: CommitmentProof,
  proofClosed: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    // 注意: channelは閉じているかもしれません
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // 注意: connection は閉じているかもしれません
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // このpacketが実際に送信されたことを確認し、ストアを確認します
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // 反対側のchannel endが閉じていることを確認します
    expected = ChannelEnd{CLOSED, channel.order, channel.portIdentifier,
                          channel.channelIdentifier, channel.connectionHops.reverse(), channel.version}
    abortTransactionUnless(connection.verifyChannelState(
      proofHeight,
      proofClosed,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      expected
    ))

    if channel.order === ORDERED {
      // 順序付けられた channel: recv シーケンスが要求通りであることを確認します
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
      // 順序付けられた channel: そのpacketを受信していないことを確認する
      abortTransactionUnless(nextSequenceRecv <= packet.sequence)
    } else
      // 順不同 channel: packet indexに確認応答がないことを確認する
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence
      ))

    // すべてのアサーションに問題がないので、状態を変更できます

    // commitmentを削除します
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

##### 状態の削除

packet を片付けるためには必ず確認しなければなりません。

#### 競合状態について推測する

##### ハンドシェイクの同時試行

2 台のマシンがchannel開始ハンドシェイクを同時に始め、同じ識別子を使用しようとした場合、両方とも失敗し、新しい識別子を使用しなければなりません。

##### 識別子の割当

宛先 chain 上での識別子の割り当てには、避けられない競合条件があります。module は、疑似ランダムで意味のない識別子を利用することが賢明です。ただし、別の module が使用したい識別子への要求を管理することは、煩わしい一方で、ハンドシェイクの対象となる port を受信 module がすでに所有している必要があるため、ハンドシェイクを仲介することはできません。

##### タイムアウト / packet 確認

packet が受信前にタイムアウトの高さを通過したかどうかにかかわらず、packet のタイムアウトと packetの確認の間に競合状態はありません。

##### ハンドシェイク中の中間者攻撃

cross-chain の状態を検証することで、connection ハンドシェイクと channel ハンドシェイクの両方で中間者攻撃を防ぐことができます。なぜなら、すべての情報（送信元、宛先 client 、channel など）はハンドシェイクを開始する module によって知られており、ハンドシェイクが完了する前に確認されるからです。

##### 通信中の packet によるconnection/channelの閉鎖

packet が通信中に connection や channel が閉じられた場合、宛先 chain では packet を受信できなくなり、送信元 chain ではタイムアウトする可能性があります。

#### channelsの問い合わせ

Channelは `queryChannel` で問い合わせることができます:

```typescript
function queryChannel(connId: Identifier, chanId: Identifier): ChannelEnd | void {
    return provableStore.get(channelPath(connId, chanId))
}
```

### 特性と不変条件

- channel と port 識別子のユニークな組み合わせは、先着順です。一旦ペアが割り当てられると、問題の port を所有するmoduleだけが、そのchannelで送受信できます。
- packet は、chain がタイムアウトウィンドウ内で生きていると仮定して、正確に一度だけ配信され、タイムアウトした場合には、送信chainで正確に一度だけタイムアウトすることができます。
- channel のハンドシェイクは、どちらかのブロックチェーン上の他のmoduleや他のブロックチェーンのIBCハンドラーによって中間者攻撃を受けることはありません。

## 後方互換性

該当しません。

## 前方互換性

データ構造とエンコーディングは、connection または channel レベルでバージョン管理することができます。channel ロジックは packet データフォーマットに完全に依存しないため、module によっていつでも好きなように変更することができます。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

2019年6月5日 - ドラフト提出

2019年7月4日 - 順序付けされていない channel及び応答部分の修正

2019年7月16日 - マルチホップルーティングの将来の互換性のための変更

2019年07月29日 - connection 終了後のタイムアウトを処理するための改訂

2019年8月13日 - 諸々編集

2019年8月25日 - 整理

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
