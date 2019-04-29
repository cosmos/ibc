---
ics: 24
title: State Machine Requirements
stage: draft
category: ibc-core
requires: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-04-29
---

# Synopsis

This specification defines the minimal set of properties and interfaces which must be provided by a state machine hosting an implementation of the interblockchain communication protocol (IBC handler).

# Specification

## Motivation

IBC is designed to be a common standard which will be hosted by a variety of blockchains & state machines and must clearly define the requirements of the host.

## Definitions

`RootOfTrust` is as defined in [ICS 2](../ics-2-consensus-requirements).

## Desired Properties

IBC should require as simple an interface from the underlying state machine as possible to maximize the ease of correct implementation.

## Technical Specification

### Key/value store

Connection handlers and subsequent protocols make use of a simple key-value store interface provided by the underlying state machine. This store must provide two functions, which behave in the way you would expect:
- `Get(Key) -> Value | null`
- `Set(Key, Value)`

`Key` and `Value` are assumed to be byte slices; encoding details are left to a later ICS.

### Root-of-trust introspection

Blockchains also need the ability to introspect their own root-of-trust (with `getRootOfTrust`) in order to confirm that the connecting chain has stored the correct one.

### Datagram submission

Must define `submitDatagram` function.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

`submitDatagram` can change over time as relayer should be able to update their processes.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

April 29 2019 - Initial draft

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
