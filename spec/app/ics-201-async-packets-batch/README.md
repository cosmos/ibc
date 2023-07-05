## Synopsis

This standard document specifies batched execution of packets,
with purpose to allow execute callbacks when all packets from batch succeed and/or after each packet from batch until final packet.

### Motivation

Basic use case execute some actions after multiple ICS-20 and ICS-721 packets.


## Technical Specification

### Data structures

```typescript
interface Packet {
  sourceChannel: string
  sourceSequence: uint64
  status : string
}
interface AsyncBatchPacketData {  
  tracking: Packet[] 
  memo: string
}
```

### Good

#### 1. Sender chain

`Batch` packet is sent. `Batch` has ordered list of all `channels` and `sequences` within batch.

`App` packets are sent with `batch` to be `sequence` of `Batch` packet.

#### 2. Receiver chain

`Batch` received and stored.

On each `App` received, it executed. Result stored in `Batch` data.

Each `App` packet ACK success.

Batch packet ACK success, configured callback functions is called with all results of all `App` success packets.

#### 3. Sender chain

`Batch` packet updated with each success ACK. 

On final `App` packet callback arrival, `Batch` executes configured callback function with results of `App` success executions.

#### Bad

In case of `App` fail or timeout, `Batch` collects results and decided how to handle based on callback logic.

`App` packet with `batch` sequence was received before `Batch`, than this packet gets `ACK` error. `Batch` will set unknown result for this packet.

`Batch` received, but one of `App` packets timeout is less than `Batch` one. All `App` packets and `Batch` timeout.

## Details
Like near receopts