---
ics: 20
title: Fungible Token Transfer
stage: draft
category: ibc-app
requires: 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-07-15 
modified: 2019-07-15
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe mult-chain denomination handling with permissionless channel opening.

### Motivation

### Definitions

### Desired Properties

- Preservation of fungibility (two-way peg).
- Preservation of total supply (constant or inflationary on a single source chain & module).
- Permissionless token transfers, no need to whitelist connections, modules, or denominations.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-inflation of tokens originating on chain `A`, on chain `A`, as a result of chain `B`'s Byzantine behaviour (though any users who sent tokens to chain `B` may be at risk).

## Technical Specification

### Data Structures

Only one packet data type is required:

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
}
```

### Subprotocols

In plain English, between chains `A` and `B`:
- Chain `A` bank module accepts new connections / channels from any module on another chain.
- Denominations sent from chain `B` are prefixed with the hash of the root of trust and the name of the counterparty port of `B`, e.g. `0x1234/bank` for the bank module on chain `B` with root-of-trust hash `0x1234`. No supply limits are enforced, but the bank module on chain `A` tracks the amount of each denomination sent by chain `B` and keeps it in a store location which can be queried / proven.
- Coins sent by chain `A` to chain `B` are prefixed in the same way when sent (`0x4567/bank` if the bank module is running on a hub with root-of-trust hash `0x4567`). Outgoing supply is tracked in a store location which can be queried and proven. Chain `B` is allowed to send back coins prefixed with `0x4567/bank` only up to the amount which has been sent to it.

```typescript
function handleFungibleTokenPacketSend(denomination: string, amount: uint256) {
}
```

```typescript
function handleFungibleTokenPacketRecv(denomination: string, amount: uint256) {
}
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Modules can negotiate packet versions in the channel handshake.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

15 July 2019 - Draft written

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
