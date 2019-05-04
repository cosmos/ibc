---
ics: 5
title: Packet Semantics & Handling
stage: proposal
category: ibc-core
requires: 3, 4, 19
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-05-05
---

# Synopsis

Packets contain the data transmitted between modules.

# Specification

## Motivation

Transmit packets between modules.

## Definitions

## Desired Properties

- ordering (if selected)
- exactly-once delivery

## Technical Specification

### Definitions

We define an IBC *packet* `P` as the five-tuple `(type, sequence, source, destination, data)`, where:

`type` is an opaque routing field

`sequence` is an unsigned, arbitrary-precision integer

`source` is a string uniquely identifying the chain, connection, and channel from which this packet was sent

`destination` is a string uniquely identifying the chain, connection, and channel which should receive this packet

`data` is an opaque application payload

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

### Handling receipts

When we wish to create a transaction that atomically commits or rolls back across two chains, we must look at the execution result returned in the IBC receipt. For example, if I want to send tokens from Alice on chain `A` to Bob on chain `B`, chain `A` must decrement Alice's account *if and only if* Bob's account was incremented on chain `B`. We can achieve that by storing a protected intermediate state on chain `A` (escrowing the assets in question), which is then committed or rolled back based on the result of executing the transaction on chain `B`.

To do this requires that we not only provably send a packet from chain `A` to chain `B`, but provably return the result of executing that packet (the receipt `data`) from chain `B` to chain `A`. If a valid IBC packet was sent from `A` to `B`, then the result of executing it is stored in `incoming_B`. Since the receipts are stored in a queue with the same key construction as the sending queue, we can generate the same set of proofs for them, and perform a similar sequence of steps to handle a receipt coming back to `A` for a message previously sent to `B`. Receipts, like packets, are processed in order.

To handle an IBC receipt on blockchain `A` received from blockchain `B`, with a Merkle proof `M_kvh` and the current set of trusted headers for that chain `T_B`:

`handle_receipt(R{sequence, source, destination, data}, M_kvh)`

```
case
  outgoing_A == nil ⇒ fail with "unregistered sender"
  destination /= (A, connection, channel) ⇒ fail with "wrong destination"
  sequence /= head(incoming_A) ⇒ fail with "out of order" 
  H_h not in T_B ⇒ fail with "must submit header for height h"
  valid(H_h, M_kvh) == false ⇒ fail with "invalid Merkle proof" 
  otherwise ⇒
    set P{type, _, _, _, _} = pop(outgoing_A)
    f_type(result)
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

The `receipt_handler` function on chain `A` can now verify timeouts and pass valid timeout receipts to the application handler (which can revert state changes such as escrowing assets):

`receipt_handler`

```
case
  ... 
  result == TimeoutError ⇒ case
    not expired(H_h, P) ⇒ fail with "message timeout not yet reached"
    otherwise ⇒ f_type(R, TimeoutError)
  ... 
```

This adds one more guarantee:

`A:M_kvh == ∅` if message i was sent and timeout proven before height h (and the receipts for all messages j < i were also handled).

Now chain `A` can rollback all transactions that were blocked by this flood of unrelayed packets - since they can never confirm - without waiting for chain `B` to process them and return a receipt. Adding reasonable timeouts to all packets allows us to gracefully handle any errors with the IBC relay processes or a flood of unrelayed "spam" IBC packets. If a blockchain requires a timeout on all messages and imposes some reasonable upper limit, we can guarantee that if a packet is not processed by the upper limit of the timeout period, then all previous packets must also have either been processed or reached the timeout period. 

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

Additionally, if timestamp-based timeouts are used instead of height-based timeouts, the destination chain's consensus ruleset must enforce always-increasing timestamps (or the sending chain must use a more complex `expired` function).

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Can be versioned at the channel level.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

5 May 2019 - Draft submitted

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
