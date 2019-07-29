---
ics: 20
title: Fungible Token Transfer
stage: draft
category: ibc-app
requires: 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-07-15 
modified: 2019-07-29
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe mult-chain denomination handling with permissionless channel opening.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilize an asset issued on one chain on another chain, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing chain. This application-layer standard describes a protocol for transferring fungible tokens between chains connected with IBC which preserves asset fungibility, preserves asset ownership, contains Byzantine faults, and requires no additional permissioning.

### Definitions

The IBC handler interface & IBC relayer module interface are as defined in [ICS 25](../ics-025-handler-interface) and [ICS 26](../ics-026-relayer-module), respectively.

### Desired Properties

- Preservation of fungibility (two-way peg).
- Preservation of total supply (constant or inflationary on a single source chain & module).
- Permissionless token transfers, no need to whitelist connections, modules, or denominations.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-inflation of tokens originating on chain `A`, on chain `A`, as a result of chain `B`'s Byzantine behaviour (though any users who sent tokens to chain `B` may be at risk).

## Technical Specification

### Data Structures

Only one packet data type, `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, and receiving account, is required.

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
}
```

### Subprotocols

The subprotocols described herein should be implemented in a "bank-ibc-bridge" module with access to a bank module and to the IBC relayer module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialized) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  relayerModule.bindPort("bank", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseConfirm,
    onRecvPacket,
    onTimeoutPacket,
  })
  escrowAddress = newAddress()
}
```

Once the `setup` function has been called, channels can be created through the IBC relayer module between instances of the fungible token transfer module on separate chains.

#### Sending packets

In plain English, between chains `A` and `B`:
- Chain `A` bank module accepts new connections / channels from any module on another chain.
- Denominations sent from chain `B` are prefixed with the connection identifier and the name of the counterparty port of `B`, e.g. `0x1234/bank` for the bank module on chain `B` with connection identifier `0x1234`. No supply limits are enforced, but the bank module on chain `A` tracks the amount of each denomination sent by chain `B` and keeps it in a store location which can be queried / proven.
- Coins sent by chain `A` to chain `B` are prefixed in the same way when sent (`0x4567/bank` if the bank module is running on a hub with connection identifier `0x4567`). Outgoing supply is tracked in a store location which can be queried and proven. Chain `B` is allowed to send back coins prefixed with `0x4567/bank` only up to the amount which has been sent to it.
- Each chain, locally, can keep a lookup table to use short, user-friendly local denominations in state which are translated to and from the longer denominations when sending and receiving packets.

```typescript
function handleFungibleTokenPacketSend(denomination: string, amount: uint256, receiver: string) {
  // transfer coins from user
  // construct receiving denomination
  // escrow coins in amount (if source) or unescrow (if destination)
  // send packet
  data = FungibleTokenPacketData{denomination, amount, receiver}
  relayerModule.sendPacket(data)
}
```

#### Relayer module callbacks

```typescript
function onChanOpenInit(
  portIdentifier: Identifier, channelIdentifier: Identifier, counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier, connectionHops: [Identifier], nextTimeoutHeight: uint64): boolean {
  // only allow channels to "bank" port on counterparty chain
  return counterpartyPortIdentifier === "bank"
}
```

```typescript
function onChanOpenTry(
  portIdentifier: Identifier, channelIdentifier: Identifier, counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier, connectionHops: [Identifier], nextTimeoutHeight: uint64): boolean {
  // only allow channels to "bank" port on counterparty chain
  return counterpartyPortIdentifier === "bank"
}
```

```typescript
function onChanOpenAck(portIdentifier: Identifier, channelIdentifier: Identifier, nextTimeoutHeight: uint64): boolean {
  return true
}
```

```typescript
function onChanOpenConfirm(portIdentifier: Identifier, channelIdentifier: Identifier): boolean {
  return true
}
```

```typescript
function onChanOpenTimeout(portIdentifier: Identifier, channelIdentifier: Identifier): void {
  // ??
}
```

```typescript
function onChanCloseConfirm(portIdentifier: Identifier, channelIdentifier: Identifier): void {
  return true
}
```

```typescript
function onRecvPacket(packet: Packet): bytes {
  FungibleTokenPacketData data = packet.data
  // verify receiving denomination
  // unescrow coins in amount (if source) or escrow (if destination)
  // transfer coins to user
  return 0x
}
```

```typescript
function onTimeoutPacket(packet: Packet): boolean {
  FungibleTokenPacketData data = packet.data
  // refund tokens
}
```

#### Reasoning

##### Correctness

This implementation preserves both fungibility & supply.

Fungibility: send back to other chain. If had previously sent, can send back.

Supply: Redefine supply as unlocked tokens. All send-recv pairs are net zero. Source chain can change supply.

##### Multi-chain notes

This does not yet handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B, chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

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
29 July 2019 - Major revisions; cleanup

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
