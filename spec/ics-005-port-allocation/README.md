---
ics: 5
title: Port Allocation
stage: Draft
requires: 24
required-by: 4
category: IBC/TAO
author: Christopher Goes <cwgoes@tendermint.com>
created: 29 June 2019
modified: 29 June 2019
---

## Synopsis

This standard specifies the port allocation system by which modules can bind to uniquely named ports, allocated by the IBC handler,
from and to which channels can then be opened, and which can be transferred or later released by the module which originally bound to them and then reused.

### Motivation

The interblockchain communication protocol is designed to facilitate module-to-module traffic, where modules are independent, possibly mutually distrusted, self-contained
elements of code executing on sovereign ledgers. In order to provide the desired end-to-end semantics, the IBC handler must permission channels to particular modules, and
for convenience they should be addressable by name. This specification defines the *port allocation and ownership* system which realises that model.

Conventions may emerge as to what kind of module logic is bound to a particular port name, such as "bank" for fungible token handling or "staking" for interchain collateralisation.
This is analogous to port 80's common use for HTTP servers — the protocol cannot enforce that particular module logic is actually bound to conventional ports, so
users must check that themselves. Ephemeral ports with pseudorandom identifiers may be created for temporary protocol handling.

Modules may bind to multiple ports and connect to multiple ports bound to by another module on a separate machine. Any number of (uniquely identified) channels can utilise a single
port simultaneously. Channels are end-to-end between two ports, each of which must have been previously bound to by a module, which will then control that end of the channel.

Optionally, the host state machine can elect to expose port binding only to a specially-permissioned module manager,
which can control which ports modules can bind to with a custom rule-set, and transfer ports to modules only when it
has validated the port name & module. This role can be played by the relayer module (see [ICS 26](../ics-026-relayer-module)).

### Definitions

`Identifier`, `get`, `set`, and `delete` are defined as in [ICS 24](../ics-024-host-requirements).

A *port* is a particular kind of identifier which is used to permission channel opening and usage to modules.

A *module* is a sub-component of the host state machine independent of the IBC handler. Examples include Ethereum smart contracts and Cosmos SDK & Substrate modules.
The IBC specification makes no assumptions of module functionality other than the ability of the host state machine to use object-capability or source authentication to permission ports to modules.

### Desired Properties

- Once a module has bound to a port, no other modules can use that port until the module releases it
- A module can, on its option, release a port or transfer it to another module
- A single module can bind to multiple ports at once
- Ports are allocated first-come first-serve and "reserved" ports for known modules can be bound when the chain is first started

## Technical Specification

### Data Structures

The host state machine MUST support either object-capability reference or source authentication for modules.

In the former case, the IBC handler must have the ability to generate *object-capabilities*, unique, opaque references
which can be passed to a module and will not be duplicable by other modules. Two examples are store keys as used in the Cosmos SDK ([reference](https://github.com/cosmos/cosmos-sdk/blob/master/store/types/store.go#L224))
and object references as used in Agoric's Javascript runtime ([reference](https://github.com/Agoric/SwingSet)).

```typescript
type CapabilityKey object
```

```typescript
function newCapabilityPath(): CapabilityKey {
  // provided by host state machine, e.g. pointer address in Cosmos SDK
}
```

In the latter case, the IBC handler must have the ability to securely read the *source identifier* of the calling module,
a unique string for each module in the host state machine, which cannot be altered by the module or faked by another module.
An example is smart contract addresses as used by Ethereum ([reference](https://ethereum.github.io/yellowpaper/paper.pdf)).

```typescript
type SourceIdentifier string
```

```typescript
function callingModuleIdentifier(): SourceIdentifier {
  // provided by host state machine, e.g. contract address in Ethereum
}
```

`generate` and `authenticate` functions are then defined as follows.

In the former case, `generate` returns a new object-capability key, which must be returned by the outer-layer function, and `authenticate` requires that the outer-layer function take an extra argument `capability`, which is an object-capability key with uniqueness enforced by the host state machine. Outer-layer functions are any functions exposed by the IBC handler ([ICS 25](../ics-025-handler-interface)) or relayer module ([ICS 26](../ics-026-relayer-module)) to modules.

```
function generate(): CapabilityKey {
  return newCapabilityPath()
}
```

```
function authenticate(key: CapabilityKey): boolean {
  return capability === key
}
```

In the latter case, `generate` returns the calling module's identifier and `authenticate` merely checks it.

```
function generate(): SourceIdentifier {
  return callingModuleIdentifier()
}
```

```
function authenticate(id: SourceIdentifier): boolean {
  return callingModuleIdentifier() === id
}
```



### Subprotocols

#### Preliminaries

`portKey` takes an `Identifier` and returns the store key under which the object-capability reference or owner module identifier associated with a port should be stored.

```typescript
function portPath(id: Identifier): Path {
  return "ports/{id}"
}
```

#### Binding to a port

The IBC handler MUST implement `bindPort`. `bindPort` binds to an unallocated port, failing if the port has already been allocated.

If the host state machine does not implement a special module manager to control port allocation, `bindPort` SHOULD be available to all modules. If it does, `bindPort` SHOULD only be callable by the module manager.

```typescript
function bindPort(id: Identifier) {
  assert(provableStore.get(portPath(id)) === null)
  key = generate()
  provableStore.set(key, portPath(id))
  return key
}
```

#### Transferring ownership of a port

If the host state machine supports object-capabilities, no additional protocol is necessary, since the port reference is a bearer capability. If it does not, the IBC handler MAY implement the following `transferPort` function.

`transferPort` SHOULD be available to all modules.

```typescript
function transferPort(id: Identifier) {
  assert(authenticate(provableStore.get(portPath(id))))
  key = generate()
  provableStore.set(portPath(id), key)
}
```

#### Releasing a port

The IBC handler MUST implement the `releasePort` function, which allows a module to release a port such that other modules may then bind to it.

`releasePort` SHOULD be available to all modules.

```typescript
function releasePort(id: Identifier) {
  assert(authenticate(provableStore.get(portPath(id))))
  provableStore.delete(portPath(id))
}
```

### Properties & Invariants

- By default, port identifiers are first-come-first-serve: once a module has bound to a port, only that module can utilise the port until the module transfers or releases it. A module manager can implement custom logic which overrides this.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Port binding is not a wire protocol, so interfaces can change independently on separate chains as long as the ownership semantics are unaffected.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

29 June 2019 - Initial draft

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
