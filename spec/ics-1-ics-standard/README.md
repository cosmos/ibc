---
ics: 1
title: ICS Specification Standard
stage: proposal
category: meta
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-12
modified: 2019-03-04
---

## What is an ICS?

## Components

### Header

#### Required fields

`ics: #` - ICS number (assigned sequentially)

`title` - ICS title (keep it short & sweet)

`stage` - Current ICS stage, one of the following:
- `proposal` - A standard in the "proposal" stage
- `draft` - A standard in the "draft" stage
- `candidate` - A standard in the "candidate" stage
- `finalized` - A standard in the "finalized" stage

See [README.md](../../README.md) for a description of the ICS acceptance stages.

`category` - ICS category, one of the following:
- `meta` - A standard about the ICS process
- `ibc`  - A standard about the inter-blockchain communication system
- `util` - A standard about utility features, e.g. message signing

`author` - ICS author(s) & contact information (in order of preference: email, Github handle, Twitter handle, other contact methods).
           The first author is the primary "owner" of the ICS and is responsible for advancing it through the standardization process.
           Subsequent author ordering should be in order of contribution amount.

`created` - Date ICS was first created (`YYYY-MM-DD`)

`modified` - Date ICS was last modified (`YYYY-MM-DD`)

#### Optional fields

`requires` - Other ICS standards, referenced by number, which are required or depended upon by this standard.

`required-by` - Other ICS standards, referenced by number, which require or depend upon this standard.

`replaces` - Another ICS standard replaced or supplanted by this standard, if applicable.

`replaced-by` - Another ICS standard which replaces or supplants this standard, if applicable.

### Synopsis

Following the header, each ICS should include a brief (~200 word) synopsis providing a high-level
description of and rationale for the specification.

### Specification

## Formatting

### General

ICS specifications must be written in Github-flavored Markdown.

For a Github-flavored Markdown cheat sheet, see [here](https://github.com/adam-p/markdown-here/wiki/Markdown-Cheatsheet). For a local Markdown renderer, see [here](https://github.com/joeyespo/grip).

### Language

ICS specifications should be written in Simple English, avoiding obscure terminology and unnecessary jargon. For excellent examples of Simple English, please see the [Simple English Wikipedia](https://simple.wikipedia.org/wiki/Main_Page).

### Pseudocode

Pseudocode in specifications should be language-agnostic and formatted in the CS-paper standard, with line numbers, variables, simple conditional blocks, for loops, and
English fragments where necessary to explain further functionality such as scheduling timeouts.

Example pseudocode:

```
11: FunctionStartRound(round):
12:   round_p ← round
13:   step_p ← propose
14:   if proposer(h_p, round_p) = p then
15:     if validValue_p /= nil then
16:       proposal ← validValue_p
17:     else
18:       proposal ← getValue()
19:     broadcast <PROPOSAL, h_p, round_p, proposal, validRound>
20:   else
21:     scheduleOnTimeoutPropose(h_p, round_p) to be executed after timeoutPropose(round_p)
```

## History

This specification was significantly inspired by and derived from Ethereum's [EIP 1](https://github.com/ethereum/EIPs/blob/master/EIPS/eip-1.md), which
was in turn derived from Bitcoin's BIP process and Python's PEP process. Antecedent authors are not responsible for any shortcomings of this ICS spec or
the ICS process. Please direct all comments to the ICS repository maintainers.

March 4th, 2019: Initial ICS 1 draft finished and submitted as a PR

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
