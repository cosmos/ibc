---
ics: 2
title: Consensus Verification
stage: draft
category: ibc-core
requires: 23, 24
required-by: 3, 24
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-05-28
---

## Synopsis

This standard specifies the properties that consensus algorithms of chains implementing the interblockchain
communication protocol are required to satisfy. These properties are necessary for efficient and safe
verification in the higher-level protocol abstractions. The algorithm utilized in IBC to verify the
consensus transcript & state subcomponents of another chain is referred to as a "light client verifier",
and pairing it with a state that the verifier trusts forms a "light client".

This standard also specifies how the light clients will be stored, registered, and updated in the
canonical IBC handler. The stored light client instances will be verifiable by a third party actor,
such as a user inspecting the state of the chain and deciding whether or not to send an IBC packet.

### Motivation

In the IBC protocol, a chain needs to be able to verify updates to the state of another chain
which the other chain's consensus algorithm has agreed upon, and reject any possible updates
which the other chain's consensus algorithm has not agreed upon. A light client is the algorithm
with which a chain can do so. This standard formalizes the light client model and requirements,
so that the IBC protocol can easily connect with new chains which are running new consensus algorithms,
as long as associated light client algorithms fulfilling the requirements are provided.

### Definitions

* `get`, `set`, `Key`, and `Identifier` are as defined in [ICS 24](../ics-24-host-requirements).

* `CommitmentRoot` is as defined in [ICS 23](../ics-23-vector-commitments).

* `ConsensusState` is an opaque type representing the state of a light client, defined in this ICS.
  `ConsensusState` must be able to verify state updates agreed upon by the associated consensus algorithm,
  and must provide an inexpensive way for downstream logic to verify whether key-value pairs are present
  in the state or not.

* `createClient`, `queryClient`, `updateClient`, `freezeClient`, and `deleteClient` function signatures are as defined in ICS 25.
  The function implementations are defined in this standard.

### Desired Properties

Light clients must provide a secure algorithm to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level abstractions will then be able to verify
subcomponents of the state with the `CommitmentRoot` stored in the `ConsensusState`, which is
guaranteed to have been committed by the other chain's consensus algorithm.

`ValidityPredicate`s are expected to reflect the behaviour of the full node which is running the  
corresponding consensus algorithm. Given a `ConsensusState` and `[Message]`, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects it, then the light client MUST also reject it. The consensus algorithm
ensures this correspondence. However light clients are not replaying the whole messages, so it
is possible that the light clients' behaviour differs from the full nodes'. In this case, the
equivocation proof which proves the divergence between the `ValidityPredicate` and the full node will be
generated and submitted to the chain so that the chain can safely deactivate the
light client.

## Technical Specification

### Data Structures

#### ValidityPredicateBase

`ValidityPredicateBase` is the data that is being used by the `ValidityPredicate`s. The exact
type is dependent on the type of `ValidityPredicate`. The `ValidityPredicateBase`s SHOULD
have a function `Height() int64`. The function returns the height of the
`ValidityPredicateBase`. A `ValidityPredicateBase` can verify a `Header` only if it has a
higher height than itself. `ValidityPredicateBase`s have to be generated from `Consensus`,
which assigns unique heights for each `ValidityPredicateBase`. Two `ValidityPredicateBase`
on a same chain SHOULD NOT have same height, if not equal. Such event is called an
"equivocation", and the proof for it can be generated and submitted(see Subprotocols-Freeze).

```typescript
type ValidityPredicateBase interface {
  Height() int64
}
```

#### ConsensusState

`ConsensusState` is a blockchain commit which contains a `CommitmentRoot` and the requisite
state to verify future roots. The `ConsensusState` of a chain is stored by other chains in order to verify the state of this chain. It is defined as:

```typescript
interface ConsensusState {
  roots: Map<uint64, CommitmentRoot>
}
```
where
  * `Base` is a data used by `Consensus.ValidityPredicate` to verify `Header`s.
  * `Root` is the `CommitmentRoot`, used to prove internal state.

`ValidityPredicateBase` is defined dependently on `ConsensusState`.

#### Header

`Header` is a blockchain header which provides information to update a `ConsensusState`,
submitted to one blockchain to update the stored `ConsensusState`. Defined as

```typescript
interface Header {
  height: uint64
  proof: HeaderProof
  base: Maybe[ValidityPredicateBase]
  root: CommitmentRoot
}
```
where
  * `Proof` is the commit proof used by `Consensus.ValidityPredicate` to be verified.
  * `Base` is the new verify, if it needs to be updated.
  * `Root` is the new `CommitmentRoot` which will replace the existing one.

#### ValidityPredicate

`ValidityPredicate` is a light client ValidityPredicate proving `Header` depending on the `Commit`.
Using the ValidityPredicate SHOULD be far more computationally efficient than replaying `Consensus` logic
for the given parent `Header` and the list of network messages, ideally in constant time
independent from the size of message stored in the `Header`. Defined as

```typescript
type ValidityPredicate = (ConsensusState, Header) => Error | ConsensusState
```

The detailed specification of `ValidityPredicate` is defined in [ValidityPredicate.md](./ValidityPredicate.md)

#### LightClient

LightClient is defined as
```typescript
interface LightClient {
  validityPredicate: ValidityPredicate
  consensusState: ConsensusState
}
```
where
  * `ConsensusState` is the root of trust providing the `ValidityPredicateBase`.
  * `ValidityPredicate` is the lightclient verification logic.

The exact type of each fields are depending on the type of the actual consensus logic.

### Subprotocols

The chains MUST implement the functions defined below ending with `Client`, as they form
the `handleDatagram`.

#### Preliminaries

`newID` is a function which generates a new `Identifier` for a `Client`.
The generation of the `Identifier` MAY depend on the `Header` of the `Client` that will be
registered under that `Identifier`. The behaviour of `newID` is implementation specific.
Possible implementations are:

* Random bytestring.
* Hash of the `Header`.
* Incrementing integer index

`newID` MUST NOT return an `Identifier` which has already been generated.

`storekey` takes an `Identifier` and returns a `KVStore` compatible `Key`.

```typescript
function storekey(id: Identifier) {
  return "clients/{id}"
}
```

`freezekey` takes an `Identifier` and returns a `KVStore` compatible `Key`.

```typescript
function freezekey(id: Identifier) {
  return "clients/{id}/freeze"
}
```

#### Create

Creating a new `LightClient` is done simply by submitting it to the `createClient` function,
as the chain automatically generates the `Identifier` for the `LightClient`.

```typescript
function createClient(info: LightClient) {
  id = newID()
  set(storekey(id), info)
  set(freezekey(id), false)
  return id
}
```

#### Query

Clients can be queried by their identifier.

```typescript
function queryClient(id: Identifier) {
  if get(freezekey(id)) then
    return nil
  else
    return get(storekey(id))
}
```

#### Update

Updating `LightClient` is done by submitting a new `Header`. The `Identifier` is used to point the
stored `LightClient` that the logic will update. When the new `Header` is verified with
the stored `LightClient`'s `ValidityPredicate` and `ConsensusState`, then it SHOULD update the
`LightClient` unless an additional logic intentionally blocks the updating process (e.g.
waiting for the equivocation proof period.

```typescript
function updateClient(id: Identifier, header: Header) {
  assert(!freezekey(id))

  stored = get(storekey(id))
  assert(stored /= nil)

  state = stored.ConsensusState
  pred = stored.ValidityPredicate

  assert(pred(state, header))

  state.Root = header.Root

  if header.Base !== null
    state.Base = header.Base

  set(storekey(id), {state, pred})

  return nil
}
```

#### Freeze

A client can be frozen, in case when the application logic decided that there was a malicious
activity on the client. Frozen client SHOULD NOT be deleted from the state, as a recovery
method can be introduced in the future versions.

```typescript
function freezeClient(id: Identifier, header1: Header, header2: Header) {
  assert(!get(freezekey(id)))
  stored = get(storekey(id))
  assert(stored /= nil)
  set(freezekey(id))
}
```

#### Delete

Deletes the stored client, when the client is no longer needed or no longer valid, as
determined by the application logic.

```typescript
function deleteClient(id: Identifier) {
  assert(get(storekey(id)) /= nil)
  assert(get(freezekey(id)))
  delete(storekey(id))
  return nil
}
```

### Example Implementation

An example blockchain `Op` runs on a single `Op`erator consensus algorithm,
where the valid blocks are signed by the operator. The operator signing Key
can be changed while the chain is running.

`Op` is constructed from the followings:
* `OpValidityPredicateBase`: Operator pubkey
* `OpValidityPredicate`: Signature ValidityPredicate
* `OpCommitmentRoot`: KVStore Merkle root
* `OpHeaderProof`: Operator signature

#### Consensus

`B` is defined as `(Op, Gen, [H])`. `B` satisfies `Blockchain`:

```typescript
type TX = RegisterLightClient | UpdateLightClient | ChangeOperator(Pubkey)

function commit(cs: State, txs: [TX]): H {
  newpubkey = c.Pubkey

  for (const tx of txs)
    switch tx {
      case RegisterLightClient:
        register(tx)
      case UpdateLightClient:
        update(tx)
      case ChangeOperator(pubkey):
        newpubkey = pubkey
    }

  root = getMerkleRoot()
  result = H(_, newpubkey, root)
  result.Sig = Privkey.Sign(result)
  return result
}

function verify(rot: CS, h: H): bool {
  return rot.Pubkey.VerifySignature(h.Sig)
}

type Op = (commit, verify)

type Gen = CS(InitialPubkey, EmptyLogStore)
```

The `[H]` is generated by `Op.commit`, recursively applied on the genesis and its successors.
The `[TX]` applied on the `Op.commit` can be any value, but when the `B` is instantiated
in the real world, the `[TX]` is fixed to a single value to satisfy consensus properties. In
this example, we assume that it is enforced by a legal authority.

#### ConsensusState

Type `CS` is defined as `(Pubkey, LogStore)`. `CS` satisfies `ConsensusState`:

```
function CS.base() returns CS.Pubkey
function CS.root() returns CS.LogStore
```

#### Header

Type `H` is defined as `(Sig, Maybe<Pubkey>, LogStore)`. `H` satisfies `Header`:

```
function proof(header: H): Signature {
  return H.Sig
}

function base(header: H): PubKey {
  return H.PubKey
}

function root(header: H): LogStore {
  return H.LogStore
}
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

In a future version, this ICS will define a new function `unfreezeClient` that can be called 
when the application logic resolves an equivocation event.

## History

March 5th 2019: Initial draft finished and submitted as a PR.
May 29 2019: Various revisions, notably multiple commitment-roots
