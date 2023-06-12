---
ics: 30
title: IBC Middleware
stage: draft
category: IBC/APP
requires: 4, 25, 26
kind: instantiation
version compatibility: ibc-go v7.0.0
author: Aditya Sripal <aditya@interchain.berlin>, Ethan Frey <ethan@confio.tech>
created: 2021-06-01
modified: 2022-07-06
---

## Synopsis

This standard documents specifies the interfaces and state machine logic that a module must implement in order to act as middleware between core IBC and an underlying application(s). IBC Middleware will enable arbitrary extensions to an application's functionality without requiring changes to the application or core IBC.

### Motivation

IBC applications are designed to be self-contained modules that implement their own application-specific logic through a set of interfaces with the core IBC handlers. These core IBC handlers, in turn, are designed to enforce the correctness properties of IBC (transport, authentication, ordering) while delegating all application-specific handling to the IBC application modules. However, there are cases where some functionality may be desired by many applications, yet not appropriate to place in core IBC. The most prescient example of this, is the generalized fee payment protocol. Most applications will want to opt in to a protocol that incentivizes relayers to relay packets on their channel. However, some may not wish to enable this feature and yet others will want to implement their own custom fee handler.

Without a middleware approach, developers must choose whether to place this extension in application logic inside each relevant application; or place the logic in core IBC. Placing it in each application is redundant and prone to error. Placing the logic in core IBC requires an opt-in from all applications and violates the abstraction barrier between core IBC (TAO) and the application. Either case is not scalable as the number of extensions increase, since this must either increase code bloat in applications or core IBC handlers.

Middleware allows developers to define the extensions as separate modules that can wrap over the base application. This middleware can thus perform its own custom logic, and pass data into the application so that it may run its logic without being aware of the middleware's existence. This allows both the application and the middleware to implement its own isolated logic while still being able to run as part of a single packet flow.

### Definitions

`Middleware`: A self-contained module that sits between core IBC and an underlying IBC application during packet execution. All messages between core IBC and underlying application must flow through middleware, which may perform its own custom logic.

`Underlying Application`: An underlying application is the application that is directly connected to the middleware in question. This underlying application may itself be middleware that is chained to a base application.

`Base Application`: A base application is an IBC application that does not contain any middleware. It may be nested by 0 or multiple middleware to form an application stack.

`Application Stack (or stack)`: A stack is the complete set of application logic (middleware(s) +  base application) that gets connected to core IBC. A stack may be just a base application, or it may be a series of middlewares that nest a base application.

### Desired Properties

- Middleware enables arbitrary extensions of application logic
- Middleware can be arbitrarily nested to create a chain of app extensions
- Core IBC does not need to change
- Base Application logic does not need to change

## Technical Specification

### General Design

In order to function as IBC Middleware, a module must implement the IBC application callbacks and pass along the pre-processed data to the nested application. It must also implement `WriteAcknowledgement` and `SendPacket`, which will be called by the end application, so that it may post-process the information before passing data along to core ibc.

When nesting an application, the module must make sure that it is in the middle of communication between core IBC and the application in both directions. Developers should do this by registering the top-level module directly with the IBC router (not any nested applications). The nested applications in turn, must be given access only to the middleware's `WriteAcknowledgement` and `SendPacket` rather than to the core IBC handlers directly.

Additionally, the middleware must take care to ensure that the application logic can execute its own version negotiation without interference from the nesting middleware. In order to do this, the middleware will format the version in a JSON-encoded string containing the middleware version and the application version (and potentially also other custom parameter fields). The application version may as well be a JSON-encoded string, possibly including further middleware and app versions, if the application stack consists of multiple milddlewares wrapping a base application.  The format of the version string is as follows:

```json
{
    "<middleware_version_key>": "<middleware_version_value>",
    "app_version": "<application_version_value>",
    // ... other custom parameter fields
}
```

The `<middleware_version_key>` key in the JSON struct should be replaced by the actual name of the key for the corresponding middleware (e.g. `fee_version` for ICS-29 fee middleware).

In the application callbacks, the middleware can unmarshal the version string and retrieve the middleware and application versions. It must do its own version negotiation on `<middleware_version_value>` and then hand over `<application_version_value>` to the nested application's callback. This is only relevant if the middleware expects a compatible counterparty middleware at the same level on the counterparty stack. Middleware that only executes on a single side of the channel MUST NOT modify the channel version.

Each application stack must reserve its own unique port with core IBC. Thus two stacks with the same base application must bind to separate ports.

#### Interfaces

```typescript
// Middleware implements the ICS26 Module interface
interface Middleware extends ICS26Module {
    // middleware has acccess to an underlying application which may be wrapped 
    // by more middleware.
    app: ICS26Module
    // middleware has access to ICS4Wrapper which may be core IBC Channel Handler 
    // or a higher-level middleware that wraps this middleware.
    ics4Wrapper: ICS4Wrapper 
}
```

```typescript
// This is implemented by ICS4 and all middleware that are wrapping base application.
// The base application will call `sendPacket` or `writeAcknowledgement` of the
// middleware directly above them which will call the next middleware until it reaches
// the core IBC handler.
interface ICS4Wrapper {
    sendPacket(
      capability: CapabilityKey,
      sourcePort: Identifier,
      sourceChannel: Identifier,
      timeoutHeight: Height,
      timeoutTimestamp: uint64,
      data: bytes): uint64
    writeAcknowledgement(packet: Packet, ack: Acknowledgement)
}
```

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
            // call the underlying application's onChanOpenInit callback
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
    } else {
        metadata = {
            // set middleware version to default value
            middlewareVersion: defaultMiddlewareVersion,
            // allow application to return its default version
            appVersion: "",
        }
    }

    doCustomLogic()
    
    // call the underlying application's OnChanOpenInit callback.
    // if the version string is empty, OnChanOpenInit is expected to return
    // a default version string representing the version(s) it supports
    appVersion, err = app.OnChanOpenInit(
        capability,
        order,
        connectionHops,
        portIdentifier,
        channelIdentifier,
        counterpartyPortIdentifier,
        counterpartyChannelIdentifier,
        metadata.appVersion, // note we only pass app version here
    )
    abortTransactionUnless(err != nil)

    // a new version string is constructed with the app version returned 
    // by the underlying application, in case it is different than the 
    // one passed by the caller
    metadata = {
        // note this should have a different field name specific to middleware
        middlewareVersion: metadata.middlewareVersion,
        appVersion: appVersion,
    }

    return MarshallJSON(metadata), nil
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

    // select mutually compatible middleware version
    if !isCompatible(cpMetadata.middlewareVersion) {
        return "", error
    }
    middlewareVersion = selectMiddlewareVersion(cpMetadata.middlewareVersion)

    doCustomLogic()

    // call the underlying application's OnChanOpenTry callback
    appVersion, err = app.OnChanOpenTry(
        capability,
        order,
        connectionHops,
        portIdentifier,
        channelIdentifier,
        counterpartyPortIdentifier,
        counterpartyChannelIdentifier,
        cpMetadata.appVersion, // note we only pass counterparty app version here
    )
    abortTransactionUnless(err != nil)

    // a new version string is constructed with the final middleware version
    // that is selected and the app version returned by the underlying
    // application (which may be different than the one passed by the caller)
    metadata = {
        // note this should have a different field name specific to middleware
        middlewareVersion: middlewareVersion,
        appVersion: appVersion,
    }

    return MarshalJSON(metadata), nil
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

    if !isSupported(cpMetadata.middlewareVersion) {
        return error
    } 
    doCustomLogic()
    
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
    doCustomLogic()
    app.OnChanOpenConfirm(portIdentifier, channelIdentifier)
}
```

NOTE: Middleware that does not need to negotiate with a counterparty middleware on the remote stack will not implement the version unmarshaling and negotiation, and will simply perform its own custom logic on the callbacks without relying on the counterparty behaving similarly.

#### Packet Callbacks

```typescript
function onRecvPacket(packet: Packet, relayer: string): bytes {
    doCustomLogic()

    app_acknowledgement = app.onRecvPacket(packet, relayer)

    // middleware may modify ack
    ack = doCustomLogic(app_acknowledgement)
   
    return marshal(ack)
}

function onAcknowledgePacket(packet: Packet, acknowledgement: bytes, relayer: string) {
    doCustomLogic()

    // middleware may modify ack
    app_ack = getAppAcknowledgement(acknowledgement)

    app.onAcknowledgePacket(packet, app_ack, relayer)

    doCustomLogic()
}

function onTimeoutPacket(packet: Packet, relayer: string) {
    doCustomLogic()

    app.onTimeoutPacket(packet, relayer)

    doCustomLogic()
}

function onTimeoutPacketClose(packet: Packet, relayer: string) {
    doCustomLogic()

    app.onTimeoutPacketClose(packet, relayer)

    doCustomLogic()
}
```

NOTE: Middleware may do pre- and post-processing on underlying application data for all IBC Module callbacks defined in ICS-26.

#### ICS-4 Wrappers

```typescript
function writeAcknowledgement(
  packet: Packet,
  acknowledgement: bytes) {
    // middleware may modify acknowledgement
    ack_bytes = doCustomLogic(acknowledgement)

    return ics4Wrapper.writeAcknowledgement(packet, ack_bytes)
}
```

```typescript
function sendPacket(
  capability: CapabilityKey,
  sourcePort: Identifier,
  sourceChannel: Identifier,
  timeoutHeight: Height,
  timeoutTimestamp: uint64,
  app_data: bytes): uint64 {
    // middleware may modify packet
    data = doCustomLogic(app_data)

    return ics4Wrapper.sendPacket(
      capability,
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      data)
}
```

### User Interaction

In the case where the middleware requires some user input in order to modify the outgoing packet messages from the underlying application, the middleware MUST get this information from the user before it receives the packet message from the underlying application. It must then do its own authentication of the user input, and ensure that the user input provided to the middleware is matched to the correct outgoing packet message. The middleware MAY accomplish this by requiring that the user input to middleware, and packet message to underlying application are sent atomically and ordered from outermost middleware to base application.

### Security Model

As seen above, IBC middleware may arbitrarily modify any incoming or outgoing data from an underlying application. Thus, developers should not use any untrusted middleware in their application stacks.

## Backwards Compatibility

The Middleware approach is a design pattern already enabled by current IBC. This ICS seeks to standardize a particular design pattern for IBC middleware. There are no changes required to core IBC or any existing application.

## Forwards Compatibility

Not applicable.

## Example Implementations

- Implementation of ICS 29 in Go following ICS 30 design pattern can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

## History

June 22, 2021 - Draft submitted

July 6, 2022 - Update with latest changes from implementation

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
