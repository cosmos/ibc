## Synopsis

Extends [ICS-20 fungible token transfer](../ics-020-fungible-token-transfer) with multiple tokens in same packet.

### Motivation

Allow to transfer multiple token at same packet.

### Desired Properties

- All transfers success or all fail at same time.

## Technical Specification

All omitted details are exactly same as for ICS-20.

### Data Structures

```typescript
interface Coin {
  denom: string
  amount: uint256
}

interface FungibleTokensPacketData {
  sender: string
  // denom ordered set of assets
  funds: Coin[]
  receiver: string
  memo: string
}
```

### Protocol

Same as of ICS-20, but in plural form.

In case of failure of any assets, for example in case of invalid denomination,
all success assets transfers done during packet handling are rolled back.

```typescript
function sendFungibleTokens(
  localFunds: Coin[],
  sender: string,
  receiver: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64): uint64 {

    // does same operation regarding prefixing/transfer/burn as ICS-20 in per token level
    funds = toIbcPrefixedCoins(sourcePort,sourceChannel, funds)
    
    datas = FungibleTokensPacketData{funds, sender, receiver}
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
  FungibleTokensPacketData data = packet.data
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}

  transactionBegin()
  for (let coin of data.funds) {    
    err = onRecvIcs20Packet(packet, coin);
    if (err != nil) {
      transactionRollback()
      ack = FungibleTokenPacketAcknowledgement{false, err}
    }
  }
  transactionCommit()
  
  return ack
```

```typescript
function refundTokens(packet: Packet) {
  FungibleTokensPacketData data = packet.data
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // same as ICS-20 on per token basis
  refundTokensDoesNotFail(data.funds, packet.sourcePort, packet.sourceChannel)
}
```

## Backwards Compatibility

Version to use is "ics20-2". If multi token version not supported, packet acknowledgement errors. 