---
ics: 29
title: General Fee Payment
stage: draft
category: IBC/APP
requires: 20, 25, 26
kind: instantiation
author: Ethan Frey <ethan@confio.tech>
created: 2021-06-01
modified: 2021-06-01
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for handling fee
payments on top of any ICS application protocol. It requires some standard packet changes, but can be adopted by any
application, without forcing other applications to use this implementation.

### Motivation

There has been much discussion on a general incentivization mechanism for relayers. A simple proposal was created to
[extend ICS20 to incentivize relaying](https://github.com/cosmos/ibc/pull/577) on the destination chain. However,
it was very specific to ICS20 and would not work for other protocols. This was then extended to a more
[general fee payment design](https://github.com/cosmos/ibc/issues/578) that could be adopted by any ICS application
protocol.

In general, the Interchain dream will never scale unless there is a clear way to incentivize relayers. We seek to
define a clear interface that can be easily adopted by any application, but not preclude chains that don't use tokens.

### Desired Properties

- Incentivize timely delivery of the packet (`OnReceivePacket` called)
- Incentivize relaying acks for these packets (`OnAcknowledgement` called)
- Incentivize relaying timeouts for these packets when the timeout has expired before packet is delivered (for example as receive fee was too low) (`OnTimeout` called)
- Produces no extra IBC packets
- One direction works, even when destination chain does not support concept of fungible tokens
- Opt-in for each chain implementing this. eg. ICS27 with fee support on chain A could connect to ICS27 without fee support on chain B.
- Standardized interface for each chain implementing this extension
- Support custom fee-handling logic within the same framework
- Relayer addresses should not be forgable
- Permissionless relaying

## Technical Specification

### General Design

In order to avoid extra packets on the order of the number of fee packets, as well as provide an opt-in approach, we
store all fee payment info only on the source chain. The source chain is the one location where the sender can provide tokens
to incentivize the packet. The fee distribution may be implementation specific and thus does not need to be in the ibc spec
(just high-level requirements are needed in this doc).

We require that the [relayer address is exposed to application modules](https://github.com/cosmos/ibc/pull/579) for
all packet-related messages, so the modules are able to incentivize the packet relayer. `OnAcknowledgement`, `OnTimeout`,
and `OnTimeoutClose` messages will therefore have the relayer address and be capable of sending escrowed tokens to such address.
However, we need a way to reliably get the address of the relayer that submitted `OnReceivePacket` on the destination chain to
the source chain. In fact, we need a *source address* for this relayer to pay out to, not the *destination address* that signed
the packet.

Given this, the flow would be:

1. User/module submits a send packet on the `source` chain, along with some tokens and fee information on how to distribute them. The fee tokens are all escrowed by the fee module.
2. RelayerA submits `OnReceivePacket` on the `destination` chain. Along with this message, the *forward relayer* will submit a `payToOnSource` address where payment should be sent.
3. Destination application includes this `payToOnSource` in the acknowledgement (there are multiple approaches to discuss below)
4. RelayerB submits `OnAcknowledgement` which provides the *return relayer* address on the source chain, along with the `payToOnSource` address
5. Source application can distribute the tokens escrowed in (1) to both the *forward* and the *return* relayers.

Alternate flow:

1. User/module submits a send packet on the `source` chain, along with some tokens and fee information on how to distribute them
2. Relayer submits `OnTimeout` which provides its address on the source chain
3. Source application can distribute the tokens escrowed in (1) to this relayer, and potentially return remainder tokens to the original packet sender.

### Fee details

For an example implementation in the Cosmos SDK, we consider 3 potential fee payments, which may be defined. Each one may be
paid out in a different token. Imagine a connection between IrisNet and the Cosmos Hub. They may define:

- ReceiveFee: 0.003 channel-7/ATOM vouchers (ATOMs already on IrisNet via ICS20)
- AckFee: 0.001 IRIS
- TimeoutFee: 0.002 IRIS

Ideally the fees can easily be redeemed in native tokens on both sides, but relayers may select others. In this example, the relayer collects a fair bit of IRIS, covering it's costs there and more. It also collects channel-7/ATOM vouchers from many packets. After relaying a few thousand packets, the account on the Cosmos Hub is running low, so the relayer will send those channel-7/ATOM vouchers back over channel-7 to it's account on the Hub to replenish the supply there. 

The sender chain will escrow 0.003 channel-7/ATOM and 0.002 IRIS. In the case that a forward relayer submits the `MsgRecvPacket` and a reverse relayer submits the `MsgAckPacket`, the forward relayer is reqarded 0.003 channel-7/ATOM and the reverse relayer is rewarded 0.001 IRIS. In the case where the packet times out, the timeout relayer receives 0.002 IRIS and 0.003 channel-7/ATOM is refunded to the original fee payer.

The logic involved in collecting fees from users and then paying it out to the relevant relayers is encapsulated by a separate fee module and may vary between implementations. However, all fee modules must implement a uniform interface such that the ICS-4 handlers can correctly pay out fees to the right relayers, and so that relayers themselves can easily determine the fees they can expect for relaying a packet.

### Fee Module Contract

While the details may vary between fee modules, all Fee modules **must** ensure it does the following:

- It must have in escrow the maximum fees that all outstanding packets may pay out (or it must have ability to mint required amount of tokens)
- It must pay the receive fee for a packet to the forward relayer specified in `PayFee` callback
- It must pay the ack fee for a packet to the reverse relayer specified in `PayFee` callback
- It must pay the timeout fee for a packet to the timeout relayer specified in `PayTimeoutFee` callback
- It must refund any remainder fees in escrow to the original fee payer(s) if applicable

```typescript
function PayFee(packet: Packet, forward_relayer: string, reverse_relayer: string) {
    // pay the forward fee to the forward relayer address
    // pay the reverse fee to the reverse relayer address
    // refund extra tokens to original fee payer(s)
}

function PayTimeoutFee(packet: Packet, timeout_relayer: string) {
    // pay the timeout fee to the timeout relayer address
    // refund extra tokens to original fee payer(s)
}
```


The fee module should also expose the following queries so that relayers may query their expected fee:

```typescript
// Gets the fee expected for submitting ReceivePacket msg for the given packet
function GetReceiveFee(portID, channelID, sequence) Fee

// Gets the fee expected for submitting AcknowledgePacket msg for the given packet
function GetAckFee(portID, channelID, sequence) Fee

// Gets the fee expected for submitting TimeoutPacket msg for the given packet
function GetTimeoutFee(portID, channelID, sequence) Fee
```

Since different chains may have different representations for fungible tokens and this information is not being sent to other chains; this ICS does not specify a particular representation for the `Fee`. Each chain may choose its own representation, it is incumbent on relayers to interpret the Fee correctly.

### Connection Negotiation

The chains must agree to enable the incentivization feature during the connection handshake. This can be done by bumping the connection version.

```{"2", ["ORDER_ORDERED", "ORDER_UNORDERED"]}```

Since most chains that support incentivization will wish to be compatible with chains that do not, a chain with `V2` enabled will send its possible connection versions: `{{"1", ["ORDER_ORDERED", "ORDER_UNORDERED"]}, {"2", ["ORDER_ORDERED", "ORDER_UNORDERED"]}}` in `ConnOpenInit`. The counterparty chain will select the highest version that it can support.

If the negotiated connection is `V2`, then the ICS-4 `WriteAcknowledgement` function must write the forward relayer address into a structured acknowledgement and the ICS-4 handlers for `AcknowledgePacket` and `TimeoutPacket` must pay fees through the ibc fee module callbacks. If the negotiated connection is on `V1`, then the ICS-4 handlers must not modify the acknowledgement provided by the application and should not call any fee callbacks even if relayer incentivization is enabled on the chain.

Thus, a chain can support incentivization while still maintaining connections that do not have the incentivization feature. This is crucial to enable a chain with incentivization to connect with a chain that does not have incentivization feature, as the acknowledgements need to be sent over the wire without the forward relayer.

### Channel Changes

The ibc-fee callbacks will then be utilized in the ICS-4 handlers like so:

```typescript
function acknowledgePacket(
  packet: OpaquePacket,
  acknowledgement: bytes,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {
    // get the underlying connection for this channel
    connection = getConnection()
    // only call the fee module callbacks if the connection supports incentivization.
    if IsSupportedFeature(connection, "INCENTIVE_V1") {
        // get the forward relayer from the acknowledgement
        // and pay fees to forward and reverse relayers.
        // reverse_relayer is submitter of acknowledgement message
        // provided in function arguments
        // NOTE: Fee may be zero
        forward_relayer = getForwardRelayer(acknowledgement)
        PayFee(packet, forward_relayer, relayer)
    }
}

function timeoutPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  nextSequenceRecv: Maybe<uint64>,
  relayer: string): Packet {
    // get the underlying connection for this channel
    connection = getConnection()
    // only call the fee module callbacks if the connection supports incentivization.
    if IsSupportedFeature(connection, "INCENTIVE_V1") {
        // get the timeout relayer from function arguments
        // and pay timeout fee.
        // NOTE: Fee may be zero
        PayTimeoutFee(packet, relayer)
    }
}
```

#### Reasoning

This proposal satisfies the desired properties. All parts of the packet flow (receive/acknowledge/timeout) can be properly incentivized and rewarded. The protocol does not specify the relayer beforehand, thus the incentivization is permissionless. The escrowing and distribution of funds is completely handled on source chain, thus there is no need for additional IBC packets or the use of ICS-20 in the fee protocol. The fee protocol only assumes existence of fungible tokens on the source chain. Using the connection version, the protocol enables opt-in incentivization and backwards compatibility. The fee module can implement arbitrary custom logic so long as it respects the callback interfaces that IBC expects.

##### Correctness

The fee module is responsible for correctly escrowing and distributing funds to the provided relayers. It is IBC's responsibility to provide the fee module with the correct relayers. The ack and timeout relayers are trivially retrievable since they are the senders of the acknowledgment and timeout message.

The receive relayer submits the message to the counterparty chain. Thus the counterparty chain must communicate the knowledge of who relayed the receive packet to the source chain using the acknowledgement. The address that is sent back **must** be the address of the forward relayer on the source chain.

The source chain will use a "best efforts" approach with regard to the forward relayer address. Since it is not verified directly by the counterparty and is instead just treated as a string to be passed back in the acknowledgement, the forward relayer `payOnSender` address may not be a valid source chain address. In this case, the invalid address is discarded, the receive fee is refunded, and the acknowledgement processing continues. It is incumbent on relayers to pass their `payOnSender` addresses to the counterparty chain correctly.
In the event that the counterparty chain itself incorrectly sends the forward relayer address, this will cause relayers to not collect fees on source chain for relaying packets. The incentivize-driven relayers will stop relaying for the chain until the acknowledgement logic is fixed, however the channel remains functional.

We cannot return an error on an invalid `payOnSender` address as this would permanently prevent the source chain from processing the acknowledgment of a packet that was otherwise correctly received, processed and acknowledged on the counterparty chain. The IBC protocol requires that incorrect or malicious relayers may at best affect the liveness of a user's packets. Preventing successful acknowledgement in this case would leave the packet flow at a permanently incomplete state, which may be very consequential for certain IBC applications like ICS-20.

Thus, the forward relayer reward is contingent on it providing the correct `payOnSender` address when it sends the `receive_packet` message. The packet flow will continue processing successfully even if the fee payment is unsuccessful.

With the forward relayer correctly embedded in the acknowledgement, and the reverse and timeout relayers available directly in the message; IBC is able to provide the correct relayer addresses to the fee module for each step of the packet flow.

#### Optional addenda

## Backwards Compatibility

This can be added to any existing protocol without breaking it on the other side.

## Forwards Compatibility

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

## History

June 1 2020 - Draft written

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
