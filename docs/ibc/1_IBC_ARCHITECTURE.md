# 3: IBC Architecture

> This is an overview of the high-level architecture & dataflow of the IBC protocol.

> For a broad set of protocol design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).

> For definitions of terms used in IBC specifications, see [here](./3_IBC_TERMINOLOGY.md).

This document outlines the architecture of the authentication, transport, and ordering layers of the IBC protocol stack. This document does not describe specific protocol details — those are contained in individual ICSs.

## What is IBC?

The *inter-blockchain communication protocol* is designed for use as a reliable module-to-module protocol between modules running on independent distributed ledgers across an untrusted network layer.

## What is IBC not?

IBC is not (only) an atomic-swap protocol: arbitrary cross-chain data transfer and computation is supported.

IBC is not (only) a token transfer protocol: token transfer is a possible application-layer use of the IBC protocol.

IBC is not (only) a sharding protocol: there is no single state machine being split across chains, but rather a diverse set of different state machines on different chains which share some common interfaces.

IBC is not (only) a layer-two scaling protocol: all chains implementing IBC exist on the same "layer", although they may occupy different points in the network topology, and there is no single root chain or single validator set.

## Motivation

The two predominant blockchains, Bitcoin and Ethereum, support about seven and about twenty transactions per second respectively. Both have been operating at capacity in recent past despite still being utilized primarily by a userbase of early-adopter enthusiasts, far from the desired mainstream adoption. Throughput limitations are a fundamental limitation of distributed state machines, since every (validating) node in the network must process every transaction and store all state. Faster consensus algorithms, such as [Tendermint](https://github.com/tendermint/tendermint), may increase throughput by a large constant factor but will be unable to scale indefinitely for this reason. In order to support the transaction throughput, application diversity, and cost efficiency required to facilitate wide deployment of distributed ledger applications, execution and storage must be split across many independent consensus instances which can run concurrently.

One design direction is to shard a single programmable state machine across separate chains, referred to as "shards", which execute concurrently and store disjoint partitions of the state. In order to reason about safety and liveness, and in order to correctly route data and code between shards, these designs must take a "top-down approach" — constructing a particular network topology, featuring a single root ledger and a star or tree of shards, and engineering protocol rules & incentives to enforce that topology. This approach possesses advantages in simplicity and predictability, but faces hard [technical](https://medium.com/nearprotocol/the-authoritative-guide-to-blockchain-sharding-part-1-1b53ed31e060) [problems](https://medium.com/nearprotocol/unsolved-problems-in-blockchain-sharding-2327d6517f43), requires the adherence of all shards to a single validator set (or randomly elected subset thereof) and a single state machine or mutually comprehensible VM, and may face future problems in social scalability due to the general necessity of reaching global consensus on alterations to the network topology.

The *interblockchain communication protocol* takes an orthogonal approach to a differently formulated version of the problem: enabling safe, reliable interoperation of a network of heterogeneous distributed ledgers, arranged in an unknown topology, which can diversify, develop, and rearrange independently of each other or of a particular imposed topology or state machine design. In a wide, dynamic network of interoperating chains, sporadic Byzantine faults are expected, so the protocol must also detect, mitigate, and contain the potential damage of Byzantine faults in accordance with the requirements of the applications & blockchains involved. For a longer list of design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).

To faciliate this heterogeneous interoperation, the interblockchain communication protocol takes a "bottom-up" approach, specifying the set of requirements, functions, and properties necessary to implement interoperation between two ledgers, and then specifying different ways in which multiple interoperating ledgers might be composed which preserve the requirements of higher-level protocols and occupy different points in the safety/speed tradeoff space. IBC thus presumes nothing about and requires nothing of the overall network topology, and of the implementing ledgers requires only that a known, minimal set of functions are available and properties fulfilled.

IBC is an end-to-end, connection-oriented, stateful protocol for reliable, ordered, authenticated communication between modules on separate distributed ledgers. IBC implementations are expected to be co-resident with higher-level modules and protocols on the host ledger. Ledgers hosting IBC must provide a certain set of functions for consensus transcript verification and accumulator proof generation, and IBC packet relayers (off-chain processes) are expected to have access to network protocols and physical datalinks as required to read the state of one ledger and submit data to another.

### Dataflow layers

```
+--------------------------+                           +--------------------------+
| Distributed Ledger A     |                           | Distributed Ledger B     |
|                          |                           |                          |
| +----------+     +-----+ |        +---------+        | +-----+     +----------+ |
| | Module A | <-> | IBC | | <----> | Relayer | <----> | | IBC | <-> | Module B | |
| +----------+     +-----+ |        +---------+        | +-----+     +----------+ |
+--------------------------+                           +--------------------------+
```

## Scope

IBC handles authentication, transport, and ordering of structured data packets relayed between modules on separate ledgers. The protocol is intended to be in simultaneous use between any number of modules on any number of ledgers over arbitrarily structured underlying networks.

## Interfaces

IBC sits between modules — smart contracts, state machine components, or otherwise independent pieces of application logic on ledgers — on one side, and underlying consensus protocols, ledgers, and network infrastructure (e.g. TCP/IP), on the other side.

To modules IBC provides a set of functions much like the functions which might be provided to a module for interacting with another module on the same ledger: sending data packets and receiving data packets on an established connection & channel — in addition to calls to manage the protocol state: opening and closing connections and channels, choosing connection, channel, and packet delivery options. Considerable flexibility is provided to ledger developers as to which of these functions to expose to which modules, and how to restrict parameter choices — if at all — the protocol generally assumes the most permissionless setting possible, and implementers can choose to restrict usage according to their application's requirements.

Of the underlying consensus protocols and ledgers IBC requires a set of primitive functions and properties as defined in [ICS 2](../../spec/ics-2-consensus-requirements), primarily finality, cheaply-verifiable consensus transcripts, and simple key-value store functionality. Of the network infrastructure protocol layer (and physical network layer) IBC requires only eventual data delivery — no authentication, synchrony, or ordering properties are assumed.

## Operation

(inspired by TCP RFC)

### Data relay

- off-chain relayers
- read state from one blockchain
- write it to the other

### Reliability

- underlying network can damage, lose, duplicate packets, behave arbitrarily
- only thing assumed: eventual liveness
- state commitments, indempotent submission

### Flow control & ordering

- ordering of packets (optional)
- application-layer flow control (e.g. value) to bound damage from Byzantine faults

### Authentication

- signed by consensus
- knowledge that other chain is implementing correct IBC protocol
- verify accumulator proofs on other blockchains

### Connections

- stateful information about consensus state of counterparty chain
- metadata on connection, sequence number, encoding formats

### Multiplexing

- channels for multiplexing
- same consensus information, any number of concurrent datastreams

## Philosophy

(inspired by TCP RFC) (do we really need this? seems redudant with a lot of the above)

### Ledger topology

- assume nothing

### Host environment

- ledger state machine

### Interfaces

### Relation to other protocols

### Reliable communication

## Dataflow

IBC can be conceptualized as a layered protocol stack, through which data flows top-to-bottom (when sending IBC packets) and bottom-to-top (when receiving IBC packets).

---

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
