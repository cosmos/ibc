---
ics: 2
title: Client Semantics
stage: draft
category: IBC/TAO
kind: interface
requires: 24
required-by: 4
version compatibility: ibc-go v10.0.0
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>, Aditya Sripal <aditya@interchain.io>
created: 2019-02-25
modified: 2024-08-22
---

## Synopsis

The IBC protocol provides secure packet flow between applications on different ledgers by verifying the packet messages using clients of the counterparty state machines. While ICS-4 defines the core packet flow logic between two chains and the provable commitments they must make in order to communicate, this standard ICS-2 specifies **how** a chain verifies the IBC provable commitments of the counterparty which is crucial to securely receive and process a packet flow message arriving from the counterparty.

This standard focuses on how to keep track of the counterparty consensus and verify the state machine; it also  specifies the properties that consensus algorithms of state machines implementing the inter-blockchain
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

The IBC protocol needs to be able to verify updates to the state of another state machine (i.e., the *remote state machine*). 
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

Client interfaces should also be constructed so that custom validation logic can be provided safely
to define a custom client at runtime, as long as the underlying state machine can provide an
appropriate gas metering mechanism to charge for compute and storage. On a host state machine
which supports WASM execution, for example, the validity predicate and misbehaviour predicate
could be provided as executable WASM functions when the client instance is created.

### Definitions

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

- `ClientMessage` is an arbitrary message defined by the client type that relayers can submit in order to update the client.
  The ClientMessage may be intended as a regular update which may add new consensus state for proof verification, or it may contain
  misbehaviour which should freeze the client.

- `ValidityPredicate` is a function that validates a ClientMessage sent by a relayer in order to update the client. 
  Using the `ValidityPredicate` SHOULD be more computationally efficient than executing `Consensus`.

```typescript
type ValidityPredicate = (clientState: bytes, trustedConsensusState: bytes, trustedHeight: Number) => (newConsensusState: bytes, newHeight: Number, err: Error)
```

- `ConsensusState` is the *trusted view* of the state of a state machine at a particular `Height`.
  It MUST contain sufficient information to enable the `ValidityPredicate` to validate future state updates, 
  which can then be used to generate new `ConsensusState`s. 

- `ClientState` is the state of a client. It MUST expose an interface to higher-level protocol abstractions, 
  e.g., functions to verify proofs of the existence of particular values at particular paths at particular `Height`s.

- `MisbehaviourPredicate` is a function that checks whether the rules of `Consensus` were broken, 
  in which case the client MUST be *frozen*, i.e., no subsequent `ConsensusState`s can be generated.
  Verification against the client after it is frozen will also fail.

```typescript
type MisbehaviourPredicate = (clientState: bytes, trustedConsensusState: bytes, trustedHeight: Number, misbehaviour: bytes) => bool
```

- `Misbehaviour` is the proof needed by the `MisbehaviourPredicate` to determine whether 
  a violation of the consensus protocol occurred. For example, in the case the state machine 
  is a blockchain, a `Misbehaviour` might consist of two signed block headers with 
  different `ConsensusState` for the same `Height`.

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
which would result in the client being frozen. Once the client is frozen, a recovery mechanism to address
the situation must occur before client processing can presume. This recovery mechanism is out-of-scope 
of the IBC protocol as the specific recovery needed is highly case-dependent.

## Technical Specification

This specification outlines what each *client type* must define. A client type is a set of definitions
of the data structures, initialisation logic, validity predicate, and misbehaviour predicate required
to operate a light client. State machines implementing the IBC protocol can support any number of client
types, and each client type can be instantiated with different initial consensus states in order to track
different consensus instances.

Specific client types and their specifications are defined in the light clients section of this repository.

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
There MUST NOT be two valid `ConensusState`s for the same height. 
Such an event is called an "equivocation" and MUST be classified
as misbehaviour. Should one occur, a proof should be generated and submitted so that the client can be frozen
and previous state roots invalidated as necessary.

```typescript
type ConsensusState = bytes
```

The `ConsensusState` MUST define a `getTimestamp()` method which returns the timestamp **in seconds** associated with that consensus state.
This timestamp MUST be the timestamp used in the counterparty state machine and agreed to by `Consensus`.

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

#### `CommitmentProof`

`CommitmentProof` is an opaque data structure defined by the client type.

```typescript
type CommitmentProof = bytes
```

It is utilised to verify presence or absence of a particular key/value pair in state
at a particular finalised height (necessarily associated with a particular commitment root).

### State verification

Client types must define functions to authenticate internal state of the state machine which the client tracks.
Internal implementation details may differ (for example, a loopback client could simply read directly from the state and require no proofs).

`verifyMembership` is a generic proof verification method which verifies a proof of the existence of a value at a given `CommitmentPath` at the specified height. It MUST return an error if the verification is not successful. 
The caller is expected to construct the full `CommitmentPath` from a `CommitmentPrefix` and a standardized path (as defined in [ICS 4](../ics-004-packet-semantics/PACKET.md)). 

```typescript
type verifyMembership = (
  clientState: ClientState,
  height: Height,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: bytes)
  => Error
```

`verifyNonMembership` is a generic proof verification method which verifies a proof of absence of a given `CommitmentPath` at the specified height. It MUST return an error if the verification is not successful. 
The caller is expected to construct the full `CommitmentPath` from a `CommitmentPrefix` and a standardized path (as defined in [ICS 24](../ics-024-host-requirements/README.md#path-space)).

Since the verification method is designed to give complete control to client implementations, clients can support chains that do not provide absence proofs by verifying the existence of a non-empty sentinel `ABSENCE` value. Thus in these special cases, the proof provided will be an Existence proof, and the client will verify that the `ABSENCE` value is stored under the given path for the given height.

```typescript
type verifyNonMembership = (
  clientState: ClientState,
  height: Height,
  proof: CommitmentProof,
  path: CommitmentPath)
  => Error
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

For clients of state machines with Merklized state trees, these functions can be implemented as MerkleTree Existence and NonExistence proofs. Client implementations may choose to implement these methods for the specific tree used by the counterparty chain or they can use the tree-generic [ICS-23](https://github.com/cosmos/ics23) `verifyMembership` or `verifyNonMembership` methods, using a verified Merkle
root stored in the `ClientState`, to verify presence or absence of particular key/value pairs in state at particular heights for any ICS-23 compliant tree given a ProofSpec that describes how the tree is constructed. In this case, the ICS-23 `ProofSpec` MUST be provided to the client on initialization.

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

#### CreateClient

Calling `createClient` with the client state and initial consensus state creates a new client. The intiator of this client is responsible for setting all of the initial parameters of the `ClientState` and the initial root-of-trust `ConsensusState`. The client implementation is then responsible for executing the light client `ValidityPredicate` against these initial parameters. Thus, once a root-of-trust is instantiated; the light client guarantees to preserve that trust within the confines of the security model as parameterized by the `ClientState`. If a user verifies that a client is a valid client of the counterparty chain once, they can be guaranteed that it will remain a valid client into the future so long as the `MisbehaviourPredicate` is not triggered. If the `MisbehaviourPredicate` is triggered however, this can be submitted as misbehaviour to freeze the IBC light client operations.

CreateClient Inputs:

`clientType: string`: This is the client-type that references a particular light client implementation on the chain. The `CreateClient` message will create a new instance of the given client-type.
`ClientState: bytes`: This is the opaque client state as defined for the given client type. It will contain any parameters needed for verifying client updates and proof verification against a `ConsensusState`. The `ClientState` parameterizes the security model as implemented by the client type.
`ConsensusState: bytes`: This is the opaque consensus state as defined for the given client type. It is the initial consensus state provided and MUST be capable of being used by the `ValidityPredicate` to add new `ConsensusState`s to the client. The initial `ConsensusState` MAY also be used for proof verification but it is not necessary.
`Height: Number`: This is the height that is associated with the initial consensus state.

CreateClient Preconditions:

- The provided `clientType` is supported by the chain and can be routed to by the IBC handler.

CreateClient PostConditions:

- A unique identifier `clientId` is generated for the client
- The provided `ClientState` is persisted to state and retrievable given the `clientId`.
- The provided `ConsensusState` is persisted to state and retrievable given the `clientId` and `height`.

CreateClient ErrorConditions:

- The provided `ClientState` is invalid given the client type.
- The provided `ConsensusState` is invalid given the client type.
- The `Height` is not a positive number.

#### RegisterCounterparty

IBC Version 2 introduces a `registerCounterparty` procedure. Calling `registerCounterparty` with the clientId will register the counterparty clientId 
that the counterparty will use to write packet messages intended for our chain. All ICS24 provable paths to our chain will be keyed on the counterparty clientId, so each client must be aware of the counterparty's identifier in order to construct the path for key verification and ensure there is an authenticated stream of packet data between the clients that do not get written to by other clients.
The `registerCounterparty` also includes the `CommitmentPrefix` to use for the counterparty chain. Most chains will not store the ICS24 directly under the root of a MerkleTree and will instead store the standardized paths under a custom prefix, thus the counterparty client must be given this information to verify proofs correctly. The `CommitmentPrefix` is defined as an array of byte arrays to support nested Merkle trees. In this case, each element of the outer array is a key for each tree in the nested structure ordered from the top-most tree to the lowest level tree. In this case, the ICS24 path is appended to the key of the lowest-level tree (i.e. the last element of the commitment prefix) in order to get the full `CommitmentPath` for proof verification.

RegisterCounterparty Inputs:

`clientId: bytes`: The clientId on the executing chain.
`counterpartyClientId: bytes`: The identifier of the client used by the counterparty chain to verify the executing chain.
`counterpartyCommitmentPrefix: []bytes`: The prefix used by the counterparty chain.

RegisterCounterparty Preconditions:

- A client has already been created for the `clientId`

RegisterCounterparty Postconditions:

- The `counterpartyClientId` is retrievable given the `clientId`.
- The `counterpartyCommitmentPrefix` is retrievable given the `clientId`.

RegisterCounterparty ErrorConditions:

- There does not exist a client for the given `clientId`
- `RegisterCounterparty` has already been called for the given `clientId`

NOTE: Once the clients and counterparties have been registered on both sides, the connection between the clients is established and packet flow between the clients may commence. Users are expected to verify that the clients and counterparties are set correctly before using the connection to send packets. They may do this directly themselves or through social consensus.
NOTE: `RegisterCounterparty` is setting information that will be crucial for proper proof verification of IBC messages using our client. Thus, it must be authenticated properly. The `RegisterCounterparty` message can be permissionless in which case the fields must be authenticated against the counterparty chain using the client which may prove difficult and cumbersome. It is RECOMMENDED to simply ensure that the client creator address is the same as the one that registers the counterparty. Once the client and counterparty are set by the same creator, users can decide if the configuration is secure out-of-band.

#### Update

Updating a client is done by submitting a new `ClientMessage`. The `Identifier` is used to point to the
stored `ClientState` that the logic will update. When a new `ClientMessage` is verified using the `ValidityPredicate` with
the stored `ClientState` and a previously stored `ConsensusState`, the client MUST then add a new `ConsensusState` with a new `Height`.

If a client can no longer be updated (if, for example, the trusting period has passed),
then new packet flow will not be able to be processed. Manual intervention must take place to
reset the client state or migrate the client. This
cannot safely be done completely automatically, but chains implementing IBC could elect
to allow governance mechanisms to perform these actions
(perhaps even per-client/connection/channel in a multi-sig or contract).

UpdateClient Inputs:

`clientId: bytes`: The identifier of the client being updated.
`clientMessage: bytes`: The opaque clientMessage to update the client as defined by the given `clientType`. It MUST include the `trustedHeight` we wish to update from. This `trustedHeight` will be used to retrieve a trusted ConsensusState which we will use to update to a new consensus state using the `ValidityPredicate`.

UpdateClient Preconditions:

- A client has already been created for the `clientId`

UpdateClient Postconditions:

- A new `ConsensusState` is added to the client and persisted with a new `Height`
- Implementations MAY automatically detect misbehaviour in `UpdateClient` if the update itself is proof of misbehaviour (e.g. There is already a different `ConsensusState` for the given height, or time monotonicity is broken). It is recommended to automatically freeze the client in this case to avoid having to send a redundant `submitMisbehaviour` message.

UpdateClient ErrorConditions:

- The trusted `ConsensusState` referenced in the `ClientMessage` does not exist in state
- `ValidityPredicate(clientState, trustedConsensusState, trustedHeight)` returns an error

#### Misbehaviour

If `Consensus` of the counterparty chain is violated, then the relayer can submit proof of this as misbehaviour. Once the client is frozen, no updates may take place and all proof verification will fail. The client may be unfrozen by an out-of-band protocol once trust in the counterparty `Consensus` is restored and any invalid state caused by the break in `Consensus` is reverted on the executing chain.

SubmitMisbehaviour Inputs:
`clientId: bytes`: The identifier of the client being frozen.
`clientMessage: bytes`: The opaque clientMessage to freeze the client as defined by the given `clientType`. It MUST include the `trustedHeight` we wish to verify misbehaviour from. This `trustedHeight` will be used to retrieve a trusted ConsensusState which we will use to freeze the client given the `MisbehaviourPredicate`. It MUST also include the misbehaviour being submitted.

SubmitMisbehaviour Preconditions:

- A client has already been created for the `clientId`.

SubmitMisbehaviour Postconditions:

- The client is frozen, update and proof verification will fail until client is unfrozen again.

SubmitMisbehaviour ErrorConditions:

- The trusted `ConsensusState` referenced in the `ClientMessage` does not exist in state.
- `MisbehaviourPredicate(clientState, trustedConsensusState, trustedHeight, misbehaviour)` returns `false`.

### VerifyMembership and VerifyNonmembership

The IBC core packet handler uses the consensus states created in `UpdateClient` to verify ICS-4 standardized paths to authenticate packet messages. In order to do this, the IBC packet handler constructs the expected key/value for the given packet flow message and sends the expected path and value to the client along with the relayer-provided proof to the client for verification. Note that the proof is relayer provided, but the path and value are constructed by the IBC packet handler for the given packet. Thus, the relayer cannot forge proofs for packets that did not get sent. IBC Packet handler must also have the ability to prove nonmembership of a given path in order to enable timeout processing. Thus, clients must expose the following `verifyMembership` and `verifyNonMembership` methods:

```typescript
type verifyMembership = (ClientState, Height, CommitmentProof, Path, Value) => boolean
```

```typescript
type verifyNonMembership = (ClientState, Height, CommitmentProof, Path) => boolean
```

ProofVerification Inputs:

- `clientId: bytes`: The identifier of the client that will verify the proof.
- `Height: Number`: The height for the consensus state that the proof will be verified against.
- `Path: CommitmentPath`: The path of the key being proven. In the IBC protocol, this will be an ICS24 standardized path prefixed by the `CommitmentPrefix` registered on the counterparty. The `Path` MUST be constructed by the IBC handler given the IBC message, it MUST NOT be provided by the relayer as the relayer is untrusted.
- `Value: Optional<bytes>`: The value being proven.  If it is non-empty this is a membership proof. If the value is nil, this is a non-membership proof.

ProofVerification Preconditions:

- A client has already been created for the `clientId`.
- A `ConsensusState` is stored for the given `Height`.

ProofVerification Postconditions:

- Proof verification should be stateless in most cases. In the case that the proof verification is a signature check, we may wish to increment a nonce to prevent replay attacks.

ProofVerification Errorconditions:

- `CommitmentProof` does not successfully verify with the provided `CommitmentPath` and `Value` with the retrieved `ConsensusState` for the provided `Height`.

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

August 22, 2024 - [Changes for IBC/TAO V2](https://github.com/cosmos/ibc/pull/1147)  

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
