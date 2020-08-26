---
ics: 23
title: Vector Commitments
stage: draft
required-by: 2, 24
category: IBC/TAO
kind: interface
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-08-25
---

## 概要

*vector commitment* は、要素のインデックス付きベクトル に対して束縛される定数サイズの commitment と、ベクトル内の任意のインデックスと要素に対する短い包含証明および/または非包含証明を生成する構成のことです。この仕様では、IBC プロトコルが用いる commitment 構成で要求される関数とプロパティを列挙します。特に、IBC で使用される commitment は、*位置的な束縛*であることが要求されます。すなわち、特定位置（インデックス）での値の存在/非存在証明ができなければなりません。

### 動機

ある chain 上で発生した特定の状態遷移を別の chain 上で検証できるという保証を提供するために、IBC は、state 内の特定の path における特定の値の包含または非包含を証明するための、効率的で暗号論的な構成を必要とします。

### 定義

vector commitment の *管理者* は、commitment から要素を追加したり削除したりする能力と責務を持った actor です。通常これは blockchain の state machine になります。

*証明者* は、特定の要素の包含または非包含の証明を生成する責務を持つ actor です。通常これは relayer となります（[ICS 18](.../ics-018-relayer-algorithms)を参照）。

*検証者* は、commitment の管理者が特定の要素を追加したかどうかを検証するために、proof をチェックする actor です。通常これは別の chain 上で実行される IBC handler（IBC を実装した module）になります。

commitment は特定の *path* と *値* 型で初期化され、 これらは任意のシリアライズ可能なデータであることが前提となります。

*無視可能関数*とは、[ここ](https://en.wikipedia.org/wiki/Negligible_function)で定義されているように、すべての正の多項式の逆数よりもゆっくりと成長する関数のことです。

### 望ましい特性

このドキュメントでは望ましい特性のみを定義し、具体的な実装は定義しません —  以降の「特性」を参照してください。

## 技術仕様

### データ型

commitment 構成は以下のデータ型を指定しなければなりません。これらのデータ型は指定されない場合は opaque （検査される必要がない）ですが、シリアライズ可能でなければなりません。

#### Commitment State

`CommitmentState` は commitment の完全な state で、manager によって保管されます。

```typescript
type CommitmentState = object
```

#### Commitment Root

`CommitmentRoot` は特定の commitment state にコミットします。定数サイズであるべきです。

定数サイズの state を用いる commitment 構成では、`CommitmentState` と `CommitmentRoot` は同じ型になるかもしれません。

```typescript
type CommitmentRoot = object
```

#### Commitment Path

`CommitmentPath` は commitment proof を検証するのに用いる path で、（commitment type によって定義された）所定の構造化オブジェクトでも良いです。（以下で定義される）`applyPrefix` によって計算される必要があります。

```typescript
type CommitmentPath = object
```

#### Prefix

`CommitmentPrefix` は commitment proof の保管用接頭辞を定義します。path が proof 検証関数に渡される前に、path に対して適用されます。

```typescript
type CommitmentPrefix = object
```

`applyPrefix` 関数は引数から新しい commitment path を構築します。path 引数は prefix 引数の文脈に合わせて解釈されます。

2つの `(prefix, path)` タプルに対して、 `applyPrefix(prefix, path)` はそれらのタプルの要素が等しい場合に限って同じキーを返さなくてはなりません。

`applyPrefix` は `path`ごとに実装される必要があります。これは `path` が異なる具象構造を持てるためです。`applyPrefix` は複数の `CommitmentPrefix` 型に対応してもよいです。

`applyPrefix` によって返される `CommitmentPath` はシリアライズ可能である必要はありません（例えば、ツリーノード識別子のリストであるなど）が、等値比較ができる必要はあります。

```typescript
type applyPrefix = (prefix: CommitmentPrefix, path: Path) => CommitmentPath
```

#### Proof

`CommitmentProof` は、ある要素または要素集合が存在しているか存在していないかを示し、既知の commitment root と組み合わせて検証が可能です。証明は簡潔（succinct）でなければなりません。

```typescript
type CommitmentProof = object
```

### 必須となる関数

commitment 構成は、path を シリアライズ可能なオブジェクト、値を byte 配列として定義し、以降の関数を提供しなければなりません。

```typescript
type Path = string

type Value = []byte
```

#### 初期化

`generate` 関数は path から value への 初期（おそらく空の）map による commitment の state を初期化します。

```typescript
type generate = (initial: Map<Path, Value>) => CommitmentState
```

#### Root 計算

`calculateRoot` 関数は proof 検証に用いる commitment state への 定数サイズの commitment を計算します。

```typescript
type calculateRoot = (state: CommitmentState) => CommitmentRoot
```

#### 要素の追加と削除

`set` 関数は commitment 内に value への path を追加します。

```typescript
type set = (state: CommitmentState, path: Path, value: Value) => CommitmentState
```

`remove` 関数は commitment から path とそれに関連付けられた value を取り除きます。

```typescript
type remove = (state: CommitmentState, path: Path) => CommitmentState
```

#### Proof 生成

`createMembershipProof` 関数は、特定の commitment path が commitment 内の特定の value にセットされているという proof を生成します。

```typescript
type createMembershipProof = (state: CommitmentState, path: CommitmentPath, value: Value) => CommitmentProof
```

`createNonMembershipProof` 関数は、commitment path が commitment 内のどの value にもセットされていないという proof を生成します。

```typescript
type createNonMembershipProof = (state: CommitmentState, path: CommitmentPath) => CommitmentProof
```

#### Proof 検証

`verifyMembership` 関数は、path が commitment 内の特定の value にセットされているという proof を検証します。

```typescript
type verifyMembership = (root: CommitmentRoot, proof: CommitmentProof, path: CommitmentPath, value: Value) => boolean
```

`verifyNonMembership` 関数は、path が commitment 内のいかなる value にもセットされていないという proof を検証します。

```typescript
type verifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, path: CommitmentPath) => boolean
```

### オプション関数

commitment 構成は、以下の関数を提供してもよいです。

`batchVerifyMembership` 関数は、多くの path が commitment 内の個別の値にセットされているという proof を検証します。

```typescript
type batchVerifyMembership = (root: CommitmentRoot, proof: CommitmentProof, items: Map<CommitmentPath, Value>) => boolean
```

`batchVerifyNonMembership` 関数は、多くの path が commitment 内のどの値にもセットされていないという proof を検証します。

```typescript
type batchVerifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, paths: Set<CommitmentPath>) => boolean
```

もしこれらの関数が定義されている場合は、それぞれ `verifyMembership` の結合、`verifyNonMembership` の結合と同じ結果にならなければなりません（効率性は異なるかもしれません）。

```typescript
batchVerifyMembership(root, proof, items) ===
  all(items.map((item) => verifyMembership(root, proof, item.path, item.value)))
```

```typescript
batchVerifyNonMembership(root, proof, items) ===
  all(items.map((item) => verifyNonMembership(root, proof, item.path)))
```

バッチ検証が可能で、要素ごとに1つの proof を個別に検証するよりも効率的な場合、commitment 構成はバッチ検証関数を定義すべきです。

### 特性と不変条件

commitment は*完全で*、*健全であり*、*位置に束縛されてい*なければなりません。これらの特性は安全性のパラメータ `k`に関して定義されています。管理者、証明者、検証者によって合意されていなければなりません（そして、通例 commitment アルゴリズムが定数時間であることにも）。

#### 完全性

commitment proof は *完全で*なければなりません。commitment に追加される path => 値 のマッピングは、`k` において無視できるほど低確率の場合を除いて、常に包含されていることが証明でき、また包含されていない path は常に除外されていることが証明できます。

commitment `acc` 内で値 `value` に最後に設定された任意の接頭辞 `prefix` と任意の path `path`について、以下のようになります。

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, applyPrefix(prefix, path), value)
```

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === false) negligible in k
```

commitment `acc` 内でセットされなかった任意の接頭辞 `prefix` と任意のpath `path` について、`proof` の全ての値と `value` の全ての値について、以下のようになります。

```typescript
root = getRoot(acc)
proof = createNonMembershipProof(acc, applyPrefix(prefix, path))
```

```
Probability(verifyNonMembership(root, proof, applyPrefix(prefix, path)) === false) negligible in k
```

#### 健全性

commitment proof は*健全で*なければなりません。設定可能な安全性パラメータ `k` において無視できるほど低い確率の場合を除いて、commitment に追加されていない path => 値のマッピングは包含されていることは証明できず、また commitment に追加された path が除外されていることは証明できません。

commitment `acc` 内で値 `value` に最後にセットされた任意の prefix `prefix` と 任意の path `path` に関して、`proof` の全ての値について以下のとおりです。

```
Probability(verifyNonMembership(root, proof, applyPrefix(prefix, path)) === true) negligible in k
```

commitment `acc` 内でセットされていない任意の prefix `prefix` と任意の path `path` に関して、`proof` の全ての値と `value` の全ての値について以下のとおりです。

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === true) negligible in k
```

#### 位置への束縛

commitment proof は*位置に束縛されてい*なければなりません。与えられた commitment path は1つの値にのみマップされ、k において無視できるほど低い確率の場合を除いて、commitment proof は同じ path が異なる値に開かれていると証明することはできません。

commitment `acc` 内でセットされる任意の prefix ` prefix` と任意の path `path` に関して、以下を満たす1つの `value` が存在します。

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, applyPrefix(prefix, path), value)
```

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === false) negligible in k
```

`value !== otherValue` を満たす他の全ての値 `otherValue` に関して、`proof` の全ての値について以下のとおりです。

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), otherValue) === true) negligible in k
```

## 後方互換性

該当しません。

## 前方互換性

commitment アルゴリズムは確定される想定です。新しいアルゴリズムの導入は、connection と channelをバージョニング管理することによって行われる可能性があります。

## 実装例

まもなく公開予定。

## その他の実装

まもなく公開予定。

## 変更履歴

セキュリティの定義は、ほとんどがこれらの論文から引用されています（多少簡略化されています）。

- [Vector Commitments and their Applications](https://eprint.iacr.org/2011/495.pdf)
- [Commitments with Applications to Anonymity-Preserving Revocation](https://eprint.iacr.org/2017/043.pdf)
- [Batching Techniques for Commitments with Applications to IOPs and Stateless Blockchains](https://eprint.iacr.org/2018/1188.pdf)

Dev Ojha には、この仕様に関する幅広いコメントをいただきました。

2019年4月25日 - ドラフトを提出

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
