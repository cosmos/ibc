## Why IBC?

The design space of "interblockchain communication protocols" is wide, and the term itself has become a bit too all-encompassing. The "Interblockchain Communication Protocol" (IBC) is a very particular point in that design space, chosen to provide specific security, composability, and modularity properties for the expected interchain ecosystem of interoperable blockchains. This document outlines the "why" of IBC and enumerates the primary high-level design goals.

### Heterogeneity

- Supports heterogeneous chains with different state machines
- Once across the IBC interface barrier, state machines can do whatever they want (e.g. shield tokens)

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

### Modularity

- Protocol separable into layered components with explicit security properties
- Component implementations can vary (e.g. different consensus) as long as they provide the requisite properties (finality, <1/3 Byzantine safety)
- State machines must only understand compatible subsets of the IBC protocol to safely interact

### Automatability

- IBC "users" need not be human, can be smart contracts or modules on the chains themselves
- Complex interactions across multiple chains must eventually be abstracted away from the user
- Eventually, may be possible to automatically spin up chains (modulo physical infrastructure provisioning) and start IBC connections

### Efficiency

- Amortized cost should mostly be the cost of the underlying state transitions or operations associated with packets
- Consensus transcript (header) update cost should scale with consensus speed regardless of packet throughput
