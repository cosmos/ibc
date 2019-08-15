---
ics: 24
title: Host State Machine Requirements
stage: draft
category: ibc-core
required-by: 2, 3, 4, 5, 18
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-05-11
---

## Synopsis

This specification defines the minimal set of interfaces which must be provided and properties which must be fulfilled by a blockchain & state machine hosting an IBC handler (implementation of the interblockchain communication protocol; see [the architecture document](../../docs/ibc/1_IBC_ARCHITECTURE.md) for details).

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

The host chain MUST provide two separate key-value store interfaces, each with three functions which behave in the standard way:

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
type del = (key: Key) => void
```

`Key` is as defined above. `Value` is an arbitrary bytestring encoding of a particular data structure. Encoding details are left to separate ICSs.

These functions MUST be permissioned to the IBC handler module (the implementation of which is described in separate standards) only, so only the IBC handler module can `set` or `delete` the keys which can be read by `get`. This can possibly be implemented as a sub-store (prefixed key-space) of a larger key-value store used by the entire state machine.

The first interface provided by the host state machine MUST write to a key-value store whose data can be externally proved with a vector commitment as defined in [ICS 23](../ics-023-vector-commitments). The second interface MAY support external proofs, but is not required to - the IBC handler will never write data to it which needs to be proved.

These interfaces are referred to throughout specifications which utilise them as the `provableStore` and the `privateStore` respectively, where `get`, `set`, and `del` are called as methods, e.g. `provableStore.set('key', 'value')`.

### Consensus State Introspection

Host chains MUST provide the ability to introspect their current height, with `getCurrentHeight`:

```
type getCurrentHeight = () => uint64
```

Host chains MUST define a unique `ConsensusState` type fulfilling the requirements of [ICS 2](../ics-002-validity-predicate):

```typescript
type ConsensusState object
```

Host chains MUST provide the ability to introspect their own consensus state, with `getConsensusState`:

```typescript
type getConsensusState = (height: uint64) => ConsensusState
```

`getConsensusState` is RECOMMENDED to return the consensus state for the consensus algorithm of the host chain at the specified height, for all heights greater than zero and less than or equal to the current height. `getConsensusState` MAY return the consensus state only for some number of recent heights, where the number is constant for the host chain.

### Port system

Host chains MUST implement a port system, where the IBC handler can expose functions to different parts of the state machine (perhaps modules) that can bind to uniquely named ports.

Host chains MUST permission interaction with the IBC handler such that:

- Once a module has bound to a port, no other modules can use that port until the module releases it
- A single module can bind to multiple ports
- Ports are allocated first-come first-serve and "reserved" ports for known modules can be bound when the chain is first started

This permissioning can be implemented either with unique references (object capabilities) for each port (a la the Cosmos SDK) or with source authentication (a la Ethereum), in either case enforced by the host state machine. See [ICS 5](../ics-005-port-allocation) for details.

Modules which wish to make use of particular IBC features MAY implement certain handler functions, e.g. to add additional logic to a channel handshake with an associated module on another chain.

### Datagram submission

Host chains MAY define a unique `Datagram` type & `submitDatagram` function to submit [datagrams](../../docs/ibc/2_IBC_TERMINOLOGY.md) directly to the relayer module:

```typescript
type Datagram object
// fields defined per datagram type, and possible additional fields defined per chain

type SubmitDatagram = (datagram: Datagram) => void
```

`submitDatagram` allows relayers to relay IBC datagrams directly to the host chain. Host chains MAY require that the relayer submitting the datagram has an account to pay transaction fees, signs over the datagram in a larger transaction structure, etc - `submitDatagram` MUST define any such packaging required.

Host chains MAY also define a `pendingDatagrams` function to scan the pending datagrams to be sent to another counterparty chain:

```typescript
type PendingDatagrams = (counterparty: Chain) => Set<Datagram>
```

```typescript
interface Chain {
  submitDatagram: SubmitDatagram
  pendingDatagrams: PendingDatagrams
}
```

### Exception system

Host chains MUST support an exception system, whereby a transaction can abort execution and revert any previously made state changes, exposed through an `assert` function:

```typescript
type assert = (bool) => ()
```

If the boolean passed to `assert` is `true`, the host chain need not do anything. If the boolean passed to `assert` is `false`, the host chain MUST abort the transaction and revert any previously made state changes, such as writes to the key-value store.

### Data availability

For safety (e.g. exactly-once packet delivery), host chains MUST have eventual data availability, such that any key-value pairs in state can be eventually retrieved by relayers.

For liveness (relaying packets, which will have a timeout), host chains MUST have partially synchronous data availability (e.g. within a wall clock or block height bound), such that any key-value pairs in state can be retrieved by relayers within the bound.

Data computable from a subset of state and knowledge of the state machine (e.g. IBC packet data, which is not directly stored) are also assumed to be available to and efficiently computable by relayers.

Light clients of particular consensus algorithms may have different and/or more strict data availability requirements.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Key-value store functionality and consensus state type are unlikely to change during operation of a single host chain.

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
