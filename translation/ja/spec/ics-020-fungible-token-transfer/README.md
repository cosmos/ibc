---
ics: 20
title: Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Christopher Goes <cwgoes@interchain.berlin>
created: 2019-07-15
modified: 2020-02-24
---

## 概要

この標準化文書は、別々の chain 上の2つの module 間の IBC channel を介した 交換可能な token の転送のための packet データ構造、state machine 処理ロジック、およびエンコーディングの詳細を規定しています。提示された state machine ロジックは、許可なしの channel 開始による安全な multi-chain denomination 処理を可能にします。このロジックは、IBCルーティング module と host state machine 上の既存の asset 追跡 module との間のインターフェイスである "fungible token transfer bridge module” を構成しています。

### 動機

IBCプロトコルを介して接続された一連の chain の利用者は、発行元 chain の asset との互換性を維持しながら、交換やプライバシー保護などの追加機能を利用するために、ある chain で発行された asset を別の chain で利用したい場合があるかもしれません。このアプリケーション層の標準規格は、IBC で接続された chain 間で互換性のあるトークンを転送するためのプロトコルを記述します。それは asset の互換性を維持し、asset の所有権も維持し、ビザンチン障害の影響を制限し、追加の許可を必要としないものです。

### 定義

IBCハンドラインターフェースとIBCルーティングモジュールインターフェースは、それぞれ[ICS 25](../ics-025-handler-interface)と[ICS 26](../ics-026-routing-module)で定義されています。

### 望ましい特性

- 交換可能性の維持 (two-way peg).
- 総供給量の維持 (単一の発行元 chain と module 上での一定またはインフレ）.
- 許可の不要な token 交換、connection や module または denomination の許可リストが不要
- 対称 (すべての chain は同じロジックを実装し、hub と zone のプロトコル内の区別なし).
- 障害封じ込め: chain `B`のビザンチンな振る舞いの結果、chain `A`を起点とするトークンがビザンチンインフレーションとなるのを防ぎます(ただし、chain `B`にトークンを送ったユーザーはリスクを負う可能性があります)。

## 技術仕様

### データ構造

Only one packet data type, `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, receiving account, and whether the sending chain is the source of the asset, is required.

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
}
```

確認応答データ型は、転送が成功したかどうかと、失敗した理由（もしあれば）を説明します。

```typescript
interface FungibleTokenPacketAcknowledgement {
  success: boolean
  error: Maybe<string>
}
```

The fungible token transfer bridge module tracks escrow addresses associated with particular channels in state. Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
}
```

### サブプロトコル

The sub-protocols described herein should be implemented in a "fungible token transfer bridge" module with access to a bank module and to the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("bank", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
  claimCapability("port", capability)
}
```

<code>setup</code> 関数が呼び出されると、別の chain 上の fungible token transfer module のインスタンス間でIBCルーティング module を介して channel を作成することができます。

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### channel のライフサイクル管理

次の場合に限り、マシン`A`と`B`の両方とも、他のマシン上の任意の module からの新しい channel を受けつけます:

- 他の module が "bank" port にバインドされている。
- 作成される channel が順不同である。
- バージョン文字列が空である。

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
  // 順不同 channels のみ許可
  abortTransactionUnless(order === UNORDERED)
  // 相手先 chain の port が "bank" の channel のみを許可
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // version が "ics20-1" であること
  abortTransactionUnless(version === "ics20-1")
  // escrow address を割り当てる
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
  // 順不同 channels のみ許可
  abortTransactionUnless(order === UNORDERED)
  // version が "ics20-1" であること
  abortTransactionUnless(version === "ics20-1")
  abortTransactionUnless(counterpartyVersion === "ics20-1")
  // 相手先 chain の port が "bank" の channel のみを許可
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // escrow address を割り当てる
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // port は既に検証済み
  // version が "ics20-1" であること
  abortTransactionUnless(version === "ics20-1")
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // channel の確認を受け入れる, port は検証済み, version は検証済み
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // 処理は不要
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // 処理不要
}
```

##### packetの中継

わかりやすく説明すると、chain `A` と `B`の間には:

- source zone として動作する場合、bridge module は送信 chain 上で既存のローカル asset denomination を預託し、受信 chain上で voucher を発行します。
- sink zone として動作する場合, bridge module は送信 chain 上で local voucher を焼却し受信 chain 上でローカル asset denomination を取り戻します。
- packet がタイムアウトすると、ローカル asset が送信者に戻されるか、または発行済み voucher が送信者に適切に戻されます。
- Acknowledgement data is used to handle failures, such as invalid denominations or invalid destination accounts. Returning an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending chain to take appropriate action based on the nature of the failure.

`createOutgoingPacket`は、host state machine 上のアカウント所有者に固有の、適切な署名確認を行う module 内のトランザクションハンドラによって呼び出されなければなりません。

```typescript
function createOutgoingPacket(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  destPort: string,
  destChannel: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: uint64,
  timeoutTimestamp: uint64) {
  // 我々が source chain かどうかを決めるために denomination を調べる
  prefix = "{destPort}/{destChannel}"
  source = denomination.slice(0, len(prefix)) === prefix
  if source {
    // sender が source chain の場合: token を預託する
    // escrow account を決定する
    escrowAccount = channelEscrowAddresses[packet.sourceChannel]
    // source tokens を預託する (残高不足の場合は失敗する想定)
    bank.TransferCoins(sender, escrowAccount, denomination.slice(len(prefix)), amount)
  } else {
    // receiver が source chainの場合, vouchers を焼却する
    // denomination の受け取りを構成し, 正しさを確認する
    prefix = "{sourcePort}/{sourceChannel}"
    abortTransactionUnless(denomination.slice(0, len(prefix)) === prefix)
    // vouchers を焼却する (残高不足の場合は失敗する想定)
    bank.BurnCoins(sender, denomination, amount)
  }
  FungibleTokenPacketData data = FungibleTokenPacketData{denomination, amount, sender, receiver}
  handler.sendPacket(Packet{timeoutHeight, timeoutTimestamp, destPort, destChannel, sourcePort, sourceChannel, data}, getCapability("port"))
}
```

`onRecvPacket`は、この module 宛の packet を受信したときにこのルーティングモジュールによって呼び出されます。

```typescript
function onRecvPacket(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  // 我々が source chain かどうかを決めるために denomination を調べる
  prefix = "{packet.destPort}/{packet.destChannel}"
  source = denomination.slice(0, len(prefix)) === prefix
  // 標準の成功通知を作成する
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}
  if source {
    // sender が source の場合, receiver に vouchers を発行する (残高不足の場合は失敗する想定)
    err = bank.MintCoins(data.receiver, data.denomination, data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
  } else {
    // receiver が source chain の場合: token を返却する
    // escrow account を決定する
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // receiving denomination を作成し, 正しさを確認する
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    if (data.denomination.slice(0, len(prefix)) !== prefix)
      ack = FungibleTokenPacketAcknowledgement{false, "invalid denomination"}
    else {
      // receiver に token を返却する (残高不足の場合は失敗する想定)
      err = bank.TransferCoins(escrowAccount, data.receiver, data.denomination.slice(len(prefix)), data.amount)
      if (err !== nil)
        ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
    }
  }
  return ack
}
```

`onRecvPacket`は、この module 宛の packet を受信したときにこのルーティング module によって呼び出されます。

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // 転送が失敗した場合, token を払い戻す
  if (!ack.success)
    refundTokens(packet)
}
```

`onTimeoutPacket`は、この module から送信された packet がタイムアウトした(宛先 chain で受信されない)ときにルーティング module によって呼び出されます。

```typescript
function onTimeoutPacket(packet: Packet) {
  // packet がタイムアウトしたので, token を払い戻す
  refundTokens(packet)
}
```

`refundTokens` は、`onAcknowledgePacket` (失敗した場合)と`onTimeoutPacket` の両方で呼び出され、預託されたトークンを元の送信者に返却します。

```typescript
function refundTokens(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  prefix = "{packet.destPort}/{packet.destChannel}"
  source = data.denomination.slice(0, len(prefix)) === prefix
  if source {
    // sender が source chain の場合, token を返却する
    // escrow account を決定する
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // receiving denomination を作成し, 正しさを確認する
    // unescrow tokens back to sender
    bank.TransferCoins(escrowAccount, data.sender, data.denomination.slice(len(prefix)), data.amount)
  } else {
    // receiver が source chain の場合, voucher を発行する
    // receiving denomination を作成し, 正しさを確認する
    prefix = "{packet.sourcePort}/{packet.sourceChannel}"
    // we abort here because we couldn't have sent this packet
    // この packet を送信することができなかったので、ここで中断する
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // sender に voucher を発行する
    bank.MintCoins(data.sender, data.denomination, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // can't happen, only unordered channels allowed
}
```

#### 推論

##### 正しさ

この実装では、互換性と供給の両方を保持します。

Fungibility: If tokens have been sent to the counterparty chain, they can be redeemed back in the same denomination & amount on the source chain.

供給: ロックされていないトークンとして供給を再定義します。すべての送信-受信ペアの合計は正味ゼロになります。送信元 chain は供給を変更することができます。

##### Multi-chain の注意点

This specification does not directly handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B (and the denomination will be "{portOnD}/{channelOnD}/{portOnB}/{channelOnB}/denom"), chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

様々なパスで chain のネットワーク上を移動するすべての denomination を追跡するためには、各 denomination の “global” 送信元 chain を追跡するレジストリを実装することが、特定の chain にとって有用であるかもしれません。エンドユーザーサービスプロバイダ（ウォレット作成者など）は、UXを向上させるために、このようなレジストリを組み込むか、または、標準的な送信元 chain と読みやすい名前の独自のマッピングを保持することを望むかもしれません。

#### 任意の補足

- 各 chain は、ローカルでは短くユーザーフレンドリーなローカル denomination を使用するために、ルックアップテーブルを保持することを選択することができます。それらの denomination は送受信の際は長い通貨単位に変換されます。
- 接続できる他のマシンや確立できる channel に追加の制限が課される場合があります。

## 後方互換性

該当しません。

## 前方互換性

この初期の規格では、channel ハンドシェイクで "ics20-1 "バージョンを使用しています。

この規格の将来のバージョンでは、channel ハンドシェイクで異なるバージョンを使用し、packet データフォーマットと packet ハンドラのセマンティクスを安全に変更することができます。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 履歴

2019年7月15日 - ドラフトを書く

2019年7月29日 - 大きな改定; 整理

2019年8月25日 - 大きな改定; さらに整理

2020年2月3日 - 成功と失敗時の確認応答処理のための改訂

2020年2月24日 - ソースフィールドを推測するための改訂、バージョン文字列の追加

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
