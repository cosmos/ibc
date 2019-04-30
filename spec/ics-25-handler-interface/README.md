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

`ClientKind` enumerates the list of light client algorithms supported by the handler implementation.

```golang
type ClientKind enum {
  Tendermint
}
```

`ClientOptions` contains all the parameter choices required to create a client.

```golang
type ClientOptions struct {
  ClientKind  kind
  RootOfTrust rootOfTrust
}
```

`ClientInfo` contains information about an existing client.

```golang
type ClientInfo struct {
  ClientKind  kind
  RootOfTrust rootOfTrust
}
```

`createClient` creates a new client and returns an automatically allocated identifier.

```coffeescript
function createClient(ClientOptions options) -> string
```

`queryClient` queries a client by a known identifier, returning the associated metadata and root of trust if found.

```coffeescript
function queryClient(string id) -> Maybe<ClientInfo>
```

`updateClient` updates an existing client with a new header, returning an error if the client was not found or the header was not a valid update.

```coffeescript
function updateClient(string id, Header header) -> Maybe<Err>
```


### Connections

By default, connections are unowned, but closure is permissioned to the module which created the connection.

`ConnectionKind` enumerates the connection types supported by the handler implementation.

```golang
type ConnectionKind enum {
  Transit
  Receive
  Broadcast
  Bidirectional
}
```

`ConnectionOptions` contains all the parameter choices required to create a new connection.

```golang
type ConnectionOptions struct {
  string          clientIdentifier
  ConnectionKind  kind
}
```

`ConnectionInfo` contains metadata about & state of an existing connection.

```golang
type ConnectionInfo struct {
  ConnectionOptions options
  ConnectionState   state
}
```

`createConnection` tries to create a new connection with the provided options, failing if the client is not found or the options are invalid, returning a unique allocated identifier if successful.

```coffeescript
function createConnection(ConnectionOptions options) -> Maybe<string>
```

`queryConnection` queries an existing connection by known identifier, returning the associated metadata if found.

```coffeescript
function queryConnection(string id) -> Maybe<ConnectionInfo>
```

`closeConnection` initiates the graceful connection closing process as defined in ICS 3.

```coffeescript
function closeConnection(string id) -> Maybe<Err>
```

### Channels

By default, channels are owned by the creating module, meaning only the creating module can inspect, close, or send on the channel.

`ChannelOptions` contains all the parameter choices required to create a new channel.

```golang
type ChannelOptions struct {
  string          connectionIdentifier
  bool            ordered
  recvHandler     Packet -> ()
  timeoutHandler  Maybe<(Packet, string) -> ()>
}
```

`ChannelInfo` contains metadata about an existing channel.

```golang
type ChannelInfo struct {
  ChannelOptions options
}
```

`createChannel` tries to create a new channel with the provided options, failing if the connection is not found or the options are invalid, returning a unique allocated identifier if successful.

```coffeescript
function createChannel(ChannelOptions options) -> string
```

`queryChannel` queries an existing channel by known identifier, returning the associated metadata if found.

```coffeescript
function queryChannel(string id) -> Maybe<ChannelInfo>
```

`closeChannel` initiates the graceful channel closing process as defined in ICS 4.

```coffeescript
function closeChannel(string id) -> Future<Maybe<Err>>
```

### Packets

Packets are permissioned by channel (only a module which owns a channel can send on it).

`sendPacket` attempts to send a packet, returning an error if the packet cannot be sent (perhaps because the sending module does not own the channel in question), and returning a unique identifier if successful.

The returned identifier will be the same as that sent by the timeout handler, so it can be used by the sending module to associate a specific action with a specific packet timeout.

```coffeescript
function sendPacket(Packet packet) -> string | Err
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
