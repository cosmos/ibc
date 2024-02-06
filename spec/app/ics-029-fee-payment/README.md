---
ics: 29
title: General Fee Payment
stage: draft
category: IBC/APP
requires: 4, 25, 26, 30
kind: instantiation
version compatibility: ibc-go v7.0.0
author: Aditya Sripal <aditya@interchain.berlin>, Ethan Frey <ethan@confio.tech>
created: 2021-06-01
modified: 2022-07-06
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for handling fee
payments on top of any ICS application protocol. It requires some changes to the acknowledgement, but can be adopted by any
application, without forcing other applications to use this implementation.

### Motivation

There has been much discussion on a general incentivization mechanism for relayers. A simple proposal was created to
[extend ICS-20 to incentivize relaying](https://github.com/cosmos/ibc/pull/577) on the destination chain. However,
it was very specific to ICS-20 and would not work for other protocols. This was then extended to a more
[general fee payment design](https://github.com/cosmos/ibc/issues/578) that could be adopted by any ICS application
protocol.

In general, the Interchain dream will never scale unless there is a clear way to incentivize relayers. We seek to
define a clear interface that can be easily adopted by any application, but not preclude chains that don't use tokens.

### Desired Properties

- Incentivize timely delivery of the packet (`recvPacket` called)
- Incentivize relaying acks for these packets (`acknowledgePacket` called)
- Incentivize relaying timeouts for these packets when the timeout has expired before packet is delivered (for example as receive fee was too low) (`timeoutPacket` called)
- Produces no extra IBC packets
- One direction works, even when destination chain does not support concept of fungible tokens
- Opt-in for each chain implementing this. e.g. ICS27 with fee support on chain A could connect to ICS27 without fee support on chain B.
- Standardized interface for each chain implementing this extension
- Support custom fee-handling logic within the same framework
- Relayer addresses should not be forgeable
- Enable permissionless or permissioned relaying

### Definitions

`forward relayer`: The relayer that submits the `recvPacket` message for a given packet

`reverse relayer`: The relayer that submits the `acknowledgePacket` message for a given packet

`timeout relayer`: The relayer that submits the `timeoutPacket` or `timeoutOnClose` message for a given packet

`receive fee`: The fee paid for submitting the `recvPacket` message for a given packet

`ack fee`: The fee paid for submitting the `acknowledgePacket` message for a given packet

`timeout fee`: The fee paid for submitting the `timeoutPacket` or `timeoutOnClose` message for a given packet

`source address`: The payee address selected by a relayer on the chain that sent the packet

`destination address`: The address of a relayer on the chain that receives the packet

## Technical Specification

### General Design

In order to avoid extra fee packets on the order of the number of application packets, as well as provide an opt-in approach, we
store all fee payment info only on the source chain. The source chain is the one location where the sender can provide tokens
to incentivize the packet. The fee distribution may be implementation specific and thus does not need to be in the IBC spec
(just high-level requirements are needed in this doc).

We require that the [relayer address is exposed to application modules](https://github.com/cosmos/ibc/pull/579) for
all packet-related messages, so the modules are able to incentivize the packet relayer. `acknowledgePacket`, `timeoutPacket`,
and `timeoutOnClose` messages will therefore have the relayer address and be capable of sending escrowed tokens to such address.
However, we need a way to reliably get the address of the relayer that submitted `recvPacket` on the destination chain to
the source chain. In fact, we need a *source address* for this relayer to pay out to, not the *destination address* that signed
the packet.

The fee payment mechanism will be implemented as IBC Middleware (see ICS-30) in order to provide maximum flexibility for application developers and blockchains.

Given this, the flow would be:

1. Relayer registers their destination address to source address mapping on the destination chain's fee middleware.
1. User/module submits a send packet on the `source` chain, along with a message to the fee middleware module with some tokens and fee information on how to distribute them. The fee tokens are all escrowed by the fee module.
1. RelayerA submits `RecvPacket` on the `destination` chain.
1. Destination fee middleware will retrieve the source address for the given relayer's destination address (this mapping is already registered) and include it in the acknowledgement.
1. RelayerB submits `AcknowledgePacket` which provides the *reverse relayer* address on the source chain in the message sender, along with the source address of the *forward relayer* embedded in the acknowledgement.
1. Source fee middleware can distribute the tokens escrowed in (1) to both the *forward* and the *reverse* relayers and refund remainder tokens to original fee payer(s).

Alternate flow:

1. User/module submits a send packet on the `source` chain, along with some tokens and fee information on how to distribute them
1. Relayer submits `OnTimeout` which provides its address on the source chain
1. Source application can distribute the tokens escrowed in (1) to this relayer, and potentially return remainder tokens to the original fee payer(s).

### Fee details

For an example implementation in the Cosmos SDK, we consider 3 potential fee payments, which may be defined. Each one may be
paid out in a different token. Imagine a connection between IrisNet and the Cosmos Hub. To incentivize a packet from IrisNet to the Cosmos Hub, they may define:

- ReceiveFee: 0.003 channel-7/ATOM vouchers (ATOMs already on IrisNet via ICS20)
- AckFee: 0.001 IRIS
- TimeoutFee: 0.002 IRIS

Ideally the fees can easily be redeemed in native tokens on both sides, but relayers may select others. In this example, the relayer collects a fair bit of IRIS, covering its costs there and more. It also collects channel-7/ATOM vouchers from many packets. After relaying a few thousand packets, the account on the Cosmos Hub is running low, so the relayer will send those channel-7/ATOM vouchers back over channel-7 to it's account on the Hub to replenish the supply there. 

The sender chain will escrow 0.003 channel-7/ATOM and 0.002 IRIS from the fee payers' account. In the case that a forward relayer submits the `recvPacket` and a reverse relayer submits the `ackPacket`, the forward relayer is rewarded 0.003 channel-7/ATOM and the reverse relayer is rewarded 0.001 IRIS while 0.002 IRIS is refunded to the original fee payer. In the case where the packet times out, the timeout relayer receives 0.002 IRIS and 0.003 channel-7/ATOM is refunded to the original fee payer.

The logic involved in collecting fees from users and then paying it out to the relevant relayers is encapsulated by a separate fee module and may vary between implementations. However, all fee modules must implement a uniform interface such that the ICS-4 handlers can correctly pay out fees to the right relayers, and so that relayers themselves can easily determine the fees they can expect for relaying a packet.

### Data Structures

The incentivized acknowledgment written on the destination chain includes:

- raw bytes of the acknowledgement from the underlying application,
- the source address of the forward relayer,
- and a boolean indicative of receive operation success on the underlying application.

```typescript
interface Acknowledgement {
    appAcknowledgement: []byte
    forwardRelayerAddress: string
    underlyingAppSuccess: boolean
}
```

### Store Paths

#### Relayer Address for Async Ack Path

The forward relayer addresses are stored under a store path prefix unique to a combination of port identifier, channel identifier and sequence. This may be stored in the private store.

```typescript
function relayerAddressForAsyncAckPath(packet: Packet): Path {
    return "forwardRelayer/{packet.destinationPort}/{packet.destinationChannel}/{packet.sequence}"
}
```

### Fee Middleware Contract

While the details may vary between fee modules, all fee modules **must** ensure they does the following:

- It must allow relayers to register their counterparty payee address (i.e. source address).
- It must have in escrow the maximum fees that all outstanding packets may pay out (or it must have ability to mint required amount of tokens)
- It must pay the receive fee for a packet to the forward relayer specified in `PayFee` callback (if unspecified, it must refund forward fee to original fee payer(s))
- It must pay the ack fee for a packet to the reverse relayer specified in `PayFee` callback
- It must pay the timeout fee for a packet to the timeout relayer specified in `PayTimeoutFee` callback
- It must refund any remainder fees in escrow to the original fee payer(s) if applicable

```typescript
// RegisterCounterpartyPayee is called by the relayer on each channelEnd and 
// allows them to specify their counterparty payee address before relaying.
// This ensures they will be properly compensated for forward relaying since 
// destination chain must send back relayer's source address (counterparty 
// payee address) in acknowledgement.
// This function may be called more than once by relayer, in which case, latest 
// counterparty payee address is always used.
function RegisterCounterpartyPayee(relayer: string, counterPartyAddress: string) {
    // set mapping between relayer address and counterparty payee address
}

// EscrowPacketFee is an open callback that may be called by any module/user 
// that wishes to escrow funds in order to incentivize the relaying of the 
// given packet.
// NOTE: These fees are escrowed in addition to any previously escrowed amount 
// for the packet. In the case where the previous amount is zero, the provided 
// fees are the initial escrow amount.
// They may set a separate receiveFee, ackFee, and timeoutFee to be paid
// for each step in the packet flow. The caller must send max(receiveFee+ackFee, timeoutFee)
// to the fee module to be locked in escrow to provide payout for any potential 
// packet flow.
// The caller may optionally specify an array of relayer addresses. This MAY be
// used by the fee module to modify fee payment logic based on ultimate relayer
// address. For example, fee module may choose to only pay out relayer if the 
// relayer address was specified in the `EscrowPacketFee`.
function EscrowPacketFee(packet: Packet, receiveFee: Fee, ackFee: Fee, timeoutFee: Fee, relayers: []string) {
    // escrow max(receiveFee+ackFee, timeoutFee) for this packet
    // do custom logic with provided relayer addresses if necessary
}

// PayFee is a callback implemented by fee module called by the ICS-4 AcknowledgePacket handler.
function PayFee(packet: Packet, forward_relayer: string, reverse_relayer: string) {
    // pay the forward fee to the forward relayer address
    // pay the reverse fee to the reverse relayer address
    // refund extra tokens to original fee payer(s)
    // NOTE: if forward relayer address is empty, then refund the forward fee to original fee payer(s).
}

// PayTimeoutFee is a callback implemented by fee module called by the ICS-4 TimeoutPacket handler.
function PayTimeoutFee(packet: Packet, timeout_relayer: string) {
    // pay the timeout fee to the timeout relayer address
    // refund extra tokens to original fee payer(s)
}
```

The fee module should also expose the following queries so that relayers may query their expected fee:

```typescript
// Gets the fee expected for submitting RecvPacket msg for the given packet
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

A default representation will have the following structure:

```typescript
interface Fee {
  denom: string,
  amount: uint256,
}
```

### IBC Module Wrapper

The fee middleware will implement its own ICS-26 callbacks that wrap the application-specific module callbacks as well as the ICS-4 handler functions called by the underlying application. This fee middleware will ensure that the counterparty module supports incentivization and will implement all fee-specific logic. It will then pass on the request to the embedded application module for further callback processing.

In this way, custom fee-handling logic can be hooked up to the IBC packet flow logic without placing the code in the ICS-4 handlers or the application code. This is valuable since the ICS-4 handlers should only be concerned with correctness of core IBC (transport, authentication, and ordering), and the application handlers should not be handling fee logic that is universal amongst all other incentivized applications. In fact, a given application module should be able to be hooked up to any fee module with no further changes to the application itself.

#### Fee Protocol Negotiation

The fee middleware will negotiate its fee protocol version with the counterparty module by including its own version next to the application version. The channel version will be a string of a JSON struct containing the fee middleware version and the application version. The application version may as well be a JSON-encoded string, possibly including further middleware and app versions, if the application stack consists of multiple milddlewares wrapping a base application.

Channel Version: 

```json
{"fee_version":"<fee_protocol_version>","app_version":"<application_version>"}
```

Ex: 

```json
{"fee_version":"ics29-1","app_version":"ics20-1"}
```

The fee middleware's handshake callbacks ensure that both modules agree on compatible fee protocol version(s), and then pass the application-specific version string to the embedded application's handshake callbacks.

#### Handshake Callbacks

```typescript
function onChanOpenInit(
  capability: CapabilityKey,
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string): (version: string, err: Error) {
    if version != "" {
        // try to unmarshal JSON-encoded version string and pass 
        // the app-specific version to app callback.
        // otherwise, pass version directly to app callback.
        metadata, err = UnmarshalJSON(version)
        if err != nil {
            // call the underlying applications OnChanOpenInit callback
            return app.onChanOpenInit(
                capability,
                order,
                connectionHops,
                portIdentifier,
                channelIdentifier,
                counterpartyPortIdentifier,
                counterpartyChannelIdentifier,
                version,
            )
        }

        // check that feeVersion is supported
        if !isSupported(metadata.feeVersion) {
            return "", error
        }
    } else {
        // enable fees by default if relayer does not specify otherwise
        metadata = {
            feeVersion: "ics29-1",
            appVersion: "",
        }
    }

    // call the underlying application's OnChanOpenInit callback.
    // if the version string is empty, OnChanOpenInit is expected to return
    // a default version string representing the version(s) it supports
    appVersion, err = app.onChanOpenInit(
        capability,
        order,
        connectionHops,
        portIdentifier,
        channelIdentifier,
        counterpartyPortIdentifier,
        counterpartyChannelIdentifier,
        metadata.appVersion,
    )
    if err != nil {
        return "", err
    }

    // a new version string is constructed with the app version returned 
    // by the underlying application, in case it is different than the 
    // one passed by the caller
    version = constructVersion(metadata.feeVersion, appVersion)

    return version, nil
}

function onChanOpenTry(
  capability: CapabilityKey,
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string): (version: string, err: Error) {
    // try to unmarshal JSON-encoded version string and pass 
    // the app-specific version to app callback.
    // otherwise, pass version directly to app callback.
    cpMetadata, err = UnmarshalJSON(counterpartyVersion)
    if err != nil {
        // call the underlying application's OnChanOpenTry callback
        return app.onChanOpenTry(
            capability,
            order,
            connectionHops,
            portIdentifier,
            channelIdentifier,
            counterpartyPortIdentifier,
            counterpartyChannelIdentifier,
            counterpartyVersion,
        )
    }

    // select mutually compatible fee version
    if !isCompatible(cpMetadata.feeVersion) {
        return "", error
    }
    feeVersion = selectFeeVersion(cpMetadata.feeVersion)

    // call the underlying application's OnChanOpenTry callback
    appVersion, err = app.onChanOpenTry(
        capability,
        order,
        connectionHops,
        portIdentifier,
        channelIdentifier,
        counterpartyPortIdentifier,
        counterpartyChannelIdentifier,
        cpMetadata.appVersion,
    )
    if err != nil {
        return "", err
    }
    
    // a new version string is constructed with the final fee version
    // that is selected and the app version returned by the underlying
    // application (which may be different than the one passed by the caller)
    version = constructVersion(feeVersion, appVersion)

    return version, nil
}

function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
    cpMetadata, err = UnmarshalJSON(counterpartyVersion)
    if err != nil {
        // call the underlying application's OnChanOpenAck callback
        return app.onChanOpenAck(
            portIdentifier, 
            channelIdentifier, 
            counterpartyChannelIdentifier,
            counterpartyVersion,
        )
    }

    if !isSupported(cpMetadata.feeVersion) {
        return error
    }  
    // call the underlying application's OnChanOpenAck callback
    return app.onChanOpenAck(
        portIdentifier, 
        channelIdentifier, 
        counterpartyChannelIdentifier,
        cpMetadata.appVersion,
    )
}

function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // fee middleware performs no-op on ChanOpenConfirm,
    // just call underlying callback
    return app.onChanOpenConfirm(portIdentifier, channelIdentifier)
}
```

#### Packet Callbacks

```typescript
function onRecvPacket(packet: Packet, relayer: string): bytes {
    app_acknowledgement = app.onRecvPacket(packet, relayer)

    // in case of asynchronous acknowledgement, we must store the relayer
    // address. It will be retrieved later and used to get the source 
    // address that will be written in the acknowledgement.
    if app_acknowledgement == nil {
        privateStore.set(relayerAddressForAsyncAckPath(packet), relayer)
    }

    // get source address by retrieving counterparty payee address of 
    // this relayer stored in fee middleware.
    // NOTE: source address may be empty or invalid, counterparty
    // must refund fee in these cases
    sourceAddress = getCounterpartyPayeeAddress(relayer)

    // wrap the acknowledgement with forward relayer and return marshalled bytes
    // constructIncentivizedAck takes:
    // - the app-specific acknowledgement,
    // - the receive-packet relayer (forward relayer)
    // - and a boolean indicative of receive operation success,
    // and constructs the incentivized acknowledgement struct with 
    // the forward relayer and app-specific acknowledgement embedded.
    ack = constructIncentivizedAck(app_acknowledgment, sourceAddress, app_acknowledgment.success)
    return marshal(ack)
}

function onAcknowledgePacket(packet: Packet, acknowledgement: bytes, relayer: string) {
    // the acknowledgement is a marshalled struct containing:
    // - the forward relayer address as a string (called forward_relayer)
    // - and the raw acknowledgement bytes returned by the counterparty application module (called app_ack).

    // get the forward relayer from the (incentivized) acknowledgement
    // and pay fees to forward and reverse relayers.
    // reverse_relayer is submitter of acknowledgement message
    // provided in function arguments
    // NOTE: Fee may be zero
    ack = unmarshal(acknowledgement)
    forward_relayer = getForwardRelayer(ack)
    PayFee(packet, forward_relayer, relayer)

    // unwrap the raw acknowledgement bytes sent by counterparty application
    // and pass it to the application callback.
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

function constructIncentivizedAck(
  app_ack: bytes, 
  forward_relayer: string, 
  success: boolean): Acknowledgement {
    return Acknowledgement{
	appAcknowledgement:    app_ack,
	forwardRelayerAddress: relayer,
        underlyingAppSuccess:  success,
    }
}

function getForwardRelayer(ack: Acknowledgement): string {
    ack.forwardRelayerAddress
}

function getAppAcknowledgement(ack: Acknowledgement): bytes {
    ack.appAcknowledgement
}
```

#### Embedded applications calling into ICS-4

Note that if the embedded application uses asynchronous acks then, the `WriteAcknowledgement` call in the application must call the fee middleware's `WriteAcknowledgement` rather than calling the ICS-4 handler's `WriteAcknowledgement` function directly.

```typescript
// Fee Middleware writeAcknowledgement function
function writeAcknowledgement(
  packet: Packet,
  acknowledgement: bytes) {
    // retrieve the relayer that was stored in `onRecvPacket`
    relayer = privateStore.get(relayerAddressForAsyncAckPath(packet))
    // get source address by retrieving counterparty payee address 
    // of this relayer stored in fee middleware.
    sourceAddress = getCounterpartyPayeeAddress(relayer)
    ack = constructIncentivizedAck(acknowledgment, sourceAddress, acknowledgment.success)
    ack_bytes = marshal(ack)
    // ics4Wrapper may be core IBC or higher-level middleware
    return ics4Wrapper.writeAcknowledgement(packet, ack_bytes)
}

// Fee Middleware sendPacket function just forwards data to ics-4 handler
function sendPacket(
  capability: CapabilityKey,
  sourcePort: Identifier,
  sourceChannel: Identifier,
  timeoutHeight: Height,
  timeoutTimestamp: uint64,
  data: bytes): uint64 {
    // ics4Wrapper may be core IBC or higher-level middleware
    return ics4Wrapper.sendPacket(
      capability,
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      data)
}
```

### User Interaction with Fee Middleware

**User sending Packets**

A user may specify a fee to incentivize the relaying during packet submission, by submitting a fee payment message atomically with the application-specific "send packet" message (e.g. ICS-20 `MsgTransfer`). The fee middleware will escrow the fee for the packet that is created atomically with the escrow. The fee payment message itself is not specified in this document as it may vary greatly across implementations. In some middleware, there may be no fee payment message at all if the fees are being paid out from an altruistic pool.

Since the fee middleware does not need to modify the outgoing packet, the fee payment message may be placed before or after the send packet message. However in order to maintain consistency with other middleware messages, it is recommended that fee middleware require their messages to be placed before the send packet message and escrow fees for the **next sequence** on the given channel. This way when the messages are atomically committed, the next sequence on the channel is the send packet message sent by the user, and the user escrows their fee for the created packet.

In case a user wants to pay fees on a packet after it has already been created, the fee middleware SHOULD provide a message that allows users to pay fees on a packet with the specified sequence, channel and port identifiers. This allows the user to uniquely identify a packet that has already been created, so that the fee middleware can escrow fees for that packet after the fact.

**Relayers sending RecvPacket**

Before a relayer starts relaying on a channel, they should register their counterparty message using the standardized message:

```typescript
interface RegisterCounterpartyPayeeMsg {
    portID: string
    channelID: string
    relayer: string           // destination address of the forward relayer
    counterpartyPayee: string // source address of the forward relayer
}
```

It is the responsibility of the receiving chain to authenticate that the message was received from owner of `relayer`. The receiving chain must store the mapping from: `relayer -> counterpartyPayee` for the given channel. Then, `onRecvPacket` of the destination fee middleware can query for the counterparty payee address of the `recvPacket` message sender in order to get the source address of the forward relayer. This source address is what will get embedded in the acknowledgement.

If the relayer does not register their counterparty payee address (or registers an invalid address), then the acknowledgment will still be received and processed but the forward fee will be refunded to the original fee payer(s).

#### Backwards Compatibility

Maintaining backwards compatibility with an unincentivized chain directly in the fee module, would require the top-level fee module to negotiate versions that do not contain a fee version and communicate with both incentivized and unincentivized modules. This pattern causes unnecessary complexity as the layers of nested applications increase.

Instead, the fee module will only connect to a counterparty fee module. This simplifies the fee module logic, and doesn't require it to mimic the underlying nested application(s).

In order for an incentivized chain to maintain backwards compatibility with an unincentivized chain for a given application (e.g. ICS-20), the incentivized chain should host both a top-level ICS-20 module and a top-level fee module that nests an ICS-20 application each of which should bind to unique ports.

#### Reasoning

This proposal satisfies the desired properties. All parts of the packet flow (receive/acknowledge/timeout) can be properly incentivized and rewarded. The protocol does not specify the relayer beforehand, thus the incentivization can be permissionless or permissioned. The escrowing and distribution of funds is completely handled on source chain, thus there is no need for additional IBC packets or the use of ICS-20 in the fee protocol. The fee protocol only assumes existence of fungible tokens on the source chain. By creating application stacks for the same base application (one with fee middleware, one without), we can get backwards compatibility.

##### Correctness

The fee module is responsible for correctly escrowing and distributing funds to the provided relayers. The ack and timeout relayers are trivially retrievable since they are the senders of the acknowledgment and timeout message. The forward relayer is responsible for registering their source address before sending `recvPacket` messages, so that the destination fee middleware can embed this address in the acknowledgement. The fee middleware on source will then use the address in acknowledgement to pay the forward relayer on the source chain.

The source chain will use a "best efforts" approach with regard to the forward relayer address. Since it is not verified directly by the counterparty and is instead just treated as a string to be passed back in the acknowledgement, the registered forward relayer source address may not be a valid source chain address. In this case, the invalid address is discarded, the receive fee is refunded, and the acknowledgement processing continues. It is incumbent on relayers to register their source addresses to the counterparty chain correctly.
In the event that the counterparty chain itself incorrectly sends the forward relayer address, this will cause relayers to not collect fees on source chain for relaying packets. The incentivize-driven relayers will stop relaying for the chain until the acknowledgement logic is fixed, however the channel remains functional.

We cannot return an error on an invalid source address as this would permanently prevent the source chain from processing the acknowledgment of a packet that was otherwise correctly received, processed and acknowledged on the counterparty chain. The IBC protocol requires that incorrect or malicious relayers may at best affect the liveness of a user's packets. Preventing successful acknowledgement in this case would leave the packet flow at a permanently incomplete state, which may be very consequential for certain IBC applications like ICS-20.

Thus, the forward relayer reward is contingent on it providing the correct `payOnSender` address when it sends the `receive_packet` message. The packet flow will continue processing successfully even if the fee payment is unsuccessful.

With the forward relayer correctly embedded in the acknowledgement, and the reverse and timeout relayers available directly in the message; the fee middleware will accurately escrow and distribute fee payments to the relevant relayers.

#### Optional addenda

## Forwards Compatibility

Not applicable.

## Example Implementations

- Implementation of ICS 29 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

## History

June 8 2021 - Switched to middleware solution from implementing callbacks in ICS-4 directly.

June 1 2021 - Draft written

July 6, 2022 - Update with latest changes from implementation

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
