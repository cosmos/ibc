---
ics: 19
title: Standard Queue
stage: proposal
category: ibc-misc
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-03-07
---

## Synopsis

(high-level description of and rationale for specification)

## Specification

(main part of standard document - not all subsections are required)

### Motivation

(rationale for existence of standard)

### Desired Properties

(desired characteristics / properties of protocol, effects if properties are violated)

### Technical Specification

(detailed technical specification: syntax, semantics, sub-protocols, algorithms, data structures, etc)

To implement strict message ordering, we introduce an ordered *queue*. A queue can be conceptualized as a slice of an infinite array. Two numerical indices - `q_head` and `q_tail` - bound the slice, such that for every `index` where `q_head <= index < q_tail`, there is a queue element `q[index]`. Elements can be appended to the tail (end) and removed from the head (beginning). We introduce one further method, `advance`, to facilitate efficient queue cleanup.

Each IBC-supporting blockchain must provide a queue abstraction with the following functionality:

`init`

```
set q_head = 0 
set q_tail = 0 
```

`peek ⇒ e`

```
match q_head == q_tail with
  true ⇒ return nil 
  false ⇒ 
    return q[q_head]
```

`pop ⇒ e`

```
match q_head == q_tail with   
  true ⇒ return nil 
  false ⇒ 
    set q_head = q_head + 1 
    return q_head - 1   
```

`retrieve(i) ⇒ e`

```
match q_head <= i < q_tail with
  true ⇒ return q[i]
  false ⇒ return nil 
```

`push(e)`

```
set q[q_tail] = e 
set q_tail = q_tail + 1 
```

`advance(i)`

```
set q_head = i 
set q_tail = max(q_tail, i)
```
  
`head ⇒ i`

```
return q_head
```

`tail ⇒ i`

```
return q_tail
```

### Backwards Compatibility

(discussion of compatibility or lack thereof with previous standards)

### Forwards Compatibility

(discussion of compatibility or lack thereof with expected future standards)

### Example Implementation

(link to or description of concrete example implementation)

### Other Implementations

(links to or descriptions of other implementations)

## History

(changelog and notable inspirations / references)

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
