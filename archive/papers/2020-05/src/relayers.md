Relayer algorithms are the "physical" connection layer of IBC — off-ledger processes responsible for relaying data between two ledgers running the IBC protocol by scanning the state of each ledger, constructing appropriate datagrams, and executing them on the opposite ledger as allowed by the protocol.

\vspace{3mm}

### Motivation

\vspace{3mm}

In the IBC protocol, one ledger can only record the intention to send particular data to another ledger — it does not have direct access to a network transport layer. Physical datagram relay must be performed by off-ledger infrastructure with access to a transport layer such as TCP/IP. This standard defines the concept of a *relayer* algorithm, executable by an off-ledger process with the ability to query ledger state, to perform this relay. 

A *relayer* is an off-ledger process with the ability to read the state of and submit transactions to some set of ledgers utilising the IBC protocol.

\vspace{3mm}

### Properties

\vspace{3mm}

- No exactly-once or deliver-or-timeout safety properties of IBC depend on relayer behaviour (Byzantine relayers are assumed)
- Packet relay liveness properties of IBC depend only on the existence of at least one correct, live relayer
- Relaying can safely be permissionless, all requisite verification is performed by the ledger itself
- Requisite communication between the IBC user and the relayer is minimised
- Provision for relayer incentivisation are not included in the core protocol, but are possible at the application layer

\vspace{3mm}

### Basic relayer algorithm

\vspace{3mm}

The relayer algorithm is defined over a set of ledgers implementing the IBC protocol. Each relayer may not necessarily have access to read state from and write datagrams to all ledgers in the multi-ledger network (especially in the case of permissioned or private ledgers) — different relayers may relay between different subsets.

Every so often, although no more frequently than once per block on either ledger, a relayer calculates the set of all valid datagrams to be relayed from one ledger to another based on the state of both ledgers. The relayer must possess prior knowledge of what subset of the IBC protocol is implemented by the ledgers in the set for which they are relaying (e.g. by reading the source code). Datagrams can be submitted individually as single transactions or atomically as a single transaction if the ledger supports it. 

Different relayers may relay between different ledgers — as long as each pair of ledgers has at least one correct and live relayer and the ledgers remain live, all packets flowing between ledgers in the network will eventually be relayed.

\vspace{3mm}

### Packets, acknowledgements, timeouts

\vspace{3mm}

#### Relaying packets in an ordered channel

Packets in an ordered channel can be relayed in either an event-based fashion or a query-based fashion.
For the former, the relayer should watch the source ledger for events emitted whenever packets are sent,
then compose the packet using the data in the event log. For the latter, the relayer should periodically
query the send sequence on the source ledger, and keep the last sequence number relayed, so that any sequences
in between the two are packets that need to be queried and then relayed. In either case, subsequently, the relayer process
should check that the destination ledger has not yet received the packet by checking the receive sequence, and then relay it.

\vspace{3mm}

#### Relaying packets in an unordered channel

Packets in an unordered channel can most easily be relayed in an event-based fashion.
The relayer should watch the source ledger for events emitted whenever packets
are send, then compose the packet using the data in the event log. Subsequently,
the relayer should check whether the destination ledger has received the packet
already by querying for the presence of an acknowledgement at the packet's sequence
number, and if one is not yet present the relayer should relay the packet.

\vspace{3mm}

#### Relaying acknowledgements

Acknowledgements can most easily be relayed in an event-based fashion. The relayer should
watch the destination ledger for events emitted whenever packets are received and acknowledgements
are written, then compose the acknowledgement using the data in the event log,
check whether the packet commitment still exists on the source ledger (it will be
deleted once the acknowledgement is relayed), and if so relay the acknowledgement to
the source ledger.

\vspace{3mm}

#### Relaying timeouts

Timeout relay is slightly more complex since there is no specific event emitted when
a packet times-out — it is simply the case that the packet can no longer be relayed,
since the timeout height or timestamp has passed on the destination ledger. The relayer
process must elect to track a set of packets (which can be constructed by scanning event logs),
and as soon as the height or timestamp of the destination ledger exceeds that of a tracked
packet, check whether the packet commitment still exists on the source ledger (it will
be deleted once the timeout is relayed), and if so relay a timeout to the source ledger.

\vspace{3mm}

#### Ordering constraints

There are implicit ordering constraints imposed on the relayer process determining which datagrams must be submitted in what order. For example, a header must be submitted to finalise the stored consensus state and commitment root for a particular height in a light client before a packet can be relayed. The relayer process is responsible for frequently querying the state of the ledgers between which they are relaying in order to determine what must be relayed when.

\vspace{3mm}

#### Bundling

If the host ledger supports it, the relayer process can bundle many datagrams into a single transaction, which will cause them to be executed in sequence, and amortise any overhead costs (e.g. signature checks for fee payment).

\vspace{3mm}

#### Race conditions

Multiple relayers relaying between the same pair of modules and ledgers may attempt to relay the same packet (or submit the same header) at the same time. If two relayers do so, the first transaction will succeed and the second will fail. Out-of-band coordination between the relayers or between the actors who sent the original packets and the relayers is necessary to mitigate this.

\vspace{3mm}

#### Incentivisation

The relay process must have access to accounts on both ledgers with sufficient balance to pay for transaction fees. Relayers may employ application-level methods to recoup these fees, such by including a small payment to themselves in the packet data.

Any number of relayer processes may be safely run in parallel (and indeed, it is expected that separate relayers will serve separate subsets of the multi-ledger network). However, they may consume unnecessary fees if they submit the same proof multiple times, so some minimal coordination may be ideal (such as assigning particular relayers to particular packets or scanning mempools for pending transactions).
