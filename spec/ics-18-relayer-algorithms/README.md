---
ics: 18
title: Relayer Algorithms
stage: proposal
category: ibc-misc
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-03-30
---

## Synopsis

Relayer algorithms are the "physical" connection layer of IBC — off-chain processes responsible for relaying data between two chains running the IBC protocol by scanning the state of each chain, constructing appropriate datagrams, and executing them on the opposite chain as allowed by the protocol.

## Specification

### Motivation

- IBC needs physical layer
- Describe algorithm for implementors

### Desired Properties

- No safety properties of IBC should depend on relayer behavor (assume Byzantine relayers).
- Liveness properties of IBC should depend only on the existence of at least one correct, live relayer.
- Relaying should be permissionless, all requisite verification should be performed on-chain.
- Requisite communication between the IBC user and the relayer should be minimized.
- Provision for relayer incentivization at the application layer should be considered.

### Technical Specification

(rewrite this)

The blockchain itself only records the *intention* to send the given message to the recipient chain. Physical network packet relay must be performed by off-chain infrastructure. We define the concept of a *relay* process that connects two chains by querying one for all outgoing packets & proofs, then committing those packets & proofs to the recipient chain.

The relay process must have access to accounts on both chains with sufficient balance to pay for transaction fees but needs no other permissions. Relayers may employ application-level methods to recoup these fees. Any number of *relay* processes may be safely run in parallel. However, they will consume unnecessary fees if they submit the same proof multiple times, so some minimal coordination is ideal.

As an example, here is a naive algorithm for relaying outgoing packets from `A` to `B` and incoming receipts from `B` back to `A`. All reads of variables belonging to a chain imply queries and all function calls imply submitting a transaction to the blockchain.

```
while true
   set pending = tail(outgoing_A)
   set received = tail(incoming_B)
   if pending > received
       set U_h = A.latestHeader
       if U_h /= B.knownHeaderA
          B.updateHeader(U_h)
       for i from received to pending
           set P = outgoing_A[i]
           set M_kvh = A.prove(U_h, P)
           B.receive(P, M_kvh)
```

Note that updating a header is a costly transaction compared to posting a Merkle proof for a known header. Thus, a process could wait until many messages are pending, then submit one header along with multiple Merkle proofs, rather than a separate header for each message. This decreases total computation cost (and fees) at the price of additional latency and is a trade-off each relay can dynamically adjust.

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

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
