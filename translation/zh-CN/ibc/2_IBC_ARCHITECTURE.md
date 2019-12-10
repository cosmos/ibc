# 2: Inter-blockchain Communication Protocol Architecture

**This is an overview of the high-level architecture & data-flow of the IBC protocol.**

**For definitions of terms used in IBC specifications, see [here](./1_IBC_TERMINOLOGY.md).**

**For a broad set of protocol design principles, see [here](./3_IBC_DESIGN_PRINCIPLES.md).**

**For a set of example use cases, see [here](./4_IBC_USECASES.md).**

**For a discussion of design patterns, see [here](./5_IBC_DESIGN_PATTERNS.md).**

This document outlines the architecture of the authentication, transport, and ordering layers of the inter-blockchain communication (IBC) protocol stack. This document does not describe specific protocol details — those are contained in individual ICSs.

> Note: *Ledger*, *chain*, and *blockchain* are used interchangeably throughout this document, in accordance with their colloquial usage.

## IBC 是什么？

IBC 是一种可靠的安全的链间通信协议，其中模块是包括可复制状态机（区块链或分布式账本）在内的在独立机器上运行的确定性程序。

IBC 可以被构建在在安全可靠的模块间通信上的应用程序所使用。典型的应用包括跨链资产转移、原子交换、多链智能合约以及各类数据和代码片段。

## IBC 不是什么？

IBC 不是一个应用层的协议：它只负责处理数据的传输、认证和可靠性问题。

IBC 不是一个原子交换的协议：支持任意跨链数据的传输和计算。

IBC 不是一个通证转移协议：通证转移是使用 IBC 协议的一种潜在的应用场景。

IBC 不是一个分片协议：这里不存在一个从这些链中分离出来的状态机，而是不同种类的区块链拥有不同的状态机，她们拥有彼此分享一些共享的接口。

IBC 不是一个二层扩容（链下扩容）协议：所有应用 IBC 的链都位于同一层，这些链并不需要一条单独的主链或单个验证者集合，尽管它们可能占据网络拓扑中的不同点。

## Motivation

The two predominant blockchains at the time of writing, Bitcoin and Ethereum, currently support about seven and about twenty transactions per second respectively. Both have been operating at capacity in recent past despite still being utilised primarily by a user-base of early-adopter enthusiasts. Throughput is a limitation for most blockchain use cases, and throughput limitations are a fundamental limitation of distributed state machines, since every (validating) node in the network must process every transaction (modulo future zero-knowledge constructions, which are out-of-scope of this specification at present), store all state, and communicate with other validating nodes. Faster consensus algorithms, such as [Tendermint](https://github.com/tendermint/tendermint), may increase throughput by a large constant factor but will be unable to scale indefinitely for this reason. In order to support the transaction throughput, application diversity, and cost efficiency required to facilitate wide deployment of distributed ledger applications, execution and storage must be split across many independent consensus instances which can run concurrently.

One design direction is to shard a single programmable state machine across separate chains, referred to as "shards", which execute concurrently and store disjoint partitions of the state. In order to reason about safety and liveness, and in order to correctly route data and code between shards, these designs must take a "top-down approach" — constructing a particular network topology, featuring a single root ledger and a star or tree of shards, and engineering protocol rules & incentives to enforce that topology. This approach possesses advantages in simplicity and predictability, but faces hard [technical](https://medium.com/nearprotocol/the-authoritative-guide-to-blockchain-sharding-part-1-1b53ed31e060) [problems](https://medium.com/nearprotocol/unsolved-problems-in-blockchain-sharding-2327d6517f43), requires the adherence of all shards to a single validator set (or randomly elected subset thereof) and a single state machine or mutually comprehensible VM, and may face future problems in social scalability due to the necessity of reaching global consensus on alterations to the network topology.

Furthermore, any single consensus algorithm, state machine, and unit of Sybil resistance may fail to provide the requisite levels of security and versatility. Consensus instances are limited in the number of independent operators they can support, meaning that the amortised benefits from corrupting any particular operator increase as the value secured by the consensus instance increases — while the cost to corrupt the operator, which will always reflect the cheapest path (e.g. physical key exfiltration or social engineering), likely cannot scale indefinitely. A single global state machine must cater to the common denominator of a diverse application set, making it less well-suited for any particular application than a specialised state machine would be. Operators of a single consensus instance may abuse their privileged position to extract rent from applications which cannot easily elect to exit. It would be preferable to construct a mechanism by which separate, sovereign consensus instances & state machines can safely, voluntarily interact while sharing only a minimum requisite common interface.

The *interblockchain communication protocol* takes a different approach to a differently formulated version of the scaling & interoperability problems: enabling safe, reliable interoperation of a network of heterogeneous distributed ledgers, arranged in an unknown topology, preserving secrecy where possible, where the ledgers can diversify, develop, and rearrange independently of each other or of a particular imposed topology or state machine design. In a wide, dynamic network of interoperating chains, sporadic Byzantine faults are expected, so the protocol must also detect, mitigate, and contain the potential damage of Byzantine faults in accordance with the requirements of the applications & ledgers involved. For a longer list of design principles, see [here](./3_IBC_DESIGN_PRINCIPLES.md).

To facilitate this heterogeneous interoperation, the interblockchain communication protocol takes a "bottom-up" approach, specifying the set of requirements, functions, and properties necessary to implement interoperation between two ledgers, and then specifying different ways in which multiple interoperating ledgers might be composed which preserve the requirements of higher-level protocols and occupy different points in the safety/speed tradeoff space. IBC thus presumes nothing about and requires nothing of the overall network topology, and of the implementing ledgers requires only that a known, minimal set of functions are available and properties fulfilled. Indeed, ledgers within IBC are defined as their light client consensus validation functions, thus expanding the range of what a "ledger" can be to include single machines and complex consensus algorithms alike.

IBC is an end-to-end, connection-oriented, stateful protocol for reliable, optionally ordered, authenticated communication between modules on separate machines. IBC implementations are expected to be co-resident with higher-level modules and protocols on the host state machine. State machines hosting IBC must provide a certain set of functions for consensus transcript verification and cryptographic commitment proof generation, and IBC packet relayers (off-chain processes) are expected to have access to network protocols and physical data-links as required to read the state of one machine and submit data to another.

## 范围

IBC 旨在处理在独立计算机上的模块之间中继的结构化数据包的身份验证，传输和排序。 该协议是在两台计算机上的模块之间定义的，但同时被设计来可以被在以任意拓扑连接下任意数量的机器上任意数量的模块之间同时安全地使用。

## 接口

IBC 一方面位于智能合约、其他状态机组件或状态机上其他独立的应用程序逻辑等这些模块之间，另一方面位于基础共识协议，机器和网络基础结构（例如TCP / IP）之间。

IBC 为模块提供了一组功能，类似于为一个模块提供与该状态机上其他的模块进行互操作的功能：在已建立的连接和通道上发送数据包和接收数据包——除了用于管理协议状态的调用外：建立和关闭连接和通道、选择连接、通道和数据包传递选项外，还包括检查连接和通道的状态。

IBC 假设 ICS 2中定义的基础共识协议和机器的功能和特性，主要是最终确定性（或最终确定性阀值），易于验证的共识记录和简单的键/值存储功能。在网络方面，IBC 仅需要保障最终的数据传递——不假定身份验证、同步或排序属性。

### 协议关系

```
+------------------------------+                           +------------------------------+
| Distributed Ledger A         |                           | Distributed Ledger B         |
|                              |                           |                              |
| +--------------------------+ |                           | +--------------------------+ |
| | State Machine            | |                           | | State Machine            | |
| |                          | |                           | |                          | |
| | +----------+     +-----+ | |        +---------+        | | +-----+     +----------+ | |
| | | Module A | <-> | IBC | | | <----> | Relayer | <----> | | | IBC | <-> | Module B | | |
| | +----------+     +-----+ | |        +---------+        | | +-----+     +----------+ | |
| +--------------------------+ |                           | +--------------------------+ |
+------------------------------+                           +------------------------------+
```

## 可操作性

IBC 的主要目的是为了在独立主机上运行的模块之间提供可靠的、经过身份验证的有序的通信，这需要以下领域的协议逻辑：

- Data relay
- Data confidentiality & legibility
- Reliability
- Flow control
- Authentication
- Statefulness
- Multiplexing
- Serialisation

The following paragraphs outline the protocol logic within IBC for each area.

### 数据中继

在 IBC 的架构中，模块之间并不是通过在网络基础设施上直接向彼此发送消息，而是先创建消息，然后通过监听“中继进程”对消息进行物理层面的中继。IBC 假定存在一组中继进程，这些中继进程可以访问基础网络协议堆栈（可能是TCP / IP，UDP / IP 或 QUIC / IP）和物理互连基础设施。 这些中继进程监听着应用了 IBC 协议的一组主机，连续扫描每台主机的状态，并在向外提交数据包时在另一台主机上执行交易。 为了正确操作并在两台主机之间建立连接的进度，IBC 仅要求至少存在一个可以在主机之间进行中继的正确且实时的中继程序。

### 数据保密性和可识别性

IBC 协议仅要求使 IBC 协议正确执行所需的最小数据可识别性（以标准格式序列化），并且状态机可以选择使该数据仅对特定中继器可用（尽管其详细信息超出本规范的范围）。 该数据包括共识状态、客户端（或译为轻节点）、连接、通道和数据包信息，以及在状态中用于验证或排除特定键/值对所必需的任何辅助状态结构。 所有必须证明给另一台主机的数据也必须具备可识别性， 即，必须以本规范定义的格式进行序列化。

### 可靠性

网络层和中继器进程可能以任意方式运行，丢弃、重新排序或复制数据包，有意尝试发送无效交易或以拜占庭方式进行操作。这一定不能损害 IBC 的安全性或活跃度。这是由为发送在 IBC 连接上的每个数据包分配一个序列号来实现的，该序列号由接收方上的 IBC 处理程序（状态机中实现了 IBC 的部分）检查，并提供一种方法，让发送方在发送更多数据包或采取进一步措施之前，去检查接收方实际上已经接收并处理了该数据包。密码学承诺用于防止包的伪造：发送方机器对外发数据包进行承诺，而接收方计算机检查这些承诺，因此中继程序在传输过程中更改的包将被拒绝。 IBC还支持无序通道，该通道不强制对发送的数据包进行排序，但仍强调严格一次发送的原则。

### 流程控制

IBC 不会提供对计算层面或者经济层面的流程控制规定。机器的底层结构将具有自身的吞吐量限制和流程控制机制（例如 ETH 的“燃料”机制）。应用层面的经济流控制——根据内容对数据包的费率进行限制——或许对提升安全性（对任一状态机的状态值进行限定）和控制拜占庭容错带来的危害（提供挑战期，在挑战期内，证明如果具有多签行为可以关闭其连接）是有帮助的。例如，通过 IBC 通道进行价值跨链的应用可能希望通过限制每个区块所包含的跨链交易额，来限制潜在的拜占庭问题带来的危害。IBC 为模块提供了拒绝数据包的功能，并为上层的应用协议预留了具体的空间去处理其细节。

### 身份认证

在 IBC 中的所有包必须是通过身份认证的：由发送链的共识算法完成的块必须通过加密承诺提交跨链数据包，并且接收链的 IBC 处理模块必须在采取进一步行动之前验证包中的共识记录和加密承诺证明。

### 有状态性

如上所述，可靠性，流量控制和身份验证要求 IBC 初始化并维护每个数据流的某些状态信息。 此信息分为两个抽象：连接和通道。 每个连接对象都包含有关已连接主机的共识状态信息。通道是对一对特定的两个模块而言的，通道均包含有关协商的编码和多路复用选项以及状态和序列号的信息。 当两个模块希望进行通信时，它们必须在其两台机器之间找到一个现有的连接和通道，否则，则需要初始化一个新的连接和通道。 初始化连接和通道都需要进行多次握手，一旦握手完成，中继的数据包将根据需要进行身份验证，编码和排序。

### 多路复用

为了允许单个主机中的许多模块能够同时使用 IBC 建立的连接，IBC 在每个连接中提供了一组通道，每个通道唯一地标识按序发送包的数据流（对于已构建通道的模块） ，并确保发送目的链的对应模块做到“精确一次”。 通常希望一个通道与主机上的单个模块进行关联，但是一对多和多对一的通道也是可能的。 通道的数量是无限的，通道数不受限制，从而促进并发吞吐量仅受底层计算机的吞吐量限制，而跟踪共识信息只需一个连接即可（因此，使用某一连接的所有通道共同承担验证共识记录的成本）。

### 连贯性

IBC 充当了彼此无法互通的机器之间的接口边界，并且必须提供基于最小的数据结构编码和数据包结构的足够的互理解性，使得两台都正确实现 IBC 协议的机器能够相互理解。 为此，IBC 规范定义了数据结构的规范编码——proto3 格式，用于在通过 IBC 进行通信的两台机器之间进行序列化、中继或检查证明。

> Note that a subset of proto3 which provides canonical encodings (the same structure always serialises to the same bytes) must be used. Maps and unknown fields are thus prohibited.

## 数据流

IBC 可以被概念化为分层协议栈，通过该协议栈，数据从上到下（在发送IBC数据包时）和自下而上（在接收IBC数据包时）流过。

“处理逻辑”是状态机中实现 IBC 协议的部分，该状态机负责将模块之间的调用与数据包之间进行转换，并在通道和连接之间进行适当的路由。

我们来考虑两条链之间的 IBC 数据包的路径——我们先称之为路径 A 和路径 B，我们来观察数据的流向以及每个模块所对应的子协议：

### Diagram

```
+---------------------------------------------------------------------------------------------+
| Distributed Ledger A                                                                        |
|                                                                                             |
| +----------+     +----------------------------------------------------------+               |
| |          |     | IBC Module                                               |               |
| | Module A | --> |                                                          | --> Consensus |
| |          |     | Handler --> Packet --> Channel --> Connection --> Client |               |
| +----------+     +----------------------------------------------------------+               |
+---------------------------------------------------------------------------------------------+

    +---------+
==> | Relayer | ==>
    +---------+

+--------------------------------------------------------------------------------------------+
| Distributed Ledger B                                                                       |
|                                                                                            |
|               +---------------------------------------------------------+     +----------+ |
|               | IBC Module                                              |     |          | |
| Consensus --> |                                                         | --> | Module B | |
|               | Client -> Connection --> Channel --> Packet --> Handler |     |          | |
|               +---------------------------------------------------------+     +----------+ |
+--------------------------------------------------------------------------------------------+
```

### Steps

1. On chain *A*在链 A 上
    1. Module (application-specific)模块 A（特定应用）
    2. Handler (parts defined in different ICSs)处理程序（在不同子协议中定义的实现部分）
    3. Packet (defined in [ICS 4](../spec/ics-004-channel-and-packet-semantics))数据包（在子协议 ICS 04 中定义）
    4. Channel (defined in [ICS 4](../spec/ics-004-channel-and-packet-semantics))通道（在子协议 ICS 04 中定义）
    5. Connection (defined in [ICS 3](../spec/ics-003-connection-semantics))连接（在子协议 ICS 03 中定义）
    6. Client (defined in [ICS 2](../spec/ics-002-client-semantics))客户端（在子协议 ICS 02 中定义）
    7. Consensus (confirms the transaction with the outgoing packet)共识（确认流出数据包中的交易）
2. Off-chain链下部分 
    1. Relayer (defined in [ICS 18](../spec/ics-018-relayer-algorithms))中继层（在子协议 ICS 18 中定义）
3. On chain *B* 在链 B 上
    1. Consensus (confirms the transaction with the incoming packet)共识（确认流入数据包中的交易）
    2. Client (defined in [ICS 2](/../spec/ics-002-client-semantics))客户端（在子协议 ICS 02 中定义）
    3. Connection (defined in [ICS 3](/../spec/ics-003-connection-semantics))连接（在子协议 ICS 03 中定义）
    4. Channel (defined in [ICS 4](/../spec/ics-004-channel-and-packet-semantics))通道（在子协议 ICS 04 中定义）
    5. Packet (defined in [ICS 4](/../spec/ics-004-channel-and-packet-semantics))数据包（在子协议 ICS 04 中定义）
    6. Handler (parts defined in different ICSs)处理程序（在不同子协议中定义的实现部分）
    7. Module (application-specific)模块 B（特定应用）
