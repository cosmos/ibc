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
    // the provingConnectionIdentifier is the connection Identifier that exists on the executing chain
    // It must be used to prove any information coming from the sourceHops (stored in the RouteInfo path)
    // The provingConnectionIdentifier must be the counterparty identifier of the last hop on the sourceHops
    provingConnectionIdentifier: Identifier,
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
function routeChannelData(
    srcHops: [Identifier],
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    path: CommitmentPath
)

