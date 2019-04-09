## IBC Architecture

> This is an overview of the high-level architecture & dataflow of the IBC protocol.

> For the design rationale behind the protocol, see [here](./IBC_DESIGN_RATIONALE.md).

> For definitions of terms used in IBC specifications, see [here](./IBC_TERMINOLOGY.md).

This document outlines the architecture of the authentication, transport, and ordering layers of the IBC protocol stack.

### Protocol Stack

IBC can be conceptualized as a layered protocol stack, through which data flows top-to-bottom (when sending IBC packets) and bottom-to-top (when receiving IBC packets).

Consider the path of an IBC packet between two chains — call them *A* and *B*:

---

Dataflow on chain *A*:

Actor --> Module --> Handler -> Packet --> Channel --> Connection --> Consensus -->

---

Off-chain:

--> Relayer -->

---

Dataflow on chain *B*:

--> Consensus --> Connection --> Channel --> Packet --> Handler --> Module

---

### Packet Traversal

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
  . 1. Channel (defined in [ICS 4](../spec/ics-4-channel-semantics))
    1. Packet (defined in [ICS 5](../spec/ics-5-packet-semantics))
    1. Handler (parts defined in different ICSs)
    1. Module (application-specific)
