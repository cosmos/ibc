---
ics: 26
title: Relayer Module
stage: Draft
category: ibc-core
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-06-09
modified: 2019-06-09
---

## Synopsis

(high-level description of and rationale for specification)

### Motivation

(rationale for existence of standard)

### Definitions

(definitions of any new terms not defined in common documentation)

### Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

## Technical Specification

(main part of standard document - not all subsections are required)

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

### Datagrams

#### Connection lifecycle management

```typescript
interface ConnOpenInit {
  identifier: Identifier
  desiredCounterpartyIdentifier: Identifier
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnOpenTry {
  desiredIdentifier: Identifier
  counterpartyIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  clientIdentifier: Identifier
  proofInit: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnOpenAck {
  identifier: Identifier
  proofTry: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnOpenConfirm {
  identifier: Identifier
  proofAck: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
interface ConnOpenTimeout {
  identifier: Identifier
  proofTimeout: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
interface ConnCloseInit {
  identifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnCloseTry {
  identifier: Identifier
  proofInit: CommitmentProof
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
}
```

```typescript
interface ConnCloseAck {
  identifier: Identifier
  proofTry: CommitmentProof
  timeoutHeight: uint64
}
```

```typescript
interface ConnCloseTimeout {
  identifier: Identifier
  proofTimeout: CommitmentProof
  timeoutHeight: uint64
}
```

#### Channel lifecycle management

```typescript
interface ChanOpenInit {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ChanOpenTry {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  moduleIdentifier: Identifier
  counterpartyModuleIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofInit: CommitmentProof
}
```

```typescript
interface ChanOpenAck {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  nextTimeoutHeight: uint64
  proofTry: CommitmentProof
}
```

```typescript
interface ChanOpenConfirm {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofAck: CommitmentProof
}
```

```typescript
interface ChanOpenTimeout {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTimeout: CommitmentProof
}
```

```typescript
interface ChanCloseInit {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  nextTimeoutHeight: uint64
}
```

```typescript
interface ChanCloseAck {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTry: CommitmentProof
}
```

```typescript
interface ChanCloseTimeout {
  connectionIdentifier: Identifier
  channelIdentifier: Identifier
  timeoutHeight: uint64
  proofTimeout: CommitmentProof
}
```


### Data Structures

(new data structures, if applicable)

### Subprotocols

(subprotocols, if applicable)

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

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
