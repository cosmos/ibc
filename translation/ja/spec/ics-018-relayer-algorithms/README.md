---
ics: 18
title: Relayer Algorithms
stage: draft
category: IBC/TAO
kind: interface
requires: 24, 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## 概要

relayer アルゴリズムは IBC の「物理的」な connection 層 — 各 chain の state をスキャンし、適切な datagram を構築し、プロトコルによって許可された相手側 chain 上で実行することで、IBC プロトコルを実行している2つの chain 間でのデータ中継を担当するオフチェーンプロセスです。

### モチベーション

IBC プロトコルでは、ブロックチェーンは特定のデータを他の chain に送信するための*意図*を記録することができるだけで、ネットワークのトランスポート層に直接アクセスすることはできません。物理的な datagram の中継は、TCP/IP のようなトランスポート層へのアクセスを持つオフチェーンのインフラによって行われなければなりません。この規格では、上記の中継を実行するために、chain の state を照会する機能を持つオフチェーンプロセスによって実行可能な *relayer* アルゴリズムの概念を定義しています。

### 定義

*relayer* は、IBC プロトコルを利用して、トランザクションの状態を読み取り、いくつかの台帳にトランザクションを送信する機能を持つオフチェーンプロセスです。

### 望ましい特性

- IBC における、正確に1度だけ、または、配信もしくはタイムアウトといった安全特性は、relayer の動作に依存しません（ビザンチンな relayer を想定）。
- IBC の packet 中継 に対する liveness な性質は、正常に動作中の relayer が少なくとも1つ存在していることにのみ依存します。
- 中継はパーミッションレスであるべきであり、必要な検証はすべてオンチェーンで行われるべきです。
- IBC ユーザーと relayer 間に必要な通信は最小限に抑えるべきです。
- relayer インセンティブの提供は、アプリケーション層で可能とするべきです。

## 技術仕様

### 基本的な relayer アルゴリズム

relayer アルゴリズムは IBC プロトコルを実装した chain 集合 `C` に対して定義されます。各 relayer は chain 間ネットワーク内のすべての chain に対して state を読み取ったり datagram を書き込んだりできるとは限りません(特に許可された chain やプライベート chain の場合)。 — 異なる relayer はネットワーク内の異なる部分集合間を中継します。

`pendingDatagrams` は両方の chain の state に基づいて、ある chain から別の chain に中継されるべきすべての有効な datagram を計算します。relayer は中継対象のブロックチェーンが IBC プロトコルのどのサブセットを実装しているか事前に知っていなければなりません（例えば、ソースコードを読むなど）。以下に例を示します。

`submitDatagram` は chain ごとに定義された、（何らかのトランザクションを提出する）プロシージャです。chain がサポートしていれば、複数の datagram を個々のトランザクションとして個別に、または単一のトランザクションとしてアトミックに送信することができます。

`relay` は relayer によって頻繁に呼び出されます — どちらの chain でもブロックごとに1回以下の頻度で、relayer がどれくらいの頻度で中継したいかによっては、より少ない頻度になります。

異なる relayer は異なる chain 間で中継することができます —  各 chain のペアが少なくとも1つの正しく live な relayer を持ち、chain が live のままである限り、ネットワーク内の chain 間を流れるすべての packet は最終的に中継されます。

```typescript
function relay(C: Set<Chain>) {
  for (const chain of C)
    for (const counterparty of C)
      if (counterparty !== chain) {
        const datagrams = chain.pendingDatagrams(counterparty)
        for (const localDatagram of datagrams[0])
          chain.submitDatagram(localDatagram)
        for (const counterpartyDatagram of datagrams[1])
          counterparty.submitDatagram(counterpartyDatagram)
      }
}
```

### packet、通知、タイムアウト

#### 順序付けられた channel での packet の中継

順序付けされた channel の packet はイベントベースの方法かクエリベースの方法のいずれかで中継されます。前者の場合 relayer は packet が送信されるたびに発せられる送信元 chain のイベントを監視し、イベントログのデータを使用して packet を構成します。後者の場合 relayer は定期的に発信元 chain 上の送信シーケンスをクエリし、最後にリレーされたシーケンス番号を保持することで、それらの間にあるどのシーケンスも、クエリされ中継される必要のある packet になります。いずれの場合も、その後 relayer プロセスは、受信シーケンスをチェックすることで、宛先 chain が packet をまだ受信していないことを確認し、その packet を中継します。

#### 順序付けられていない channel での packet 中継

順序付けられていない channel の packet はイベントベースの方法で中継することができます。relayer は packet が送信されるたびに発せられるイベントを送信元 chain で監視し、イベントログのデータを使用して packet を構成します。その後 relayer は packet のシーケンス番号で確認応答があるかどうかを問い合わせることで、宛先 chain が既に packet を受信しているかどうかをチェックし、まだ確認応答がない場合は packet を中継しなければなりません。

#### 確認応答の中継

確認応答はイベントベースの方法で中継することができます。relayer は packet を受信して確認応答が書き込まれたときに宛先 chainが発するイベントを監視し、イベントログのデータを使って確認応答を作成し、送信元 chain 上に packet commitmentがまだ存在するかどうかをチェックし（確認応答が中継されると削除されます）、存在する場合は発信元 chain に確認応答を中継します。

#### タイムアウトの中継

タイムアウトの中継は packet がタイムアウトした場合に特定のイベントが発生しないため少々複雑です。タイムアウトのブロック高さやタイムスタンプは宛先 chain へ渡されるので、packet がもはや中継されないような場合は packet がタイムアウトしています。<br>relayer プロセスは（イベントログをスキャンすることによって構築可能な） packet 追跡を行わねばならず、追跡中の packet が宛先 chain のブロック高さやタイムスタンプを超えるとすぐに、送信元 chainの packet commitmentがまだ存在しているかどうかを確認し（タイムアウトが中継されるとすぐに削除されます）、もしまだ存在していれば送信元 chain にタイムアウトを中継しなければなりません。

### datagram の保留

`pendingDatagrams` はある machine から別の machine に送信される datagram を照合します。この関数の実装は両方の machine がサポートする IBC プロトコルのサブセットと、送信元 machine の state machine レイアウトに依存します。特定の relayer は中継される可能性のある datagram のサブセットだけを中継するために、独自のフィルタ関数を実装したいと思うでしょう（例えば、何らかのオフチェーンの方法で中継するために支払われた datagram サブセット)。

2つの chain 間で単方向での中継を行う実装例を以下に示します。この例は `chain` と `counterparty` を切り替えることで双方向での中継を行うように変更することができます。relayer プロセスがどの datagram を担当するかは柔軟に選択できます。この例では、relayer は `chain` で開始したすべてのハンドシェイクを中継し（両 chain に datagramを送信）、`chain` から`counterparty` に送信されたすべての packet を中継し、`counterparty` から `chain` に送信された packet のすべての確認応答を中継します。

```typescript
function pendingDatagrams(chain: Chain, counterparty: Chain): List<Set<Datagram>> {
  const localDatagrams = []
  const counterpartyDatagrams = []

  // ICS2 : Clients
  // - Determine if light client needs to be updated (local & counterparty)
  height = chain.latestHeight()
  client = counterparty.queryClientConsensusState(chain)
  if client.height < height {
    header = chain.latestHeader()
    counterpartyDatagrams.push(ClientUpdate{chain, header})
  }
  counterpartyHeight = counterparty.latestHeight()
  client = chain.queryClientConsensusState(counterparty)
  if client.height < counterpartyHeight {
    header = counterparty.latestHeader()
    localDatagrams.push(ClientUpdate{counterparty, header})
  }

  // ICS3 : Connections
  // - Determine if any connection handshakes are in progress
  connections = chain.getConnectionsUsingClient(counterparty)
  for (const localEnd of connections) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    if (localEnd.state === INIT &&
          (remoteEnd === null || remoteEnd.state === INIT))
      // Handshake has started locally (1 step done), relay `connOpenTry` to the remote end
      counterpartyDatagrams.push(ConnOpenTry{
        desiredIdentifier: localEnd.counterpartyConnectionIdentifier,
        counterpartyConnectionIdentifier: localEnd.identifier,
        counterpartyClientIdentifier: localEnd.clientIdentifier,
        counterpartyPrefix: localEnd.commitmentPrefix,
        clientIdentifier: localEnd.counterpartyClientIdentifier,
        version: localEnd.version,
        counterpartyVersion: localEnd.version,
        proofInit: localEnd.proof(),
        proofConsensus: localEnd.client.consensusState.proof(),
        proofHeight: height,
        consensusHeight: localEnd.client.height,
      })
    else if (localEnd.state === INIT && remoteEnd.state === TRYOPEN)
      // Handshake has started on the other end (2 steps done), relay `connOpenAck` to the local end
      localDatagrams.push(ConnOpenAck{
        identifier: localEnd.identifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(),
        proofConsensus: remoteEnd.client.consensusState.proof(),
        proofHeight: remoteEnd.client.height,
        consensusHeight: remoteEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake has confirmed locally (3 steps done), relay `connOpenConfirm` to the remote end
      counterpartyDatagrams.push(ConnOpenConfirm{
        identifier: remoteEnd.identifier,
        proofAck: localEnd.proof(),
        proofHeight: height,
      })
  }

  // ICS4 : Channels & Packets
  // - Determine if any channel handshakes are in progress
  // - Determine if any packets, acknowledgements, or timeouts need to be relayed
  channels = chain.getChannelsUsingConnections(connections)
  for (const localEnd of channels) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    // Deal with handshakes in progress
    if (localEnd.state === INIT &&
          (remoteEnd === null || remoteEnd.state === INIT))
      // Handshake has started locally (1 step done), relay `chanOpenTry` to the remote end
      counterpartyDatagrams.push(ChanOpenTry{
        order: localEnd.order,
        connectionHops: localEnd.connectionHops.reverse(),
        portIdentifier: localEnd.counterpartyPortIdentifier,
        channelIdentifier: localEnd.counterpartyChannelIdentifier,
        counterpartyPortIdentifier: localEnd.portIdentifier,
        counterpartyChannelIdentifier: localEnd.channelIdentifier,
        version: localEnd.version,
        counterpartyVersion: localEnd.version,
        proofInit: localEnd.proof(),
        proofHeight: height,
      })
    else if (localEnd.state === INIT && remoteEnd.state === TRYOPEN)
      // Handshake has started on the other end (2 steps done), relay `chanOpenAck` to the local end
      localDatagrams.push(ChanOpenAck{
        portIdentifier: localEnd.portIdentifier,
        channelIdentifier: localEnd.channelIdentifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(),
        proofHeight: localEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake has confirmed locally (3 steps done), relay `chanOpenConfirm` to the remote end
      counterpartyDatagrams.push(ChanOpenConfirm{
        portIdentifier: remoteEnd.portIdentifier,
        channelIdentifier: remoteEnd.channelIdentifier,
        proofAck: localEnd.proof(),
        proofHeight: height
      })

    // Deal with packets
    // First, scan logs for sent packets and relay all of them
    sentPacketLogs = queryByTopic(height, "sendPacket")
    for (const logEntry of sentPacketLogs) {
      // relay packet with this sequence number
      packetData = Packet{logEntry.sequence, logEntry.timeoutHeight, logEntry.timeoutTimestamp,
                          localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketRecv{
        packet: packetData,
        proof: packet.proof(),
        proofHeight: height,
      })
    }
    // Then, scan logs for received packets and relay acknowledgements
    recvPacketLogs = queryByTopic(height, "recvPacket")
    for (const logEntry of recvPacketLogs) {
      // relay packet acknowledgement with this sequence number
      packetData = Packet{logEntry.sequence, logEntry.timeoutHeight, logEntry.timeoutTimestamp,
                          localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketAcknowledgement{
        packet: packetData,
        acknowledgement: logEntry.acknowledgement,
        proof: packet.proof(),
        proofHeight: height,
      })
    }
  }

  return [localDatagrams, counterpartyDatagrams]
}
```

relayer は特定の client、特定の connection、特定の channel、あるいは特定の種類の packet を中継するために、これらの datagram をフィルタリングしても良いでしょう。 ひょっとすると fee 支払いモデル（ケースによって異なるのでこの文書では指定していませんが）に沿ったフィルタリングになったりするかもしれません。

### 順序付けの制約

どの datagram がどの順番で送信されなければならないかを決定する relayer プロセスには、暗黙の順序制約があります。例えば packet を中継する前に、light client 内に保存された特定のブロック高さへの consensus state と commitment root が finality を得るために、header が提出されなければなりません。relayer プロセスは、いつ何を中継しなければならないかを決定するために、中継している chain の state を頻繁に問い合わせる責任があります。

### バンドル

ホスト state machine がサポートしている場合、relayer プロセスは多くの datagram を単一のトランザクションに束ねることができ、それによってそれらが順番に実行され、オーバーヘッドコスト（例えば、fee 支払いのための署名チェック）を償却することができます。

### 競合条件

同じペアの module と chain 間で中継する複数の relayer は、同時に同じ packet を中継しようとするかもしれません（または同じ header を送信しようとするかもしれません）。2つの relayer がそうした場合、最初のトランザクションは成功し、2番目のトランザクションは失敗します。これを緩和するためには、relayer 間、または元の packet を送った actor と relayer 間での外の枠組みでの調整が必要になります。これ以上の議論はこの標準規格の範囲外です。

### インセンティブ

relayer プロセスはトランザクション fee を支払うのに十分な残高を持った両 chain 上のアカウントへの アクセス権を持たなければなりません。relayer はこれらの fee を回収するためにアプリケーションレベルの方法、例えば packet データに自身への小額の支払いを含めるといった方法を取り入れても良いでしょう — relayer fee 支払いのためのプロトコルは、本 ICS の将来のバージョンまたは別の ICS で記述される予定です。

relayer プロセスはいくつでも安全に並行実行できるかもしれません（実際、別個の relayer が chain 集合の別個のサブセットにサービスを提供することが想定されています)。しかし、同じ proof を複数回提出すると不必要な fee が発生する可能性があるため、最小限の調整が行われると理想的かもしれません（特定の packet に特定の relayer を割り当てたり、保留中のトランザクションのために mempool をスキャンしたりするなど)。

## 後方互換性

該当しません。relayer 処理はオフチェーンであり、必要に応じてアップグレードやダウングレードが可能です。

## 前方互換性

該当しません。relayer 処理はオフチェーンであり、必要に応じてアップグレードやダウングレードが可能です。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

2019年3月30日 - 最初のドラフトを提出

2019年4月15日 - 書式と明確さのための改訂

2019年4月23日 - コメントからの改定、ドラフトをマージ

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
