---

ics: 2
title: Consensus Verification
stage: draft
category: ibc-core
requires: 23, 24
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-04-29

---

## Synopsis

This standard specifies the properties that consensus algorithms of chains implementing IBC are
expected to satisfy. These properties are needed for efficient and safe verification in the higher
level protocol abstractions. The algorithm which uses these properties to verify substates of
another chain is referred to as a "light client verifier", and pairing it with a state that the
verifier trusts forms a "light client".

This standard also specifies how the light clients will be stored, registered, and updated on a
blockchain. The stored light client instances will be able to be verified by a third party actor, such as a user inspecting the state of the chain.

## Specification

### Motivation

In the IBC protocol, a chain needs to be able to verify updates to the state of another chain. A light client is the algorithm with which they can do so.
This standard formalizes the common
model of light client to minimise the dependency on consensus algorithms, so that the protocol can
easily connect with new chains which are running new consensus algorithms, without the need to
upgrade the light client protocol itself.

### Desired Properties

Light clients must provide a secure algorithm to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level abstractions will then be able to verify subcomponents of the state
with the `CommitmentRoot` stored in the `ConsensusState`, which is guaranteed to be committed by
the other chain's consensus algorithm.

* `ValidityPredicate`s are expected to reflect the behaviour of the full node which is running the  
corresponding consensus algorithm. Given a `ConsensusState` and `[Message]`, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects it, then the light client MUST also reject it. The consensus algorithm
ensures this correspondence. However light clients are not replaying the whole messages, so it
is possible that the light clients' behaviour differs from the full nodes'. In this case, the
equivocation proof which proves the divergence between the `ValidityPredicate` and the full node will be
generated and submitted to the chain, as defined in
[ICS ?](https://github.com/cosmos/ics/issues/53), so that the chain can safely deactivate the
light client.

### Technical Specification

#### Requirements

* `get`, `set`, `Key`, and `Identifier` are as defined in [ICS24](../ics-24-host-requirements).
are used by the datagram handler.

* `CommitmentRoot` is as defined in [ICS23](https://github.com/cosmos/ics/pull/74).
`ConsensusState`. The downstream logic can use it to verify whether key-value pairs are present
in the state or not.

* `createClient`, `queryClient`, `updateClient`, `freezeClient`, `deleteClient` are as
defined in [ICS25](https://github.com/cosmos/ics/pull/79).

#### Definitions

##### ValidityPredicateBase

`ValidityPredicateBase` is the data that is being used by the `ValidityPredicate`s. The exact
type is dependent on the type of `ValidityPredicate`. The `ValidityPredicateBase`s SHOULD
have a function `Height() int64`. The function returns the height of the
`ValidityPredicateBase`. A `ValidityPredicateBase` can verify a `Header` only if it has a
higher height than itself. `ValidityPredicateBase`s have to be generated from `Consensus`,
which assigns unique heights for each `ValidityPredicateBase`. Two `ValidityPredicateBase`
on a same chain SHOULD NOT have same height, if not equal. Such event is called an
"equivocation", and the proof for it can be generated and submitted(see Subprotocols-Freeze).

```go
type ValidityPredicateBase interface {
  Height() int64
}
```

##### ConsensusState

`ConsensusState` is a blockchain commit which contains a `CommitmentRoot` and the requisite
state to verify future roots. The `ConsensusState` of a chain is stored by other chains in order to verify the state of this chain. It is defined as:
Defined as

```go
type ConsensusState struct {
  Base ValidityPredicateBase
  Root CommitmentRoot
}
```
where
  * `Base` is a data used by `Consensus.ValidityPredicate` to verify `Header`s.
  * `Root` is the `CommitmentRoot`, used to prove internal state.

`ValidityPredicateBase` is defined dependently on `ConsensusState`.

##### Header

`Header` is a blockchain header which provides information to update a `ConsensusState`,
submitted to one blockchain to update the stored `ConsensusState`.
Defined as

```go
type Header struct {
  Proof HeaderProof
  Base Maybe[ValidityPredicateBase]
  Root CommitmentRoot
}
```
where
  * `Proof` is the commit proof used by `Consensus.ValidityPredicate` to be verified.
  * `Base` is the new verify, if it needs to be updated.
  * `Root` is the new `CommitmentRoot` which will replace the existing one.

##### ValidityPredicate

`ValidityPredicate` is a light client ValidityPredicate proving `Header` depending on the `Commit`.
Using the ValidityPredicate SHOULD be far more computationally efficient than replaying `Consensus` logic
for the given parent `Header` and the list of network messages, ideally in O(1) time.
Defined as

```go
type ValidityPredicate func(ConsensusState, Header) (Error|ConsensusState)
```

The detailed specification of `ValidityPredicate` is defined in [ValidityPredicate.md](./ValidityPredicate.md)

##### LightClient

LightClient is defined as
```go
type LightClient struct {
  ValidityPredicate ValidityPredicate
  ConsensusState ConsensusState
}
```
where
  * `ConsensusState` is the root of trust providing the `ValidityPredicateBase`.
  * `ValidityPredicate` is the lightclient verification logic.

The exact type of each fields are depending on the type of the actual consensus logic.

#### Subprotocols

The chains MUST implement functions `register` and `update`, as they form the `handleDatagram`.
Calling both functions MAY be permissionless.

##### Preliminaries

`newID` is a function which generates a new `Identifier` for a `LightClient`, which MAY depending
on the `Header`. The behaviour of `newID` is implementation specific. Possible implementations are:

* Random bytestring.
* Hash of the `Header`.
* Incrementing integer index, big-endian encoded.

`newID` MUST NOT return an `Identifier` which has already been generated.

`storekey` takes an `Identifier` and returns a `KVStore` compatible `Key`.

```coffee
function storekey(id)
  return "clients/{id}"
```

`freezekey` takes an `Identifier` and returns a `KVStore` compatible `Key`.

```coffee
function freezekey(id)
  return "clients/{id}/freeze"
```

##### Create

Creating a new `LightClient` is done simply by submitting it to the `createClient` function,
as the chain automatically generates the `Identifier` for the `LightClient`.

```coffee
function createClient(info)
  id = newID()
  set(storekey(id), info)
  set(freezekey(id), false)
  return id
}
```

##### Query

Clients can be queried by their identifier.

```coffee
function queryClient(id)
  if get(freezekey(id)) then
    return nil
  else
    return get(storekey(id))
```

##### Update

Updating `LightClient` is done by submitting a new `Header`. The `Identifier` is used to point the
stored `LightClient` that the logic will update. When the new `Header` is verified with
the stored `LightClient`'s `ValidityPredicate` and `ConsensusState`, then it SHOULD update the
`LightClient` unless an additional logic intentionally blocks the updating process (e.g.
waiting for the equivocation proof period.

```coffee
function updateClient(id, header)
  assert(!freezekey(id))

  stored = get(storekey(id))
  assert(stored /= nil)

  state = stored.ConsensusState
  pred = stored.ValidityPredicate

  assert(pred(state, header))

  state.Root = header.Root
  if header.Base? then state.Base = header.Base

  set(storekey(id), {state, pred})

  return nil
}
```

##### Freeze

A client can be frozen, in case when the application logic decided that there was a malicious
activity on the client. Frozen client SHOULD NOT be deleted from the state, as a recovery
method can be introduced in the future versions.

```coffee
function freezeClient(id, header1, header2)
  assert(!get(freezekey(id)))
  stored = get(storekey(id))
  assert(stored /= nil)
  set(freezekey(id))
```

##### Delete

Deletes the stored client, when the client is no longer needed or no longer valid, as
determined by the application logic.

```coffee
function deleteClient(id)
  assert(get(storekey(id)) /= nil)
  assert(get(freezekey(id)))
  del(storekey(id))
  return nil
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

```coffee
TX = RegisterLightClient | UpdateLightClient | ChangeOperator(Pubkey)

function commit(cs :: State, txs :: [TX]) returns H {
  newpubkey := c.Pubkey

  foreach tx in txs:
    case RegisterLightClient:
      register(tx)
    case UpdateLightClient:
      update(tx)
    case ChangeOperator(pubkey):
      newpubkey = pubkey

  root := getMerkleRoot()
  result := H(_, newpubkey, root)
  result.Sig := Privkey.Sign(result)
  return result
}

function verify(rot :: CS, h :: H) returns rot.Pubkey.VerifySignature(h.Sig)

Op = (commit, verify)

Gen = CS(InitialPubkey, EmptyLogStore)
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
function H.proof() returns H.Sig
function H.base() returns H.Pubkey
function H.root() returns H.LogStore
```

## History

March 5th 2019: Initial ICS 2 draft finished and submitted as a PR
