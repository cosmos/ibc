---
ics: 29
title: General Fee Payment
stage: draft
category: IBC/APP
requires: 20, 25, 26
kind: instantiation
author: Ethan Frey <ethan@confio.tech>
created: 2021-06-01
modified: 2021-06-01
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for handling fee
payments on top of any ICS application protocol. It requires some standard packet changes, but can be adopted by any
application, without forcing other applications to use this implementation.

### Motivation

There has been much discussion on a general incentivization mechanism for relayers. A simple proposal was created to
[extend ICS20 to incentivize relaying](https://github.com/cosmos/ibc/pull/577) on the destination chain. However,
it was very specific to ICS20 and would not work for other protocols. This was then extended to a more
[general fee payment design](https://github.com/cosmos/ibc/issues/578) that could be adopted by any ICS application
protocol.

In general, the Interchain dream will never scale unless there is a clear way to incentivize relayers. We seek to
define a clear interface that can be easily adopted by any application, but not preclude chains that don't use tokens.

### Desired Properties

- Incentivize timely delivery of the packet (`OnReceivePacket` called)
- Incentivize relaying acks for these packets (`OnAcknowledgement` called)
- Incentivize relaying timeouts for these packets when the receive fee was too low (`OnTimeout` called)
- Produces no extra IBC packets
- One direction works, even when one chain does not support concept of fungible tokens
- Opt-in for each chain implementing this. eg. ICS27 with fee support on chain A could connect to ICS27 without fee support on chain B.
- Standardized interface for each chain implementing this extension
- Permissionless relaying

## Technical Specification

### General Design

In order to avoid extra packets on the order of the number of fee packets, as well as provide an opt-in approach, we
store all fee payment info only on the source chain. The source chain is the one location where the sender can provide tokens
to incentivize the packet. The fee distribution may be implementation specific and thus does not need to be in the ibc spec
(just high-level requirements are needed in this doc).

We require that the [relayer address is exposed to application modules](https://github.com/cosmos/ibc/pull/579) for
all packet-related messages, so the modules are able to incentivize the packet relayer. `OnAcknowledgement`, `OnTimeout`,
and `OnTimeoutClose` messages will therefore have the relayer address and be capable of sending escrowed tokens to such address.
However, we need a way to reliably get the address of the relayer that submitted `OnReceivePacket` on the destination chain to
the source chain. In fact, we need a *source address* for this relayer to pay out to, not the *destination address* that signed
the packet.

Given this, the flow would be:

1. User/module submits a send packet on the `source` chain, along with some tokens and fee information on how to distribute them
2. Relayer submits `OnReceivePacket` on the `destination` chain. Along with this message, the *forward relayer* will submit a `payToOnSource` address where payment should be sent.
3. Destination application includes this `payToOnSource` in the acknowledgement (there are multiple approaches to discuss below)
4. Relayer submits `OnAcknowledgement` which provides the *return relayer* address on the source chain, along with the `payToOnSource` address
5. Source application can distribute the tokens escrowed in (1) to both the *forward* and the *return* relayers.

Alternate flow:

1. User/module submits a send packet on the `source` chain, along with some tokens and fee information on how to distribute them
2. Relayer submits `OnTimeout` which provides their address on the source chain
3. Source application can distribute the tokens escrowed in (1) to this relayer, and potentially return extra tokens to the original packet sender.

### Fee details

For an example implementation in the Cosmos SDK, we consider 3 potential fee payments, which may be defined. Each one may be
paid out in a different token. Imagine a connection between IrisNet and the Cosmos Hub. They may define:

- ReceiveFee: 0.003 channel-7/ATOM vouchers (ATOMs already on IrisNet via ICS20)
- AckFee: 0.001 IRIS
- TimeoutFee: 0.002 IRIS

Ideally the fees can easily be redeemed in native tokens on both sides, but relayers may select others. In this example, the relayer collects a fair bit of IRIS, covering it's costs there and more. It also collects channel-7/ATOM vouchers from many packets. After relaying a few thousand packets, the account on the Cosmos Hub is running low, so the relayer will send those channel-7/ATOM vouchers back over channel-7 to it's account on the Hub to replenish the supply there. 

#### Reasoning

##### Correctness

#### Optional addenda

## Backwards Compatibility

This can be added to any existing protocol without break it on the other side.

## Forwards Compatibility

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Jul 15, 2019 - Draft written

Jul 29, 2019 - Major revisions; cleanup

Aug 25, 2019 - Major revisions, more cleanup

Feb 3, 2020 - Revisions to handle acknowledgements of success & failure

Feb 24, 2020 - Revisions to infer source field, inclusion of version string

July 27, 2020 - Re-addition of source field

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
