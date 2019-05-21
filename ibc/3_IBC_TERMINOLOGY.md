# 3: IBC Terminology

**This is an overview of terms used in IBC specifications.**

**For an architectural overview, see [here](./1_IBC_ARCHITECTURE.md).**

**For a broad set of protocol design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).**

**For a set of example use cases, see [here](./4_IBC_USECASES.md).**

This document provides definitions in plain English of key terms used throughout the IBC specification set.

## Definitions

### Actor

An *actor*, or a *user* (used interchangeably), is an entity interacting with the IBC protocol. An actor can be a human end-user, a module or smart contract running on a blockchain, or an off-chain relayer process capable of signing transactions.

### Chain / Ledger

A *chain*, *blockchain*, or *ledger* (used interchangeably), is a distributed ledger (or "blockchain", although a strict chain of blocks may not be required) implementing part or all of the IBC specification as a component or module within its state machine.

### State Machine

The *state machine* of a particular chain defines the structure of the state as well as the set of rules which determines valid transactions that trigger state-transitions based on the current state agreed upon by the consensus algorithm of the chain.

### Consensus

A *consensus* algorithm is the protocol used by the set of processes operating a distributed ledger to come to agreement on the same state, generally under the presence of a bounded number of Byzantine faults.

### Consensus State

The *consensus state*, or the *root-of-trust*, is the set of information about the state of a consensus algorithm required to verify proofs about the output of that consensus algorithm (e.g. commitment roots in signed headers).

### Header

A *header* is an update to the consensus state of a particular blockchain that can be verified in a well-defined fashion by a "light client" algorithm defined for that particular consensus algorithm.

### Finality

*Finality* is the guarantee provided by a consensus algorithm that a particular block will not be reverted, subject to certain assumptions about the behavior of the validator set. The IBC protocol requires finality.

### Commitment 

A cryptographic *commitment* is a way to cheaply verify membership of a key => value pair in a mapping, where the mapping can be committed to with a short witness string. An *vector commitment proof* refers to the proof structure which proves whether a particular key maps to a particular value in a committed-to set or not.

### Handler

An IBC *handler* is the module or subcomponent within the state machine of a ledger responsible for implementing the IBC specification by "handling" datagrams, performing the appropriate checks, proof verifications, and/or storage alterations, and routing valid packets to other parts of the state machine, as defined by the application-layer semantics.

### Datagram

A *datagram* is an opaque bytestring transmitted over some physical network, and handled by the top-level IBC handler implemented in the ledger's state machine. In some implementations, the datagram may be a field in a ledger-specific transaction or message data structure which also contains other information (e.g. a fee for spam prevention, nonce for replay prevention, type identifier to route to the IBC handler, etc.). All IBC subprotocols (such as opening a connection, creating a channel, sending a packet) are defined in terms of sets of datagrams and protocols for handling them.

### Connection

A *connection* is a set of persistent data structures on two chains that contain information about the consensus state of the other ledger in the connection. Updates to the consensus state of one chain changes the state of the connection object on the other chain.

### Channel

A *channel* is a set of persistent data structures on two chains that contain metadata to facilitate packet ordering, exactly-once delivery, and replay prevention. Packets sent through a channel change its internal state. Channels are associated with connections in a many-to-one relationship — a single connection can have any number of associated channels, and all channels must have a single associated connection, which must be open in order for the channel to be used.

### Packet

A *packet* is a particular data structure with sequence-related metadata (defined by the IBC specification) and an opaque value field referred to as the packet *data* (with semantics defined by the application layer, e.g. token amount and denomination). Packets are sent through a particular channel (and by extension, through a particular connection).

### Module

A *module* is a subcomponent of the state machine of a particular blockchain which may interact with the IBC handler and alter state according to the *data* field of particular IBC packets sent or received (minting or burning tokens, for example).

## Auxiliary Terms

### Handshake

A *handshake* is a particular class of subprotocol involving multiple datagrams, generally used to initialize some common state on the two involved chains such as roots-of-trust for each others' consensus algorithms.

### Trust

To *trust* a blockchain or validator set means to expect that the validator set will behave in a particular way (such as < 1/3 Byzantine) relative to a well-defined consensus & state machine protocol.

### Authentication

*Authentication* refers to the protocols used to ensure that datagrams were in fact sent by a particular chain and associated state alterations committed by it.

### Equivocation

*Equivocation* refers to a particular class of consensus fault committed by a validator or validators which sign votes on multiple different successors to a single block.

### Subprotocol

Subprotocols are defined as a set of datagram types and functions which must be implemented by the IBC handler module of the implementing blockchain.

Datagrams must be relayed between chains by an external process. This process is assumed to behave in an arbitrary manner — no safety properties are dependent on its behavior, although progress is generally dependent on the existence of at least one correct relayer process.

IBC subprotocols are reasoned about as interactions between two chains `A` and `B` — there is no prior distinction between these two chains and they are assumed to be executing the same, correct IBC protocol. `A` is simply by convention the chain which goes first in the subprotocol and `B` the chain which goes second. Protocol definitions should generally avoid including `A` and `B` in variable names to avoid confusion (as the chains themselves do not know whether they are `A` or `B` in the protocol).
