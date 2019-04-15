# 3: IBC Architecture

> This is an overview of the high-level architecture & dataflow of the IBC protocol.

> For the design rationale behind the protocol, see [here](./1_IBC_DESIGN_RATIONALE.md).

> For definitions of terms used in IBC specifications, see [here](./2_IBC_TERMINOLOGY.md).

This document outlines the architecture of the authentication, transport, and ordering layers of the IBC protocol stack.

## What is IBC?

The *inter-blockchain communication protocol* is designed for use as a reliable module-to-module protocol between modules running on independent distributed ledgers across an untrusted network layer. The protocol handles authentication, transport, and ordering of structured data packets relayed between modules on separate ledgers.

## What is IBC not?

IBC is not (only) an atomic-swap protocol: arbitrary cross-chain data transfer and computation is supported.

IBC is not (only) a token transfer protocol: token transfer is a possible application-layer use of the IBC protocol.

IBC is not (only) a sharding protocol: there is no single state machine being split across chains, but rather a diverse set of different state machines on different chains which share some common interfaces.

IBC is not (only) a layer-two scaling protocol: all chains implementing IBC exist on the same "layer", although they may occupy different points in the network topology, and there is no single root chain or single validator set.

## Motivation

- blockchains are useful
- connection-oriented, stateful, end-to-end
- reliable inter-module communication
- assumptions of lower layers?
- co-resident with higher level protocols
- cite near protocol post

## Scope

## Interfaces

## Operation

- data relay
- reliability
- flow control & ordering
- authentication
- multiplexing
- connections

(explain each)

## Philosophy

- elements of the ledger topology
- host environment
- interfaces
- relation to other protocols
- reliable communication

## Protocol Stack

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
