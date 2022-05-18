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
function routeChannelData(
    sourceHops: [Identifier],
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    path: CommitmentPath
)

