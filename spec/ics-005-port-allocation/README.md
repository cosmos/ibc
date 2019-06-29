---
ics: 5
title: Port Allocation
stage: Draft
category: IBC Core
author: Christopher Goes <cwgoes@tendermint.com>
created: 29 June 2019
modified: 29 June 2019
---

## Synopsis

The interblockchain communication protocol is designed to faciliate module-to-module traffic, where modules are independent, possibly mutually distributed, self-contained
elements of code executing on sovereign ledgers. In order to provide the desired end-to-end semantics, the IBC handler must permission channels to particular modules, and
for convenience they should be addressable by name. This specification defines the *port allocation and ownership* system which realises that model.

### Motivation

Conventions may emerge...

### Definitions

A *port* is a named identifier.

A *module* is a subcomponent of the host state machine independent of the IBC handler. Examples include Ethereum smart contracts and Cosmos SDK & Substrate modules.
The IBC specification makes no assumptions of module functionality other than the ability to use object-capability or source authentication to permission ports to modules.

### Desired Properties

- Once a module has bound to a port, no other modules can use that port until the module releases it
- A single module can bind to multiple ports
- Ports are allocated first-come first-serve and "reserved" ports for known modules can be bound when the chain is first started

## Technical Specification

### Store keys

(main part of standard document - not all subsections are required)

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Data Structures

```typescript
type CapabilityKey object
```

```typescript
type SourceIdentifier string
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
}
```

#### Transferring ownership of a port

If the host state machine supports object-capability keys, no additional protocol is necessary, since the port reference is a bearer capability.

```typescript
function transferPort(id: Identifier, newOwner: Identifier) {
}
```

#### Releasing a port

```typescript
function releasePort(id: Identifier) {
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
