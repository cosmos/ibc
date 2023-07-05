## Synopsis

Extends [ICS-20 fungible token transfer](../ics-020-fungible-token-transfer) with multiple tokens in same packet.

### Motivation

Allow to transfer multiple token at same packet.

### Desired Properties

- All transfers success or all fail at same time.

## Technical Specification

### Data Structures

```typescript
interface Coin {
  denom: string
  amount: uint256
}

interface FungibleTokenPacketData {
  sender: string
  // denom ordered set of assets
  funds: Coin[]
  receiver: string
  memo: string
}
```

### Protocol

Same as of ICS-20, but in plural form

In case of failure of any assets, for example in case of invalid denomination,
 

`sendFungibleTokens` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function sendFungibleTokens(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64): uint64 {
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

    // create FungibleTokenPacket data
    data = FungibleTokenPacketData{denomination, amount, sender, receiver}

    // send packet using the interface defined in ICS4
    sequence = handler.sendPacket(
      getCapability("port"),
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      data
    )

    return sequence
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
  source = data.denom.slice(0, len(prefix)) === prefix
  if source {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    // unescrow tokens to receiver (assumed to fail if balance insufficient)
    err = bank.TransferCoins(escrowAccount, data.receiver, data.denom.slice(len(prefix)), data.amount)
    if (err !== nil)
      ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
  } else {
    prefix = "{packet.destPort}/{packet.destChannel}/"
    prefixedDenomination = prefix + data.denom
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
  if (!acknowledgement.success)
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
  source = data.denom.slice(0, len(prefix)) !== prefix
  if source {
    // sender was source chain, unescrow tokens back to sender
    escrowAccount = channelEscrowAddresses[packet.srcChannel]
    bank.TransferCoins(escrowAccount, data.sender, data.denom, data.amount)
  } else {
    // receiver was source chain, mint vouchers back to sender
    bank.MintCoins(data.sender, data.denom, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // can't happen, only unordered channels allowed
}
```

## Backwards Compatibility

Version to use is "ics20-2". If multi token version not supported, packet acknowledgement errors. 