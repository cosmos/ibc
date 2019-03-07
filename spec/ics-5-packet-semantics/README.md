---
ics: 5
title: Packet Semantics & Handling
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

#### Definitions

We define an IBC *packet* `P` as the five-tuple `(type, sequence, source, destination, data)`, where:

`type` is an opaque routing field

`sequence` is an unsigned, arbitrary-precision integer

`source` is a string uniquely identifying the chain, connection, and channel from which this packet was sent

`destination` is a string uniquely identifying the chain, connection, and channel which should receive this packet

`data` is an opaque application payload

#### Sending Packets

To send an IBC packet, an application module on the source chain must call the send method of the IBC module, providing a packet as defined above. The IBC module must ensure that the destination chain was already properly registered and that the calling module has permission to write this packet. If all is in order, the IBC module simply pushes the packet to the tail of `outgoing_a`, which enables all the proofs described above.

The packet must provide routing information in the `type` field, so that different modules can write different kinds of packets and maintain any application-level invariants related to this area. For example, a "coin" module can ensure a fixed supply, or a "NFT" module can ensure token uniqueness. The IBC module on the destination chain must associate every supported packet type with a particular handler (`f_type`).

To send an IBC packet from blockchain `A` to blockchain `B`:

`send(P{type, sequence, source, destination, data}) ⇒ success | failure`

```
case
  source /= (A, connection, channel) ⇒ fail with "wrong sender"
  sequence /= tail(outgoing_A) ⇒ fail with "wrong sequence"
  otherwise ⇒
    push(outgoing_A, P)
    success
```

Note that the `sequence`, `source`, and `destination` can all be encoded in the Merkle tree key for the channel and do not need to be stored individually in each packet.

#### Receiving Packets

Upon packet receipt, chain `B` must check that the packet is valid, that it was intended for the destination, and that all previous packets have been processed. `receive` must write the receipt queue upon accepting a valid packet regardless of the result of handler execution so that future packets can be processed.

To receive an IBC packet on blockchain `B` from a source chain `A`, with a Merkle proof `M_kvh` and the current set of trusted headers for that chain `T_A`:

`receive(P{type, sequence, source, destination, data}, M_kvh) ⇒ success | failure`

```
case
  incoming_B == nil ⇒ fail with "unregistered sender"
  destination /= (B, connection, channel) ⇒ fail with "wrong destination"
  sequence /= head(Incoming_B) ⇒ fail with "out of order"
  H_h not in T_A ⇒ fail with "must submit header for height h"
  valid(H_h, M_kvh) == false ⇒ fail with "invalid Merkle proof"
  otherwise ⇒
    set result = f_type(data)
    push(incoming_B, R{tail(incoming_B), (B, connection, channel), (A, connection, channel), result})
    success
```

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
