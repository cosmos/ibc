---
ics: 21
title: Permissioned Token Transfer
stage: draft
category: IBC/APP
kind: instantiation
author: John Letey <john@nobleassets.xyz>, Daniel Kanefsky <dan@nobleassets.xyz>
created: 2024-06-14
modified: 2024-06-14
requires: 25, 26
required-by: (optional list of ics numbers)
implements: (optional list of ics numbers)
version compatibility: (optional list of compatible implementations' releases)
---

> This standard document follows the same design principles of [ICS 20](../ics-020-fungible-token-transfer) and inherits most of its content therefrom.

## Synopsis

(high-level description of and rationale for specification)

### Motivation

(rationale for existence of standard)

### Definitions

- `Host Chain`: The chain where the permissioned tokens are considered native. The host chain facilitates connections to mirror chains, and ensures the propagation of token specific allowlists and blocklists.
- `Mirror Chain`: The chain receiving the permissioned tokens and issuing *controlled* voucher tokens. It is up to the mirror chain to enforce the propagated allowlists and blocklists.
- `Allowlist`: A group of addresses that are allowed to interact with a permissioned token. Any address not on the allowlist is forbidden to interact with the token.
- `Blocklist`: A group of addresses that aren't allowed to interact with a permissioned token. Any address not on the blocklist is allowed to interact with the token.

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

### Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

## Technical Specification

(main part of standard document - not all subsections are required)

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Data Structures

We utilize the existing [ICS 20 `FungibleTokenPacketData`](../ics-020-fungible-token-transfer/README.md#data-structures) structure to transfer tokens over ICS 21 channels in order is to maintain client compatibility. Note that the Host Chain will block all transfers of permissioned token transfers over non-ICS 21 channels.

Additionally, we define a new packet data type for the propagation of token allowlists and blacklists.

```typescript
interface PermissionPropagationPacketData {
  denom: string
  allowlist_additions: string[]
  allowlist_removals: string[]
  blocklist_additions: string[]
  blocklist_removals: string[]
}
```

### Sub-protocols

(sub-protocols, if applicable)

### Port & channel setup

An ICS 21 Host module must always bind to a port with the id `ics21host`. Mirror Chains will bind to ports dynamically, as specified in the identifier format [section](#identifier-formats).

The example below assumes a module is implementing the entire `ICS21HostModule` interface. The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialized) to bind to the appropriate port.

```typescript
function setup() {
  capability = routingModule.bindPort("ics21host", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onChanUpgradeInit, // read-only
    onChanUpgradeTry,  // read-only
    onChanUpgradeAck,  // read-only
    onChanUpgradeOpen,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
  claimCapability("port", capability)
}
```

Once the `setup` function has been called, channels can be created via the IBC routing module.

### Identifier formats

TBD

### Properties & Invariants

(properties & invariants maintained by the protocols specified, if applicable)

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version `"ics21-1"` in the channel handshake.

A future version of this standard could use a different version in the channel handshake, and safely alter the packet data format & packet handler semantics.

## Example Implementations

- An implementation of ICS 21 Host & Mirror in Golang can be found [here](https://github.com/noble-assets/ics21).
- An implementation of ICS 21 Mirror in [CosmWasm](https://cosmwasm.com) can be found [here](https://github.com/noble-assets/cw-ics21).

## History

(changelog and notable inspirations / references)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
