# Router Specification

### Synopsis

The following document specifies the interfaces and state machine logic that IBC implementations must implement if they wish to serve as a Router chain.

### Motivation

// TODO

### Desired Properties

// TODO

## Technical Specification

### Data Structures

### Store Paths

### Channel Handshake

```typescript
function routeChanOpenTry(
    sourceConnectionHops: [Identifier],
    destConnectionHops: [Identifier],
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier,
    initChannel: ChannelEnd,
    proofInit: CommitmentProof,
    proofHeight: Height
) {
    route = join(append(sourceConnectionHops, destConnectionHops...), "/")
    included = false
    for ch in initChannel.connectionHops {
        if route == ch {
            included = true
        }
    }
    abortTransactionUnless(included)
    abortTransactionUnless(len(sourceConnectionHops) != 0)

    connection = getConnection(sourceConnectionHops[len(sourceConnectionHops)-1])
    if sourceConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by sourceConnectionHops[0:len(sourceConnectionHops)-1]
        path = append(sourceConnectionHops[0:len(sourceConnectionHops)-1], channelPath(portIdentifier, channelIdentifier))
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(initChannel)
        verifyMembership(clientState, proofHeight, 0, 0, proofInit, path, value)
    } else {
        // prove that previous hop (original source) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofInit, counterpartyPortIdentifier, counterpartyChannelIdentifier, initChannel)
    }

    // store channel under channelPath prefixed by sourceConnectionHops
    path = append(sourceConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    store.set(path, initChannel)

    // store sourceConnectionHops -> source identifiers
    store(sourceConnectionHops, counterpartyPortIdentifier, counterpartyChannelIdentifier)
}

function routeChanOpenAck(
  sourceConnectionHops: [Identifier],
  destConnectionHops: [Identifier],
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  tryChannel: ChannelEnd,
  proofTry: CommitmentProof,
  proofHeight: Height) {
    route = join(append(sourceConnectionHops, destConnectionHops...), "/")
    included = false
    for ch in tryChannel.connectionHops {
        if route == ch {
            included = true
        }
    }
    abortTransactionUnless(included)
    abortTransactionUnless(len(sourceConnectionHops) != 0)

    connection = getConnection(sourceConnectionHops[len(sourceConnectionHops)-1])
    if sourceConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by sourceConnectionHops[0:len(sourceConnectionHops)-1]
        path = append(sourceConnectionHops[0:len(sourceConnectionHops)-1], channelPath(portIdentifier, channelIdentifier))
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(tryChannel)
        verifyMembership(clientState, proofHeight, 0, 0, proofTry, path, value)
    } else {
        // prove that previous hop (original source) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofTry, counterpartyPortIdentifier, counterpartyChannelIdentifier, tryChannel)
    }

    // store channel under channelPath prefixed by sourceConnectionHops
    path = append(sourceConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    store.set(path, tryChannel)

    // store sourceConnectionHops -> source identifiers
    store(sourceConnectionHops, counterpartyPortIdentifier, counterpartyChannelIdentifier)
}

// similar logic for other handshake methods
```

### Packet Handling

```typescript
// if path is prefixed by portId and channelId and it is stored by last
// connection in srcConnectionHops then store under same path prefixed by srcConnectionHops
function routePacket(
    packet: OpaquePacket,
    proof: CommitmentProof,
    proofHeight: Height,
    sourceConnectionHops: [Identifier], //ordered from source up to self. It starts from 0.
    destConnectionHops: [Identifier] //ordered from dest up to self. It starts from 0.
) {
    previousHopConnectionId = sourceConnectionHops[len(sourceConnectionHops)-1]
    connection = getConnection(previousHopConnectionId)
    if len(sourceConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        prefix = sourceConnectionHops[0:len(sourceConnectionHops)-2]
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routePacketPath(prefix, packet.sourcePort, packet.sourceChannel, packet.sequence),
                                                hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp)))
    } else {
        abortTransactionUnless(connection.verifyPacketData(proofHeight,
                                                           proof,
                                                           packet.sourcePort,
                                                           packet.sourceChannel,
                                                           packet.sequence,
                                                           concat(packet.data,
                                                                  packet.timeoutHeight,
                                                                  packet.timeoutTimestamp)))
    }

    path = routePacketPath(sourceConnectionHops, packet.sourcePort, packet.sourceChannel, packet.sequence)
    provableStore.set(path, hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    if len(destConnectionHops) > 1{
        emitLogEntry("routePacket", {sequence: packet.sequence, data: packet.data, timeoutHeight: packet.timeoutHeight, timeoutTimestamp: packet.timeoutTimestamp})
    } else{
        emitLogEntry("sendPacket", {sequence: packet.sequence, data: packet.data, timeoutHeight: packet.timeoutHeight, timeoutTimestamp: packet.timeoutTimestamp})
    }
}
```

```typescript
function routeAcknowledgmentPacket(
    packet: OpaquePacket,
    acknowledgement: bytes,
    proof: CommitmentProof,
    proofHeight: Height,
    sourceConnectionHops: [Identifier], //ordered from source up to self. It starts from 0.
    destConnectionHops: [Identifier] //ordered from dest up to self. It starts from 0.
) {
    previousHopConnectionId = sourceConnectionHops[len(sourceConnectionHops)-1]
    connection = getConnection(previousHopConnectionId)
    if len(sourceConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        prefix = sourceConnectionHops[0:len(sourceConnectionHops)-2]
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routeAckPath(prefix, packet.destPort, packet.destChannel, packet.sequence),
                                                hash(acknowledgement)))
    } else {
        abortTransactionUnless(connection.verifyPacketAcknowledgement(proofHeight,
                                                                      proof,
                                                                      packet.destPort,
                                                                      packet.destChannel,
                                                                      packet.sequence,
                                                                      acknowledgement))
    }

    path = routeAckPath(sourceConnectionHops, packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, hash(acknowledgement))

    if len(destConnectionHops) > 1{
        emitLogEntry("routeAcknowledgement", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, acknowledgement})
    } else{
        emitLogEntry("writeAcknowledgement", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, acknowledgement})
    }
}
```

```typescript
function routeTimeoutPacket(
    packet: OpaquePacket,
    acknowledgement: bytes,
    proof: CommitmentProof,
    proofHeight: Height,
    nextSequenceRecv: uint64,
    sourceConnectionHops: [Identifier], //ordered from source up to self. It starts from 0.
    destConnectionHops: [Identifier] //ordered from dest up to self. It starts from 0.
) {

    channel = provableStore.get(append(sourceConnectionHops ,channelPath(packet.sourcePort, packet.sourceChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state === OPEN)

    previousHopConnectionId = sourceConnectionHops[len(sourceConnectionHops)-1]
    connection = getConnection(previousHopConnectionId)
    if len(sourceConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        prefix = sourceConnectionHops[0:len(sourceConnectionHops)-2]
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routeTimeoutPath(prefix, packet.destPort, packet.destChannel, packet.sequence),
                                                1))
    } else {
        abortTransactionUnless((packet.timeoutHeight > 0 && proofHeight >= packet.timeoutHeight) ||
                               (packet.timeoutTimestamp > 0 && connection.getTimestampAtHeight(proofHeight) > packet.timeoutTimestamp))
        if channel.order === ORDERED {
            // ordered channel: check that packet has not been received
            abortTransactionUnless(nextSequenceRecv <= packet.sequence)
            // ordered channel: check that the recv sequence is as claimed
            abortTransactionUnless(connection.verifyNextSequenceRecv(proofHeight,
                                                                     proof,
                                                                     packet.destPort,
                                                                     packet.destChannel,
                                                                     nextSequenceRecv))
        } else {
            // unordered channel: verify absence of receipt at packet index
            abortTransactionUnless(connection.verifyPacketReceiptAbsence(proofHeight,
                                                                         proof,
                                                                         packet.destPort,
                                                                         packet.destChannel,
                                                                         packet.sequence))
        }
    }

    path = routeTimeoutPath(sourceConnectionHops, packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, 1)

    emitLogEntry("routeTimeout", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data})
}
```

