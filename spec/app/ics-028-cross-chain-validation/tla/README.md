# Modeling and checking cross-chain validaiton

The file `CCV.tla` contains a TLA+ specification of a simplified version of cross-chain validation, with the following restrictions:
  
  - We only track voting power, not bonded tokens
  - CCV channel creation is atomic and never fails/times out.
  - There is a fixed upper bound on the number of consumer chains. 
  - Block height is not modeled.

# Parameterization

The specification uses the following parameters:

  - `Nodes: Set(N)`: The set of all nodes, which may take on a validator role. At every given point in time, the set of currently recognized validators is a nonempty subset of `Nodes`.
  - `ConsumerChains: Set(C)`: An upper bound on participating consumer chains. At every given point in time, every element `c` of `ConsumerChains` is in one of four distinct states: `Unused`, `Initializing`, `Active`, or `Dropped`
    - `Unused` chains are not treated as consumers w.r.t. the provider chain, but may become consumers in the future.
    - `Initializing` chains are considered consumers w.r.t. the provider chain, in a state of initialization (modeling the establishment of the communication channel). They receive all messages broadcast by the provider, but cannot respond until their status changes.
    - `Active` chains are considered consumers w.r.t. the provider chain, with a fully established communication channel. They may send and receive messages.
    - `Dropped` chains are not treated as consumers w.r.t. the provider chain, and may never become consumers (again).
  - `UnbondingPeriod: Int`: Time that needs to elapse, before a received VPC is considered mature on a chain.
  - `Timeout: Int`: Time that needs to elapse on the provider chain after having sent a message to an `Active` chain, to which no reply was received, before that message is considered to have timed out (resulting in the removal of the related consumer chain).
  - `MaxDrift: Int`: Maximal time by which clocks are assumed to differ from the provider chain. The specification doesn't force clocks to maintain bounded drift, but the invariants are only verified in cases where clocks never drift too far.
  - `InactivityTimeout: Int`: Time that needs to elapse on the provider chain after having sent a message to an `Initializing` chain, to which no reply was received, before that message is considered to have timed out (resulting in the removal of the related consumer chain).

# Model variables

The mutable system components modeled are as follows:

  - Provider chain only:
    - `votingPowerRunning: N -> Int`: Current voting power on the provider chain of all validator nodes. The following holds true: `node \in Nodes` is a validator iff `node \in DOMAIN votingPowerRunning`.
    - `votingPowerHist: Int -> (N -> Int)`: Snapshots of the voting power on the provider chain, at the times when a VPC packet was sent. The following holds true: `t \in DOMAIN votingPowerHist` iff VPC packet sent at time `t`.
    - `consumerStatus: C -> STATUS`: Current status for each chain in `ConsumerChains`. May be one of: `Unused`, `Initializing`, `Active`, `Dropped`.
    - `expectedResponders: Int -> Set(C)`: The set of chains live at the time a packet was sent (who are expected to reply). A chain is live, if it is either `Initializing` or `Active`.
    - `maturePackets: Set(matureVSCPacket)`: The set of `MatureVSCPacket`s sent by consumer chains to the provider chain.
  - Consumer chains or both:
    - `votingPowerReferences: C -> Int`: Representation of the current voting power, as understood by consumer chains. Because consumer chains may not arbitrarily modify their own voting power, but must instead update in accordance to VPC packets received from the provider, it is sufficient to only track the last received packet. The voting power on chain `c` is then equal to `votingPowerHist[votingPowerReferences[c]]`.
    - `ccvChannelsPending: C -> Seq(Int)`: The queues of VPC packets, waiting to be received by consumer chains. Note that a packet being placed in the channel is not considered received by the consumer, until the receive-action is taken.
    - `ccvChannelsResolved: C -> Seq(Int)`: The queues of VPC packets, that have been received by consumer chains in the past.
    - `currentTimes: C -> Int`: The current clocks of all chains (including the provider).
    - `maturityTimes: C -> Int -> Int`: Bookkeeping of maturity times for received packets. A consumer may only send a `MatureVSCPacket` (i.e. notify the provider) after its local time exceeds the time designated in `maturityTimes`. For each consumer chain `c`, and VSC packet `t` sent by the provider, the following holds true:
      - `t \in DOMAIN maturityTimes[c]` iff `c` has received packet `t`.
      - if `t \in DOMAIN maturityTimes[c]`, then maturity for `t` on `c` is reached when `currentTimes[c] >= maturityTimes[c][t]`.
  - Bookkeeping:
    - `lastAction: Str`: Name of the last action taken, for debugging.
    - `votingPowerHasChanged: Bool`: We use this flag to determine whether it is necessary to send a VPC packet.
    - `boundedDrift: Bool`: Invariant flag, TRUE iff clocks never drifted too much

# System actions

The following actions are modeled:
  
  - `VotingPowerChange`: 
    - Prerequisite(s): None.
    - Effect(s): 
        - Modifies `votingPowerRunning` and sets `votingPowerHasChanged` to `TRUE`. 
        - May change the set of current validators.
  - `EndProviderBlockAndSendPacket`: 
    - Prerequisite(s): `votingPowerHasChanged` is `TRUE`.
    - Effect(s): 
        - Sends packet to all currently live consumers' queues.
        - Resets `votingPowerHasChanged` to `FALSE`.
        - Provider clock advances, consumer clocks may advance.
  - `RcvPacket`: 
    - Prerequisite(s): There exists a consumer chain `c` with a nonempty packet queue.
    - Effect(s):
        - Adjusts `votingPowerReferences` at `c` to the head packet.
        - Moves the packet from `ccvChannelsPending[c]` to `ccvChannelsResolved[c]`.
        - Determines future maturity point (in `maturityTimes`).
  - `SendMatureVSCPacket`:
    - Prerequisite(s): 
        - There exists a consumer chain `c` with a received packet `p`.
        - The current local clock of `c` is at least `maturityTimes[c][p]`.
        - `MatureVSCPacket` response has not yet been sent by `c` for `p`.
    - Effect(s): `MatureVSCPacket` is added to `maturePackets` for the pair `c` and `p`.
  - `AdvanceTime`:
    - Prerequisite(s): None.
    - Effect(s): Local clocks may advance for the provider and any of the consumers.

Additionally, after every action, consumer chain statuses may promote, following the below restrictions:
  
  - Each consumer chain's state may remain unchanged
  - `Unused` chains may become `Initializing`, `Active` or `Dropped`
  - `Initializing` chains may become `Active` or `Dropped`
  - `Active` chains may become `Dropped`
  - `Active` chains _must_ become dropped, if their packets time out either on reception, or on signaling maturity (w.r.t `Timeout`).
  - `Initializing` chains _must_ become dropped, if they do not signal maturity by `InactivityTimeout` (time measured solely on the provider chain).

# Invariants

The specification implements the following invarians from [this table](https://github.com/cosmos/interchain-security/blob/6036419de667590518cccd1c8fcae0d01cbe67e9/docs/old/quality_assurance.md): 4.11, 6.01, 6.02, 6.03, 7.01, and 7.02 as
`Inv411`, `Inv601`, `Inv602`, `Inv603`, `Inv701` and `Inv702` respectively.
See the specification for details and adjustments.

# Testing

We've tested the specification with the [Apalache](https://github.com/informalsystems/apalache/) model checker.

Last test: November 2022

Invariant tested: 
```
Inv ==
  /\ Inv411
  /\ Inv601
  /\ Inv602
  /\ Inv603
  /\ Inv701
  /\ Inv702
```

Outcome: No invariant violation found in 9 steps.
