---
ics: 21
title: Non-Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Christopher Goes <cwgoes@interchain.berlin>, Haifeng Xi <haifeng@bianjie.ai>
created: 2021-11-10 
modified: 2021-11-10
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of non-fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe multi-chain denomination handling with permissionless channel opening. This logic constitutes a "non-fungible token transfer bridge module", interfacing between the IBC routing module and an existing asset tracking module on the host state machine.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilise an asset issued on one chain on another chain, perhaps to make use of additional features such as exchange or privacy protection, while retaining non-fungibility with the original asset on the issuing chain. This application-layer standard describes a protocol for transferring non-fungible tokens between chains connected with IBC which preserves asset non-fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

### Definitions

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

### Desired Properties

- Preservation of uniqueness (two-way peg).
- Preservation of total supply (maintained on a single source chain & module).
- Permissionless token transfers, no need to whitelist connections, modules, or denominations.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-inflation of tokens originating on chain `A`, as a result of chain `B`'s Byzantine behaviour (though any users who sent tokens to chain `B` may be at risk).

## Technical Specification

### Data Structures

Only one packet data type is required: `NonFungibleTokenPacketData`, which specifies the denomination, amount, sending account, and receiving account.

```typescript
interface NonFungibleTokenPacketData {
  classId: string
  classUri: string
  tokenId: string
  tokenUri: string
  sender: string
  receiver: string
}
```

As tokens are sent across chains using the ICS 21 protocol, they begin to accrue a record of channels for which they have been transferred across. This information is encoded into the `classId` field. 

The ics21 token classes are represented in the form `{ics21Port}/{ics21Channel}/{classId}`, where `ics21Port` and `ics21Channel` are an ics21 port and channel on the current chain for which the token exists. The prefixed port and channel pair indicate which channel the token was previously sent through. If `{classId}` contains `/`, then it must also be in the ics21 form which indicates that this token has a multi-hop record. Note that this requires that the `/` (slash character) is prohibited in non-IBC token denomination names.

A sending chain may be acting as a source or sink zone. When a chain is sending tokens across a port and channel which are not equal to the last prefixed port and channel pair, it is acting as a source zone. When tokens are sent from a source zone, the destination port and channel will be prefixed onto the `classId` (once the tokens are received) adding another hop to a tokens record. When a chain is sending tokens across a port and channel which are equal to the last prefixed port and channel pair, it is acting as a sink zone. When tokens are sent from a sink zone, the last prefixed port and channel pair on the `classId` is removed (once the tokens are received), undoing the last hop in the tokens record. A more complete explanation is present in the ibc-go implementation (TBD).

The acknowledgement data type describes whether the transfer succeeded or failed, and the reason for failure (if any).

```typescript
type NonFungibleTokenPacketAcknowledgement = NonFungibleTokenPacketSuccess | NonFungibleTokenPacketError;

interface NonFungibleTokenPacketSuccess {
  // This is binary 0x01 base64 encoded
  success: "AQ=="
}

interface NonFungibleTokenPacketError {
  error: string
}
```

Note that both the `NonFungibleTokenPacketData` as well as `NonFungibleTokenPacketAcknowledgement` must be JSON-encoded (not Protobuf encoded) when they serialized into packet data.

The non-fungible token transfer bridge module tracks escrow addresses associated with particular channels in state. Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "non-fungible token transfer bridge" module with access to the NFT asset tracking module and the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("nft", ModuleCallbacks{
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
  claimCapability("port", capability)
}
```

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the non-fungible token transfer module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels
to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion
that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics21-1`.

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
  // assert that version is "ics21-1"
  abortTransactionUnless(version === "ics21-1")
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
  // assert that version is "ics21-1"
  abortTransactionUnless(version === "ics21-1")
  abortTransactionUnless(counterpartyVersion === "ics21-1")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // port has already been validated
  // assert that version is "ics21-1"
  abortTransactionUnless(version === "ics21-1")
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated, version has already been validated
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

- When acting as the source zone, the bridge module escrows an existing local non-fungible token on the sending chain and mints a corresponding voucher on the receiving chain.
- When acting as the sink zone, the bridge module burns the local voucher on the sending chain and unescrows the local non-fungible token on the receiving chain.
- When a packet times-out, local non-fungible tokens are unescrowed back to the sender or vouchers minted back to the sender appropriately.
- Acknowledgement data is used to handle failures, such as invalid destination accounts. Returning
  an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending chain to take appropriate action based on the nature of the failure.

`createOutgoingPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function createOutgoingPacket(
  classId: string,
  tokenId: string,
  sender: string,
  receiver: string,
  source: boolean,
  destPort: string,
  destChannel: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64) {
  prefix = "{sourcePort}/{sourceChannel}/"
  // we are the source if the denomination is not prefixed
  source = classId.slice(0, len(prefix)) !== prefix
  if source {
    // determine escrow account
    escrowAccount = channelEscrowAddresses[sourceChannel]
    // escrow source token (assumed to fail if sender is not owner)
    nft.Transfer(sender, escrowAccount, classId, tokenId)
  } else {
    // receiver is source chain, burn voucher (assumed to fail if sender is not owner)
    bank.Burn(sender, classId, tokenId)
  }
  Class class = nft.getClass(classId)
  NFT token = nft.getNFT(classId, tokenId)
  NonFungibleTokenPacketData data = NonFungibleTokenPacketData{classId, class.getUri(), tokenId, token.getUri(), sender, receiver}
  handler.sendPacket(Packet{timeoutHeight, timeoutTimestamp, destPort, destChannel, sourcePort, sourceChannel, data}, getCapability("port"))
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  NonFungibleTokenPacketData data = packet.data
  // construct default acknowledgement of success
  NonFungibleTokenPacketAcknowledgement ack = NonFungibleTokenPacketAcknowledgement{true, null}
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the packets were prefixed by the sending chain
  source = data.classId.slice(0, len(prefix)) === prefix
  if source {
    // receiver is source chain: unescrow token
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // unescrow token to receiver
    err = nft.Transfer(escrowAccount, data.receiver, data.classId.slice(len(prefix)), data.tokenId)
    if (err !== nil)
      ack = NonFungibleTokenPacketAcknowledgement{false, "transfer nft failed"}
  } else {
    prefix = "{packet.destPort}/{packet.destChannel}/"
    prefixedClassId = prefix + data.classId
    // sender was source, mint voucher to receiver
    err = nft.Mint(data.receiver, prefixedClassId, data.tokenId)
    if (err !== nil)
      ack = NonFungibleTokenPacketAcknowledgement{false, "mint nft failed"}
  }
  return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // if the transfer failed, refund the tokens
  if (!ack.success)
    refundToken(packet)
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundToken(packet)
}
```

`refundToken` is called by both `onAcknowledgePacket`, on failure, and `onTimeoutPacket`, to refund escrowed token to the original sender.

```typescript
function refundToken(packet: Packet) {
  NonFungibleTokenPacketData data = packet.data
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the classId is not prefixed
  source = data.classId.slice(0, len(prefix)) !== prefix
  if source {
    // sender was source chain, unescrow tokens back to sender
    escrowAccount = channelEscrowAddresses[packet.srcChannel]
    nft.Transfer(escrowAccount, data.sender, data.classId, data.tokenId)
  } else {
    // receiver was source chain, mint voucher back to sender
    bank.Mint(data.sender, data.classId, data.tokenId)
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

This implementation preserves both uniqueness & supply.

Uniqueness: If tokens have been sent to the counterparty chain, they can be redeemed back in the same `classId` & `tokenId` on the source chain.

Supply: Redefine supply as unlocked tokens. All send-recv pairs for any given token class sum to net zero. Source chain can change supply.

##### Multi-chain notes

This specification does not directly handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B (and the `classId` will be "{portOnD}/{channelOnD}/{portOnB}/{channelOnB}/classId"), chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

In order to track all of the tokens moving around the network of chains in various paths, it may be helpful for a particular chain to implement a registry which will track the "global" source chain for each `classId`. End-user service providers (such as wallet authors) may want to integrate such a registry or keep their own mapping of canonical source chains and human-readable names in order to improve UX.

#### Optional addenda

- Each chain, locally, could elect to keep a lookup table to use short, user-friendly local `classId`s in state which are translated to and from the longer `classId`s when sending and receiving packets. 
- Additional restrictions may be imposed on which other machines may be connected to & which channels may be established.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version "ics21-1" in the channel handshake.

A future version of this standard could use a different version in the channel handshake, and safely alter the packet data format & packet handler semantics.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Nov 10, 2021 - Initial draft adapted from ICS20 spec

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
