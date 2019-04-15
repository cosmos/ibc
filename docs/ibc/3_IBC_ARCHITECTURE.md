# 3: IBC Architecture

> This is an overview of the high-level architecture & dataflow of the IBC protocol.

> For the design rationale behind the protocol, see [here](./1_IBC_DESIGN_RATIONALE.md).

> For definitions of terms used in IBC specifications, see [here](./2_IBC_TERMINOLOGY.md).

This document outlines the architecture of the authentication, transport, and ordering layers of the IBC protocol stack. This document does not describe specific protocol details — those are contained in individual ICSs.

## What is IBC?

The *inter-blockchain communication protocol* is designed for use as a reliable module-to-module protocol between modules running on independent distributed ledgers across an untrusted network layer.

## What is IBC not?

IBC is not (only) an atomic-swap protocol: arbitrary cross-chain data transfer and computation is supported.

IBC is not (only) a token transfer protocol: token transfer is a possible application-layer use of the IBC protocol.

IBC is not (only) a sharding protocol: there is no single state machine being split across chains, but rather a diverse set of different state machines on different chains which share some common interfaces.

IBC is not (only) a layer-two scaling protocol: all chains implementing IBC exist on the same "layer", although they may occupy different points in the network topology, and there is no single root chain or single validator set.

## Motivation

### Concurrent heterogeneous networks of ledgers

### Reliable inter-module communication

- connection-oriented, stateful, end-to-end
- reliable inter-module communication
- assumptions of lower layers?
- co-resident with higher level protocols
- cite near protocol post

## Scope

IBC handles authentication, transport, and ordering of structured data packets relayed between modules on separate ledgers. The protocol is intended to be in simultaneous use between any number of modules on any number of ledgers over arbitrarily structured underlying networks.

## Interfaces

IBC sits between modules — smart contracts, state machine components, or otherwise independent pieces of application logic on ledgers — on one side, and underlying consensus protocols, ledgers, and network infrastructure (e.g. TCP/IP), on the other side.

To modules IBC provides a set of functions much like the functions which might be provided to a module for interacting with another module on the same ledger: sending data packets and receiving data packets on an established connection & channel — in addition to calls to manage the protocol state: opening and closing connections and channels, choosing connection, channel, and packet delivery options. Considerable flexibility is provided to ledger developers as to which of these functions to expose to which modules, and how to restrict parameter choices — if at all — the protocol generally assumes the most permissionless setting possible, and implementers can choose to restrict usage according to their application's requirements.

Of the underlying consensus protocols and ledgers IBC requires a set of primitive functions and properties as defined in [ICS 2](../spec/ics-2-consensus-requirements), primarily finality, cheaply-verifiable consensus transcripts, and simple key-value store functionality. Of the network infrastructure protocol layer (and physical network layer) IBC requires only eventual data delivery — no authentication, synchrony, or ordering properties are assumed.

## Operation

### Data relay

### Reliability

### Flow control & ordering

### Authentication

### Connections

### Multiplexing

## Philosophy

### Ledger topology

### Host environment

### Interfaces

### Relation to other protocols

### Reliable communication

## Dataflow

IBC can be conceptualized as a layered protocol stack, through which data flows top-to-bottom (when sending IBC packets) and bottom-to-top (when receiving IBC packets).

Consider the path of an IBC packet between two chains — call them *A* and *B*:

---

Dataflow on chain *A*:

Actor --> Module --> Handler -> Packet --> Channel --> Connection --> Consensus -->

---

Off-chain (note that one relayer can handle many chains, connections, and packets):

--> Relayer -->

---

Dataflow on chain *B*:

--> Consensus --> Connection --> Channel --> Packet --> Handler --> Module

---

## Packet Traversal

Consider the path of an IBC packet between two chains — call them *A* and *B*.

1. On chain *A*
    1. Actor (application-specific)
    1. Module (application-specific)
    1. Handler (parts defined in different ICSs)
    1. Packet (defined in [ICS 5](../spec/ics-5-packet-semantics))
    1. Channel (defined in [ICS 4](../spec/ics-4-channel-semantics))
    1. Connection (defined in [ICS 3](../spec/ics-3-connection-semantics))
    1. Consensus (defined in [ICS 2](../spec/ics-2-consensus-requirements))
2. Off-chain
    1. Relayer (defined in [ICS 18](../spec/ics-18-offchain-relayer))
3. On chain *B*
    1. Consensus (defined in [ICS 2](../spec/ics-2-consensus-requirements))
    1. Connection (defined in [ICS 3](../spec/ics-3-connection-semantics))
    1. Channel (defined in [ICS 4](../spec/ics-4-channel-semantics))
    1. Packet (defined in [ICS 5](../spec/ics-5-packet-semantics))
    1. Handler (parts defined in different ICSs)
    1. Module (application-specific)
