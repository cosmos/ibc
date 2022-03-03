---
ics: 721
title: Non-Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Haifeng Xi <haifeng@bianjie.ai>
created: 2021-11-10
modified: 2022-02-17
---

> This standard document follows the same design principles of [ICS 20](../ics-020-fungible-token-transfer) and inherits most of its content therefrom, while replacing `bank` module based asset tracking logic with that of the `nft` module.

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of non-fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe multi-chain `classId` handling with permissionless channel opening. This logic constitutes a _non-fungible token transfer bridge module_, interfacing between the IBC routing module and an existing asset tracking module on the host state machine, which could be either a Cosmos-style native module or a smart contract running in a virtual machine.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilize a non-fungible token on a chain other than the chain where the token was originally issued -- perhaps to make use of additional features such as exchange, royalty payment or privacy protection. This application-layer standard describes a protocol for transferring non-fungible tokens between chains connected with IBC which preserves asset non-fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

### Definitions

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

### Desired Properties

- Preservation of non-fungibility and uniqueness (i.e., only one instance of the token is *live* across all the IBC-connected blockchains).
- Permissionless token transfers, no need to whitelist connections, modules, or `classId`s.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-creation of tokens originating on chain `A`, as a result of chain `B`'s Byzantine behavior.

## Technical Specification

### Data Structures

Only one packet data type is required: `NonFungibleTokenPacketData`, which specifies the class id, class uri, token id's, token uri's, sender address, and receiver address.

```typescript
interface NonFungibleTokenPacketData {
  classId: string
  classUri: string
  tokenIds: []string
  tokenUris: []string
  sender: string
  receiver: string
}
```
`classId` uniquely identifies the class/collection which the tokens being transferred belong to in the sending chain. In the case of an ERC-1155 compliant smart contract, for example, this could be a string representation of the top 128 bits of the token ID.

`classUri` is optional, but will be extremely beneficial for cross-chain interoperability with NFT marketplaces like OpenSea, where [class/collection metadata](https://docs.opensea.io/docs/contract-level-metadata) can be added for better user experience.

`tokenIds` uniquely identifies some tokens of the given class that are being transferred.  In the case of an ERC-1155 compliant smart contract, for example, a `tokenId` could be a string representation of the bottom 128 bits of the token ID.

Each `tokenId` has a corresponding entry in `tokenUris`, which refers to an off-chain resource that is typically an immutable JSON file containing the token's metadata.

As tokens are sent across chains using the ICS-721 protocol, they begin to accrue a record of channels across which they have been transferred. This record information is encoded into the `classId` field.

An ICS-721 token class is represented in the form `{ics721Port}/{ics721Channel}/{classId}`, where `ics721Port` and `ics721Channel` identify the channel on the current chain from which the tokens arrived. The prefixed port and channel pair indicate which channel the tokens were previously sent through. If `{classId}` contains `/`, then it must also be in the ICS-721 form which indicates that the tokens have a multi-hop record. Note that this requires that the `/` (slash character) is prohibited in non-IBC token `classId`s.

A sending chain may be acting as a source or sink zone. When a chain is sending tokens across a port and channel which are not equal to the last prefixed port and channel pair, it is acting as a source zone. When tokens are sent from a source zone, the destination port and channel will be prefixed onto the `classId` (once the tokens are received) adding another hop to the tokens record. When a chain is sending tokens across a port and channel which are equal to the last prefixed port and channel pair, it is acting as a sink zone. When tokens are sent from a sink zone, the last prefixed port and channel pair on the `classId` is removed (once the tokens are received), undoing the last hop in the tokens record.

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

Note that both the `NonFungibleTokenPacketData` as well as `NonFungibleTokenPacketAcknowledgement` must be JSON-encoded (not Protobuf encoded) when serialized into packet data.

The non-fungible token transfer bridge module maintains a separate escrow address for each NFT channel.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "non-fungible token transfer bridge" module with access to the NFT asset tracking module and the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port (owned by the module).

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

This specification defines packet handling semantics only, and defines them in such a fashion that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics721-1`.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  version: string) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics721-1"
  abortTransactionUnless(version === "ics721-1")
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
  // assert that version is "ics721-1"
  abortTransactionUnless(version === "ics721-1")
  abortTransactionUnless(counterpartyVersion === "ics721-1")
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string,
  counterpartyChannelIdentifier: string) {
  // port has already been validated
  // assert that version is "ics721-1"
  abortTransactionUnless(counterpartyVersion === "ics721-1")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated, version has already been validated
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // abort and return error to prevent channel closing by user
  abortTransactionUnless(FALSE)
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

- When a non-fungible token is sent away from its source, the bridge module escrows the token on the sending chain and mints a corresponding voucher on the receiving chain.
- When a non-fungible token is sent back toward its source, the bridge module burns the token on the sending chain and unescrows the corresponding locked token on the receiving chain.
- When a packet times out, tokens represented in the packet are either unescrowed or minted back to the sender appropriately -- depending on whether the tokens are being moved away from or back toward their source.
- Acknowledgement data is used to handle failures, such as invalid destination accounts. Returning an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending chain to take appropriate action based on the nature of the failure.

`createOutgoingPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function createOutgoingPacket(
  classId: string,
  tokenIds: []string,
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
  // we are the source if the classId is not prefixed with the sourcePort and sourceChannel
  source = classId.slice(0, len(prefix)) !== prefix
  tokenUris = []
  for (let tokenId in tokenIds) {
    // assert that sender is token owner
    abortTransactionUnless(sender === nft.getOwner(classId, tokenId))
    if source {
      // determine escrow account
      escrowAccount = channelEscrowAddresses[sourceChannel]
      // escrow source token
      nft.Transfer(classId, tokenId, escrowAccount)
    } else {
      // receiver is source chain, burn voucher
      nft.Burn(classId, tokenId)
    }
    tokenUris.push(nft.getNFT(classId, tokenId).getUri())
  }
  NonFungibleTokenPacketData data = NonFungibleTokenPacketData{classId, nft.getClass(classId).getUri(), tokenIds, tokenUris, sender, receiver}
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
  // we are the source if the classId is prefixed with the packet's sourcePort and sourceChannel
  source = data.classId.slice(0, len(prefix)) === prefix
  for (var i in data.tokenIds) {
    if source {
      // receiver is source chain: unescrow token
      err = nft.Transfer(data.classId.slice(len(prefix)), data.tokenIds[i], data.receiver)
      if (err !== nil) {
        ack = NonFungibleTokenPacketAcknowledgement{false, err.Error()}
        break
      }
    } else {
      prefix = "{packet.destPort}/{packet.destChannel}/"
      prefixedClassId = prefix + data.classId
      // sender was source, mint voucher to receiver
      err = nft.Mint(prefixedClassId, data.classUri, data.tokenIds[i], data.tokenUris[i], data.receiver)
      if (err !== nil) {
        ack = NonFungibleTokenPacketAcknowledgement{false, err.Error()}
        break
      }
    }
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

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed out (such that it will not be received on the destination chain).

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
  for (let tokenId in data.tokenIds) {
    // we are the source if the classId is not prefixed with the packet's sourcePort and sourceChannel
    source = data.classId.slice(0, len(prefix)) !== prefix
    if source {
      // sender was source chain, unescrow tokens back to sender
      nft.Transfer(data.classId, tokenId, data.sender)
    } else {
      // receiver was source chain, mint voucher back to sender
      nft.Mint(data.classId, tokenId, data.sender)
    }
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

This implementation preserves token uniqueness.

Uniqueness: If tokens have been sent to the counterparty chain, they can be redeemed back in the same `classId` & `tokenId` on the source chain.

##### Multi-chain notes

This specification does not directly handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B (and the `classId` will be "{portOnD}/{channelOnD}/{portOnB}/{channelOnB}/classId"), chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

In order to track all of the tokens moving around the network of chains in various paths, it may be helpful for a particular chain to implement a registry which will track the "global" source chain for each `classId`. End-user service providers (such as wallet authors) may want to integrate such a registry or keep their own mapping of canonical source chains and human-readable names in order to improve UX.

#### Optional addenda

- Each chain, locally, could elect to keep a lookup table to use short, user-friendly local `classId`s in state which are translated to and from the longer `classId`s when sending and receiving packets.
- Additional restrictions may be imposed on which other machines may be connected to & which channels may be established.

## Further Discussion
Extended and complex use cases such as royalties, marketplaces or permissioned transfers can be supported on top of this specification. Solutions could be modules, hooks, [IBC middleware](../ics-030-middleware) and so on. Designing a guideline for this is out of the scope.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version "ics721-1" in the channel handshake.

A future version of this standard could use a different version in the channel handshake, and safely alter the packet data format & packet handler semantics.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History
| Date          | Description                                          |
| ------------- | ---------------------------------------------------- |
| Nov 10, 2021  | Initial draft adapted from ICS 20 spec               |
| Nov 17, 2021  | Revisions to better accommodate smart contracts      |
| Nov 17, 2021  | Renamed from ICS 21 to ICS 721                       |
| Nov 18, 2021  | Revisions to allow for multiple tokens in one packet |
| Feb 10, 2022  | Revisions to incorporate feedbacks from IBC team     |
| Feb 14, 2022  | Revisions to resolve comments from IBC team          |

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
