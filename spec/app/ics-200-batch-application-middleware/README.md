## Synopsis

This standard document specifies batched execution of packets,
with purpose to allow execute packets in batch, 
have callbacks after each packet from batch handled,
and when all packets from batch handled.

### Motivation

Basic use case execute some transaction after multiple ICS-22 and ICS-721 packets succeed.

Extended use case are other multichannel packets, like governance and swap extensions. 

## Technical Specification

All `Batch` packet packets must be in same relayer message.

Any channel type is supported.



### Data structures


Updates ICS-004 packet in backward compatible way by adding new field: 

```typescript
interface Packet {
  // ... existing packet ...
  batch: BatchReference?
}
```

In case of no `batch` presented, packet behaves as before.  


```typescript
interface BatchReference {
   /// reference to batch packet reference number parent packet is part of
   batchSequence : uint64
   batchChannel: string,
   /// order of 
   order : uint64
}

interface BatchPacketData {  
  tracking: uint8
  memo: string
}
```

So on the wire IBC module should receive from Relayer this ordered set of packets

We could put batch into memo, so this whay for not to use memo for anything else and force to use memo in batch in the end.

```json
batch = {tracking = 5} packet = {sequence = 42, channel = 7}
packet_a = {order = 1, batchSequence = 42, channel = 7}
packet_a = {order = 2, batchSequence = 42, channel = 7}
packet_a = {order = 3, batchSequence = 42, channel = 7}
packet_a = {order = 4, batchSequence = 42, channel = 7}
packet_a = {order = 5, batchSequence = 42, channel = 7}
```

### Technocal Details

```typescript
function onRecvPacket(packet: Packet, relayer: string): bytes {
    /// if case of packet is part of batch it will error:
    /// - but batch not found
    /// - previous packet as per order in batch was not onRecvPacket
    /// - timeout of packet is longer than timeout of batch
    if (packet.batch != null) {
      err = ensureBatch(packet)
      if (err != null) {
        return marshalErrAck(err)
      }
    }

    app_acknowledgement = app.onRecvPacket(packet, relayer)

    markRecvInBatch(packet, app_acknowledgement)
       
    return marshal(ack)
}

function onAcknowledgePacket(packet: Packet, acknowledgement: bytes, relayer: string) {
    app_ack = getAppAcknowledgement(acknowledgement)

    app.onAcknowledgePacket(packet, app_ack, relayer)

    if (packet.batch != null) {
      // acknowledge packet can be in any order
      markAckInBatch(packet)
    }

    /// in case all packets in batch handled ACK packet
    if (isDone(packet.batch)) {
       batch.onAcknowledgePacket(packet.batch, app_ack, packet)
    }
}

function onTimeoutPacket(packet: Packet, relayer: string) {
    app.onTimeoutPacket(packet, relayer)

    if (packet.batch != null) {
      // acknowledge packet can be in any order
      markTimeoutInBatch(packet)
    }

    /// so any packet in batch timeout passed
    if (isBatch(packet)) {
      batch.onTimeoutPacket(packet.batch)
    }
    /// one of subpackjets ended
    else if (isDone(packet.batch)) {
       batch.onAcknowledgePacket(packet.batch)
    }
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

Middleware tacke all

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

### Async packets seqeunce

Could hold off execution of next packet in batch until previous packet ACK forcefully.
But given that implementers if packets and chain are free to choose stragey, sequence may or not be here,
so up to implementer of interpreter to on target chain t dice.s

## Referenes

[Near Transaction](https://docs.near.org/concepts/basics/transactions/overview) is similar to this specification. App packet is action. Receipt is batch packet.