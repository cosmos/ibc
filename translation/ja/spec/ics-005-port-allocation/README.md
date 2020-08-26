---
ics: 5
title: Port Allocation
stage: Draft
requires: 24
required-by: 4
category: IBC/TAO
kind: interface
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-06-20
modified: 2019-08-25
---

## 概要

この規格は port 割り当てシステムを規定します。このシステムによって module は、IBC handler によって割り当てられる一意に命名された port にバインドできます。port は channel を開くのに使われ、最初にバインドした module によって移転されたり、後で解放されたりすることができます。

### 動機

IBC プロトコルは module 間の往来を容易にするために設計されています。module は独立しており、相互に信頼されない可能性があり、独立統治されている台帳上で実行されるコードを備えています。望ましいエンドツーエンドのセマンティクスを提供するために、IBC handler は特定の module に channel を許可する必要があります。この仕様では、上記のモデルを実現する *port 割り当てと所有権*システムを定義します。

module ロジックがどのような port 名にバインドされるかについては規約があるかもしれません。例えば、代替可能トークンを処理するための "bank" や、chain 間担保のための "staking" など。これは HTTP サーバでポート 80 が一般的に使用されていることに似ています。プロトコルは、特定の module ロジックが実際に従来の port にバインドされているかどうかを強制できません。したがって user は自身でチェックする必要があります。疑似ランダムな識別子を持つエフェメラル port は一時的なプロトコル処理のために作成されるかもしれません。

module は複数の port にバインドし、別の machine 上の他の module によってバインドされた複数の port に接続することができます。任意の数の（一意に識別された）channel が同時に1つの port を利用することができます。channel は2つの port 間でエンドツーエンドであり、それぞれの port は前もって module にバインドされている必要があります。その後、module は channel 終端をコントロールします。

オプションとして、ホスト state machine は、port をバインドする能力に特化した capability キーを生成することで、特別に許可された module マネージャのみに対して port バインディングを公開するようにできます。module マネージャは、module がどの port にバインドできるかをカスタムルールセットで制御し、port 名と module を検証した場合にのみ、module に port を移転することができます。この役割は routing module で果たすことができます（[ICS 26](.../ics-026-routing-module) を参照）。

### 定義

`Identifier`、`get`、`set`、`delete` は [ICS 24](../ics-024-host-requirements) で定義されます。

*port* は channel の開始を許可する際や module によって利用される、個別の識別子の一種です。

*module*は、IBC handler から独立したホスト state machineのサブコンポーネントです。例としては、Ethereum スマートコントラクトや Cosmos SDK & Substrate モジュールなどがあります。IBC 仕様では、ホスト state machine が module に port を許可するために object-capablitity やソース認証を使用すること以外、module の機能性については何も仮定しません。

### 望ましい特性

- 一度 module が port にバインドすると、module がそれを解放するまで他の module はその port を使用することができません。
- module は、オプションで port を解放したり、別の module に移転したりすることができます。
- 1つの module は複数の port に対して一度にバインドすることができます。
- port は先着順に割り当てられ、既知の module 用の「予約済み」port は、該当 chain が最初に開始されたときにバインドすることができます。

参考になりそうな比較として、次の TCP アナロジーは概ね正しいです。

IBC 概念 | TCP/IP 概念 | 違い
--- | --- | ---
IBC | TCP | 多くの場合、IBC を記述したアーキテクチャドキュメントを参照してください。
Port （例 "bank"） | Port （例 80） | 小さい数値を用いた予約済み port はなく、port 自体が文字列です。
Module （例 "bank"） | Application （例 <br> Nginx） | アプリケーション固有です。
Client | - | 直接的なアナロジーはありませんが、L2 ルーティングや TLS にやや似ています。
Connection | - | 直接的なアナロジーはなく、TCP の接続に畳み込まれています。
Channel | Connection | port との間で、任意の数の channel を同時に開くことができます。

## 技術仕様

### データ構造

ホスト state machine は、module の object-capability への参照、あるいはソース認証をサポートしなければなりません。

前者の場合、IBC handler は *object-capability* を生成する能力を持っていなければなりません。object-capability とは、module に渡すことができ、他の module で重複しない、一意で opaque な参照のことです。2つ例をあげると、1つは Cosmos SDK（[参照](https://github.com/cosmos/cosmos-sdk/blob/97eac176a5d533838333f7212cbbd79beb0754bc/store/types/store.go#L275)）が用いる保管キーで、もう1つは Agoric の Javascript ランタイムが用いているオブジェクト参照（[参照](https://github.com/Agoric/SwingSet)）です。

```typescript
type CapabilityKey object
```

`newCapability` は、受け取った名前がローカルで capability キーにマップされ、後で `getCapability` で使用できるよう、名前から一意の capability キーを生成する必要があります。

```typescript
function newCapability(name: string): CapabilityKey {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`authenticateCapability` は名前と capability を受け取り、名前がローカルで capability にマップされているかどうかを確認しなければなりません。 名前は信頼できないユーザ入力です。

```typescript
function authenticateCapability(name: string, capability: CapabilityKey): bool {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`claimCapability` は名前と（他の module から与えられた） capablity を受け取って、ローカルで capability に名前をマップし、後で用いるために「主張」できなければなりません。

```typescript
function claimCapability(name: string, capability: CapabilityKey) {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`getCapability` は、名前によって以前作成されたか主張された capability を module が探索できるようにしなければなりません。

```typescript
function getCapability(name: string): CapabilityKey {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`releaseCapability` は module が 自身の所有する capability を解放できるようにしなければなりません。

```typescript
function releaseCapability(capability: CapabilityKey) {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

後者のソース認証の場合には、IBC handler は 呼び出している module の *source 識別子* を安全に読み出せる必要があります。source 識別子はホスト state machine 内の各 module に対する一意の文字列で、module 自身が変更したり他の module が偽造したりできません。例として Ethereum が用いているスマートコントラクトアドレス（[参照](https://ethereum.github.io/yellowpaper/paper.pdf)）が挙げられます。

```typescript
type SourceIdentifier string
```

```typescript
function callingModuleIdentifier(): SourceIdentifier {
  // provided by host state machine, e.g. contract address in Ethereum
}
```

`newCapability`、`authenticateCapability`、`claimCapability`、`getCapability`、`releaseCapability` は次のように実装されます。

```
function newCapability(name: string): CapabilityKey {
  return callingModuleIdentifier()
}
```

```
function authenticateCapability(name: string, capability: CapabilityKey) {
  return callingModuleIdentifier() === name
}
```

```
function claimCapability(name: string, capability: CapabilityKey) {
  // no-op
}
```

```
function getCapability(name: string): CapabilityKey {
  // not actually used
  return nil
}
```

```
function releaseCapability(capability: CapabilityKey) {
  // no-op
}
```

#### 保管 path

`portPath` は `Identifier` を受け取り、object-capability 参照や port に関連付いた所有者 module 識別子が保管されているべき保管 path を返します。

```typescript
function portPath(id: Identifier): Path {
    return "ports/{id}"
}
```

### サブプロトコル

#### 識別子の検証

port の所有者 module 識別子は、一意の `Identifier` 接頭辞の下に格納されます。検証関数 `validatePortIdentifier` が提供されてもよいです。

```typescript
type validatePortIdentifier = (id: Identifier) => boolean
```

提供されない場合、デフォルトの `validatePortIdentifier` 関数は常に `true` を返すでしょう。

#### port へのバインド

IBC handler は `bindPort` を実装しなければなりません。`bindPort` は未割り当ての port をバインドします。その port が既に割り当てられている場合には失敗します。

ホスト state machine が port 割り当てをコントロールする特別な module マネージャを実装しない場合は、`bindPort` はすべての module に利用可能でなければなりません。module マネージャが実装される場合、 `bindPort` は その module マネージャからのみ呼び出されるようにすべきです。

```typescript
function bindPort(id: Identifier): CapabilityKey {
    abortTransactionUnless(validatePortIdentifier(id))
    abortTransactionUnless(getCapability(portPath(id)) === null)
    capability = newCapability(portPath(id))
    return capability
}
```

#### port の所有権譲渡

ホスト state machine が object-capability をサポートする場合、port 参照は持ち主の capability であるので、追加プロトコルは不要です。

#### port の解放

IBC handler は `releasePort` 関数を実装する必要があります。この関数は module が port を 解放でき、後で他の module がその port をバインドできるようにします。

`releasePort` はすべての module から利用できるべきです。

> 警告: port を解放すると、他の module がその port にバインドすることが可能になり、受信 channel の開始ハンドシェイクを傍受する可能性があります。module は安全な場合にのみ port を解放すべきです。

```typescript
function releasePort(capability: CapabilityKey) {
    abortTransactionUnless(authenticateCapability(portPath(id), capability))
    releaseCapability(capability)
}
```

### 特性と不変条件

- デフォルトでは port 識別子は先着順です。module が port にバインドすると、その module が その port を移転するか解放するまで専有します。module マネージャはこれを上書きするカスタムロジックを実装することができます。

## 後方互換性

該当しません。

## 前方互換性

port のバインドは wire プロトコルではないので、所有権のセマンティクスが影響を受けない限り、インタフェースは別々の chain で独立して変更することができます。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

2019年1月29日 - 最初のドラフト

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
