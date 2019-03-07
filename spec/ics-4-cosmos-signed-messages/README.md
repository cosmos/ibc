---
ics: 4
title: Cosmos Signed Messages
stage: draft
category: misc
author: Aleksandr Bezobchuk <alex@tendermint.com>
created: 2019-03-07
modified: 2019-03-07
---

## Synopsis

Having the ability to sign messages off-chain has proven to be a fundamental aspect
of nearly any blockchain. The notion of signing messages off-chain has many
added benefits such as saving on computational costs and reducing transaction
throughput and overhead. Within the context of the Cosmos, some of the major
applications of signing such data includes, but is not limited to, providing a
cryptographic secure and verifiable means of proving validator identity and
possibly associating it with some other framework or organization. In addition,
having the ability to sign Cosmos messages with a Ledger or similar HSM device.

A standardized protocol for hashing, signing, and verifying messages that can be
implemented by the Cosmos SDK and other third-party organizations is needed.

## Specification

### Desired Properties

The Cosmos signed messages standardized protocol subscribes to the following:

* Use of a secure cryptographic hash function
* Hash and sign over human-readable and machine-parsable messages
* Is invulnerable to chosen ciphertext attacks
* Allow for signing over structured data
* Contains a framework for deterministic and injective encoding of structured data
* Have builtin framework and support for domain separation and replay protection
* Has protection against potentially signing transactions a user did not intend to

### Technical Specification

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

### Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

### Example Implementation

(link to or description of concrete example implementation)

## History

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).