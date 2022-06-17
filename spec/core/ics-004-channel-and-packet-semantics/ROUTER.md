# Router Specification

### Synopsis

The following document specifies the interfaces and state machine logic that IBC implementations must implement if they wish to serve as a Router chain.

### Motivation

// TODO

### Desired Properties

// TODO

## Technical Specification

### Store Paths

#### RouteInfoPath

The route info path includes the connectionID on the executing chain along with the path to the executing chain and the source port and channelID.

```typescript
function routeInfoPath(connectionId: Identifier, route: Identifier, portIdentifier: Identifer, channelIdentifier: Identifier) {
    return "routeInfo/connectionId/{connectionId}/route/{route}/portIdentifier/{portIdentifier}/channelIdentifier/{channelIdentifier}"
}
```

### Channel Handshake

```typescript
// srcConnectionHops is the list of identifiers routing back to the initializing chain
// destConnectionHops is the list of identifiers routing to the TRY chain
// NOTE: since srcConnectionHops is routing in the opposite direction, it will contain all the counterparty connection identifiers from the connection identifiers specified by the initializing chain up to this point.
// For example, if the route specified by the initializing chain is "connection-1/connection-3"
// Then `routeChanOpenTry` may be called on the router chain with srcConnectionHops: "connection-4", destConnectionHops: "connection-3"
// where connection-4 is the counterparty connectionID on the router chain to connection-1 on the initializing chain
// and connection-3 is the connection on the router chain to the next hop in the route which in this case is the TRY chain.
function routeChanOpenTry(
    srcConnectionHops: [Identifier],
    destConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier,
    initChannel: ChannelEnd,
    proofInit: CommitmentProof,
    proofHeight: Height
) {
    abortTransactionUnless(len(srcConnectionHops) != 0)
    abortTransactionUnless(len(destConnectionHops) != 0)
    route = join(append(srcConnectionHops, destConnectionHops...), "/")
    included = false
    for ch in initChannel.connectionHops {
        if route == ch {
            included = true
        }
    }
    abortTransactionUnless(included)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops-1)] == connection.counterpartyConnectionIdentifier)

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-1]
        path = append(srcConnectionHops[0:len(srcConnectionHops)-1], channelPath(portIdentifier, channelIdentifier))
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(initChannel)
        verifyMembership(client, proofHeight, 0, 0, proofInit, path, value)
    } else {
        // prove that previous hop (original source) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofInit, counterpartyPortIdentifier, counterpartyChannelIdentifier, initChannel)
    }

    // store channel under channelPath prefixed by srcConnectionHops
    path = append(srcConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    store.set(path, initChannel)
}

// srcConnectionHops is the connection Hops from the TRY chain to the executing router chain
// destConnectionHops is the connection hops from the executing router chain to the ACK chain.
function routeChanOpenAck(
  srcConnectionHops: [Identifier],
  destConnectionHops: [Identifier],
  provingConnectionIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  tryChannel: ChannelEnd,
  proofTry: CommitmentProof,
  proofHeight: Height) {
    abortTransactionUnless(len(srcConnectionHops) != 0)
    abortTransactionUnless(len(destConnectionHops) != 0)

    connection = getConnection(provingConnectionIdentifier)
    
    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops-1)] == connection.counterpartyConnectionIdentifier)

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-1]
        path = append(srcConnectionHops[0:len(srcConnectionHops)-1], channelPath(portIdentifier, channelIdentifier))
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(tryChannel)
        verifyMembership(clientState, proofHeight, 0, 0, proofTry, path, value)
    } else {
        // prove that previous hop (original source) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofTry, counterpartyPortIdentifier, counterpartyChannelIdentifier, tryChannel)
    }

    // store channel under channelPath prefixed by srcConnectionHops
    path = append(srcConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    store.set(path, tryChannel)
}

// routeChanOpenConfirm routes an ACK to the confirmation chain
// srcConnectionHops is the connectionHops from the ACK chain
// destConnectionHops is the connectionHops to the CONFIRM chain
function routeChanOpenConfirm(
    srcConnectionHops: [Identifier],
    destConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    ackChannel: ChannelEnd,
    proofAck: CommitmentProof,
    proofHeight: Height
) {
    abortTransactionUnless(len(srcConnectionHops) != 0)
    abortTransactionUnless(len(destConnectionHops) != 0)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops-1)] == connection.counterpartyConnectionIdentifier)

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-1]
        path = append(srcConnectionHops[0:len(srcConnectionHops)-1], channelPath(portIdentifier, channelIdentifier))
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(tryChannel)
        verifyMembership(clientState, proofHeight, 0, 0, proofTry, path, value)
    } else {
        // prove that previous hop (original src) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofTry, counterpartyPortIdentifier, counterpartyChannelIdentifier, tryChannel)
    }

     // store channel under channelPath prefixed by srcConnectionHops
    path = append(srcConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    store.set(path, ackChannel)
}
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

    emitLogEntry("routePacket", {sequence: packet.sequence, data: packet.data, timeoutHeight: packet.timeoutHeight, timeoutTimestamp: packet.timeoutTimestamp})
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

    emitLogEntry("routeAcknowledgement", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, acknowledgement})
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

    channel = provableStore.get(append(sourceConnectionHops, channelPath(packet.sourcePort, packet.sourceChannel)))
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

