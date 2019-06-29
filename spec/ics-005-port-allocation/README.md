---
ics: 5
title: Port Allocation
stage: Draft
requires: 24
required-by: 4
category: IBC Core
author: Christopher Goes <cwgoes@tendermint.com>
created: 29 June 2019
modified: 29 June 2019
---

## Synopsis

This standard specifies the port allocation system by which modules can bind to uniquely named ports, allocated by the IBC handler,
from and to which channels can then be opened, and which can be transferred or later released by the module which originally bound to them and then reused.

### Motivation

The interblockchain communication protocol is designed to faciliate module-to-module traffic, where modules are independent, possibly mutually distrusted, self-contained
elements of code executing on sovereign ledgers. In order to provide the desired end-to-end semantics, the IBC handler must permission channels to particular modules, and
for convenience they should be addressable by name. This specification defines the *port allocation and ownership* system which realises that model.

Conventions may emerge as to what kind of module logic is bound to a particular port name, such as "bank" for fungible token handling or "staking" for interchain collateralization.
This is analogous to port 80's common use for HTTP servers — the protocol cannot enforce that particular module logic is *actually* bound to conventional ports, so
users must check that themselves.

### Definitions

`Identifier`, `get`, `set`, and `del` are defined as in [ICS 24](../ics-024-host-requirements).

A *port* is a particular kind of identifier which is used to permission channel opening and usage to modules.

A *module* is a subcomponent of the host state machine independent of the IBC handler. Examples include Ethereum smart contracts and Cosmos SDK & Substrate modules.
The IBC specification makes no assumptions of module functionality other than the ability of the host state machine to use object-capability or source authentication to permission ports to modules.

### Desired Properties

- Once a module has bound to a port, no other modules can use that port until the module releases it
- A module can, on its option, release a port or transfer it to another module
- A single module can bind to multiple ports at once
- Ports are allocated first-come first-serve and "reserved" ports for known modules can be bound when the chain is first started

## Technical Specification

### Data Structures

The host state machine MUST support either object-capability reference or source authentication for modules.

In the former case, the IBC handler must have the ability to generate *object-capability keys*, unique, opaque references
which can be passed to a module and will not be duplicable by other modules. Two examples are store keys as used in the Cosmos SDK ([reference](https://github.com/cosmos/cosmos-sdk/blob/master/store/types/store.go#L224))
and object references as used in Agoric's Javascript runtime ([reference](https://github.com/Agoric/SwingSet)).

```typescript
type CapabilityKey object
```

```typescript
function newCapabilityKey(id: string): CapabilityKey {
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

### Subprotocols

#### Preliminaries

`portKey` takes an `Identifier` and returns the store key under which the object-capability reference or owner module identifier associated with a port should be stored.

```typescript
function portKey(id: Identifier) {
  return "ports/{id}"
}
```

#### Binding to a port

```typescript
function bindPort(id: Identifier) {
  set(portKey(id))
}
```

#### Transferring ownership of a port

If the host state machine supports object-capability keys, no additional protocol is necessary, since the port reference is a bearer capability.

```typescript
function transferPort(id: Identifier, newOwner: Identifier) {
  set(portKey(id))
}
```

#### Releasing a port

```typescript
function releasePort(id: Identifier) {
  del(portKey(id))
}
```

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
