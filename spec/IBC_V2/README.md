---
title: IBC v2
stage: EXPERIMENTAL
version compatibility: ibc-go v10.0.0
author: Aditya Sripal <aditya@interchainlabs.io>
created: 2024-08-15
---

## IBC v2

### Introduction

IBC v2 is an end-to-end protocol for reliable, authenticated communication between modules on separate distributed ledgers. IBC makes NO assumptions about the consensus algorithm or the state machine. So long as the distributed ledger satisfies the minimal requirements in [ICS-24 Host Requirements](../core/ics-024-host-requirements/README.md), it can support the IBC v2 protocol and communicate across any application in the IBC v2 network.

The IBC v2 protocol can be conceptualized in three distinct layers: **IBC CLIENTS**, **IBC CORE**, and **IBC APPS**. **IBC APPS** are the modules that wish to communicate with each other across different ledgers in the IBC v2 network. On example is ICS-20 fungible token transfer which facilitates sending tokens securely from one ledger to another by sending token packet data using **IBC CORE** and executing escrow/mint logic upon sending/receiving the ICS-20 token packet data from counterparty ICS20 applications.
**IBC CLIENTS** identifies and verifies the state of the counterparty ledger. An **IBC CLIENT** is responsible for tracking updates to the state machine and verifying state against a given update.
**IBC CORE** is the handler that implements the transport, authentication, and ordering semantics (hereafter `IBC/TAO`), it **uses** the **IBC CLIENT** to authenticate the packet and then sends the application packet data to the **IBC APP** which will handle the application data. Thus, each layer has a specific isolated responsibility. The **IBC CLIENT** only needs to verify key/value proofs of the counterparty state. The **IBC APP** only needs to process application data coming from a counterparty application. **IBC CORE** enables authenticated IBC packet flow of `SendPacket`, `RecvPacket`, `AcknowledgePacket`, `TimeoutPacket` for the **IBC APP** using the **IBC CLIENT** as a verification oracle.

The goal of this document is to provide a basic overview of the IBC V2 protocol. Where appropriate, distinctions from IBC v1 will be highlighted. For a detailed specification of each layer please refer to the ICS-standards.

### Specification

### IBC Clients

The IBC Client keeps track of counterparty state updates and exposes a verifier of the counterparty state to **IBC CORE**. An IBC client implementation achieves this through the use of two distinct structures: the `ClientState` and the `ConsensusState`. From the perspective of IBC, these are opaque bytes and are defined by the specific light client implementation. The `ClientState` is intended to encapsulate parameters of the counterparty consensus that SHOULD NOT change across heights, this can include a chain identifier and security parameters like a staking unbonding period. The `ConsensusState` on the other hand is a **view** or a snapshot of the counterparty consensus at a particular height. This **view** in almost all cases will be a highly compressed view of the counterparty consensus. The **IBC CLIENT** will not store the entire state of the counterparty chain, nor will it execute all transactions of the counterparty chain as this would be equivalent to hosting a full node. A common pattern is to have the counterparty Consensus to create a Merklized commitment of the counterparty state on each state update. The **IBC CLIENT** can add the merkle root hash to the `ConsensusState` and then verify membership/nonmembership proofs against the root hash in the stored consensus state.

The **IBC CLIENT** encapsulates a particular security model, this can be anything from a multisign bridge committee to a fully verified light client of the counterparty consensus algorithm. It is up to users sending packets on the client to decide whether the security model is acceptable to them or not.
The IBC client is responsible for taking an initial view of the counterparty consensus, and updating that view from this trusted point using the security model instantiated in the client. This initial counterparty consensus is trusted axiomatically. Thus, IBC does not have any **in-protocol** awareness of which chain a particular client is verifying. A user can inspect a client and verify for themselves that the client is tracking the chain that they care about (i.e. validating a specific consensus state matches the consensus output of the desired counterparty chain) and that the parameters of the security model encoded in the `ClientState` are satisfactory. A user need only verify the client once, either by themselves or through social consensus, once that initial trust is established; the **IBC CLIENT** MUST continue updating the view of the counterparty state from previously trusted views given the parameterized security model. Thus, once a user trusts a light client they can be guaranteed that the trust will not be violated by the client.

If the security model is violated by counterparty consensus, the **IBC CLIENT** implementation MUST provide the ability to freeze the client and prevent further updates and verification. The evidence for this violation is called `Misbehaviour` and upon verification the client is frozen and all packet processing against the client is paused. Any damage already done cannot be automatically reverted in-protocol, however this mechanism ensures the attack is stopped as soon as possible so that an out-of-band recovery mechanism can intervene (e.g. governance). This recovery mechanism should ensure the consensus violation is corrected on the counterparty and any invalid state is reverted to the extent possible before resuming packet processing by unfreezing the client. 

The **IBC CLIENT** **must** have external endpoints for relayers (off-chain processes that have full-node access to other chains in the network) to initialize a client, update the client, and submit misbehaviour.

The implementation of each of these endpoints will be specific to the particular consensus mechanism targeted. The choice of consensus algorithm itself is arbitrary, it may be a Proof-of-Stake algorithm like CometBFT, or a multisig of trusted authorities, or a rollup that relies on an additional underlying client in order to verify its consensus. However, a light client must have the ability to define finality for a given snapshot of the state machine, this may be either through single-slot finality or a finality gadget.

Thus, the endpoints themselves should accept arbitrary bytes for the arguments passed into these client endpoints as it is up to each individual client implementation to unmarshal these bytes into the structures they expect.

```typescript
// initializes client with a starting client state containing all light client parameters
// and an initial consensus state that will act as a trusted seed from which to verify future headers
function createClient(
    clientState: bytes,
    consensusState: bytes,
): (id: bytes, err: error)

// once a client has been created, it can be referenced with the identifier and passed the header
// to keep the client up-to-date. In most cases, this will cause a new consensus state derived from the header
// to be stored in the client
function updateClient(
    clientId: bytes,
    header: bytes,
): error

// once a client has been created, relayers can submit misbehaviour that proves the counterparty chain violated the trust model.
// The light client must verify the misbehaviour using the trust model of the consensus mechanism
// and execute some custom logic such as freezing the client from accepting future updates and proof verification.
function submitMisbehaviour(
    clientId: bytes,
    misbehaviour: bytes,
): error
```

As relayers keep the client up-to-date and add `ConsensusState`s to the client, **IBC CORE** will use the exposed verification endpoints: `VerifyMembership` and `VerifyNonMembership` to verify incoming packet-flow messages coming from the counterparty.

```typescript
// verifies a membership of a path and value in the counterparty chain identified by the provided clientId
// against a particular ConsensusState identified by the provided height
function verifyMembership(
    clientId: bytes,
    height: Number,
    proof: bytes,
    path: CommitmentPath,
    value: bytes
): error

// verifies the nonmembership of a path in the counterparty chain identified by the provided clientId
// against a particular ConsensusState identified by the provided height
function verifyNonMembership(
    cliendId: bytes,
    height: Number,
    proof: bytes,
    path: CommitmentPath
): error
```

### Core IBC Functionality

IBC in its essence is the ability for applications on different blockchains with different consensus mechanisms to communicate with each other through client backed security. Thus, IBC needs the client described above and the IBC applications that define the packet data they wish to send and receive.

In addition to these layers, IBC v1 introduced the connection and channel abstractions to connect these two fundamental layers. IBC v2 intends to compress only the necessary aspects of connection and channel layers into a single packet handler with no handshakes but before doing this it is critical to understand what service they currently provide.

Properties of IBC v1 Connection:

- Verifies the validity of the counterparty client
- Establishes a unique identifier on each side for a shared abstract understanding (the connection)
- Establishes an agreement on the IBC version and supported features
- Allows multiple connections to be built against the same client pair
- Establishes the delay period so this security parameter can be instantiated differently for different connections against the same client pairing.
- Defines which channel orderings are supported

Properties of IBC v1 Channel:

- Separates applications into dedicated 1-1 communication channels. This prevents applications from writing into each other's channels.
- Allows applications to come to agreement on the application parameters (version negotiation). Ensures that each side can understand the other's communication and that they are running mutually compatible logic. This version negotiation is a multi-step process that allows the finalized version to differ substantially from the one initially proposed
- Establishes the ordering of the channel
- Establishes unique identifiers for the applications on either chain to use to reference each other when sending and receiving packets.
- The application protocol can be continually upgraded over time by using the upgrade handshake which allows the same channel which may have accumulated state to use new mutually agreed upon application packet data format(s) and associated new logic.
- Ensures exactly-once delivery of packet flow datagrams (Send, Receive, Acknowledge, Timeout)
- Ensures valid packet flow (Send => Receive => Acknowledge) XOR (Send => Timeout)

### Identifying Counterparties

In core IBC, the connection and channel handshakes serve to ensure the validity of counterparty clients, ensure the IBC and application versions are mutually compatible, as well as providing unique identifiers for each side to refer to the counterparty.

Since we are removing handshakes in IBC V2, we must have a different way to provide the chain with knowledge of the counterparty. With a client, we can prove any key/value path on the counterparty. However, without knowing which identifier the counterparty uses when it sends messages to us; we cannot differentiate between messages sent from the counterparty to our chain vs messages sent from the counterparty with other chains. Most implementations will not be able to store the ICS-24 paths directly as a key in the global namespace; but will instead write to a reserved, prefixed keyspace so as not to conflict with other application state writes. Thus the counterparty information we must have includes both its identifier for our chain as well as the key prefix under which it will write the provable ICS-24 paths.

Thus, IBC V2 will introduce a new message `RegisterCounterparty` that will associate the counterparty client of our chain with our client of the counterparty. Thus, if the `RegisterCounterparty` message is submitted to both sides correctly. Then both sides have mirrored <client,client> pairs that can be treated as identifiers for the sender and receiver chains the packet is associated with. Assuming they are correct, the client on each side is unique and provides an authenticated stream of packet data between the two chains. If the `RegisterCounterparty` message submits the wrong clientID, this can lead to invalid behaviour; but this is equivalent to a relayer submitting an invalid client in place of a correct client for the desired chain. In the simplest case, we can rely on out-of-band social consensus to only send on valid <client, client> pairs that represent a connection between the desired chains of the user; just as we rely on out-of-band social consensus that a given clientID and channel built on top of it is the valid, canonical identifier of our desired chain in IBC V1.

```typescript
interface Counterparty {
    clientId: bytes
    counterpartyPrefix: []bytes
}
```

This `Counterparty` will be keyed on the client identifier existing on our chain. Thus, both sides get access to each other's client identifier. This effectively creates a connection with unique identifiers on both sides that reference each other's consensus. Thus, the resulting `client, client` pairing in IBC V2 replaces the separate connection layer that existed in IBC V1.

The `RegisterCounterparty` method allows for authentication that implementations may verify before storing the provided counterparty identifier. The strongest authentication possible is to have a valid clientState and consensus state of our chain in the authentication along with a proof it was stored at the claimed counterparty identifier. This is equivalent to the `validateSelfClient` logic performed in the connection handshake.
A simpler but weaker authentication would simply be to check that the `RegisterCounterparty` message is sent by the same relayer that initialized the client. This would make the client parameters completely initialized by the relayer. Thus, users must verify that the client is pointing to the correct chain and that the counterparty identifier is correct as well before using identifiers to send a packet. In practice, this is verified by social consensus.

### IBC V2 Packet Processing

IBC V2 will simply provide packet delivery between two chains communicating and identifying each other by on-chain light clients as specified in [ICS-02](core/ics-002-client-semantics/README.md) with application packet data being routed to their specific IBC applications with packet-flow semantics as specified in [ICS-04](core/ics-004-packet-semantics/PACKET_HANDLER.md). The packet clientIDs as mentioned above will tell the IBC router which chain to send the packets to and which chain a received packet came from, while the portIDs in the payload specifies which application on the router the packet should be sent to.

Thus, once two chains have set up clients for each other with specific Identifiers, they can send IBC packets like so.

```typescript
interface Packet {
  sequence: uint64
  timeoutTimestamp: uint64
  sourceClientId: Identifier // identifier of the destination client on sender chain
  destClientId: Identifier // identifier of the sender client on the destination chain
  payload: []Payload
}
```

Since the packets are addressed **directly** with the underlying light clients, there are **no** more handshakes necessary. Instead the packet sender must be capable of providing the correct <client, client> pair.

Sending a packet with the wrong source client is equivalent to sending a packet with the wrong source channel. Sending a packet on a client with the wrong provided counterparty is an error and will cause the packet to be rejected. If the counterparty is set incorrectly for the new client, this is a misconfiguration in the IBC V2 setup process. Unexpected behavior may occur in this case, though it is expected that users validate the counterparty configurations on both sides are correct before sending packets using the client identifiers. This validation may be done directly or through social consensus.

If the client and counterparty identifiers are setup correctly, then the correctness and soundness properties of IBC holds. IBC packet flow is guaranteed to succeed. If the counterparty is misconfigured, then as we will see it will be impossible for the intended destination to correctly verify the packet thus, the packet will simply time out.

The Payload contains all the application specific information. This includes the opaque application data that the sender application wishes to send to the receiving application; it also includes the `Encoding` and `Version` that should be used to decode and process the application data. Note that this is a departure from IBC V1 where this metadata about how to process the application data was negotiated in the channel handshake. Here, each packet carries the information about how its individual data should be processed. This allows the `Version` and `Encoding` to change from packet to packet; allowing applications to upgrade asynchronously and optimistically send new packet encodings and versions to their counterparties. If the counterparty application can support receiving the new payload, it will successfully be processed; otherwise the receive will simply error and the sending application reverts state upon receiving the `ErrorAcknowledgement`. This increases the possibility for errors to occur during an application's packet processing but massively increases the flexibility of IBC applications to upgrade and evolve over time. Similarly, the portIDs on the sender and receiver application are no longer prenegotiated in the channel handshake and instead are in the payload. Thus in IBC v2; a sending application can route its packet to ANY OTHER application on the receiving application by simply specifying its portID in the payload as a receiver. It is incumbent on applications to restrict which counterparty applications it wishes to communicate with by validating the source and destination portIDs provided in the payload. Thus, the per-packet `Payload` replaces the separate channel layer that existed in IBC V1.

For more details on the Payload structure, see [ICS-04](core/ics-004-packet-semantics/PACKET.md).

### Registering IBC applications on the router

**IBC CORE** contains routers mapping reserved application portIDs to individual IBC applications as well as a mapping from clientIDs to individual IBC clients.

```typescript
type IBCRouter struct {
    apps: portID -> IBCApp
    clients: clientId -> IBCClient
}
```

### Packet Flow

For a detailed specification of the packet flow, please refer to [ICS-04](core/ics-004-packet-semantics/PACKET_HANDLER.md).

The packet-flow messages defined by IBC are: `SendPacket`, `ReceivePacket`, `AcknowledgePacket` and `TimeoutPacket`. `SendPacket` will most often be triggered by user-action that wants to initiate a cross-chain action (e.g. token transfer) by sending a packet from an application on the sender chain to an application on the destination chain. Every other message is the result of counterparty action, thus they must be submitted by an off-chain relayer that can submit a proof of counterparty that authenticates the message is valid. For example, the `RecvPacket` message can only be submitted if the relayer can prove that the counterparty did send a packet to our chain by submitting a proof to our on-chain client. The source chain commits the packet under the ICS-04 standardized commitment path which is constructed with `packet.sourceClientId` and `packet.sequence`. Since the `packet.sourceClientId` is a unique reference to the destination chain on the source chain, a packet commitment stored on this path is guaranteed to be a packet the source chain intends to send to the destination chain. The destination chain can verify this path using its on-chain client identified by `packet.destClientId`

Similarly, `AcknowledgePacket` and `TimeoutPacket` are messages that get sent back to the sending chain after the an attempted packet receipt. If the packet receipt is successful, an application-specific acknowledgement will be written to the ICS-04 standardized acknowledgement path under the `packet.destClientId` and `packet.sequence`. The sending chain can verify that the relayer-provided acknowledgment was committed to by the receiving chain by verifying this path using the on-chain client identified by `packet.sourceClientId`. Since the `packet.destClientId` is a unique reference to the sending chain on the destination chain and the `sequence` is unique in the stream of packets from source chain to destination; we can be guaranteed that the acknowledgement was written for the packet we previously sent with the provided `sourceClientId` and `sequence`. This acknowledgement is then given to the sending application to perform appropriate application logic for the given acknowledgement.

The `TimeoutPacket` is called if the packet receipt is unsuccessful. All compliant implementations must write a sentinel non-empty value into the standardized ICS-04 receipt path if it successfully receives a packet. This receipt path is constructed using the `packet.destClientId` and `packet.sequence`. Thus, if the value does not exist after the packet timeout has been passed, we can be guaranteed that the packet has timed out. The sending chain verifies a relayer-provided `NonMembership` proof for the receipt path of the given packet, if it succeeds then the timeout is verified and the timeout logic for the sending application is executed. Note the nonmembership proof MUST be verified against a consensus state that is executed past the timeout timestamp of the packet, and packet receiving MUST fail on the destination after the timeout has elapsed. This ensures that a packet cannot be timed out on the source chain and received on the destination simultaneously.

Thus, the packet handler implements the handlers for these messages by constructing the necessary path and value to authenticate the message as specified in [ICS-04](core/ics-004-packet-semantics/PACKET.md); it then routes the verification of the membership/nonmembership proof to the relevant [ICS-02](core/ics-002-client-semantics/README.md) client as specified in the packet. If the IBC TAO checks succeed and the client verification succeeds; then the packet message is authenticated and the application data in the payload can be processed by the application as trusted data. The packet sequence ensures that the stream of packets from a source chain to destination chain are all uniquely identified and prevents replay attacks. More detailed specification of the IBC TAO checks and packet handler behaviour can be found in [ICS-04](core/ics-004-packet-semantics/PACKET_HANDLER.md).

### Correctness

Claim: If the clients are setup correctly, then a chain can always verify packet flow messages sent by a valid counterparty.

If the clients are correct, then they can verify any key/value membership proof as well as a key non-membership proof.

All packet flow message (SendPacket, RecvPacket, and TimeoutPacket) are sent with the full packet. The packet contains both sender and receiver identifiers. Thus on packet flow messages sent to the receiver (RecvPacket), we use the receiver identifier in the packet to retrieve our local client and the source identifier to determine which path the sender stored the packet under. We can thus use our retrieved client to verify a key/value membership proof to validate that the packet was sent by the counterparty.

Similarly, for packet flow messages sent to the sender (AcknowledgePacket, TimeoutPacket); the packet is provided again. This time, we use the sender identifier to retrieve the local client and the destination identifier to determine the key path that the receiver must have written to when it received the packet. We can thus use our retrieved client to verify a key/value membership proof to validate that the packet was sent by the counterparty. In the case of timeout, if the packet receipt wasn't written to the receipt path determined by the destination identifier this can be verified by our retrieved client using the key nonmembership proof.

### Soundness

Claim: If the clients are setup correctly, then a chain cannot mistake a packet flow message intended for a different chain as a valid message from a valid counterparty.

We must note that client identifiers are unique to each chain but are not globally unique. Let us first consider a user that correctly specifies the source and destination identifiers in the packet.

We wish to ensure that well-formed packets (i.e. packets with correctly setup client ids) cannot have packet flow messages succeed on third-party chains. Ill-formed packets (i.e. packets with invalid client ids) may in some cases complete in invalid states; however we must ensure that any completed state from these packets cannot mix with the state of other valid packets.

We are guaranteed that the source identifier is unique on the source chain, the destination identifier is unique on the destination chain. Additionally, the destination identifier points to a valid client of the source chain, and the source identifier points to a valid client of the destination chain.

Suppose the RecvPacket is sent to a chain other than the one identified by the sourceClient on the source chain.

In the packet flow messages sent to the receiver (RecvPacket), the packet send is verified using the client on the destination chain (retrieved using destination identifier) with the packet commitment path derived by the source identifier. This verification check can only pass if the chain identified by the destination client committed the packet we received under the source client identifier. This is only possible if the destination client is pointing to the original source chain, or if it is pointing to a different chain that committed the exact same packet. Pointing to the original source chain would mean we sent the packet to the correct . Since the sender only sends packets intended for the destination chain by setting to a unique source identifier, we can be sure the packet was indeed intended for us. Since our client on the receiver is also correctly pointing to the sender chain, we are verifying the proof against a specific consensus algorithm that we assume to be honest. If the packet is committed to the wrong key path, then we will not accept the packet. Similarly, if the packet is committed by the wrong chain then we will not be able to verify correctly.
