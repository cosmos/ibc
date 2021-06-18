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
- Relayer addresses should not be forgeable
- Permissionless relaying

## Technical Specification

### General Design

In order to avoid extra packets on the order of the number of fee packets, as well as provide an opt-in approach, we
store all fee payment info only on the source chain. The source chain is the one location where the sender can provide tokens
to incentivize the packet. The fee distribution may be implementation specific and thus does not need to be in the IBC spec
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

Ideally the fees can easily be redeemed in native tokens on both sides, but relayers may select others. In this example, the relayer collects a fair bit of IRIS, covering its costs there and more. It also collects channel-7/ATOM vouchers from many packets. After relaying a few thousand packets, the account on the Cosmos Hub is running low, so the relayer will send those channel-7/ATOM vouchers back over channel-7 to it's account on the Hub to replenish the supply there. 

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
// EscrowPacketFee is an open callback that may be called by any module/user that wishes to escrow funds in order to
// incentivize the relaying of the given packet.
// NOTE: These fees are escrowed in addition to any previously escrowed amount for the packet.
// They may set a separate receiveFee, ackFee, and timeoutFee to be paid
// for each step in the packet flow. The caller must send max(receiveFee+ackFee, timeoutFee) to the fee module to be locked
// in escrow to provide payout for any potential packet flow.
// The caller may optionally specify an array of relayer addresses. This MAY be used by the fee module to modify fee payment logic
// based on ultimate relayer address. For example, fee module may choose to only pay out relayer if the relayer address was specified in
// the `EscrowPacketFee`.
function EscrowPacketFee(packet: Packet, receiveFee: Fee, ackFee: Fee, timeoutFee: Fee, relayers: []string) {
    // escrow max(receiveFee+ackFee, timeoutFee) for this packet
    // do custom logic with provided relayer addresses if necessary
}

// PayFee is a callback implemented by fee module called by the ICS-4 AcknowledgePacket handler.
function PayFee(packet: Packet, forward_relayer: string, reverse_relayer: string) {
    // pay the forward fee to the forward relayer address
    // pay the reverse fee to the reverse relayer address
    // refund extra tokens to original fee payer(s)
}

// PayFee is a callback implemented by fee module called by the ICS-4 TimeoutPacket handler.
function PayTimeoutFee(packet: Packet, timeout_relayer: string) {
    // pay the timeout fee to the timeout relayer address
    // refund extra tokens to original fee payer(s)
}
```


The fee module should also expose the following queries so that relayers may query their expected fee:

```typescript
// Gets the fee expected for submitting ReceivePacket msg for the given packet
// Caller should provide the intended relayer address in case the fee is dependent on specific relayer(s).
function GetReceiveFee(portID, channelID, sequence, relayer) Fee

// Gets the fee expected for submitting AcknowledgePacket msg for the given packet
// Caller should provide the intended relayer address in case the fee is dependent on specific relayer(s).
function GetAckFee(portID, channelID, sequence, relayer) Fee

// Gets the fee expected for submitting TimeoutPacket msg for the given packet
// Caller should provide the intended relayer address in case the fee is dependent on specific relayer(s).
function GetTimeoutFee(portID, channelID, sequence, relayer) Fee
```

Since different chains may have different representations for fungible tokens and this information is not being sent to other chains; this ICS does not specify a particular representation for the `Fee`. Each chain may choose its own representation, it is incumbent on relayers to interpret the Fee correctly.

### IBC Module Wrapper

The fee module will implement its own ICS-26 callbacks that wrap the application-specific module callbacks. This fee module middleware will ensure that the counterparty module supports incentivization and will implement all fee-specific logic. It will then pass on the request to the embedded application module for further callback processing.

In this way, custom fee-handling logic can be hooked up to the IBC packet flow logic without placing the code in the ICS-4 handlers or the application code. This is valuable since the ICS-4 handlers should only be concerned with IBC correctness, and the application handlers should not be handling fee logic that is universal amongst all other incentivized applications. In fact, a given application module should be able to be hooked up to any fee module with no further changes to the application itself.

As mentioned above, the fee module will implement the ICS-26 callbacks, and can embed an application IBC Module. All ICS-26 callbacks in the fee module will call into the embedded application's callback. Thus, only the fee callbacks that do something additional are explicitly specified here.

#### Fee Protocol Negotiation

The fee middleware will negotiate its fee protocol version with the counterparty module by prepending its own version to the application version. 

Channel Version: `fee_v{fee_protocol_version}:{application_version}`

Ex: `fee_v1:ics20-1`

The fee middleware's handshake callbacks ensure that both modules agree on compatible fee protocol version(s), and then pass the application-specific version string to the embedded application's handshake callbacks.

### Port

The portID will similarly be nested like the channel version to allow all nested modules to negotiate their respective ports.

Applications must note however, that the portID in its entirety will be contained in the packet.

PortID: `fee:transfer`

Once the fee module has done its own logic based on the full port, it will strip the fee prefix and pass along the nested `portID` to the nested module.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
    // if the version and port is prefixed,
    // then remove the prefix and pass the app-specific version to app callback.
    // otherwise, pass version directly to app callback.
    // ensure fee ports are compatible (if they exist)
    feePort, appPort = splitFeePort(portID)
    feeVersion, appVersion = splitFeeVersion(version)
    cpFeePort, cpAppPort = splitFeePort(counterpartyPortIdentifier)
    if !isCompatible(feePort, cpFeePort) {
        return error
    }
    app.OnChanOpenInit(
        order,
        connectionHops,
        appPort,
        channelIdentifier,
        cpAppPort,
        counterpartyChannelIdentifier,
        appVersion,
    )
}

function OnChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
      cpFeeVersion, cpAppVersion = splitFeeVersion(counterpartyVersion)
      feeVersion, appVersion = splitFeeVersion(version)
      feePort, appPort = splitFeePort(portID)
      cpFeePort, cpAppPort = splitFeePort(counterpartyPortIdentifier)
      if !isCompatible(feePort, cpFeePort) {
          return error
      }
      if !isCompatible(cpFeeVersion, feeVersion) {
          return error
      }

      // call the underlying applications OnChanOpenTry callback
      app.OnChanOpenTry(
          order,
          connectionHops,
          portIdentifier,
          channelIdentifier,
          counterpartyPortIdentifier,
          counterpartyChannelIdentifier,
          cpAppVersion,
          appVersion,
      )
}

function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
      if !isCompatible(version) {
          return error
      }
}

function splitFeeVersion(version: string): []string {
    if hasPrefix(version, "fee") {
        splitVersions = split(version,  ":")
        feeVersion = version[0]
        appVersion = join(version[1:], ":")
        // if version has fee prefix
        // return first split as fee version and the rest of the string as app version
        return []string{feeVersion, appVersion}
    }
    // otherwise return an empty fee version and full version as app version
    return []string{"", version}
}

function splitFeePort(portID: string) []string {
    // identical logic to splitFeeVersion
}
```

#### Packet Callbacks

```typescript
function onRecvPacket(packet: Packet, relayer: string): bytes {
    app_acknowledgement = app.onRecvPacket(packet, relayer)

    // in case of asynchronous acknowledgement, we must store the relayer address so that we can retrieve it later to write the acknowledgement.
    if app_acknowledgement == nil {
        privateStore.set(forwardRelayerPath(packet), relayer)
    }

    // if channel is incentivized, wrap the acknowledgement with forward relayer and return marshalled bytes
    // constructIncentivizedAck takes the app-specific acknowledgement and receive-packet relayer (forward relayer)
    // and constructs the incentivized acknowledgement struct with the forward relayer and app-specific acknowledgement embedded.
    ack = constructIncentivizedAck(app_acknowledgment, relayer)
    return marshal(ack)
}

function onAcknowledgePacket(packet: Packet, acknowledgement: bytes, relayer: string) {
    // If incentivization is enabled, then the acknowledgement
    // is a marshalled struct containing the forward relayer address as  a string (called forward_relayer),
    // and the raw acknowledgement bytes returned by the counterparty application module (called app_ack).

    // get the forward relayer from the acknowledgement
    // and pay fees to forward and reverse relayers.
    // reverse_relayer is submitter of acknowledgement message
    // provided in function arguments
    // NOTE: Fee may be zero
    ack = unmarshal(acknowledgement)
    forward_relayer = getForwardRelayer(ack)
    PayFee(packet, forward_relayer, relayer)

    // unwrap the raw acknowledgement bytes sent by counterparty application and pass it to the application callback.
    app_ack = getAppAcknowledgement(acknowledgement)

    app.OnAcknowledgePacket(packet, app_ack, relayer)
}

function onTimeoutPacket(packet: Packet, relayer: string) {
    // get the timeout relayer from function arguments
    // and pay timeout fee.
    // NOTE: Fee may be zero
    PayTimeoutFee(packet, relayer)
    app.OnTimeoutPacket(packet, relayer)
}

function onTimeoutPacketClose(packet: Packet, relayer: string) {
    // get the timeout relayer from function arguments
    // and pay timeout fee.
    // NOTE: Fee may be zero
    PayTimeoutFee(packet, relayer)
    app.onTimeoutPacketClose(packet, relayer)
}

function constructIncentivizedAck(app_ack: bytes, forward_relayer: string): Acknowledgement {
    // TODO: see https://github.com/cosmos/ibc/pull/582
}

function getForwardRelayer(ack: Acknowledgement): string {
    // TODO: see https://github.com/cosmos/ibc/pull/582
}

function getAppAcknowledgement(ack: Acknowledgement): bytes {
    // TODO: see https://github.com/cosmos/ibc/pull/582
}
```

#### Embedded applications calling into ICS-4

Whenever embedded applications call into ICS-4, they must go through their parent application. For example, if ICS-20 wants to bind to the `transfer` port. Rather than calling `BindPort` directly, implementers must take care to ensure that this call passes through the top-level fee module first; which will then prepend `fee:` and cause ICS-4 to bind the top-level fee module to the `fee:transfer` port.

Note that if the embedded application uses asynchronous acks then, the `WriteAcknowledgement` call in the application must call the fee middleware's `WriteAcknowledgement` rather than calling the ICS-4 handler's `WriteAcknowledgement` function directly.

```typescript
// Fee Middleware writeAcknowledgement function
function writeAcknowledgement(
  packet: Packet,
  acknowledgement: bytes) {
    // if the channel is incentivized,
    // retrieve the forward relayer that was stored in `onRecvPacket`
    relayer = privateStore.get(forwardRelayerPath(packet))
    ack = constructIncentivizedAck(acknowledgment, relayer)
    ack_bytes marshal(ack)
    return ics4.writeAcknowledgement(packet, ack_bytes)
    // otherwise just write the acknowledgement directly
    return ics4.writeAcknowledgement(packet, ack_bytes)
}
```

#### Backwards Compatibility

Maintaining backwards compatibility with an unincentivized chain directly in the fee module, would require the top-level fee module to negotiate versions that do not contain a fee version and bind to nested ports directly without a fee port prefix. This pattern causes unnecessary complexity as the layers of nested applications increase.

Instead, the fee module will only connect to a counterparty fee module. This simplifies the fee module logic, and doesn't require it to mimic the underlying nested application(s).

In order for an incentivized chain to maintain backwards compatibility with an unincentivized chain for a given application (e.g. ICS-20), the incentivized chain should host both a top-level ICS-20 module and a top-level fee module that nests an ICS-20 application.

Thus, a relayer looking to create an incentivized channel between two incentivized chains can do so by creating channel between their `fee:transfer` modules. A relayer looking to create an unincentivized channel on a backwards-compatible incentivized chain may do so by creating the channel on the `transfer` port.

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
