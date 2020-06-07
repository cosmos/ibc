The abstraction of an IBC *connection* encapsulates two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together facilitate cross-chain sub-state verification and packet association (through channels). Connections are safely established in an unknown, dynamic topology using a handshake subprotocol. 

### Motivation

The IBC protocol provides *authorisation* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction in conjunction with the *client* abstraction  defines the *authorisation* semantics of IBC. Ordering semantics are determined by channels.

### Definitions

The opening handshake protocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

A *connection end* is state tracked for an end of a connection on one chain, with the following fields:

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
}
```

```typescript
interface ConnectionEnd {
  state: ConnectionState
  counterpartyConnectionIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string | []string
}
```

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

A correct protocol execution, between two chains `A` and `B`, with connection states formatted as `(A, B)`, flows as follows:

| Chain | Datagram          | Prior state       | Posterior state   |
| ----- | ----------------- | ----------------- | ----------------- |
| A     | `ConnOpenInit`    | `(-, -)`          | `(INIT, -)`       |
| B     | `ConnOpenTry`     | `(INIT, none)`    | `(INIT, TRYOPEN)` |
| A     | `ConnOpenAck`     | `(INIT, TRYOPEN)` | `(OPEN, TRYOPEN)` |
| B     | `ConnOpenConfirm` | `(OPEN, TRYOPEN)` | `(OPEN, OPEN)`    |

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
