# Micro IBC Architecture

### Context

The implementation of the entire IBC protocol as it currently stands is a large undertaking. While there exists ready-made implementations like ibc-go this is only deployable on the Cosmos-SDK. Similarly, there exists ibc-rs which is a library for chains to integrate. However, this requires the chain to be implemented in Rust, there still exists some non-trivial work to integrate the ibc-rs library into the target state machine, and certain limitations either in the state machine or in ibc-rs may prevent using the library for the target chain.

Writing an implementation from scratch is a problem many ecosystems face as a major barrier for IBC adoption.

The goal of this document is to serve as a "micro-IBC" specification that will allow new ecosystems to implement a protocol that can communicate with fully implemented IBC chains using the same security assumptions.

The micro-IBC protocol must have the same security properties as IBC, and must be completely compatible with IBC applications. It may not have the full flexibility offered by standard IBC.

### Desired Properties

- Light-client backed security
- Unique identifiers for each channel end
- Authenticated application channel (must verify that the counterparty is running the correct client and app parameters)
- Applications must be mutually compatible with standard IBC applications.
- Must be capable of being implemented in smart contract environments with resource constraints and high gas costs.

### Specification

### Light Clients

The light client module can be implemented exactly as-is with regards to its functionality. It **must** have external endpoints for relayers (off-chain processes that have full-node access to other chains in the network) to initialize a client, update the client, and submit misbehaviour in case the trust model of the client is violated by the counterparty consensus mechanism (e.g. committing to different headers for the same height).

The implementation of each of these endpoints will be specific to the particular consensus mechanism targetted. The choice of consensus algorithm itself is arbitrary, it may be a Proof-of-Stake algorithm like CometBFT, or a multisig of trusted authorities, or a rollup that relies on an additional underlying client in order to verify its consensus.

Thus, the endpoints themselves should accept arbitrary bytes for the arguments passed into these client endpoints as it is up to each individual client implementation to unmarshal these bytes into the structures they expect.

```typescript
// initializes client with a starting client state containing all light client parameters
// and an initial consensus state that will act as a trusted seed from which to verify future headers
function createClient(
    clientState: bytes,
    consensusState: bytes,
): (Identifier, error)

// once a client has been created, it can be referenced with the identifier and passed the header
// to keep the client up-to-date. In most cases, this will cause a new consensus state derived from the header
// to be stored in the client
function updateClient(
    clientId: Identifier,
    header: bytes,
): error

// once a client has been created, relayers can submit misbehaviour that proves the counterparty chain
// The light client must verify the misbehaviour using the trust model of the consensus mechanism
// and execute some custom logic such as freezing the client from accepting future updates and proof verification.
function submitMisbehaviour(
    clientId: Identifier,
    misbehaviour: bytes,
): error
```

### Router

IBC in its essence is the ability for applications on different blockchains with different security models to communicate with each other through light-client backed security. Thus, IBC needs the light client described above and the IBC applications that define the packet data they wish to send and receive. In addition to these layers, core IBC introduces the connection and channel abstractions to connect these two fundamental layers. Micro IBC intends to compress only the necessary aspects of connection and channel layers to a new router layer but before doing this it is critical to understand what service they currently provide.

Properties of Connection:

- Verifies the validity of the counterparty client
- Establishes a unique identifier on each side for a shared abstract understanding (the connection)
- Establishes an agreement on the IBC version and supported features
- Allows multiple connections to be built against the same client pair
- Establishes the delay period so this security parameter can be instantiated differently for different connections against the same client pairing.
- Defines which channel orderings are supported

Properties of Channel:

- Separates applications into dedicated 1-1 communication channels. This prevents applications from writing into each other's channels.
- Allows applications to come to agreement on the application parameters (version negotiation). Ensures that each side can understand the other's communication and that they are running mutually compatible logic. This version negotiation is a multi-step process that allows the finalized version to differ substantially from the one initially proposed
- Establishes the ordering of the channel
- Establishes unique identifiers for the applications on either chain to use to reference each other when sending and receiving packets.
- The application protocol can be continually upgraded over time by using the upgrade handshake which allows the same channel which may have accumulated state to use new mutually agreed upon application packet data format(s) and associated new logic.
- Ensures exactly-once delivery of packet flow datagrams (Send, Receive, Acknowledge, Timeout)
- Ensures valid packet flow (Send => Receive => Acknowledge) XOR (Send => Timeout)

Of these which are the critical properties that micro-IBC must maintain:

Desired Properties of micro-IBC:

##### Authenticating Counterparty Clients

Before application data can flow between chains, we must ensure that the clients are both valid views of the counterparty consensus.

In the router we must then introduce an ability to submit the counterparty client state and consensus state for verification against a client stored in our own chain.

```typescript
function verifyCounterpartyClient(
    localClient: Identifer, // this is the client of the counterparty that exists on our own chain
    remoteClientStoreIdentifier: Identifier, // this is the identifier of the 
    remoteClient: ClientState, // this is the client on the counterparty chain that purports to be a client of ourselves
    remoteConsensusState: ConsensusState, // this is the consensus state that is being used for verification of our consensus
    remoteConsensusHeight: Height, // this is the height of our chain that the remote consensus state is associated with
    // the proof fields are written in IBC convention,
    // but implementations in practice will use []byte for proof
    // and an unsigned integer for the height
    // as their local client implementation will expect for VerifyMembership
    proofClient: CommitmentProof,
    proofConsensus: CommitmentProof,
    proofHeight: Height,
) {
    // validate that the remote client and remote consensus state
    // are valid for our chain. Note: This requires the ability to introspect our own consensus within this function
    // e.g. ability to verify that the validator set at the height of the consensus state was in fact the validator set on our chaina that height 
    validateSelfClient(remoteClient)
    validateSelfConsensus(remoteConsensusState, remoteConsensusHeight)

    // use the local client to verify that the remote client and remote consensus state are stored as expected under the remoteClientStoreIdentifier with the ICS24 paths we expect.
    clientPath = append(remoteClientStoreIdentifier, "/clientState")
    consensusPath = append(remoteClientStoreIdentifier, "/consensusState/{remoteConsensusHeight}")
    assert(localClient.VerifyMembership(
        proofHeight,
        0,
        0,
        proofClient,
        clientPath,
        proto.marshal(remoteClient),
    ))
    assert(localClient.VerifyMembership(
        proofHeight,
        0,
        0,
        proofConsensus,
        consensusPath,
        proto.marshal(remoteConsensusState),
    )
}
```

#### Identification

// TODO store the counterparty ClientStoreIdentifier and ApplicationStoreIdentifier so we know where they are storing client states, and packet commitments
// A secure, unique connection consists of
// (ChainA, ClientStoreID, AppStoreID) => (ChainB, ClientStoreID, AppStoreID)
// where both sides are aware of each others store ID's

