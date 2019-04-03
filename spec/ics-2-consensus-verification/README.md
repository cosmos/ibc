---

ics: 2
title: Consensus Verification
stage: proposal
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
level protocol abstractions. The algorithm who uses these properties to verify substates of 
another chain is referred to as a "light client".

## Specification

### Motivation

`FullNode`s are procedures running a `Consensus`. Given a `([Transaction], Commit)`, a 
`FullNode` can compute the result `RootOfTrust` that the `Consensus` is expected to commit on 
with the same `[Transaction]`, if exists. 



`Header` defines required properties of the blockchain on the network. The implementors can 
check whether the consensus that they are using is qualified to be connected to the network or 
not. If not, they can modify the algorithm or wrap it with additional logic to make it 
compatible with the specification. It also provides base layer for the protocol that the other 
components can rely on.

A lightclient is an algorithm specific to each `Consensus`. 

### Desired Properties

This standard specification provides secure layer to verify other chains' canonical headers, 
using the existing `RootOfTrust`. The higher level logics can be able to verify the substate 
with the `AccumulatorRoot` stored in the `RootOfTrust`, which is guaranteed to be committed by 
the other chain's consensus algorithm.



* Blockchains, defined as an infinite list of `Header` starting from its genesis, is linear; no 
conflicting `Header`s can be both validated, thus no data can be rewritten after it has been 
committed. Two `Header`s are conflicting when both has same height but not equal.

* Verifiers can verify future `Header`s using an existing `RootOfTrust`. When the verifier 
validates it, the verified header is in the canonical blockchain.

* `RootOfTrust`s contains an accumulator root(ICS23) that the other logics can verify whether 
key-value pairs exists or not with it.

### Technical Specification

#### Definitions

* `RootOfTrust` is a blockchain commit which contains an accumulator root and the requisite 
  state to verify future roots, stored in one blockchain to verify the state of the other.
  Defined as 2-tuple `(v :: Header -> (Error|RootOfTrust), r :: AccumulatorRoot)`, where
    * `v` is the verifier, proves child `Header.p` and returns the updated `RootOfTrust`
    * `r` is the `AccumuatorRoot`, used to prove internal state

* `Header` is a blockchain header which provides information to update `RootOfTrust`, 
  submitted to one blockchain to update the stored `RootOfTrust`.
  Defined as 3-tuple `(p :: HeaderProof, v :: Maybe<Header -> (Error|RootOfTrust)>,
  r :: AccumulatorRoot)`, where
    * `p` is the commit proof used by `RootOfTrust.v` to verify
    * `v` is the new verifier, if needed to be updated
    * `r` is the new `AccumulatorRoot` which will replace the existing one
 
* `Consensus` is a blockchain consensus algorithm which generates valid `Header`s.
  Defined as a function `RootOfTrust -> [Transaction] -> Header`

* `Blockchain` is a subset of `(Consensus, RootOfTrust, [Header])`, generated 
by a `Consensus`.

#### Requirements

Consensus verification requires the following accumulator primitives with datastructures and
properties as defined in ICS23:

* `AccumulatorRoot`

#### Subprotocols

##### Verifier

Verifiers prove new `Header` generated from a blockchain. It is expected to prove a `Header` 
efficiently; more efficiently than replaying `Consensus` logic for given parent `Header` and the
transactions. `Header.p` provides the proof that the verifier can use. Verifiers assume the
following properties will be satisfied for the `Header`s:

1. `Header`s have no more than one direct child
 
* Satisfied if: deterministic safety
* Possible violation scenario: validator double signing, deep chain reorg

2. `Header`s have at least one direct child

* Satisfied if: liveness, lightclient verifier continuability
* Possible violation scenario: synchronized halt, incompatible hard fork

3. `Header`s' accumulator root are valid transition from the parents'

* Satisfied if: decentralized block generation, well implemented state machine
* Possible violation scenario: invariant break, validator cartel

// XXX: should it be described on the connection spec?
If a `Header` is submitted to the chain(and optionally passed some challenge period for fraud 
proof), then it is assured that the packet is finalized so the application logic can process it.
If a `Header` is proven to violate one of the properties, but still can be verified, the tracking 
chain should detect and take action on the event to prevent further impact. See (link for the ICS 
for equivocation and fraud proof) for details.

##### Accumulator Root

`RootOfTrust` contains an accumulator root, which identifies the whole state of the 
corresponding blockchain at the point of time that the commit is generated. It is expected that 
the verifying inclusion or exclusion of certain data in the accumulator is done efficient. See 
ICS23 for the details about the `AccumulatorRoot`s.

##### Consensus 

`Consensus` is a blockchain protocol which actually generates a list of `Header` from the latest
state and the incoming transactions. While the chains on the network does not directly proving the 
consensus process, it is expected that the consensus algorithms will generate valid `Header`s.

### Example Implementation

An example blockchain `B` runs on a single operator consensus algorithm, called `Op`. If a 
block is signed by the operator, then it is valid. The operator signing key can be changed whil 
the chain is running. In that case, the new header stores the updated pubkey. 

`H` contains `LogStore`, which is a list with type of `[bytes]`. The whole state is serialized 
and stored as `AccumulatorRoot`.

#### Consensus

`B` is defined as `(Op, Gen, [C])`. `B` satisfies `Blockchain`:

```
TX = Append(bytes) or ChangeOperator(Pubkey)

function Op(c :: C, txs :: [TX]) returns C {
    store := c.AccumulatorRoot
    newpubkey := c.Pubkey
    foreach tx in txs {
        case Append(data): 
            state = state + data
        case ChangeOperator(pubkey): 
            newpubkey = pubkey
    }
    result := H(newpubkey, store)
    sig := Privkey.Sign()
    return C(sig, result)
}

Gen = H(InitialPubkey, EmptyLogStore)
```

It is possible that the `[C]` in a `B` can be any member of set `[C]`, but when the `B` is 
instantiated in the real world, the `[C]` can have only one form. In this example, we assume
that it is enforced by a legal authority.

#### RootOfTrust

Type `R` is defined as `(Pubkey, LogStore)`. `R` satisfies `RootOfTrust`:

```
function R.v(h :: H) returns (Error|R) {
    if h.p().VerifySignature(R.Pubkey) {
        if h.Pubkey != Nothing {
            return R(h.Pubkey, h.LogStore)
        } else {
            return R(R.Pubkey, h.LogStore)
        }
    } else {
        return Error
    }
}

function R.r() returns LogStore {
    return R.LogStore 
}
```

#### Header

Type `H` is defined as `(Sig, Maybe<Pubkey>, LogStore)`. `H` satisfies `Header`:

```
function H.p() returns H.Sig
function H.v() returns H.Pubkey
function H.r() returns H.LogStore
```

## History 

March 5th 2019: Initial ICS 2 draft finished and submitted as a PR
