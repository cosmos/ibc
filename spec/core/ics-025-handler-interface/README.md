---
ics: 25
title: Handler Interface
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 3, 4, 23, 24
version compatibility: ibc-go v7.0.0
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-23
modified: 2019-08-25
---

## Synopsis

This document describes the interface exposed by the standard IBC implementation (referred to as the IBC handler) to modules within the same state machine, and the implementation of that interface by the IBC handler.

### Motivation

IBC is an inter-module communication protocol, designed to facilitate reliable, authenticated message passing between modules on separate blockchains. Modules should be able to reason about the interface they interact with and the requirements they must adhere to in order to utilise the interface safely.

### Definitions

Associated definitions are as defined in referenced prior standards (where the functions are defined), where appropriate.

### Desired Properties

- Creation of clients, connections, and channels should be as permissionless as possible.
- The module set should be dynamic: chains should be able to add and destroy modules, which can themselves bind to and unbind from ports, at will with a persistent IBC handler.
- Modules should be able to write their own more complex abstractions on top of IBC to provide additional semantics or guarantees.

## Technical Specification

> Note: If the host state machine is utilising object capability authentication (see [ICS 005](../ics-005-port-allocation)), all functions utilising ports take an additional capability key parameter.

### Client lifecycle management

By default, clients are unowned: any module may create a new client, query any existing client, update any existing client, and delete any existing client not in use.

The handler interface exposes `createClient`, `updateClient`, `queryConsensusState`, `queryClientState`, and `submitMisbehaviourToClient` as defined in [ICS 2](../ics-002-client-semantics).

### Connection lifecycle management

The handler interface exposes `connOpenInit`, `connOpenTry`, `connOpenAck`, `connOpenConfirm`, and `queryConnection`, as defined in [ICS 3](../ics-003-connection-semantics).

The default IBC routing module SHALL allow external calls to `connOpenTry`, `connOpenAck`, and `connOpenConfirm`.

### Channel lifecycle management

By default, channels are owned by the creating port, meaning only the module bound to that port is allowed to inspect, close, or send on the channel. A module can create any number of channels utilising the same port.

The handler interface exposes `chanOpenInit`, `chanOpenTry`, `chanOpenAck`, `chanOpenConfirm`, `chanCloseInit`, `chanCloseConfirm`, and `queryChannel`, as defined in [ICS 4](../ics-004-channel-and-packet-semantics).

The default IBC routing module SHALL allow external calls to `chanOpenTry`, `chanOpenAck`, `chanOpenConfirm`, and `chanCloseConfirm`.

### Packet relay

Packets are permissioned by channel (only a port which owns a channel can send or receive on it).

The handler interface exposes `sendPacket`, `recvPacket`, `acknowledgePacket`, `timeoutPacket`, and `timeoutOnClose` as defined in [ICS 4](../ics-004-channel-and-packet-semantics).

The default IBC routing module SHALL allow external calls to `sendPacket`, `recvPacket`, `acknowledgePacket`, `timeoutPacket`, and `timeoutOnClose`.

### Properties & Invariants

The IBC handler module interface as defined here inherits properties of functions as defined in their associated specifications.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This interface MAY change when implemented on new chains (or upgrades to an existing chain) as long as the semantics remain the same.

## Example Implementations

Coming soon.

## History

Jun 9, 2019 - Draft written

Aug 24, 2019 - Revisions, cleanup

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
