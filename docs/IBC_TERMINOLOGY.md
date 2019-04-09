### IBC Terminology

> This is an overview of terms used in IBC specifications.

> For an architectural overview, see [here](./IBC_ARCHITECTURE.md).

> For the design rationale behind the protocol, see [here](./IBC_DESIGN_RATIONALE.md).

This document provides definitions in plain English of key terms used throughout the IBC specification set.

### Definitions

#### Actor

An *actor*, or a *user* (used interchangeably), is an entity interacting with the IBC protocol. An actor can be a human end-user, a module or smart contract running on a blockchain, or an off-chain relayer process capable of signing transactions.

#### Chain

A *chain*, or *ledger* (used interchangeably), is a distributed ledger (or "blockchain", although a strict chain of blocks may not be required) implementing part or all of the IBC specification as a component or module within its state machine.

#### State Machine

The *state machine* of a particular ledger is the set of rules which determines valid transactions and blocks based on the current state agreed upon by the consensus algorithm of the ledger.

#### Consensus

A *consensus* algorithm is the protocol used by the set of processes operating a distributed ledger to come to agreement on the same state, generally under the presence of a bounded number of Byzantine faults.

#### Handler

An IBC *handler* is the module or subcomponent within the state machine of a ledger responsible for implementing the IBC specification by "handling" datagrams, performing the appropriate checks, proof verifications, and/or storage alterations, and routing valid packets to other parts of the state machine, as defined by the application-layer semantics.

#### Datagram

A *datagram* is an opaque data blob transmitted over some physical network, and handled by the top-level IBC handler implemented in the ledger's state machine. In some implementations, the datagram may be a field in a ledger-specific transaction or message data structure which also contains other information (e.g. a fee for spam prevention, nonce for replay prevention, type identifier to route to the IBC handler, etc.). All IBC subprotocols (such as opening a connection, creating a channel, sending a packet) are defined in terms of sets of datagrams and protocols for handling them.

#### Connection

A *connection* is a set of persistent data structures on particular ledgers (usually two) that contain information about the consensus state of the other ledgers in the connection. Updates to their consensus states change the state of the connections.

#### Channel

A *channel* is a set of persistent data structures on particular ledgers (usually two) that contain metadata to facilitate packet ordering, exactly-once delivery, and replay prevention. Packets sent through a channel change its internal state. Channels are associated with connections in a many-to-one relationship — a single connection can have any number of associated channnels, and all channels must have a single associated connection.

#### Packet

A *packet* is a particular data structure with sequence-related metadata (defined by the IBC specification) and an opaque value field referred to as the packet *data* (with semantics defined by the application layer, e.g. token amount and denomination). Packets are sent through a particular channel (and by extension, through a particular connection).

#### Module

A *module* is a subcomponent of the state machine of a particular blockchain which interacts with the IBC handler and alters state according to the *data* field of particular IBC packets sent or received (minting or burning tokens, for example).

### Auxiliary Terms

#### Handshake

A *handshake* is a particular class of subprotocol involving multiple datagrams, generally used to initialize some state on multiple chains.

#### Trust

To *trust* a blockchain or validator set means to expect that the validator set will behave in a particular way (such as < 1/3 Byzantine) relative to a well-defined consensus & state machine protocol.

#### Authorization

*Authorization* refers to the protocols used to ensure that datagrams were in fact sent by a particular chain and associated state alterations committed by it. 

#### Equivocation

*Equivocation* refers to a particular class of consensus fault committed by a validator or validators which sign multiple different sucessors to a single block.
