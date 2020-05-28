The abstraction of an IBC *connection* encapsulates two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together facilitate cross-chain sub-state verification and packet association (through channels). Connections are safely established in an unknown, dynamic topology using a handshake subprotocol. 

### Motivation

The IBC protocol provides *authorisation* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction in conjunction with the *client* abstraction  defines the *authorisation* semantics of IBC. Ordering semantics are determined by channels.

### Definitions

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

A *connection end* is state tracked for an end of a connection on one chian, with the following fields:

- The `state` field describes the current state of the connection end.
- The `counterpartyConnectionIdentifier` field identifies the connection end on the counterparty chain associated with this connection.
- The `counterpartyPrefix` field contains the prefix used for state verification on the counterparty chain associated with this connection.
  Chains should expose an endpoint to allow relayers to query the connection prefix.
  If not specified, a default `counterpartyPrefix` of `"ibc"` should be used.
- The `clientIdentifier` field identifies the client associated with this connection.
- The `counterpartyClientIdentifier` field identifies the client on the counterparty chain associated with this connection.
- The `version` field is an opaque string which can be utilised to determine encodings or protocols for channels or packets utilising this connection.
  If not specified, a default `version` of `""` should be used.

#### Opening Handshake

The opening handshake sub-protocol serves to initialise consensus states for two chains on each other.

The opening handshake defines four datagrams: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, and *ConnOpenConfirm*.

A correct protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram          | Chain acted upon | Prior state (A, B) | Posterior state (A, B) |
| --------- | ----------------- | ---------------- | ------------------ | ---------------------- |
| Actor     | `ConnOpenInit`    | A                | (none, none)       | (INIT, none)           |
| Relayer   | `ConnOpenTry`     | B                | (INIT, none)       | (INIT, TRYOPEN)        |
| Relayer   | `ConnOpenAck`     | A                | (INIT, TRYOPEN)    | (OPEN, TRYOPEN)        |
| Relayer   | `ConnOpenConfirm` | B                | (OPEN, TRYOPEN)    | (OPEN, OPEN)           |

At the end of an opening handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain has each other's correct consensus state as originally specified by the initiating actor.
- Each chain has knowledge of and has agreed to its identifier on the other chain.

This sub-protocol need not be permissioned, modulo anti-spam measures.

*ConnOpenInit* initialises a connection attempt on chain A.

*ConnOpenTry* relays notice of a connection attempt on chain A to chain B (this code is executed on chain B).

*ConnOpenAck* relays acceptance of a connection open attempt from chain B back to chain A (this code is executed on chain A).

*ConnOpenConfirm* confirms opening of a connection on chain A to chain B, after which the connection is open on both chains (this code is executed on chain B).

#### Versioning

During the handshake process, two ends of a connection come to agreement on a version bytestring associated
with that connection. At the moment, the contents of this version bytestring are opaque to the IBC core protocol.
In the future, it might be used to indicate what kinds of channels can utilise the connection in question, or
what encoding formats channel-related datagrams will use. Host state machines may utilise the version data
to negotiate encodings, priorities, or connection-specific metadata related to custom logic on top of IBC.
Host state machines may also safely ignore the version data or specify an empty string.

\begin{figure*}[!h]

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
function connOpenInit(
  identifier: Identifier,
  desiredCounterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  clientIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier) {
    abortTransactionUnless(validateConnectionIdentifier(identifier))
    abortTransactionUnless(provableStore.get(connectionPath(identifier)) == null)
    state = INIT
    connection = ConnectionEnd{state, desiredCounterpartyConnectionIdentifier, counterpartyPrefix,
      clientIdentifier, counterpartyClientIdentifier, getCompatibleVersions()}
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
\end{lstlisting}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
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
    expected = ConnectionEnd{INIT, desiredIdentifier, getCommitmentPrefix(), counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersions}
    version = pickVersion(counterpartyVersions)
    connection = ConnectionEnd{TRYOPEN, counterpartyConnectionIdentifier, counterpartyPrefix,
                               clientIdentifier, counterpartyClientIdentifier, version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofInit, counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
    previous = provableStore.get(connectionPath(desiredIdentifier))
    abortTransactionUnless(
      (previous === null) ||
      (previous.state === INIT &&
        previous.counterpartyConnectionIdentifier === counterpartyConnectionIdentifier &&
        previous.counterpartyPrefix === counterpartyPrefix &&
        previous.clientIdentifier === clientIdentifier &&
        previous.counterpartyClientIdentifier === counterpartyClientIdentifier &&
        previous.version === version))
    identifier = desiredIdentifier
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
\end{lstlisting}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
function connOpenAck(
  identifier: Identifier,
  version: string,
  proofTry: CommitmentProof,    
  proofConsensus: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {    
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === INIT || connection.state === TRYOPEN)
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{TRYOPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier, connection.clientIdentifier,
                             version} 
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofTry, connection.counterpartyConnectionIdentifier, expected))
    abortTransactionUnless(connection.verifyClientConsensusState(
      proofHeight, proofConsensus, connection.counterpartyClientIdentifier, consensusHeight, expectedConsensusState))
    connection.state = OPEN
    abortTransactionUnless(getCompatibleVersions().indexOf(version) !== -1)
    connection.version = version
    provableStore.set(connectionPath(identifier), connection)
}
\end{lstlisting}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
function connOpenConfirm(
  identifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === TRYOPEN)
    expected = ConnectionEnd{OPEN, identifier, getCommitmentPrefix(), connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    abortTransactionUnless(connection.verifyConnectionState(proofHeight, proofAck, connection.counterpartyConnectionIdentifier, expected))
    connection.state = OPEN
    provableStore.set(connectionPath(identifier), connection)
}
\end{lstlisting}
\end{subfigure}

\caption{Connection handshake algorithm}

\end{figure*}
