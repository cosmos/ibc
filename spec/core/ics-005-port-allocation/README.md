---
ics: 5
title: Port Allocation
stage: Draft
requires: 24
required-by: 4
category: IBC/TAO
kind: interface
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-06-20
modified: 2019-08-25
---

## Synopsis

This standard specifies the port allocation system by which modules can bind to uniquely named ports allocated by the IBC handler.
Ports can then be used to open channels and can be transferred or later released by the module which originally bound to them.

### Motivation

The interblockchain communication protocol is designed to facilitate module-to-module traffic, where modules are independent, possibly mutually distrusted, self-contained
elements of code executing on sovereign ledgers. In order to provide the desired end-to-end semantics, the IBC handler must permission channels to particular modules.
This specification defines the *port allocation and ownership* system which realises that model.

Conventions may emerge as to what kind of module logic is bound to a particular port name, such as "bank" for fungible token handling or "staking" for interchain collateralisation.
This is analogous to port 80's common use for HTTP servers — the protocol cannot enforce that particular module logic is actually bound to conventional ports, so
users must check that themselves. Ephemeral ports with pseudorandom identifiers may be created for temporary protocol handling.

Modules may bind to multiple ports and connect to multiple ports bound to by another module on a separate machine. Any number of (uniquely identified) channels can utilise a single
port simultaneously. Channels are end-to-end between two ports, each of which must have been previously bound to by a module, which will then control that end of the channel.

Optionally, the host state machine can elect to expose port binding only to a specially-permissioned module manager,
by generating a capability key specifically for the ability to bind ports. The module manager
can then control which ports modules can bind to with a custom rule-set, and transfer ports to modules only when it
has validated the port name & module. This role can be played by the routing module (see [ICS 26](../ics-026-routing-module)).

### Definitions

`Identifier`, `get`, `set`, and `delete` are defined as in [ICS 24](../ics-024-host-requirements).

A *port* is a particular kind of identifier which is used to permission channel opening and usage to modules.

A *module* is a sub-component of the host state machine independent of the IBC handler. Examples include Ethereum smart contracts and Cosmos SDK & Substrate modules.
The IBC specification makes no assumptions of module functionality other than the ability of the host state machine to use object-capability or source authentication to permission ports to modules.

### Desired Properties

- Once a module has bound to a port, no other modules can use that port until the module releases it
- A module can, on its option, release a port or transfer it to another module
- A single module can bind to multiple ports at once
- Ports are allocated first-come first-serve, and "reserved" ports for known modules can be bound when the chain is first started

As a helpful comparison, the following analogies to TCP are roughly accurate:

| IBC Concept             | TCP/IP Concept            | Differences                                                           |
| ----------------------- | ------------------------- | --------------------------------------------------------------------- |
| IBC                     | TCP                       | Many, see the architecture documents describing IBC                   |
| Port (e.g. "bank")      | Port (e.g. 80)            | No low-number reserved ports, ports are strings                       |
| Module (e.g. "bank")    | Application (e.g. Nginx)  | Application-specific                                                  |
| Client                  | -                         | No direct analogy, a bit like L2 routing and a bit like TLS           |
| Connection              | -                         | No direct analogy, folded into connections in TCP                     |
| Channel                 | Connection                | Any number of channels can be opened to or from a port simultaneously |

## Technical Specification

### Data Structures

The host state machine MUST support either object-capability reference or source authentication for modules.

In the former object-capability case, the IBC handler must have the ability to generate *object-capabilities*, unique, opaque references
which can be passed to a module and will not be duplicable by other modules. Two examples are store keys as used in the Cosmos SDK ([reference](https://github.com/cosmos/cosmos-sdk/blob/97eac176a5d533838333f7212cbbd79beb0754bc/store/types/store.go#L275))
and object references as used in Agoric's Javascript runtime ([reference](https://github.com/Agoric/SwingSet)).

```typescript
type CapabilityKey object
```

`newCapability` must take a name and generate a unique capability key, such that the name is locally mapped to the capability key and can be used with `getCapability` later.

```typescript
function newCapability(name: string): CapabilityKey {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`authenticateCapability` must take a name & a capability and check whether the name is locally mapped to the provided capability. The name can be untrusted user input.

```typescript
function authenticateCapability(name: string, capability: CapabilityKey): bool {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`claimCapability` must take a name & a capability (provided by another module) and locally map the name to the capability, "claiming" it for future usage.

```typescript
function claimCapability(name: string, capability: CapabilityKey) {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`getCapability` must allow a module to lookup a capability which it has previously created or claimed by name.

```typescript
function getCapability(name: string): CapabilityKey {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

`releaseCapability` must allow a module to release a capability which it owns.

```typescript
function releaseCapability(capability: CapabilityKey) {
  // provided by host state machine, e.g. ADR 3 / ScopedCapabilityKeeper in Cosmos SDK
}
```

In the latter source authentication case, the IBC handler must have the ability to securely read the *source identifier* of the calling module,
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

`newCapability`, `authenticateCapability`, `claimCapability`, `getCapability`, and `releaseCapability` are then implemented as follows:

```typescript
function newCapability(name: string): CapabilityKey {
  return callingModuleIdentifier()
}
```

```typescript
function authenticateCapability(name: string, capability: CapabilityKey) {
  return callingModuleIdentifier() === name
}
```

```typescript
function claimCapability(name: string, capability: CapabilityKey) {
  // no-op
}
```

```typescript
function getCapability(name: string): CapabilityKey {
  // not actually used
  return nil
}
```

```typescript
function releaseCapability(capability: CapabilityKey) {
  // no-op
}
```

#### Store paths

`portPath` takes an `Identifier` and returns the store path under which the object-capability reference or owner module identifier associated with a port should be stored.

```typescript
function portPath(id: Identifier): Path {
    return "ports/{id}"
}
```

### Sub-protocols

#### Identifier validation

Owner module identifier for ports are stored under a unique `Identifier` prefix.
The validation function `validatePortIdentifier` MAY be provided.

```typescript
type validatePortIdentifier = (id: Identifier) => boolean
```

If not provided, the default `validatePortIdentifier` function will always return `true`. 


#### Binding to a port

The IBC handler MUST implement `bindPort`. `bindPort` binds to an unallocated port, failing if the port has already been allocated.

If the host state machine does not implement a special module manager to control port allocation, `bindPort` SHOULD be available to all modules. If it does, `bindPort` SHOULD only be callable by the module manager.

```typescript
function bindPort(id: Identifier): CapabilityKey {
    abortTransactionUnless(validatePortIdentifier(id))
    abortTransactionUnless(getCapability(portPath(id)) === null)
    capability = newCapability(portPath(id))
    return capability
}
```

#### Transferring ownership of a port

If the host state machine supports object-capabilities, no additional protocol is necessary, since the port reference is a bearer capability.

#### Releasing a port

The IBC handler MUST implement the `releasePort` function, which allows a module to release a port such that other modules may then bind to it.

`releasePort` SHOULD be available to all modules.

> Warning: releasing a port will allow other modules to bind to that port and possibly intercept incoming channel opening handshakes. Modules should release ports only when doing so is safe.

```typescript
function releasePort(id: Identifier, capability: CapabilityKey) {
    abortTransactionUnless(authenticateCapability(portPath(id), capability))
    releaseCapability(capability)
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

Jun 29, 2019 - Initial draft

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
