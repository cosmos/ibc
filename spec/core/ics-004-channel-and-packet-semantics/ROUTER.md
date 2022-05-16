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
    order: ChannelOrder,
    sourceConnectionHops: [Identifier],
    destConnectionHops: [Identifier],
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier,
    proofInit: CommitmentProof,
    proofHeight: Height
) {
    if len(sourceConnectionHops) == 0 {
        return
    }
    connection = getConnection(sourceConnectionHops[len(sourceConnectionHops)-1])
    if sourceConnectionHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by sourceConnectionHops[1:]
    } else {
        // prove that previous hop (original source) stored channel under channel path
    }
    connectionHops = append(sourceConnectionHops, destConnectionHops...)
    // store channel under channelPath prefixed by sourceConnectionHops

    // store source identifiers -> sourceConnectionHops
    store(counterpartyPortIdentifier, counterpartyChannelIdentifier, sourceConnectionHops)
}

// similar logic for other handshake methods
```



