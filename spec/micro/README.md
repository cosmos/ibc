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

```
// TODO: Keep very limited buffer of consensus states
// Keep ability to migrate client (without necessarily consensus governance)
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
    localClient: ClientState, // this is the client of the counterparty that exists on our own chain
    remoteClientStoreIdentifier: CommitmentPath, // this is the identifier of the 
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

// TODO: Make this a 3-step handshake. Each side needs to know the remoteChannelStoreIdentifier and verify the counterparty stored it correctly.

```typescript
type Counterparty struct {
    clientStoreIdentifier: Identifier,
    channelStoreIdentifier: Identifier,
    clientIdentifier: Identifer
}

function initializeMultiChannel(
    localClientIdentifier: Identifier,
    remoteClientIdentifier: Identifier,
    remoteClientStoreIdentifier: CommitmentPath,
    remoteChannelStoreIdentifier: CommitmentPath,
    remoteClient: ClientState,
    remoteConsensusState: ConsensusState,
    remoteConsensusHeight: Height,
    proofClient: CommitmentProof,
    proofConsensus: CommitmentProof,
    proofHeight: Height
) {
    localClient = getClient(localClientIdentifier)
    // first verify the counterparty client is a valid client of our chain
    assert(localClient.verifyCounterpartyClient(
        remoteClientIdentifier,
        remoteClientStoreIdentifier,
        remoteClient,
        remoteConsensusState,
        remoteConsensusHeight,
        proofConsensus,
        proofHeight,
    ))

    channelId = generateIdentifier()

    multiChannel = Channel{
        state: INIT,
        ordering: UNORDERED,
        counterpartyPortIdentifier: MULTI_IBC_PORT,
        counterpartyChannelIdentifier: "",
        connectionHops: [localClientIdentifier],
        version: MULTI_IBC,
        upgradeSequence: 0,
    }

    channelStore = getChannelStore(localChannelStoreIdentifier)
    set("channelEnds/ports/{MULTI_IBC_PORT}/channels/{channelId}", multiChannel)
    
    // TODO: Should we validate and prove this on the other side?
    privateStore.set(counterpartyPath(), Counterparty{
        clientStoreIdentifier: remoteClientStoreIdentifier,
        channelStoreIdentifier: remoteChannelStoreIdentifier,
        clientIdentifier: remoteClientIdentifier
    })
}

function openMultiChannel(
    localClientIdentifier: Identifier,
    remoteClientIdentifier: Identifier,
    remoteClientStoreIdentifier: Identifier,
    remoteChannelIdentifier: Identifier,
    remoteChannelStoreIdentifier: Identifier,
    remoteClient: ClientState,
    remoteConsensusState: ConsensusState,
    remoteConsensusHeight: Height,
    proofClient: CommitmentProof,
    proofConsensus: CommitmentProof,
    proofChannel: CommitmentProof,
    proofHeight: Height
) {
    localClient = getClient(localClientIdentifier)
    // first verify the counterparty client is a valid client of our chain
    assert(localClient.verifyCounterpartyClient(
        remoteClientIdentifier,
        remoteClientStoreIdentifier,
        remoteClient,
        remoteConsensusState,
        remoteConsensusHeight,
        proofConsensus,
        proofHeight,
    ))

    expectedCounterpartyChannel = Channel{
        state: OPEN,
        ordering: UNORDERED,
        counterpartyPortIdentifier: MULTI_IBC_PORT,
        counterpartyChannelIdentifier: "",
        connectionHops: [remoteClientIdentifier],
        version: MULTI_IBC,
        upgradeSequence: 0,
    }
    channelPath = append(remoteChannelStoreIdentifier, "/channelEnds/ports/{MULTI_IBC_PORT}/channels/{remoteChannelIdentifier}")
    assert(localClient.VerifyMembership(
        proofHeight,
        0,
        0,
        proofChannel,
        channelPath,
        proto.marshal(expectedCounterpartyChannel,)
    ))

    channelId = generateIdentifier()

    multiChannel = Channel{
        state: OPEN,
        ordering: UNORDERED,
        counterpartyPortIdentifier: MULTI_IBC_PORT,
        counterpartyChannelIdentifier: remoteChannelIdentifier,
        connectionHops: [localClientIdentifier],
        version: MULTI_IBC,
        upgradeSequence: 0,
    }

    channelStore = getChannelStore(localChannelStoreIdentifier)
    channelStore.set("channelEnds/ports/{MULTI_IBC_PORT}/channels/{channelId}", multiChannel)

    privateStore.set(counterpartyPath(), Counterparty{
        clientStoreIdentifier: remoteClientStoreIdentifier,
        channelStoreIdentifier: remoteChannelStoreIdentifier,
        clientIdentifier: remoteClientIdentifier
    })
}

function confirmMultiChannel(
    localChannelIdentifier: Identifier,
    remoteChannelidentifier: Identifier,
    proofChannel: CommitmentProof,
    proofHeight: Height
) {
    channel = getChannel(localChannelIdentifier)
    localClient = getClient(channel.connectionHops[0])

    counterparty = privateStore.get(counterpartyPath())

    expectedCounterpartyChannel = Channel{
        state: OPEN,
        ordering: UNORDERED,
        counterpartyPortIdentifier: MULTI_IBC_PORT,
        counterpartyChannelIdentifier: localChannelIdentifier,
        connectionHops: [counterparty.clientIdentifier],
        version: MULTI_IBC,
        upgradeSequence: 0,
    }

    // verify counterparty channel opened under claimed paths
    proofChannel = append(counterparty.remoteChannelStoreIdentifier, "/channelEnds/ports/{MULTI_IBC_PORT}/channels/{remoteChannelIdentifier}")
    assert(localClient.VerifyMembership(
        proofHeight,
        0,
        0,
        proofChannel,
        channelPath,
        proto.marshal(expectedCounterpartyChannel,)
    ))

    channel.counterpartyChannelIdentifier = remoteChannelIdentifier
    channelStore = getChannelStore(localChannelStoreIdentifier)
    channelStore.set("channelEnds/ports/{MULTI_IBC_PORT}/channels/{channelId}", channel)
}
```

### Registering IBC applications on the router

The IBC router contains a mapping from a reserved application port and the supported versions of that application as well as a mapping from channelIdentifiers to channels.

```typescript
type IBCRouter struct {
    apps: portID -> [Version]
    callbacks: portID -> [Callback]
    channels: channelId -> Channel
}
```

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