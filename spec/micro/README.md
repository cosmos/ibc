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

In core IBC, the connection and channel handshakes serve to validate the clients are valid clients of the counterparty, ensure the IBC version and application versions are mutually compatible, as well as providing unique identifiers for each side to refer to the counterparty.

Since we are removing handshakes in IBC lite, we must have a different way to provide the chain with knowledge of the counterparty identifier. With a client, we can prove any key/value path on the counterparty. However, without knowing which identifier the counterparty uses when it sends messages to us; we cannot differentiate between messages sent from the counterparty to our chain vs messages sent from the counterparty with other chains.

Thus, IBC lite will introduce a new message `ProvideCounterparty` that will associate the counterparty client of our chain with our client of the counterparty. Thus, if the `ProvideCounterparty` message is submitted to both sides correctly. Then both sides have mirrored <client,client> pairs that can be treated as channel identifiers. Assuming they are correct, the client on each side is unique and provides an authenticated stream of packet data between the two chains. If the `ProvideCounterparty` message submits the wrong clientID, this can lead to invalid behaviour; but this is equivalent to a relayer submitting an invalid client in place of a correct client for the desired chain. In the simplest case, we can rely on out-of-band social consensus to only send on valid <client, client> pairs that represent a connection between the desired chains of the user; just as we currently rely on out-of-band social consensus that a given clientID and channel built on top of it is the valid, canonical identifier of our desired chain.

```typescript
function ProvideCounterparty(
    channelIdentifier: Identifier, // this will be our own client identifier representing our channel to desired chain
    counterpartyChannelIdentifier: Identifier, // this is the counterparty's identifier of our chain
    authentication: data, // implementation-specific authentication data
) {
    assert(verify(authentication))

    privateStore.set(counterpartyPath(channelIdentifier), counterpartyChannelIdentifier)
}

// getCounterparty retrieves the stored counterparty identifier
// given the channelIdentifier on our chain once it is provided
function getCounterparty(channelIdentifier: Identifier): Identifier {
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
    apps: portID -> [Version]
    callbacks: portID -> [Callback]
    clients: clientId -> Client
}
```

### Packet Flow through the Router

```typescript
function sendPacket(
    sourcePort: Identifier,
    sourceChannel: Identifier,
    destPort: Identifier,
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

    // IBC only commits sourcePort, sourceChannel, sequence in the commitment path
    // and packet data, and timeout information in the value
    // For IBC Lite, we can't automatically retrieve the destination port since we don't have an actual stored channel
    // in order to get around this, if sourcePort and destinationPort are the same we will leave the path as-is.
    // if the ports are different than we must append the port ids together
    // so the receiver can verify the requested destination port
    commitPort = sourcePort
    if sourcePort != destPort {
        commitPort = sourcePort + "/" + destPort
    }

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

    counterparty = getCounterparty(packet.destChannel)
    assert(packet.sourceChannel == counterparty)

    srcCommitPort = packet.sourcePort
    if packet.sourcePort != packet.destPort {
        srcCommitPort = packet.sourcePort + "/" + packet.destPort
    }

    packetPath = packetCommitmentPath(srcCommitPort, packet.sourceChannel, packet.sequence)
    // DISCUSSION NEEDED: Should we have an in-protocol notion of Prefixing the path
    // or should we make this a concern of the client's VerifyMembership
    // proofPath = applyPrefix(client.counterpartyChannelStoreIdentifier, packetPath)
    assert(client.verifyMembership(
        proofHeight,
        0, 0, // zeroed out delay period
        proof,
        packetPath,
        hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)
    ))

    assert(packet.timeoutHeight === 0 || getConsensusHeight() < packet.timeoutHeight)
    assert(packet.timeoutTimestamp === 0 || currentTimestamp() < packet.timeoutTimestamp)

    dstCommitPort = packet.destPort
    if packet.sourcePort != packet.destPort {
        dstCommitPort = packet.destPort + "/" + packet.sourcePort
    }
    
    // we must set the receipt so it can be verified on the other side
    // this receipt does not contain any data, since the packet has not yet been processed
    // it's the sentinel success receipt: []byte{0x01}
    packetReceipt = channelStore.get(packetReceiptPath(dstCommitPort, packet.destChannel, packet.sequence))
    assert(packetReceipt === null)
    channelStore.set(
        packetReceiptPath(destCommitPort, packet.destChannel, packet.sequence),
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
    
    channelStore.set(packetAcknowledgementPath(destCommitPort, packet.destChannel, packet.sequence), ack)
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

    counterparty = getCounterparty(packet.sourceChannel)
    assert(packet.destChannel == counterparty)

    srcCommitPort = packet.sourcePort
    if packet.sourcePort != packet.destPort {
        srcCommitPort = packet.sourcePort + "/" + packet.destPort
    }

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(srcCommitPort, packet.sourceChannel, packet.sequence))
           === hash(hash(packet.data), packet.timeoutHeight, packet.timeoutTimestamp))

    destCommitPort = packet.destPort
    if packet.sourcePort != packet.destPort {
        destCommitPort = packet.destPort + "/" + packet.sourcePort
    }

    ackPath = packetAcknowledgementPath(destCommitPort, packet.destChannel)
    assert(client.verifyMembership(
        proofHeight,
        0, 0,
        proof,
        ackPath,
        hash(acknowledgement)
    ))

    cbs = router.callbacks[packet.sourcePort]
    cbs.OnAcknowledgePacket(packet, acknowledgement, relayer)

    channelStore.delete(packetCommitmentPath(srcCommitPort, packet.sourceChannel, packet.sequence))
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

    counterparty = getCounterparty(packet.sourceChannel)
    assert(packet.destChannel == counterparty)

    srcCommitPort = packet.sourcePort
    if packet.sourcePort != packet.destPort {
        srcCommitPort = packet.sourcePort + "/" + packet.destPort
    }

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(srcCommitPort, packet.sourceChannel, packet.sequence))
           === hash(hash(packet.data), packet.timeoutHeight, packet.timeoutTimestamp))

    // get the timestamp from the final consensus state in the channel path
    var proofTimestamp
    proofTimestamp = client.getTimestampAtHeight(proofHeight)
    assert(err != nil)

    // check that timeout height or timeout timestamp has passed on the other end
    asert(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && proofTimestamp >= packet.timeoutTimestamp))

    destCommitPort = packet.destPort
    if packet.sourcePort != packet.destPort {
        destCommitPort = packet.destPort + "/" + packet.sourcePort
    }

    receiptPath = packetReceiptPath(destCommitPort, packet.destChannel, packet.sequence)
    assert(client.verifyNonMembership(
        proofHeight
        0, 0,
        proof,
        receiptPath
    ))

    cbs = router.callbacks[packet.sourcePort]
    cbs.OnTimeoutPacket(packet, relayer)

    channelStore.delete(packetCommitmentPath(srcCommitPort, packet.sourceChannel, packet.sequence))
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



------

IGNORE THE MULTICHANNEL WORK FOR NOW

### Sending packets on the MultiChannel


Sending packets in the multichannel requires you to construct a packet data that contains a map from the application reserved portID to the requested version and opaque packet data.

```typescript
type MultiPacketData struct {
    AppData: Map[string]{
        AppPacketData{
            Version: string,
            Data: []byte,
        }
    }
}
```

The router will check that the channelID exists and it has a `MULTI_PORT_ID`. It will send a verifyMembership of the packet to the underlying client. It will then iterate over the packet data's in the multiPacketData. It will check that each application supports the requested version, and then it will construct a packet that only contains the application packet data and send it to the application. It will collect acknowledgements from each and put it into a multiAcknowledgement.

```typescript
type MultiAcknowledgment struct {
    success: bool,
    AppAcknowledgement: Map[string]Acknowledgement,
}
```

This will in turn be unpacked and sent to each application as an individual acknowledgment. However, the total success value must be the same for all apps since the packet receiving logic is atomic.

### Router Methods

```typescript
function sendPacket(
  sourceChannel: Identifier,
  timeoutHeight: Height,
  timeoutTimestamp: uint64,
  packetData: MultiPacketData): uint64 {
    // get provable channel store
    channelStore = getChannelStore(localChannelStoreIdentifier)

    channel = get(channelPath(MULTI_IBC_PORT, sourceChannel))
    assert(channel !== null)
    assert(channel.state === OPEN)

    // get provable client store
    clientStore = getClientStore(localClientStoreIdentifier)
    // in this specification, the connection hops fields will house
    // the underlying client identifier
    client = get(clientPath(channel.connectionHops[0]))
    assert(client !== null)

    // disallow packets with a zero timeoutHeight and timeoutTimestamp
    assert(timeoutHeight !== 0 || timeoutTimestamp !== 0)

    // check that the timeout height hasn't already passed in the local client tracking the receiving chain
    latestClientHeight = client.latestClientHeight()
    assert(timeoutHeight === 0 || latestClientHeight < timeoutHeight)

    // increment the send sequence counter
    sequence = channelStore.get(nextSequenceSendPath(MULTI_IBC_PORT, sourceChannel))
    channelStore.set(nextSequenceSendPath(MULTI_IBC_PORT, sourceChannel), sequence+1)

    // store commitment to the packet data & packet timeout
    channelStore.set(
      packetCommitmentPath(MULTI_IBC_PORT, sourceChannel, sequence),
      hash(hash(data), timeoutHeight, timeoutTimestamp)
    )

    // log that a packet can be safely sent
    emitLogEntry("sendPacket", {
      sequence: sequence,
      data: data,
      timeoutHeight: timeoutHeight,
      timeoutTimestamp: timeoutTimestamp
    })

    mulitPacketData.AppData.forEach((port, appData) => {
        supportedVersions = app[port]
        // check if router supports the desired port and version
        if supportedVersions.contains(appData.Version) {
            // send each individual packet data to application
            // to do send packet logic. e.g. escrow tokens
            appData = appData.Data
            // abort transaction on the first failure in send packet
            // note this is an additional callback as the flow of execution
            // differs from traditional IBC
            assert(callbacks[port].onSendPacket(
                packet.sourcePort,
                packet.sourceChannel,
                sequence,
                appData,
                relayer))
        }
    })

    return sequence
}
```

```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {
    // get provable channel store
    channelStore = getChannelStore(localChannelStoreIdentifier)

    channel = get(channelPath(MULTI_IBC_PORT, sourceChannel))
    assert(channel !== null)
    assert(channel.state === OPEN)

    assert(packet.sourcePort === channel.counterpartyPortIdentifier)
    assert(packet.sourceChannel === channel.counterpartyChannelIdentifier)

    // get provable client store
    clientStore = getClientStore(localClientStoreIdentifier)
    // in this specification, the connection hops fields will house
    // the underlying client identifier
    client = get(clientPath(channel.connectionHops[0]))
    assert(client !== null)

    packetPath = packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence)
    proofPath = applyPrefix(client.counterpartyChannelStoreIdentifier, packetPath)
    assert(client.verifyPacketData(
        proofHeight,
        0, 0, // zeroed out delay period
        proof,
        proofPath,
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

    multiPacketData = unmarshal(packetData)
    multiAck = MultiAcknowledgement{true, make(map[string]Acknowledgement)}

    // NEEDS DISCUSSION: Should we break early on first failure?
    mulitPacketData.AppData.forEach((port, appData) => {
        supportedVersions = app[port]
        // check if router supports the desired port and version
        if supportedVersions.contains(appData.Version) {
            // create a new packet with just the application data
            // in the packet data for the desired application
            appPacket = Packet{
                sequence: packet.sequence,
                timeoutHeight: packet.timeoutHeight,
                timeoutTimestamp: packet.timeoutTimestamp,
                sourcePort: packet.sourcePort,
                sourceChannel: packet.sourceChannel,
                destPort: packet.destPort,
                destChannel: packet.destChannel,
                data: appData.Data,
            }
            // TODO: Support aysnc acknowledgements
            appAck = callbacks[port].onRecvPacket(packet, relayer)
            // success of multiack must be false if even a single app acknowledgement returns false (atomic multipacket behaviour)
            // and puts the custom acknowledgement in the app acknowledgement map under the port key
            multiAck = multiAck{
                success: multiAck.success && appAck.Success(),
                appAcknowledgement: multiAck.AppAcknowledgement.put(port, appAck.Acknowledgement())
            }
        } else {
            // requested port/version was not supported so we must error
            multiAck = multiAck{
                success: false,
                appAcknowledgement: multiAck
            }
        }
    })

    // write the acknowledgement
    channelStore.set(
      packetAcknowledgementPath(packet.destPort, packet.destChannel, packet.sequence),
      hash(acknowledgement)
    )

    // log that a packet has been acknowledged
    emitLogEntry("writeAcknowledgement", {
      sequence: packet.sequence,
      timeoutHeight: packet.timeoutHeight,
      port: packet.destPort,
      channel: packet.destChannel,
      timeoutTimestamp: packet.timeoutTimestamp,
      data: packet.data,
      acknowledgement
    })
}
```

```typescript
function acknowledgePacket(
  packet: OpaquePacket,
  acknowledgement: bytes,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {
    // get provable channel store
    channelStore = getChannelStore(localChannelStoreIdentifier)

    // check channel is open
    channel = get(channelPath(MULTI_IBC_PORT, packet.sourceChannel))
    assert(channel !== null)
    assert(channel.state === OPEN)

    // verify counterparty information
    assert(packet.destPort === channel.counterpartyPortIdentifier)
    assert(packet.destChannel === channel.counterpartyChannelIdentifier)

    client = provableStore.get(connectionPath(channel.connectionHops[0]))
    assert(client !== null)
    
    // verify we sent the packet and haven't cleared it out yet
    assert(channelStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    assert(connection.verifyPacketAcknowledgement(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence,
        acknowledgement
      ))

    // all assertions passed, we can alter state

    // delete our commitment so we can't "acknowledge" again
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    multiPacketData = unmarshal(packet.data)

    ackSuccess = multiAck.success
    // send each app acknowledgement to relevant port
    // and override the success value with the multiack success value
    multiAck.forEach((port, ack) => {
        supportedVersions = app[port]
        appData = multiPacketData.AppData[port]
        // check if router supports the desired port and version
        if supportedVersions.contains(appData.Version) {
            // create a new packet with just the application data
            // in the packet data for the desired application
            appPacket = Packet{
                sequence: packet.sequence,
                timeoutHeight: packet.timeoutHeight,
                timeoutTimestamp: packet.timeoutTimestamp,
                sourcePort: packet.sourcePort,
                sourceChannel: packet.sourceChannel,
                destPort: packet.destPort,
                destChannel: packet.destChannel,
                data: appData.Data,
            }
            // construct app acknowledgement with multi-app success value
            // and individual ack info
            // NOTE: application MUST support the standard acknowledgement
            // described in ICS-04
            var appAck AppAcknowledgement
            if ackSuccess {
                // the acknowledgement was a success,
                // put app info into result
                appAck = AppAcknowledgement{
                    result: ack
                }
            } else {
                // the acknowledgement was a failure,
                // put app info into error.
                // note it is possible that this application succeeded
                // and its custom app info included information
                // of a successfully executed callback
                // however, we will still put the info in error;
                // so that the application knows the receive failed
                // on the other side.
                // Thus the callback reversion logic must be implementable
                // given the success boolean as opposed to specific information in the acknowledgement
                appAck = AppAcknowledgement{
                    error: ack
                }
            }
            
            // abort on first error in callbacks
            // NEEDS DISCUSSION: Should we fail on first acknowledge error
            // or optimistically try them all and succeed anyway
            assert(callbacks[port].onAcknowledgePacket(appPacket, appAck, relayer))
        } else {
            // should never happen
            assert(false)
        }
    })
}
```

```typescript
function onTimeoutPacket(packet: Packet, relayer: string) {
    // get provable channel store
    channelStore = getChannelStore(localChannelStoreIdentifier)

    // check channel is open
    channel = get(channelPath(MULTI_IBC_PORT, packet.sourceChannel))
    assert(channel !== null)
    assert(channel.state === OPEN)

     // verify counterparty information
    assert(packet.destPort === channel.counterpartyPortIdentifier)
    assert(packet.destChannel === channel.counterpartyChannelIdentifier)

    client = provableStore.get(connectionPath(channel.connectionHops[0]))
    assert(client !== null)

    proofTimestamp, err = client.getTimestampAtHeight(connection, proofHeight)

    // check that timeout height or timeout timestamp has passed on the other end
    abortTransactionUnless(
      (packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
      (packet.timeoutTimestamp > 0 && proofTimestamp >= packet.timeoutTimestamp))

    // verify we actually sent this packet, check the store
    abortTransactionUnless(provableStore.get(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))
           === hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    // all assertions passed, we can alter state

    // delete our commitment so we can't "time out" again
    provableStore.delete(packetCommitmentPath(packet.sourcePort, packet.sourceChannel, packet.sequence))

    // unordered channel: verify absence of receipt at packet index
    abortTransactionUnless(connection.verifyPacketReceiptAbsence(
        proofHeight,
        proof,
        packet.destPort,
        packet.destChannel,
        packet.sequence
    ))

    mulitPacketData.AppData.forEach((port, appData) => {
        supportedVersions = app[port]
        // check if router supports the desired port and version
        if supportedVersions.contains(appData.Version) {
            // create a new packet with just the application data
            // in the packet data for the desired application
            appPacket = Packet{
                sequence: packet.sequence,
                timeoutHeight: packet.timeoutHeight,
                timeoutTimestamp: packet.timeoutTimestamp,
                sourcePort: packet.sourcePort,
                sourceChannel: packet.sourceChannel,
                destPort: packet.destPort,
                destChannel: packet.destChannel,
                data: appData.Data,
            }
            // NEEDS DISCUSSION: Should we fail on first timeout error
            // or optimistically try them all and succeed anyway
            assert(callbacks[port].onTimeoutPacket(packet, relayer))
        } else {
            // should never happen
            assert(false)
        }
    })

}
```