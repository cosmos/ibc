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
expected to satisfy. The properties are needed for efficient and safe verification in the higher
level protocol abstractions. The algorithm which uses these properties to verify substates of
another chain is referred to as a "light client verifier", and pairing it with a state that the
verifier have to trust forms a "light client".

This standard also specifics how the light clients will be stored, registered, and updated on a
blockchain. The stored lightclient instances will be able to be verified by a thrid party actor.

## Specification

### Motivation

Light clients are the verification method of IBC protocol. One chain can track another chain's
updating state with a light client pointing to that chain. This standard formalises the common
model of light client to minimise the dependency on consensus algorithms, so the protocol can
easily connect with new chains which are running new consensus algorithms, without need to
upgrade the light client protocol itself.

<!--
`FullNode`s are procedures running a `Consensus`. Given a `([Transaction], Commit)`, a
`FullNode` can compute the result `ConsensusState` that the `Consensus` is expected to commit on
with the same `[Transaction]`, if exists.
--->
### Desired Properties

This standard specification provides secure layer to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level logics can be able to verify the substate
with the `CommitmentRoot` stored in the `ConsensusState`, which is guaranteed to be committed by
the other chain's consensus algorithm.

* `Verifier`s are expected to reflect the behaviour of the full node which is running on the  
corresponding consensus algorithm. Given a `ConsensusState` and `[Message]`, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects, then the light client MUST also reject. The consensus algorithm
ensures this correspondence. However light clients are not replaying the whole messages, so it
is possible that the light clients' behaviour differs from the full nodes'. In this case, the
equivocation proof which proves the divergence between the `Verifier` and the full node will be
generated and submitted to the chain, as defined in
[ICS ?](https://github.com/cosmos/ics/issues/53), so the chain can safely deactivate the
light client.

### Technical Specification

#### Requirements

* `get`, `set`, `Key`, `Identifier`, as defined in [ICS24](https://github.com/cosmos/ics/pull/75),
are used by the datagram handler.

* `CommitmentRoot`, as defined in [ICS23](https://github.com/cosmos/ics/pull/74), is used by
`ConsensusState`. The downstream logic can use it to verify whether key-value pairs are present
in the state or not.

#### Definitions

##### ConsensusState

`ConsensusState` is a blockchain commit which contains an `CommitmentRoot` and the requisite
state to verify future roots, stored in one blockchain to verify the state of the other.
Defined as

```go
type ConsensusState struct {
  Base VerifierBase
  Root CommitmentRoot
}
```
where
  * `Base` is a data used by `Consensus.Verifier` to verify `Header`s
  * `Root` is the `CommitmentRoot`, used to prove internal state

##### Header

`Header` is a blockchain header which provides information to update `ConsensusState`,
submitted to one blockchain to update the stored `ConsensusState`.
Defined as

```go
type Header struct {
  Proof HeaderProof
  Base Maybe[VerifierBase]
  Root CommitmentRoot
}
```
where
  * `Proof` is the commit proof used by `Consensus.Verifier` to be verified
  * `Base` is the new verify, if needed to be updated
  * `Root` is the new `CommitmentRoot` which will replace the existing one

##### Verifier

`Verifier` is a light client verifier proving `Header` depending on the `Commit`.
It SHOULD prove far more efficiently than replaying `Consensus` logic
for the given parent `Header` and the list of network messages, ideally in O(1) time.
Defined as

```go
type Verifier func(ConsensusState, Header) (Error|ConsensusState)
```

The detailed specification of `Verifier` is defined in [VERIFIER.md](./VERIFIER.md)

##### LightClient

LightClient is defined as
```go
type LightClient struct {
  Verifier Verifier
  ConsensusState ConsensusState
}
```
where
  * `ConsensusState` is the root of trust providing the `VerifierBase`
  * `Verifier` is the lightclient verification logic

The exact type of each fields are depending on the type of the actual consensus logic.

#### Subprotocols

The chains MUST implement function `register` and `update`, as they form the `handleDatagram`.
Calling both functions MAY be permissionless.

##### Preliminaries

`newID` is a function which generates a new `Identifier` for a `LightClient`, which MAY depending
on the `Header`. The behaviour of `newID` is implementation specific. Possible implementations are:

* Random bytestring
* Hash of the `Header`
* Incrementing integer index, bigendian encoded

`newID` MUST NOT return an `Identifier` which has already been generated, so it can be stateful to check
the `Identifier`

`storekey` takes an `Identifier` and returns a `KVStore` compatible `Key`.

```coffee
newID = (header) -> # impl specific
storekey = (id) -> # impl specific
```

##### Register

Registering new `LightClient` is done simply by submitting it to the `register` function,
as the chain automatically generates the `Identifier` for the `LightClient`.

```coffee
register = (lightclient) ->
  id = newID()
  set(storekey(id), register)
  id
}
```

##### Update

Updating `LightClient` is done by submitting a new `Header`. The `Identifier` is used to point the
stored `LightClient` that the logic will update. When the new `Header` is verifiable with
the stored `LightClient`'s `Verifier` and `ConsensusState`, then it SHOULD update the
`LightClient` unless an additional logic intentionally blocks the updating process, for example,
waiting for the equivocation proof period.

```coffee
update = (id, header) ->
  stored = get(storekey(id))
  state = stored.ConsensusState
  verifier = stored.Verifier

  assert(verifier(state, header))

  state.Root = header.Root
  if header.Base? then state.Base = header.Base

  setLightClient(update.ID, {state, verifier})

  return nil
}
```

<!--

### Example Implementation

An example blockchain `B` runs on a single operator consensus algorithm, called `Op`. If a
block is signed by the operator, then it is valid. The operator signing key can be changed while
the chain is running. In that case, the new header stores the updated pubkey.

`H` contains a `KVStoreRoot`. The internal `KVStore`'s Merkle root is stored as the `KVStoreRoot`.

#### Consensus

`B` is defined as `(Op, Gen, [H])`. `B` satisfies `Blockchain`:

```
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

-->

## History

March 5th 2019: Initial ICS 2 draft finished and submitted as a PR
