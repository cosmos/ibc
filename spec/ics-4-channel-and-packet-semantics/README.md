---
ics: 4
title: Channel & Packet Semantics
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-05-12
---

# Synopsis

The "channel" abstraction provides message delivery semantics to the interblockchain communication protocol, in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one chain and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination chain. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortizing the cost of header verification across all the channels utilizing a connection & light client.

Channels are payload-agnostic. The module which receives an IBC packet on chain `B` decides how to act upon the incoming data, and must utilize its own application logic to determine which state transactions to apply according to what data the packet contains. Both chains must only agree that the packet has been received.

# Specification

## Motivation

The interblockchain communication protocol uses a cross-chain message passing model which makes no assumptions about network synchrony. IBC *packets* are relayed from one blockchain to the other by external relayer processes. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed, censored, or re-ordered arbitrarily. Packets are public and can be read from a blockchain by any relayer and submitted to any other blockchain. In order to provide the desired ordering, exactly-once delivery, and module permissioning semantics to the application layer, the interblockchain communication protocol must implement an abstraction to enforce these semantics — channels are this abstraction.

## Definitions

`Connection` is as defined in ICS 3.

`Commitment`, `CommitmentProof`, and `CommitmentRoot` are as defined in ICS 23.

`Get`, `Set`, and module-system related primitives are as defined in ICS 24.

A *channel* is a pipeline for exactly-once packet delivery between specific modules on separate blockchains, which has at least one send and one receive end.

A *bidirectional* channel is a channel where packets can flow in both directions: from `A` to `B` and from `B` to `A`.

A *unidirectional* channel is a channel where packets can only flow in one direction: from `A` to `B`.

```golang
enum Direction {
  Tx
  Rx
  TxRx
}
```

An *end* of a channel is a data structure on one chain storing channel metadata:

```golang
struct ChannelEnd {
  name            string
  direction       Direction
  lastTxSequence  uint64
  lastRxSequence  uint64
  rxCommitment    CommitmentRoot
}
```

Certain fields may be omitted depending on the direction of the end.

An *ordered* channel is a channel where packets are delivered exactly in the order which they were sent.

An *unordered* channel is a channel where packets can be delivered in any order, which may differ from the order in which they were sent.

Directionality and ordering are independent, so one can speak of a bidirectional unordered channel, a unidirectional ordered channel, etc.

All channels provide exactly-once packet delivery.

An IBC *packet* is a particular datagram, defined as follows:

```golang
struct Packet {
  sequence      uint64
  sourceChannel string
  destChannel   string
  data          bytes
}
```

## Desired Properties

- The speed of packet transmission and confirmation should be limited only by the speed of the underlying chains.
- Exactly-once packet delivery, without assumptions of network synchrony and even if one or both of the chains should halt (no-more-than-once delivery in that case).
- Ordering, for ordered channels, whereby if packet *x* is sent before packet *y* on chain `A`, packet *x* must be received before packet *y* on chain `B`.

### Exactly-once delivery

either ordering or accumulator

### Ordering

IBC channels implement a vector clock[[2](references.md#2)] for the restricted case of two processes (in our case, blockchains). Given *x* → *y* means *x* is causally before *y*, chains `A` and `B`, and *a* ⇒ *b* means *a* implies *b*:

*A:send(msg<sub>i </sub>)* → *B:receive(msg<sub>i </sub>)*

*B:receive(msg<sub>i </sub>)* → *A:receipt(msg<sub>i </sub>)*

*A:send(msg<sub>i </sub>)* → *A:send(msg<sub>i+1 </sub>)*

*x* → *A:send(msg<sub>i </sub>)* ⇒
*x* → *B:receive(msg<sub>i </sub>)*

*y* → *B:receive(msg<sub>i </sub>)* ⇒
*y* → *A:receipt(msg<sub>i </sub>)*

Every transaction on the same chain already has a well-defined causality relation (order in history). IBC provides an ordering guarantee across two chains which can be used to reason about the combined state of both chains as a whole.

For example, an application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

### Permissioning

## Technical Specification

We introduce the abstraction of an IBC *channel*: a set of the required packet queues to facilitate ordered bidirectional communication between two blockchains `A` and `B`. An IBC connection, as defined earlier, can have any number of associated channels. IBC connections handle header initialization & updates. All IBC channels use the same connection, but implement independent queues and thus independent ordering guarantees.

An IBC channel consists of four distinct queues, two on each chain:

`outgoing_A`: Outgoing IBC packets from chain `A` to chain `B`, stored on chain `A`

`incoming_A`: IBC receipts for incoming IBC packets from chain `B`, stored on chain `A`

`outgoing_B`: Outgoing IBC packets from chain `B` to chain `A`, stored on chain `B`

`incoming_B`: IBC receipts for incoming IBC packets from chain `A`, stored on chain `B`

### Sending Packets

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

### Receiving Packets

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

### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves. 

One solution is to include a timeout in the IBC packet itself.  When sending a packet, one can specify a block height or timestamp on chain `B` after which the packet is no longer valid. If the packet is posted before the cutoff, it will be processed normally. If it is posted after the cutoff, it will be a guaranteed error. In order to provide the necessary guarantees, the timeout must be specified relative to a condition on the receiving chain, and the sending chain must have proof of this condition after the cutoff. 

For a sending chain `A` and a receiving chain `B`, with an IBC packet `P={_, i, _, _, _}` and some height `h` on chain `B`, the base IBC protocol provides the following guarantees:

`A:M_kvh == ∅` if message `i` was not sent before height `h` 

`A:M_kvh == ∅` if message `i` was sent and the corresponding receipt received before height `h` (and the receipts for all messages j < i were also handled)

`A:M_kvh /= ∅` otherwise, if message `i` was sent but the receipt has not yet been processed

`B:M_kvh == ∅` if message `i` was not received before height `h` 

`B:M_kvh /= ∅` if message `i` was received before height `h` 

We can make a few modifications of the above protocol to allow us to prove timeouts, by adding some fields to the messages in the send queue and defining an expired function that returns true iff `h > maxHeight` or `timestamp(H_h) > maxTime`.

`P = (type, sequence, source, destination, data, maxHeight, maxTime)`

`expired(H_h, P) ⇒ true | false`

We then update message handling in `receive`, so that chain `B` doesn't even call the handler function if the timeout was reached but instead directly writes an error in the receipt queue:

`receive`

```
case 
  ... 
  expired(latestHeader, v) ⇒ push(incoming_b, R{..., TimeoutError})
  otherwise ⇒
    set result = f_type(data)
    push(incoming_B, R{tail(incoming_B), (B, connection, channel), (A, connection, channel), result})
```

Now chain `A` can rollback all transactions that were blocked by this flood of unrelayed packets - since they can never confirm - without waiting for chain `B` to process them and return a receipt. Adding reasonable timeouts to all packets allows us to gracefully handle any errors with the IBC relay processes or a flood of unrelayed "spam" IBC packets. If a blockchain requires a timeout on all messages and imposes some reasonable upper limit, we can guarantee that if a packet is not processed by the upper limit of the timeout period, then all previous packets must also have either been processed or reached the timeout period. 

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

Additionally, if timestamp-based timeouts are used instead of height-based timeouts, the destination chain's consensus ruleset must enforce always-increasing timestamps (or the sending chain must use a more complex `expired` function).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Data structures & encoding can be versioned at the connection or channel level.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

12 May 2019 - Draft submitted

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
