---
ics: 24
title: Host State Machine Requirements
stage: draft
category: IBC/TAO
required-by: 2, 3, 4, 5, 18
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-05-11
---

## Synopsis

This specification defines the minimal set of interfaces which must be provided and properties which must be fulfilled by a state machine hosting an implementation of the interblockchain communication protocol.

### Motivation

IBC is designed to be a common standard which will be hosted by a variety of blockchains & state machines and must clearly define the requirements of the host.

### Definitions

### Desired Properties

IBC should require as simple an interface from the underlying state machine as possible to maximise the ease of correct implementation.

## Technical Specification

### Keys, Identifiers, Separators

A `Key` is a bytestring used as the key for an object stored in state. Keys MUST contain only alphanumeric characters and the separator `/`.

An `Identifier` is a bytestring used as a key for an object stored in state, such as a connection, channel, or light client. Identifiers MUST consist of alphanumeric characters only.

Identifiers are not intended to be valuable resources — to prevent name squatting, minimum length requirements or pseudorandom generation MAY be implemented.

The separator `/` is used to separate and concatenate two identifiers or an identifier and a constant bytestring. Identifiers MUST NOT contain the `/` character, which prevents ambiguity.

Variable interpolation, denoted by curly braces, MAY be used as shorthand to define key formats, e.g. `client/{clientIdentifier}/consensusState`.

### Key/value Store

The host state machine MUST provide two separate key-value store interfaces, each with three functions which behave in the standard way:

```typescript
type Key = string

type Value = string
```

```typescript
type get = (key: Key) => Value | void
```

```typescript
type set = (key: Key, value: Value) => void
```

```typescript
type delete = (key: Key) => void
```

`Key` is as defined above. `Value` is an arbitrary bytestring encoding of a particular data structure. Encoding details are left to separate ICSs.

These functions MUST be permissioned to the IBC handler module (the implementation of which is described in separate standards) only, so only the IBC handler module can `set` or `delete` the keys which can be read by `get`. This can possibly be implemented as a sub-store (prefixed key-space) of a larger key-value store used by the entire state machine.

The first interface provided by the host state machine MUST write to a key-value store whose data can be externally proved with a vector commitment as defined in [ICS 23](../ics-023-vector-commitments). The second interface MAY support external proofs, but is not required to - the IBC handler will never write data to it which needs to be proved.

These interfaces are referred to throughout specifications which utilise them as the `provableStore` and the `privateStore` respectively, where `get`, `set`, and `delete` are called as methods, e.g. `provableStore.set('key', 'value')`.

The `provableStore` and `privateStore` differ also in their encoding restrictions. The `provableStore` MUST use canonical data structure encodings provided in these specifications as proto3 files, since values stored in the `provableStore` must be comprehensible to other machines implementing IBC.

> Note: any key-value store interface which provides these methods & properties is sufficient for IBC. Host state machines may implement "proxy stores" with underlying storage models which do not directly match the key & value pairs set and retrieved through the store interface — keys could be grouped into buckets & values stored in pages which could be proved in a single commitment, keyspaces could be remapped non-contiguously in some bijective manner, etc — as long as `get`, `set`, and `delete` behave as expected and other machines can verify commitment proofs of key & value pairs (or their absence) in the provable store.

### Consensus state introspection

Host state machines MUST provide the ability to introspect their current height, with `getCurrentHeight`:

```
type getCurrentHeight = () => uint64
```

Host state machines MUST define a unique `ConsensusState` type fulfilling the requirements of [ICS 2](../ics-002-client-semantics):

```typescript
type ConsensusState object
```

Host state machines MUST provide the ability to introspect their own consensus state, with `getConsensusState`:

```typescript
type getConsensusState = (height: uint64) => ConsensusState
```

`getConsensusState` MUST return the consensus state for at least some number `n` of contiguous recent heights, where `n` is constant for the host state machine. Heights older than `n` MAY be safely pruned (causing future calls to fail for those heights).

### Port system

Host state machines MUST implement a port system, where the IBC handler can expose functions to different parts of the state machine (perhaps modules) that can bind to uniquely named ports.

Host state machines MUST permission interaction with the IBC handler such that:

- Once a module has bound to a port, no other modules can use that port until the module releases it
- A single module can bind to multiple ports
- Ports are allocated first-come first-serve and "reserved" ports for known modules can be bound when the state machine is first started

This permissioning can be implemented either with unique references (object capabilities) for each port (a la the Cosmos SDK) or with source authentication (a la Ethereum), in either case enforced by the host state machine. See [ICS 5](../ics-005-port-allocation) for details.

Modules which wish to make use of particular IBC features MAY implement certain handler functions, e.g. to add additional logic to a channel handshake with an associated module on another state machine.

### Datagram submission

Host state machines MAY define a unique `Datagram` type & `submitDatagram` function to submit [datagrams](../../docs/ibc/2_IBC_TERMINOLOGY.md) directly to the relayer module:

```typescript
type Datagram object
// fields defined per datagram type, and possible additional fields defined per state machine

type SubmitDatagram = (datagram: Datagram) => void
```

`submitDatagram` allows relayers to relay IBC datagrams directly to the host state machine. Host state machines MAY require that the relayer submitting the datagram has an account to pay transaction fees, signs over the datagram in a larger transaction structure, etc - `submitDatagram` MUST define any such packaging required.

Host state machines MAY also define a `pendingDatagrams` function to scan the pending datagrams to be sent to another counterparty state machine:

```typescript
type PendingDatagrams = (counterparty: Machine) => Set<Datagram>
```

```typescript
interface Machine {
  submitDatagram: SubmitDatagram
  pendingDatagrams: PendingDatagrams
}
```

### Exception system

Host state machines MUST support an exception system, whereby a transaction can abort execution and revert any previously made state changes, exposed through an `assert` function:

```typescript
type assert = (bool) => ()
```

If the boolean passed to `assert` is `true`, the host state machine need not do anything. If the boolean passed to `assert` is `false`, the host state machine MUST abort the transaction and revert any previously made state changes, such as writes to the key-value store.

### Data availability

For safety (e.g. exactly-once packet delivery), host state machines MUST have eventual data availability, such that any key-value pairs in state can be eventually retrieved by relayers.

For liveness (relaying packets, which will have a timeout), host state machines MUST have partially synchronous data availability (e.g. within a wall clock or block height bound), such that any key-value pairs in state can be retrieved by relayers within the bound.

Data computable from a subset of state and knowledge of the state machine (e.g. IBC packet data, which is not directly stored) are also assumed to be available to and efficiently computable by relayers.

Light clients of particular consensus algorithms may have different and/or more strict data availability requirements.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Key-value store functionality and consensus state type are unlikely to change during operation of a single host state machine.

`submitDatagram` can change over time as relayers should be able to update their processes.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

29 April 2019 - Initial draft
11 May 2019 - Rename "RootOfTrust" to "ConsensusState"
25 June 2019 - Use "ports" instead of module names

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
