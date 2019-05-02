---
ics: 4
title: Channel Semantics
stage: proposal
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-05-02
---

# Synopsis

IBC uses a cross-chain message passing model that makes no assumptions about network synchrony. IBC *data packets* (hereafter just *packets*) are relayed from one blockchain to the other by external infrastructure. Chain `A` and chain `B` confirm new blocks independently, and packets from one chain to the other may be delayed or censored arbitrarily. The speed of packet transmission and confirmation is limited only by the speed of the underlying chains.

# Specification

(main part of standard document - not all subsections are required)

## Motivation

The IBC protocol as defined here is payload-agnostic. The packet receiver on chain `B` decides how to act upon the incoming message, and may add its own application logic to determine which state transactions to apply according to what data the packet contains. Both chains must only agree that the packet has been received.

To facilitate useful application logic, we introduce an IBC *channel*: a set of reliable messaging queues that allows us to guarantee a cross-chain causal ordering[[5](./references.md#5)] of IBC packets. Causal ordering means that if packet *x* is processed before packet *y* on chain `A`, packet *x* must also be processed before packet *y* on chain `B`.

## Definitions

## Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

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

## Technical Specification

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

We introduce the abstraction of an IBC *channel*: a set of the required packet queues to facilitate ordered bidirectional communication between two blockchains `A` and `B`. An IBC connection, as defined earlier, can have any number of associated channels. IBC connections handle header initialization & updates. All IBC channels use the same connection, but implement independent queues and thus independent ordering guarantees.

An IBC channel consists of four distinct queues, two on each chain:

`outgoing_A`: Outgoing IBC packets from chain `A` to chain `B`, stored on chain `A`

`incoming_A`: IBC receipts for incoming IBC packets from chain `B`, stored on chain `A`

`outgoing_B`: Outgoing IBC packets from chain `B` to chain `A`, stored on chain `B`

`incoming_B`: IBC receipts for incoming IBC packets from chain `A`, stored on chain `B`

## Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

## Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

## Example Implementation

(link to or description of concrete example implementation)

## Other Implementations

(links to or descriptions of other implementations)

# History

(changelog and notable inspirations / references)

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
