## Synopsis

This standard document specifies batched execution of packets,
with purpose to allow execute packets in one transaction, callbacks when all packets from batch succeed and/or after each packet from batch until final packet.

### Motivation

Basic use case execute some transaction after multiple ICS-20 and ICS-721 packets succeed.

Extended use case are other packets, like governance and swap extensions. 

## Technical Specification

All `Batch` packet packets must be in same relayer message.


Any channel type is supported.

### Data structures

```typescript

interface BatchPacketData {  
  tracking: uint8 
  memo: string
}

interface Packet {
  batch: BatchReference?
}

```

### Good

#### 1. Sender chain

`Batch` packet is sent. `Batch` has ordered list of all `channels` and `sequences` within batch.

`App` packets are sent with `batch` to be `sequence` of `Batch` packet.

#### 2. Receiver chain

`Batch` received.

`App` packets are all received.

On final packet, each of `App` packet is executed in order defined in `batch tracking` of packets.

`Batch` and `App` packets are `ACK` success.

#### 3. Sender chain

Receives all results for each `App` packet.

After all results for `App` packets received, `Batch` packet callbacks configured functions.

#### Bad

In case of `App` received before `Batch` or `Batch` received not in single relayer message with all relevant `Apps`, the all always error ACK.

`Batch` received, but one of `App` packets timeout is less than `Batch` one. All `App` packets and `Batch` timeout.

If any `App` packet error `ACK` in rececingi trnsaction, all packets are errored next. 
If `App` packet errors after some other packets dispatched, than part of packets will success nad part falue.

So there is no atomic transactions.

If some `App` does nor timeouts not `ACK`, `Batch` packet also not timeouts nor `ACK`.

## Backwards Compatibility

`Batch` aware `App` packet will fail until `Batch` transferred. Relayer will burn gas and have to be aware of batches.

`Batch` packet holds off finalizaiton..  


## Alternatives 


### Accumulating batch in storage.

Possible to accumulate bundle on chain, 
so seems better relayer will cook batch bundle off chain.


### Bundle all packets into one batch packet

So it improves allowing to set same timeout and one memo for whole batch,
it is incompatible with existing sequence increments, channel timeout logic, middleware and fees.

### Sync atomic batches

Could hold off execution of next packet in batch until previous packet ACK or error.
But if all packets are well know to be executed in this block, really get sync batch as degenrative case.

## Referenes

[Near Transaction](https://docs.near.org/concepts/basics/transactions/overview) is similar to this specification. Packet is action. Receipt is batch.