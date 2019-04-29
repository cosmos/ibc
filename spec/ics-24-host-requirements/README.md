---
ics: 24
title: Host State Machine Requirements
stage: draft
category: ibc-core
requires: 2
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-04-29
---

# Synopsis

This specification defines the minimal set of interfaces which must be provided and properties which must be fulfilled by a state machine hosting an IBC handler (implementation of the interblockchain communication protocol).

# Specification

## Motivation

IBC is designed to be a common standard which will be hosted by a variety of blockchains & state machines and must clearly define the requirements of the host state machine.

## Definitions

`RootOfTrust` is as defined in [ICS 2](../ics-2-consensus-requirements).

## Desired Properties

IBC should require as simple an interface from the underlying state machine as possible to maximize the ease of correct implementation.

## Technical Specification

### Keys, identifiers, separators

A `Key` is a bytestring used as the key for an object stored in state. Keys contain only alphanumeric characters and the separator `/`.

An `Identifier` is a bytestring used as a key for an object stored in state, such as a connection, channel, or light client. Identifiers MUST consist of alphanumeric characters only. Identifiers are not intended to be valuable resources — to prevent name squatting, minimum length requirements or pseudorandom generation may be implemented.

The separator `/` is used to separate and concatenate two identifiers or an identifier and a constant bytestring. Identifiers cannot contain the `/` character, which prevents ambiguity.

Variable interpolation, denoted by curly braces, may be used in shorthand to define key formats, e.g. `client/{clientIdentifier}/rootOfTrust`.

### Key/value store

Host chains MUST provide a simple key-value store interface, with two functions which behave in the way you would expect:

```coffeescript
function get(Key key) -> Value | null
```

```coffeescript
function set(Key key, Value value)
```

`Key` is as defined above. `Value` is an arbitrary bytestring encoding of a particular data structure. Encoding details are left to separate ICSs.

### Root-of-trust introspection

Host chains MUST provide the ability to introspect their own root-of-trust, with `getRootOfTrust`.

### Datagram submission

Host chains MUST define a unique `submitDatagram` function.

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
