<!-- omit in toc -->
# CCV: System Model and Properties
[&uparrow; Back to main document](./README.md)

<!-- omit in toc -->
## Outline
- [Assumptions](#assumptions)
- [Desired Properties](#desired-properties)
  - [Validator sets, validator updates and VSCs](#validator-sets-validator-updates-and-vscs)
  - [Staking Module Interface](#staking-module-interface)
  - [Validator Set Update](#validator-set-update)
- [Correctness Reasoning](#correctness-reasoning)

## Assumptions
[&uparrow; Back to Outline](#outline)

As part of an ABCI application, CCV interacts with both the consensus engine (via ABCI) and other application modules, such as the Staking module. 
As an IBC application, CCV interacts with external relayers (defined in [ICS 18](../../relayer/ics-018-relayer-algorithms)).  
In this section we specify what we assume about these other components, 
i.e., CCV relies on the following assumptions: *Valid Blockchain*, *Correct Relayer*, *Validator Update Provision*, and *Unbonding Safety*. 

Intuitively, CCV safety relies on the *Valid Blockchain* assumption, and CCV liveness relies on the *Correct Relayer* assumption. 
The *Validator Update Provision* and *Unbonding Safety* assumptions define what is needed from the provider Staking module. 
A more thorough discussion of the environment in which CCV operates is given in the section [Placing CCV within an ABCI Application](./technical_specification.md#placing-ccv-within-and-abci-application).

- ***Valid Blockchain***: Both the provider and the consumer chains are *valid*. 
  This means that the protocols are executed correctly and the underlying consensus algorithm satisfies both safety and liveness properties. 
  For more details, take a look at the [Tendermint Paper](https://arxiv.org/pdf/1807.04938.pdf).

- ***Correct Relayer***: There is at least one *correct* relayer between the provider and consumer chains -- every packet sent on the CCV channel is relayed to the receiving end before the packet timeout elapses. 
  Clearly, the CCV protocol is responsible of setting the packet timeouts (i.e., `timeoutHeight` and `timeoutTimestamp`) such that the *Correct Relayer* assumption is feasible. 
 
> **Discussion**: IBC relies on timeouts to signal that a sent packet is not going to be received on the other end. 
> Once an ordered IBC channel timeouts, the channel is closed (see [ICS 4](../../core/ics-004-channel-and-packet-semantics)). 
> The *Correct Relayer* assumption is necessary to ensure that the CCV channel **cannot** ever timeout and, as a result, cannot transit to the closed state. 
> 
> **In practice**, the *Correct Relayer* assumption is realistic since any validator could play the role of the relayer and it is in the best interest of correct validators to successfully relay packets.
> 
> The following strategy is a practical example of how to ensure the *Correct Relayer* assumption holds. 
> Let S denote the sending chain and D the destination chain; 
> and let `drift(S,D)` be the time drift between S and D, 
> i.e., `drift(S,D) =  S.currentTimestamp() - D.currentTimestamp()` (`drift(S,D) > 0` means that S is "ahead" of D). 
> For every packet, S only sets `timeoutTimestamp = S.currentTimestamp() + to`, with `to` an application-level parameter. 
> The `timeoutTimestamp` indicates *a timestamp on the destination chain* after which the packet will no longer be processed (cf. [ICS 4](../../core/ics-004-channel-and-packet-semantics)). 
> Therefore, the packet MUST be relayed within a time period of `to - drift(S,D)`, 
> i.e., `to - drift(S,D) > RTmax`, where `RTmax` is the maximum relaying time across all packet. 
> Theoretically, choosing the value of `to` requires knowing the value of `drift(S,D)` (i.e., `to > drift(S,D)`); 
> yet, `drift(S,D)` is not known at a chain level. 
> In practice, choosing `to` such that `to >> drift(S,D)` and `to >> RTmax`, e.g., `to = 4 weeks`, makes the *Correct Relayer* assumption feasible.

The following assumptions define the guarantees CCV expects from the provider Staking module. 
- ***Validator Update Provision***: Let `{U1, U2, ..., Ui}` be a batch of validator updates applied (by the provider Staking module) to the validator set of the provider chain at the end of a block `B` with timestamp `t`. 
  Then, the *first* batch of validator updates obtained (by the provider CCV module) from the provider Staking module at time `t` MUST be exactly the batch `{U1, U2, ..., Ui}`.

- ***Unbonding Safety***: Let `uo` be any unbonding operation that starts with an unboding transaction being executed 
  and completes with the event that returns the corresponding stake; 
  let `U(uo)` be the validator update caused by initiating `uo`; 
  let `vsc(uo)` be the VSC that contains `U(uo)`. 
  Then,
  - (*unbonding initiation*) the provider CCV module MUST be notified of `uo`'s initiation before receiving `U(uo)`;
  - (*unbonding completion*) `uo` MUST NOT complete on the provider chain before the provider chain registers notifications of `vsc(uo)`'s maturity from all consumer chains.

> **Note**: Depending on the implementation, the (*unbonding initiation*) part of the *Unbonding Safety* MAY NOT be necessary for validator unbonding operations.

## Desired Properties
[&uparrow; Back to Outline](#outline)

The following properties are concerned with **one provider chain** providing security to **multiple consumer chains**. 
Between the provider chain and each consumer chain, a separate (unique) CCV channel is established. 

First, we define the properties for the CCV channels. Then, we define the guarantees provided by CCV.

- ***Channel Uniqueness***: The channel between the provider chain and a consumer chain MUST be unique.
- ***Channel Validity***: If a packet `P` is received by one end of the channel, then `P` MUST have been sent by the other end of the channel.
- ***Channel Order***: If a packet `P1` is sent over the channel before a packet `P2`, then `P2` MUST NOT be received by the other end of the channel before `P1`. 
- ***Channel Liveness***: Every packet sent over the channel MUST eventually be received by the other end of the channel. 

CCV provides the following system invariants:
- ***Validator Set Invariant***: Every validator set on any consumer chain MUST either be or have been a validator set on the provider chain.
- ***Voting Power Invariant***: Let
  - `pBonded(t,val)` be the number of tokens bonded by validator `val` on the provider chain at time `t`; 
    note that `pBonded(t,val)` includes also unbonding tokens (i.e., tokens in the process of being unbonded);
  - `VP(T)` be the voting power associated to a number `T` of tokens;
  - `Power(cc,t,val)` be the voting power granted to a validator `val` on a consumer chain `cc` at time `t`.  
  
  Then, for all times `t` and `s`, all consumer chains `cc`, and all validators `val`,  
  ```
  t <= s <= t + UnbondingPeriod: Power(cc,t,val) <= VP(pBonded(s,val))
  ```

  > **Intuition**: 
  > - The *Voting Power Invariant* ensures that validators that validate on the consumer chain have enough tokens bonded on the provider chain for a sufficient amount of time such that the security model holds. 
  > This means that if the validators misbehave on the consumer chain, their tokens bonded on the provider chain can be slashed during the unbonding period.
  > For example, if one unit of voting power requires `1.000.000` bonded tokens (i.e., `VP(1.000.000)=1`), 
  > then a validator that gets one unit of voting power on the consumer chain must have at least `1.000.000` tokens bonded on the provider chain for at least `UnbondingPeriod`.

Before we define the properties of CCV needed for these invariants to hold, 
we provide a short discussion on how the validator set, the validator updates, and the VSCs relates in the context of multiple chains. 

### Validator sets, validator updates and VSCs
[&uparrow; Back to Outline](#outline)

Every chain consists of a sequence of blocks. 
At the end of each block, validator updates (i.e., changes in the validators voting power) results in changes in the validator set of the next block. 
Thus, the sequence of blocks produces a sequence of validator updates and a sequence of validator sets. 
Furthermore, the sequence of validator updates on the provider chain results in a sequence of VSCs to all consumer chains. 
Ideally, this sequence of VSCs is applied by every consumer chain, resulting in a sequence of validator sets identical to the one on the provider chain. 
However, in general this need not be the case. The reason is twofold: 
- first, given any two chains `A` and `B`, we cannot assume that `A`'s rate of adding new block is the same as `B`'s rate 
  (i.e., we consider the sequences of blocks of any two chains to be completely asynchronous); 
- and second, due to relaying delays, we cannot assume that the rate of sending VSCs matches the rate of receiving VSCs.

As a result, is it possible for multiple VSCs to be received by a consumer chain within the same block and be applied together at the end of the block, 
i.e., the validator updates within the VSCs are being *aggregated* by keeping only the latest update per validator. 
As a consequence, some validator sets on the provider chain are not existing on all consumer chains. 
In other words, the validator sets on each consumer chain form a *subsequence* of the validator sets on the provider chain. 
Nonetheless, as a **requirement of CCV**, *all the validator updates on the provider chain MUST be included in the sequence of validator sets on all consumer chains*.

This is possible since every validator update contains *the absolute voting power* of that validator. 
Given a validator `val`, the sequence of validator updates targeting `val` (i.e., updates of the voting power of `val`) is the prefix sum of the sequence of relative changes of the voting power of `val`. 
Thus, given a validator update `U` targeting `val` that occurs at at a time `t`, 
`U` *sums up* all the relative changes of the voting power of `val` that occur until `t`, 
i.e., `U = c_1+c_2+...+c_i`, such that `c_i` is the last relative change that occurs by `t`. 
Note that relative changes are integer values. 

As a consequence, CCV can rely on the following property:
- ***Validator Update Inclusion***: Let `U1` and `U2` be two validator updates targeting the same validator `val`. 
  If `U1` occurs before `U2`, then `U2` sums up all the changes of the voting power of `val` that are summed up by `U1`, i.e., 
  - `U1 = c_1+c_2+...+c_i` and
  - `U2 = c_1+c_2+...+c_i+c_(i+1)+...+c_j`.
 
The *Validator Update Inclusion* property enables CCV to aggregate multiple VSCs. 
It is sufficient for the consumer chains to apply only the last update per validator. 
Since the last update of a validator *includes* all the previous updates of that validator, once it is applied, all the previous updates are also applied.

### Staking Module Interface
[&uparrow; Back to Outline](#outline)

The following properties define the guarantees of CCV on *providing* VSCs to the consumer chains as a consequence of validator updates on the provider chain. 
- ***Validator Update To VSC Validity***: Every VSC provided to a consumer chain MUST contain only validator updates that were applied to the validator set of the provider chain (i.e., resulted from a change in the amount of bonded tokens on the provider chain).
- ***Validator Update To VSC Order***: Let `U1` and `U2` be two validator updates on the provider chain. If `U1` occurs before `U2`, then `U2` MUST NOT be included in a provided VSC before `U1`. Note that the order within a single VSC is not relevant.
- ***Validator Update To VSC Liveness***: Every update of a validator in the validator set of the provider chain MUST eventually be included in a VSC provided to all consumer chains. 

Note that as a consequence of the *Validator Update To VSC Liveness* property, CCV guarantees the following property:
- **Provide VSC uniformity**: If the provider chain provides a VSC to a consumer chain, then it MUST eventually provide that VSC to all consumer chains. 

### Validator Set Update
[&uparrow; Back to Outline](#outline)

The provider chain providing VSCs to the consumer chains has two desired outcomes: the consumer chains apply the VSCs; and the provider chain registers VSC maturity notifications from every consumer chain. 
Thus, for clarity, we split the properties of VSCs in two: properties of applying provided VSCs on the consumer chains; and properties of registering VSC maturity notifications on the provider chain. 
For simplicity, we focus on a single consumer chain.

The following properties define the guarantees of CCV on *applying* on the consumer chain VSCs *provided* by the provider chain.  
- ***Apply VSC Validity***: Every VSC applied by the consumer chain MUST be provided by the provider chain.
- ***Apply VSC Order***: If a VSC `vsc1` is provided by the provider chain before a VSC `vsc2`, then the consumer chain MUST NOT apply the validator updates included in `vsc2` before the validator updates included in `vsc1`.
- ***Apply VSC Liveness***: If the provider chain provides a VSC `vsc`, then the consumer chain MUST eventually apply all validator updates included in `vsc`.

The following properties define the guarantees of CCV on *registering* on the provider chain maturity notifications (from the consumer chain) of VSCs *provided* by the provider chain to the consumer chain.
- ***Register Maturity Validity***: If the provider chain registers a maturity notification of a VSC from the consumer chain, then the provider chain MUST have provided that VSC to the consumer chain. 
- ***Register Maturity Timeliness***: The provider chain MUST NOT register a maturity notification of a VSC `vsc` before `UnbondingPeriod` has elapsed on the consumer chain since the consumer chain applied `vsc`.
- ***Register Maturity Order***: If a VSC `vsc1` was provided by the provider chain before another VSC `vsc2`, then the provider chain MUST NOT register the maturity notification of `vsc2` before the maturity notification of `vsc1`.
- ***Register Maturity Liveness***: If the provider chain provides a VSC `vsc` to the consumer chain, then the provider chain MUST eventually register a maturity notification of `vsc` from the consumer chain.

> Note that, except for *Apply VSC Liveness* and *Register Maturity Liveness*, none of the properties of CCV require the *Correct Relayer* assumption to hold.

## Correctness Reasoning
[&uparrow; Back to Outline](#outline)

In this section we argue the correctness of the CCV protocol described in the [Technical Specification](./technical_specification.md), 
i.e., we informally prove the properties described in the [previous section](#desired-properties).

- ***Channel Uniqueness*:** The provider chain sets the CCV channel when receiving (from the consumer chain) the first `ChanOpenConfirm` message and it marks the channel as `INVALID` when receiving any subsequent `ChanOpenConfirm` messages (cf. *Valid Blockchain*). 
  Similarly, the consumer chain sets the CCV channel when receiving the first `VSCPacket` and ignores any packets received on different channels (cf. *Valid Blockchain*). 

- ***Channel Validity*:** Follows directly from the *Valid Blockchain* assumption.

- ***Channel Order*:** The provider chain accepts only ordered channels when receiving a `ChanOpenTry` message (cf. *Valid Blockchain*). 
  Similarly, the consumer chain accepts only ordered channels when receiving `ChanOpenInit` messages (cf. *Valid Blockchain*). 
  Thus, the property follows directly from the fact that the CCV channel is ordered. 

- ***Channel Liveness*:** The property follows from the *Correct Relayer* assumption. 

- ***Validator Update To VSC Validity***: The provider CCV module provides only VSCs that contain validator updates obtained from the Staking module, 
  i.e., by calling the `GetValidatorUpdates()` method (cf. *Valid Blockchain*). 
  Furthermore, these validator updates were applied to the validator set of the provider chain (cf. *Validator Update Provision*).

- ***Validator Update To VSC Order***: We prove the property through contradiction. 
  Given two validator updates `U1` and `U2`, with `U1` occurring on the provider chain before `U2`, we assume `U2` is included in a provided VSC before `U1`. 
  However, `U2` could not have been obtained by the provider CCV module before `U1` (cf. *Validator Update Provision*). 
  Thus, the provider CCV module could not have provided a VSC that contains `U2` before a VSC that contains `U1` (cf. *Valid Blockchain*), which contradicts the initial assumption.
  
- ***Validator Update To VSC Liveness***: The provider CCV module eventually provides to all consumer chains VSCs containing all validator updates obtained from the provider Staking module (cf. *Valid Blockchain*). 
  Thus, it is sufficient to prove that every update of a validator in the validator set of the provider chain MUST eventually be obtained from the provider Staking module. 
  We prove this through contradiction. Given a validator update `U` that is applied to the validator set of the provider chain at the end of a block `B` with timestamp `ts(B)`, we assume `U` is never obtained by the provider CCV module. 
  However, there is a time `t >= ts(B)` when the provider CCV module tries to obtain a new batch of validator updates from the provider Staking module (cf. liveness property guaranteed by *Valid Blockchain*). 
  Thus, this batch of validator updates MUST contain all validator updates applied to the validator set of the provider chain at the end of block `B`, including `U` (cf. *Validator Update Provision*), which contradicts the initial assumption.

- ***Apply VSC Validity*:** The property follows from the following two assertions.
  - The consumer chain only applies VSCs received in `VSCPacket`s through the CCV channel (cf. *Valid Blockchain*).
  - The provider chain only sends `VSCPacket`s containing provided VSCs (cf. *Valid Blockchain*). 

- ***Apply VSC Order*:** We prove the property through contradiction. 
  Given two VSCs `vsc1` and `vsc2` such that the provider chain provides `vsc1` before `vsc2`, we assume the consumer chain applies the validator updates included in `vsc2` before the validator updates included in `vsc1`. 
  The following sequence of assertions leads to a contradiction.
  - The provider chain could not have sent a `VSCPacket` `P2` containing `vsc2` before a `VSCPacket` `P1` containing `vsc1` (cf. *Valid Blockchain*).
  - The consumer chain could not have received `P2` before `P1` (cf. *Channel Order*).
  - Given the *Valid Blockchain* assumption, we distinguish two cases.
    - First, the consumer chain receives `P1` during block `B1` and `P2` during block `B2` (with `B1` < `B2`). 
    Then, it applies the validator updates included in `vsc1` at the end of `B1` and the validator updates included in `vsc2` at the end of `B2` (cf. *Validator Update Inclusion*), which contradicts the initial assumption. 
    - Second, the consumer chain receives both `P1` and `P2` during the same block. 
    Then, it applies the validator updates included in both `vsc1` and `vsc2` at the end of the block. 
    Thus, it could not have apply the validator updates included in `vsc2` before.

- ***Apply VSC Liveness*:** The provider chain eventually sends over the CCV channel a `VSCPacket` containing `vsc` (cf. *Valid Blockchain*). 
  As a result, the consumer chain eventually receives this packet (cf. *Channel Liveness*). 
  Then, the consumer chain aggregates all received VSCs at the end of the block and applies all the aggregated updates (cf. *Valid Blockchain*). 
  As a result, the consumer chain applies all validator updates in `vsc` (cf. *Validator Update Inclusion*).

- ***Register Maturity Validity***: The property follows from the following sequence of assertions.
  - The provider chain only registers VSC maturity notifications when receiving on the CCV channel acknowledgements of `VSCPacket`s (cf. *Valid Blockchain*). 
  - The provider chain receives on the CCV channel only packets sent by the consumer chain (cf. *Channel Validity*).
  - The consumer chain only acknowledges `VSCPacket`s that it receives on the CCV channel (cf. *Valid Blockchain*).
  - The consumer chain receives on the CCV channel only packets sent by the provider chain (cf. *Channel Validity*). 
  - The provider chain only sends `VSCPacket`s containing provided VSCs (cf. *Valid Blockchain*). 

- ***Register Maturity Timeliness*:** We prove the property through contradiction. 
  Given a VSC `vsc` provided by the provider chain to the consumer chain, we assume that the provider chain registers a maturity notification of `vsc` before `UnbondingPeriod` has elapsed on the consumer chain since the consumer chain applied `vsc`. 
  The following sequence of assertions leads to a contradiction.
  - The provider chain could not have register a maturity notification of `vsc` before receiving on the CCV channel an acknowledgements of a `VSCPacket` `P` with `P.updates = C` (cf. *Valid Blockchain*). 
  - The provider chain could not have received an acknowledgement of `P` on the CCV channel before the consumer chain sent it (cf. *Channel Validity*).
  - The consumer chain could not have sent an acknowledgement of `P` before at least `UnbondingPeriod` has elapsed since receiving `P` on the CCV channel (cf. *Valid Blockchain*). 
  Note that since time is measured in terms of the block time, the time of receiving `P` is the same as the time of applying `vsc`.
  - The consumer chain could not have received `P` on the CCV channel before the provider chain sent it (cf. *Channel Validity*).  
  - The provider chain could not have sent `P` before providing `vsc`. 
  - Since the duration of sending packets through the CCV channel cannot be negative, the provider chain could not have registered a maturity notification of `vsc` before `UnbondingPeriod` has elapsed on the consumer chain since the consumer chain applied `vsc`.

- ***Register Maturity Order*:** We prove the property through contradiction. Given two VSCs `vsc1` and `vsc2` such that the provider chain provides `vsc1` before `vsc2`, we assume the provider chain registers the maturity notification of `vsc2` before the maturity notification of `vsc1`. 
  The following sequence of assertions leads to a contradiction.
  - The provider chain could not have sent a `VSCPacket` `P2`, with `P2.updates = C2`, before a `VSCPacket` `P1`, with `P1.updates = C1` (cf. *Valid Blockchain*).
  - The consumer chain could not have received `P2` before `P1` (cf. *Channel Order*).
  - The consumer chain could not have sent the acknowledgment of `P2` before the acknowledgement of `P1` (cf. *Valid Blockchain*).
  - The provider chain could not have received the acknowledgment of `P2` before the acknowledgement of `P1` (cf. *Channel Order*).
  - The provider chain could not have registered the maturity notification of `vsc2` before the maturity notification of `vsc1` (cf. *Valid Blockchain*).

- ***Register Maturity Liveness*:** The property follows from the following sequence of assertions.
  - The provider chain eventually sends on the CCV channel a `VSCPacket` `P`, with `P.updates = C` (cf. *Valid Blockchain*).
  - The consumer chain eventually receives `P` on the CCV channel (cf. *Channel Liveness*).
  - The consumer chain eventually sends an acknowledgement of `P` on the CCV channel (cf. *Valid Blockchain*).
  - The provider chain eventually receives the acknowledgement of `P` on the CCV channel (cf. *Channel Liveness*).
  - The provider chain eventually registers the maturity notification of `vsc` (cf. *Valid Blockchain*).


- ***Validator Set Invariant***: The invariant follows from the *Valid Blockchain* assumption and both the *Apply VSC Validity* and *Validator Update To VSC Validity* properties. 

- ***Voting Power Invariant***: To prove the invariant, we use the following property that follows directly from the design of the protocol (cf. *Valid Blockchain*).
  - *Property1*: Let `val` be a validator; let `Ua` and `Ub` be two updates of `val` that are applied subsequently by a consumer chain `cc`, at times `ta` and `tb`, respectively (i.e., no other updates of `val` are applied in between). 
  Then, `Power(cc,ta,val) = Power(cc,t,val)`, for all times `t`, such that `ta <= t < tb` (i.e., the voting power granted to `val` on `cc` in the period between `ta` and `tb` is constant).  

  We prove the invariant through contradiction. 
  Given a consumer chain `cc`, a validator `val`, and times `t` and `s` such that `t <= s <= t + UnbondingPeriod`, we assume `Power(cc,t,val) > VP(pBonded(s,val))`. 
  The following sequence of assertions leads to a contradiction.
  - Let `U1` be the latest update of `val` that is applied by `cc` before or not later than time `t` 
    (i.e., `U1` is the update that sets `Power(cc,t,val)` for `val`). 
    Let `t1` be the time `U1` occurs on the provider chain; let `t2` be the time `U1` is applied on `cc`. 
    Then, `t1 <= t2 <= t` and `Power(cc,t2,val) = Power(cc,t,val)`.
    This means that some of the tokens bonded by `val` at time `t1` (i.e., `pBonded(t1,val)`) were *completely* unbonded before or not later than time `s` (cf. `pBonded(s,val) < pBonded(t1,val)`). 
  - Let `uo` be the first such unbonding operation that is initiated on the provider chain at a time `t3`, such that `t1 < t3 <= s`. 
    Note that at time `t3`, the tokens unbonded by `uo` are still part of `pBonded(t1,val)`.
    Let `U2` be the validator update caused by initiating `uo`.
    Let `t4` be the time `U2` is applied on `cc`; clearly, `t3 <= t4` and `Power(cc,t4,val) < Power(cc,t,val)`. 
    Note that the existence of `t4` is ensured by *Validator Update To VSC Liveness* and *Apply VSC Liveness*.
    Then, `t4 > t2` (cf. `t3 > t1`, *Validator Update To VSC Order*, *Apply VSC Order*). 
  - `Power(cc,t,val) = Power(cc,t2,val) = Power(cc,t',val)`, for all times `t'`, such that `t2 <= t' < t4` (cf. *Property1*). 
    Thus, `t4 > t` (cf. `Power(cc,t4,val) < Power(cc,t,val)`).
  - `uo` cannot complete before `t4 + UnbondingPeriod`, which means it cannot complete before `s` (cf. `t4 > t`, `s <= t + UnbondingPeriod`). 