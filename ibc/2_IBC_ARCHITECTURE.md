# 2: Inter-blockchain Communication Protocol Architecture

**This is an overview of the high-level architecture & data-flow of the IBC protocol.**

**For definitions of terms used in IBC specifications, see [here](./1_IBC_TERMINOLOGY.md).**

**For a broad set of protocol design principles, see [here](./3_IBC_DESIGN_PRINCIPLES.md).**

**For a set of example use cases, see [here](./4_IBC_USECASES.md).**

**For a discussion of design patterns, see [here](./5_IBC_DESIGN_PATTERNS.md).**

This document outlines the architecture of the authentication, transport, and ordering layers of the inter-blockchain communication (IBC) protocol stack. This document does not describe specific protocol details — those are contained in individual ICSs.

> Note: *Ledger*, *chain*, and *blockchain* are used interchangeably throughout this document, in accordance with their colloquial usage.

## What is IBC?

The *inter-blockchain communication protocol* is a reliable & secure inter-module communication protocol, where modules are deterministic processes that run on independent machines, including replicated state machines (like "blockchains" or "distributed ledgers").

IBC can be used by any application which builds on top of reliable & secure inter-module communication. Example applications include cross-chain asset transfer, atomic swaps, multi-chain smart contracts (with or without mutually comprehensible VMs), and data & code sharding of various kinds.

## What is IBC not?

IBC is not an application-layer protocol: it handles data transport, authentication, and reliability only.

IBC is not an atomic-swap protocol: arbitrary cross-chain data transfer and computation is supported.

IBC is not a token transfer protocol: token transfer is a possible application-layer use of the IBC protocol.

IBC is not a sharding protocol: there is no single state machine being split across chains, but rather a diverse set of different state machines on different chains which share some common interfaces.

IBC is not a layer-two scaling protocol: all chains implementing IBC exist on the same "layer", although they may occupy different points in the network topology, and there is not necessarily a single root chain or single validator set.

## Motivation

The two predominant blockchains at the time of writing, Bitcoin and Ethereum, currently support about seven and about twenty transactions per second respectively. Both have been operating at capacity in recent past despite still being utilised primarily by a user-base of early-adopter enthusiasts. Throughput is a limitation for most blockchain use cases, and throughput limitations are a fundamental limitation of distributed state machines, since every (validating) node in the network must process every transaction (modulo future zero-knowledge constructions, which are out-of-scope of this specification at present), store all state, and communicate with other validating nodes. Faster consensus algorithms, such as [Tendermint](https://github.com/tendermint/tendermint), may increase throughput by a large constant factor but will be unable to scale indefinitely for this reason. In order to support the transaction throughput, application diversity, and cost efficiency required to facilitate wide deployment of distributed ledger applications, execution and storage must be split across many independent consensus instances which can run concurrently.

One design direction is to shard a single programmable state machine across separate chains, referred to as "shards", which execute concurrently and store disjoint partitions of the state. In order to reason about safety and liveness, and in order to correctly route data and code between shards, these designs must take a "top-down approach" — constructing a particular network topology, featuring a single root ledger and a star or tree of shards, and engineering protocol rules & incentives to enforce that topology. This approach possesses advantages in simplicity and predictability, but faces hard [technical](https://medium.com/nearprotocol/the-authoritative-guide-to-blockchain-sharding-part-1-1b53ed31e060) [problems](https://medium.com/nearprotocol/unsolved-problems-in-blockchain-sharding-2327d6517f43), requires the adherence of all shards to a single validator set (or randomly elected subset thereof) and a single state machine or mutually comprehensible VM, and may face future problems in social scalability due to the necessity of reaching global consensus on alterations to the network topology.

Furthermore, any single consensus algorithm, state machine, and unit of Sybil resistance may fail to provide the requisite levels of security and versatility. Consensus instances are limited in the number of independent operators they can support, meaning that the amortised benefits from corrupting any particular operator increase as the value secured by the consensus instance increases — while the cost to corrupt the operator, which will always reflect the cheapest path (e.g. physical key exfiltration or social engineering), likely cannot scale indefinitely. A single global state machine must cater to the common denominator of a diverse application set, making it less well-suited for any particular application than a specialised state machine would be. Operators of a single consensus instance may abuse their privileged position to extract rent from applications which cannot easily elect to exit. It would be preferable to construct a mechanism by which separate, sovereign consensus instances & state machines can safely, voluntarily interact while sharing only a minimum requisite common interface.

The *interblockchain communication protocol* takes a different approach to a differently formulated version of the scaling & interoperability problems: enabling safe, reliable interoperation of a network of heterogeneous distributed ledgers, arranged in an unknown topology, preserving secrecy where possible, where the ledgers can diversify, develop, and rearrange independently of each other or of a particular imposed topology or state machine design. In a wide, dynamic network of interoperating chains, sporadic Byzantine faults are expected, so the protocol must also detect, mitigate, and contain the potential damage of Byzantine faults in accordance with the requirements of the applications & ledgers involved. For a longer list of design principles, see [here](./3_IBC_DESIGN_PRINCIPLES.md).

To facilitate this heterogeneous interoperation, the interblockchain communication protocol takes a "bottom-up" approach, specifying the set of requirements, functions, and properties necessary to implement interoperation between two ledgers, and then specifying different ways in which multiple interoperating ledgers might be composed which preserve the requirements of higher-level protocols and occupy different points in the safety/speed tradeoff space. IBC thus presumes nothing about and requires nothing of the overall network topology, and of the implementing ledgers requires only that a known, minimal set of functions are available and properties fulfilled. Indeed, ledgers within IBC are defined as their light client consensus validation functions, thus expanding the range of what a "ledger" can be to include single machines and complex consensus algorithms alike.

IBC is an end-to-end, connection-oriented, stateful protocol for reliable, optionally ordered, authenticated communication between modules on separate machines. IBC implementations are expected to be co-resident with higher-level modules and protocols on the host state machine. State machines hosting IBC must provide a certain set of functions for consensus transcript verification and cryptographic commitment proof generation, and IBC packet relayers (off-chain processes) are expected to have access to network protocols and physical data-links as required to read the state of one machine and submit data to another.

## Scope

IBC handles authentication, transport, and ordering of structured data packets relayed between modules on separate machines. The protocol is defined between modules on two machines, but designed for safe simultaneous use between any number of modules on any number of machines connected in arbitrary topologies.

## Interfaces

IBC sits between modules — smart contracts, other state machine components, or otherwise independent pieces of application logic on state machines — on one side, and underlying consensus protocols, machines, and network infrastructure (e.g. TCP/IP), on the other side.

IBC provides to modules a set of functions much like the functions which might be provided to a module for interacting with another module on the same state machine: sending data packets and receiving data packets on an established connection & channel (primitives for authentication & ordering, see [definitions](./1_IBC_TERMINOLOGY.md)) — in addition to calls to manage the protocol state: opening and closing connections and channels, choosing connection, channel, and packet delivery options, and inspecting connection & channel status.

IBC assumes functionalities and properties of the underlying consensus protocols and machines as defined in [ICS 2](../../spec/ics-002-client-semantics), primarily finality (or thresholding finality gadgets), cheaply-verifiable consensus transcripts, and simple key/value store functionality. On the network side, IBC requires only eventual data delivery — no authentication, synchrony, or ordering properties are assumed (these properties are defined precisely later on).

### Protocol relations

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

## Operation

The primary purpose of IBC is to provide reliable, authenticated, ordered communication between modules running on independent host machines. This requires protocol logic in the following areas:

- Data relay
- Data confidentiality & legibility
- Reliability
- Flow control
- Authentication
- Statefulness
- Multiplexing
- Serialisation

The following paragraphs outline the protocol logic within IBC for each area.

### Data relay

In the IBC architecture, modules are not directly sending messages to each other over networking infrastructure, but rather creating messages to be sent which are then physically relayed by monitoring "relayer processes". IBC assumes the existence of a set of relayer processes with access to an underlying network protocol stack (likely TCP/IP, UDP/IP, or QUIC/IP) and physical interconnect infrastructure. These relayer processes monitor a set of machines implementing the IBC protocol, continuously scanning the state of each machine and executing transactions on another machine when outgoing packets have been committed. For correct operation and progress in a connection between two machines, IBC requires only that at least one correct and live relayer process exists which can relay between the machines.

### Data confidentiality & legibility

The IBC protocol requires only that the minimum data necessary for correct operation of the IBC protocol be made available & legible (serialised in a standardised format), and the state machine may elect to make that data available only to specific relayers (though the details thereof are out-of-scope of this specification). This data consists of consensus state, client, connection, channel, and packet information, and any auxiliary state structure necessary to construct proofs of inclusion or exclusion of particular key/value pairs in state. All data which must be proved to another machine must also be legible; i.e., it must be serialised in a format defined by this specification.

### Reliability

The network layer and relayer processes may behave in arbitrary ways, dropping, reordering, or duplicating packets, purposely attempting to send invalid transactions, or otherwise acting in a Byzantine fashion. This must not compromise the safety or liveness of IBC. This is achieved by assigning a sequence number to each packet sent over an IBC connection (at the time of send), which is checked by the IBC handler (the part of the state machine implementing the IBC protocol) on the receiving machine, and providing a method for the sending machine to check that the receiving machine has in fact received and handled a packet before sending more packets or taking further action. Cryptographic commitments are used to prevent datagram forgery: the sending machine commits to outgoing packets, and the receiving machine checks these commitments, so datagrams altered in transit by a relayer will be rejected. IBC also supports unordered channels, which do not enforce ordering of packet receives relative to sends but still enforce exactly-once delivery.

### Flow control

IBC does not provide specific provisions for compute-level or economic-level flow control. The underlying machines will have compute throughput limitations and flow control mechanisms of their own (such as "gas" markets). Application-level economic flow control — limiting the rate of particular packets according to their content — may be useful to ensure security properties (limiting the value on a single machine) and contain damage from Byzantine faults (allowing a challenge period to prove an equivocation, then closing a connection). For example, an application transferring value over an IBC channel might want to limit the rate of value transfer per block to limit damage from potential Byzantine behaviour. IBC provides facilities for modules to reject packets and leaves particulars up to the higher-level application protocols.

### Authentication

All datagrams in IBC are authenticated: a block finalised by the consensus algorithm of the sending machine must commit to the outgoing packet via a cryptographic commitment, and the receiving chain's IBC handler must verify both the consensus transcript and the cryptographic commitment proof that the datagram was sent before acting upon it.

### Statefulness

Reliability, flow control, and authentication as described above require that IBC initialises and maintains certain status information for each datastream. This information is split between two abstractions: connections & channels. Each connection object contains information about the consensus state of the connected machine. Each channel, specific to a pair of modules, contains information concerning negotiated encoding & multiplexing options and state & sequence numbers. When two modules wish to communicate, they must locate an existing connection & channel between their two machines, or initialise a new connection & channels if none yet exists. Initialising connections & channels requires a multi-step handshake which, once complete, ensures that only the two intended machines are connected, in the case of connections, and ensures that two modules are connected and that future datagrams relayed will be authenticated, encoded, and sequenced as desired, in the case of channels.

### Multiplexing

To allow for many modules within a single host machine to use an IBC connection simultaneously, IBC provides a set of channels within each connection, which each uniquely identify a datastream over which packets can be sent in order (in the case of an ordered module), and always exactly once, to a destination module on the receiving machine. Channels are usually expected to be associated with a single module on each machine, but one-to-many and many-to-one channels are also possible. The number of channels is unbounded, facilitating concurrent throughput limited only by the throughput of the underlying machines with only a single connection necessary to track consensus information (and consensus transcript verification cost thus amortised across all channels using the connection).

### Serialisation

IBC serves as the interface boundary between otherwise mutually incomprehensible machines, and must provide the requisite mutual comprehensibility of the minimal set of data structure encodings & datagram formats in order to allow two machines which both correctly implement the protocol to understand each other. For this purpose, the IBC specification defines
canonical encodings of data structures which must be serialised and relayed or checked in proofs between two machines talking over IBC, provided in proto3 format in this repository.

> Note that a subset of proto3 which provides canonical encodings (the same structure always serialises to the same bytes) must be used. Maps and unknown fields are thus prohibited.

## Dataflow

IBC can be conceptualised as a layered protocol stack, through which data flows top-to-bottom (when sending IBC packets) and bottom-to-top (when receiving IBC packets).

The "handler" is the part of the state machine implementing the IBC protocol, which is responsible for translating calls from modules to and from packets and routing them appropriately to and from channels & connections.

Consider the path of an IBC packet between two chains — call them *A* and *B*:

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

1. On chain *A*
    1. Module (application-specific)
    1. Handler (parts defined in different ICSs)
    1. Packet (defined in [ICS 4](../spec/ics-004-channel-and-packet-semantics))
    1. Channel (defined in [ICS 4](../spec/ics-004-channel-and-packet-semantics))
    1. Connection (defined in [ICS 3](../spec/ics-003-connection-semantics))
    1. Client (defined in [ICS 2](../spec/ics-002-client-semantics))
    1. Consensus (confirms the transaction with the outgoing packet)
2. Off-chain
    1. Relayer (defined in [ICS 18](../spec/ics-018-relayer-algorithms))
3. On chain *B*
    1. Consensus (confirms the transaction with the incoming packet)
    1. Client (defined in [ICS 2](/../spec/ics-002-client-semantics))
    1. Connection (defined in [ICS 3](/../spec/ics-003-connection-semantics))
    1. Channel (defined in [ICS 4](/../spec/ics-004-channel-and-packet-semantics))
    1. Packet (defined in [ICS 4](/../spec/ics-004-channel-and-packet-semantics))
    1. Handler (parts defined in different ICSs)
    1. Module (application-specific)
