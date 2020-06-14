The *channel* abstraction provides message delivery semantics to the interblockchain communication protocol in three categories: ordering, exactly-once delivery, and module permissioning. A channel serves as a conduit for packets passing between a module on one ledger and a module on another, ensuring that packets are executed only once, delivered in the order in which they were sent (if necessary), and delivered only to the corresponding module owning the other end of the channel on the destination ledger. Each channel is associated with a particular connection, and a connection may have any number of associated channels, allowing the use of common identifiers and amortising the cost of header verification across all the channels utilising a connection and light client.

Channels are payload-agnostic. The modules which send and receive IBC packets decide how to construct packet data and how to act upon the incoming packet data, and must utilise their own application logic to determine which state transactions to apply according to what data the packet contains.

\vspace{3mm}

### Motivation

\vspace{3mm}

The interblockchain communication protocol uses a cross-ledger message passing model. IBC *packets* are relayed from one ledger to the other by external relayer processes. Two ledgers, A and B, confirm new blocks independently, and packets from one ledger to the other may be delayed, censored, or re-ordered arbitrarily. Packets are visible to relayers and can be read from a ledger by any relayer process and submitted to any other ledger.

The IBC protocol must provide ordering (for ordered channels) and exactly-once delivery guarantees to allow applications to reason about the combined state of connected modules on two ledgers. For example, an application may wish to allow a single tokenised asset to be transferred between and held on multiple ledgers while preserving fungibility and conservation of supply. The application can mint asset vouchers on ledger B when a particular IBC packet is committed to ledger B, and require outgoing sends of that packet on ledger A to escrow an equal amount of the asset on ledger A until the vouchers are later redeemed back to ledger A with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both ledgers and that any vouchers minted on ledger B can later be redeemed back to ledger A. A more detailed explanation of this example is provided later on.

\vspace{3mm}

### Definitions

\vspace{3mm}

A *channel* is a pipeline for exactly-once packet delivery between specific modules on separate ledgers, which has at least one end capable of sending packets and one end capable of receiving packets.

An *ordered* channel is a channel where packets are delivered exactly in the order which they were sent.

An *unordered* channel is a channel where packets can be delivered in any order, which may differ from the order in which they were sent.

All channels provide exactly-once packet delivery, meaning that a packet sent on one end of a channel is delivered no more and no less than once, eventually, to the other end.

A *channel end* is a data structure storing metadata associated with one end of a channel on one of the participating ledgers, defined as follows:

```typescript 
interface ChannelEnd {
  state: ChannelState
  ordering: ChannelOrder
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  nextSequenceSend: uint64
  nextSequenceRecv: uint64
  nextSequenceAck: uint64
  connectionHops: [Identifier]
  version: string
}
```

- The `state` is the current state of the channel end.
- The `ordering` field indicates whether the channel is ordered or unordered. This is an enumeration instead of a boolean in order to allow additional kinds of ordering to be easily supported in the future.
- The `counterpartyPortIdentifier` identifies the port on the counterparty ledger which owns the other end of the channel.
- The `counterpartyChannelIdentifier` identifies the channel end on the counterparty ledger.
- The `nextSequenceSend`, stored separately, tracks the sequence number for the next packet to be sent.
- The `nextSequenceRecv`, stored separately, tracks the sequence number for the next packet to be received.
- The `nextSequenceAck`, stored separately, tracks the sequence number for the next packet to be acknowledged.
- The `connectionHops` stores the list of connection identifiers, in order, along which packets sent on this channel will travel. At the moment this list must be of length 1. In the future multi-hop channels may be supported.
- The `version` string stores an opaque channel version, which is agreed upon during the handshake. This can determine module-level configuration such as which packet encoding is used for the channel. This version is not used by the core IBC protocol.

Channel ends have a *state*:

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  CLOSED,
}
```

- A channel end in `INIT` state has just started the opening handshake.
- A channel end in `TRYOPEN` state has acknowledged the handshake step on the counterparty ledger.
- A channel end in `OPEN` state has completed the handshake and is ready to send and receive packets.
- A channel end in `CLOSED` state has been closed and can no longer be used to send or receive packets.

A `Packet`, encapsulating opaque data to be transferred from one module to another over a channel, is a particular interface defined as follows:

```typescript
interface Packet {
  sequence: uint64
  timeoutHeight: uint64
  timeoutTimestamp: uint64
  sourcePort: Identifier
  sourceChannel: Identifier
  destPort: Identifier
  destChannel: Identifier
  data: bytes
}
```

- The `sequence` number corresponds to the order of sends and receives, where a packet with an earlier sequence number must be sent and received before a packet with a later sequence number.
- The `timeoutHeight` indicates a consensus height on the destination ledger after which the packet will no longer be processed, and will instead count as having timed-out.
- The `timeoutTimestamp` indicates a timestamp on the destination ledger after which the packet will no longer be processed, and will instead count as having timed-out.
- The `sourcePort` identifies the port on the sending ledger.
- The `sourceChannel` identifies the channel end on the sending ledger.
- The `destPort` identifies the port on the receiving ledger.
- The `destChannel` identifies the channel end on the receiving ledger.
- The `data` is an opaque value which can be defined by the application logic of the associated modules.

Note that a `Packet` is never directly serialised. Rather it is an intermediary structure used in certain function calls that may need to be created or processed by modules calling the IBC handler.

\vspace{3mm}

### Properties

\vspace{3mm}

#### Efficiency

As channels impose no flow control of their own, the speed of packet transmission and confirmation is limited only by the speed of the underlying ledgers.

\vspace{3mm}

#### Exactly-once delivery

IBC packets sent on one end of a channel are delivered no more than exactly once to the other end.
No network synchrony assumptions are required for exactly-once safety.
If one or both of the ledgers halt, packets may be delivered no more than once, and once the ledgers resume packets will be able to flow again.

\vspace{3mm}

#### Ordering

On ordered channels, packets are be sent and received in the same order: if packet `x` is sent before packet `y` by a channel end on ledger A, packet `x` will be received before packet `y` by the corresponding channel end on ledger B.

On unordered channels, packets may be sent and received in any order. Unordered packets, like ordered packets, have individual timeouts specified in terms of the destination ledger's height or timestamp.

\vspace{3mm}

#### Permissioning

Channels are permissioned to one module on each end, determined during the handshake and immutable afterwards (higher-level logic could tokenise channel ownership by tokenising ownership of the port).
Only the module which owns the port associated with a channel end is able to send or receive on the channel.

\vspace{3mm}

### Channel lifecycle management

\vspace{3mm}

#### Opening handshake

The channel opening handshake, between two ledgers `A` and `B`, with state formatted as `(A, B)`, flows as follows:

| Datagram         | Prior state     | Posterior state  |
| ---------------- | --------------- | ---------------- |
| `ChanOpenInit`     | `(-, -)`    | `(INIT, -)`     |
| `ChanOpenTry`      | `(INIT, -)`    | `(INIT, TRYOPEN)`  |
| `ChanOpenAck`      | `(INIT, TRYOPEN)` | `(OPEN, TRYOPEN)`  |
| `ChanOpenConfirm`  | `(OPEN, TRYOPEN)` | `(OPEN, OPEN)`     |

`ChanOpenInit`, executed on ledger A, initiates a channel opening handshake from a module on ledger A to a module on ledger B,
providing the identifiers of the local channel identifier, local port, remote port, and remote channel identifier. ledger A
stores a channel end object in its state.

`ChanOpenTry`, executed on ledger B, relays notice of a channel handshake attempt to the module on ledger B, providing the
pair of channel identifiers, a pair of port identifiers, and a desired version. ledger B verifies a proof that ledger A has stored these identifiers
as claimed, looks up the module which owns the destination port, calls that module to check that the version requested is compatible,
and stores a channel end object in its state.

`ChanOpenAck`, executed on ledger A, relays acceptance of a channel handshake attempt back to the module on ledger A,
providing the identifier which can now be used to look up the channel end. ledger A verifies a proof that ledger B
has stored the channel metadata as claimed and marks its end of the channel as `OPEN`.

`ChanOpenConfirm`, executed on ledger B, confirms opening of a channel from ledger A to ledger B.
Ledger B simply checks that ledger A has executed `ChanOpenAck` and marked the channel as `OPEN`.
Ledger B subsequently marks its end of the channel as `OPEN`. After execution of `ChanOpenConfirm`,
the channel is open on both ends and can be used immediately.

When the opening handshake is complete, the module which initiates the handshake will own the end of the created channel on the host ledger, and the counterparty module which
it specifies will own the other end of the created channel on the counterparty ledger. Once a channel is created, ownership can only be changed by changing ownership of the associated ports.

\vspace{3mm}

#### Versioning

During the handshake process, two ends of a channel come to agreement on a version bytestring associated
with that channel. The contents of this version bytestring are opaque to the IBC core protocol.
Host ledgers may utilise the version data to indicate supported application-layer protocols, agree on packet
encoding formats, or negotiate other channel-related metadata related to custom logic on top of IBC.
Host ledgers may also safely ignore the version data or specify an empty string.

\vspace{3mm}

#### Closing handshake

The channel closing handshake, between two ledgers `A` and `B`, with state formatted as `(A, B)`, flows as follows:

| Datagram         | Prior state | Posterior state  |
| ---------------- | -------------- | ----------------- |
| `ChanCloseInit`    | \texttt{\small{(OPEN, OPEN)}}  | \texttt{\small{(CLOSED, OPEN)}}   |
| `ChanCloseConfirm` | \texttt{\small{(CLOSED, OPEN)}} | \texttt{\small{(CLOSED, CLOSED)}}  |

`ChanCloseInit`, executed on ledger A, closes the end of the channel on ledger A.

`ChanCloseInit`, executed on ledger B, simply verifies that the channel has been
marked as closed on ledger A and closes the end on ledger B.

Any in-flight packets can be timed-out as soon as a channel is closed.

Once closed, channels cannot be reopened and identifiers cannot be reused. Identifier reuse is prevented because
we want to prevent potential replay of previously sent packets. The replay problem is analogous to using sequence
numbers with signed messages, except where the light client algorithm "signs" the messages (IBC packets), and the replay
prevention sequence is the combination of port identifier, channel identifier, and packet sequence — hence we cannot
allow the same port identifier and channel identifier to be reused again with a sequence reset to zero, since this
might allow packets to be replayed. It would be possible to safely reuse identifiers if timeouts of a particular
maximum height/time were mandated and tracked, and future protocol versions may incorporate this feature.

\vspace{3mm}

### Sending packets

\vspace{3mm}

The `sendPacket` function is called by a module in order to send an IBC packet on a channel end owned by the calling module to the corresponding module on the counterparty ledger.

Calling modules must execute application logic atomically in conjunction with calling `sendPacket`.

The IBC handler performs the following steps in order:

- Checks that the channel and connection are open to send packets
- Checks that the calling module owns the sending port
- Checks that the packet metadata matches the channel and connection information
- Checks that the timeout height specified has not already passed on the destination ledger
- Increments the send sequence counter associated with the channel (in the case of ordered channels)
- Stores a constant-size commitment to the packet data and packet timeout

Note that the full packet is not stored in the state of the ledger — merely a short hash-commitment to the data and timeout value. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

\vspace{3mm}

### Receiving packets

\vspace{3mm}

The `recvPacket` function is called by a module in order to receive and process an IBC packet sent on the corresponding channel end on the counterparty ledger.

Calling modules must execute application logic atomically in conjunction with calling `recvPacket`, likely beforehand to calculate the acknowledgement value.

The IBC handler performs the following steps in order:

- Checks that the channel and connection are open to receive packets
- Checks that the calling module owns the receiving port
- Checks that the packet metadata matches the channel and connection information
- Checks that the packet sequence is the next sequence the channel end expects to receive (for ordered channels)
- Checks that the timeout height has not yet passed
- Checks the inclusion proof of packet data commitment in the outgoing ledger's state
- Sets the opaque acknowledgement value at a store path unique to the packet (if the acknowledgement is non-empty or the channel is unordered)
- Increments the packet receive sequence associated with the channel end (for ordered channels)

\vspace{3mm}

#### Acknowledgements

\vspace{3mm}

The `acknowledgePacket` function is called by a module to process the acknowledgement of a packet previously sent by
the calling module on a channel to a counterparty module on the counterparty ledger. `acknowledgePacket` also cleans up the packet commitment,
which is no longer necessary since the packet has been received and acted upon.

Calling modules may atomically execute appropriate application acknowledgement-handling logic in conjunction with calling `acknowledgePacket`.

The IBC handler performs the following steps in order:

- Checks that the channel and connection are open to acknowledge packets
- Checks that the calling module owns the sending port
- Checks that the packet metadata matches the channel and connection information
- Checks that the packet was actually sent on this channel
- Checks that the packet sequence is the next sequence the channel end expects to acknowledge (for ordered channels)
- Checks the inclusion proof of the packet acknowledgement data in the receiving ledger's state
- Deletes the packet commitment (cleaning up state and preventing replay)
- Increments the next acknowledgement sequence (for ordered channels)

\vspace{3mm}

### Timeouts

\vspace{3mm}

Application semantics may require some timeout: an upper limit to how long the ledger will wait for a transaction to be processed before considering it an error. Since the two ledgers have different local clocks, this is an obvious attack vector for a double spend — an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout — so applications cannot safely implement naive timeout logic themselves. In order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination ledger is running and reachable. The timeout must be proven on the recipient ledger, not simply the absence of a response on the sending ledger.

\vspace{3mm}

#### Sending end

The `timeoutPacket` function is called by a module which originally attempted to send a packet to a counterparty module,
where the timeout height or timeout timestamp has passed on the counterparty ledger without the packet being committed, to prove that the packet
can no longer be executed and to allow the calling module to safely perform appropriate state transitions.

Calling modules may atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutPacket`.

The IBC handler performs the following steps in order:

- Checks that the channel and connection are open to timeout packets
- Checks that the calling module owns the sending port
- Checks that the packet metadata matches the channel and connection information
- Checks that the packet was actually sent on this channel
- Checks a proof that the packet has not been confirmed on the destination ledger
- Checks a proof that the destination ledger has exceeded the timeout height or timestamp
- Deletes the packet commitment (cleaning up state and preventing replay)

In the case of an ordered channel, `timeoutPacket` additionally closes the channel if a packet has timed out. Unordered channels are expected to continue in the face of timed-out packets.

If relations are enforced between timeout heights of subsequent packets, safe bulk timeouts of all packets prior to a timed-out packet can be performed.

\vspace{3mm}

#### Timing-out on close

If a channel is closed, in-flight packets can never be received and thus can be safely timed-out.
The `timeoutOnClose` function is called by a module in order to prove that the channel
to which an unreceived packet was addressed has been closed, so the packet will never be received
(even if the `timeoutHeight` or `timeoutTimestamp` has not yet been reached). Appropriate
application-specific logic may then safely be executed.

\vspace{3mm}

#### Cleaning up state

If an acknowledgement is not written (as handling the acknowledgement would clean up state in that case), `cleanupPacket` may be called by a module in order to remove a received packet commitment from storage. The receiving end must have already processed the packet (whether regularly or past timeout).

In the ordered channel case, `cleanupPacket` cleans-up a packet on an ordered channel by proving that the receive sequence has passed the packet's sequence on the other end.

In the unordered channel case, `cleanupPacket` cleans-up a packet on an unordered channel by proving that the associated acknowledgement has been written.
