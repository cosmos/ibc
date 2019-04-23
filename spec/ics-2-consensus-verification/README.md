---

ics: 2
title: Consensus Verification
stage: draft
category: ibc-core
requires: 23
required-by: 3
author: Juwoon Yun <joon@tendermint.com>, Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-25
modified: 2019-04-02

---

## Synopsis

This standard specifies the properties that consensus algorithms of chains implementing IBC are 
expected to satisfy. The properties are needed for efficient and safe verification in the higher
level protocol abstractions. The algorithm which uses these properties to verify substates of 
another chain is referred to as a "light client".

## Specification

### Motivation

`FullNode`s are procedures running a `Consensus`. Given a `([Transaction], Commit)`, a 
`FullNode` can compute the result `ConsensusState` that the `Consensus` is expected to commit on 
with the same `[Transaction]`, if exists. 

`Blockchain` defines required properties of the blockchain on the network. The implementors can 
check whether the consensus that they are using is qualified to be connected to the network or 
not. If not, they can modify the algorithm or wrap it with additional logic to make it 
compatible with the specification. It also provides base layer for the protocol that the other 
components can rely on.

### Desired Properties

This standard specification provides secure layer to verify other chains' canonical headers, 
using the existing `ConsensusState`. The higher level logics can be able to verify the substate 
with the `AccumulatorRoot` stored in the `ConsensusState`, which is guaranteed to be committed by 
the other chain's consensus algorithm.

* Blockchains, defined as an infinite list of `Header` starting from a genesis `ConsensusState`, 
are linear; no conflicting `Header`s can be both validated, thus no past accumulator roots can 
be changed after they have been committed. Two `Header`s are conflicting when they both have the
same height in a blockchain but are not equal.

* Verifiers can verify future `Header`s using an existing `ConsensusState`. When the verifier 
validates it, the verified header is in the canonical blockchain.

* `ConsensusState`s contains an accumulator root (ICS23) that the downstream logic can use to 
verify whether key-value pairs are present in the state or not.

### Technical Specification

#### Definitions

##### ConsensusState

`ConsensusState` is a blockchain commit which contains an accumulator root and the requisite 
state to verify future roots, stored in one blockchain to verify the state of the other.
Defined as 

```go
type ConsensusState struct {
  Base VerifierBase
  Root AccumulatorRoot
} 
``` 
where
  * `Base` is a data used by `Consensus.Verifier` to verify `Header`s 
  * `Root` is the `AccumuatorRoot`, used to prove internal state

##### Header

`Header` is a blockchain header which provides information to update `ConsensusState`, 
submitted to one blockchain to update the stored `ConsensusState`.
Defined as 

```go
type Header struct {
  Proof HeaderProof
  Base Maybe[VerifierBase]
  Root AccumulatorRoot
}
```
where
  * `Proof` is the commit proof used by `Consensus.Verifier` to be verified
  * `Base` is the new verify, if needed to be updated
  * `Root` is the new `AccumulatorRoot` which will replace the existing one

##### Consensus

`Consensus` is a blockchain consensus algorithm which generates valid `Header`s.
It is defined as commit function which generates a list of headers starting from 
a genesis `ConsensusState` with arbitrary messages. Defined as 

```go
type Consensus func(ConsensusState, [Message]) Header
```

A `Consensus` is expected to satisfy the followings:

1. The `Header`s have no more than one direct child
     
* Satisfied if: deterministic safety
* Possible violation scenario: validator double signing, chain reorganization (Nakamoto consensus)

2. The `Header`s eventually have at least one direct child

* Satisfied if: liveness, light-client verifier continuity
* Possible violation scenario: synchronised halt, incompatible hard fork

3. The `Header`s are generated from the `Consensus`, which ensures valid transition of the state

* Satisfied if: correct block generation & state machine
* Possible violation scenario: invariant break, validator cartel

##### Verifier

`Verifier` is a light client verifier proving `Header` depending on the `Consensus`.
It is expected to prove far more efficiently than replaying `Consensus` logic 
for the given parent `Header` and the list of network messages, idealy in O(1) time. 
Defined as

```go
type Verifier func(ConsensusState, Header) (Error|ConsensusState)
```

##### Blockchain

// XXX: is it okay to be placed in the protocol spec?

`Blockchain` is defined as
```go
type Blockchain struct {
  Consensus ConsensusState
  Verifier Verifiers
  Genesis ConsensusState
  Headers []Header
}
```
where
  * `Consensus` is the Consensus algorithm that is running the blockchain
  * `Verifier` is the lightclient verifier
  * `Genesis` is the genesis `ConsensusState`
  * `Headers` is the list of header generated by `c`, starting from the `gen`. In detail, 
    `B.hs` is defined as `B.hs = fold(B.c, zip(B.gen:B.hs, msgs))` for arbitrary `msgs` 
    in `[Message]` 
    // XXX: make it readable

##### Functions

* `Height :: Blockchain -> Header -> Uint` returns the position of the header in the
  Blockchain's header list.

#### Requirements

Consensus verification requires the following accumulator primitives with datastructures and
properties as defined in ICS23:

* `AccumulatorRoot`

#### Subprotocols

##### Register

##### Update

* `update` is a helper function using `Consensus.verify` to update the existing 
  `ConsensusState`, defined as

```go
function update(cons :: Consensus, rot :: ConsensusState, h :: Header) returns (Error|ConsensusState) {
      if cons.verify(rot, h):
        if h.base != Nothing:
            return ConsensusState(h.base, h.root)
        else:
            return RoofOfTrust(rot.base, h.root)
    else:
        return Error  
   
}
``` 

### Example Implementation

An example blockchain `B` runs on a single operator consensus algorithm, called `Op`. If a 
block is signed by the operator, then it is valid. The operator signing key can be changed while
the chain is running. In that case, the new header stores the updated pubkey. 

`H` contains `LogStore`, which is a list with type of `[bytes]`. The whole state is serialized 
and stored as `AccumulatorRoot`

#### Consensus

`B` is defined as `(Op, Gen, [H])`. `B` satisfies `Blockchain`:

```
TX = Append(bytes) or ChangeOperator(Pubkey)

function commit(h :: H, txs :: [TX]) returns C {
    store := c.AccumulatorRoot
    newpubkey := c.Pubkey

    foreach tx in txs:
        case Append(data): 
            state = state + data
        case ChangeOperator(pubkey): 
            newpubkey = pubkey

    result := H(newpubkey, store)
    sig := Privkey.Sign(result)
    return C(sig, result)
}

function verify(rot :: CS, h :: H) returns rot.Pubkey.VerifySignature(h.Sig)

Op = (commit, verify)

Gen = CS(InitialPubkey, EmptyLogStore)
```

The `[H]` is generated by `Op.commit`, recursivly applied on the genesis and its successors.
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
