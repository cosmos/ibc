## Scope

IBC handles authentication, transport, and ordering of opaque data packets relayed between modules on separate ledgers — ledgers can be run on solo machines, replicated by many nodes running a consensus algorithm, or constructed by any process whose state can be verified. The protocol is defined between modules on two ledgers, but designed for safe simultaneous use between any number of modules on any number of ledgers connected in arbitrary topologies.

## Interfaces

IBC sits between modules — smart contracts, other ledger components, or otherwise independently executed pieces of application logic on ledgers — on one side, and underlying consensus protocols, blockchains, and network infrastructure (e.g. TCP/IP), on the other side.

IBC provides to modules a set of functions much like the functions which might be provided to a module for interacting with another module on the same ledger: sending data packets and receiving data packets on an established connection and channel, in addition to calls to manage the protocol state: opening and closing connections and channels, choosing connection, channel, and packet delivery options, and inspecting connection and channel status.

IBC requires certain functionalities and properties of the underlying ledgers, primarily finality (or thresholding finality gadgets), cheaply-verifiable consensus transcripts (such that a light client algorithm can verify the results of the consensus process with much less computation & storage than a full node), and simple key/value store functionality. On the network side, IBC requires only eventual data delivery — no authentication, synchrony, or ordering properties are assumed.

## Operation

The primary purpose of IBC is to provide reliable, authenticated, ordered communication between modules running on independent host ledgers. This requires protocol logic in the areas of data relay, data confidentiality and legibility, reliability, flow control, authentication, statefulness, and multiplexing.

\vspace{3mm}

### Data relay

\vspace{3mm}

In the IBC architecture, modules are not directly sending messages to each other over networking infrastructure, but rather are creating messages to be sent which are then physically relayed from one ledger to another by monitoring "relayer processes". IBC assumes the existence of a set of relayer processes with access to an underlying network protocol stack (likely TCP/IP, UDP/IP, or QUIC/IP) and physical interconnect infrastructure. These relayer processes monitor a set of ledgers implementing the IBC protocol, continuously scanning the state of each ledger and requesting transaction execution on another ledger when outgoing packets have been committed. For correct operation and progress in a connection between two ledgers, IBC requires only that at least one correct and live relayer process exists which can relay between the ledgers.

\vspace{3mm}

### Data confidentiality and legibility

\vspace{3mm}

The IBC protocol requires only that the minimum data necessary for correct operation of the IBC protocol be made available and legible (serialised in a standardised format) to relayer processes, and the ledger may elect to make that data available only to specific relayers. This data consists of consensus state, client, connection, channel, and packet information, and any auxiliary state structure necessary to construct proofs of inclusion or exclusion of particular key/value pairs in state. All data which must be proved to another ledger must also be legible; i.e., it must be serialised in a standardised format agreed upon by the two ledgers.

\vspace{3mm}

### Reliability

\vspace{3mm}

The network layer and relayer processes may behave in arbitrary ways, dropping, reordering, or duplicating packets, purposely attempting to send invalid transactions, or otherwise acting in a Byzantine fashion, without compromising the safety or liveness of IBC. This is achieved by assigning a sequence number to each packet sent over an IBC channel, which is checked by the IBC handler (the part of the ledger implementing the IBC protocol) on the receiving ledger, and providing a method for the sending ledger to check that the receiving ledger has in fact received and handled a packet before sending more packets or taking further action. Cryptographic commitments are used to prevent datagram forgery: the sending ledger commits to outgoing packets, and the receiving ledger checks these commitments, so datagrams altered in transit by a relayer will be rejected. IBC also supports unordered channels, which do not enforce ordering of packet receives relative to sends but still enforce exactly-once delivery.

\vspace{3mm}

### Flow control

\vspace{3mm}

IBC does not provide specific protocol-level provisions for compute-level or economic-level flow control. The underlying ledgers are expected to have compute throughput limiting devices and flow control mechanisms of their own such as gas markets. Application-level economic flow control — limiting the rate of particular packets according to their content — may be useful to ensure security properties and contain damage from Byzantine faults. For example, an application transferring value over an IBC channel might want to limit the rate of value transfer per block to limit damage from potential Byzantine behaviour. IBC provides facilities for modules to reject packets and leaves particulars up to the higher-level application protocols.

\vspace{3mm}

### Authentication

\vspace{3mm}

All data sent over IBC are authenticated: a block finalised by the consensus algorithm of the sending ledger must commit to the outgoing packet via a cryptographic commitment, and the receiving ledger's IBC handler must verify both the consensus transcript and the cryptographic commitment proof that the datagram was sent before acting upon it.

\vspace{3mm}

### Statefulness

\vspace{3mm}

Reliability, flow control, and authentication as described above require that IBC initialises and maintains certain status information for each datastream. This information is split between three abstractions: clients, connections, and channels. Each client object contains information about the consensus state of the counterparty ledger. Each connection object contains a specific pair of named identifiers agreed to by both ledgers in a handshake protocol, which uniquely identifies a connection between the two ledgers. Each channel, specific to a pair of modules, contains information concerning negotiated encoding and multiplexing options and state and sequence numbers. When two modules wish to communicate, they must locate an existing connection and channel between their two ledgers, or initialise a new connection and channel(s) if none yet exist. Initialising connections and channels requires a multi-step handshake which, once complete, ensures that only the two intended ledgers are connected, in the case of connections, and ensures that two modules are connected and that future datagrams relayed will be authenticated, encoded, and sequenced as desired, in the case of channels.

\vspace{3mm}

### Multiplexing

\vspace{3mm}

To allow for many modules within a single host ledger to use an IBC connection simultaneously, IBC allows any number of channels to be associated with a single connection. Each channel uniquely identifies a datastream over which packets can be sent in order (in the case of an ordered channel), and always exactly once, to a destination module on the receiving ledger. Channels are usually expected to be associated with a single module on each ledger, but one-to-many and many-to-one channels are also possible. The number of channels per connection is unbounded, facilitating concurrent throughput limited only by the throughput of the underlying ledgers with only a single connection and pair of clients necessary to track consensus information (and consensus transcript verification cost thus amortised across all channels using the connection).
