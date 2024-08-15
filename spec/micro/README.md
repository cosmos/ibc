# Micro IBC Architecture

### Context

The implementation of the entire IBC protocol as it currently stands is a large undertaking. While there exists ready-made implementations like ibc-go this is only deployable on the Cosmos-SDK. Similarly, there exists ibc-rs which is a library for chains to integrate. However, this requires the chain to be implemented in Rust, there still exists some non-trivial work to integrate the ibc-rs library into the target state machine, and certain limitations either in the state machine or in ibc-rs may prevent using the library for the target chain.

Writing an implementation from scratch is a problem many ecosystems face as a major barrier for IBC adoption.

The goal of this document is to serve as a "micro-IBC" specification that will allow new ecosystems to implement a protocol that can communicate with fully implemented IBC chains using the same security assumptions. It will also explain the motivations of the original design choices of the IBC protocol and how the micro-ibc architecture rethinks these design choices while still retaining the desired properties of IBC.

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

The implementation of each of these endpoints will be specific to the particular consensus mechanism targetted. The choice of consensus algorithm itself is arbitrary, it may be a Proof-of-Stake algorithm like CometBFT, or a multisig of trusted authorities, or a rollup that relies on an additional underlying client in order to verify its consensus. However, a light client must have the ability to define finality for a given snapshot of the state machine, this may be either through single-slot finality or a finality gadget.

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

// TODO: Keep very limited buffer of consensus states
// Keep ability to migrate client (without necessarily consensus governance)

### Core IBC Functionality

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

### Identifying Counterparties

In core IBC, the connection and channel handshakes serve to ensure the validity of counterparty clients, ensure the IBC and application versions are mutually compatible, as well as providing unique identifiers for each side to refer to the counterparty.

Since we are removing handshakes in IBC lite, we must have a different way to provide the chain with knowledge of the counterparty. With a client, we can prove any key/value path on the counterparty. However, without knowing which identifier the counterparty uses when it sends messages to us; we cannot differentiate between messages sent from the counterparty to our chain vs messages sent from the counterparty with other chains. Most implementations will not be able to store the ICS-24 paths directly as a key in the global namespace; but will instead write to a reserved, prefixed keyspace so as not to conflict with other application state writes. Thus the counteparty information we must have includes both its identifier for our chain as well as the key prefix under which it will write the provable ICS-24 paths.

Thus, IBC lite will introduce a new message `ProvideCounterparty` that will associate the counterparty client of our chain with our client of the counterparty. Thus, if the `ProvideCounterparty` message is submitted to both sides correctly. Then both sides have mirrored <client,client> pairs that can be treated as channel identifiers. Assuming they are correct, the client on each side is unique and provides an authenticated stream of packet data between the two chains. If the `ProvideCounterparty` message submits the wrong clientID, this can lead to invalid behaviour; but this is equivalent to a relayer submitting an invalid client in place of a correct client for the desired chain. In the simplest case, we can rely on out-of-band social consensus to only send on valid <client, client> pairs that represent a connection between the desired chains of the user; just as we currently rely on out-of-band social consensus that a given clientID and channel built on top of it is the valid, canonical identifier of our desired chain.

```typescript
interface Counterparty {
    channelId: Identifier
    keyPrefix: CommitmentPrefix
}

function ProvideCounterparty(
    channelIdentifier: Identifier, // this will be our own client identifier representing our channel to desired chain
    counterpartyChannelIdentifier: Identifier, // this is the counterparty's identifier of our chain
    counterpartyKeyPrefix: CommitmentPrefix,
    authentication: data, // implementation-specific authentication data
) {
    assert(verify(authentication))

    counterparty = Counterparty{
        channelId: counterpartyChannelIdentifier,
        keyPrefix: counterpartyKeyPrefix
    }

    privateStore.set(counterpartyPath(channelIdentifier), counterparty)
}

// getCounterparty retrieves the stored counterparty identifier
// given the channelIdentifier on our chain once it is provided
function getCounterparty(channelIdentifier: Identifier): Counterparty {
    return privateStore.get(counterpartyPath(channelIdentifier))
}
```

The `ProvideCounterparty` method allows for authentication data that implementations may verify before storing the provided counterparty identifier. The strongest authentication possible is to have a valid clientState and consensus state of our chain in the authentication along with a proof it was stored at the claimed counterparty identifier. This is equivalent to the `validateSelfClient` logic performed in the connection handshake.
A simpler but weaker authentication would simply be to check that the `ProvideCounterparty` message is sent by the same relayer that initialized the client. This would make the client parameters completely initialized by the relayer. Thus, users must verify that the client is pointing to the correct chain and that the counterparty identifier is correct as well before using the lite channel identified by the provided client-client pair.

### IBC Lite

IBC lite will simply provide packet delivery between two chains communicating and identifying each other by on-chain light clients as specified in ICS-02 with application packet data being routed to their specific IBC applications with packet-flow semantics remaining as they were defined in ICS-04. The channelID derived from the clientIDs as mentioned above will tell the IBC router which chain to send the packets to and which chain a received packet came from, while the portID specifies which application on the router the packet should be sent to.

Thus, once two chains have set up clients for each other with specific Identifiers, they can send IBC packets like so.

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: Height
  timeoutTimestamp: uint64
  sourcePort: Identifier // identifier of the application on sender
  sourceChannel: Identifier // identifier of the client of destination on sender chain
  destPort: Identifier // identifier of the application on destination
  destChannel: Identifier // identifier of the client of sender on the destination chain
}
```

Since the packets are addressed **directly** with the underlying light clients, there are **no** more handshakes necessary. Instead the packet sender must be capable of providing the correct <client, client> pair.

Sending a packet with the wrong source client is equivalent to sending a packet with the wrong source channel. Sending a packet on a channel with the wrong provided counterparty is a new source of errors, however this is added to the burden of out-of-band social consensus.

If the client and counterparty identifiers are setup correctly, then the correctness and soundness properties of IBC holds. IBC packet flow is guaranteed to succeed. If a user sends a packet with the wrong destination channel, then as we will see it will be impossible for the intended destination to correctly verify the packet thus, the packet will simply time out.


### Registering IBC applications on the router

The IBC router contains a mapping from a reserved application port and the supported versions of that application as well as a mapping from channelIdentifiers to channels.

```typescript
type IBCRouter struct {
    versions: portID -> [Version]
    callbacks: portID -> [Callback]
    clients: clientId -> Client
    ports: portID -> counterpartyPortID
}
```

### Packet Flow through the Router

```typescript
function sendPacket(
    sourcePort: Identifier,
    sourceChannel: Identifier,
    timeoutHeight: Height,
    timeoutTimestamp: uint64,
    packetData: []byte
): uint64 {
    // in this specification, the source channel is the clientId
    client = router.clients[packet.sourceChannel]
    assert(client !== null)

    // disallow packets with a zero timeoutHeight and timeoutTimestamp
    assert(timeoutHeight !== 0 || timeoutTimestamp !== 0)

    // check that the timeout height hasn't already passed in the local client tracking the receiving chain
    latestClientHeight = client.latestClientHeight()
    assert(timeoutHeight === 0 || latestClientHeight < timeoutHeight)

    // if the sequence doesn't already exist, this call initializes the sequence to 0
    sequence = channelStore.get(nextSequenceSendPath(commitPort, sourceChannel))
    
    // store commitment to the packet data & packet timeout
    channelStore.set(
      packetCommitmentPath(commitPort, sourceChannel, sequence),
      hash(hash(data), timeoutHeight, timeoutTimestamp)
    )

    // increment the sequence. Thus there are monotonically increasing sequences for packet flow
    // from sourcePort, sourceChannel pair
    channelStore.set(nextSequenceSendPath(commitPort, sourceChannel), sequence+1)

    // log that a packet can be safely sent
    emitLogEntry("sendPacket", {
      sequence: sequence,
      data: data,
      timeoutHeight: timeoutHeight,
      timeoutTimestamp: timeoutTimestamp
    })

}

function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {
    // in this specification, the destination channel is the clientId
    client = router.clients[packet.destChannel]
    assert(client !== null)

    // assert source channel is destChannel's counterparty channel identifier
    counterparty = getCounterparty(packet.destChannel)
    assert(packet.sourceChannel == counterparty.channelId)

    // assert source port is destPort's counterparty port identifier
    assert(packet.sourcePort == ports[packet.destPort])

    packetPath = packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence)
    merklePath = applyPrefix(counterparty.keyPrefix, packetPath)
    // DISCUSSION NEEDED: Should we have an in-protocol notion of Prefixing the path
    // or should we make this a concern of the client's VerifyMembership
    // proofPath = applyPrefix(client.counterpartyChannelStoreIdentifier, packetPath)
    assert(client.verifyMembership(
        proofHeight,
        0, 0, // zeroed out delay period
        proof,
        merklePath,
        hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)
    ))

    assert(packet.timeoutHeight === 0 || getConsensusHeight() < packet.timeoutHeight)
    assert(packet.timeoutTimestamp === 0 || currentTimestamp() < packet.timeoutTimestamp)

  
    // we must set the receipt so it can be verified on the other side
    // this receipt does not contain any data, since the packet has not yet been processed
    // it's the sentinel success receipt: []byte{0x01}
    packetReceipt = channelStore.get(packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence))
    assert(packetReceipt === null)

    channelStore.set(
        packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence),
        SUCCESSFUL_RECEIPT
    )

    // log that a packet has been received
    emitLogEntry("recvPacket", {
      data: packet.data
      timeoutHeight: packet.timeoutHeight,
      timeoutTimestamp: packet.timeoutTimestamp,
      sequence: packet.sequence,
      sourcePort: packet.sourcePort,
      sourceChannel: packet.sourceChannel,
      destPort: packet.destPort,
      destChannel: packet.destChannel,
      order: channel.order,
      connection: channel.connectionHops[0]
    })

    cbs = router.callbacks[packet.destPort]
    // IMPORTANT: if the ack is error, then the callback reverts its internal state changes, but the entire tx continues
    ack = cbs.OnRecvPacket(packet, relayer)
    
    if ack != nil {
        channelStore.set(packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence), ack)
    }
}

function acknowledgePacket(
    packet: OpaquePacket,
    acknowledgement: bytes,
    proof: CommitmentProof,
    proofHeight: Height,
    relayer: string
) {
    // in this specification, the source channel is the clientId
    client = router.clients[packet.sourceChannel]
    assert(client !== null)

    // assert dest channel is sourceChannel's counterparty channel identifier
    counterparty = getCounterparty(packet.destChannel)
    assert(packet.sourceChannel == counterparty.channelId)
   
    // assert dest port is sourcePort's counterparty port identifier
    assert(packet.destPort == ports[packet.sourcePort])

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(hash(packet.data), packet.timeoutHeight, packet.timeoutTimestamp))

    ackPath = packetAcknowledgementPath(packet.destPort, packet.destChannel)
    merklePath = applyPrefix(counterparty.keyPrefix, ackPath)
    assert(client.verifyMembership(
        proofHeight,
        0, 0,
        proof,
        merklePath,
        hash(acknowledgement)
    ))

    cbs = router.callbacks[packet.sourcePort]
    cbs.OnAcknowledgePacket(packet, acknowledgement, relayer)

    channelStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
}

function timeoutPacket(
    packet: OpaquePacket,
    proof: CommitmentProof,
    proofHeight: Height,
    relayer: string
) {
    // in this specification, the source channel is the clientId
    client = router.clients[packet.sourceChannel]
    assert(client !== null)

    // assert dest channel is sourceChannel's counterparty channel identifier
    counterparty = getCounterparty(packet.destChannel)
    assert(packet.sourceChannel == counterparty.channelId)
   
    // assert dest port is sourcePort's counterparty port identifier
    assert(packet.destPort == ports[packet.sourcePort])

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(hash(packet.data), packet.timeoutHeight, packet.timeoutTimestamp))

    // get the timestamp from the final consensus state in the channel path
    var proofTimestamp
    proofTimestamp = client.getTimestampAtHeight(proofHeight)
    assert(err != nil)

    // check that timeout height or timeout timestamp has passed on the other end
    asert(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && proofTimestamp >= packet.timeoutTimestamp))

    receiptPath = packetReceiptPath(packet.destPort, packet.destChannel, packet.sequence)
    merklePath = applyPrefix(counterparty.keyPrefix, receiptPath)
    assert(client.verifyNonMembership(
        proofHeight
        0, 0,
        proof,
        merklePath
    ))

    cbs = router.callbacks[packet.sourcePort]
    cbs.OnTimeoutPacket(packet, relayer)

    channelStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
}
```

### Correctness

Claim: If the clients are setup correctly, then a chain can always verify packet flow messages sent by a valid counterparty.

If the clients are correct, then they can verify any key/value membership proof as well as a key non-membership proof.

All packet flow message (SendPacket, RecvPacket, and TimeoutPacket) are sent with the full packet. The packet contains both sender and receiver identifiers. Thus on packet flow messages sent to the receiver (RecvPacket), we use the receiver identifier in the packet to retrieve our local client and the source identifier to determine which path the sender stored the packet under. We can thus use our retrieved client to verify a key/value membership proof to validate that the packet was sent by the counterparty.

Similarly, for packet flow messages sent to the sender (AcknowledgePacket, TimeoutPacket); the packet is provided again. This time, we use the sender identifier to retrieve the local client and the destination identifier to determine the key path that the receiver must have written to when it received the packet. We can thus use our retrieved client to verify a key/value membership proof to validate that the packet was sent by the counterparty. In the case of timeout, if the packet receipt wasn't written to the receipt path determined by the destination identifier this can be verified by our retrieved client using the key nonmembership proof.

### Soundness

// To do after prototyping and going through some of these examples before writing it down

Claim: If the clients are setup correctly, then a chain cannot mistake a packet flow message intended for a different chain as a valid message from a valid counterparty.

We must note that client identifiers are unique to each chain but are not globally unique. Let us first consider a user that correctly specifies the source and destination identifiers in the packet. 

We wish to ensure that well-formed packets (i.e. packets with correctly setup client ids) cannot have packet flow messages succeed on third-party chains. Ill-formed packets (i.e. packets with invalid client ids) may in some cases complete in invalid states; however we must ensure that any completed state from these packets cannot mix with the state of other valid packets.

We are guaranteed that the source identifier is unique on the source chain, the destination identifier is unique on the destination chain. Additionally, the destination identifier points to a valid client of the source chain, and the source identifier points to a valid client of the destination chain.

Suppose the RecvPacket is sent to a chain other than the one identified by the sourceClient on the source chain. 

In the packet flow messages sent to the receiver (RecvPacket), the packet send is verified using the client on the destination chain (retrieved using destination identifier) with the packet commitment path derived by the source identifier. This verification check can only pass if the chain identified by the destination client committed the packet we received under the source channel identifier. This is only possible if the destination client is pointing to the original source chain, or if it is pointing to a different chain that committed the exact same packet. Pointing to the original source chain would mean we sent the packet to the correct . Since the sender only sends packets intended for the desination chain by setting to a unique source identifier, we can be sure the packet was indeed intended for us. Since our client on the reciver is also correctly pointing to the sender chain, we are verifying the proof against a specific consensus algorithm that we assume to be honest. If the packet is committed to the wrong key path, then we will not accept the packet. Similarly, if the packet is committed by the wrong chain then we will not be able to verify correctly.

