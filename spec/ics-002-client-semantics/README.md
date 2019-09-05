---
ics: 2
title: Client Semantics
stage: draft
category: IBC/TAO
requires: 23, 24
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-08-25
---

## Synopsis

This standard specifies the properties that consensus algorithms of machines implementing the interblockchain
communication protocol are required to satisfy. These properties are necessary for efficient and safe
verification in the higher-level protocol abstractions. The algorithm utilised in IBC to verify the
consensus transcript & state sub-components of another machine is referred to as a "validity predicate",
and pairing it with a state that the verifier assumes to be correct forms a "light client" (often shortened to "client").

This standard also specifies how light clients will be stored, registered, and updated in the
canonical IBC handler. The stored client instances will be introspectable by a third party actor,
such as a user inspecting the state of the chain and deciding whether or not to send an IBC packet.

### Motivation

In the IBC protocol, an actor, which may be an end user, an off-chain process, or a machine,
needs to be able to verify updates to the state of another machine
which the other machine's consensus algorithm has agreed upon, and reject any possible updates
which the other machine's consensus algorithm has not agreed upon. A light client is the algorithm
with which a machine can do so. This standard formalises the light client model and requirements,
so that the IBC protocol can easily integrate with new machines which are running new consensus algorithms
as long as associated light client algorithms fulfilling the listed requirements are provided.

Beyond the properties described in this specification, IBC does not impose any requirements on
the internal operation of machines and their consensus algorithms. A machine may consist of a
single process signing operations with a private key, a quorum of processes signing in unison,
many processes operating a Byzantine fault-tolerant consensus algorithm, or other configurations yet to be invented
— from the perspective of IBC, a machine is defined entirely by its light client validation & equivocation detection logic.
Clients will generally not include validation of the state transition logic in general
(as that would be equivalent to simply executing the other state machine), but may
elect to validate parts of state transitions in particular cases.

Clients could also act as thresholding views of other clients. In the case where
modules utilising the IBC protocol to interact with probabilistic-finality consensus algorithms
which might require different finality thresholds for different applications, one write-only
client could be created to track headers and many read-only clients with different finality
thresholds (confirmation depths after which state roots are considered final) could use that same state.

The client protocol should also support third-party introduction. Alice, a module on a machine,
wants to introduce Bob, a second module on a second machine who Alice knows (and who knows Alice),
to Carol, a third module on a third machine, who Alice knows but Bob does not. Alice must utilise
an existing channel to Bob to communicate the canonically-serialisable validity predicate for
Carol, with which Bob can then open a connection and channel so that Bob and Carol can talk directly.
If necessary, Alice may also communicate to Carol the validity predicate for Bob, prior to Bob's
connection attempt, so that Carol knows to accept the incoming request.

Client interfaces should also be constructed so that custom validation logic can be provided safely
to define a custom client at runtime, as long as the underlying state machine can provide an
appropriate gas metering mechanism to charge for compute and storage. On a host state machine
which supports WASM execution, for example, the validity predicate and equivocation predicate
could be provided as executable WASM functions when the client instance is created.

### Definitions

* `get`, `set`, `Path`, and `Identifier` are as defined in [ICS 24](../ics-024-host-requirements).

* `CommitmentRoot` is as defined in [ICS 23](../ics-023-vector-commitments). It must provide an inexpensive way for
  downstream logic to verify whether key/value pairs are present in state at a particular height.

* `ConsensusState` is an opaque type representing the state of a validity predicate.
  `ConsensusState` must be able to verify state updates agreed upon by the associated consensus algorithm.
  It must also be serialisable in a canonical fashion so that third parties, such as counterparty machines,
  can check that a particular machine has stored a particular `ConsensusState`. It must finally be
  introspectable by the state machine which it is for, such that the state machine can look up its
  own `ConsensusState` at a past height.

* `ClientState` is an opaque type representing the state of a client.
  A `ClientState` must expose query functions to verify membership or non-membership of
  key/value pairs in state at particular heights and to retrieve the current `ConsensusState`.

### Desired Properties

Light clients must provide a secure algorithm to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level abstractions will then be able to verify
sub-components of the state with the `CommitmentRoot`s stored in the `ConsensusState`, which are
guaranteed to have been committed by the other chain's consensus algorithm.

Validity predicates are expected to reflect the behaviour of the full nodes which are running the
corresponding consensus algorithm. Given a `ConsensusState` and a list of messages, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects it, then the light client MUST also reject it.

Light clients are not replaying the whole message transcript, so it is possible under cases of
consensus misbehaviour that the light clients' behaviour differs from the full nodes'.
In this case, a misbehaviour proof which proves the divergence between the validity predicate
and the full node can be generated and submitted to the chain so that the chain can safely deactivate the
light client, invalidate past state roots, and await higher-level intervention.

## Technical Specification

This specification outlines what each *client type* must define. A client type is a set of definitions
of the data structures, initialisation logic, validity predicate, and misbehaviour predicate required
to operate a light client. State machines implementing the IBC protocol can support any number of client
types, and each client type can be instantiated with different initial consensus states in order to track
different consensus instances. In order to establish a connection between two machines (see [ICS 3](../ics-003-connection-semantics)),
the machines must each support the client type corresponding to the other machine's consensus algorithm.

Specific client types shall be defined in later versions of this specification and a canonical list shall exist in this repository.
Machines implementing the IBC protocol are expected to respect these client types, although they may elect to support only a subset.

### Data Structures

#### ConsensusState

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

#### Header

A `Header` is an opaque data structure defined by a client type which provides information to update a `ConsensusState`.
Headers can be submitted to an associated client to update the stored `ConsensusState`. They likely contain a height, a proof,
a commitment root, and possibly updates to the validity predicate.

```typescript
type Header = bytes
```

#### Validity predicate

A validity predicate is an opaque function defined by a client type to verify `Header`s depending on the current `ConsensusState`.
Using the validit pPredicate SHOULD be far more computationally efficient than replaying the full consensus algorithm
for the given parent `Header` and the list of network messages.

The validity predicate & client state update logic are combined into a single `checkValidityAndUpdateState` type, which is defined as

```typescript
type checkValidityAndUpdateState = (Header) => Void
```

`checkValidityAndUpdateState` MUST throw an exception if the provided header was not valid.

If the provided header was valid, the client MUST also mutate internal state to store
now-finalised consensus roots and update any necessary signature authority tracking (e.g.
changes to the validator set) for future calls to the validity predicate.

#### Misbehaviour predicate

A misbehaviour predicate is an opaque function defined by a client type, used to check if data
constitutes a violation of the consensus protocol. This might be two signed headers
with different state roots but the same height, a signed header containing invalid
state transitions, or other evidence of malfeasance as defined by the consensus algorithm.

The misbehaviour predicate & client state update logic are combined into a single `checkMisbehaviourAndUpdateState` type, which is defined as

```typescript
type checkMisbehaviourAndUpdateState = (bytes) => Void
```

`checkMisbehaviourAndUpdateState` MUST throw an exception if the provided evidence was not valid.

If misbehaviour was valid, the client MUST also mutate internal state to mark appropriate heights which
were previously considered valid as invalid, according to the nature of the misbehaviour.

#### ClientState

`ClientState` is an opaque data structure defined by a client type.
It may keep arbitrary internal state to track verified roots and past misbehaviours.

Light clients are representation-opaque — different consensus algorithms can define different light client update algorithms —
but they must expose this common set of query functions to the IBC handler.

```typescript
type ClientState = bytes
```

Client types must also define a method to initialize a client state with a provided consensus state:

```typescript
type initialize = (state: ConsensusState) => ClientState
```

#### CommitmentProof

`CommitmentProof` is an opaque data structure defined by a client type.
It is utilised to verify presence or absence of a particular key/value pair in state
at a particular finalised height (necessarily associated with a particular commitment root).

```typescript
type CommitmentProof = bytes
```

#### State verification

Client types must define functions, in accordance with [ICS 23](../ics-023-vector-commitments), to verify presence or absence of particular key/value pairs
in state at particular heights. The behaviour of these functions MUST comply with the properties defined in [ICS 23](../ics-023-vector-commitments); however,
internal implementation details may differ (for example, a loopback client could simply read directly from the state and require no proofs).

```typescript
type verifyMembership = (ClientState, uint64, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, uint64, CommitmentProof, Path) => boolean
```

### Sub-protocols

IBC handlers MUST implement the functions defined below.

#### Path-space

Clients are stored under a unique `Identifier` prefix.
This ICS does not require that client identifiers be generated in a particular manner, only that they be unique.

`clientStatePath` takes an `Identifier` and returns a `Path` under which to store a particular client state.

```typescript
function clientStatePath(id: Identifier): Path {
    return "clients/{id}/state"
}
```

`clientTypePath` takes an `Identifier` and returns `Path` under which to store the type of a particular client.

```typescript
function clientTypePath(id: Identifier): Path {
    return "clients/{id}/type"
}
```

Consensus states MUST be stored separately so that they can be independently verified.

`consensusStatePath` takes an `Identifier` and returns a `Path` under which to store the consensus state of a client.

```typescript
function consensusStatePath(id: Identifier): Path {
    return "clients/{id}/consensusState"
}
```

##### Utilising past roots

To avoid race conditions between client updates (which change the state root) and proof-carrying
transactions in handshakes or packet receipt, many IBC handler functions allow the caller to specify
a particular past root to reference, which is looked up by height. IBC handler functions which do this
must ensure that they also perform any requisite checks on the height passed in by the caller to ensure
logical correctness.

#### Create

Calling `createClient` with the specified identifier & initial consensus state creates a new client.

```typescript
function createClient(
  id: Identifier,
  clientType: ClientType,
  consensusState: ConsensusState) {
    abortTransactionUnless(privateStore.get(clientStatePath(id)) === null)
    abortSystemUnless(privateStore.get(clientTypePath(id)) === null)
    clientState = clientType.initialize(consensusState)
    privateStore.set(clientStatePath(id), clientState)
    provableStore.set(clientTypePath(id), clientType)
}
```

#### Query

Client consensus state and client internal state can be queried by identifier. The returned
client state must fulfil an interface allowing membership / non-membership verification.

```typescript
function queryClientConsensusState(id: Identifier): ConsensusState {
    return provableStore.get(consensusStatePath(id))
}
```

```typescript
function queryClient(id: Identifier): ClientState {
    return privateStore.get(clientStatePath(id))
}
```

#### Update

Updating a client is done by submitting a new `Header`. The `Identifier` is used to point to the
stored `ClientState` that the logic will update. When a new `Header` is verified with
the stored `ClientState`'s validity predicate and `ConsensusState`, the client MUST
update its internal state accordingly, possibly finalising commitment roots and
updating the signature authority logic in the stored consensus state.

```typescript
function updateClient(
  id: Identifier,
  header: Header) {
    clientType = provableStore.get(clientTypePath(id))
    abortTransactionUnless(clientType !== null)
    clientState = privateStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    clientType.checkValidityAndUpdateState(clientState, header)
}
```

#### Misbehaviour

If the client detects evidence of misbehaviour, the client can be alerted, possibly invalidating
previously valid state roots & preventing future updates.

```typescript
function submitMisbehaviourToClient(
  id: Identifier,
  evidence: bytes) {
    clientType = provableStore.get(clientTypePath(id))
    abortTransactionUnless(clientType !== null)
    clientState = privateStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    clientType.checkMisbehaviourAndUpdateState(clientState, evidence)
}
```

### Example Implementation

An example validity predicate is constructed for a chain running a single-operator consensus algorithm,
where the valid blocks are signed by the operator. The operator signing Key
can be changed while the chain is running.

The client-specific types are then defined as follows:

- `ConsensusState` stores the latest height and latest public key
- `Header`s contain a height, a new commitment root, a signature by the operator, and possibly a new public key
- `checkValidityAndUpdateState` checks that the submitted height is monotonically increasing and that the signature is correct, then mutates the internal state
- `checkMisbehaviourAndUpdateState` checks for two headers with the same height & different commitment roots, then mutates the internal state

```typescript
interface ClientState {
  frozen: boolean
  pastPublicKeys: Set<PublicKey>
  verifiedRoots: Map<uint64, CommitmentRoot>
}

interface ConsensusState {
  sequence: uint64
  publicKey: PublicKey
}

interface Header {
  sequence: uint64
  commitmentRoot: CommitmentRoot
  signature: Signature
  newPublicKey: Maybe<PublicKey>
}

interface Evidence {
  h1: Header
  h2: Header
}

// algorithm run by operator to commit a new block
function commit(
  root: CommitmentRoot,
  sequence: uint64,
  newPublicKey: Maybe<PublicKey>): Header {
    signature = privateKey.sign(root, sequence, newPublicKey)
    header = Header{sequence, root, signature, newPublicKey}
    return header
}

// initialisation function defined by the client type
function initialize(consensusState: ConsensusState): ClientState {
  return ClientState{false, Set.singleton(consensusState.publicKey), Map.empty()}
}

// validity predicate function defined by the client type
function checkValidityAndUpdateState(
  clientState: ClientState,
  header: Header) {
    abortTransactionUnless(consensusState.sequence + 1 === header.sequence)
    abortTransactionUnless(consensusState.publicKey.verify(header.signature))
    if (header.newPublicKey !== null) {
      consensusState.publicKey = header.newPublicKey
      clientState.pastPublicKeys.add(header.newPublicKey)
    }
    consensusState.sequence = header.sequence
    clientState.verifiedRoots[sequence] = header.commitmentRoot
}

// state membership verification function defined by the client type
function verifyMembership(
  clientState: ClientState,
  sequence: uint64,
  proof: CommitmentProof
  path: Path,
  value: Value) {
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, value, proof)
}

// state non-membership function defined by the client type
function verifyNonMembership(
  clientState: ClientState,
  sequence: uint64,
  proof: CommitmentProof,
  path: Path) {
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyNonMembership(path, proof)
}

// misbehaviour verification function defined by the client type
// any duplicate signature by a past or current key freezes the client
function checkMisbehaviourAndUpdateState(
  clientState: ClientState,
  evidence: Evidence) {
    h1 = evidence.h1
    h2 = evidence.h2
    abortTransactionUnless(clientState.pastPublicKeys.contains(h1.publicKey))
    abortTransactionUnless(h1.sequence === h2.sequence)
    abortTransactionUnless(h1.commitmentRoot !== h2.commitmentRoot || h1.publicKey !== h2.publicKey)
    abortTransactionUnless(h1.publicKey.verify(h1.signature))
    abortTransactionUnless(h2.publicKey.verify(h2.signature))
    clientState.frozen = true
}
```

### Properties & Invariants

- Client identifiers are immutable & first-come-first-serve. Clients cannot be deleted (allowing deletion would potentially allow future replay of past packets if identifiers were re-used).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

New client types can be added by IBC implementations at-will as long as they confirm to this interface.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

March 5th 2019: Initial draft finished and submitted as a PR
May 29 2019: Various revisions, notably multiple commitment-roots
Aug 15 2019: Major rework for clarity around client interface

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
