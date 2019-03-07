---
ics: 11
title: State Optimizations & Pruning
stage: proposal
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-03-07
---

## Synopsis

(high-level description of and rationale for specification)

## Specification

(main part of standard document - not all subsections are required)

### Motivation

(rationale for existence of standard)

### Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

### Technical Specification

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

While we clean up the _send queue_ upon getting a receipt, if left to run indefinitely, the _receipt queues_ could grow without limit and create a major storage cost for the chains. However, we must not delete receipts until they have been proven to be processed by the sending chain, or we lose important information and sacrifice reliability.

Additionally, with the above timeout implementation, when we perform the timeout on the sending chain, we do not update the _receipt queue_ on the receiving chain, and now it is blocked waiting for a packet `i`, which no longer exists on the sending chain. We can update the guarantees of the receipt queue as follows to allow us to handle both:

`B:M_kvh == ∅` if packet `i` was not received before height `h`

`B:M_kvh == ∅` if packet i was provably resolved on the sending chain before height `h`

`B:M_kvh /= ∅` otherwise (if packet `i` was processed before height `h` but chain `A` has not handled the receipt)

Consider a connection where many messages have been sent, and their receipts processed on the sending chain, either explicitly or through a timeout. We wish to quickly advance over all the processed messages, either for a normal cleanup, or to prepare the queue for normal use again after timeouts.

Through the definition of the send queue, we know that all packets `i < head` have been fully processed and all packets `head <= i < tail` are awaiting processing. By proving a much advanced `head` of `outgoing_B`, we can demonstrate that the sending chain already handled all messages. Thus, we can safely advance `incoming_A` to the new head of `outgoing_B`.

```
cleanup(A, M_kvh, head) = case
  incoming_A == ∅ => fail with "unknown sender"
  H_h ∉ T_B => fail with "must submit header for height h"
  not valid(H_h, M_kvh, head) => fail with "invalid Merkle proof of outgoing_B queue height"
  head >= head(incoming_A) => fail with "cleanup must go forward"
  otherwise =>
    advance(incoming_A, head)
```

This allows us to invoke the `cleanup` function to resolve all outstanding messages up to and including `index` with one Merkle proof. Note that if this handles both recovering from a blocked queue after timeouts, as well as a routine cleanup method to recover space. In the cleanup scenario, we assume that there may also be a number of packets that have been processed by the receiving chain, but not yet posted to the sending chain, `tail(incoming_B) > head(outgoing_A)`. As such, `advance` must not modify any packets between the head and the tail.

### Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

### Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

### Example Implementation

(link to or description of concrete example implementation)

### Other Implementations

(links to or descriptions of other implementations)

## History

(changelog and notable inspirations / references)

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
