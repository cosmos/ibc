---
ics: 3
title: IBC Connection Semantics
status: Proposal
category: IBC
author: Juwoon Yun <joon@tendermint.com>
created: 2019-02-25
---

## Abstract

`Block` is $(p \in \mathbb{B}, f \in \mathbb{B} \rightarrow \{true, false\}, s \in \mathbb{S}, u \in mathbb{N})$ where $p$ is parent block, $f$ is lightclient verifier, $s$ is state and $u$ is unbonding period.

Definitions:

1. $\forall b=(\_, f, \_, \_), b'=(p', \_, \_, \_) \in \mathbb{B} : f(b') \land b=p' \iff child(b, b')$
If a block verifies another block and the other block has the block as its parent, the other block is the child of the block.

2. $\forall b, b', b'' \in \mathbb{B} : child(b, b'') \lor child(b, b') \land descendant(b', b'') \iff descendant(b, b'')$
Descendants are either direct child or descendants of direct child

3. $\forall b, b'=(p', \_, \_, \_), b''=(p'', \_, \_, \_) \in \mathbb{B} : b=p'' \lor b=p' \land ancestor(p', p'') \iff ancestor(b, b'')$
Ancestors are either direct parent or ancestors of direct parent

`Block` satisfies the followings:

1. $\forall b=(\_, f, \_, \_) \in \mathbb{B} : \exists b'=(p', \_, \_, \_) \in \mathbb{B} : child(b, b')
Blocks have one direct child

2. $\forall b=(\_, f, \_, \_), b'=(p', \_, \_, \_), b''=(p'', \_, \_, \_) \in \mathbb{B} : child(b, b') \land child(b, b'') \rightarrow b'=b''$
Blocks have only one direct child

3. $\forall b=(\_, f, \_, \_), b' \in \mathbb{B} : f(b') \rightarrow descendant(b, b')$
If a block verifies another block then it is a descendant of the block

// TODO: is it needed?
// This implies that the the height difference between 
4. $\forall b=(\_, f, \_, \_), b', b'' \in \mathbb{B} : \lnot f(b') \land descendant(b
', b'') \rightarrow \lnot f(b'')$
Blocks cannot validate descentants from a block that they cannot validate

// TODO: unbonding period is a specific term for BPoS, change it to something like max skip height?
5. $\forall b=(\_, f, \_, u), b' \in \mathbb{B} : diff(b, b') >= u \rightarrow \lnot f(b') $
Blocks cannot validate the block after its unbonding period

When a blockchain satisfies deterministic safety(no more than one child block for all block) and liveness(no less than one child block for all block), All of the above are satisfied. When the chain violates this assumption(byzantine behaviour / consensus failure), some of the above are no longer satisfied, so the connection must be halt.
 
`State` is $(c \in \mathbf{chainid} \rightarrow \mathbb{C})$. $c$ is a map from `ChainID`s to connections and $p$ is a map from `PortID`s to ports. For sake of simplicity, we omit every logic excepts those are directly associated with the connection & channel.

// TODO: extend $b \in mathbb{B}$ to $b \in mathbb{B}^+$ so we can access on the previous versions
`Connection` is $(b \in \mathbb{B}, q \in \mathbb{portid} \rightarrow (\mathbf{channel} + null))$ where $b$ is the last known header from the other chain and $q$ is a map from `PortID` to `Channel`. 

Definitions:

1. 
Connection can be updated by inserting valid descendent of the current referring block
// TODO: change "the current referring block" to "the latest block stored in the connection which is an ancestor of the new block" 

`Connection` satisfies the followings

1. 
Connection can be updated only if
Connection cannot refer a block from the other chain which is referring a desecendant of its block
// TODO: 

2. 
Connection can only refer the same block or a descendant that the connections from the ancestors of its block are referring
