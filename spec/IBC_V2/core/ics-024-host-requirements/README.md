---
ics: 24
title: Host State Machine Requirements
stage: draft
category: IBC/TAO
kind: interface
required-by: 4
version compatibility: ibc-go v10.0.0
author: Aditya Sripal <aditya@interchain.io>
created: 2024-08-21
modified: 2024-08-21
---

## Synopsis

This specification defines the minimal set of properties which must be fulfilled by a state machine hosting an implementation of the interblockchain communication protocol. IBC relies on a key-value provable store for cross-chain communication. In version 2 of the specification, the expected key-value storage will only be for the keys that are relevant for packet processing.

### Motivation

IBC is designed to be a common standard which will be hosted by a variety of blockchains & state machines and must clearly define the requirements of the host.

### Definitions

### Desired Properties

IBC should require as simple an interface from the underlying state machine as possible to maximise the ease of correct implementation.

## Technical Specification

### Module system

The host state machine must support a module system, whereby self-contained, potentially mutually distrusted packages of code can safely execute on the same ledger, control how and when they allow other modules to communicate with them, and be identified and manipulated by a "master module" or execution environment.

The IBC core handlers as defined in ICS-4 must have 

### Paths, identifiers, separators

An `Identifier` is a bytestring used as a key for an object stored in state, such as a packet commitment, acknowledgement, or receipt.

Identifiers MUST be non-empty (of positive integer length).

Identifiers MUST consist of characters in one of the following categories only:

- Alphanumeric
- `.`, `_`, `+`, `-`, `#`
- `[`, `]`, `<`, `>`

A `Path` is a bytestring used as the key for an object stored in state. Paths MUST contain only identifiers, constant bytestrings, and the separator `"/"`.

Identifiers are not intended to be valuable resources — to prevent name squatting, minimum length requirements or pseudorandom generation MAY be implemented, but particular restrictions are not imposed by this specification.

The separator `"/"` is used to separate and concatenate two identifiers or an identifier and a constant bytestring. Identifiers MUST NOT contain the `"/"` character, which prevents ambiguity.

By default, identifiers have the following minimum and maximum lengths in characters:

| Port identifier | Client identifier |
| --------------- | ----------------- |
| 2 - 128         | 2 - 64            |

### Key/value Store

The host state machine MUST provide a key/value store interface 
with three functions that behave in the standard way:

```typescript
type get = (path: Path) => Value | void
```

```typescript
type set = (path: Path, value: Value) => void
```

```typescript
type queryProof = (path: Path) => (CommitmentProof, Value)
```

`queryProof` will return a `Membership` proof if there exists a value for that path in the key/value store and a `NonMembership` proof if there is no value stored for the path.

The host state machine SHOULD provide an interface for deleting
a Path from the key/value store as well though it is not required:

```typescript
type delete = (path: Path) => void
```

`Path` is as defined above. `Value` is an arbitrary bytestring encoding of a particular data structure. The specific Path and Values required to be written to the provable store are defined in [ICS-4](../ics-004-packet-semantics/PACKET.md).

These functions MUST be permissioned to the IBC packet handler module (the implementation of which is described in [ICS-4](../ics-004-packet-semantics/PACKET_HANDLER.md)) only, so only the IBC handler module can `set` or `delete` the paths that can be read by `get`.

In most cases, this will be implemented as a sub-store (prefixed key-space) of a larger key/value store used by the entire state machine. This is why ICS-2 defines a `counterpartyCommitmentPrefix` that is associated with the client. The IBC handler will prefix the `counterpartyCommitmentPrefix` to the ICS-4 standardized path before proof verification against a `ConsensusState` in the client.

### Provable Path-space

IBC/TAO implementations MUST implement the following paths for the `provableStore` in the exact format specified. This is because counterparty IBC/TAO implementations will construct the paths according to this specification and send it to the light client to verify the IBC specified value stored under the IBC specified path.

Future paths may be used in future versions of the protocol, so the entire key-space in the provable store MUST be reserved for the IBC handler.

| Value                      | Path format                                  |
| -------------------------- | -------------------------------------------- |
| Packet Commitment          | {sourceClientId}0x1{bigEndianUint64Sequence} |
| Packet Receipt             | {destClientId}0x2{bigEndianUint64Sequence}   |
| Acknowledgement Commitment | {destClientId}0x3{bigEndianUint64Sequence}   |

IBC V2 only proves commitments related to packet handling, thus the commitments and how to construct them are specifed in [ICS-4](../ics-004-packet-semantics/PACKET.md).

As mentioned above, the provable path space controlled by the IBC handler may be prefixed in a global provable key/value store. In this case, the prefix must be appended by the IBC handler before the proof is verified.

The provable store MUST be capable of providing `MembershipProof` for a key/value pair that exists in the store. It MUST also be capable of providing a `NonMembership` proof for a key that does not exist in the store.

In the case, the state machine does not support `NonMembership` proofs; a client may get around this restriction by associating a `SENTINEL_ABSENCE_VALUE` with meaning the key does not exist and treating a `MembershipProof` with a `SENTINEL_ABSENCE_VALUE` as a `NonMembershipProof`. In this case, the state machine is responsible for ensuring that there is a way to write a `SENTINEL_ABSENCE_VALUE` to the keys that IBC needs to prove nonmembership for and it MUST ensure that an actor cannot set the `SENTINEL_ABSENCE_VALUE` directly for a key accidentally. These requirements and how to implement them are outside the scope of this specification and remain the responsibility of the bespoke IBC implementation.

### Finality

The state machine MUST make updates sequentially so that all state updates happen in order and can be associated with a unique `Height` in that order. Each state update at a height `h` MUST be eventually **finalized** at a finite timestamp `t` such that the order of state updates from the initial state up to `h` will never change after time `t`.

IBC handlers will only accept packet-flow messages from state updates which are already deemed to be finalized. In cases where the finality property is probabilistically guaranteed, this probabilitic guarantee must be handled within the ICS-2 client in order to provide a final view of the remote state machine for the ICS-4 packet handler.

### Time

As the state updates are applied to the state machine over time, the state update algorithm MUST itself have secure access to the current timestamp at which the state update is being applied. This is needed for IBC handlers to process timeouts correctly.

If the state machine update mechanism does not itself provide a timestamp to the state machine handler, then there must be a time oracle updates as part of the state machine update itself. In this case, the security model of IBC will also include the security model of the time oracle.

This timestamp for a state update MUST be monotonically increasing and it MUST be the greater than or equal to the timestamp that the counterparty client will return for the `ConsensusState` associated with that state update.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Key/value store functionality and consensus state type are unlikely to change during operation of a single host state machine.

`submitDatagram` can change over time as relayers should be able to update their processes.

## Example Implementations

## History

Aug 21, 2024 - [Initial draft](https://github.com/cosmos/ibc/pull/1144)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
