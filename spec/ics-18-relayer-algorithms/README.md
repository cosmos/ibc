---
ics: 18
title: Relayer Algorithms
stage: draft
category: ibc-misc
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-04-15
---

## Synopsis

Relayer algorithms are the "physical" connection layer of IBC — off-chain processes responsible for relaying data between two chains running the IBC protocol by scanning the state of each chain, constructing appropriate datagrams, and executing them on the opposite chain as allowed by the protocol.

## Specification

### Motivation

In the IBC protocol, a blockchain can only record the *intention* to send particular data to another chain. Physical datagram relay must be performed by off-chain infrastructure. This standard defines the concept of a *relayer* algorithm, executable by an off-chain process with the ability to query chain state, to perform this relay.

### Definitions

A *relayer* is an off-chain process with the ability to read the state of and submit transactions to some set of ledgers utilizing the IBC protocol.

### Desired Properties

- No safety properties of IBC should depend on relayer behavor (assume Byzantine relayers).
- Liveness properties of IBC should depend only on the existence of at least one correct, live relayer.
- Relaying should be permissionless, all requisite verification should be performed on-chain.
- Requisite communication between the IBC user and the relayer should be minimized.
- Provision for relayer incentivization should be possible at the application layer.

### Technical Specification

#### Relayer Algorithm

The relayer algorithm is defined over a set `C` of chains implementing the IBC protocol.

`pendingDatagrams` calculates the set of all valid datagrams to be relayed from one chain to another based on the state of both chains. Subcomponents of this function are defined in individual ICSs. The relayer must possess prior knowledge of what subset of the IBC protocol is implemented by the blockchains in the set for which they are relaying (e.g. by reading the source code).

`submitDatagram` is a procedure defined per-chain (submitting a transaction of some sort).

```coffeescript
function relay(C)
  for chain in C
    for counterparty in C if counterparty != chain
      datagrams = pendingDatagrams(chain, counterparty)
      for datagram in datagrams
        submitDatagram(counterparty, datagram)
```

#### Incentivization

The relay process must have access to accounts on both chains with sufficient balance to pay for transaction fees. Relayers may employ application-level methods to recoup these fees.

Any number of relayer processes may be safely run in parallel (and indeed, it is expected that separate relayers will serve separate subsets of the interchain). However, they may consume unnecessary fees if they submit the same proof multiple times, so some minimal coordination may be ideal.

### Backwards Compatibility

Not applicable. The relayer process is off-chain and can be upgraded or downgraded as necessary.

### Forwards Compatibility

Not applicable. The relayer process is off-chain and can be upgraded or downgraded as necessary.

### Example Implementation

Coming soon.

### Other Implementations

Coming soon.

## History

30 March 2019 - Initial draft submitted
15 April 2019 - Revisions for formatting and clarity

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
