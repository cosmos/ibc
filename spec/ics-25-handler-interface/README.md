---
ics: 25
title: Handler Interface
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-23
modified: 2019-04-23
---

# Synopsis

(high-level description of and rationale for specification)

# Specification

(main part of standard document - not all subsections are required)

## Motivation

(rationale for existence of standard)

## Definitions

(definitions of any new terms not defined in common documentation)

## Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

## Technical Specification

### Clients

```golang
type ClientOptions struct {
}
```

```coffeescript
function createClient(ClientOptions options, RootOfTrust rootOfTrust) -> string
```

```coffeescript
function queryClient(string id) -> Maybe<RoofOfTrust>
```

```coffeescript
function updateClient(string id, Header header) -> Maybe<Err>
```

By default, clients are unowned.

### Connections

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
  string          client
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

By default, connections are unowned, but closure is permissioned.

### Channels

```golang
type ChannelOptions struct {
  string      connection
  bool        ordered
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

By default, channels are owned by the creating module.

### Packets

```coffeescript
function sendPacket(Packet packet) -> Future<Maybe<Timeout>>
```

```coffeescript
funtion queryPacket(Packet packet) -> Maybe<PacketInfo>
```

```coffeescript
function recvPacket(string chanId) -> Future<Packet>
```

Packets are permissioned by channel.

#### Example

Simple module for native asset send/receive.

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

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

## Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

## Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

## Example Implementation

(link to or description of concrete example implementation)

## Other Implementations

(links to or descriptions of other implementations)

# History

(changelog and notable inspirations / references)

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
