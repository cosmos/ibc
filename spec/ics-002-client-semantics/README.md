---
ics: 2
title: Client Semantics
stage: draft
category: IBC/TAO
kind: interface
requires: 23, 24
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2020-01-13
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

#### Consensus

`Consensus` is a `Header` generating function which takes the previous
`ConsensusState` with the messages and returns the result.

```typescript
type Consensus = (ConsensusState, [Message]) => Header
```

### Blockchain

A blockchain is a consensus algorithm which generates valid `Header`s.
It generates a unique list of headers starting from a genesis `ConsensusState` with arbitrary
messages.

`Blockchain` is defined as

```typescript
interface Blockchain {
  genesis: ConsensusState
  consensus: Consensus
}
```
where
  * `Genesis` is the genesis `ConsensusState`
  * `Consensus` is the header generating function

The headers generated from a `Blockchain` are expected to satisfy the following:

1. Each `Header` MUST NOT have more than one direct child

* Satisfied if: finality & safety
* Possible violation scenario: validator double signing, chain reorganisation (Nakamoto consensus)

2. Each `Header` MUST eventually have at least one direct child

* Satisfied if: liveness, light-client verifier continuity
* Possible violation scenario: synchronised halt, incompatible hard fork

3. Each `Header`s MUST be generated by `Consensus`, which ensures valid state transitions

* Satisfied if: correct block generation & state machine
* Possible violation scenario: invariant break, super-majority validator cartel

Unless the blockchain satisfies all of the above the IBC protocol
may not work as intended: the chain can receive multiple conflicting
packets, the chain cannot recover from the timeout event, the chain can
steal the user's asset, etc.

The validity of the validity predicate is dependent on the security model of the
`Consensus`. For example, the `Consensus` can be a proof of authority with
a trusted operator, or a proof of stake but with
insufficient value of stake. In such cases, it is possible that the
security assumptions break, the correspondence between `Consensus` and
the validity predicate no longer exists, and the behaviour of the validity predicate becomes
undefined. Also, the `Blockchain` may not longer satisfy
the requirements above, which will cause the chain to be incompatible with the IBC
protocol. In cases of attributable faults, a misbehaviour proof can be generated and submitted to the
chain storing the client to safely freeze the light client and
prevent further IBC packet relay.

#### Validity predicate

A validity predicate is an opaque function defined by a client type to verify `Header`s depending on the current `ConsensusState`.
Using the validity predicate SHOULD be far more computationally efficient than replaying the full consensus algorithm
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

Client types MUST define a method to initialise a client state with a provided consensus state, writing to state as appropriate.

```typescript
type initialise = (consensusState: ConsensusState) => ClientState
```

Client types MUST define a method to fetch the current height (height of the most recent validated header).

```typescript
type latestClientHeight = (
  clientState: ClientState)
  => uint64
```

#### CommitmentProof

`CommitmentProof` is an opaque data structure defined by a client type in accordance with [ICS 23](../ics-023-vector-commitments).
It is utilised to verify presence or absence of a particular key/value pair in state
at a particular finalised height (necessarily associated with a particular commitment root).

#### State verification

Client types must define functions to authenticate internal state of the state machine which the client tracks.
Internal implementation details may differ (for example, a loopback client could simply read directly from the state and require no proofs).

##### Required functions

`verifyClientConsensusState` verifies a proof of the consensus state of the specified client stored on the target machine.

```typescript
type verifyClientConsensusState = (
  clientState: ClientState,
  height: uint64,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState)
  => boolean
```

`verifyConnectionState` verifies a proof of the connection state of the specified connection end stored on the target machine.

```typescript
type verifyConnectionState = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd)
  => boolean
```

`verifyChannelState` verifies a proof of the channel state of the specified channel end, under the specified port, stored on the target machine.

```typescript
type verifyChannelState = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd)
  => boolean
```

`verifyPacketData` verifies a proof of an outgoing packet commitment at the specified port, specified channel, and specified sequence.

```typescript
type verifyPacketData = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes)
  => boolean
```

`verifyPacketAcknowledgement` verifies a proof of an incoming packet acknowledgement at the specified port, specified channel, and specified sequence.

```typescript
type verifyPacketAcknowledgement = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes)
  => boolean
```

`verifyPacketAcknowledgementAbsence` verifies a proof of the absence of an incoming packet acknowledgement at the specified port, specified channel, and specified sequence.

```typescript
type verifyPacketAcknowledgementAbsence = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64)
  => boolean
```

`verifyNextSequenceRecv` verifies a proof of the next sequence number to be received of the specified channel at the specified port.

```typescript
type verifyNextSequenceRecv = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64)
  => boolean
```

##### Implementation strategies

###### Loopback

A loopback client of a local machine merely reads from the local state, to which it must have access.

###### Simple signatures

A client of a solo machine with a known public key checks signatures on messages sent by that local machine,
which are provided as the `Proof` parameter. The `height` parameter can be used as a replay protection nonce.

Multi-signature or threshold signature schemes can also be used in such a fashion.

###### Proxy clients

Proxy clients verify another (proxy) machine's verification of the target machine, by including in the
proof first a proof of the client state on the proxy machine, and then a secondary proof of the sub-state of
the target machine with respect to the client state on the proxy machine. This allows the proxy client to
avoid storing and tracking the consensus state of the target machine itself, at the cost of adding
security assumptions of proxy machine correctness.

###### Merklized state trees

For clients of state machines with Merklized state trees, these functions can be implemented by calling `verifyMembership` or `verifyNonMembership`, using a verified Merkle
root stored in the `ClientState`, to verify presence or absence of particular key/value pairs in state at particular heights in accordance with [ICS 23](../ics-023-vector-commitments).

```typescript
type verifyMembership = (ClientState, uint64, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, uint64, CommitmentProof, Path) => boolean
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

Calling `createClient` with the specified identifier & initial consensus state creates a new client.

```typescript
function createClient(
  id: Identifier,
  clientType: ClientType,
  consensusState: ConsensusState) {
    abortTransactionUnless(validateClientIdentifier(id))
    abortTransactionUnless(privateStore.get(clientStatePath(id)) === null)
    abortSystemUnless(provableStore.get(clientTypePath(id)) === null)
    clientType.initialise(consensusState)
    provableStore.set(clientTypePath(id), clientType)
}
```

#### Query

Client consensus state and client internal state can be queried by identifier, but
the specific paths which must be queried are defined by each client type.

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
  commitmentRoot: CommitmentRoot,
  sequence: uint64,
  newPublicKey: Maybe<PublicKey>): Header {
    signature = privateKey.sign(commitmentRoot, sequence, newPublicKey)
    header = {sequence, commitmentRoot, signature, newPublicKey}
    return header
}

// initialisation function defined by the client type
function initialise(consensusState: ConsensusState): () {
  clientState = {
    frozen: false,
    pastPublicKeys: Set.singleton(consensusState.publicKey),
    verifiedRoots: Map.empty()
  }
  privateStore.set(identifier, clientState)
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

function verifyClientConsensusState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusState: ConsensusState) {
    path = applyPrefix(prefix, "clients/{clientIdentifier}/consensusStates/{height}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, consensusState, proof)
}

function verifyConnectionState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd) {
    path = applyPrefix(prefix, "connections/{connectionIdentifier}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, connectionEnd, proof)
}

function verifyChannelState(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, channelEnd, proof)
}

function verifyPacketData(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/packets/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, data, proof)
}

function verifyPacketAcknowledgement(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, acknowledgement, proof)
}

function verifyPacketAcknowledgementAbsence(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/acknowledgements/{sequence}")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyNonMembership(path, proof)
}

function verifyNextSequenceRecv(
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64) {
    path = applyPrefix(prefix, "ports/{portIdentifier}/channels/{channelIdentifier}/nextSequenceRecv")
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[sequence].verifyMembership(path, nextSequenceRecv, proof)
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

New client types can be added by IBC implementations at-will as long as they conform to this interface.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Mar 5, 2019 - Initial draft finished and submitted as a PR

May 29, 2019 - Various revisions, notably multiple commitment-roots

Aug 15, 2019 - Major rework for clarity around client interface

Jan 13, 2020 - Revisions for client type separation & path alterations

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
