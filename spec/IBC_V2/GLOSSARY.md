# Glossary of Terms

This document serves as a glossary of terms for IBC v2 so that new readers can familiarize themselves with the terms used throughout the specification. The terms are not ordered alphabetically but rather in the order readers are likely to encounter them. The terms listed here are used throughout the specifications and have no explicit definition in the individual documents. For definitions of client-specific terms, please see [ICS-2](./core/ics-002-client-semantics/README.md). For definitions of packet-specific terms please see [ICS-4](./core/ics-004-packet-semantics/PACKET.md).

**ledger/chain/state machine**: This is the verifiable computer that implements IBC protocol to communicate with other IBC-supporting ledgers. It must contain a state machine that is able to create provable commitments of snapshotted states. This can be a blockchain that produces commits signed by the validator set with a merklized root hash of the state. Or it can be a single computer with a signature over its state root or many other possible configurations.

**module/application**: A module is an isolated component of the state machine that maintains control over its own subset of the global state that can be modified through access-controlled methods like transaction handlers or inter-module API endpoints.

**identifier**: An identifier is an opaque string used to uniquely identify a component of IBC state: ie clients, applications, etc. Since uniqueness can only be guaranteed within the state machine, an identifier can be guaranteed to be unique on the chain it is on, but not globally across the IBC network. For example, there can only be one `client-1` identifier mapping to a clientstate on a given chain A, however there may also exist a `client-1` identifier on chain B and chain C.

**consensus algorithm/consensus**: The consensus algorithm is the process by which a ledger agrees on a state update to the state machine and produces a new provable commitment of the state machine. An example of a consensus algorithm is the CometBFT consensus algorithm. If the consensus algorithm is compute-intensive, there should be a light client algorithm that can efficiently verify the execution of the consensus algorithm.

**light client algorithm/Validity Predicate**: The light client algorithm validates a new provable commitment of state given a previous trusted commitment and a client-specific message that includes the proof that the new commitment was generated from the previous commitment by the counterparty ledger. This algorithm should be more efficient than executing the consensus algorithm directly.

**commitment/root**: The state of an IBC ledger must be represented in a small-size provable commitment that can be verified by a light client algorithm. This commitment while small in size must be able to securely prove that a key membership/non-membership in the original state. It should also be computationally infeasible to construct a valid proof against the commitment for an invalid key membership/nonmembership of the original state. An example of a commitment is the root hash of the Merkle tree for a Merklized state machine. 

**IBC Client/client**: An IBC Client will track the state updates of a counterparty ledger by executing the light client algorithm to verify new state commitments. These commitments are stored on the light client for future use in proving key/value pairs in the counterparty IBC state.

**relayer**: The relayer is an off-chain process that enables cross-chain communication between IBC chains. It keeps the clients updated by submitting client-specific messages to execute the validity predicate and add new commitments. It also enables packet flow by verifying the packet flow against the state commitments stored in the IBC client. Since everything provided by the relayer is verified on-chain before being used, the relayer is not trusted for security and is only relied upon for liveness of the IBC connections.

**IBC Connection**: An IBC connection is the flow of packets between two chains.
