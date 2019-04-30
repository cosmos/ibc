---
ics: 25
title: Handler Interface
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-23
modified: 2019-04-30
---

# Synopsis

This document describes the interface exposed by the standard IBC implementation (referred to as the IBC handler) to modules within the same state machine, and the implementation of that interface by the IBC handler.

# Specification

## Motivation

IBC is an inter-module communication protocol, designed to faciliate reliable, authentication message passing between modules on separate blockchains. Modules should be able to reason about the interface they interact with and the requirements they must adhere to in order to utilize it safely.

## Definitions

`Client` and `RootOfTrust` are as defined in ICS 2.

`Connection` and `ConnectionState` are as defined in ICS 3.

`Channel` is as defined in ICS 4.

`Packet` is as defined in ICS 5.

## Desired Properties

- Client, connection, channel creation as permissionless as possible
- Dynamic module set
- Modules can write their own more complex abstractions on top of IBC

## Technical Specification

### Clients

By default, clients are unowned: any module can create a new client, query any existing client, and update any existing client.

```golang
type ClientKind enum {
  Tendermint
}
```

```golang
type ClientOptions struct {
  ClientKind  kind
  RootOfTrust rootOfTrust
}
```

```coffeescript
function createClient(ClientOptions options) -> string
```

```coffeescript
function queryClient(string id) -> Maybe<RoofOfTrust>
```

```coffeescript
function updateClient(string id, Header header) -> Maybe<Err>
```


### Connections

By default, connections are unowned, but closure is permissioned to the module which created the connection.

```golang
type ConnectionKind enum {
  Transit
  Receive
  Broadcast
  Bidirectional
}
```

```golang
type ConnectionOptions struct {
  string          clientIdentifier
  ConnectionKind  kind
}
```

```golang
type ConnectionInfo struct {
  ConnectionOptions options
  ConnectionState   state
}
```

```coffeescript
function createConnection(ConnectionOptions options) -> string
```

```coffeescript
function queryConnection(string id) -> Maybe<ConnectionInfo>
```

```coffeescript
function closeConnection(string id) -> Maybe<Err>
```


### Channels

By default, channels are owned by the creating module, meaning only the creating module can inspect, close, or send on the channel.

```golang
type ChannelOptions struct {
  string          connectionIdentifier
  bool            ordered
  recvHandler     Packet -> ()
  timeoutHandler  Maybe<Packet -> ()>
}
```

```golang
type ChannelInfo struct {
  ChannelOptions options
}
```

```coffeescript
function createChannel(ChannelOptions options) -> string
```

```coffeescript
function queryChannel(string id) -> Maybe<ChannelInfo>
```

```coffeescript
function closeChannel(string id) -> Future<Maybe<Err>>
```

### Packets

Packets are permissioned by channel (only a module which owns a channel can send on it).

```coffeescript
function sendPacket(Packet packet) -> Future<Maybe<Timeout>>
```

```coffeescript
function queryPacket(Packet packet) -> Maybe<PacketInfo>
```

#### Example

As a demonstration of interface usage, a simple module handling send/receive of a native asset could be implemented as follows:

```golang
type State struct {
  channel     string
}
```

```golang
type PacketData struct {
  asset         string
  amount        integer
  source        address
  destination   address
}
```

```coffeescript
function myModuleInit()
  client = createClient(rootOfTrust)
  connection = createConnection(nil, client)
  state.channel = createChannel(nil, connection, myModuleRecv, myModuleTimeout)
```

```coffeescript
function myModuleSend(string asset, integer amount, address source, address destination)
  checkSignature(source)
  deductBalance(source, asset, amount)
  escrow(asset, amount)
  sendPacket({
    channel: state.channel,
    data: {
      asset       : asset,
      amount      : amount,
      source      : source,
      destination : destination,
    }
  })
```

```coffeescript
function myModuleRecv(Packet packet)
  assert(packet.channel == channel)
  data = packet.data
  unescrow(data.asset, data.amount)
  increaseBalance(data.destination, data.asset, data.amount)
```

```coffeescript
function myModuleTimeout(Packet packet)
  data = packet.data
  unescrow(packet.asset, packet.amount)
  increaseBalance(packet.source, packet.asset, packet.amount)
```

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

The interface can change when implemented on new chains (or upgrades to an existing chain) as long as the semantics remain the same.

## Example Implementation

Coming soon.

## Other Implementations

Coming soon.

# History

30 April 2019 - Draft written

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
