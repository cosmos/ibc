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

Channel structures are stored under a store path prefix unique to a combination of a port identifier and channel identifier:

```typescript
function routeChannelPath(route: Identifier , portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "routeChannelEnds/routes/{route}/ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

Constant-size commitments to packet data fields are stored under the packet sequence number:

```typescript
function routePacketCommitmentPath(route: Identifier, portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "routeCommitments/routes/{route}/ports/{portIdentifier}/channels/{channelIdentifier}/packets/" + sequence
}
```

Packet acknowledgement data are stored under the `routePacketAcknowledgementPath`:

```typescript
function routePacketAcknowledgementPath(route: Identifier, portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "routeAcks/routes/{route}/ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/" + sequence
}
```

Packet timeout data are stored under the `routePacketTimeoutPath`:

```typescript
function routePacketTimeoutPath(route: Identifier, portIdentifier: Identifier, channelIdentifier: Identifier, sequence: uint64): Path {
    return "routeTimeouts/routes/{route}/ports/{portIdentifier}/channels/{channelIdentifier}/timeouts/" + sequence
}
```

### Data Structures


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
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    initChannel: ChannelEnd,
    proofInit: CommitmentProof,
    proofHeight: Height
) {

    //verify that the route determined by srcConnectionHops and destConnectionHops is one of the tryChannel
    //after verifying tryChannel at the previous hop, this means that this route is legit
    route = join(append(srcConnectionHops, destConnectionHops...), "/")
    abortTransactionUnless(route in initChannel.connectionHops)

    abortTransactionUnless(initChannel.state == INIT)
    abortTransactionUnless(initChannel.counterpartyPortIdentifier == portIdentifier)
    abortTransactionUnless(initChannel.counterpartyChannelIdentifier == channelIdentifier)

    abortTransactionUnless(provableStore.get(routeChannelPath(append(srcConnectionHops), portIdentifier, channelIdentifier)) !== null)

    connection = getConnection(provingConnectionIdentifier)
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops)-1] == connection.counterpartyConnectionIdentifier)

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-2]
        prefixRoute = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(initChannel)
        verifyMembership(client, proofHeight, 0, 0, proofInit, routeChannelPath(prefixRoute, portIdentifier, channelIdentifier), value)
    } else {
        // prove that previous hop (original source) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofInit, portIdentifier, channelIdentifier, initChannel)
    }
    
    // verification passed, storing the channel under the route prefix
    provableStore.set(routeChannelPath(append(srcConnectionHops), portIdentifier, channelIdentifier), initChannel)    

}
```

```typescript
// srcConnectionHops is the list of identifiers routing back to the TRY chain
// destConnectionHops is the list of identifiers routing to the ACK chain
// NOTE: since srcConnectionHops is routing in the opposite direction, it will contain all the counterparty connection identifiers from the connection identifiers specified by the TRY chain up to this point.
// For example, if the route specified by the TRY chain is "connection-1/connection-3"
// Then `routeChanOpenAck` may be called on the router chain with srcConnectionHops: "connection-4", destConnectionHops: "connection-3"
// where connection-4 is the counterparty connectionID on the router chain to connection-1 on the TRY chain
// and connection-3 is the connection on the router chain to the next hop in the route which in this case is the ACK chain.
function routeChanOpenAck(
  srcConnectionHops: [Identifier],
  destConnectionHops: [Identifier],
  provingConnectionIdentifier: Identifier,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  tryChannel: ChannelEnd,
  proofTry: CommitmentProof,
  proofHeight: Height) {

    //verify that the route determined by srcConnectionHops and destConnectionHops is one of the tryChannel
    //after verifying tryChannel at the previous hop, this means that this route is legit
    route = join(append(srcConnectionHops, destConnectionHops...), "/")
    abortTransactionUnless(route in tryChannel.connectionHops)

    abortTransactionUnless(tryChannel.state == TRYOPEN)
    abortTransactionUnless(tryChannel.counterpartyPortIdentifier == portIdentifier)
    abortTransactionUnless(tryChannel.counterpartyChannelIdentifier == channelIdentifier)

    abortTransactionUnless(provableStore.get(routeChannelPath(append(srcConnectionHops), portIdentifier, channelIdentifier)) !== null)

    connection = getConnection(provingConnectionIdentifier)
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)
    
    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops1-1)] == connection.counterpartyConnectionIdentifier)

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-2]
        prefixRoute = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(tryChannel)
        verifyMembership(clientState, proofHeight, 0, 0, proofTry, routeChannelPath(prefixRoute, portIdentifier, channelIdentifier), value)
    } else {
        // prove that previous hop (original source) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofTry, portIdentifier, channelIdentifier, tryChannel)
    }

    // verification passed, storing the channel under the route prefix
    // note that tryChannel does not overwrites the possibly already stored initChannel, as the route's prefix
    // is different: it contains the connections ids from the destination to the source
    provableStore.set(routeChannelPath(append(srcConnectionHops), portIdentifier, channelIdentifier), tryChannel)    
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
    //verify that the route determined by srcConnectionHops and destConnectionHops is one of the tryChannel
    //after verifying tryChannel at the previous hop, this means that this route is legit
    route = join(append(srcConnectionHops, destConnectionHops...), "/")
    abortTransactionUnless(route in ackChannel.connectionHops)

    abortTransactionUnless(ackChannel.state == TRYOPEN)
    abortTransactionUnless(ackChannel.counterpartyPortIdentifier == portIdentifier)
    abortTransactionUnless(ackChannel.counterpartyChannelIdentifier == channelIdentifier)

    connection = getConnection(provingConnectionIdentifier)
    abortTransactionUnless(connection !== null)
    abortTransactionUnless(connection.state === OPEN)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops1)-1] == connection.counterpartyConnectionIdentifier)

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-2]
        prefixRoute = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        client = queryClient(connection.clientIdentifier)
        value = protobuf.marshal(ackChannel)
        verifyMembership(clientState, proofHeight, 0, 0, proofAck, routeChannelPath(prefixRoute, portIdentifier, channelIdentifier), value)
    } else {
        // prove that previous hop (original src) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofAck, portIdentifier, channelIdentifier, tryChannel)
    }

    // verification passed, storing the channel under the route prefix
    // note that ackChannel does overwrites the possibly already stored initChannel
    provableStore.set(routeChannelPath(append(srcConnectionHops), portIdentifier, channelIdentifier), ackChannel)    
}
```
```typescript
function routeChanCloseConfirm(
  srcConnectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proofClose: CommitmentProof,
  proofHeight: Height) {

    // retrieve channel state.
    channel = provableStore.get(append(srcConnectionHops, channelPath(portIdentifier, channelIdentifier)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops)-1] == connection.counterpartyConnectionIdentifier)

    closeChannel = ChannelEnd{CLOSED, channel.order, portIdentifier,
                              channelIdentifier, channel.connectionHops, channel.version}

    if srcConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcConnectionHops[0:len(srcConnectionHops)-2]
        prefixRoute = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        client = queryClient(connection.clientIdentifier)
        verifyMembership(clientState, proofHeight, 0, 0, proofClose, routeChannelPath(prefixRoute, portIdentifier, channelIdentifier), closeChannel)
    } else {
        // prove that previous hop (original src) stored channel under channel path
        verifyChannelState(connection, proofHeight, proofClose, portIdentifier, channelIdentifier, closeChannel)
    }

    channel.state = CLOSED
    provableStore.set(append(srcConnectionHops, channelPath(portIdentifier, channelIdentifier)), channel)

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
    srcConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier
) {

    // retrieve channel state.
    // this retrieves either the TryChannel or the AckChannel, depending which chain sends the packet
    // this depends on srcConncetionHops
    // we use packet.destPort, packet.destChannel because the channel state is stored under the counterparties
    // port and channel
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.destPort, packet.destChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops)-1] == connection.counterpartyConnectionIdentifier)

    if len(srcConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        routePrefix = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routePacketCommitmentPath(routePrefix, packet.sourcePort, packet.sourceChannel, packet.sequence),
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
    
    path = routePacketCommitmentPath(append(srcConnectionHops), packet.sourcePort, packet.sourceChannel, packet.sequence)
    provableStore.set(path, hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    routeSuffixes = []
    prefix = srcConnectionHops.join("/")
    for route in channel.connectionHops {
        if route.startsWith(prefix) {
            routeArray = route.split("/")
            indexStart = routeArray.indexOf(len(srcConnectionHops)-1) + 2
            routeSuffixes.push(routeArray[indexStart:len(routeArray)-1].join("/"))
        }
    }

    emitLogEntry("routePacket", {sequence: packet.sequence, data: packet.data, timeoutHeight: packet.timeoutHeight, timeoutTimestamp: packet.timeoutTimestamp, routes: routeSuffixes})
}
```

```typescript
function routeAcknowledgmentPacket(
    packet: OpaquePacket,
    acknowledgement: bytes,
    proof: CommitmentProof,
    proofHeight: Height,
    srcConnectionHops: [Identifier], 
    provingConnectionIdentifier: Identifier,
) {

    // retrieve channel state. This will retrieve either the TryChannel or the AckChannel, depending which chain sends the packet
    // this depends on srcConncetionHops
    // we use packet.sourcePort, packet.sourceChannel because the relevant channel state is stored under the counterparties
    // port and channel of the packet's sender
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.sourcePort, packet.sourceChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops-1)] == connection.counterpartyConnectionIdentifier)

    if len(srcConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        routePrefix = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routePacketAcknowledgementPath(routePrefix, packet.destPort, packet.destChannel, packet.sequence),
                                                hash(acknowledgement)))
    } else {
        abortTransactionUnless(connection.verifyPacketAcknowledgement(proofHeight,
                                                                      proof,
                                                                      packet.destPort,
                                                                      packet.destChannel,
                                                                      packet.sequence,
                                                                      acknowledgement))
    }

    path = routePacketAcknowledgementPath(append(srcConnectionHops), packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, hash(acknowledgement))

    routeSuffixes = []
    prefix = srcConnectionHops.join("/")
    for route in channel.connectionHops {
        if route.startsWith(prefix) {
            routeArray = route.split("/")
            indexStart = routeArray.indexOf(len(srcConnectionHops)-1) + 2
            routeSuffixes.push(routeArray[indexStart:len(routeArray)-1].join("/"))
        }
    }

    emitLogEntry("routeAcknowledgement", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, acknowledgement, routes: routeSuffixes})
}
```

```typescript
function routeTimeoutPacket(
    packet: OpaquePacket,
    proof: CommitmentProof,
    proofHeight: Height,
    nextSequenceRecv: uint64,
    srcConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
) {

    // retrieve channel state. This will retrieve either the TryChannel or the AckChannel, depending which chain sends the packet
    // this depends on srcConncetionHops
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.sourcePort, packet.sourceChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops-1)] == connection.counterpartyConnectionIdentifier)

    if len(srcConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        routePrefix = append(srcConnectionHops[0:len(srcConnectionHops)-2])
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routePacketRoutePath(routePrefix, packet.destPort, packet.destChannel, packet.sequence),
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

    path = routeTimeoutPath(append(srcConnectionHops), packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, 1)

    routeSuffixes = []
    routePrefix = srcConnectionHops.join("/")
    for route in channel.connectionHops {
        if route.startsWith(routePrefix) {
            routeArray = route.split("/")
            indexStart = routeArray.indexOf(len(srcConnectionHops)-1) + 2
            routeSuffixes.push(routeArray[indexStart:len(routeArray)-1].join("/"))
        }
    }

    emitLogEntry("routeTimeout", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data, route: routeSuffixes})
}
```

```typescript
function routeTimeoutOnClose(
    packet: OpaquePacket,
    proofClosed: CommitmentProof,
    proofHeight: Height,
    nextSequenceRecv: uint64,
    srcConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
) {

    // retrieve channel state. This will retrieve either the TryChannel or the AckChannel, depending which chain sends the packet
    // this depends on srcConncetionHops
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.sourcePort, packet.sourceChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)

    connection = getConnection(provingConnectionIdentifier)

    // verify that proving connection is counterparty of the last src connection hop
    abortTransactionUnless(srcConnectionHops[len(srcConnectionHops-1)] == connection.counterpartyConnectionIdentifier)

    if len(srcConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        prefix = srcConnectionHops[0:len(srcConnectionHops)-2]
        abortTransactionUnless(verifyMembership(clientState,
                                                proofHeight,
                                                0,
                                                0,
                                                proof,
                                                routeTimeoutPath(prefix, packet.destPort, packet.destChannel, packet.sequence),
                                                1))
    } else {
        counterpartyChannel = provableStore.get(append(srcConnectionHops, channelPath(packet.destPort, packet.destChannel)))
        // check that the opposing channel end has closed
        expected = ChannelEnd{CLOSED, counterpartyChannel.order, packet.destPort,
                              packet.destChannel, counterpartyChannel.connectionHops, counterpartyChannel.version}
        abortTransactionUnless(connection.verifyChannelState(
                               proofHeight,
                               proof,
                               packet.destPort,
                               packet.destChannel,
                               expected))

        if channel.order === ORDERED {
            // ordered channel: check that the recv sequence is as claimed
            abortTransactionUnless(connection.verifyNextSequenceRecv(proofHeight,
                                                                     proof,
                                                                     packet.destPort,
                                                                     packet.destChannel,
                                                                     nextSequenceRecv))
            // ordered channel: check that packet has not been received
            abortTransactionUnless(nextSequenceRecv <= packet.sequence)
        } else {
            // unordered channel: verify absence of receipt at packet index
            abortTransactionUnless(connection.verifyPacketReceiptAbsence(proofHeight,
                                                                         proof,
                                                                         packet.destPort,
                                                                         packet.destChannel,
                                                                         packet.sequence))
        }
    }

    path = routeTimeoutPath(append(srcConnectionHops), packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, 1)

    emitLogEntry("routeTimeout", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data})
}
```

