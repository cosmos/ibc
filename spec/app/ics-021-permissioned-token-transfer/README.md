---
ics: 21
title: Permissioned Token Transfer
stage: draft
category: IBC/APP
kind: instantiation
author: John Letey <john@nobleassets.xyz>, Daniel Kanefsky <dan@nobleassets.xyz>
created: 2024-06-14
modified: 2024-06-14
requires: 25, 26
required-by: (optional list of ics numbers)
implements: (optional list of ics numbers)
version compatibility: (optional list of compatible implementations' releases)
---

> This standard document follows the same design principles of [ICS 20](../ics-020-fungible-token-transfer) and inherits most of its content therefrom.

## Synopsis

(high-level description of and rationale for specification)

### Motivation

(rationale for existence of standard)

### Definitions

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

### Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

## Technical Specification

(main part of standard document - not all subsections are required)

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Data Structures

(new data structures, if applicable)

### Sub-protocols

(sub-protocols, if applicable)

### Properties & Invariants

(properties & invariants maintained by the protocols specified, if applicable)

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version `"ics21-1"` in the channel handshake.

A future version of this standard could use a different version in the channel handshake, and safely alter the packet data format & packet handler semantics.

## Example Implementations

(links to or descriptions of concrete example implementations)

## History

(changelog and notable inspirations / references)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
