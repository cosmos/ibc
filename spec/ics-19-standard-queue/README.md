---
ics: 19
title: Standard Queue
stage: proposal
category: ibc-misc
requires: 23, 24
author: Christopher Goes <cwgoes@tendermint.com>, Juwoon Yun <joon@tendermint.com>
created: 2019-03-07
modified: 2019-05-05
---

## Synopsis

Queues are used to enforce and prove ordering of packets within IBC channels.

## Specification

### Motivation

- ordering things, proving that things were ordered

### Desired Properties

- proofs of elements at positions in queue
- proofs of current position in queue (e.g. things processed)

### Technical Specification

- note specific key prefix for queue elements
- note specific key prefix for queue position
- note usage in commitment proofs

To implement strict message ordering, we introduce an ordered *queue*. A queue can be conceptualized as a slice of an infinite array. Two numerical indices - `q_head` and `q_tail` - bound the slice, such that for every `index` where `q_head <= index < q_tail`, there is a queue element `q[index]`. Elements can be appended to the tail (end) and removed from the head (beginning). We introduce one further method, `advance`, to facilitate efficient queue cleanup.

Each IBC-supporting blockchain must provide a queue abstraction with the following functionality:

- do we actually need all of these functions?
- also we should just provide implementation using `Get` / `Set`, this isn't a primitive that needs to be defined (yet)

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

Not applicable.

### Forwards Compatibility

Channel versioning can upgrade the queue structure and keyspaces.

### Example Implementation

Coming soon.

### Other Implementations

Coming soon.

## History

5 May 2019 - Draft submitted

## Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
