---
ics: 26
title: Routing Module
stage: Draft
category: IBC/TAO
kind: instantiation
version compatibility: ibc-go v7.0.0
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-06-09
modified: 2019-08-25
---

## Synopsis

The routing module is a default implementation of a secondary module which will accept external datagrams and call into the interblockchain communication protocol handler to deal with handshakes and packet relay.
The routing module keeps a lookup table of modules, which it can use to look up and call a module when a packet is received, so that external relayers need only ever relay packets to the routing module.

### Motivation

The default IBC handler uses a receiver call pattern, where modules must individually call the IBC handler in order to bind to ports, start handshakes, accept handshakes, send and receive packets, etc. This is flexible and simple but is a bit tricky to understand and may require extra work on the part of relayer processes, who must track the state of many modules. This standard describes an IBC "routing module" to automate most common functionality, route packets, and simplify the task of relayers.

The routing module can also play the role of the module manager as discussed in [ICS 5](../ics-005-port-allocation) and implement
logic to determine when modules are allowed to bind to ports and what those ports can be named.

### Definitions

All functions provided by the IBC handler interface are defined as in [ICS 25](../ics-025-handler-interface).

The functions `newCapability` & `authenticateCapability` are defined as in [ICS 5](../ics-005-port-allocation).

The functions `writeChannel` and `writeAcknowledgement` are defined as in [ICS 4](../ics-004-channel-and-packet-semantics)

### Desired Properties

- Modules should be able to bind to ports and own channels through the routing module.
- No overhead should be added for packet sends and receives other than the layer of call indirection.
- The routing module should call specified handler functions on modules when they need to act upon packets.

## Technical Specification

> Note: If the host state machine is utilising object capability authentication (see [ICS 005](../ics-005-port-allocation)), all functions utilising ports take an additional capability parameter.

### Module callback interface

Modules must expose the following function signatures to the routing module, which are called upon the receipt of various datagrams:

#### **OnChanOpenInit**

`onChanOpenInit` will verify that the relayer-chosen parameters
are valid and perform any custom `INIT` logic.
It may return an error if the chosen parameters are invalid
in which case the handshake is aborted.
If the provided version string is non-empty, `onChanOpenInit` should return
the version string or an error if the provided version is invalid.
If the version string is empty, `onChanOpenInit` is expected to
return a default version string representing the version(s)
it supports.
If there is no default version string for the application,
it should return an error if provided version is empty string.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
    // defined by the module
}
```

#### **OnChanOpenTry**

`onChanOpenTry` will verify the INIT-chosen parameters along with the
counterparty-chosen version string and perform custom `TRY` logic.
If the INIT-chosen parameters
are invalid, the callback must return an error to abort the handshake.
If the counterparty-chosen version is not compatible with this modules
supported versions, the callback must return an error to abort the handshake.
If the versions are compatible, the try callback must select the final version
string and return it to core IBC.
`onChanOpenTry` may also perform custom initialization logic

```typescript
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) => (version: string, err: Error) {
    // defined by the module
}
```

#### **OnChanOpenAck**

`onChanOpenAck` will error if the counterparty selected version string
is invalid to abort the handshake. It may also perform custom ACK logic.

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier, 
  counterpartyVersion: string) {
    // defined by the module
}
```

#### **OnChanOpenConfirm**

`onChanOpenConfirm` will perform custom CONFIRM logic and may error to abort the handshake.

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}

function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier): void {
    // defined by the module
}

function onRecvPacket(packet: Packet, relayer: string): bytes {
    // defined by the module, returns acknowledgement
}

function onTimeoutPacket(packet: Packet, relayer: string) {
    // defined by the module
}

function onAcknowledgePacket(packet: Packet, acknowledgement: bytes, relayer: string) {
    // defined by the module
}

function onTimeoutPacketClose(packet: Packet, relayer: string) {
    // defined by the module
}
```

Exceptions MUST be thrown to indicate failure and reject the handshake, incoming packet, etc.

These are combined together in a `ModuleCallbacks` interface:

```typescript
interface ModuleCallbacks {
  onChanOpenInit: onChanOpenInit
  onChanOpenTry: onChanOpenTry
  onChanOpenAck: onChanOpenAck
  onChanOpenConfirm: onChanOpenConfirm
  onChanCloseInit: onChanCloseInit
  onChanCloseConfirm: onChanCloseConfirm
  onRecvPacket: onRecvPacket
  onTimeoutPacket: onTimeoutPacket
  onAcknowledgePacket: onAcknowledgePacket
  onTimeoutPacketClose: onTimeoutPacketClose
}
```

Callbacks are provided when the module binds to a port.

```typescript
function callbackPath(portIdentifier: Identifier): Path {
    return "callbacks/{portIdentifier}"
}
```

The calling module identifier is also stored for future authentication should the callbacks need to be altered.

```typescript
function authenticationPath(portIdentifier: Identifier): Path {
    return "authentication/{portIdentifier}"
}
```

### Port binding as module manager

The IBC routing module sits in-between the handler module ([ICS 25](../ics-025-handler-interface)) and individual modules on the host state machine.

The routing module, acting as a module manager, differentiates between two kinds of ports:

- "Existing name” ports: e.g. “bank”, with standardised prior meanings, which should not be first-come-first-serve
- “Fresh name” ports: new identity (perhaps a smart contract) w/no prior relationships, new random number port, post-generation port name can be communicated over another channel

A set of existing names are allocated, along with corresponding modules, when the routing module is instantiated by the host state machine.
The routing module then allows allocation of fresh ports at any time by modules, but they must use a specific standardised prefix.

The function `bindPort` can be called by a module in order to bind to a port, through the routing module, and set up callbacks.

```typescript
function bindPort(
  id: Identifier,
  callbacks: Callbacks): CapabilityKey {
    abortTransactionUnless(privateStore.get(callbackPath(id)) === null)
    privateStore.set(callbackPath(id), callbacks)
    capability = handler.bindPort(id)
    claimCapability(authenticationPath(id), capability)
    return capability
}
```

The function `updatePort` can be called by a module in order to alter the callbacks.

```typescript
function updatePort(
  id: Identifier,
  capability: CapabilityKey,
  newCallbacks: Callbacks) {
    abortTransactionUnless(authenticateCapability(authenticationPath(id), capability))
    privateStore.set(callbackPath(id), newCallbacks)
}
```

The function `releasePort` can be called by a module in order to release a port previously in use.

> Warning: releasing a port will allow other modules to bind to that port and possibly intercept incoming channel opening handshakes. Modules should release ports only when doing so is safe.

```typescript
function releasePort(
  id: Identifier,
  capability: CapabilityKey) {
    abortTransactionUnless(authenticateCapability(authenticationPath(id), capability))
    handler.releasePort(id)
    privateStore.delete(callbackPath(id))
    privateStore.delete(authenticationPath(id))
}
```

The function `lookupModule` can be used by the routing module to lookup the callbacks bound to a particular port.

```typescript
function lookupModule(portId: Identifier) {
    return privateStore.get(callbackPath(portId))
}
```

### Datagram handlers (write)

*Datagrams* are external data blobs accepted as transactions by the routing module. This section defines a *handler function* for each datagram,
which is executed when the associated datagram is submitted to the routing module in a transaction.

All datagrams can also be safely submitted by other modules to the routing module.

No message signatures or data validity checks are assumed beyond those which are explicitly indicated.

#### Client lifecycle management

`ClientCreate` creates a new light client with the specified identifier & consensus state.

```typescript
interface ClientCreate {
  identifier: Identifier
  clientState: ClientState
  consensusState: ConsensusState
}
```

```typescript
function handleClientCreate(datagram: ClientCreate) {
    handler.createClient(datagram.clientState, datagram.consensusState)
}
```

`ClientUpdate` updates an existing light client with the specified identifier & new header.

```typescript
interface ClientUpdate {
  identifier: Identifier
  header: Header
}
```

```typescript
function handleClientUpdate(datagram: ClientUpdate) {
    handler.updateClient(datagram.identifier, datagram.header)
}
```

`ClientSubmitMisbehaviour` submits proof-of-misbehaviour to an existing light client with the specified identifier.

```typescript
interface ClientMisbehaviour {
  identifier: Identifier
  evidence: bytes
}
```

```typescript
function handleClientMisbehaviour(datagram: ClientMisbehaviour) {
    handler.submitMisbehaviourToClient(datagram.identifier, datagram.evidence)
}
```

#### Connection lifecycle management

The `ConnOpenInit` datagram starts the connection handshake process with an IBC module on another chain.

```typescript
interface ConnOpenInit {
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string
  delayPeriodTime: uint64
  delayPeriodBlocks: uint64
}
```

```typescript
function handleConnOpenInit(datagram: ConnOpenInit) {
  handler.connOpenInit(
    datagram.counterpartyPrefix,
    datagram.clientIdentifier,
    datagram.counterpartyClientIdentifier,
    datagram.version,
    datagram.delayPeriodTime,
    datagram.delayPeriodBlocks
  )
}
```

The `ConnOpenTry` datagram accepts a handshake request from an IBC module on another chain.

```typescript
interface ConnOpenTry {
  counterpartyConnectionIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  counterpartyClientIdentifier: Identifier
  clientIdentifier: Identifier
  clientState: ClientState
  counterpartyVersions: string[]
  delayPeriodTime: uint64
  delayPeriodBlocks: uint64
  proofInit: CommitmentProof
  proofClient: CommitmentProof
  proofConsensus: CommitmentProof
  proofHeight: Height
  consensusHeight: Height
}
```

```typescript
function handleConnOpenTry(datagram: ConnOpenTry) {
  handler.connOpenTry(
    datagram.counterpartyConnectionIdentifier,
    datagram.counterpartyPrefix,
    datagram.counterpartyClientIdentifier,
    datagram.clientIdentifier,
    datagram.clientState,
    datagram.counterpartyVersions,
    datagram.delayPeriodTime,
    datagram.delayPeriodBlocks,
    datagram.proofInit,
    datagram.proofClient,
    datagram.proofConsensus,
    datagram.proofHeight,
    datagram.consensusHeight
  )
}
```

The `ConnOpenAck` datagram confirms a handshake acceptance by the IBC module on another chain.

```typescript
interface ConnOpenAck {
  identifier: Identifier
  clientState: ClientState
  version: string
  counterpartyIdentifier: Identifier
  proofTry: CommitmentProof
  proofClient: CommitmentProof
  proofConsensus: CommitmentProof
  proofHeight: Height
  consensusHeight: Height
}
```

```typescript
function handleConnOpenAck(datagram: ConnOpenAck) {
  handler.connOpenAck(
    datagram.identifier,
    datagram.clientState,
    datagram.version,
    datagram.counterpartyIdentifier,
    datagram.proofTry,
    datagram.proofClient,
    datagram.proofConsensus,
    datagram.proofHeight,
    datagram.consensusHeight
  )
}
```

The `ConnOpenConfirm` datagram acknowledges a handshake acknowledgement by an IBC module on another chain & finalises the connection.

```typescript
interface ConnOpenConfirm {
  identifier: Identifier
  proofAck: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handleConnOpenConfirm(datagram: ConnOpenConfirm) {
    handler.connOpenConfirm(
      datagram.identifier,
      datagram.proofAck,
      datagram.proofHeight
    )
}
```

#### Channel lifecycle management

```typescript
interface ChanOpenInit {
  order: ChannelOrder
  connectionHops: [Identifier]
  portIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  version: string
}
```

```typescript
function handleChanOpenInit(datagram: ChanOpenInit) {
  module = lookupModule(datagram.portIdentifier)
  channelIdentifier, channelCapability = handler.chanOpenInit(
    datagram.order,
    datagram.connectionHops,
    datagram.portIdentifier,
    datagram.counterpartyPortIdentifier
  )
  version, err = module.onChanOpenInit(
    channelCapability, // pass in channel capability so that module can claim it (if needed)
    datagram.order,
    datagram.connectionHops,
    datagram.portIdentifier,
    channelIdentifier,
    datagram.counterpartyPortIdentifier,
    datagram.version
  )
  abortTransactionUnless(err === nil)
  writeChannel(
    datagram.portIdentifier,
    channelIdentifier,
    INIT,
    datagram.order,
    datagram.counterpartyPortIdentifier,
    datagram.counterpartyChannelIdentifier,
    datagram.connectionHops,
    version
  )
}
```

```typescript
interface ChanOpenTry {
  order: ChannelOrder
  connectionHops: [Identifier]
  portIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  counterpartyVersion: string
  proofInit: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handleChanOpenTry(datagram: ChanOpenTry) {
  module = lookupModule(datagram.portIdentifier)
  channelIdentifier, channelCapability = handler.chanOpenTry(
    datagram.order,
    datagram.connectionHops,
    datagram.portIdentifier,
    datagram.channelIdentifier,
    datagram.counterpartyPortIdentifier,
    datagram.counterpartyChannelIdentifier,
    datagram.counterpartyVersion,
    datagram.proofInit,
    datagram.proofHeight
  )
  version, err = module.onChanOpenTry(
    channelCapability, // pass in channel capability so that module can claim it (if needed)
    datagram.order,
    datagram.connectionHops,
    datagram.portIdentifier,
    channelIdentifier,
    datagram.counterpartyPortIdentifier,
    datagram.counterpartyChannelIdentifier,
    datagram.counterpartyVersion
  )
  abortTransactionUnless(err === nil)
  writeChannel(
    datagram.portIdentifier,
    channelIdentifier,
    TRYOPEN,
    datagram.order,
    datagram.counterpartyPortIdentifier,
    datagram.counterpartyChannelIdentifier,
    datagram.connectionHops,
    version
  )
}
```

```typescript
interface ChanOpenAck {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  counterpartyVersion: string
  proofTry: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handleChanOpenAck(datagram: ChanOpenAck) {
  module = lookupModule(datagram.portIdentifier)
  handler.chanOpenAck(
    datagram.portIdentifier,
    datagram.channelIdentifier,
    datagram.counterpartyChannelIdentifier,
    datagram.counterpartyVersion,
    datagram.proofTry,
    datagram.proofHeight
  )
  err = module.onChanOpenAck(
    datagram.portIdentifier,
    datagram.channelIdentifier,
    datagram.counterpartyChannelIdentifier,
    datagram.counterpartyVersion
  )
  abortTransactionUnless(err === nil)
}
```

```typescript
interface ChanOpenConfirm {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  proofAck: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handleChanOpenConfirm(datagram: ChanOpenConfirm) {
  module = lookupModule(datagram.portIdentifier)
  handler.chanOpenConfirm(
    datagram.portIdentifier,
    datagram.channelIdentifier,
    datagram.proofAck,
    datagram.proofHeight
  )
  err = module.onChanOpenConfirm(
    datagram.portIdentifier,
    datagram.channelIdentifier
  )
  abortTransactionUnless(err === nil)
}
```

```typescript
interface ChanCloseInit {
  portIdentifier: Identifier
  channelIdentifier: Identifier
}
```

```typescript
function handleChanCloseInit(datagram: ChanCloseInit) {
    module = lookupModule(datagram.portIdentifier)
    err = module.onChanCloseInit(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
    abortTransactionUnless(err === nil)
    handler.chanCloseInit(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
}
```

```typescript
interface ChanCloseConfirm {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  proofInit: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handleChanCloseConfirm(datagram: ChanCloseConfirm) {
    module = lookupModule(datagram.portIdentifier)
    err = module.onChanCloseConfirm(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
    abortTransactionUnless(err === nil)
    handler.chanCloseConfirm(
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.proofInit,
      datagram.proofHeight
    )
}
```

#### Packet relay

Packets are sent by the module directly (by the module calling the IBC handler).

```typescript
interface PacketRecv {
  packet: Packet
  proof: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handlePacketRecv(datagram: PacketRecv) {
  module = lookupModule(datagram.packet.destPort)
  handler.recvPacket(
    datagram.packet,
    datagram.proof,
    datagram.proofHeight,
    acknowledgement
  )
  acknowledgement = module.onRecvPacket(datagram.packet)
  writeAcknowledgement(datagram.packet, acknowledgement)
}
```

```typescript
interface PacketAcknowledgement {
  packet: Packet
  acknowledgement: string
  proof: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handlePacketAcknowledgement(datagram: PacketAcknowledgement) {
  module = lookupModule(datagram.packet.sourcePort)
  handler.acknowledgePacket(
    datagram.packet,
    datagram.acknowledgement,
    datagram.proof,
    datagram.proofHeight
  )
  module.onAcknowledgePacket(
    datagram.packet,
    datagram.acknowledgement
  )   
}
```

#### Packet timeouts

```typescript
interface PacketTimeout {
  packet: Packet
  proof: CommitmentProof
  proofHeight: Height
  nextSequenceRecv: Maybe<uint64>
}
```

```typescript
function handlePacketTimeout(datagram: PacketTimeout) {
  module = lookupModule(datagram.packet.sourcePort)
  handler.timeoutPacket(
    datagram.packet,
    datagram.proof,
    datagram.proofHeight,
    datagram.nextSequenceRecv
  )
  module.onTimeoutPacket(datagram.packet)
}
```

```typescript
interface PacketTimeoutOnClose {
  packet: Packet
  proof: CommitmentProof
  proofHeight: Height
}
```

```typescript
function handlePacketTimeoutOnClose(datagram: PacketTimeoutOnClose) {
  module = lookupModule(datagram.packet.sourcePort)
  handler.timeoutOnClose(
    datagram.packet,
    datagram.proof,
    datagram.proofHeight
  )
  module.onTimeoutPacket(datagram.packet)
}
```

#### Closure-by-timeout & packet cleanup

```typescript
interface PacketCleanup {
  packet: Packet
  proof: CommitmentProof
  proofHeight: Height
  nextSequenceRecvOrAcknowledgement: Either<uint64, bytes>
}
```

### Query (read-only) functions

All query functions for clients, connections, and channels should be exposed (read-only) directly by the IBC handler module.

### Interface usage example

See [ICS 20](../../app/ics-020-fungible-token-transfer) for a usage example.

### Properties & Invariants

- Proxy port binding is first-come-first-serve: once a module binds to a port through the IBC routing module, only that module can utilise that port until the module releases it.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

routing modules are closely tied to the IBC handler interface.

## Example Implementations

- Implementation of ICS 26 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).
- Implementation of ICS 26 in Rust can be found in [ibc-rs repository](https://github.com/cosmos/ibc-rs).

## History

Jun 9, 2019 - Draft submitted

Jul 28, 2019 - Major revisions

Aug 25, 2019 - Major revisions

Mar 28, 2023 - Fix order of executing module handlee and application callback

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
