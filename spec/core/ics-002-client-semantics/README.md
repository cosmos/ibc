---
ics: 2
title: Client Semantics
stage: draft
category: IBC/TAO
kind: interface
requires: 23, 24
required-by: 3
version compatibility: ibc-go v7.0.0
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>, Aditya Sripal <aditya@interchain.io>
created: 2019-02-25
modified: 2022-08-04
---

## Synopsis

This standard specifies the properties that consensus algorithms of state machines implementing the inter-blockchain
communication (IBC) protocol are required to satisfy. 
These properties are necessary for efficient and safe verification in the higher-level protocol abstractions. 
The algorithm utilised in IBC to verify the state updates of a remote state machine is referred to as a *validity predicate*. 
Pairing a validity predicate with a trusted state (i.e., a state that the verifier assumes to be correct), 
implements the functionality of a *light client* (often shortened to *client*) for a remote state machine on the host state machine.
In addition to state update verification, every light client is able to detect consensus misbehaviours through a *misbehaviour predicate*.

Beyond the properties described in this specification, IBC does not impose any requirements on
the internal operation of the state machines and their consensus algorithms. 
A state machine may consist of a single process signing operations with a private key (the so-called "solo machine"), a quorum of processes signing in unison,
many processes operating a Byzantine fault-tolerant consensus algorithm (e.g., Tendermint), or other configurations yet to be invented
— from the perspective of IBC, a state machine is defined entirely by its light client validation and misbehaviour detection logic.

This standard also specifies how the light client's functionality is registered and how its data is stored and updated by the IBC protocol. 
The stored client instances can be introspected by a third party actor,
such as a user inspecting the state of the state machine and deciding whether or not to send an IBC packet.

### Motivation

In the IBC protocol, an actor, which may be an end user, an off-chain process, or a module on a state machine,
needs to be able to verify updates to the state of another state machine (i.e., the *remote state machine*). 
This entails accepting *only* the state updates that were agreed upon by the remote state machine's consensus algorithm. 
A light client of the remote state machine is the algorithm that enables the actor to verify state updates of that state machine. 
Note that light clients will generally not include validation of the entire state transition logic
(as that would be equivalent to simply executing the other state machine), but may
elect to validate parts of state transitions in particular cases.
This standard formalises the light client model and requirements. 
As a result, the IBC protocol can easily be integrated with new state machines running new consensus algorithms,
as long as the necessary light client algorithms fulfilling the listed requirements are provided.

The IBC protocol can be used to interact with probabilistic-finality consensus algorithms.
In such cases, different validity predicates may be required by different applications. For probabilistic-finality consensus, a validity predicate is defined by a finality threshold (e.g., the threshold defines how many block needs to be on top of a block in order to consider it finalized).
As a result, clients could act as *thresholding views* of other clients:
One *write-only* client could be used to store state updates (without the ability to verify them), 
while many *read-only* clients with different finality thresholds (confirmation depths after which 
state updates are considered final) are used to verify state updates. 

The client protocol should also support third-party introduction.
For example, if `A`, `B`, and `C` are three state machines, with 
Alice a module on `A`, Bob a module on `B`, and Carol a module on `C`, such that
Alice knows both Bob and Carol, but Bob knows only Alice and not Carol, 
then Alice can utilise an existing channel to Bob to communicate the canonically-serialisable 
validity predicate for Carol. Bob can then use this validity predicate to open a connection and channel 
so that Bob and Carol can talk directly.
If necessary, Alice may also communicate to Carol the validity predicate for Bob, prior to Bob's
connection attempt, so that Carol knows to accept the incoming request.

Client interfaces should also be constructed so that custom validation logic can be provided safely
to define a custom client at runtime, as long as the underlying state machine can provide an
appropriate gas metering mechanism to charge for compute and storage. On a host state machine
which supports WASM execution, for example, the validity predicate and misbehaviour predicate
could be provided as executable WASM functions when the client instance is created.

### Definitions

- `get`, `set`, `Path`, and `Identifier` are as defined in [ICS 24](../ics-024-host-requirements).

- `Consensus` is a state update generating algorithm. It takes the previous state of a state machine together 
  with a set of messages (i.e., state machine transactions) and generates a valid state update of the state machine.
  Every state machine MUST have a `Consensus` that generates a unique, ordered list of state updates 
  starting from a genesis state. 
  
  This specification expects that the state updates generated by `Consensus` 
  satisfy the following properties:
  - Every state update MUST NOT have more than one direct successor in the list of state updates. 
    In other words, the state machine MUST guarantee *finality* and *safety*. 
  - Every state update MUST eventually have a successor in the list of state updates. 
    In other words, the state machine MUST guarantee *liveness*.
  - Every state update MUST be valid (i.e., valid state transitions).
    In other words, `Consensus` MUST be *honest*, 
    e.g., in the case `Consensus` is a Byzantine fault-tolerant consensus algorithm, 
    such as Tendermint, less than a third of block producers MAY be Byzantine.
  
  Unless the state machine satisfies all of the above properties, the IBC protocol
may not work as intended, e.g., users' assets might be stolen. Note that specific client 
types may require additional properties. 

- `Height` specifies the order of the state updates of a state machine, e.g., a sequence number. 
  This entails that each state update is mapped to a `Height`.

- `CommitmentRoot` is as defined in [ICS 23](../ics-023-vector-commitments). 
  It provides an efficient way for higher-level protocol abstractions to verify whether
  a particular state transition has occurred on the remote state machine, i.e.,
  it enables proofs of inclusion or non-inclusion of particular values at particular paths 
  in the state of the remote state machine at particular `Height`s.

- `ClientMessage` is an arbitrary message defined by the client type that relayers can submit in order to update the client.
  The ClientMessage may be intended as a regular update which may add new consensus state for proof verification, or it may contain
  misbehaviour which should freeze the client.

- `ValidityPredicate` is a function that validates a ClientMessage sent by a relayer in order to update the client. 
  Using the `ValidityPredicate` SHOULD be more computationally efficient than executing `Consensus`.

- `ConsensusState` is the *trusted view* of the state of a state machine at a particular `Height`.
  It MUST contain sufficient information to enable the `ValidityPredicate` to validate state updates, 
  which can then be used to generate new `ConsensusState`s. 
  It MUST be serialisable in a canonical fashion so that remote parties, such as remote state machines,
  can check whether a particular `ConsensusState` was stored by a particular state machine.
  It MUST be introspectable by the state machine whose view it represents, 
  i.e., a state machine can look up its own `ConsensusState`s at past `Height`s.

- `ClientState` is the state of a client. It MUST expose an interface to higher-level protocol abstractions, 
  e.g., functions to verify proofs of the existence of particular values at particular paths at particular `Height`s.

- `MisbehaviourPredicate` is a function that checks whether the rules of `Consensus` were broken, 
  in which case the client MUST be *frozen*, i.e., no subsequent `ConsensusState`s can be generated.

- `Misbehaviour` is the proof needed by the `MisbehaviourPredicate` to determine whether 
  a violation of the consensus protocol occurred. For example, in the case the state machine 
  is a blockchain, a `Misbehaviour` might consist of two signed block headers with 
  different `CommitmentRoot`s, but the same `Height`.

### Desired Properties

Light clients MUST provide state verification functions that provide a secure way 
to verify the state of the remote state machines using the existing `ConsensusState`s. 
These state verification functions enable higher-level protocol abstractions to 
verify sub-components of the state of the remote state machines.

`ValidityPredicate`s MUST reflect the behaviour of the remote state machine and its `Consensus`, i.e.,
`ValidityPredicate`s accept *only* state updates that contain state updates generated by 
the `Consensus` of the remote state machine.

In case of misbehavior, the behaviour of the `ValidityPredicate` might differ from the behaviour of 
the remote state machine and its `Consensus` (since clients do not execute the `Consensus` of the 
remote state machine). In this case, a `Misbehaviour` SHOULD be submitted to the host state machine, 
which would result in the client being frozen and higher-level intervention being necessary.

## Technical Specification

This specification outlines what each *client type* must define. A client type is a set of definitions
of the data structures, initialisation logic, validity predicate, and misbehaviour predicate required
to operate a light client. State machines implementing the IBC protocol can support any number of client
types, and each client type can be instantiated with different initial consensus states in order to track
different consensus instances. In order to establish a connection between two state machines (see [ICS 3](../ics-003-connection-semantics)),
the state machines must each support the client type corresponding to the other state machine's consensus algorithm.

Specific client types shall be defined in later versions of this specification and a canonical list shall exist in this repository.
State machines implementing the IBC protocol are expected to respect these client types, although they may elect to support only a subset.

### Data Structures

#### `Height`

`Height` is an opaque data structure defined by a client type.
It must form a partially ordered set & provide operations for comparison.

```typescript
type Height
```

```typescript
enum Ord {
  LT
  EQ
  GT
}

type compare = (h1: Height, h2: Height) => Ord
```

A height is either `LT` (less than), `EQ` (equal to), or `GT` (greater than) another height.

`>=`, `>`, `===`, `<`, `<=` are defined through the rest of this specification as aliases to `compare`.

There must also be a zero-element for a height type, referred to as `0`, which is less than all non-zero heights.

#### `ConsensusState`

`ConsensusState` is an opaque data structure defined by a client type, used by the validity predicate to
verify new commits & state roots. Likely the structure will contain the last commit produced by
the consensus process, including signatures and validator set metadata.

`ConsensusState` MUST be generated from an instance of `Consensus`, which assigns unique heights
for each `ConsensusState` (such that each height has exactly one associated consensus state).
Two `ConsensusState`s on the same chain SHOULD NOT have the same height if they do not have
equal commitment roots. Such an event is called an "equivocation" and MUST be classified
as misbehaviour. Should one occur, a proof should be generated and submitted so that the client can be frozen
and previous state roots invalidated as necessary.

The `ConsensusState` of a chain MUST have a canonical serialisation, so that other chains can check
that a stored consensus state is equal to another (see [ICS 24](../ics-024-host-requirements) for the keyspace table).

```typescript
type ConsensusState = bytes
```

The `ConsensusState` MUST be stored under a particular key, defined below, so that other chains can verify that a particular consensus state has been stored.

The `ConsensusState` MUST define a `getTimestamp()` method which returns the timestamp associated with that consensus state:

```typescript
type getTimestamp = ConsensusState => uint64
```

#### `ClientState`

`ClientState` is an opaque data structure defined by a client type.
It may keep arbitrary internal state to track verified roots and past misbehaviours.

Light clients are representation-opaque — different consensus algorithms can define different light client update algorithms —
but they must expose this common set of query functions to the IBC handler.

```typescript
type ClientState = bytes
```

Client types MUST define a method to initialise a client state with the provided client identifier, client state and consensus state, writing to internal state as appropriate.

```typescript
type initialise = (identifier: Identifier, clientState: ClientState, consensusState: ConsensusState) => Void
```

Client types MUST define a method to fetch the current height (height of the most recent validated state update).

```typescript
type latestClientHeight = (
  clientState: ClientState)
  => Height
```

Client types MUST define a method on the client state to fetch the timestamp at a given height

```typescript
type getTimestampAtHeight = (
  clientState: ClientState,
  height: Height
) => uint64
```

#### `ClientMessage`

A `ClientMessage` is an opaque data structure defined by a client type which provides information to update the client.
`ClientMessage`s can be submitted to an associated client to add new `ConsensusState`(s) and/or update the `ClientState`. They likely contain a height, a proof, a commitment root, and possibly updates to the validity predicate.

```typescript
type ClientMessage = bytes
```

### Store paths

Client state paths are stored under a unique client identifier.

```typescript
function clientStatePath(id: Identifier): Path {
  return "clients/{id}/clientState"
}
```

Consensus state paths are stored under a unique combination of client identifier and height:

```typescript
function consensusStatePath(id: Identifier, height: Height): Path {
  return "clients/{id}/consensusStates/{height}"
}
```

#### Validity predicate

A validity predicate is an opaque function defined by a client type to verify `ClientMessage`s depending on the current `ConsensusState`.
Using the validity predicate SHOULD be far more computationally efficient than replaying the full consensus algorithm
for the given parent `ClientMessage` and the list of network messages.

The validity predicate is defined as:

```typescript
type verifyClientMessage = (ClientMessage) => Void
```

`verifyClientMessage` MUST throw an exception if the provided ClientMessage was not valid.

#### Misbehaviour predicate

A misbehaviour predicate is an opaque function defined by a client type, used to check if a ClientMessage
constitutes a violation of the consensus protocol. For example, if the state machine is a blockchain, this might be two signed headers
with different state roots but the same height, a signed header containing invalid
state transitions, or other proof of malfeasance as defined by the consensus algorithm.

The misbehaviour predicate is defined as

```typescript
type checkForMisbehaviour = (ClientMessage) => bool
```

`checkForMisbehaviour` MUST throw an exception if the provided proof of misbehaviour was not valid.

#### Update state

Function `updateState` is an opaque function defined by a client type that will update the client given a verified `ClientMessage`. Note that this function is intended for **non-misbehaviour** `ClientMessage`s.

```typescript
type updateState = (ClientMessage) => Void
```

`verifyClientMessage` must be called before this function, and `checkForMisbehaviour` must return false before this function is called.

The client MUST also mutate internal state to store
now-finalised consensus roots and update any necessary signature authority tracking (e.g.
changes to the validator set) for future calls to the validity predicate.

Clients MAY have time-sensitive validity predicates, such that if no ClientMessage is provided for a period of time
(e.g. an unbonding period of three weeks) it will no longer be possible to update the client, i.e., the client is being frozen. 
In this case, a permissioned entity such as a chain governance system or trusted multi-signature MAY be allowed
to intervene to unfreeze a frozen client & provide a new correct ClientMessage.

#### Update state on misbehaviour

Function `updateStateOnMisbehaviour` is an opaque function defined by a client type that will update the client upon receiving a verified `ClientMessage` that is valid misbehaviour.

```typescript
type updateStateOnMisbehaviour = (ClientMessage) => Void
```

`verifyClientMessage` must be called before this function, and `checkForMisbehaviour` must return `true` before this function is called.

The client MUST also mutate internal state to mark appropriate heights which
were previously considered valid as invalid, according to the nature of the misbehaviour.

Once misbehaviour is detected, clients SHOULD be frozen so that no future updates can be submitted.
A permissioned entity such as a chain governance system or trusted multi-signature MAY be allowed
to intervene to unfreeze a frozen client & provide a new correct ClientMessage which updates the client to a valid state.

#### Retrieve Client Status 

Status is an opaque function defined by a client type to retrieve the current clientState. Status can be either Active, Expired, Unknown or Frozen. 

The function is defined as:

```typescript
type Status = (Client) => Void
```

#### `CommitmentProof`

`CommitmentProof` is an opaque data structure defined by a client type in accordance with [ICS 23](../ics-023-vector-commitments).
It is utilised to verify presence or absence of a particular key/value pair in state
at a particular finalised height (necessarily associated with a particular commitment root).

### State verification

Client types must define functions to authenticate internal state of the state machine which the client tracks.
Internal implementation details may differ (for example, a loopback client could simply read directly from the state and require no proofs).

- The `delayPeriodTime` is passed to the verification functions for packet-related proofs in order to allow packets to specify a period of time which must pass after a consensus state is added before it can be used for packet-related verification.
- The `delayPeriodBlocks` is passed to the verification functions for packet-related proofs in order to allow packets to specify a period of blocks which must pass after a consensus state is added before it can be used for packet-related verification.

`verifyMembership` is a generic proof verification method which verifies a proof of the existence of a value at a given `CommitmentPath` at the specified height. It MUST return an error if the verification is not successful. 
The caller is expected to construct the full `CommitmentPath` from a `CommitmentPrefix` and a standardized path (as defined in [ICS 24](../ics-024-host-requirements/README.md#path-space)). If the caller desires a particular delay period to be enforced,
then it can pass in a non-zero `delayPeriodTime` or `delayPeriodBlocks`. If a delay period is not necessary, the caller must pass in 0 for `delayPeriodTime` and `delayPeriodBlocks`,
and the client will not enforce any delay period for verification.

```typescript
type verifyMembership = (
  clientState: ClientState,
  height: Height,
  delayPeriodTime: uint64,
  delayPeriodBlocks: uint64,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: bytes)
  => Error
```

`verifyNonMembership` is a generic proof verification method which verifies a proof of absence of a given `CommitmentPath` at the specified height. It MUST return an error if the verification is not successful. 
The caller is expected to construct the full `CommitmentPath` from a `CommitmentPrefix` and a standardized path (as defined in [ICS 24](../ics-024-host-requirements/README.md#path-space)). If the caller desires a particular delay period to be enforced,
then it can pass in a non-zero `delayPeriodTime` or `delayPeriodBlocks`. If a delay period is not necessary, the caller must pass in 0 for `delayPeriodTime` and `delayPeriodBlocks`,
and the client will not enforce any delay period for verification.

Since the verification method is designed to give complete control to client implementations, clients can support chains that do not provide absence proofs by verifying the existence of a non-empty sentinel `ABSENCE` value. Thus in these special cases, the proof provided will be an ICS-23 Existence proof, and the client will verify that the `ABSENCE` value is stored under the given path for the given height.

```typescript
type verifyNonMembership = (
  clientState: ClientState,
  height: Height,
  delayPeriodTime: uint64,
  delayPeriodBlocks: uint64,
  proof: CommitmentProof,
  path: CommitmentPath)
  => Error
```

### Query interface

#### Chain queries

These query endpoints are assumed to be exposed over HTTP or an equivalent RPC API by nodes of the chain associated with a particular client.

`queryUpdate` MUST be defined by the chain which is validated by a particular client, and should allow for retrieval of clientMessage for a given height. This endpoint is assumed to be untrusted.

```typescript
type queryUpdate = (height: Height) => ClientMessage
```

`queryChainConsensusState` MAY be defined by the chain which is validated by a particular client, to allow for the retrieval of the current consensus state which can be used to construct a new client.
When used in this fashion, the returned `ConsensusState` MUST be manually confirmed by the querying entity, since it is subjective. This endpoint is assumed to be untrusted. The precise nature of the
`ConsensusState` may vary per client type.

```typescript
type queryChainConsensusState = (height: Height) => ConsensusState
```

Note that retrieval of past consensus states by height (as opposed to just the current consensus state) is convenient but not required.

`queryChainConsensusState` MAY also return other data necessary to create clients, such as the "unbonding period" for certain proof-of-stake security models. This data MUST also be verified by the querying entity.

#### On-chain state queries

This specification defines a single function to query the state of a client by-identifier.

```typescript
function queryClientState(identifier: Identifier): ClientState {
  return provableStore.get(clientStatePath(identifier))
}
```

The `ClientState` type SHOULD expose its latest verified height (from which the consensus state can then be retrieved using `queryConsensusState` if desired).

```typescript
type latestHeight = (state: ClientState) => Height
```

Client types SHOULD define the following standardised query functions in order to allow relayers & other off-chain entities to interface with on-chain state in a standard API.

`queryConsensusState` allows stored consensus states to be retrieved by height.

```typescript
type queryConsensusState = (
  identifier: Identifier,
  height: Height,
) => ConsensusState
```

#### Proof construction

Each client type SHOULD define functions to allow relayers to construct the proofs required by the client's state verification algorithms. These may take different forms depending on the client type.
For example, Tendermint client proofs may be returned along with key-value data from store queries, and solo client proofs may need to be constructed interactively on the solo state machine in question (since the user will need to sign the message).
These functions may constitute external queries over RPC to a full node as well as local computation or verification.

```typescript
type queryAndProveClientConsensusState = (
  clientIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix,
  consensusStateHeight: Height) => ConsensusState, Proof

type queryAndProveConnectionState = (
  connectionIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix) => ConnectionEnd, Proof

type queryAndProveChannelState = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix) => ChannelEnd, Proof

type queryAndProvePacketData = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix,
  sequence: uint64) => []byte, Proof

type queryAndProvePacketAcknowledgement = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix,
  sequence: uint64) => []byte, Proof

type queryAndProvePacketAcknowledgementAbsence = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix,
  sequence: uint64) => Proof

type queryAndProveNextSequenceRecv = (
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  height: Height,
  prefix: CommitmentPrefix) => uint64, Proof
```

#### Implementation strategies

##### Loopback

A loopback client of a local state machine merely reads from the local state, to which it must have access.

##### Simple signatures

A client of a solo state machine with a known public key checks signatures on messages sent by that local state machine,
which are provided as the `Proof` parameter. The `height` parameter can be used as a replay protection nonce.

Multi-signature or threshold signature schemes can also be used in such a fashion.

##### Proxy clients

Proxy clients verify another (proxy) state machine's verification of the target state machine, by including in the
proof first a proof of the client state on the proxy state machine, and then a secondary proof of the sub-state of
the target state machine with respect to the client state on the proxy state machine. This allows the proxy client to
avoid storing and tracking the consensus state of the target state machine itself, at the cost of adding
security assumptions of proxy state machine correctness.

##### Merklized state trees

For clients of state machines with Merklized state trees, these functions can be implemented by calling the [ICS-23](../ics-023-vector-commitments/README.md) `verifyMembership` or `verifyNonMembership` methods, using a verified Merkle
root stored in the `ClientState`, to verify presence or absence of particular key/value pairs in state at particular heights in accordance with [ICS 23](../ics-023-vector-commitments).

```typescript
type verifyMembership = (ClientState, Height, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, Height, CommitmentProof, Path) => boolean
```

### Sub-protocols

IBC handlers MUST implement the functions defined below.

#### Identifier validation

Clients are stored under a unique `Identifier` prefix.
This ICS does not require that client identifiers be generated in a particular manner, only that they be unique.
However, it is possible to restrict the space of `Identifier`s if required.
The validation function `validateClientIdentifier` MAY be provided.

```typescript
type validateClientIdentifier = (id: Identifier) => boolean
```

If not provided, the default `validateClientIdentifier` will always return `true`. 

##### Utilising past roots

To avoid race conditions between client updates (which change the state root) and proof-carrying
transactions in handshakes or packet receipt, many IBC handler functions allow the caller to specify
a particular past root to reference, which is looked up by height. IBC handler functions which do this
must ensure that they also perform any requisite checks on the height passed in by the caller to ensure
logical correctness.

#### Create

Calling `createClient` with the client state and initial consensus state creates a new client.

```typescript
function createClient(clientState: clientState, consensusState: ConsensusState) {
  // implementations may define a identifier generation function
  identifier = generateClientIdentifier()
  abortTransactionUnless(provableStore.get(clientStatePath(identifier)) === null)
  initialise(identifier, clientState, consensusState)
}
```

#### Query

Client consensus state and client internal state can be queried by identifier, but
the specific paths which must be queried are defined by each client type.

#### Update

Updating a client is done by submitting a new `ClientMessage`. The `Identifier` is used to point to the
stored `ClientState` that the logic will update. When a new `ClientMessage` is verified with
the stored `ClientState`'s validity predicate and `ConsensusState`, the client MUST
update its internal state accordingly, possibly finalising commitment roots and
updating the signature authority logic in the stored consensus state.

If a client can no longer be updated (if, for example, the trusting period has passed),
it will no longer be possible to send any packets over connections & channels associated
with that client, or timeout any packets in-flight (since the height & timestamp on the
destination chain can no longer be verified). Manual intervention must take place to
reset the client state or migrate the connections & channels to another client. This
cannot safely be done completely automatically, but chains implementing IBC could elect
to allow governance mechanisms to perform these actions
(perhaps even per-client/connection/channel in a multi-sig or contract).

```typescript
function updateClient(
  id: Identifier,
  clientMessage: ClientMessage) {
    // get clientState from store with id
    clientState = provableStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)

    verifyClientMessage(clientMessage)
    
    foundMisbehaviour := clientState.CheckForMisbehaviour(clientMessage)
    if foundMisbehaviour {
      updateStateOnMisbehaviour(clientMessage)
      // emit misbehaviour event
    }
    else {    
      updateState(clientMessage) // expects no-op on duplicate clientMessage
      // emit update event
    }
}
```

#### Misbehaviour

A relayer may alert the client to the misbehaviour directly, possibly invalidating
previously valid state roots & preventing future updates.

```typescript
function submitMisbehaviourToClient(
  id: Identifier,
  clientMessage: ClientMessage) {
    clientState = provableStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    // authenticate client message
    verifyClientMessage(clientMessage)
    // check that client message is valid instance of misbehaviour
    abortTransactionUnless(clientState.checkForMisbehaviour(clientMessage))
    // update state based on misbehaviour
    updateStateOnMisbehaviour(misbehaviour)
}
```

### Properties & Invariants

- Client identifiers are immutable & first-come-first-serve. Clients cannot be deleted (allowing deletion would potentially allow future replay of past packets if identifiers were re-used).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

New client types can be added by IBC implementations at-will as long as they conform to this interface.

## Example Implementations

Please see the ibc-go implementations of light clients for examples of how to implement your own: <https://github.com/cosmos/ibc-go/blob/main/modules/light-clients>.

## History

Mar 5, 2019 - Initial draft finished and submitted as a PR

May 29, 2019 - Various revisions, notably multiple commitment-roots

Aug 15, 2019 - Major rework for clarity around client interface

Jan 13, 2020 - Revisions for client type separation & path alterations

Jan 26, 2020 - Addition of query interface

Jul 27, 2022 - Addition of `verifyClientState` function, and move `ClientState` to the `provableStore`

August 4, 2022 - Changes to ClientState interface and associated handler to align with changes in 02-client-refactor ADR: <https://github.com/cosmos/ibc-go/pull/1871>

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
