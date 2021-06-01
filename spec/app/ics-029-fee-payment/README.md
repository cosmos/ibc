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
to incentivize the packet.

We require that the [relayer address is exposed to application modules](https://github.com/cosmos/ibc/pull/579) for
all packet-related messages, so the modules are able to incentivize the packet relayer


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
