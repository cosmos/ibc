---
ics: 30
title: IBC Middleware
stage: draft
category: IBC/APP
requires: 4, 25, 26
kind: instantiation
author: Aditya Sripal <aditya@interchain.berlin>, Ethan Frey <ethan@confio.tech>
created: 2021-06-01
modified: 2021-06-18
---

## Synopsis

This standard documents specifies the interfaces and state machine logic that a module must implement in order to act as middleware between core IBC and an underlying application(s). IBC Middleware will enable arbitrary extensions to an application's functionality without requiring changes to the application or core IBC.

### Motivation

IBC applications are designed to be self-contained modules that implement their own application-specific logic through a set of interfaces with the core IBC handlers. These core IBC handlers, in turn, are designed to enforce the correctness properties of IBC (transport, authentication, ordering) while delegating all application-specific handling to the IBC application modules. However, there are cases where some functionality may be desired by many applications, yet not appropriate to place in core IBC. The most prescient example of this, is the generalized middleware payment protocol. Most applications will want to opt in to a protocol that incentivizes relayers to relay packets on their channel. However, some may not wish to enable this feature and yet others will want to implement their own custom logic.

Without a middleware approach, developers must choose whether to place this extension to application logic inside each relevant application; or place the logic in core IBC. Placing it in each application is redundant and prone to error. Placing the logic in core IBC requires an opt-in from all applications and violates the abstraction barrier between core IBC (tao) and the application. Either case is not scalable as the number of extensions increase, since this must either increase code bloat in applications or core IBC handlers.

Middleware allows developers to define the extensions as seperate modules that can wrap over the end application. This middleware can thus perform its own custom logic, and pass data into the application so that it may run its logic without being aware of the middleware's existence. This allows both the application and the middleware to implement its own isolated logic while still being able to run as part of a single packet flow.

### Desired Properties

- Middleware enables arbitrary extensions of application logic
- Middleware can be arbitrarily nested to create a chain of app extensions
- Core IBC does not need to change
- Base Application logic does not need to change

## Technical Specification

### General Design

In order to function as IBC Middleware, a module must implement the IBC application callbacks and pass along the pre-processed data to the nested application. It must also implement `WriteAcknowledgement` and `SendPacket`, which will be called by the end application, so that it may post-process the information before passing data along to core ibc.

When nesting an application, the module must make sure that it is in the middle of communication between core IBC and the application in both directions. Developers should do this by registering the top-level module directly with the IBC router (not any nested applications). The nested applications in turn, must be given access only to the middleware's `WriteAcknowledgement` and `SendPacket` rather than to the core IBC handlers directly.

Additionally, the middleware must take care to ensure that the application logic can execute its own port and version negotiation without interference from the nesting middleware. In order to do this, the middleware will prepend the portID and version with its own portID and version. In the application callbacks, the middleware must do its own version and port negotation and then strip out the prefixes before handing over the data to the nested application's callback. Middleware SHOULD always prepend the portID with its own port. This will allow the original application to also exist as a top-level module connected to the IBC Router.

PortID: `{middleware_port}:{app_port}`

Version: `{middleware_version}:{app_version}`

#### Handshake Callbacks

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
    middlewarePort, appPort = splitMiddlewarePort(portID)
    middlewareVersion, appVersion = splitMiddlewareVersion(version)
    cpMiddlewarePort, cpAppPort = splitMiddlewarePort(counterpartyPortIdentifier)
    if !isCompatible(middlewarePort, cpMiddlewarePort) {
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
      cpMiddlewareVersion, cpAppVersion = splitMiddlewareVersion(counterpartyVersion)
      middlewareVersion, appVersion = splitMiddlewareVersion(version)
      middlewarePort, appPort = splitMiddlewarePort(portID)
      cpMiddlewarePort, cpAppPort = splitMiddlewarePort(counterpartyPortIdentifier)
      if !isCompatible(middlewarePort, cpMiddlewarePort) {
          return error
      }
      if !isCompatible(cpMiddlewareVersion, middlewareVersion) {
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

function splitMiddlewareVersion(version: string): []string {
    splitVersions = split(version,  ":")
    middlewareVersion = version[0]
    appVersion = join(version[1:], ":")
    return []string{middlewareVersion, appVersion}
}

function splitMiddlewarePort(portID: string) []string {
    // identical logic to splitMiddlewareVersion
}
```

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

    app.OnAcknowledgePacket(packet, app_ack, relayer)

    doCustomLogic()
}

function onTimeoutPacket(packet: Packet, relayer: string) {
    doCustomLogic()

    app.OnTimeoutPacket(packet, relayer)

    doCustomLogic()
}

function onTimeoutPacketClose(packet: Packet, relayer: string) {
    doCustomLogic()

    app.onTimeoutPacketClose(packet, relayer)

    doCustomLogic()
}
```

#### ICS-4 Wrappers

```typescript
function writeAcknowledgement(
  packet: Packet,
  acknowledgement: bytes) {
    // middleware may modify acknowledgement
    ack_bytes = doCustomLogic(acknowledgement)

    return ics4.writeAcknowledgement(packet, ack_bytes)
}
```

```typescript
function sendPacket(app_packet: Packet) {
    // middleware may modify packet
    packet = doCustomLogic(app_packet)

    return ics4.sendPacket(packet)
}
```