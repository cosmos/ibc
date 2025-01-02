enum ConnectionState {
  INIT = 'INIT',
  TRYOPEN = 'TRYOPEN',
  OPEN = 'OPEN'
}

## Connection handshake

### Initiating a handshake

\vspace{3mm}

```typescript
/**
 * Initializes a new connection between two chains.
 * @param identifier Connection identifier
 * @param desiredCounterpartyConnectionIdentifier Desired counterparty connection identifier
 * @param counterpartyPrefix Counterparty's prefix
 * @param clientIdentifier Client identifier
 * @param counterpartyClientIdentifier Counterparty client identifier
 */
function connOpenInit(
  identifier: Identifier,
  desiredCounterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  clientIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier) {
    abortTransactionUnless(validateConnectionIdentifier(identifier))
    abortTransactionUnless(provableStore.get(connectionPath(identifier)) == null)
    const state = ConnectionState.INIT
    const connection = ConnectionEnd{state, desiredCounterpartyConnectionIdentifier, counterpartyPrefix,
      clientIdentifier, counterpartyClientIdentifier, getCompatibleVersions()}
    provableStore.set(connectionPath(identifier), connection)
}
```

### Responding to a handshake initiation

\vspace{3mm}

```typescript
function connOpenTry(
  desiredIdentifier: Identifier,
  counterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  counterpartyClientIdentifier: Identifier,
  clientIdentifier: Identifier,
  counterpartyVersions: string[],
  proofInit: CommitmentProof,
  proofConsensus: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(validateConnectionIdentifier(desiredIdentifier))
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{ConnectionState.INIT, desiredIdentifier, getCommitmentPrefix(), counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersions}
    version = pickVersion(counterpartyVersions)
    connection = ConnectionEnd{ConnectionState.TRYOPEN, counterpartyConnectionIdentifier, counterpartyPrefix,
                               clientIdentifier, counterpartyClientIdentifier, version}
    abortTransactionUnless(
      connection.verifyConnectionState(proofHeight, proofInit, counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
    previous = provableStore.get(connectionPath(desiredIdentifier))
    abortTransactionUnless(
      (previous === null) ||
      (previous.state === ConnectionState.INIT &&
        previous.counterpartyConnectionIdentifier === counterpartyConnectionIdentifier &&
        previous.counterpartyPrefix === counterpartyPrefix &&
        previous.clientIdentifier === clientIdentifier &&
        previous.counterpartyClientIdentifier === counterpartyClientIdentifier &&
        previous.version === version))
    identifier = desiredIdentifier
    provableStore.set(connectionPath(identifier), connection)
}
```

### Acknowledging the response

\vspace{3mm}

```typescript
function connOpenAck(
  identifier: Identifier,
  version: string,
  proofTry: CommitmentProof,    
  proofConsensus: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {    
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === ConnectionState.INIT || connection.state === ConnectionState.TRYOPEN)
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{ConnectionState.TRYOPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier, connection.clientIdentifier,
                             version} 
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofTry,
      connection.counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, connection.counterpartyClientIdentifier,
      consensusHeight, expectedConsensusState))
    connection.state = ConnectionState.OPEN
    abortTransactionUnless(getCompatibleVersions().indexOf(version) !== -1)
    connection.version = version
    provableStore.set(connectionPath(identifier), connection)
}
```

### Finalising the connection

\vspace{3mm}

```typescript
function connOpenConfirm(
  identifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === ConnectionState.TRYOPEN)
    expected = ConnectionEnd{ConnectionState.OPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    abortTransactionUnless(connection.verifyConnectionState(
      proofHeight, proofAck, connection.counterpartyConnectionIdentifier, expected))
    connection.state = ConnectionState.OPEN
    provableStore.set(connectionPath(identifier), connection)
}
```
