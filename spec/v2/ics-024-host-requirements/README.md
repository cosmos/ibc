---
ics: 24
title: Host State Machine Requirements
stage: draft
category: IBC/TAO
kind: interface
requires: 23
required-by: 4
version compatibility: ibc-go v10.0.0
author: Aditya Sripal <aditya@interchain.io>
created: 2024-08-21
modified: 2024-08-21
---

## Synopsis

This specification defines the minimal set of interfaces which must be provided and properties which must be fulfilled by a state machine hosting an implementation of the interblockchain communication protocol. IBC relies on a key-value provable store for cross-chain communication. In version 2 of the specification, the expected key-value storage will only be for the keys that are relevant for packet processing.

### Motivation

IBC is designed to be a common standard which will be hosted by a variety of blockchains & state machines and must clearly define the requirements of the host.

### Definitions

### Desired Properties

IBC should require as simple an interface from the underlying state machine as possible to maximise the ease of correct implementation.

## Technical Specification

### Module system

The host state machine must support a module system, whereby self-contained, potentially mutually distrusted packages of code can safely execute on the same ledger, control how and when they allow other modules to communicate with them, and be identified and manipulated by a "master module" or execution environment.

The IBC/TAO specifications define the implementations of two modules: the core "IBC handler" module and the "IBC relayer" module. IBC/APP specifications further define other modules for particular packet handling application logic. IBC requires that the "master module" or execution environment can be used to grant other modules on the host state machine access to the IBC handler module and/or the IBC routing module, but otherwise does not impose requirements on the functionality or communication abilities of any other modules which may be co-located on the state machine.

### Paths, identifiers, separators

An `Identifier` is a bytestring used as a key for an object stored in state, such as a packet commitment, acknowledgement, or receipt.

Identifiers MUST be non-empty (of positive integer length).

Identifiers MUST consist of characters in one of the following categories only:

- Alphanumeric
- `.`, `_`, `+`, `-`, `#`
- `[`, `]`, `<`, `>`

A `Path` is a bytestring used as the key for an object stored in state. Paths MUST contain only identifiers, constant strings, and the separator `"/"`.

Identifiers are not intended to be valuable resources — to prevent name squatting, minimum length requirements or pseudorandom generation MAY be implemented, but particular restrictions are not imposed by this specification.

The separator `"/"` is used to separate and concatenate two identifiers or an identifier and a constant bytestring. Identifiers MUST NOT contain the `"/"` character, which prevents ambiguity.

Variable interpolation, denoted by curly braces, is used throughout this specification as shorthand to define path formats, e.g. `client/{clientIdentifier}/consensusState`.

All identifiers, and all strings listed in this specification, must be encoded as ASCII unless otherwise specified.

By default, identifiers have the following minimum and maximum lengths in characters:

| Port identifier | Client identifier | Channel identifier |
| --------------- | ----------------- | ------------------ |
| 2 - 128         | 9 - 64            | 8 - 64             |

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
type delete = (path: Path) => void
```

`Path` is as defined above. `Value` is an arbitrary bytestring encoding of a particular data structure. Encoding details are left to separate ICSs.

These functions MUST be permissioned to the IBC handler module (the implementation of which is described in separate standards) only, so only the IBC handler module can `set` or `delete` the paths that can be read by `get`. This can possibly be implemented as a sub-store (prefixed key-space) of a larger key/value store used by the entire state machine.

Host state machines MUST provide two instances of this interface -
a `provableStore` for storage read by (i.e. proven to) other chains,
and a `privateStore` for storage local to the host, upon which `get`
, `set`, and `delete` can be called, e.g. `provableStore.set('some/path', 'value')`.

The `provableStore`:

- MUST write to a key/value store whose data can be externally proved with a vector commitment that can be proven by an IBC light client as defined in [ICS-002](../ics-002-client-semantics/README.md)
- MUST use canonical data structure encodings provided in these specifications.

The `privateStore`:

- MAY support external proofs, but is not required to - the IBC handler will never write data to it which needs to be proved.
- MAY use canonical proto3 data structures, but is not required to - it can use
  whatever format is preferred by the application environment.

> Note: any key/value store interface which provides these methods & properties is sufficient for IBC. Host state machines may implement "proxy stores" with path & value mappings which do not directly match the path & value pairs set and retrieved through the store interface — paths could be grouped into buckets & values stored in pages which could be proved in a single commitment, path-spaces could be remapped non-contiguously in some bijective manner, etc — as long as `get`, `set`, and `delete` behave as expected and other machines can verify commitment proofs of path & value pairs (or their absence) in the provable store. If applicable, the store must expose this mapping externally so that clients (including relayers) can determine the store layout & how to construct proofs. Clients of a machine using such a proxy store must also understand the mapping, so it will require either a new client type or a parameterised client.
>
> Note: this interface does not necessitate any particular storage backend or backend data layout. State machines may elect to use a storage backend configured in accordance with their needs, as long as the store on top fulfils the specified interface and provides commitment proofs.

### Provable Path-space

IBC/TAO implementations MUST implement the following paths for the `provableStore` in the exact format specified. This is because counterparty IBC/TAO implementations will construct the paths according to this specification and send it to the light client to verify the IBC specified value stored under the IBC specified path.

Future paths may be used in future versions of the protocol, so the entire key-space in the provable store MUST be reserved for the IBC handler.

| Value                      | Path format                                       | Value type | Defined in                           |
| -------------------------- | ------------------------------------------------- | ---------- | ------------------------------------ |
| Packet Commitment          | {channelIdentifier}|0x1|{bigEndianUint64Sequence} | bytes      | [ICS 4](../ics-004-channel-and-packet-semantics) |
| Packet Receipt             | {channelIdentifier}|0x2|{bigEndianUint64Sequence} | bytes      | [ICS 4](../ics-004-channel-and-packet-semantics) |
| Acknowledgement Commitment | {channelIdentifier}|0x3|{bigEndianUint64Sequence} | bytes      | [ICS 4](../ics-004-channel-and-packet-semantics) |

### Provable Commitments

In addition to specifying the paths that are expected across implementations, ICS-24 also standardizes the commitments stored under each standardized path. These commitments will also be constructed and checked by counterparties in order to enable packet flow.

#### Packet Commitment

When sending a packet, all IBC implementations are expected to store a packet commitment under the above specified packet commitment path. When receiving a packet, all IBC implementations will reconstruct the expected packet commitment and verify it was stored under the expected packet commitment path in order to verify the packet before sending it to the application.

IBC standardizes the fields that a packet must have but does not standardize the structure containing them nor their on-chain encoding. Thus, different implementations may house these fields in different structs and encode the struct differently for their internal use so long as they create the same exact commitment when they store the packet under the packet commitment path.

The structure of the packet and commitment of the packet timeout and application data is further specified in ICS4.

```typescript
// commit packet hashes the destination identifier, the timeout and the data meant to be
// processed by the destination state machine
func commitPacket(destIdentifier: bytes, timeoutBytes: bytes, data: bytes): bytes {
  buffer = sha256.Hash(destIdentifier)
  buffer = append(buffer, sha256.hash(bigEndian(timeoutBytes)))
  buffer = append(buffer, sha256.hash(data))
  return sha256.hash(buffer)
}
```

Since the packet commitment is keyed on the source identifier and sequence, with this key and value together; the receiving chain can prove that the sending chain has sent a packet with the given source and destination identifiers at a given sequence with the timeout and application data.

#### Packet Acknowledgement

The acknowledgement will be provided with the packet to the sending chain. Thus we only need to provably associate the acknowledgement with the original packet. This association is already accomplished by the acknowledgment path which contains the destination identifier and the sequence. Thus on the sending chain, we can prove the acknowledgement was indeed sent for the packet we sent. We prove the packet was sent by us by checking that we stored the packet commitment under the packet commitment path. We can retrieve the client from the source identifier and then prove the counterparty stored under the destination identifier and sequence. Thus, we can associate the acknowledgement stored under this path with the unique packet provided. The acknowledgement commitment can therefore simply consist of a hash of the acknowledgment data sent to the state machine.

The creation of `ackData` for a given packet is further specified in ICS4.

```typescript
func commitAcknowledgment(ackData: bytes): bytes {
  return sha256.hash(ackData)
}
```

#### Packet Receipt

A packet receipt will only tell the sending chain that the counterparty has successfully received the packet. Thus we just need a provable boolean flag uniquely associated with the sent packet. Thus, the receiver chain stores the packet receipt keyed on the destination identifier and the sequence to uniquely identify the packet.

For chains that support nonexistence proofs of their own state, they can simply write a `SENTINEL_RECEIPT_VALUE` under the receipt path. This `SENTINEL_RECEIPT_PATH` can be any non-nil value so it is recommended to write a single byte.

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
