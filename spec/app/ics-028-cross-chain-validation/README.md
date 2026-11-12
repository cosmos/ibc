---
# SPDX-License-Identifier: Community-Spec-1.0
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

## Example Implementations

- Interchain Security [Go implementation](https://github.com/cosmos/interchain-security).

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

This specification is licensed under the Community Specification License 1.0, and source code included in it under the Apache License 2.0 unless otherwise marked. In the case of any conflict or confusion within this specification between the Community Specification License and the designated source code license, the terms of the Community Specification License shall apply.

The repository uses three licenses, as set out in [Copyright and license](https://github.com/lfdt-ibc/ibc/blob/main/CONTRIBUTING.md#copyright-and-license):

- **Specifications:** [Community Specification License 1.0](https://github.com/lfdt-ibc/ibc/blob/main/spec/01-community-specification-license-v1.md) ([canonical text](https://github.com/CommunitySpecification/1.0))
- **Code:** [Apache License 2.0](https://github.com/lfdt-ibc/ibc/blob/main/LICENSE) ([canonical text](https://www.apache.org/licenses/LICENSE-2.0))
- **Documentation:** [Creative Commons Attribution 4.0 International](https://github.com/lfdt-ibc/ibc/blob/main/docs/LICENSE) ([canonical text](https://creativecommons.org/licenses/by/4.0/legalcode))

THESE MATERIALS ARE PROVIDED “AS IS.” The Contributors and Licensees expressly disclaim any warranties (express, implied, or otherwise), including implied warranties of merchantability, non-infringement, fitness for a particular purpose, or title, related to the materials.  The entire risk as to implementing or otherwise using the materials is assumed by the implementer and user. IN NO EVENT WILL THE CONTRIBUTORS OR LICENSEES BE LIABLE TO ANY OTHER PARTY FOR LOST PROFITS OR ANY FORM OF INDIRECT, SPECIAL, INCIDENTAL, OR CONSEQUENTIAL DAMAGES OF ANY CHARACTER FROM ANY CAUSES OF ACTION OF ANY KIND WITH RESPECT TO THIS DELIVERABLE OR ITS GOVERNING AGREEMENT, WHETHER BASED ON BREACH OF CONTRACT, TORT (INCLUDING NEGLIGENCE), OR OTHERWISE, AND WHETHER OR NOT THE OTHER MEMBER HAS BEEN ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
