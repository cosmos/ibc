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

### Data Structures

In order to implement the Router specification for ICS-4, the chain must store the following `routeInfo` data under the `routeInfoPath`

```typescript
interface RouteInfo {
    // this is a list of the continued routes from the current chain to the destination chain
    // since there may be multiple routes for a given channel that includes the same chain,
    // this may be a list of Identifiers.
    // Each Identifier is a route (ie a joined list of connection identifiers by `/`)
    destHops: [Identifier],
}
```

### Channel Handshake

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

    // create unique path for the source route: provingConnectionIdentifier/sourceHops
    path = append(provingConnectionIdentifier, srcConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))

    // merge destHops into a route and
    // append the route to the routeInfo if it does not already exist.
    routeInfo = store.get(path)
    route = mergeHops(destHops)
    if routeInfo == nil {
        routeInfo = RouteInfo{
            DestHops: [route],
        }
    } else {
        routes = append(routeInfo.destHops, route)
        routeInfo.DestHops = routes
    }
    store.set(path, routeInfo)
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

    // create unique path for the source route: provingConnectionIdentifier/sourceHops
    path = append(provingConnectionIdentifier, srcConnectionHops, channelPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))

    // merge destHops into a route and
    // append the route to the routeInfo if it does not already exist.
    routeInfo = store.get(path)
    route = mergeHops(destHops)
    if routeInfo == nil {
        routeInfo = RouteInfo{
            DestHops: [route],
        }
    } else {
        routes = append(routeInfo.destHops, route)
        routeInfo.DestHops = routes
    }
    store.set(path, routeInfo)
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
    provingConnectionIdentifier: Identifier,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier
) {

    // retrieve channel state 
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.destPort, packet.destChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    // abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    routePrefix = srcConnectionHops.join("/")
    abortTransactionUnless(channel.connectionHops.some(route => route.startsWith(routePrefix)))

    connection = getConnection(provingConnectionIdentifier)

    if len(srcConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        prefix = srcConnectionHops[0:len(srcConnectionHops)-2]
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

    
    path = routePacketPath(srcConnectionHops, packet.sourcePort, packet.sourceChannel, packet.sequence)
    provableStore.set(path, hash(packet.data, packet.timeoutHeight, packet.timeoutTimestamp))

    routeSuffixes = []
    for route in channel.connectionHops {
        if route.startsWith(routePrefix) {
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
    secConnectionHops: [Identifier], 
    destConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier
) {

    // I think this is redundant, given that we check the route valid.
    abortTransactionUnless(len(srcConnectionHops) != 0)
    abortTransactionUnless(len(destConnectionHops) != 0)

    // retrieve channel state 
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.destPort, packet.destChannel)))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(channel.state !== CLOSED)
    // abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    routePrefix = srcConnectionHops.join("/")
    abortTransactionUnless(channel.connectionHops.some(route => route.startsWith(routePrefix)))

    connection = getConnection(provingConnectionIdentifier)

    if len(srcConnectionHops) > 1 {
        clientState = queryClient(connection.clientIdentifier)
        prefix = srcConnectionHops[0:len(srcConnectionHops)-2]
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

    path = routeAckPath(srcConnectionHops, packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, hash(acknowledgement))

    routeSuffixes = []
    for route in channel.connectionHops {
        if route.startsWith(routePrefix) {
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
    destConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier
) {

    // I think this is redundant, given that we check the route valid.
    abortTransactionUnless(len(srcConnectionHops) != 0)
    abortTransactionUnless(len(destConnectionHops) != 0)

    // retrieve channel state 
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.destPort, packet.destChannel)))
    // note: the channel may have been closed
    // abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    route = join(append(srcConnectionHops, destConnectionHops...), "/")
    abortTransactionUnless(route in channel.connectionHops)

    connection = getConnection(provingConnectionIdentifier)

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

    path = routeTimeoutPath(srcConnectionHops, packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, 1)

    routeSuffixes = []
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
    proof: CommitmentProof,
    proofHeight: Height,
    nextSequenceRecv: uint64,
    srcConnectionHops: [Identifier],
    destConnectionHops: [Identifier],
    provingConnectionIdentifier: Identifier,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier
) {

    // I think this is redundant, given that we check the route valid.
    abortTransactionUnless(len(srcConnectionHops) != 0)
    abortTransactionUnless(len(destConnectionHops) != 0)

    // retrieve channel state 
    channel = provableStore.get(append(srcConnectionHops, channelPath(packet.destPort, packet.destChannel)))
    // abortTransactionUnless(authenticateCapability(channelCapabilityPath(packet.sourcePort, packet.sourceChannel), capability))
    abortTransactionUnless(packet.destPort === channel.counterpartyPortIdentifier)
    abortTransactionUnless(packet.destChannel === channel.counterpartyChannelIdentifier)

    route = join(append(srcConnectionHops, destConnectionHops...), "/")
    abortTransactionUnless(route in channel.connectionHops)

    connection = getConnection(provingConnectionIdentifier)

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

    path = routeTimeoutPath(srcConnectionHops, packet.destPort, packet.destChannel, packet.sequence)
    provableStore.set(path, 1)

    emitLogEntry("routeTimeout", {sequence: packet.sequence, timeoutHeight: packet.timeoutHeight, port: packet.destPort,channel: packet.destChannel, timeoutTimestamp: packet.timeoutTimestamp, data: packet.data})
}
```

