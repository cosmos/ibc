# 1：IBCの用語

**これは、IBC仕様で使用される用語の概要です。**

**アーキテクチャ概要は、[こちら](./2_IBC_ARCHITECTURE.md)を参照してください。**

**大まかなプロトコル設計原則については、[こちら](./3_IBC_DESIGN_PRINCIPLES.md)を参照してください。**

**ユースケース例については、[こちら](./4_IBC_USECASES.md)を参照してください。**

**デザインパターンの議論については、[こちら](./5_IBC_DESIGN_PATTERNS.md)を参照してください。**

このドキュメントは、IBC仕様セット全体で使用される主要な用語の平易な日本語での定義を提供します。

## 抽象化の定義

### Actor

*actor* または *user* （互換的に使用）は、IBC プロトコルと対話するエンティティです。actor は、人間のエンドユーザー、ブロックチェーンで実行されている module またはスマートコントラクト、またはトランザクションに署名できるオフチェーンの relayer プロセスです。

### Machine / Chain / Ledger

*machine*、*chain*、*ブロックチェーン*、または*ledger*（互換的に使用）は、IBC 仕様の一部またはすべてを実装する state machine（分散台帳または「ブロックチェーン」である場合もありますが、厳密なブロックチェーンは必要ない場合があります）です。

### Relayer process

*relayer プロセス*はオフチェーンプロセスで、2つ以上の machine 間で IBC packet データとメタデータを中継します。そのために machine の state をスキャンしてトランザクションを送信します。

### State Machine

特定 chain の *state machine* は state の構造と有効なトランザクションを決定するルールセットを定義します。トランザクションは chain の consensus アルゴリズムで合意された現在の state に基づいて、状態遷移を引き起こします。

### Consensus

*consensus* アルゴリズムは、分散台帳を操作する一連のプロセスで使用されるプロトコルであり、通常、限られた数のビザンチン障害の存在下で、同じ state で合意に達します。

### Consensus State

*consensus state* は、その consensus アルゴリズムの出力に関する証明を検証するために必要な consensus アルゴリズムの state に関する情報です（たとえば、署名済み header の commitment root）。

### Commitment

暗号 *commitment* は、マッピング内にキー/値ペアが属しているか否かを手軽に検証できる方法です。短いウィットネス文字列を用いてマッピングをコミットできます。

### Header

*header* は、現在の state への commitment を含む、特定のブロックチェーンの consensus state への更新であり、「light client」アルゴリズムによって明確に定義された方法で検証できます。

### CommitmentProof

*commitment proof* は、特定のキーがコミット先セット内の特定の値にマッピングされているかどうかを証明するデータ構造のことです。

### Handler Module

IBC *handler module* は state machine 内のmoduleです。 [ICS 25](../spec/ics-025-handler-interface) を実装し、client、connection、channelを管理し、proof を検証し、packet の適切な commitment を格納します。

### Routing Module

IBC *routing module* は state machine 内の module で、 [ICS 26](../spec/ics-026-routing-module)を実装し、外部インターフェイスを利用するホスト state machine 上の他のモジュールと routing module の間で packet をルーティングします。

### Datagram

*datagram* は何らかの物理ネットワークを介して送信される不透明（ opaque ）な byte 文字列であり、台帳の state machine に実装されているIBC routing module によって処理されます。いくつかの実装では、datagram は、他の情報も含む台帳固有のトランザクションまたはメッセージデータ構造のフィールドである場合があります（たとえば、スパム防止のための fee、リプレイ防止の nonce 、IBC handler にルーティングする type 識別子など） 。すべての IBC サブプロトコル（ connection を開く、channel を作成する、packet を送信するなど）は、routing module を介してそれらを処理するための datagram とプロトコルのセットによって定義されます。

### Connection

*connection* は2つの chain 上の永続的なデータ構造で、接続中の他方の台帳の consensus state に関する情報を含んでいます。一方の  chain の consensus state を更新すると、もう一方の chain の connection オブジェクトの state が変化します。

### Channel

*channel* はメタデータを含む2つの chain 上の永続的なデータ構造で、packet の順序付け、正確に1回限りの配信、およびリプレイの防止を容易にします。channel を介して送信された packet は、その内部 state を変更します。channel は、多対1の関係で connection に関連付けられます。単一の connection には、任意の数の関連付けられた channel を含めることができます。すべての channel には、channel が作成されるより前に作成された単一の関連付けられた connection が必要になります。

### Packet

*packet* は、シーケンス関連のメタデータ（ IBC 仕様で定義）と packet *データ*と呼ばれる不透明な値フィールド（トークンの量や額面などのアプリケーション層で定義されたセマンティクス）を備えた個別のデータ構造です。packet は、特定の channel を介して（さらに、特定の connection を介して）送信されます。

### Module

*module* は、個別の blockchain の state machine のサブコンポーネントで、IBC handler と相互作用し、特定の IBC packet の送受信（例えばトークンの鋳造や焼却）の *data* フィールドに応じて state を変更します。

### Handshake

*handshake* は、複数の datagram を含む特定のサブプロトコルであり、一般に、相互の consensus アルゴリズムの信頼できる state など、2つの chain でいくつかの共通の state を初期化するために使用されます。

### Sub-protocol

sub-protocol は、blockchain の IBC handler module によって実装される必要がある datagram の種類と関数の集合として定義されます。

datagram は、外部の relayer プロセスによって chain 間で relay される必要があります。この relayer プロセスは所定の方法で動作すると想定されています。安全性( safety property )はその動作に依存しませんが、進行状況は通常、少なくとも1つの正しい relayer プロセスの存在に依存しています。

IBC sub-protocol は、2つの chain `A` と `B` 間の相互作用と見なされます。これら2つの chain 間に事前の区別はなく、同一の正しい IBC プロトコルを実行していると想定されます。 `A` は慣例により、sub-protocol で最初に動作する chain であり、 `B` は2番目に動作する chain です。プロトコルの定義では、混乱を避けるために、変数名に `A` と `B` を含めないようにする必要があります（ chain 自体がプロトコルで `A` と `B` どちらであるかがわからないため）。

### Authentication

*Authentication* は、datagram が IBC handler によって定義された方法で特定の chain によって実際に送信されたことを保証する特性です。

## Property definitions

### Finality

*finality* とは、consensus アルゴリズムによって提供される定量化可能な保証のことであり、validator セットの動作に関するある種の仮定に従って、特定の block が取り消されないことを保証します。 IBC プロトコルは、finality を必要としますが、それが絶対的である必要はありません。（例えば、Nakamoto consensus アルゴリズムに関する閾値の finality gadget は、miner の行動に関する経済的仮定に基づいて finality を提供しています）。

### Misbehaviour

*misbehavior*は consensus アルゴリズムによって定義され、その consensus アルゴリズムのlight client によって検出可能（おそらく原因も可能）な consensus 違反です。

### Equivocation

*equivocation* は、単一の block を親とする複数の異なる block への投票に無効な方法で署名する validator によって commit された consensus 違反です。すべての equivocation は misbehaviour です。

### データの可用性（ Data availability ）

*データの可用性* はオフチェーンの relayer プロセスが machineの state を一定時間内に取得できることです。

### データの機密性（ Data confidentiality ）

*データの機密性*とは、IBC プロトコルの機能を損なうことなく、特定のデータを特定の関係者が利用できなくするホスト state machine の機能です。

### 否認不可性（ Non-repudiability ）

*否認不可性*とは、machine が特定の packet を送信したり、特定の state を commit したりすることに異議を唱えられないということです。 IBC は state machine によるデータの機密性の選択を問わず、否認不可なプロトコルです。

### Consensus liveness

*consensus liveness* とは、特定の machine の consensus アルゴリズムによる block 生成が継続している状態のことです。

### Transactional liveness

*transactional liveness*とは、特定の machine の consensus アルゴリズムによる受信トランザクション（トランザクションは文脈によって明確になっている必要がある）が継続的に確認できることです。transactional liveness には consensus liveness が必要ですが、consensus liveness は必ずしも transactional liveness を提供するわけではありません。transactional liveness は、検閲への耐性を意味します。

### Bounded consensus liveness

*制限付き consensus liveness* は特定の制限内での consensus liveness です。

### Bounded transactional liveness

*制限付き transactional liveness*は、特定の制限内での transactional liveness です。

### Exactly-once safety

*Exactly-once safety* は packet が1度しか確認されない特性のことです （一般的に exactly-once は結果的な transactional liveness を想定しています）。

### Deliver-or-timeout safety

*deliver-or-timeout safety* は、packet が配信された場合は実行され、そうでなければ送信者に証明できる方法でタイムアウトするという特性です。

### （複雑性に関する）Constant

*constant* は、空間または時間の複雑さを指す場合、 `O(1)` を意味します。

### Succinct

*Succinct*  は、空間または時間の複雑さを指す場合、 `O(poly(log n))` 以上を意味します。
