## Why IBC?

The design space of "interblockchain communication protocols" is wide, and the term itself has become a bit too all-encompassing. The "Interblockchain Communication Protocol" (IBC) is a very particular point in that design space, chosen to provide specific versatility, locality, modularity, and efficiency properties for the expected interchain ecosystem of interoperable blockchains. This document outlines the "why" of IBC and enumerates the primary high-level design goals.

### Versatility

IBC is designed to be a *versatile* protocol. The protocol supports *heterogeneous* blockchains whose state machines implement different semantics in different languages. Applications written on top of IBC can be *composed* together, and IBC protocol steps themselves can be *automated*.

#### Heterogeneity

IBC can be implemented by any consensus algorithm and state machine with a basic set of requirements (fast finality & accumulator proofs). The protocol handles data authentication, transport, and ordering — common requirements of any multi-chain application — but is agnostic to the semantics of the application itself. Heterogeneous chains connected over IBC must understand a compatible application-layer "interface" (such as for transferring tokens), but once across the IBC interface handler, the state machines can support arbitrary bespoke functionality (such as shielded transactions).

#### Composability

Applications written on top of IBC can be composed together by both protocol developers and users. IBC defines a set of primitives for authentication, transport, and ordering, and a set of application-layer standards for asset & data semantics. Chains which support compatible standards can be connected together and transacted between by any user who elects to open a connection (or reuse a connection), and assets & data can be relayed across multiple chains both automatically ("multi-hop") and manually (by sending several IBC relay transactions in sequence).

#### Automatability

The "users", or "actors", in IBC — who initiate connections, create channels, send packets, report Byzantine fraud, etc. — may be but need not be human. Modules, smart contracts, and automated off-chain processes can make use of the protocol (subject to e.g. gas costs to charge for computation) and take actions on their own or in concert. Complex interactions across multiple chains (such as the three-step connection opening handshake or multi-hop token transfers) are designed such that all but the single initiating action can be abstracted away from the user. Eventually, it may be possible to automatically spin up a new blockchain (modulo physical infrastructure provisioning), start IBC connections, and make use of the new chain's state machine & throughput entirely automatically.

### Modularity

IBC is designed to be a *modular* protocol. The protocol is constructed as a series of layered components with explicit security properties & requirements. Implementations of a component at a particular layer can vary (such as a different consensus algorithm or connection opening procedure) as long as they provide the requisite properties to the higher layers (such as finality, < 1/3 Byzantine safety, or embedded roots-of-trust on two chains). State machines need only understand compatible subsets of the IBC protocol (e.g. lite client verification algorithms for each other's consensus) in order to safely interact.

### Locality

IBC is designed to be a *local* protocol, meaning that only information about the two connected chains is necessary to reason about the security and correctness of a bidirectional IBC connection. Security requirements of the authentication primitives refer only to consensus algorithms and validator sets of the blockchains involved in the connection, and blockchains maintaining a set of IBC connections need only understand the state of the chains to which they are connected (no matter which other chains those chains are connected to). 

#### Locality of communication & information

IBC makes no assumptions, and relies upon no characteristics, of the topological structure of the network of blockchains in which it is operating. No view of the global network-of-blockchains topology is required: Security & correctness can be reasoned about at the level of a single connection between two chains, and by compositionn reasoned about for subgraphs in the network topology. Users and chains can reason about their assumptions and risks given information about only part of the network graph of blockchains they know and trust (to variable degrees). There is no necessary "root chain" in IBC — some subgraphs of the global network may evolve into a hub-spoke structure, others may remain tightly connected, others still may take on more exotic topologies.

#### Locality of trust & security

Users of IBC — at the blockchain level and at the human or smart contract level — choose which consensus algorithms, state machines, and validator sets they "trust" (to behave in a particular way, e.g. < 1/3 Byzantine) and in which ways they trust them. Assuming the IBC protocol is implemented correctly, users are never exposed to risks of application-level invariant violations (such as asset inflation) due to Byzantine behavior or faulty state machines transitions committed by validator sets or blockchains they did not explicitly decide to trust. This is particulary important in the expected large network topology of interconnected blockchains, where some number of blockchains and validator sets can be expected to be Byzantine occaisionally — IBC, implemented conservatively, bounds the risk and limits the possible damage incurred.

#### Locality of permissioning

Actions in IBC — such as opening a connection, creating a channel, or sending a packet — are permissioned locally by the state machines and actors involved in a particular connection between two chains. Individual chains could choose to require approval from a permissioning mechanism (such as governance) for specific application-layer actions (such as delegated-security slashing), but for the base protocol, actions are permissionless (modulo gas & storage costs) — by default, connections can be opened, channels created, and packets sent without any approval process. Of course, users themselves must inspect the state & consensus of each IBC connection and decide whether it is safe to used (based e.g. on the roots-of-trust stored).

### Efficiency

IBC is designed to be an *efficient* protocol: the amortized cost of interchain data & asset relay should be mostly comprised of the cost of the underlying state transitions or operations associated with packets (such as transferring tokens), plus some small constant overhead.
