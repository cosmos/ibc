### Architecture of IBC

> This is an overview of the architecture of IBC. For the design rationale behind the protocol, see [here](./WHY_IBC.md).

In plain English, with definitions

### Definitions

#### Actor

#### Chain

A ledger, or chain, is a distributed ledger (or "blockchain", although a strict chain of blocks may not be required) implementing part or all of the IBC specification as a component or module within its state machine.

#### Consensus

#### Authorization

#### Equivocation

#### Handler

An IBC handler is the module or subcomponent within the state machine of a ledger responsible for implementing the IBC specification by "handling" datagrams, performing the appropriate checks, proof verifications, and/or storage alterations, and routing valid packets to other parts of the state machine, as defined by the application-layer semantics.

#### Module

#### Datagram

A datagram is an opaque data blob transmitted over some network, and handled by the top-level IBC handler implemented in the ledger's state machine. In some implementations, the datagram may be a field in a ledger-specific transaction or message data structure which also contains other information (e.g. a fee for spam prevention, nonce for replay prevention, type identifier to route to the IBC handler, etc.)

#### Connection

A connection is a persistent data structure on a particular ledger that contains information about the consensus state of another ledger. Updates to that consensus state change the state of the connection.

#### Channel

A channel is a persistent data structure on a particular ledger that contains metadata to facilitate packet ordering, exactly-once delivery, and replay prevention. Often it may make sense to reason about a "channel" across two chains (reasoning about subsets of state on both ledgers).

#### Packet

A packet is a particular data structure with sequence-related metadata (defined by the IBC specification) and an opaque value field (with semantics defined by the application layer).

### Terms

#### Handshake

#### Trust
