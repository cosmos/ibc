---
ics: 1
title: ICS Specification Standard
stage: draft
category: meta
kind: meta
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-12
modified: 2019-08-25
---

## What is an ICS?

An interchain standard (ICS) is a design document describing a particular protocol,
standard, or feature expected to be of use to the Cosmos ecosystem.
An ICS should list the desired properties of the standard, explain the design rationale, and
provide a concise but comprehensive technical specification. The primary ICS author
is responsible for pushing the proposal through the standardisation process, soliciting
input and support from the community, and communicating with relevant stakeholders to
ensure (social) consensus.

The interchain standardisation process should be the primary vehicle for proposing
ecosystem-wide protocols, changes, and features, and ICS documents should persist after
consensus as a record of design decisions and an information repository for future implementers.

Interchain standards should *not* be used for proposing changes to a particular blockchain
(such as the Cosmos Hub), specifying implementation particulars (such as language-specific data structures),
or debating governance proposals on existing Cosmos blockchains (although it is possible
that individual blockchains in the Cosmos ecosystem may utilise their governance processes
to approve or reject interchain standards).

## Components

An ICS consists of a header, synopsis, specification, history log, and copyright notice. All top-level sections are required.
References should be included inline as links, or tabulated at the bottom of the section if necessary.

### Header

An ICS header contains metadata relevant to the ICS.

#### Required fields

`ics: #` - ICS number (assigned sequentially)

`title` - ICS title (keep it short & sweet)

`stage` - Current ICS stage, see [PROCESS.md](../../meta/PROCESS.md) for the list of possible stages.

See [README.md](../../README.md) for a description of the ICS acceptance stages.

`category` - ICS category, one of the following:

- `meta` - A standard about the ICS process.
- `IBC/TAO` - A standard about an inter-blockchain communication system core transport, authentication, and ordering layer protocol.
- `IBC/APP` - A standard about an inter-blockchain communication system application layer protocol.

`kind` - ICS kind, one of the following:

- `meta` - A standard about the ICS process.
- `interface` - A standard about the minimal set of interfaces that must be provided and properties that must be fulfilled by a state machine hosting an implementation of the inter-blockchain communication protocol.
- `instantiation` - A standard about concrete implementation details that explains how the standard is realized in pseudocode or software components.

`author` - ICS author(s) & contact information (in order of preference: email, GitHub handle, Twitter handle, other contact methods likely to elicit response).
           The first author is the primary "owner" of the ICS and is responsible for advancing it through the standardisation process.
           Subsequent author ordering should be in order of contribution amount.

`created` - Date ICS was first created (`YYYY-MM-DD`)

`modified` - Date ICS was last modified (`YYYY-MM-DD`)

#### Optional fields

`requires` - Other ICS standards, referenced by number, which are required or depended upon by this standard.

`required-by` - Other ICS standards, referenced by number, which require or depend upon this standard.

`replaces` - Another ICS standard replaced or supplanted by this standard, if applicable.

`replaced-by` - Another ICS standard which replaces or supplants this standard, if applicable.

`version compatibility` - List of versions of implementations compatible with the ICS standard.

### Synopsis

Following the header, an ICS should include a brief (~200 word) synopsis providing a high-level
description of and rationale for the specification.

### Specification

The specification section is the main component of an ICS, and should contain protocol documentation, design rationale,
required references, and technical details where appropriate.

#### Sub-components

The specification may have any or all of the following sub-components, as appropriate to the particular ICS. Included sub-components should be listed in the order specified here.

- *Motivation* - A rationale for the existence of the proposed feature, or the proposed changes to an existing feature.
- *Definitions* - A list of new terms or concepts utilised in this ICS or required to understand this ICS. Any terms not defined in the top-level "docs" folder must be defined here.
- *Desired Properties* - A list of the desired properties or characteristics of the protocol or feature specified, and expected effects or failures when the properties are violated.
- *Technical Specification* - All technical details of the proposed protocol including syntax, semantics, sub-protocols, data structures, algorithms, and pseudocode as appropriate.
    The technical specification should be detailed enough such that separate correct implementations of the specification without knowledge of each other are compatible.
- *Backwards Compatibility* - A discussion of compatibility (or lack thereof) with previous feature or protocol versions.
- *Forwards Compatibility* - A discussion of compatibility (or lack thereof) with future possible or expected features or protocol versions.
- *Example Implementations* - Concrete example implementations or descriptions of expected implementations to serve as the primary reference for implementers.
- *Other Implementations* - A list of candidate or finalised implementations (external references, not inline).

### History

An ICS should include a history section, listing any inspiring documents and a plaintext log of significant changes.

See an example history section [below](#history-1).

### Copyright

An ICS should include a copyright section waiving rights via [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).

## Formatting

### General

ICS specifications must be written in GitHub-flavoured Markdown.

For a GitHub-flavoured Markdown cheat sheet, see [here](https://github.com/adam-p/markdown-here/wiki/Markdown-Cheatsheet). For a local Markdown renderer, see [here](https://github.com/joeyespo/grip).

### Language

ICS specifications should be written in Simple English, avoiding obscure terminology and unnecessary jargon. For excellent examples of Simple English, please see the [Simple English Wikipedia](https://simple.wikipedia.org/wiki/Main_Page).

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in specifications are to be interpreted as described in [RFC 2119](https://tools.ietf.org/html/rfc2119).

### Pseudocode

Pseudocode in specifications should be language-agnostic and formatted in a simple imperative standard, with line numbers, variables, simple conditional blocks, for loops, and
English fragments where necessary to explain further functionality such as scheduling timeouts. LaTeX images should be avoided because they are difficult to review in diff form.

Pseudocode for structs should be written in simple Typescript, as interfaces.

Example pseudocode struct:

```typescript
interface Connection {
  state: ConnectionState
  version: Version
  counterpartyIdentifier: Identifier
  consensusState: ConsensusState
}
```

Pseudocode for algorithms should be written in simple Typescript, as functions.

Example pseudocode algorithm:

```typescript
function startRound(round) {
  round_p = round
  step_p = PROPOSE
  if (proposer(h_p, round_p) === p) {
    if (validValue_p !== nil)
      proposal = validValue_p
    else
      proposal = getValue()
    broadcast( {PROPOSAL, h_p, round_p, proposal, validRound} )
  } else
    schedule(onTimeoutPropose(h_p, round_p), timeoutPropose(round_p))
}
```

## History

This specification was significantly inspired by and derived from Ethereum's [EIP 1](https://github.com/ethereum/EIPs/blob/master/EIPS/eip-1.md), which
was in turn derived from Bitcoin's BIP process and Python's PEP process. Antecedent authors are not responsible for any shortcomings of this ICS spec or
the ICS process. Please direct all comments to the ICS repository maintainers.

Mar 4, 2019 - Initial draft finished and submitted as a PR

Mar 7, 2019 - Draft merged

Apr 11, 2019 - Updates to pseudocode formatting, add definitions subsection

Aug 17, 2019 - Clarifications to categories

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
