---
ics: 23
title: Standard Accumulator
stage: draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-04-16
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

Two parties:
- Accumulator manager (initializes, adds / removes elements)
- Third-party verifier

Functions

### Algorithms

required

- Gen(S_0) -> a_0
- Add(a_t, x) -> (a_t+1)
- Del(a_t, x) -> (a_t+1)

- MemWitCreate(a_t, x) -> w_xt
- NonMemWitCreate(a_t, x) -> w_xt

- VerMem(a_t, x, w_xt) -> {0, 1}
- VerNonMem(a_t, x, w_xt) -> {0, 1}

must be
- correct: for an element in the accumulator, vermem(element) => 1, vernonmem(element) => 0
- sound: for an element not in the accumulator, vernonmem(element) => 1, vermem(element) => 0

optional

- BatchVerifyMem(a_t, {x}, w_xt) -> {0, 1}
- BatchVerifyNonMem(a_t, {x}, w_xt) -> {0, 1}

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

Security definitions are mostly sources from https://eprint.iacr.org/2017/043.pdf
Also from https://eprint.iacr.org/2018/1188.pdf

(changelog and notable inspirations / references)

# Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
