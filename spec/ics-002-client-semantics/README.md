---
ics: 2
title: Client Semantics
stage: draft
category: IBC/TAO
requires: 23, 24
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-08-15
---

## Synopsis

This standard specifies the properties that consensus algorithms of machines implementing the interblockchain
communication protocol are required to satisfy. These properties are necessary for efficient and safe
verification in the higher-level protocol abstractions. The algorithm utilised in IBC to verify the
consensus transcript & state sub-components of another machine is referred to as a "validity predicate",
and pairing it with a state that the verifier trusts forms a "light client" (often shortened to "client").

This standard also specifies how light clients will be stored, registered, and updated in the
canonical IBC handler. The stored client instances will be introspectable by a third party actor,
such as a user inspecting the state of the chain and deciding whether or not to send an IBC packet.

### Motivation

In the IBC protocol, a machine needs to be able to verify updates to the state of another machine
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

Clients could also act as thresholding views of other clients. In the case where
modules utilising the IBC protocol to interact with probabilistic-finality consensus algorithms
which might require different finality thresholds for different applications, one write-only
client could be created to track headers and many read-only clients with different finality
thresholds (confirmation depths after which state roots are considered final) could use that same state.

Another problem to consider is that of third-party introduction. Alice, a module on a machine,
wants to introduce Bob, a second module on a second machine who Alice knows (and who knows Alice),
to Carol, a third module on a third machine, who Alice knows but Bob does not. Alice must utilise
an existing channel to Bob to communicate the canonically-serializable validity predicate for
Carol, with which Bob can then open a connection & channel so that Bob and Carol can talk directly.
If necessary, Alice may also communicate to Carol the validity predicate for Bob, prior to Bob's
connection attempt, so that Carol knows to accept the incoming request.

### Definitions

* `get`, `set`, `Key`, and `Identifier` are as defined in [ICS 24](../ics-024-host-requirements).

* `CommitmentRoot` is as defined in [ICS 23](../ics-023-vector-commitments). It must provide an inexpensive way for
  downstream logic to verify whether key-value pairs are present in state at a particular height.

* `ConsensusState` is an opaque type representing the state of a validity predicate.
  `ConsensusState` must be able to verify state updates agreed upon by the associated consensus algorithm.
  It must also be serialisable in a canonical fashion so that third parties, such as counterparty machines,
  can check that a particular machine has stored a particular `ConsensusState`.

* `ClientState` is an opaque type representing the state of a client.
  A `ClientState` must expose query functions to retrieve trusted state roots at previously
  verified heights and retrieve the current `ConsensusState`.

* `createClient`, `queryClient`, `updateClient`, `freezeClient`, and `deleteClient` function signatures are as defined in [ICS 25](../ics-025-handler-interface).
  The function implementations are defined in this specification.

### Desired Properties

Light clients must provide a secure algorithm to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level abstractions will then be able to verify
sub-components of the state with the `CommitmentRoot`s stored in the `ConsensusState`, which are
guaranteed to have been committed by the other chain's consensus algorithm.

`ValidityPredicate`s are expected to reflect the behaviour of the full nodes which are running the  
corresponding consensus algorithm. Given a `ConsensusState` and `[Message]`, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects it, then the light client MUST also reject it.

Light clients are not replaying the whole message transcript, so it is possible under cases of
consensus misbehaviour that the light clients' behaviour differs from the full nodes'.
In this case, an misbehaviour proof which proves the divergence between the `ValidityPredicate`
and the full node can be generated and submitted to the chain so that the chain can safely deactivate the
light client, invalidate past state roots, and await higher-level intervention.

## Technical Specification

This specification outlines what each *client type* must define. A client type is a set of definitions
of the data structures, initialisation logic, validity predicate, and misbehaviour predicate required
to operate a light client. State machines implementing the IBC protocol can support any number of client
types, and each client type can be instantiated with different initial consensus states in order to track
different consensus instances. In order to establish a connection between two machines (see [ICS 3](../ics-003-connection-semantics)),
the machines must each support the client type corresponding to the other machine's consensus algorithm.

By convention, client types shall be globally namespaced between machines implementing the IBC protocol.

### Data Structures

#### ConsensusState

`ConsensusState` is a opaque data structure defined by a client type, used by the validity predicate to
verify new commits & state roots. Likely the structure will contain the last commit produced by
the consensus process, including signatures and validator set metadata.

`ConsensusState` MUST be generated from an instance of `Consensus`, which assigns unique heights
for each `ConsensusState`. Two `ConsensusState`s on the same chain SHOULD NOT have the same height
if they do not have equal commitment roots. Such an event is called an "equivocation" and MUST be classified
as misbehaviour. Should one occur, a proof should be generated and submitted so that the client can be frozen
and previous state roots invalidated as necessary.

The `ConsensusState` of a chain MUST have a canonical serialization, so that other chains can check
that a stored consensus state is equal to another.

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

#### ValidityPredicate

A `ValidityPredicate` is an opaque function defined by a client type to verify `Header`s depending on the current `ConsensusState`.
Using the ValidityPredicate SHOULD be far more computationally efficient than replaying the full consensus algorithm
for the given parent `Header` and the list of network messages.

The `ValidityPredicate` type is defined as

```typescript
type ValidityPredicate = (Header) => Void
```

The validity predicate MUST throw an exception if the provided header was not valid.

If the provided header was valid, the client MUST also mutate internal state to store
now-finalised consensus roots and update any necessary signature authority tracking (e.g.
changes to the validator set) for future calls to the validity predicate.

#### MisbehaviourPredicate

An `MisbehaviourPredicate` is an opaque function defined by a client type, used to check if data
constitutes a violation of the consensus protocol. This might be two signed headers
with different state roots but the same height, a signed header containing invalid
state transitions, or other evidence of malfeasance as defined by the consensus algorithm.

The `MisbehaviourPredicate` type is defined as

```typescript
type MisbehaviourPredicate = (bytes) => Void
```

The misbehaviour predicate MUST throw an exception if the provided evidence was not valid.

If misbehaviour was valid, the client MUST also mutate internal state to mark appropriate heights which
were previously considered valid invalid, according to the nature of the misbehaviour.

More details about `MisbehaviourPredicate`s can be found in [CONSENSUS.md](./CONSENSUS.md)

#### ClientState

`ClientState` is an opaque data structure defined by a client type.
It may keep arbitrary internal state to track verified roots and past misbehaviours.

Light clients are representation-opaque — different consensus algorithms can define different light client update algorithms —
but they must expose this common set of query functions to the IBC handler.

```typescript
type ClientState = bytes
```

Client types must also define a function to initialize a client state with a provided consensus state:

```typescript
type initialize = (ConsensusState) => ClientState
```

#### Root introspection

Client types must define a function to lookup previously verified `CommitmentRoot`s,
which are then used to verify presence or absence of particular key-value pairs in state at particular heights.

```typescript
type getVerifiedRoot = (ClientState, uint64) => CommitmentRoot
```

### Sub-protocols

IBC handlers MUST implement the functions defined below.

#### Key-space

Clients are stored under a unique `Identifier` prefix.
This ICS does not require that client identifiers be generated in a particular manner, only that they be unique.

`clientStateKey` takes an `Identifier` and returns a `Key` under which to store a particular client state.

```typescript
function clientStateKey(id: Identifier): Key {
  return "clients/{id}/state"
}
```

`clientTypeKey` takes an `Identifier` and returns ` Key` under which to store the type of a particular client.

```typescript
function clientTypeKey(id: Identifier): Key {
  return "clients/{id}/type"
}
```

Consensus states MUST be stored separately so that they can be independently verified.

`consensusStateKey` takes an `Identifier` and returns a `Key` under which to store the consensus state of a client.

```typescript
function consensusStateKey(id: Identifier): Key {
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
function createClient(id: Identifier, clientType: ClientType, consensusState: ConsensusState) {
  assert(privateStore.get(clientStateKey(id)) === null)
  assert(provableStore.get(clientTypeKey(id)) === null)
  clientState = clientType.initialize(consensusState)
  provableStore.set(clientTypeKey(id), clientType)
  privateStore.set(clientStateKey(id), clientState)
}
```

#### Query

Client consensus state and previously verified roots can be queried by identifier.

```typescript
function queryClientConsensusState(id: Identifier): ConsensusState {
  return provableStore.get(consensusStateKey(id))
}
```

```typescript
function queryClientRoot(id: Identifier, height: uint64): CommitmentRoot {
  clientType = provableStore.get(clientTypeKey(id))
  clientState = privateStore.get(clientStateKey(id))
  return clientType.getVerifiedRoot(height)
}
```

#### Update

Updating a client is done by submitting a new `Header`. The `Identifier` is used to point to the
stored `ClientState` that the logic will update. When a new `Header` is verified with
the stored `ClientState`'s `ValidityPredicate` and `ConsensusState`, the client MUST
update its internal state accordingly, possibly finalising commitment roots and
updating the signature authority logic in the stored consensus state.

```typescript
function updateClient(id: Identifier, header: Header) {
  clientType = provableStore.get(clientTypeKey(id))
  assert(clientType !== null)
  clientState = privateStore.get(clientStateKey(id))
  assert(clientState !== null)
  assert(clientType.validityPredicate(clientState, header))
}
```

#### Misbehaviour

If the client detects evidence of misbehaviour, the client can be alerted, possibly invalidating
previously valid state roots & preventing future updates.

```typescript
function submitMisbehaviourToClient(id: Identifier, evidence: bytes) {
  clientType = provableStore.get(clientTypeKey(id))
  assert(clientType !== null)
  clientState = privateStore.get(clientStateKey(id))
  assert(clientState !== null)
  assert(clientType.misbehaviourPredicate(clientState, evidence))
}
```

### Example Implementation

An example validity predicate is constructed for a chain running a single-operator consensus algorithm,
where the valid blocks are signed by the operator. The operator signing Key
can be changed while the chain is running.

The client-specific types are then defined as follows:
- `ConsensusState` stores the latest height and latest public key
- `Header`s contain a height, a new commitment root, a signature by the operator, and possibly a new public key
- `ValidityPredicate` checks that the submitted height is monotonically increasing and that the signature is correct
- `MisbehaviourPredicate` checks for two headers with the same height & different commitment roots

```typescript
interface ClientState {
  frozen: boolean
  pastPublicKeys: Set<PublicKey>
  verifiedRoots: Map<uint64, CommitmentRoot>
}

interface ConsensusState {
  height: uint64
  publicKey: PublicKey
}

interface Header {
  height: uint64
  commitmentRoot: CommitmentRoot
  signature: Signature
  newPublicKey: Maybe<PublicKey>
}

interface Evidence {
  h1: Header
  h2: Header
}

function commit(root: CommitmentRoot, height: uint64, newPublicKey: Maybe<PublicKey>): Header {
  signature = privateKey.sign(root, height, newPublicKey)
  header = Header{height, root, signature}
  return header
}

function initialize(consensusState: ConsensusState): ClientState {
  return ClientState{false, Set.singleton(consensusState.publicKey), Map.empty()}
}

function validityPredicate(clientState: ClientState, header: Header) {
  assert(consensusState.height + 1 === header.height)
  assert(consensusState.publicKey.verify(header.signature))
  if (header.newPublicKey !== null)
    consensusState.publicKey = header.newPublicKey
    clientState.pastPublicKeys.add(header.newPublicKey)
  consensusState.height = header.height
  clientState.verifiedRoots[height] = header.commitmentRoot
}

function getVerifiedRoot(clientState: ClientState, height: uint64) {
  assert(!client.frozen)
  return client.verifiedRoots[height]
}

function misbehaviourPredicate(clientState: ClientState, evidence: Evidence) {
  h1 = evidence.h1
  h2 = evidence.h2
  assert(h1.publicKey === h2.publicKey)
  assert(clientState.pastPublicKeys.contains(h1.publicKey))
  assert(h1.height === h2.height)
  assert(h1.commitmentRoot !== h2.commitmentRoot)
  assert(h1.publicKey.verify(h1.signature))
  assert(h2.publicKey.verify(h2.signature))
  client.frozen = true
}
```

### Properties & Invariants

- Client identifiers are immutable & first-come-first-serve: once a client identifier has been allocated, all future headers & roots-of-trust stored under that identifier will have satisfied the client's validity predicate.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

In a future version, this ICS will define a new function `unfreezeClient` that can be called 
when the application logic resolves an misbehaviour event.

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
