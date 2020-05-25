This standard specifies the properties that consensus algorithms of machines implementing the interblockchain
communication protocol are required to satisfy. These properties are necessary for efficient and safe
verification in the higher-level protocol abstractions. The algorithm utilised in IBC to verify the 
consensus transcript & state sub-components of another machine is referred to as a "validity predicate",
and pairing it with a state that the verifier assumes to be correct forms a "light client" (often shortened to "client").

This standard also specifies how light clients will be stored, registered, and updated in the 
canonical IBC handler. The stored client instances will be introspectable by a third party actor,
such as a user inspecting the state of the chain and deciding whether or not to send an IBC packet.

### Motivation

In the IBC protocol, an actor, which may be an end user, an off-chain process, or a machine,
needs to be able to verify updates to the state of another machine
which the other machine's consensus algorithm has agreed upon, and reject any possible updates
which the other machine's consensus algorithm has not agreed upon. A light client is the algorithm
with which a machine can do so. This standard formalises the light client model and requirements,
so that the IBC protocol can easily integrate with new machines which are running new consensus algorithms
as long as associated light client algorithms fulfilling the listed requirements are provided.

Beyond the properties described in this specification, IBC does not impose any requirements on
the internal operation of machines and their consensus algorithms. A machine may consist of a
single process signing operations with a private key, a quorum of processes signing in unison,
many processes operating a Byzantine fault-tolerant consensus algorithm, or other configurations yet to be invented
— from the perspective of IBC, a machine is defined entirely by its light client validation & equivocation detection logic.
Clients will generally not include validation of the state transition logic in general
(as that would be equivalent to simply executing the other state machine), but may 
elect to validate parts of state transitions in particular cases.

Clients could also act as thresholding views of other clients. In the case where
modules utilising the IBC protocol to interact with probabilistic-finality consensus algorithms
which might require different finality thresholds for different applications, one write-only
client could be created to track headers and many read-only clients with different finality
thresholds (confirmation depths after which state roots are considered final) could use that same state.

The client protocol should also support third-party introduction. Alice, a module on a machine,
wants to introduce Bob, a second module on a second machine who Alice knows (and who knows Alice),
to Carol, a third module on a third machine, who Alice knows but Bob does not. Alice must utilise
an existing channel to Bob to communicate the canonically-serialisable validity predicate for 
Carol, with which Bob can then open a connection and channel so that Bob and Carol can talk directly.
If necessary, Alice may also communicate to Carol the validity predicate for Bob, prior to Bob's
connection attempt, so that Carol knows to accept the incoming request.

Client interfaces should also be constructed so that custom validation logic can be provided safely
to define a custom client at runtime, as long as the underlying state machine can provide an
appropriate gas metering mechanism to charge for compute and storage. On a host state machine
which supports WASM execution, for example, the validity predicate and equivocation predicate
could be provided as executable WASM functions when the client instance is created.

### Definitions

* `get`, `set`, `Path`, and `Identifier` are as defined in [ICS 24](../ics-024-host-requirements).

* `CommitmentRoot` is as defined in [ICS 23](../ics-023-vector-commitments). It must provide an inexpensive way for
  downstream logic to verify whether key/value pairs are present in state at a particular height.

* `ConsensusState` is an opaque type representing the state of a validity predicate.
  `ConsensusState` must be able to verify state updates agreed upon by the associated consensus algorithm.
  It must also be serialisable in a canonical fashion so that third parties, such as counterparty machines,
  can check that a particular machine has stored a particular `ConsensusState`. It must finally be
  introspectable by the state machine which it is for, such that the state machine can look up its
  own `ConsensusState` at a past height.

* `ClientState` is an opaque type representing the state of a client.
  A `ClientState` must expose query functions to verify membership or non-membership of
  key/value pairs in state at particular heights and to retrieve the current `ConsensusState`.

### Desired Properties

Light clients must provide a secure algorithm to verify other chains' canonical headers,
using the existing `ConsensusState`. The higher level abstractions will then be able to verify
sub-components of the state with the `CommitmentRoot`s stored in the `ConsensusState`, which are
guaranteed to have been committed by the other chain's consensus algorithm.

Validity predicates are expected to reflect the behaviour of the full nodes which are running the
corresponding consensus algorithm. Given a `ConsensusState` and a list of messages, if a full node
accepts the new `Header` generated with `Commit`, then the light client MUST also accept it,
and if a full node rejects it, then the light client MUST also reject it.

Light clients are not replaying the whole message transcript, so it is possible under cases of
consensus misbehaviour that the light clients' behaviour differs from the full nodes'.
In this case, a misbehaviour proof which proves the divergence between the validity predicate
and the full node can be generated and submitted to the chain so that the chain can safely deactivate the
light client, invalidate past state roots, and await higher-level intervention.

### Data Structures

#### ConsensusState

`ConsensusState` is an opaque data structure defined by a client type, used by the validity predicate to
verify new commits & state roots. Likely the structure will contain the last commit produced by
the consensus process, including signatures and validator set metadata.

`ConsensusState` MUST be generated from an instance of `Consensus`, which assigns unique heights
for each `ConsensusState` (such that each height has exactly one associated consensus state).
Two `ConsensusState`s on the same chain SHOULD NOT have the same height if they do not have
equal commitment roots. Such an event is called an "equivocation" and MUST be classified
as misbehaviour. Should one occur, a proof should be generated and submitted so that the client can be frozen
and previous state roots invalidated as necessary.

The `ConsensusState` of a chain MUST have a canonical serialisation, so that other chains can check
that a stored consensus state is equal to another (see [ICS 24](../ics-024-host-requirements) for the keyspace table).

The `ConsensusState` MUST be stored under a particular key, defined below, so that other chains can verify that a particular consensus state has been stored.

The `ConsensusState` MUST define a `getTimestamp()` method which returns the timestamp associated with that consensus state:

#### Header

A `Header` is an opaque data structure defined by a client type which provides information to update a `ConsensusState`.
Headers can be submitted to an associated client to update the stored `ConsensusState`. They likely contain a height, a proof,
a commitment root, and possibly updates to the validity predicate.

#### Consensus

`Consensus` is a `Header` generating function which takes the previous
`ConsensusState` with the messages and returns the result.

The headers generated from a `Blockchain` are expected to satisfy the following:

1. Each `Header` MUST NOT have more than one direct child

* Satisfied if: finality & safety
* Possible violation scenario: validator double signing, chain reorganisation (Nakamoto consensus)

2. Each `Header` MUST eventually have at least one direct child

* Satisfied if: liveness, light-client verifier continuity
* Possible violation scenario: synchronised halt, incompatible hard fork

3. Each `Header`s MUST be generated by `Consensus`, which ensures valid state transitions

* Satisfied if: correct block generation & state machine
* Possible violation scenario: invariant break, super-majority validator cartel

Unless the blockchain satisfies all of the above the IBC protocol
may not work as intended: the chain can receive multiple conflicting
packets, the chain cannot recover from the timeout event, the chain can
steal the user's asset, etc.

The validity of the validity predicate is dependent on the security model of the
`Consensus`. For example, the `Consensus` can be a proof of authority with
a trusted operator, or a proof of stake but with
insufficient value of stake. In such cases, it is possible that the
security assumptions break, the correspondence between `Consensus` and
the validity predicate no longer exists, and the behaviour of the validity predicate becomes
undefined. Also, the `Blockchain` may not longer satisfy
the requirements above, which will cause the chain to be incompatible with the IBC
protocol. In cases of attributable faults, a misbehaviour proof can be generated and submitted to the
chain storing the client to safely freeze the light client and
prevent further IBC packet relay.

#### Validity predicate

A validity predicate is an opaque function defined by a client type to verify `Header`s depending on the current `ConsensusState`.
Using the validity predicate SHOULD be far more computationally efficient than replaying the full consensus algorithm
for the given parent `Header` and the list of network messages.

The validity predicate & client state update logic are combined into a single `checkValidityAndUpdateState` type, which is defined as

`checkValidityAndUpdateState` MUST throw an exception if the provided header was not valid.

If the provided header was valid, the client MUST also mutate internal state to store
now-finalised consensus roots and update any necessary signature authority tracking (e.g.
changes to the validator set) for future calls to the validity predicate.

Clients MAY have time-sensitive validity predicates, such that if no header is provided for a period of time
(e.g. an unbonding period of three weeks) it will no longer be possible to update the client.
In this case, a permissioned entity such as a chain governance system or trusted multi-signature MAY be allowed
to intervene to unfreeze a frozen client & provide a new correct header.

#### Misbehaviour predicate

A misbehaviour predicate is an opaque function defined by a client type, used to check if data
constitutes a violation of the consensus protocol. This might be two signed headers
with different state roots but the same height, a signed header containing invalid
state transitions, or other evidence of malfeasance as defined by the consensus algorithm.

The misbehaviour predicate & client state update logic are combined into a single `checkMisbehaviourAndUpdateState` type, which is defined as

`checkMisbehaviourAndUpdateState` MUST throw an exception if the provided evidence was not valid.

If misbehaviour was valid, the client MUST also mutate internal state to mark appropriate heights which
were previously considered valid as invalid, according to the nature of the misbehaviour.

Once misbehaviour is detected, clients SHOULD be frozen so that no future updates can be submitted.
A permissioned entity such as a chain governance system or trusted multi-signature MAY be allowed
to intervene to unfreeze a frozen client & provide a new correct header.

#### ClientState

`ClientState` is an opaque data structure defined by a client type.
It may keep arbitrary internal state to track verified roots and past misbehaviours.

Light clients are representation-opaque — different consensus algorithms can define different light client update algorithms —
but they must expose this common set of query functions to the IBC handler.

Client types MUST define a method to initialise a client state with a provided consensus state, writing to internal state as appropriate.

Client types MUST define a method to fetch the current height (height of the most recent validated header) & current timestamp.

#### State verification

Client types must define functions to authenticate internal state of the state machine which the client tracks.
Internal implementation details may differ (for example, a loopback client could simply read directly from the state and require no proofs).

### Example client instantiations

#### Loopback

A loopback client of a local machine merely reads from the local state, to which it must have access.

#### Simple signatures

A client of a solo machine with a known public key checks signatures on messages sent by that local machine,
which are provided as the `Proof` parameter. The `height` parameter can be used as a replay protection nonce.

Multi-signature or threshold signature schemes can also be used in such a fashion.

#### Proxy clients

Proxy clients verify another (proxy) machine's verification of the target machine, by including in the
proof first a proof of the client state on the proxy machine, and then a secondary proof of the sub-state of
the target machine with respect to the client state on the proxy machine. This allows the proxy client to
avoid storing and tracking the consensus state of the target machine itself, at the cost of adding
security assumptions of proxy machine correctness.

#### Merklized state trees

For clients of state machines with Merklized state trees, these functions can be implemented by calling `verifyMembership` or `verifyNonMembership`, using a verified Merkle
root stored in the `ClientState`, to verify presence or absence of particular key/value pairs in state at particular heights in accordance with [ICS 23](../ics-023-vector-commitments).

#### Identifier validation

Clients are stored under a unique `Identifier` prefix.
This ICS does not require that client identifiers be generated in a particular manner, only that they be unique.
However, it is possible to restrict the space of `Identifier`s if required.
The validation function `validateClientIdentifier` MAY be provided.

#### Utilising past roots

To avoid race conditions between client updates (which change the state root) and proof-carrying
transactions in handshakes or packet receipt, many IBC handler functions allow the caller to specify
a particular past root to reference, which is looked up by height. IBC handler functions which do this
must ensure that they also perform any requisite checks on the height passed in by the caller to ensure
logical correctness.

### Client lifecycle

#### Create

Calling `createClient` with the specified identifier & initial consensus state creates a new client.

#### Update

Updating a client is done by submitting a new `Header`. The `Identifier` is used to point to the
stored `ClientState` that the logic will update. When a new `Header` is verified with
the stored `ClientState`'s validity predicate and `ConsensusState`, the client MUST
update its internal state accordingly, possibly finalising commitment roots and
updating the signature authority logic in the stored consensus state.

If a client can no longer be updated (if, for example, the trusting period has passed),
it will no longer be possible to send any packets over connections & channels associated
with that client, or timeout any packets in-flight (since the height & timestamp on the
destination chain can no longer be verified). Manual intervention must take place to
reset the client state or migrate the connections & channels to another client. This
cannot safely be done completely automatically, but chains implementing IBC could elect
to allow governance mechanisms to perform these actions
(perhaps even per-client/connection/channel in a multi-sig or contract).

#### Misbehaviour

If the client detects evidence of misbehaviour, the client can be alerted, possibly invalidating
previously valid state roots & preventing future updates.

\begin{figure*}[!h]

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
function createClient(
  id: Identifier,
  clientType: ClientType,
  consensusState: ConsensusState) {
    abortTransactionUnless(validateClientIdentifier(id))
    abortTransactionUnless(privateStore.get(clientStatePath(id)) === null)
    abortSystemUnless(provableStore.get(clientTypePath(id)) === null)
    clientType.initialise(consensusState)
    provableStore.set(clientTypePath(id), clientType)
}
\end{lstlisting}
\caption{Client creation}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
function updateClient(
  id: Identifier,
  header: Header) {
    clientType = provableStore.get(clientTypePath(id))
    abortTransactionUnless(clientType !== null)
    clientState = privateStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    clientType.checkValidityAndUpdateState(clientState, header)
}
\end{lstlisting}
\caption{Client update handling}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
function submitMisbehaviourToClient(
  id: Identifier,
  evidence: bytes) {
    clientType = provableStore.get(clientTypePath(id))
    abortTransactionUnless(clientType !== null)
    clientState = privateStore.get(clientStatePath(id))
    abortTransactionUnless(clientState !== null)
    clientType.checkMisbehaviourAndUpdateState(clientState, evidence)
}
\end{lstlisting}
\caption{Misbehaviour submission}
\end{subfigure}

\caption{Client algorithm pseudocode}

\end{figure*}

\begin{figure*}[!h]

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyClientConsensusState = (
  clientState: ClientState,
  height: uint64,
  proof: CommitmentProof,
  clientIdentifier: Identifier,
  consensusStateHeight: uint64,
  consensusState: ConsensusState)
  => boolean
\end{lstlisting}
\caption{verifyClientConsensusState verifies a proof of the consensus state of the specified client stored on the target machine}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyConnectionState = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  connectionIdentifier: Identifier,
  connectionEnd: ConnectionEnd)
  => boolean
\end{lstlisting}
\caption{verifyConnectionState verifies a proof of the connection state of the specified connection end stored on the target machine}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyChannelState = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  channelEnd: ChannelEnd)
  => boolean
\end{lstlisting}
\caption{verifyChannelState verifies a proof of the channel state of the specified channel end, under the specified port, stored on the target machine}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyPacketData = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  data: bytes)
  => boolean
\end{lstlisting}
\caption{verifyPacketData verifies a proof of an outgoing packet commitment at the specified port, specified channel, and specified sequence}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyPacketAcknowledgement = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64,
  acknowledgement: bytes)
  => boolean
\end{lstlisting}
\caption{verifyPacketAcknowledgement verifies a proof of an incoming packet acknowledgement at the specified port, specified channel, and specified sequence}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyPacketAcknowledgementAbsence = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  sequence: uint64)
  => boolean
\end{lstlisting}
\caption{verifyPacketAcknowledgementAbsence verifies a proof of the absence of an incoming packet acknowledgement at the specified port, specified channel, and specified sequence}
\end{subfigure}

\begin{subfigure}{1.0\textwidth}
\begin{lstlisting}[language=JavaScript]
type verifyNextSequenceRecv = (
  clientState: ClientState,
  height: uint64,
  prefix: CommitmentPrefix,
  proof: CommitmentProof,
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  nextSequenceRecv: uint64)
  => boolean
\end{lstlisting}
\caption{verifyNextSequenceRecv verifies a proof of the next sequence number to be received of the specified channel at the specified port}
\end{subfigure}

\caption{Client state verification functions}

\end{figure*}
