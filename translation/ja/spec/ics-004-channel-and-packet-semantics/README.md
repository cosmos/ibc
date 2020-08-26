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

The "channel" abstraction provides message delivery semantics to the interblockchain communication protocol, in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one chain and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination chain. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortising the cost of header verification across all the channels utilising a connection & light client.

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
- The `version` string stores an opaque channel version, which is agreed upon during the handshake. This can determine module-level configuration such as which packet encoding is used for the channel. This version is not used by the core IBC protocol.

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
- The `timeoutHeight` indicates a consensus height on the destination chain after which the packet will no longer be processed, and will instead count as having timed-out.
- `timeoutTimestamp` は宛先 chain上のタイムスタンプで、これ以降では packet が処理されずにタイムアウトがあったとみなされます。
- `sourcePort` は送信元 chain の port を示します。
- `sourceChannel` は送信元 chain の channel 終端を示します。
- `destPort` は受信先 chain の port を示します。
- `destChannel` は受信先 chain の channel 終端を示します。
- `data` は関連付けられた module のアプリケーションロジックによって定義される opaque な値です。

Note that a `Packet` is never directly serialised. Rather it is an intermediary structure used in certain function calls that may need to be created or processed by modules calling the IBC handler.

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

### Preliminaries

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

During the handshake process, two ends of a channel come to agreement on a version bytestring associated with that channel. The contents of this version bytestring are and will remain opaque to the IBC core protocol. Host state machines MAY utilise the version data to indicate supported IBC/APP protocols, agree on packet encoding formats, or negotiate other channel-related metadata related to custom logic on top of IBC.

Host state machines MAY also safely ignore the version data or specify an empty string.

### サブプロトコル

> Note: If the host state machine is utilising object capability authentication (see [ICS 005](../ics-005-port-allocation)), all functions utilising ports take an additional capability parameter.

#### 識別子の検証

Channels are stored under a unique `(portIdentifier, channelIdentifier)` prefix. The validation function `validatePortIdentifier` MAY be provided.

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

    // optimistic channel handshakes are allowed
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

Once closed, channels cannot be reopened and identifiers cannot be reused. Identifier reuse is prevented because we want to prevent potential replay of previously sent packets. The replay problem is analogous to using sequence numbers with signed messages, except where the light client algorithm "signs" the messages (IBC packets), and the replay prevention sequence is the combination of port identifier, channel identifier, and packet sequence - hence we cannot allow the same port identifier & channel identifier to be reused again with a sequence reset to zero, since this might allow packets to be replayed. It would be possible to safely reuse identifiers if timeouts of a particular maximum height/time were mandated & tracked, and future specification versions may incorporate this feature.

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
2. Establishment of a connection & channel, optimistic send, in order
    1. Connection opening handshake started from *A* to *B* by module *1* (see [ICS 3](../ics-003-connection-semantics))
    2. Channel opening handshake started from *1* to *2* using the newly created connection (this ICS)
    3. Packet sent over the newly created channel from *1* to *2* (this ICS)
3. Successful completion of handshakes (if either handshake fails, the connection/channel can be closed & the packet timed-out)
    1. Connection opening handshake completes successfully (see [ICS 3](../ics-003-connection-semantics)) (this will require participation of a relayer process)
    2. Channel opening handshake completes successfully (this ICS) (this will require participation of a relayer process)
4. Packet confirmation on machine *B*, module *2* (or packet timeout if the timeout height has passed) (this will require participation of a relayer process)
5. Acknowledgement (possibly) relayed back from module *2* on machine *B* to module *1* on machine *A*

Represented spatially, packet transit between two machines can be rendered as follows:

![Packet Transit](packet-transit.png)

##### Sending packets

The `sendPacket` function is called by a module in order to send an IBC packet on a channel end owned by the calling module to the corresponding module on the counterparty chain.

Calling modules MUST execute application logic atomically in conjunction with calling `sendPacket`.

The IBC handler performs the following steps in order:

- Checks that the channel & connection are open to send packets
- Checks that the calling module owns the sending port
- Checks that the packet metadata matches the channel & connection information
- Checks that the timeout height specified has not already passed on the destination chain
- Increments the send sequence counter associated with the channel
- Stores a constant-size commitment to the packet data & packet timeout

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment to the data & timeout value. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

```typescript
function sendPacket(packet: Packet) {
    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))

    // optimistic sends are permitted once the handshake has started
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))

    abortTransactionUnless(connection !== null)

    // sanity-check that the timeout height hasn't already passed in our local client tracking the receiving chain
    latestClientHeight = provableStore.get(clientPath(connection.clientIdentifier)).latestClientHeight()
    abortTransactionUnless(packet.timeoutHeight === 0 || latestClientHeight < packet.timeoutHeight)

    nextSequenceSend = provableStore.get(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel))
    abortTransactionUnless(packet.sequence === nextSequenceSend)

    // all assertions passed, we can alter state

    nextSequenceSend = nextSequenceSend + 1
    provableStore.set(nextSequenceSendPath(packet.sourcePort, packet.sourceChannel), nextSequenceSend)
    provableStore.set(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence),
                      hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // log that a packet has been sent
    emitLogEntry("sendPacket", {sequence: packet.sequence, data: packet.data, timeoutHeight: packet.timeoutHeight, timeoutTimestamp: packet.timeoutTimestamp})
}
```

#### Receiving packets

The `recvPacket` function is called by a module in order to receive & process an IBC packet sent on the corresponding channel end on the counterparty chain.

Calling modules MUST execute application logic atomically in conjunction with calling `recvPacket`, likely beforehand to calculate the acknowledgement value.

The IBC handler performs the following steps in order:

- Checks that the channel & connection are open to receive packets
- Checks that the calling module owns the receiving port
- Checks that the packet metadata matches the channel & connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive (for ordered channels)
- Checks that the timeout height has not yet passed
- Checks the inclusion proof of packet data commitment in the outgoing chain's state
- Sets the opaque acknowledgement value at a store path unique to the packet (if the acknowledgement is non-empty or the channel is unordered)
- Increments the packet receive sequence associated with the channel end (ordered channels only)

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

    // all assertions passed (except sequence check), we can alter state

    // always set the acknowledgement so that it can be verified on the other side
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

    // log that a packet has been received & acknowledged
    emitLogEntry("recvPacket", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight,
                                timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, acknowledgement})

    // return transparent packet
    return packet
}
```

#### Acknowledgements

The `acknowledgePacket` function is called by a module to process the acknowledgement of a packet previously sent by the calling module on a channel to a counterparty module on the counterparty chain. `acknowledgePacket` also cleans up the packet commitment, which is no longer necessary since the packet has been received and acted upon.

Calling modules MAY atomically execute appropriate application acknowledgement-handling logic in conjunction with calling `acknowledgePacket`.

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
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // verify we sent the packet and haven't cleared it out yet
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // abort transaction unless correct acknowledgement on counterparty chain
    abortTransactionUnless(connection.verifyPacketAcknowledgement(
      proofHeight,
      proof,
      packet.destPort,
      packet.destChannel,
      packet.sequence,
      acknowledgement
    ))

    // abort transaction unless acknowledgement is processed in order
    if (channel.order === ORDERED) {
      nextSequenceAck = provableStore.get(nextSequenceAckPath(packet.sourcePort, packet.sourceChannel))
      abortTransactionUnless(packet.sequence === nextSequenceAck)
      nextSequenceAck = nextSequenceAck + 1
      provableStore.set(nextSequenceAckPath(packet.sourcePort, packet.sourceChannel), nextSequenceAck)
    }

    // all assertions passed, we can alter state

    // delete our commitment so we can't "acknowledge" again
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

#### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves.

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

##### Sending end

The `timeoutPacket` function is called by a module which originally attempted to send a packet to a counterparty module, where the timeout height or timeout timestamp has passed on the counterparty chain without the packet being committed, to prove that the packet can no longer be executed and to allow the calling module to safely perform appropriate state transitions.

Calling modules MAY atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutPacket`.

In the case of an ordered channel, `timeoutPacket` checks the `recvSequence` of the receiving channel end and closes the channel if a packet has timed out.

In the case of an unordered channel, `timeoutPacket` checks the absence of an acknowledgement (which will have been written if the packet was received). Unordered channels are expected to continue in the face of timed-out packets.

If relations are enforced between timeout heights of subsequent packets, safe bulk timeouts of all packets prior to a timed-out packet can be performed. This specification omits details for now.

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
    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // check that timeout height or timeout timestamp has passed on the other end
    abortTransactionUnless(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && connection.getTimestampAtHeight(proofHeight) > packet.timeoutTimestamp))

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    if channel.order === ORDERED {
      // ordered channel: check that packet has not been received
      abortTransactionUnless(nextSequenceRecv <= packet.sequence)
      // ordered channel: check that the recv sequence is as claimed
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
    } else
      // unordered channel: verify absence of acknowledgement at packet index
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence
      ))

    // all assertions passed, we can alter state

    // delete our commitment
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    if channel.order === ORDERED {
      // ordered channel: close the channel
      channel.state = CLOSED
      provableStore.set(channelPath(packet.sourcePort, packet.sourceChannel), channel)
    }

    // return transparent packet
    return packet
}
```

##### Timing-out on close

The `timeoutOnClose` function is called by a module in order to prove that the channel to which an unreceived packet was addressed has been closed, so the packet will never be received (even if the `timeoutHeight` or `timeoutTimestamp` has not yet been reached).

```typescript
function timeoutOnClose(
  packet: Packet,
  proof: CommitmentProof,
  proofClosed: CommitmentProof,
  proofHeight: uint64,
  nextSequenceRecv: Maybe<uint64>): Packet {

    channel = provableStore.get(channelPath(packet.sourcePort, packet.sourceChannel))
    // note: the channel may have been closed
    abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // note: the connection may have been closed
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // check that the opposing channel end has closed
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
      // ordered channel: check that the recv sequence is as claimed
      abortTransactionUnless(connection.verifyNextSequenceRecv(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        nextSequenceRecv
      ))
      // ordered channel: check that packet has not been received
      abortTransactionUnless(nextSequenceRecv <= packet.sequence)
    } else
      // unordered channel: verify absence of acknowledgement at packet index
      abortTransactionUnless(connection.verifyPacketAcknowledgementAbsence(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence
      ))

    // all assertions passed, we can alter state

    // delete our commitment
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // return transparent packet
    return packet
}
```

##### Cleaning up state

Packets must be acknowledged in order to be cleaned-up.

#### Reasoning about race conditions

##### Simultaneous handshake attempts

If two machines simultaneously initiate channel opening handshakes with each other, attempting to use the same identifiers, both will fail and new identifiers must be used.

##### Identifier allocation

There is an unavoidable race condition on identifier allocation on the destination chain. Modules would be well-advised to utilise pseudo-random, non-valuable identifiers. Managing to claim the identifier that another module wishes to use, however, while annoying, cannot man-in-the-middle a handshake since the receiving module must already own the port to which the handshake was targeted.

##### Timeouts / packet confirmation

packet が受信前にタイムアウトの高さを通過したかどうかにかかわらず、packet のタイムアウトと packetの確認の間に競合状態はありません。

##### Man-in-the-middle attacks during handshakes

Verification of cross-chain state prevents man-in-the-middle attacks for both connection handshakes & channel handshakes since all information (source, destination client, channel, etc.) is known by the module which starts the handshake and confirmed prior to handshake completion.

##### Connection / channel closure with in-flight packets

If a connection or channel is closed while packets are in-flight, the packets can no longer be received on the destination chain and can be timed-out on the source chain.

#### Querying channels

Channels can be queried with `queryChannel`:

```typescript
function queryChannel(connId: Identifier, chanId: Identifier): ChannelEnd | void {
    return provableStore.get(channelPath(connId, chanId))
}
```

### Properties & Invariants

- The unique combinations of channel & port identifiers are first-come-first-serve: once a pair has been allocated, only the modules owning the ports in question can send or receive on that channel.
- Packets are delivered exactly once, assuming that the chains are live within the timeout window, and in case of timeout can be timed-out exactly once on the sending chain.
- The channel handshake cannot be man-in-the-middle attacked by another module on either blockchain or another blockchain's IBC handler.

## 後方互換性

該当しません。

## Forwards Compatibility

Data structures & encoding can be versioned at the connection or channel level. Channel logic is completely agnostic to packet data formats, which can be changed by the modules any way they like at any time.

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

Jun 5, 2019 - Draft submitted

Jul 4, 2019 - Modifications for unordered channels & acknowledgements

Jul 16, 2019 - Alterations for multi-hop routing future compatibility

Jul 29, 2019 - Revisions to handle timeouts after connection closure

Aug 13, 2019 - Various edits

Aug 25, 2019 - Cleanup

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
