## Why IBC?

The design space of "interblockchain communication protocols" is wide, and the term itself has become a bit too all-encompassing. The "Interblockchain Communication Protocol" (IBC) is a very particular point in that design space, chosen to provide specific versatility, locality, modularity, and efficiency properties for the expected interchain ecosystem of interoperable blockchains. This document outlines the "why" of IBC and enumerates the primary high-level design goals.

### Versatility

IBC is designed to be a *versatile* protocol. The protocol supports *heterogeneous* blockchains whose state machines implement different semantics in different languages. Applications written on top of IBC can be *composed* together, and IBC protocol steps themselves can be *automated*.

#### Heterogeneity

IBC can be implemented by any consensus algorithm and state machine with a basic set of requirements (fast finality & accumulator proofs). The protocol handles data authentication, transport, and ordering — common requirements of any multi-chain application — but is agnostic to the semantics of the application itself. Heterogeneous chains connected over IBC must understand a compatible application-layer "interface" (such as for transferring tokens), but once across the IBC interface handler, the state machines can support arbitrary bespoke functionality (such as shielded transactions).

#### Composability

Applications written on top of IBC can be composed together.

- Common interfaces
- Common standard

#### Automatability

The "users", or "actors", in IBC — who initiate connections, create channels, send packets, report Byzantine fraud, etc. — may be but need not be human. Modules, smart contracts, and automated off-chain processes can make use of the protocol (subject to e.g. gas costs to charge for computation) and take actions on their own or in concert. Complex interactions across multiple chains (such as the three-step connection opening handshake or multi-hop token transfers) are designed such that all but the single initiating action can be abstracted away from the user. Eventually, it may be possible to automatically spin up a new blockchain (modulo physical infrastructure provisioning), start IBC connections, and make use of the new chain's state machine & throughput entirely automatically.

### Modularity

IBC is designed to be a *modular* protocol. The protocol is constructed as a series of layered components with explicit security properties & requirements. Implementations of a component at a particular layer can vary (such as a different consensus algorithm or connection opening procedure) as long as they provide the requisite properties to the higher layers (such as finality, < 1/3 Byzantine safety, or embedded roots-of-trust on two chains). State machines need only understand compatible subsets of the IBC protocol (e.g. lite client verification algorithms for each other's consensus) in order to safely interact.

### Locality

- Assumptions are designed to be informationally "local"
- Chains must only understand state of chains to which they are connected
- No necessary single root chain, dynamic network topology, no protocol-level scaling limitations
- Reflects local nature of underlying global commerce (frequency of transactions falls off over distance)

#### Locality of communication & information

- No global topology view required
- Core protocol can be reasoned about as a construction between two chains, routing built on top
- Users and chains can reason about security guarantees given what they know and trust

#### Locality of trust & security

- Users of IBC choose which consensus algorithms & validator sets they trust
- Never exposed to risk of asset inflation, application-level invariant violations due to Byzantine behavior from validator sets they didn't decide to trust
- Contained risks in large network topology of interconnected blockchains, IBC connections can track metadata (e.g. total supply flow through a connection) and limit risk

#### Locality of permissioning

- Connections can be opened permissionlessly between blockchains (particulars dependent on state machine, but e.g. with smart contracts contracts could open connections)
- Users must inspect state & consensus of connection and decide whether safe

#### Topological agnosticism

IBC makes no assumptions, and relies upon no characteristics, of the topological structure of the network of blockchains in which it is operating.

- Private chains
- Public chains

### Efficiency

- Amortized cost should mostly be the cost of the underlying state transitions or operations associated with packets
- Consensus transcript (header) update cost should scale with consensus speed regardless of packet throughput
