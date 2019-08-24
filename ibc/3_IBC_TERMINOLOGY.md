# 3: IBC Terminology

**This is an overview of terms used in IBC specifications.**

**For an architectural overview, see [here](./1_IBC_ARCHITECTURE.md).**

**For a broad set of protocol design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).**

**For a set of example use cases, see [here](./4_IBC_USECASES.md).**

**For a discussion of design patterns, see [here](./5_IBC_DESIGN_PATTERNS.md).**

This document provides definitions in plain English of key terms used throughout the IBC specification set.

## Abstraction definitions

### Actor

An *actor*, or a *user* (used interchangeably), is an entity interacting with the IBC protocol. An actor can be a human end-user, a module or smart contract running on a blockchain, or an off-chain relayer process capable of signing transactions.

### Machine / Chain / Ledger

A *machine*, *chain*, *blockchain*, or *ledger* (used interchangeably), is a state machine (which may be a distributed ledger, or "blockchain", although a strict chain of blocks may not be required) implementing part or all of the IBC specification.

### State Machine

The *state machine* of a particular chain defines the structure of the state as well as the set of rules which determines valid transactions that trigger state-transitions based on the current state agreed upon by the consensus algorithm of the chain.

### Consensus

A *consensus* algorithm is the protocol used by the set of processes operating a distributed ledger to come to agreement on the same state, generally under the presence of a bounded number of Byzantine faults.

### Consensus State

The *consensus state* is the set of information about the state of a consensus algorithm required to verify proofs about the output of that consensus algorithm (e.g. commitment roots in signed headers).

### Trusted State

A *trusted state* is a particular state which a machine has decided to assume to be correct, either a priori or having verified a progression from a preceding trusted state.

### Header

A *header* is an update to the consensus state of a particular blockchain that can be verified in a well-defined fashion by a "light client" algorithm defined for that particular consensus algorithm.

### Commitment 

A cryptographic *commitment* is a way to cheaply verify membership of a key => value pair in a mapping, where the mapping can be committed to with a short witness string.

### CommitmentProof

A *commitment proof* refers to the proof structure which proves whether a particular key maps to a particular value in a committed-to set or not.

### Handler Module

The IBC *handler module* is the module within the state machine which implements [ICS 25](../spec/ics-025-handler-module), managing clients, connections, & channels, verifying proofs, and storing appropriate commitments for packets.

### Relayer Module

The IBC *relayer module* is the module within the state machine which implements [ICS 26](../spec/ics-026-relayer-module), routing packets between the handler module and other modules on the host state machine which utilise the relayer module's external interface.

### Datagram

A *datagram* is an opaque bytestring transmitted over some physical network, and handled by the IBC relayer module implemented in the ledger's state machine. In some implementations, the datagram may be a field in a ledger-specific transaction or message data structure which also contains other information (e.g. a fee for spam prevention, nonce for replay prevention, type identifier to route to the IBC handler, etc.). All IBC sub-protocols (such as opening a connection, creating a channel, sending a packet) are defined in terms of sets of datagrams and protocols for handling them through the relayer module.

### Connection

A *connection* is a set of persistent data structures on two chains that contain information about the consensus state of the other ledger in the connection. Updates to the consensus state of one chain changes the state of the connection object on the other chain.

### Channel

A *channel* is a set of persistent data structures on two chains that contain metadata to facilitate packet ordering, exactly-once delivery, and replay prevention. Packets sent through a channel change its internal state. Channels are associated with connections in a many-to-one relationship — a single connection can have any number of associated channels, and all channels must have a single associated connection, which must be open in order for the channel to be used.

### Packet

A *packet* is a particular data structure with sequence-related metadata (defined by the IBC specification) and an opaque value field referred to as the packet *data* (with semantics defined by the application layer, e.g. token amount and denomination). Packets are sent through a particular channel (and by extension, through a particular connection).

### Module

A *module* is a sub-component of the state machine of a particular blockchain which may interact with the IBC handler and alter state according to the *data* field of particular IBC packets sent or received (minting or burning tokens, for example).

### Handshake

A *handshake* is a particular class of sub-protocol involving multiple datagrams, generally used to initialise some common state on the two involved chains such as trusted states for each others' consensus algorithms.

### Sub-protocol

Sub-protocols are defined as a set of datagram kinds and functions which must be implemented by the IBC handler module of the implementing blockchain.

Datagrams must be relayed between chains by an external process. This process is assumed to behave in an arbitrary manner — no safety properties are dependent on its behaviour, although progress is generally dependent on the existence of at least one correct relayer process.

IBC sub-protocols are reasoned about as interactions between two chains `A` and `B` — there is no prior distinction between these two chains and they are assumed to be executing the same, correct IBC protocol. `A` is simply by convention the chain which goes first in the sub-protocol and `B` the chain which goes second. Protocol definitions should generally avoid including `A` and `B` in variable names to avoid confusion (as the chains themselves do not know whether they are `A` or `B` in the protocol).

### Authentication

*Authentication* refers to the protocols used to ensure that datagrams were in fact sent by a particular chain and associated state alterations committed by it.

## Property definitions

### Finality

*Finality* is the quantifiable assurance provided by a consensus algorithm that a particular block will not be reverted, subject to certain assumptions about the behaviour of the validator set. The IBC protocol requires finality, although it need not be absolute (for example, a threshold finality gadget for a Nakamoto consensus algorithm will provide finality subject to economic assumptions about how miners behave).

### Misbehaviour

*Misbehaviour* refers to a class of consensus fault defined by a consensus algorithm & detectable (possibly also attributable) by the light client of that consensus algorithm.

### Equivocation

*Equivocation* refers to a particular class of consensus fault committed by a validator or validators which sign votes on multiple different successors to a single block. All equivocations are misbehaviours.

### Data availability

*Data availability* refers to the ability of off-chain relayer processes to retrieve data in the state of a machine within some time bound.

### Data confidentiality

*Data confidentiality* refers to the ability of the host state machine to refuse to make particular data available to particular parties without impairing the functionality of the IBC protocol.

### Consensus liveness

*Consensus liveness* refers to the continuance of block production by the consensus algorithm of a particular machine.

### Transactional liveness

*Transactional liveness* refers to the confirmation of incoming transactions (which transactions should be clear by context) by the consensus algorithm of a particular machine. Transactional liveness requires consensus liveness, but consensus liveness does not necessarily provide transactional liveness.

### Consensus synchrony

*Consensus synchrony* refers to consensus liveness within a particular bound.

### Transactional synchrony

*Transactional synchrony* refers to transactional liveness within a particular bound.

### Exactly-once safety

*Exactly-once safety* refers to the property that a packet is confirmed no more than once (and generally exactly-once assuming eventual transactional liveness).

### Deliver-or-timeout safety

*Deliver-or-timeout safety* refers to the property that a packet will either be delivered & executed or will timeout in a way that can be proved back to the sender.

### Constant (w.r.t. complexity)

*Constant*, when referring to space or time complexity, means `O(1)`.

### Succinct

*Succinct*, when referring to space or time complexity, means `O(log n)` or better.
