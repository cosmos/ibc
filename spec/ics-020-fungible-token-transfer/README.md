---
ics: 20
title: Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-07-15 
modified: 2019-08-25
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe multi-chain denomination handling with permissionless channel opening. This logic constitutes a "fungible token transfer bridge module", interfacing between the IBC relayer module and an existing asset tracking module on the host state machine.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilise an asset issued on one chain on another chain, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing chain. This application-layer standard describes a protocol for transferring fungible tokens between chains connected with IBC which preserves asset fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

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

Only one packet data type, `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, receiving account, and whether the sending chain is the source of the asset, is required.

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
  source: boolean
}
```

The fungible token transfer bridge module tracks escrow addresses associated with particular channels in state. Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "fungible token transfer bridge" module with access to a bank module and to the IBC relayer module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  relayerModule.bindPort("bank", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
}
```

Once the `setup` function has been called, channels can be created through the IBC relayer module between instances of the fungible token transfer module on separate chains.

#### Relayer module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The other module is bound to the "bank" port.
- The channel being created is unordered.
- The version string is empty.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // only allow channels to "bank" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // version not used at present
  abortTransactionUnless(version === "")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // version not used at present
  abortTransactionUnless(version === "")
  abortTransactionUnless(counterpartyVersion === "")
  // only allow channels to "bank" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "bank")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // version not used at present
  abortTransactionUnless(version === "")
  // port has already been validated
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

##### Packet relay

In plain English, between chains `A` and `B`:

- When acting as the source zone, the bridge module escrows an existing local asset denomination on the sending chain and mints vouchers on the receiving chain.
- When acting as the sink zone, the bridge module burns local vouchers on the sending chains and unescrows the local asset denomination on the receiving chain.
- When a packet times-out, local assets are unescrowed back to the sender or vouchers minted back to the sender appropriately.
- No acknowledgement data is necessary.

`createOutgoingPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function createOutgoingPacket(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  source: boolean) {
  if source {
    // sender is source chain: escrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.sourceChannel]
    // construct receiving denomination, check correctness
    prefix = "{packet/destPort}/{packet.destChannel}"
    abortTransactionUnless(denomination.slice(0, len(prefix)) === prefix)
    // escrow source tokens (assumed to fail if balance insufficient)
    bank.TransferCoins(sender, escrowAccount, denomination.slice(len(prefix)), amount)
  } else {
    // receiver is source chain, burn vouchers
    // construct receiving denomination, check correctness
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(denomination.slice(0, len(prefix)) === prefix)
    // burn vouchers (assumed to fail if balance insufficient)
    bank.BurnCoins(sender, denomination, amount)
  }
  FungibleTokenPacketData data = FungibleTokenPacketData{denomination, amount, sender, receiver, source}
  handler.sendPacket(packet)
}
```

`onRecvPacket` is called by the relayer module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet): bytes {
  FungibleTokenPacketData data = packet.data
  if data.source {
    // sender was source chain: mint vouchers
    // construct receiving denomination, check correctness
    prefix = "{packet/destPort}/{packet.destChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // mint vouchers to receiver (assumed to fail if balance insufficient)
    bank.MintCoins(data.receiver, data.denomination, data.amount)
  } else {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // construct receiving denomination, check correctness
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // unescrow tokens to receiver (assumed to fail if balance insufficient)
    bank.TransferCoins(escrowAccount, data.receiver, data.denomination.slice(len(prefix)), data.amount)
  }
  return 0x
}
```

`onAcknowledgePacket` is called by the relayer module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // nothing is necessary, likely this will never be called since it's a no-op
}
```

`onTimeoutPacket` is called by the relayer module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  if data.source {
    // sender was source chain, unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // construct receiving denomination, check correctness
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // unescrow tokens back to sender
    bank.TransferCoins(escrowAccount, data.sender, data.denomination.slice(len(prefix)), data.amount)
  } else {
    // receiver was source chain, mint vouchers
    // construct receiving denomination, check correctness
    prefix = "{packet/sourcePort}/{packet.sourceChannel}"
    abortTransactionUnless(data.denomination.slice(0, len(prefix)) === prefix)
    // mint vouchers back to sender
    bank.MintCoins(data.sender, data.denomination, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // can't happen, only unordered channels allowed
}
```

#### Reasoning

##### Correctness

This implementation preserves both fungibility & supply.

Fungibility: If tokens have been sent to the counterparty chain, they can be redeemed back in the same denomination & amount on the source chain.

Supply: Redefine supply as unlocked tokens. All send-recv pairs sum to net zero. Source chain can change supply.

##### Multi-chain notes

This does not yet handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B, chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

#### Optional addenda

- Each chain, locally, could elect to keep a lookup table to use short, user-friendly local denominations in state which are translated to and from the longer denominations when sending and receiving packets. 
- Additional restrictions may be imposed on which other machines may be connected to & which channels may be established.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

A future version of this standard could use a different version in the channel handshake.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

15 July 2019 - Draft written
29 July 2019 - Major revisions; cleanup
25 August 2019 - Major revisions, more cleanup

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
