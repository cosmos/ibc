---
ics: 20
title: Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Christopher Goes <cwgoes@interchain.berlin>
created: 2019-07-15 
modified: 2020-02-24
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe multi-chain denomination handling with permissionless channel opening. This logic constitutes a "fungible token transfer bridge module", interfacing between the IBC routing module and an existing asset tracking module on the host state machine.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilise an asset issued on one chain on another chain, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing chain. This application-layer standard describes a protocol for transferring fungible tokens between chains connected with IBC which preserves asset fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

### Definitions

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../ics-025-handler-interface) and [ICS 26](../ics-026-routing-module), respectively.

### Desired Properties

- Preservation of fungibility (two-way peg).
- Preservation of total supply (constant or inflationary on a single source chain & module).
- Permissionless token transfers, no need to whitelist connections, modules, or denominations.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-inflation of tokens originating on chain `A`, as a result of chain `B`'s Byzantine behaviour (though any users who sent tokens to chain `B` may be at risk).
- Opt-in, decentralized, pseudonymous relayer incentivization ([See ICS20-2](#Extension-ICS20-2))

## Technical Specification

### Data Structures

Only one packet data type is required: `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, and receiving account.

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
}
```

As tokens are sent across chains using the ICS 20 protocol, they begin to accrue a record of channels for which they have been transferred across. This information is encoded into the `denomination` field. 

The ics20 token denominations are represented the form `{ics20Port}/{ics20Channel}/{denom}`, where `ics20Port` and `ics20Channel` are an ics20 port and channel on the current chain for which the funds exist. The prefixed port and channel pair indicate which channel the funds were previously sent through. If `{denom}` contains `/`, then it must also be in the ics20 form which indicates that this token has a multi-hop record. Note that this requires that the `/` (slash character) is prohibited in non-IBC token denomination names.

A sending chain may be acting as a source or sink zone. When a chain is sending tokens across a port and channel which are not equal to the last prefixed port and channel pair, it is acting as a source zone. When tokens are sent from a source zone, the destination port and channel will be prefixed onto the denomination (once the tokens are received) adding another hop to a tokens record. When a chain is sending tokens across a port and channel which are equal to the last prefixed port and channel pair, it is acting as a sink zone. When tokens are sent from a sink zone, the last prefixed port and channel pair on the denomination is removed (once the tokens are received), undoing the last hop in the tokens record. A more complete explanation is [present in the ibc-go implementation](https://github.com/cosmos/ibc-go/blob/457095517b7832c42ecf13571fee1e550fec02d0/modules/apps/transfer/keeper/relay.go#L18-L49).

The acknowledgement data type describes whether the transfer succeeded or failed, and the reason for failure (if any).

```typescript
type FungibleTokenPacketAcknowledgement = FungibleTokenPacketSuccess | FungibleTokenPacketError;

interface FungibleTokenPacketSuccess {
  // This is binary 0x01 base64 encoded
  success: "AQ=="
}

interface FungibleTokenPacketError {
  error: string
}
```

Note that both the `FungibleTokenPacketData` as well as `FungibleTokenPacketAcknowledgement` must be JSON-encoded (not Protobuf encoded) when they serialized into packet data. Also note that `uint256` is string encoded when converted to JSON, but must be a valid decimal number of the form `[0-9]+`.

The fungible token transfer bridge module tracks escrow addresses associated with particular channels in state. Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "fungible token transfer bridge" module with access to a bank module and to the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("bank", ModuleCallbacks{
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

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the fungible token transfer module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels
to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion
that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics20-1`.

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
  // assert that version is "ics20-1"
  abortTransactionUnless(version === "ics20-1")
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
  // assert that version is "ics20-1"
  abortTransactionUnless(version === "ics20-1")
  abortTransactionUnless(counterpartyVersion === "ics20-1")
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
  // assert that version is "ics20-1"
  abortTransactionUnless(version === "ics20-1")
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

- When acting as the source zone, the bridge module escrows an existing local asset denomination on the sending chain and mints vouchers on the receiving chain.
- When acting as the sink zone, the bridge module burns local vouchers on the sending chains and unescrows the local asset denomination on the receiving chain.
- When a packet times-out, local assets are unescrowed back to the sender or vouchers minted back to the sender appropriately.
- Acknowledgement data is used to handle failures, such as invalid denominations or invalid destination accounts. Returning
  an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending chain
  to take appropriate action based on the nature of the failure.

`createOutgoingPacket` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function createOutgoingPacket(
  denomination: string,
  amount: uint256,
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
  source = denomination.slice(0, len(prefix)) !== prefix
  if source {
    // determine escrow account
    escrowAccount = channelEscrowAddresses[sourceChannel]
    // escrow source tokens (assumed to fail if balance insufficient)
    bank.TransferCoins(sender, escrowAccount, denomination, amount)
  } else {
    // receiver is source chain, burn vouchers
    bank.BurnCoins(sender, denomination, amount)
  }
  FungibleTokenPacketData data = FungibleTokenPacketData{denomination, amount, sender, receiver}
  handler.sendPacket(Packet{timeoutHeight, timeoutTimestamp, destPort, destChannel, sourcePort, sourceChannel, data}, getCapability("port"))
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the packets were prefixed by the sending chain
  source = data.denomination.slice(0, len(prefix)) === prefix
  if source {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // unescrow tokens to receiver (assumed to fail if balance insufficient)
    err = bank.TransferCoins(escrowAccount, data.receiver, data.denomination.slice(len(prefix)), data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
  } else {
    prefix = "{packet.destPort}/{packet.destChannel}/"
    prefixedDenomination = prefix + data.denomination
    // sender was source, mint vouchers to receiver (assumed to fail if balance insufficient)
    err = bank.MintCoins(data.receiver, prefixedDenomination, data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
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
    refundTokens(packet)
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
}
```

`refundTokens` is called by both `onAcknowledgePacket`, on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original sender.

```typescript
function refundTokens(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the denomination is not prefixed
  source = denomination.slice(0, len(prefix)) !== prefix
  if source {
    // sender was source chain, unescrow tokens back to sender
    escrowAccount = channelEscrowAddresses[packet.srcChannel]
    bank.TransferCoins(escrowAccount, data.sender, data.denomination, data.amount)
  } else {
    // receiver was source chain, mint vouchers back to sender
    bank.MintCoins(data.sender, denomination, data.amount)
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

This specification does not directly handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B (and the denomination will be "{portOnD}/{channelOnD}/{portOnB}/{channelOnB}/denom"), chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

In order to track all of the denominations moving around the network of chains in various paths, it may be helpful for a particular chain to implement a registry which will track the "global" source chain for each denomination. End-user service providers (such as wallet authors) may want to integrate such a registry or keep their own mapping of canonical source chains and human-readable names in order to improve UX.

#### Optional addenda

- Each chain, locally, could elect to keep a lookup table to use short, user-friendly local denominations in state which are translated to and from the longer denominations when sending and receiving packets. 
- Additional restrictions may be imposed on which other machines may be connected to & which channels may be established.

## Extension: ICS20-2

The above specifications defines `ics20-1`, which is the first version of the specification. Here we define
ICS20-2 which 

### Goals

- Add support for opt-in, decentralized, pseudonymous relayer incentivization
- Backwards compatibility with ics20-1
- Minimal extra logic needed to maintain backwards compatibility
- Fault-resilient: version implementation errors should not break invariants

### Data Structures

In order to help with backwards compatibility, we define a packet structure for ICS20-2, which is a superset of ICS20-1, such that any valid packet for one protocol, is also a valid packet for the other protocol. The only issue is that the `fee` info from a ICS20-2 packet will be ignored by a ICS20-1 handler.

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
  fee: Maybe<uint256>
}
```

Fee is defined here as some number of tokens to be sent to the address who submitted the IbcReceivePacket on the destination chain. The information of `signer` is available when submitting the packet, but discarded in the application-specific handlers for `ibc-go`. We can simply expose that information to the application to allow it to optionally take action based on who submitted the packet. Note that this means *anyone* can submit a valid packet and we do not hard code an allowed relayer nor force the token sender to select a relayer.

In order to maintain maximum compatibility with `ics20-1`, we will define `amount` and `fee` in such a way that a valid `ics20-2` sender and a valid `ics20-1` recipient will not create or destroy any tokens (just ignore the fee field when set). That means:

- `amount` is the total amount that is escrowed by the sending chain.
- `amount - (fee || 0)` is the total amount to be sent to the `receiver` on the receiving chain
- `fee || 0` is the total amount to be sent to the relayer account (`signer`)

Defining `amount + fee` to be escrowed on the sending chain would potentially cause lost tokens if `fee` were set in the packet sender, but the recipient ignored the field.

### Handshakes

Handshakes work much like `ics20-1` with some extensions.

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics20-1` OR `ics20-2`
- Both version and counterparty_version string are the same

The handshake implementation looks much like the above definition with the additional acceptance of `ics20-2` version string. Note that `ics20-2` implementations should store the channel version data to be used when creating packets to send (whether or not fee field is supported).

### Implementation

This is very similar to the `ics20-1` implementation, except for a few additions about fee handling. Note that `createOutgoingPacket` now needs both the extra `fee` information (set by the blockchain user), as well as the channel `version` (stored when the channel was created):

```typescript
function createOutgoingPacket(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  source: boolean,
  fee: Maybe<uint256>,
  destPort: string,
  destChannel: string,
  sourcePort: string,
  sourceChannel: string,
  version: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64) {
  prefix = "{sourcePort}/{sourceChannel}/"
  // we are the source if the denomination is not prefixed
  source = denomination.slice(0, len(prefix)) !== prefix
  if source {
    // determine escrow account
    escrowAccount = channelEscrowAddresses[sourceChannel]
    // escrow source tokens (assumed to fail if balance insufficient)
    bank.TransferCoins(sender, escrowAccount, denomination, amount)
  } else {
    // receiver is source chain, burn vouchers
    bank.BurnCoins(sender, denomination, amount)
  }
  // ADDED: we never set this field when talking with an ics20-1 chain
  if version == "ics20-1"
    fee = None
  FungibleTokenPacketData data = FungibleTokenPacketData{denomination, amount, sender, receiver, fee}
  handler.sendPacket(Packet{timeoutHeight, timeoutTimestamp, destPort, destChannel, sourcePort, sourceChannel, data}, getCapability("port"))
}
```

Likewise, `onRecvPacket` must distribute the fee (if set). This requires passing in the message signer field into the application:

```typescript
function onRecvPacket(packet: Packet, singer: string) {
  FungibleTokenPacketData data = packet.data
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the packets were prefixed by the sending chain
  source = data.denomination.slice(0, len(prefix)) === prefix
  // ADDED: calculate recipient amount
  toRcpt = data.amount - (data.fee || 0)
  if source {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // unescrow tokens to receiver (assumed to fail if balance insufficient)
    err = bank.TransferCoins(escrowAccount, data.receiver, data.denomination.slice(len(prefix)), toRcpt)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
    // ADDED: handle fee distribution
    if data.fee {
      err = bank.TransferCoins(escrowAccount, signer, data.denomination.slice(len(prefix)), data.fee)
      if (err !== nil)
        ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
    }
  } else {
    prefix = "{packet.destPort}/{packet.destChannel}/"
    prefixedDenomination = prefix + data.denomination
    // sender was source, mint vouchers to receiver (assumed to fail if balance insufficient)
    err = bank.MintCoins(data.receiver, prefixedDenomination, toRcpt)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
    // ADDED: handle fee distribution
    if data.fee {
      err = bank.MintCoins(signer, prefixedDenomination, data.fee)
      if (err !== nil)
        ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
    }
  }
  return ack
}
```

The rest of the logic can remain unchanged. Note that the biggest changes are simply providing the additional information to the packet handlers - channel version to the sending logic, and message signer to the receive logic. Both the acknowledgement and timeout handlers can remain unchanged, as `amount` remains the total amount locked up in the sending chain.

## Backwards Compatibility

All `ics20-1` packets are valid `ics20-2` packets. An `ics20-2` packet received by an `ics20-1` handler may be incorrectly processed (no funds sent to the relayer), but will not break any invariants (total number of tokens escrowed on sender == total number of tokens issued on receiver).

## Forwards Compatibility

This initial standard uses version "ics20-1" in the channel handshake. We also define a backwards-compatible "ics20-2" extension with minimal changes to the handlers.

A future version of this standard could use a different version in the channel handshake,
and safely alter the packet data format & packet handler semantics.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

Jul 15, 2019 - Draft written

Jul 29, 2019 - Major revisions; cleanup

Aug 25, 2019 - Major revisions, more cleanup

Feb 3, 2020 - Revisions to handle acknowledgements of success & failure

Feb 24, 2020 - Revisions to infer source field, inclusion of version string

July 27, 2020 - Re-addition of source field

May 24, 2020 - Added ICS20-2 extension

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
