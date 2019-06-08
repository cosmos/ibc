---
ics: 18
title: Relayer Algorithms
stage: draft
category: ibc-core
requires: 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-05-11
---

## Synopsis

Relayer algorithms are the "physical" connection layer of IBC — off-chain processes responsible for relaying data between two chains running the IBC protocol by scanning the state of each chain, constructing appropriate datagrams, and executing them on the opposite chain as allowed by the protocol.

### Motivation

In the IBC protocol, a blockchain can only record the *intention* to send particular data to another chain — it does not have direct access to a network transport layer. Physical datagram relay must be performed by off-chain infrastructure with access to a transport layer such as TCP/IP. This standard defines the concept of a *relayer* algorithm, executable by an off-chain process with the ability to query chain state, to perform this relay.

### Definitions

A *relayer* is an off-chain process with the ability to read the state of and submit transactions to some set of ledgers utilizing the IBC protocol.

### Desired Properties

- No safety properties of IBC should depend on relayer behaviour (assume Byzantine relayers).
- Liveness properties of IBC should depend only on the existence of at least one correct, live relayer.
- Relaying should be permissionless, all requisite verification should be performed on-chain.
- Requisite communication between the IBC user and the relayer should be minimized.
- Provision for relayer incentivization should be possible at the application layer.

## Technical Specification

### Relayer Algorithm

The relayer algorithm is defined over a set `C` of chains implementing the IBC protocol. Each relayer may not necessarily have access to read state from and write datagrams to all chains in the interchain network (especially in the case of permissioned or private chains) — different relayers may relay between different subsets.

`pendingDatagrams` calculates the set of all valid datagrams to be relayed from one chain to another based on the state of both chains. Subcomponents of this function are defined in individual ICSs. The relayer must possess prior knowledge of what subset of the IBC protocol is implemented by the blockchains in the set for which they are relaying (e.g. by reading the source code).

`submitDatagram` is a procedure defined per-chain (submitting a transaction of some sort).

```typescript
function relay(C: Set<Chain>) {
  for (const chain of C)
    for (const counterparty of C)
      if (counterparty !== chain) {
        const datagrams = pendingDatagrams(chain, counterparty)
        for (const datagram of datagrams)
          submitDatagram(counterparty, datagram)
      }
}
```

### Incentivization

The relay process must have access to accounts on both chains with sufficient balance to pay for transaction fees. Relayers may employ application-level methods to recoup these fees, such by including a small payment to themselves in the packet data — protocols for relayer fee payment will be described in future versions of this ICS or in separate ICSs.

Any number of relayer processes may be safely run in parallel (and indeed, it is expected that separate relayers will serve separate subsets of the interchain). However, they may consume unnecessary fees if they submit the same proof multiple times, so some minimal coordination may be ideal (such as assigning particular relayers to particular packets or scanning mempools for pending transactions).

## Backwards Compatibility

Not applicable. The relayer process is off-chain and can be upgraded or downgraded as necessary.

## Forwards Compatibility

Not applicable. The relayer process is off-chain and can be upgraded or downgraded as necessary.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

30 March 2019 - Initial draft submitted
15 April 2019 - Revisions for formatting and clarity
23 April 2019 - Revisions from comments; draft merged

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
