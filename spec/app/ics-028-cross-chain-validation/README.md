---
ics: 28
title: Cross-Chain Validation
stage: draft
category: IBC/APP
requires: 25, 26
kind: 
author: 
created: 
modified: 
---

<!-- omit in toc -->
# Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for Cross-Chain Validation (CCV). Note that CCV is the specific IBC level protocol that enables *Interchain Security*, a Cosmos-specific category of *Shared Security*.

At a high level, CCV enables a *provider chain* (e.g., the Cosmos Hub) to provide *security* to multiple *consumer chains*. This means that the validator sets on the consumer chains are chosen from the validator sets of the provider chain (for more details, see the [Security Model](./overview_and_basic_concepts.md#security-model) section).

The communication between the provider and the consumer chains is done through the IBC protocol over a *unique*, *ordered* channel (one for each consumer chain). 

> Throughout this document, we will use the terms chain and blockchain interchangeably.

## Contents
- [Overview and Basic Concepts](./overview_and_basic_concepts.md)
- [System Model and Properties](./system_model_and_properties.md)
- [Technical Specification: Data Structures and Methods](./technical_specification.md)

<!--
## Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

## Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

## Example Implementation

(link to or description of concrete example implementation)

## Other Implementations

(links to or descriptions of other implementations)

## History

(changelog and notable inspirations / references)
 -->

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
