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
    sourceConnectionHops: Identifier,
    destConnectionHops: Identifier,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier,
    proofInit: CommitmentProof,
    proofHeight: Height
) {
    srcHops = split(sourceConnectionHops, "/")
    if len(srcHops) == 0 {
        return
    }
    connection = getConnection(srcHops[len(srcHops)-1])
    if srcHops > 1 {
        // prove that previous hop stored channel under channel path and prefixed by srcHops[1:]
    } else {
        // prove that previous hop (original source) stored channel under channel path
    }
    connectionHops = append(srcHops, destConnectionHops...)
    // store channel under channelPath prefixed by srcHops

    // store source identifiers -> srcHops
    store(counterpartyPortIdentifier, counterpartyChannelIdentifier, srcHops)
}

// similar logic for other handshake methods
```



