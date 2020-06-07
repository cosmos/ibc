The *connection* abstraction encapsulates two stateful objects (*connection ends*) on two separate chains, each associated with a light client of the other chain, which together facilitate cross-chain sub-state verification and packet relay (through channels). Connections are safely established in an unknown, dynamic topology using a handshake subprotocol. 

\vspace{3mm}

### Motivation

The IBC protocol provides *authorisation* and *ordering* semantics for packets: guarantees, respectively, that packets have been committed on the sending blockchain (and according state transitions executed, such as escrowing tokens), and that they have been committed exactly once in a particular order and can be delivered exactly once in that same order. The *connection* abstraction in conjunction with the *client* abstraction  defines the *authorisation* semantics of IBC. Ordering semantics are provided by channels.

\vspace{3mm}

### Definitions

&nbsp;

A *connection end* is state tracked for an end of a connection on one chain, defined as follows:

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
  version: string
}
```

- The `state` field describes the current state of the connection end.
- The `counterpartyConnectionIdentifier` field identifies the connection end on the counterparty chain associated with this connection.
- The `counterpartyPrefix` field contains the prefix used for state verification on the counterparty chain associated with this connection.
- The `clientIdentifier` field identifies the client associated with this connection.
- The `counterpartyClientIdentifier` field identifies the client on the counterparty chain associated with this connection.
- The `version` field is an opaque string which can be utilised to determine encodings or protocols for channels or packets utilising this connection.

\vspace{3mm}

#### Opening Handshake

&nbsp;

The opening handshake subprotocol allows each chain to verify the identifier used to reference the connection on the other chain, enabling modules on each chain to reason about the reference on the other chain.

The opening handshake consists of four datagrams: `ConnOpenInit`, `ConnOpenTry`, `ConnOpenAck`, and `ConnOpenConfirm`.

A correct protocol execution, between two chains `A` and `B`, with connection states formatted as `(A, B)`, flows as follows:

| Datagram          | Prior state       | Posterior state   |
| ----------------- | ----------------- | ----------------- |
| `ConnOpenInit`    | `(-, -)`          | `(INIT, -)`       |
| `ConnOpenTry`     | `(INIT, none)`    | `(INIT, TRYOPEN)` |
| `ConnOpenAck`     | `(INIT, TRYOPEN)` | `(OPEN, TRYOPEN)` |
| `ConnOpenConfirm` | `(OPEN, TRYOPEN)` | `(OPEN, OPEN)`    |

At the end of an opening handshake between two chains implementing the subprotocol, the following properties hold:

- Each chain has each other's correct consensus state as originally specified by the initiating actor.
- Each chain has knowledge of and has agreed to its identifier on the other chain.
- Each chain knows that the other chain has agreed to the same data.

Connection handshakes can safely be performed permissionlessly, modulo anti-spam measures (paying gas).

`ConnOpenInit`, executed on chain A, initialises a connection attempt on chain A, specifying a pair of identifiers
for the connection on both chains and a pair of identifiers for existing light clients (one for
each chain). Chain A stores a connection end object in its state.

`ConnOpenTry`, executed on chain B, relays notice of a connection attempt on chain A to chain B,
providing the pair of connection identifiers, the pair of client identifiers, and a desired version.
Chain B verifies that these identifiers are valid, checks that the version is compatible, verifies
a proof that chain A has stored these identifiers, and verifies a proof that the light client chain A
is using to validate chain B has the correct consensus state for chain B. Chain B stores a connection
end object in its state.

`ConnOpenAck`, executed on chain A, relays acceptance of a connection open attempt from chain B back to chain A,
providing the identifier which can now be used to look up the connection end object. Chain A verifies
that the version requested is compatible, verifies a proof that chain B has stored the same identifiers
chain A has stored, and verifies a proof that the light client chain B is using to validate chain A has the
correct consensus state for chain A.

`ConnOpenConfirm`, executed on chain B, confirms opening of a connection on chain A to chain B.
Chain B simply checks that chain A has executed `ConnOpenAck` and marked the connection as `OPEN`.
Chain B subsequently marks its end of the connection as `OPEN`. After execution of `ConnOpenConfirm`
the connection is open on both ends and can be used immediately.

\vspace{3mm}

#### Versioning

During the handshake process, two ends of a connection come to agreement on a version bytestring associated
with that connection. At the moment, the contents of this version bytestring are opaque to the IBC core protocol.
In the future, it might be used to indicate what kinds of channels can utilise the connection in question, or
what encoding formats channel-related datagrams will use. Host state machines may utilise the version data
to negotiate encodings, priorities, or connection-specific metadata related to custom logic on top of IBC.
Host state machines may also safely ignore the version data or specify an empty string.
