<!-- omit in toc -->
# CCV: System Model and Properties
[&uparrow; Back to main document](./README.md)

<!-- omit in toc -->
## Outline
- [Assumptions](#assumptions)
- [Desired Properties](#desired-properties)
  - [System Properties](#system-properties)
  - [CCV Channel](#ccv-channel)
  - [Validator Sets, Validator Updates and VSCs](#validator-sets-validator-updates-and-vscs)
  - [Staking Module Interface](#staking-module-interface)
  - [Validator Set Update](#validator-set-update)
  - [Consumer Initiated Slashing](#consumer-initiated-slashing)
  - [Reward Distribution](#reward-distribution)
- [Correctness Reasoning](#correctness-reasoning)

## Assumptions
[&uparrow; Back to Outline](#outline)

As part of a modular ABCI application, CCV interacts with both the consensus engine (via ABCI) and other application modules (e.g, the Staking module). 
As an IBC application, CCV interacts with external relayers (defined in [ICS 18](../../relayer/ics-018-relayer-algorithms)). 
In this section we specify what we assume about these other components. 
A more thorough discussion of the environment in which CCV operates is given in the section [Placing CCV within an ABCI Application](./technical_specification.md#placing-ccv-within-an-abci-application).

> **Intuition**: 
> 
> CCV safety relies on the *Safe Blockchain* assumption, 
i.e., neither *Live Blockchain* and *Correct Relayer* are required for safety. 
Note though that CCV liveness relies on both *Live Blockchain* and *Correct Relayer* assumptions; 
furthermore, the *Correct Relayer* assumption relies on both *Safe Blockchain* and *Live Blockchain* assumptions. 
>
> The *Validator Update Provision*, *Unbonding Safety*, *Slashing Warranty*, and *Distribution Warranty* assumptions define what is needed from the ABCI application of the provider chain. 
> 
> The *Evidence Provision* assumptions defines what is needed from the ABCI application of the consumer chains.

- ***Safe Blockchain***: Both the provider and the consumer chains are *safe*. This means that, for every chain, the underlying consensus engine satisfies safety (e.g., the chain does not fork) and the execution of the state machine follows the described protocol. 
- ***Live Blockchain***: Both the provider and the consumer chains are *live*. This means that, for every chain, the underlying consensus engine satisfies liveness (i.e., new blocks are eventually added to the chain).
  > **Note**: Both *Safe Blockchain* and *Live Blockchain* assumptions require the consensus engine's assumptions to hold, e.g., less than a third of the voting power is Byzantine. For an example, take a look at the [Tendermint Paper](https://arxiv.org/pdf/1807.04938.pdf).

- ***Correct Relayer***: There is at least one *correct*, *live* relayer between the provider and consumer chains. This assumption has the following implications.
  - The opening handshake messages on the CCV channel are relayed before the Channel Initialization subprotocol times out (see `initTimeout`).
  - Every packet sent on the CCV channel is relayed to the receiving end before the packet timeout elapses (see both `vscTimeout` and `ccvTimeoutTimestamp`).
  - A correct relayer will eventually relay packets on the token transfer channel.   
  
  Clearly, the CCV protocol is responsible of setting the timeouts (see `ccvTimeoutTimestamp`, `vscTimeout`, `initTimeout` in the [CCV State](data_structures.md#ccv-state)), such that the *Correct Relayer* assumption is feasible.
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

- ***Validator Update Provision***: Let `{U1, U2, ..., Ui}` be a batch of validator updates applied (by the provider Staking module) to the validator set of the provider chain at block height `h`. 
  Then, the batch of validator updates obtained (by the provider CCV module) from the provider Staking module at height `h` MUST be exactly the batch `{U1, U2, ..., Ui}`.

- ***Unbonding Safety***: Let `uo` be any unbonding operation that starts with an unbonding transaction being executed 
  and completes with the event that returns the corresponding stake; 
  let `U(uo)` be the validator update caused by initiating `uo`; 
  let `vsc(uo)` be the VSC that contains `U(uo)`. 
  Then,
  - (*unbonding initiation*) the provider CCV module MUST be notified of `uo`'s initiation before receiving `U(uo)`;
  - (*unbonding completion*) `uo` MUST NOT complete on the provider chain before the provider chain registers notifications of `vsc(uo)`'s maturity from all consumer chains.
  > **Note**: Depending on the implementation, the (*unbonding initiation*) part of the *Unbonding Safety* MAY NOT be necessary for validator unbonding operations.

- ***Slashing Warranty***: If the provider ABCI application (e.g., the Slashing module) receives a request to slash a validator `val` that misbehaved at block height `h`, then it slashes the amount of tokens `val` had bonded at height `h` except the amount that has already completely unbonded. 

- ***Evidence Provision***: If the consumer ABCI application receives a valid evidence of misbehavior at block height `h`, then it MUST submit it to the consumer CCV module *exactly once* and at the same height `h`. 
  Furthermore, the consumer ABCI application MUST NOT submit invalid evidence to the consumer CCV module. 
  > **Note**: What constitutes a valid evidence of misbehavior depends on the type of misbehavior and it is outside the scope of this specification.

- ***Distribution Warranty***: The provider ABCI application (e.g., the Distribution module) distributes the tokens from the distribution module account among the validators that are part of the validator set.

## Desired Properties

The following properties are concerned with **one provider chain** providing security to **multiple consumer chains**. 
Between the provider chain and each consumer chain, a separate (unique) CCV channel is established. 

> **Note**: Except for liveness properties -- *Channel Liveness*, *Apply VSC Liveness*, *Register Maturity Liveness*, and *Distribution Liveness* -- none of the properties of CCV require the *Correct Relayer* assumption to hold.
> Nonetheless, the *Correct Relayer* assumption is necessary to guarantee the systems properties (except for *Validator Set Replication*) -- *Bond-Based Consumer Voting Power*, *Slashable Consumer Misbehavior*, and *Consumer Rewards Distribution*. 

### System Properties
[&uparrow; Back to Outline](#outline)

We use the following notations:
- `ts(h)` is the timestamp of a block with height `h`, i.e., `ts(h) = B.currentTimestamp()`, where `B` is the block at height `h`;
- `pBonded(h,val)` is the number of tokens bonded by validator `val` on the provider chain at block height `h`; 
- `pUnbonding(h,val)` is the number of tokens a validator `val` starts unbonding on the provider at at block height `h`;
- `VP(T)` is the voting power associated to a number `T` of tokens;
- `Power(c,h,val)` is the voting power granted to a validator `val` on a chain `c` at block height `h`;
- `Token(power)` is the amount of tokens necessary to be bonded (on the provider chain) by a validator to be granted `power` voting power, 
  i.e., `Token(VP(T)) = T`;
- `slash(val, h, hi, sf)` is the amount of token slashed from a validator `val` on the provider chain (i.e., `pc`) at height `h` for an infraction (with a slashing fraction of `sf`) committed at (provider) height `hi`, 
  i.e., `slash(val, h, hi, sf) = sf * Token(Power(pc,hi,val))`;
  note that the infraction can be committed also on a consumer chain, in which case `hi` is the corresponding height on the provider chain.

Also, we use `ha << hb` to denote an order relation between heights, i.e., the block at height `ha` *happens before* the block at height `hb`. 
For heights on the same chain, `<<` is equivalent to `<`, i.e., `ha << hb` entails `hb` is larger than `ha`.
For heights on two different chains, `<<` is establish by the packets sent over an order channel between two chains, 
i.e., if a chain `A` sends at height `ha` a packet to a chain `B` and `B` receives it at height `hb`, then `ha << hb`. 
> **Note**: `<<` is transitive, i.e., `ha << hb` and `hb << hc` entail `ha << hc`.
> 
> **Note**: The block on the proposer chain that handles a governance proposal to spawn a new consumer chain `cc` *happens before* all the blocks of `cc`.

CCV provides the following system properties.
- ***Validator Set Replication***: Every validator set on any consumer chain MUST either be or have been a validator set on the provider chain.

- ***Bond-Based Consumer Voting Power***: Let `val` be a validator, `cc` be a consumer chain, both `hc` and `hc'` be heights on `cc`, and both `hp` and `hp'` be heights on the provider chain, such that
  - `val` has `Power(cc,hc,val)` voting power on `cc` at height `hc`;
  - `hc'` is the smallest height on `cc` that satisfies `ts(hc') >= ts(hc) + UnbondingPeriod`, i.e., `val` cannot completely unbond on `cc` before `hc'`;   
  - `hp` is the largest height on the provider chain that satisfies `hp << hc`, i.e., `Power(pc,hp,val) = Power(cc,hc,val)`, where `pc` is the provider chain;
  - `hp'` is the smallest height on the provider chain that satisfies `hc' << hp'`, i.e., `val` cannot completely unbond on the provider chain before `hp'`;
  - `sumUnbonding(hp, h, val)` is the sum of all tokens of `val` that start unbonding on the provider at all heights `hu` and are still unbonding at height `h`, such that `hp < hu <= h`
  - `sumSlash(hp, h, val)` is the sum of the slashes of `val` at all heights `hs` for infractions committed at `hp`, such that `hp < hs <= h`.
  
  Then for all heights `h` on the provider chain, 
   ```
  hp <= h < hp': 
  Power(cc,hc,val) <= VP( pBonded(h,val) + sumUnbonding(hp, h, val) + sumSlash(hp, h, val) )
  ```

  > **Note**: The reason for `+ sumUnbonding(hp, h, val)` in the above inequality is that tokens that `val` start unbonding after `hp` have contributed to the power granted to `val` at height `hc` on `cc` (i.e., `Power(cc,hc,val)`). 
  > As a result, these tokens should be available for slashing until `hp'`.

  > **Note**: The reason for `+ sumSlash(hp, h, val)` in the above inequality is that slashing `val` reduces its locked tokens (i.e., `pBonded(h,val)` and `sumUnbonding(hp, h, val)`), however it does not reduce the power already granted to it at height `hc` on `cc` (i.e., `Power(cc,hc,val)`).

  > **Intuition**: The *Bond-Based Consumer Voting Power* property ensures that validators that validate on the consumer chains have enough tokens bonded on the provider chain for a sufficient amount of time such that the security model holds. 
  > This means that if the validators misbehave on the consumer chains, their tokens bonded on the provider chain can be slashed during the unbonding period.
  > For example, if one unit of voting power requires `1.000.000` bonded tokens (i.e., `VP(1.000.000)=1`), 
  > then a validator that gets one unit of voting power on a consumer chain must have at least `1.000.000` tokens bonded on the provider chain until the unbonding period elapses on the consumer chain.

  > **Note**: When an existing chain becomes a consumer chain (see [Channel Initialization: Existing Chains](overview_and_basic_concepts.md#channel-initialization-existing-chains)), the existing validator set is replaced by the provider validator set. 
  > For safety, the stake bonded by the existing validator set must remain bonded until the unbonding period elapses. 
  > Thus, the existing Staking module must be kept for at least the unbonding period. 

- ***Slashable Consumer Misbehavior***: If a validator `val` commits an infraction, with a slashing fraction of `sf`, on a consumer chain `cc` at a block height `hi`, 
  then any evidence of misbehavior that is received by `cc` at height `he`, such that `ts(he) < ts(hi) + UnbondingPeriod`, 
  MUST results in *exactly* the amount of tokens `sf*Token(Power(cc,hi,val))` to be slashed on the provider chain. 
  Furthermore, `val` MUST NOT be slashed more than once for the same misbehavior. 
  > **Note:** Unlike in single-chain validation, in CCV the tokens `sf*Token(Power(cc,hi,val))` MAY be slashed even if the evidence of misbehavior is received at height `he` such that `ts(he) >= ts(hi) + UnbondingPeriod`, 
  since unbonding operations need to reach maturity on both the provider and all the consumer chains.
  >
  > **Note:** The *Slashable Consumer Misbehavior* property also ensures that if a delegator starts unbonding an amount `x` of tokens from `val` before height `hi`, then `x` will not be slashed, since `x` is not part of `Token(Power(c,hi,val))`.

- ***Consumer Rewards Distribution***: If a consumer chain sends to the provider chain an amount `T` of tokens as reward for providing security, then
  - `T` (equivalent) tokens MUST be eventually minted on the provider chain and then distributed among the validators that are part of the validator set;
  - the total supply of tokens MUST be preserved, i.e., the `T` (original) tokens are escrowed on the consumer chain.

### CCV Channel
[&uparrow; Back to Outline](#outline)

- ***Channel Uniqueness***: The channel between the provider chain and a consumer chain MUST be unique.
- ***Channel Validity***: If a packet `P` is received by one end of a CCV channel, then `P` MUST have been sent by the other end of the channel.
- ***Channel Order***: If a packet `P1` is sent over a CCV channel before a packet `P2`, then `P2` MUST NOT be received by the other end of the channel before `P1`. 
- ***Channel Liveness***: Every packet sent over a CCV channel MUST eventually be received by the other end of the channel. 

### Validator Sets, Validator Updates and VSCs
[&uparrow; Back to Outline](#outline)

In this section, we provide a short discussion on how the validator set, the validator updates, and the VSCs relates in the context of multiple chains. 

Every chain consists of a sequence of blocks. 
At the end of each block, validator updates (i.e., changes in the validators voting power) results in changes in the validator set of the next block. 
Thus, the sequence of blocks produces a sequence of validator updates and a sequence of validator sets. 
Furthermore, the sequence of validator updates on the provider chain results in a sequence of VSCs to all consumer chains. 
Ideally, this sequence of VSCs is applied by every consumer chain, resulting in a sequence of validator sets identical to the one on the provider chain. 
However, in general this need not be the case. The reason is twofold: 
- first, given any two chains `A` and `B`, we cannot assume that `A`'s rate of adding new block is the same as `B`'s rate 
  (i.e., we consider the sequences of blocks of any two chains to be completely asynchronous); 
- and second, due to relaying delays, we cannot assume that the rate of sending VSCs matches the rate of receiving VSCs.

As a result, it is possible for multiple VSCs to be received by a consumer chain within the same block and be applied together at the end of the block, 
i.e., the validator updates within the VSCs are being *aggregated* by keeping only the latest update per validator. 
As a consequence, some validator sets on the provider chain are not existing on all consumer chains. 
In other words, the validator sets on each consumer chain form a *subsequence* of the validator sets on the provider chain. 
Nonetheless, as a **requirement of CCV**, *all the validator updates on the provider chain MUST be included in the sequence of validator sets on all consumer chains*.

This is possible since every validator update contains *the absolute voting power* of that validator. 
Given a validator `val`, the sequence of validator updates targeting `val` (i.e., updates of the voting power of `val`) is the prefix sum of the sequence of relative changes of the voting power of `val`. 
Thus, given a validator update `U` targeting `val` that occurs at a block height `h`, 
`U` *sums up* all the relative changes of the voting power of `val` that occur until height `h`, 
i.e., `U = c_1+c_2+...+c_i`, such that `c_i` is the last relative change that occurs by `h`. 
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

### Consumer Initiated Slashing
[&uparrow; Back to Outline](#outline)

- ***Consumer Slashing Warranty***: Let `cc` be a consumer chain, such that its CCV module receives at height `he` evidence that a validator `val` misbehaved on `cc` at height `hi`. 
  Let `hv` be the height when the CCV module of `cc` receives the first VSC from the provider CCV module, i.e., the height when the CCV channel is established.
  Then, the CCV module of `cc` MUST send (to the provider CCV module) *exactly one* `SlashPacket` `P`, such that
    - `P` is sent at height `h = max(he, hv)`;
    - `P.val = val` and `P.id = HtoVSC[hi]`,
    i.e., the ID of the latest VSC that updated the validator set on `cc` at height `hi` or `0` if such a VSC does not exist (if `hi < hv`).
  > **Note**: A consequence of the *Consumer Slashing Warranty* property is that the initial validator set on a consumer chain cannot be slashed during the initialization of the CCV channel. 
  Therefore, consumer chains *SHOULD NOT allow user transactions before the CCV channel is established*. 
  Note that once the CCV channel is established (i.e., a VSC is received from the provider CCV module), CCV enables the slashing of the initial validator set for infractions committed during channel initialization.

- ***Provider Slashing Warranty***: If the provider CCV module receives at height `hs` from a consumer chain `cc` a `SlashPacket` containing a validator `val` and a VSC ID `vscId`, 
  then it MUST make at height `hs` *exactly one* request to the provider Slashing module to slash `val` for misbehaving at height `h`, such that
  - if `vscId = 0`, `h` is the height of the block when the provider chain established a CCV channel to `cc`;
  - otherwise, `h` is the height of the block immediately subsequent to the block when the provider chain provided to `cc` the VSC with ID `vscId`.

- ***VSC Maturity and Slashing Order***: If a consumer chain sends to the provider chain a `SlashPacket` before a maturity notification of a VSC, then the provider chain MUST NOT receive the maturity notification before the `SlashPacket`.
  > **Note**: *VSC Maturity and Slashing Order* requires the VSC maturity notifications to be sent through their own IBC packets (i.e., `VSCMaturedPacket`s) instead of e.g., through acknowledgements of `VSCPacket`s. 

### Reward Distribution
[&uparrow; Back to Outline](#outline)

- ***Distribution Liveness***: If the CCV module on a consumer chain sends to the distribution module account on the provider chain an amount `T` of tokens as reward for providing security, then `T` (equivalent) tokens are eventually minted in the distribution module account on the provider chain.

## Correctness Reasoning
[&uparrow; Back to Outline](#outline)

In this section we argue the correctness of the CCV protocol described in the [Technical Specification](./technical_specification.md), 
i.e., we informally prove the properties described in the [previous section](#desired-properties).

- ***Channel Uniqueness***: The consumer chain side of the CCV channel is established when the consumer CCV module receives the _first_ `ChanOpenAck` message that is successfully executed; all subsequent `ChanOpenAck` messages will fail (cf. *Safe Blockchain*).
  Let `ccvChannel` denote this channel. Then, `ccvChannel` is the only `OPEN` channel that can be connected to a port owned by the consumer CCV module.
  The provider chain side of the CCV channel is established when the provider CCV module receives the _first_ `ChanOpenConfirm` message that is successfully executed; all subsequent `ChanOpenConfirm` messages will fail (cf. *Safe Blockchain*). 
  The `ccvChannel` is the only channel for which `ChanOpenConfirm` can be successfully executed (cf. *Safe Blockchain*, i.e., IBC channel opening handshake guarantee). 
  As a result, `ccvChannel` is unique. 
  Moreover, ts existence is guaranteed by the *Correct Relayer* assumption.

- ***Channel Validity***: Follows directly from the *Safe Blockchain* assumption.

- ***Channel Order***: The provider chain accepts only ordered channels when receiving a `ChanOpenTry` message (cf. *Safe Blockchain*). 
  Similarly, the consumer chain accepts only ordered channels when receiving `ChanOpenInit` messages (cf. *Safe Blockchain*). 
  Thus, the property follows directly from the fact that the CCV channel is ordered. 

- ***Channel Liveness***: The property follows from the *Correct Relayer* assumption. 

- ***Validator Update To VSC Validity***: The provider CCV module provides only VSCs that contain validator updates obtained from the Staking module, 
  i.e., by calling the `GetValidatorUpdates()` method (cf. *Safe Blockchain*). 
  Furthermore, these validator updates were applied to the validator set of the provider chain (cf. *Validator Update Provision*).

- ***Validator Update To VSC Order***: We prove the property through contradiction. 
  Given two validator updates `U1` and `U2`, with `U1` occurring on the provider chain before `U2`, we assume `U2` is included in a provided VSC before `U1`. 
  However, `U2` could not have been obtained by the provider CCV module before `U1` (cf. *Validator Update Provision*). 
  Thus, the provider CCV module could not have provided a VSC that contains `U2` before a VSC that contains `U1` (cf. *Safe Blockchain*), which contradicts the initial assumption.
  
- ***Validator Update To VSC Liveness***: The provider CCV module eventually provides to all consumer chains VSCs containing all validator updates obtained from the provider Staking module (cf. *Safe Blockchain*, *Life Blockchain*). 
  Thus, it is sufficient to prove that every update of a validator in the validator set of the provider chain MUST eventually be obtained from the provider Staking module. 
  We prove this through contradiction. Given a validator update `U` that is applied to the validator set of the provider chain at the end of a block `B` with height `h`, we assume `U` is never obtained by the provider CCV module. 
  However, at height `h`, the provider CCV module tries to obtain a new batch of validator updates from the provider Staking module (cf. *Safe Blockchain*). 
  Thus, this batch of validator updates MUST contain all validator updates applied to the validator set of the provider chain at the end of block `B`, including `U` (cf. *Validator Update Provision*), which contradicts the initial assumption.

- ***Apply VSC Validity***: The property follows from the following two assertions.
  - The consumer chain only applies VSCs received in `VSCPacket`s through the CCV channel (cf. *Safe Blockchain*).
  - The provider chain only sends `VSCPacket`s containing provided VSCs (cf. *Safe Blockchain*). 

- ***Apply VSC Order***: We prove the property through contradiction. 
  Given two VSCs `vsc1` and `vsc2` such that the provider chain provides `vsc1` before `vsc2`, we assume the consumer chain applies the validator updates included in `vsc2` before the validator updates included in `vsc1`. 
  The following sequence of assertions leads to a contradiction.
  - The provider chain could not have sent a `VSCPacket` `P2` containing `vsc2` before a `VSCPacket` `P1` containing `vsc1` (cf. *Safe Blockchain*).
  - The consumer chain could not have received `P2` before `P1` (cf. *Channel Order*).
  - Given the *Safe Blockchain* assumption, we distinguish two cases.
    - First, the consumer chain receives `P1` during block `B1` and `P2` during block `B2` (with `B1` < `B2`). 
    Then, it applies the validator updates included in `vsc1` at the end of `B1` and the validator updates included in `vsc2` at the end of `B2` (cf. *Validator Update Inclusion*), which contradicts the initial assumption. 
    - Second, the consumer chain receives both `P1` and `P2` during the same block. 
    Then, it applies the validator updates included in both `vsc1` and `vsc2` at the end of the block. 
    Thus, it could not have apply the validator updates included in `vsc2` before.

- ***Apply VSC Liveness***: The provider chain eventually sends over the CCV channel a `VSCPacket` containing `vsc` (cf. *Safe Blockchain*, *Life Blockchain*). 
  As a result, the consumer chain eventually receives this packet (cf. *Channel Liveness*). 
  Then, the consumer chain aggregates all received VSCs at the end of the block and applies all the aggregated updates (cf. *Safe Blockchain*, *Life Blockchain*). 
  As a result, the consumer chain applies all validator updates in `vsc` (cf. *Validator Update Inclusion*).

- ***Register Maturity Validity***: The property follows from the following sequence of assertions.
  - The provider chain only registers VSC maturity notifications when receiving on the CCV channel a `VSCMaturedPacket`s notifying the maturity of those VSCs (cf. *Safe Blockchain*). 
  - The provider chain receives on the CCV channel only packets sent by the consumer chain (cf. *Channel Validity*).
  - The consumer chain only sends `VSCMaturedPacket`s matching the `VSCPacket`s it receives on the CCV channel (cf. *Safe Blockchain*).
  - The consumer chain receives on the CCV channel only packets sent by the provider chain (cf. *Channel Validity*). 
  - The provider chain only sends `VSCPacket`s containing provided VSCs (cf. *Safe Blockchain*). 

- ***Register Maturity Timeliness***: We prove the property through contradiction. 
  Given a VSC `vsc` provided by the provider chain to the consumer chain, we assume that the provider chain registers a maturity notification of `vsc` before `UnbondingPeriod` has elapsed on the consumer chain since the consumer chain applied `vsc`. 
  The following sequence of assertions leads to a contradiction.
  - The provider chain could not have register a maturity notification of `vsc` before receiving on the CCV channel a `VSCMaturedPacket` `P` with `P.id = vsc.id` (cf. *Safe Blockchain*). 
  - The provider chain could not have received `P` on the CCV channel before the consumer chain sent it (cf. *Channel Validity*).
  - The consumer chain could not have sent `P` before at least `UnbondingPeriod` has elapsed since receiving a `VSCPacket` `P'` with `P'.id = P.id` on the CCV channel (cf. *Safe Blockchain*). 
  Note that since time is measured in terms of the block time, the time of receiving `P'` is the same as the time of applying `vsc`.
  - The consumer chain could not have received `P'` on the CCV channel before the provider chain sent it (cf. *Channel Validity*).  
  - The provider chain could not have sent `P'` before providing `vsc`. 
  - Since the duration of sending packets through the CCV channel cannot be negative, the provider chain could not have registered a maturity notification of `vsc` before `UnbondingPeriod` has elapsed on the consumer chain since the consumer chain applied `vsc`.

- ***Register Maturity Order***: We prove the property through contradiction. Given two VSCs `vsc1` and `vsc2` such that the provider chain provides `vsc1` before `vsc2`, we assume the provider chain registers the maturity notification of `vsc2` before the maturity notification of `vsc1`. 
  The following sequence of assertions leads to a contradiction.
  - The provider chain could not have sent a `VSCPacket` `P2`, with `P2.updates = C2`, before a `VSCPacket` `P1`, with `P1.updates = C1` (cf. *Safe Blockchain*).
  - The consumer chain could not have received `P2` before `P1` (cf. *Channel Order*).
  - The consumer chain could not have sent a `VSCMaturedPacket` `P2'`, with `P2'.id = P2.id`, before a `VSCMaturedPacket` `P1'`, with `P1'.id = P1.id` (cf. *Safe Blockchain*).
  - The provider chain could not have received `P2'` before `P1'` (cf. *Channel Order*).
  - The provider chain could not have registered the maturity notification of `vsc2` before the maturity notification of `vsc1` (cf. *Safe Blockchain*).

- ***Register Maturity Liveness***: The property follows from the following sequence of assertions.
  - The provider chain eventually sends on the CCV channel a `VSCPacket` `P`, with `P.updates = C` (cf. *Safe Blockchain*, *Life Blockchain*).
  - The consumer chain eventually receives `P` on the CCV channel (cf. *Channel Liveness*).
  - The consumer chain eventually sends on the CCV channel a `VSCMaturedPacket` `P'`, with `P'.id = P.id` (cf. *Safe Blockchain*, *Life Blockchain*).
  - The provider chain eventually receives `P'` on the CCV channel (cf. *Channel Liveness*).
  - The provider chain eventually registers the maturity notification of `vsc` (cf. *Safe Blockchain*, *Life Blockchain*).

- ***Consumer Slashing Warranty***: Follows directly from *Safe Blockchain*.

- ***Provider Slashing Warranty***: Follows directly from *Safe Blockchain*.

- ***VSC Maturity and Slashing Order***: Follows directly from *Channel Order*.

- ***Distribution Liveness***: The CCV module on the consumer chain sends to the provider chain an amount `T` of tokens through an IBC token transfer packet (as defined in [ICS 20](../ics-020-fungible-token-transfer/README.md)). 
  Thus, if the packet is relayed within the timeout period, then `T` (equivalent) tokens are minted on the provider chain. 
  Otherwise, the `T` tokens are refunded to the consumer CCV module account. 
  In this case, the `T` tokens will be part of the next token transfer packet.
  Eventually, a correct relayer will relay a token transfer packet containing the `T` tokens (cf. *Correct Relayer*, *Life Blockchain*). 
  As a result, `T` (equivalent) tokens are eventually minted on the provider chain.

- ***Validator Set Replication***: The property follows from the *Safe Blockchain* assumption and both the *Apply VSC Validity* and *Validator Update To VSC Validity* properties. 

- ***Bond-Based Consumer Voting Power***: The existence of `hp` is given by construction, i.e., the block on the proposer chain that handles a governance proposal to spawn a new consumer chain `cc` *happens before* all the blocks of `cc`. 
  The existence of `hc'` and `hp'` is given by *Life Blockchain* and *Channel Liveness*.

  To prove the *Bond-Based Consumer Voting Power* property, we use the following property that follows directly from the design of the protocol (cf. *Safe Blockchain*, *Life Blockchain*).
  - *Property1*: Let `val` be a validator; let `Ua` and `Ub` be two updates of `val` that are applied subsequently by a consumer chain `cc`, at block heights `ha` and `hb`, respectively (i.e., no other updates of `val` are applied in between). 
  Then, `Power(cc,ha,val) = Power(cc,h,val)`, for all block heights `h`, such that `ha <= h < hb` (i.e., the voting power granted to `val` on `cc` in the period between `ts(ha)` and `ts(hb)` is constant).  

  We prove the *Bond-Based Consumer Voting Power* property through contradiction.
  We assume there exist a height `h` on the provider chain between `hp` and `hp'` such that `Power(cc,hc,val) > VP( pBonded(h,val) + sumUnbonding(hp, h, val) + sumSlash(hp, h, val) )`.
  The following sequence of assertions leads to a contradiction.
  - Let `U1` be the latest update of `val` that is applied by `cc` before or not later than block height `hc` 
    (i.e., `U1` is the update that sets `Power(cc,hc,val)` for `val`). 
    Let `hp1` be the height at which `U1` occurs on the provider chain; let `hc1` be the height at which `U1` is applied on `cc`.
    Then, `hp1 << hc1 <= hc`, `hp1 <= hp`, and `Power(cc,hc,val) = Power(cc,hc1,val) = VP(pBonded(hp1,val))`.
    This means that some of the tokens bonded by `val` at height `hp1` were *completely* unbonded before or not later than height `hp'` (cf. `Power(cc,hc,val) > VP( pBonded(h,val) + sumUnbonding(hp, h, val) + sumSlash(hp, h, val) )`).
  - Let `uo` be the first such unbonding operation that is initiated on the provider chain at height `hp2`, such that `hp1 < hp2 <= hp'`. 
    Note that at height `hp2`, the tokens unbonded by `uo` are part of `pUnbonding(hp2,val)`.
    Let `U2` be the validator update caused by initiating `uo`.
    Let `hc2` be the height at which `U2` is applied on `cc`; clearly, `Power(cc,hc2,val) < Power(cc,hc,val)`.
    Note that the existence of `hc2` is ensured by *Validator Update To VSC Liveness* and *Apply VSC Liveness*.
    Then, `hc2 > hc1` (cf. `hp2 > hp1`, *Validator Update To VSC Order*, *Apply VSC Order*).
  - `Power(cc,hc,val) = Power(cc,hc1,val) = Power(cc,h,val)`, for all heights `h`, such that `hc1 <= h < hc2` (cf. *Property1*). 
    Thus, `hc2 > hc` (cf. `Power(cc,hc2,val) < Power(cc,hc,val)`).
  - `uo` cannot complete before `ts(hc2) + UnbondingPeriod`, which means it cannot complete before `hc'` and thus it cannot complete before `hp'` (cf. `hc' << hp'`). 

- ***Slashable Consumer Misbehavior***: The second part of the *Slashable Consumer Misbehavior* property (i.e., `val` is not slashed more than once for the same misbehavior) follows directly from *Evidence Provision*, *Channel Validity*, *Consumer Slashing Warranty*, *Provider Slashing Warranty*.
  To prove the first part of the *Slashable Consumer Misbehavior* property (i.e., exactly the amount of tokens `sf*Token(Power(cc,hi,val))` are slashed on the provider chain), we consider the following sequence of statements.
  - The CCV module of `cc` receives at height `he` the evidence that `val` misbehaved on `cc` at height `hi` (cf. *Evidence Provision*, *Safe Blockchain*, *Life Blockchain*).
  - Let `hv` be the height when the CCV module of `cc` receives the first VSC from the provider CCV module. 
  Then, the CCV module of `cc` sends at height `h = max(he, hv)` to the provider chain a `SlashPacket` `P`, such that `P.val = val` and `P.id = HtoVSC[hi]` (cf. *Consumer Slashing Warranty*).
  - The provider CCV module eventually receives `P` in a block `B` (cf. *Channel Liveness*). 
  - The provider CCV module requests (in the same block `B`) the provider Slashing module to slash `val` for misbehaving at height `hp = VSCtoH[P.id]` (cf. *Provider Slashing Warranty*).
  - The provider Slashing module slashes the amount of tokens `val` had bonded at height `hp` except the amount that has already completely unbonded (cf. *Slashing Warranty*).
  
  Thus, it remains to be proven that `Token(Power(cc,hi,val)) = pBonded(hp,val)`, with `hp = VSCtoH[HtoVSC[hi]]`. We distinguish two cases:
  - `HtoVSC[hi] != 0`, which means that by definition `HtoVSC[hi]` is the ID of the last VSC that update `Power(cc,hi,val)`. 
    Also by definition, this VSC contains the last updates to the voting power at height `VSCtoH[HtoVSC[hi]]` on the provider.
    Thus, `Token(Power(cc,hi,val)) = pBonded(hp,val)`. 
  - `HtoVSC[hi] == 0`, which means that by definition `Power(cc,hi,val)` was setup at genesis during Channel Initialization. 
    Also by definition, this is the same voting power of the provider chain block when the first VSC was provided to that consumer chain, i.e., `VSCtoH[HtoVSC[hi]]`. 
    Thus, `Token(Power(cc,hi,val)) = pBonded(hp,val)`. 

- ***Consumer Rewards Distribution***: The first part of the *Consumer Rewards Distribution* property (i.e., the tokens are eventually minted on the provider chain and then distributed among the validators) follows directly from *Distribution Liveness* and *Distribution Warranty*. 
  The second part of the *Consumer Rewards Distribution* property (i.e., the total supply of tokens is preserved) follows directly from the *Supply* property of the Fungible Token Transfer protocol (see [ICS 20](../ics-020-fungible-token-transfer/README.md)). 
