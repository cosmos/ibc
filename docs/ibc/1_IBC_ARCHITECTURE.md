# 1: Inter-blockchain Communication Protocol Architecture

**This is an overview of the high-level architecture & dataflow of the IBC protocol.**

**For a broad set of protocol design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).**

**For definitions of terms used in IBC specifications, see [here](./3_IBC_TERMINOLOGY.md).**

This document outlines the architecture of the authentication, transport, and ordering layers of the inter-blockchain communication (IBC) protocol stack. This document does not describe specific protocol details — those are contained in individual ICSs.

## What is IBC?

The *inter-blockchain communication protocol* is a reliable & secure inter-module communication protocol, where modules are deterministic process that run on independent distributed ledgers (also referred to as blockchains).

## What is IBC not?

IBC is not an application-layer protocol: it handles data transport, authentication, and reliability only.

IBC is not (only) an atomic-swap protocol: arbitrary cross-chain data transfer and computation is supported.

IBC is not (only) a token transfer protocol: token transfer is a possible application-layer use of the IBC protocol.

IBC is not (only) a sharding protocol: there is no single state machine being split across chains, but rather a diverse set of different state machines on different chains which share some common interfaces.

IBC is not (only) a layer-two scaling protocol: all chains implementing IBC exist on the same "layer", although they may occupy different points in the network topology, and there is no single root chain or single validator set.

## Motivation

The two predominant blockchains by market capitalization, Bitcoin and Ethereum, currently support about seven and about twenty transactions per second respectively. Both have been operating at capacity in recent past despite still being utilized primarily by a userbase of early-adopter enthusiasts. Throughput limitations are a fundamental limitation of distributed state machines, since every (validating) node in the network must process every transaction and store all state. Faster consensus algorithms, such as [Tendermint](https://github.com/tendermint/tendermint), may increase throughput by a large constant factor but will be unable to scale indefinitely for this reason. In order to support the transaction throughput, application diversity, and cost efficiency required to facilitate wide deployment of distributed ledger applications, execution and storage must be split across many independent consensus instances which can run concurrently.

One design direction is to shard a single programmable state machine across separate chains, referred to as "shards", which execute concurrently and store disjoint partitions of the state. In order to reason about safety and liveness, and in order to correctly route data and code between shards, these designs must take a "top-down approach" — constructing a particular network topology, featuring a single root ledger and a star or tree of shards, and engineering protocol rules & incentives to enforce that topology. This approach possesses advantages in simplicity and predictability, but faces hard [technical](https://medium.com/nearprotocol/the-authoritative-guide-to-blockchain-sharding-part-1-1b53ed31e060) [problems](https://medium.com/nearprotocol/unsolved-problems-in-blockchain-sharding-2327d6517f43), requires the adherence of all shards to a single validator set (or randomly elected subset thereof) and a single state machine or mutually comprehensible VM, and may face future problems in social scalability due to the general necessity of reaching global consensus on alterations to the network topology.

The *interblockchain communication protocol* takes an orthogonal approach to a differently formulated version of the problem: enabling safe, reliable interoperation of a network of heterogeneous distributed ledgers, arranged in an unknown topology, which can diversify, develop, and rearrange independently of each other or of a particular imposed topology or state machine design. In a wide, dynamic network of interoperating chains, sporadic Byzantine faults are expected, so the protocol must also detect, mitigate, and contain the potential damage of Byzantine faults in accordance with the requirements of the applications & ledgers involved. For a longer list of design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).

To faciliate this heterogeneous interoperation, the interblockchain communication protocol takes a "bottom-up" approach, specifying the set of requirements, functions, and properties necessary to implement interoperation between two ledgers, and then specifying different ways in which multiple interoperating ledgers might be composed which preserve the requirements of higher-level protocols and occupy different points in the safety/speed tradeoff space. IBC thus presumes nothing about and requires nothing of the overall network topology, and of the implementing ledgers requires only that a known, minimal set of functions are available and properties fulfilled.

IBC is an end-to-end, connection-oriented, stateful protocol for reliable, ordered, authenticated communication between modules on separate distributed ledgers. IBC implementations are expected to be co-resident with higher-level modules and protocols on the host ledger. Ledgers hosting IBC must provide a certain set of functions for consensus transcript verification and accumulator proof generation, and IBC packet relayers (off-chain processes) are expected to have access to network protocols and physical datalinks as required to read the state of one ledger and submit data to another.

## Scope

IBC handles authentication, transport, and ordering of structured data packets relayed between modules on separate ledgers. The protocol is intended to be in simultaneous use between any number of modules on any number of ledgers over arbitrarily structured underlying networks.

## Interfaces

IBC sits between modules — smart contracts, state machine components, or otherwise independent pieces of application logic on ledgers — on one side, and underlying consensus protocols, ledgers, and network infrastructure (e.g. TCP/IP), on the other side.

To modules IBC provides a set of functions much like the functions which might be provided to a module for interacting with another module on the same ledger: sending data packets and receiving data packets on an established connection & channel — in addition to calls to manage the protocol state: opening and closing connections and channels, choosing connection, channel, and packet delivery options. Considerable flexibility is provided to ledger developers as to which of these functions to expose to which modules, and how to restrict parameter choices — if at all — the protocol generally assumes the most permissionless setting possible, and implementers can choose to restrict usage according to their application's requirements.

Of the underlying consensus protocols and ledgers IBC requires a set of primitive functions and properties as defined in [ICS 2](../../spec/ics-2-consensus-requirements), primarily finality, cheaply-verifiable consensus transcripts, and simple key-value store functionality. Of the network infrastructure protocol layer (and physical network layer) IBC requires only eventual data delivery — no authentication, synchrony, or ordering properties are assumed.

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

The primary purpose of IBC is to provide reliable, authenticated, ordered communication between modules running on independent host distributed ledgers. This requires protocol logic in the following areas:
- Data relay
- Reliability
- Flow control
- Authentication
- Connections
- Multiplexing

The following paragraphs outline the protocol logic within IBC for each area.

### Data relay

IBC assumes the existence of a set of relayer processes with access to an underlying network protocol stack (likely TCP/IP, UDP/IP, or QUIC/IP) and physical interconnect infrastructure. These relayer processes monitor a set of ledgers implementing the IBC protocol, continuously scanning the state of each ledger and executing transactions on another ledger when outgoing datagrams have been committed. For correct operation and progress in a connection between two ledgers, IBC requires only that at least one correct and live relayer process exists which can relay between the ledgers.

### Reliability

The network layer and relayer processes may behave in arbitrary ways, dropping, reordering, or duplicating packets, purposely attempting to send invalid transactions, or otherwise acting Byzantine. This must not compromise the safety or liveness of IBC. This is achieved by assigning a sequence number to each packet sent over an IBC connection, which is checked by the IBC handler on the receiving ledger, and providing a method for the sending ledger to check that the receiving ledger has in fact received and handled a packet before sending more packets or taking further action. Cryptographic commitments are used to prevent datagram forgery: the sending ledger commits to outgoing datagrams, and the receiving ledger checks these commitments, so datagrams altered in transit by a relayer will be rejected.

### Flow control

IBC does not require specific provision for computation-level flow control since the underlying ledgers will have throughput limitations and flow control mechanisms of their own (such as "gas" markets). Application-level flow control — limiting the rate of particular packets according to their content — may be useful to ensure security properties (limiting the value on a single ledger) and contain damage from Byzantine faults (allowing a challenge period to prove an equivocation, then closing a connection). IBC provides facilities for modules to reject packets and leaves particulars up to the higher-level application protocols.

### Authentication

All datagrams in IBC are authenticated: a block finalized by the consensus algorithm of the sending ledger must commit to the outgoing datagram via a cryptographic accumulator, and the receiving chain must verify both the consensus transcript and the accumulator proof that the datagram was sent (and associated actions executed) before acting upon it. For correct execution semantics, both chains must implement the correct IBC protocol — of course, since IBC is permissionless, any chain could connect to another chain and provide a correct accumulator proof but incorrectly implement IBC such that its IBC handler did not behave according to protocol, so modules utilizing IBC connections (or end-users using those modules) must inspect the state of the connections and ledgers they plan to use to ensure correct implementation.

### Connections

Reliability, flow control, and authentication as described above require that IBC initializes and maintains certain status information for each datastream. This information is called a connection. Each connection object contains information about the consensus state of the connected ledger, negotiated encoding & multiplexing options, and state & sequence numbers. When two modules wish to communicate, they must locate an existing connection between their two ledgers, or initialize a new connection if none yet exists. Initializing a connection requires a multi-step handshake which, once complete, ensures that only the two intended ledgers are connected and future datagrams relayed will be authenticated, encoded, and sequenced as desired.

### Multiplexing

To allow for many modules within a single host ledger to use an IBC connection simultaneously, IBC provides a set of channels within each connection, which each uniquely identify a linear datapipe over which packets can be sent in order to a destination module on the receiving ledger. Channels are usually expected to be associated with a single module on each ledger, but one-to-many and many-to-one channels are also possible. The number of channels is unbounded, facilitating concurrent throughput limited only by the throughput of the underyling ledgers with only a single connection necessary to track consensus information (and consensus transcript verification cost thus amortized across all channels using the connection).

## Dataflow

IBC can be conceptualized as a layered protocol stack, through which data flows top-to-bottom (when sending IBC packets) and bottom-to-top (when receiving IBC packets).

Consider the path of an IBC packet between two chains — call them *A* and *B*:

### Diagram

```
+----------------------------------------------------------------------------------+
| Chain A                                                                          |
|                                                                                  |
| Actor --> Module --> Handler --> Packet --> Channel --> Connection --> Consensus |
+----------------------------------------------------------------------------------+

    +---------+
==> | Relayer | ==>
    +---------+

+----------------------------------------------------------------------------------+
| Chain B                                                                          |
|                                                                                  |
| --> Consensus --> Connection --> Channel --> Packet --> Handler --> Module       |
+----------------------------------------------------------------------------------+
```

### Steps

1. On chain *A*
    1. Actor (application-specific)
    1. Module (application-specific)
    1. Handler (parts defined in different ICSs)
    1. Packet (defined in [ICS 5](../../spec/ics-5-packet-semantics))
    1. Channel (defined in [ICS 4](../../spec/ics-4-channel-semantics))
    1. Connection (defined in [ICS 3](../../spec/ics-3-connection-semantics))
    1. Consensus (defined in [ICS 2](../../spec/ics-2-consensus-requirements))
2. Off-chain
    1. Relayer (defined in [ICS 18](../../spec/ics-18-offchain-relayer))
3. On chain *B*
    1. Consensus (defined in [ICS 2](../../spec/ics-2-consensus-requirements))
    1. Connection (defined in [ICS 3](../../spec/ics-3-connection-semantics))
    1. Channel (defined in [ICS 4](../../spec/ics-4-channel-semantics))
    1. Packet (defined in [ICS 5](../../spec/ics-5-packet-semantics))
    1. Handler (parts defined in different ICSs)
    1. Module (application-specific)
