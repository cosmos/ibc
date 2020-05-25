## Scope

IBC handles authentication, transport, and ordering of structured data packets relayed between modules on separate machines. The protocol is defined between modules on two machines, but designed for safe simultaneous use between any number of modules on any number of machines connected in arbitrary topologies.

## Interfaces

IBC sits between modules — smart contracts, other state machine components, or otherwise independent pieces of application logic on state machines — on one side, and underlying consensus protocols, machines, and network infrastructure (e.g. TCP/IP), on the other side.

IBC provides to modules a set of functions much like the functions which might be provided to a module for interacting with another module on the same state machine: sending data packets and receiving data packets on an established connection & channel (primitives for authentication & ordering, see [definitions](./1_IBC_TERMINOLOGY.md)) — in addition to calls to manage the protocol state: opening and closing connections and channels, choosing connection, channel, and packet delivery options, and inspecting connection & channel status.

IBC assumes functionalities and properties of the underlying consensus protocols and machines as defined in [ICS 2](../../spec/ics-002-client-semantics), primarily finality (or thresholding finality gadgets), cheaply-verifiable consensus transcripts, and simple key/value store functionality. On the network side, IBC requires only eventual data delivery — no authentication, synchrony, or ordering properties are assumed (these properties are defined precisely later on).

## Operation

The primary purpose of IBC is to provide reliable, authenticated, ordered communication between modules running on independent host machines. This requires protocol logic in the following areas:

- Data relay
- Data confidentiality & legibility
- Reliability
- Flow control
- Authentication
- Statefulness
- Multiplexing
- Serialisation

The following paragraphs outline the protocol logic within IBC for each area.

### Data relay

In the IBC architecture, modules are not directly sending messages to each other over networking infrastructure, but rather creating messages to be sent which are then physically relayed by monitoring "relayer processes". IBC assumes the existence of a set of relayer processes with access to an underlying network protocol stack (likely TCP/IP, UDP/IP, or QUIC/IP) and physical interconnect infrastructure. These relayer processes monitor a set of machines implementing the IBC protocol, continuously scanning the state of each machine and executing transactions on another machine when outgoing packets have been committed. For correct operation and progress in a connection between two machines, IBC requires only that at least one correct and live relayer process exists which can relay between the machines.

### Data confidentiality & legibility

The IBC protocol requires only that the minimum data necessary for correct operation of the IBC protocol be made available & legible (serialised in a standardised format), and the state machine may elect to make that data available only to specific relayers (though the details thereof are out-of-scope of this specification). This data consists of consensus state, client, connection, channel, and packet information, and any auxiliary state structure necessary to construct proofs of inclusion or exclusion of particular key/value pairs in state. All data which must be proved to another machine must also be legible; i.e., it must be serialised in a format defined by this specification.

### Reliability

The network layer and relayer processes may behave in arbitrary ways, dropping, reordering, or duplicating packets, purposely attempting to send invalid transactions, or otherwise acting in a Byzantine fashion. This must not compromise the safety or liveness of IBC. This is achieved by assigning a sequence number to each packet sent over an IBC connection (at the time of send), which is checked by the IBC handler (the part of the state machine implementing the IBC protocol) on the receiving machine, and providing a method for the sending machine to check that the receiving machine has in fact received and handled a packet before sending more packets or taking further action. Cryptographic commitments are used to prevent datagram forgery: the sending machine commits to outgoing packets, and the receiving machine checks these commitments, so datagrams altered in transit by a relayer will be rejected. IBC also supports unordered channels, which do not enforce ordering of packet receives relative to sends but still enforce exactly-once delivery.

### Flow control

IBC does not provide specific provisions for compute-level or economic-level flow control. The underlying machines will have compute throughput limitations and flow control mechanisms of their own (such as "gas" markets). Application-level economic flow control — limiting the rate of particular packets according to their content — may be useful to ensure security properties (limiting the value on a single machine) and contain damage from Byzantine faults (allowing a challenge period to prove an equivocation, then closing a connection). For example, an application transferring value over an IBC channel might want to limit the rate of value transfer per block to limit damage from potential Byzantine behaviour. IBC provides facilities for modules to reject packets and leaves particulars up to the higher-level application protocols.

### Authentication

All datagrams in IBC are authenticated: a block finalised by the consensus algorithm of the sending machine must commit to the outgoing packet via a cryptographic commitment, and the receiving chain's IBC handler must verify both the consensus transcript and the cryptographic commitment proof that the datagram was sent before acting upon it.

### Statefulness

Reliability, flow control, and authentication as described above require that IBC initialises and maintains certain status information for each datastream. This information is split between two abstractions: connections & channels. Each connection object contains information about the consensus state of the connected machine. Each channel, specific to a pair of modules, contains information concerning negotiated encoding & multiplexing options and state & sequence numbers. When two modules wish to communicate, they must locate an existing connection & channel between their two machines, or initialise a new connection & channels if none yet exists. Initialising connections & channels requires a multi-step handshake which, once complete, ensures that only the two intended machines are connected, in the case of connections, and ensures that two modules are connected and that future datagrams relayed will be authenticated, encoded, and sequenced as desired, in the case of channels.

### Multiplexing

To allow for many modules within a single host machine to use an IBC connection simultaneously, IBC provides a set of channels within each connection, which each uniquely identify a datastream over which packets can be sent in order (in the case of an ordered module), and always exactly once, to a destination module on the receiving machine. Channels are usually expected to be associated with a single module on each machine, but one-to-many and many-to-one channels are also possible. The number of channels is unbounded, facilitating concurrent throughput limited only by the throughput of the underlying machines with only a single connection necessary to track consensus information (and consensus transcript verification cost thus amortised across all channels using the connection).

### Serialisation

IBC serves as the interface boundary between otherwise mutually incomprehensible machines, and must provide the requisite mutual comprehensibility of the minimal set of data structure encodings & datagram formats in order to allow two machines which both correctly implement the protocol to understand each other. For this purpose, the IBC specification defines
canonical encodings of data structures which must be serialised and relayed or checked in proofs between two machines talking over IBC, provided in proto3 format in this repository.

## Dataflow

IBC can be conceptualised as a layered protocol stack, through which data flows top-to-bottom (when sending IBC packets) and bottom-to-top (when receiving IBC packets).

The "handler" is the part of the state machine implementing the IBC protocol, which is responsible for translating calls from modules to and from packets and routing them appropriately to and from channels & connections.

Consider the path of an IBC packet between two chains — call them *A* and *B*:

### Diagram

```
+---------------------------------------------------------------------------------------------+
| Distributed Ledger A                                                                        |
|                                                                                             |
| +----------+     +----------------------------------------------------------+               |
| |          |     | IBC Module                                               |               |
| | Module A | --> |                                                          | --> Consensus |
| |          |     | Handler --> Packet --> Channel --> Connection --> Client |               |
| +----------+     +----------------------------------------------------------+               |
+---------------------------------------------------------------------------------------------+

    +---------+
==> | Relayer | ==>
    +---------+

+--------------------------------------------------------------------------------------------+
| Distributed Ledger B                                                                       |
|                                                                                            |
|               +---------------------------------------------------------+     +----------+ |
|               | IBC Module                                              |     |          | |
| Consensus --> |                                                         | --> | Module B | |
|               | Client -> Connection --> Channel --> Packet --> Handler |     |          | |
|               +---------------------------------------------------------+     +----------+ |
+--------------------------------------------------------------------------------------------+
```


