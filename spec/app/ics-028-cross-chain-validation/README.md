---
ics: 28
title: Cross-Chain Validation
stage: draft
category: IBC/APP
requires: 25, 26, 20
author: Marius Poke <marius@informal.systems>, Aditya Sripal <aditya@interchain.io>, Jovan Komatovic <jovan.komatovic@epfl.ch>, Cezara Dragoi <cezara.dragoi@inria.fr>, Josef Widder <josef@informal.systems>
created: 2022-06-27
modified: 2022-12-02
---

<!-- omit in toc -->
# Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for Cross-Chain Validation (CCV). CCV is the specific IBC level protocol that enables *Interchain Security*, a Cosmos-specific category of *Shared Security*.

At a high level, CCV enables a *provider chain* (e.g., the Cosmos Hub) to provide *security* to multiple *consumer chains*. This means that the validator sets on the consumer chains are chosen from the validator set of the provider chain (for more details, see the [Security Model](./overview_and_basic_concepts.md#security-model) section).

The communication between the provider and the consumer chains is done through the IBC protocol over a *unique*, *ordered* channel (one for each consumer chain). 

> Throughout this document, we will use the terms chain and blockchain interchangeably.

## Contents
- [Overview and Basic Concepts](./overview_and_basic_concepts.md)
- [System Model and Properties](./system_model_and_properties.md)
- [Technical Specification: Data Structures and Methods](./technical_specification.md)
  - [Data Structures](./data_structures.md)
  - [Methods](./methods.md)

<!--
## Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)


## Forwards Compatibility

-->

## Example Implementation

Interchain Security [Go implementation](https://github.com/cosmos/interchain-security).


<!--
## Other Implementations

(links to or descriptions of other implementations)

-->

## History

Jun 27, 2022 - Draft written

Aug 3, 2022 - Revision of *Bond-Based Consumer Voting Power* property

Aug 29, 2022 - Notify Staking module of matured unbondings in `EndBlock()`

Dec 2, 2022 - Enable existing chains to become consumer chains

Dec 7, 2022 - Add provider-based timeouts 

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
