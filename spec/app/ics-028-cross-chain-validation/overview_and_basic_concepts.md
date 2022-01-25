<!-- omit in toc -->
# CCV: Overview and Basic Concepts
[&uparrow; Back to main document](./README.md)

<!-- omit in toc -->
## Outline
- [Security Model](#security-model)
- [Motivation](#motivation)
- [Definitions and Overview](#definitions-and-overview)
  - [Channel Initialization](#channel-initialization)
  - [Validator Set Update](#validator-set-update)



# Security Model
[&uparrow; Back to Outline](#outline)

We consider chains that reach consensus through a proof of stake mechanism based on the model of [weak subjectivity](https://blog.ethereum.org/2014/11/25/proof-stake-learned-love-weak-subjectivity/). 
The next block in a blockchain is *validated* and *voted* upon by a set of pre-determined *full nodes*; these pre-determined full nodes are also known as *validators*. 
We refer to the validators eligible to validate a block as that block's *validator set*. 
To be part of the validator set, a validator needs to *bond* (i.e., lock, stake) an amount of tokens for a (minimum) period of time, known as the *unbonding period*. 
The amount of tokens bonded gives a validator's *voting power*. 
If a validator misbehaves (e.g., validates two different blocks at the same height), its bonded tokens can be slashed. Note that the unbonding period enables the system to punish a misbehaving validator after the misbehavior is committed. 
For more details, take a look at the [Tendermint Specification](https://github.com/tendermint/spec/blob/master/spec/core/data_structures.md) and the [Light Client Specification](https://github.com/tendermint/spec/blob/master/spec/light-client/verification/verification_002_draft.md#part-i---tendermint-blockchain).

In the context of CCV, the validator sets of the consumer chains are chosen based on the tokens validators bonded on the provider chain, i.e., are chosen from the validator set of the provider chain. When these validators misbehave on the consumer chains, their bonded tokens on the provider chain are slashed. As a result, the security gained from the value of the bonded tokens on the provider chain is shared with the consumer chains. For more details, take a look at the [Interchain Security light paper](https://github.com/cosmos/gaia/blob/main/docs/interchain-security.md).

# Motivation
[&uparrow; Back to Outline](#outline)

CCV is a primitive (i.e., a building block) that enables arbitrary shared security models: The security of a chain can be composed of security transferred from multiple provider chains including the chain itself (a consumer chain can be its own provider). As a result, CCV enables chains to borrow security from more established chains (e.g., Cosmos Hub), in order to boost their own security, i.e., increase the cost of attacking their networks. 
> **Intuition**: For example, for chains based on Tendermint consensus, a variety of attacks against the network are possible if an attacker acquire 1/3+ or 2/3+ of all bonded tokens. Since the market cap of newly created chains could be relatively low, an attacker could realistically acquire sufficient tokens to pass these thresholds. As a solution, CCV allows the newly created chains to use validators that have stake on chains with a much larger market cap and, as a result, increase the cost an attacker would have to pay. 

Moreover, CCV enables *hub minimalism*. In a nutshell, hub minimalism entails keeping a hub in the Cosmos network (e.g., the Cosmos Hub) as simple as possible, with as few features as possible in order to decrease the attack surface. CCV enables moving distinct features (e.g., DEX) to independent chains that are validated by the same set of validators as the hub. 

> **Versioning**: Note that CCV will be developed progressively. 
> - The V1 release will require the validator set of a consumer chain to be entirely provided by the provider chain. In other words, once a provider chain agrees to provide security to a consumer chain, the entire validator set of the provider chain MUST validate also on the consumer chain.
> - The V2 release will allow the provider chain validators to opt-in to participate as validators on the consumer chain. It is up to each consumer chain to establish the benefits that provider chain validators receive for their participation.
> 
> For more details on the planned releases, take a look at the [Interchain Security light paper](https://github.com/cosmos/gaia/blob/main/docs/interchain-security.md#the-interchain-security-stack).

# Definitions and Overview
[&uparrow; Back to Outline](#outline)

This section defines the new terms and concepts introduced by CCV and provides an overview of CCV.

**Provider Chain**: The blockchain that provides security, i.e., manages the validator set of the consumer chain.

**Consumer Chain**: The blockchain that consumes security, i.e., enables the provider chain to manage its validator set.

> Note that in the current version the validator set of the consumer chain is entirely provided by the provider chain.

**CCV Module**: The module that implements the CCV protocol. Both the provider and the consumer chains have each their own CCV module. Furthermore, the functionalities provided by the CCV module differ between the provider chain and the consumer chain. For brevity, we use *provider CCV module* and *consumer CCV module* to refer to the CCV modules on the provider chain and on the consumer chain, respectively. 

**CCV Channel**: A unique, ordered IBC channel (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)) that is used by the two CCV modules to exchange IBC packets (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)).

> Note that the IBC handler interface & IBC relayer module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

**Validator Set Change (VSC)**: A change in the validator set of the provider chain that must be reflected in the validator set of the consumer chain. A VSC consists of a batch of validator updates, i.e., changes in the voting power granted to validators on the provider chain and, due to CCV, also on the consumer chain.

> **Background**: In the context of single-chain validation, the changes of the validator set are triggered by the Staking module. For more details, take a look at the [Cosmos SDK documentation](https://docs.cosmos.network/master/modules/staking/). 

**Matured VSC**: A VSC that has matured on the consumer chain, i.e., a certain period of time, known as the *unbonding period* (i.e., `UnbondingPeriod`) has elapsed since the VSC was applied by the consumer chain. 

> **Note**: Time periods are measured in terms of the block time, i.e., `currentTimestamp()` (as defined in [ICS 24](../../core/ics-024-host-requirements)). As a result, the consumer chain MAY start the unbonding period for every VSC that it applies in a block at any point during that block.

> **Intuition**: Every VSC consists of a batch of validator updates, some of which can be decreases in the voting power granted to validators. These decreases may be a consequence of unbonding operations on the provider chain, which MUST NOT complete before reaching maturity on both the provider and all the consumer chains. Thus, a VSC reaching maturity on a consumer chain means that all the unbonding operations that resulted in validator updates included in that VSC have matured on the consumer chain.

> **Background**: An *unbonding operation* is any operation of unbonding an amount of the tokens a validator bonded. Note that the bonded tokens correspond to the validator's voting power. Unbonding operations have two components: 
> - The *initiation*, e.g., a delegator requests their delegated tokens to be unbonded. The initiation of an operation of unbonding an amount of the tokens a validator bonded results in a change in the voting power of that validator.
> - The *completion*, e.g., the tokens are actually unbonded and transferred back to the delegator. To complete, unbonding operations must reach *maturity*, i.e., `UnbondingPeriod` must elapse since the operations were initiated.

CCV must handle the following types of operations:
- **Channel Initialization**: Create a unique, ordered IBC channel between the provider chain and the consumer chain.
- **Validator Set Update**: It is a two-part operation, i.e., 
  - update the validator set of the consumer chain based on the information obtained from the *provider Staking module* (i.e., the Staking module on the provider chain) on the amount of tokens bonded by validators on the provider chain;
  - and enable the timely completion (cf. the unbonding periods on the consumer chains) of unbonding operations (i.e., operations of unbonding bonded tokens).

## Channel Initialization
[&uparrow; Back to Outline](#outline)

The following Figure shows an overview of the CCV Channel initialization. 

![Channel Initialization Overview](./figures/ccv-init-overview.png?raw=true)

Consumer chains are created through governance proposals. For details on how governance proposals work, take a look at the [Cosmos SDK documentation](https://docs.cosmos.network/master/modules/gov/).

The channel initialization consists of four phases:
- **Create clients**: The provider CCV module handles every passed proposal to spawn a new consumer chain. Once it receives a proposal, it creates a client of the consumer chain (as defined in [ICS 2](../../core/ics-002-client-semantics)). 
  Then, the operators of validators in the validator set of the provider chain must each start a full node (i.e., a validator) of the consumer chain. 
  Once the consumer chain starts, the `InitGenesis()` method of the consumer CCV module is invoked and a client of the provider chain is created (for more details on `InitGenesis()`, take a look at the [Cosmos SDK documentation](https://docs.cosmos.network/master/building-modules/genesis.html)). 
  For client creation, both a `ClientState` and a `ConsensusState` are necessary (as defined in [ICS 2](../../core/ics-002-client-semantics)); both are contained in the `GenesisState` of the consumer CCV module. 
  This `GenesisState` is distributed to all operators that need to start a full node of the consumer chain (the mechanism of distributing the `GenesisState` is outside the scope of this specification).
  > Note that although the mechanism of distributing the `GenesisState` is outside the scope of this specification, a possible approach would entail the creator of the proposal to spawn the new consumer chain to distribute the `GenesisState` via the gossip network. 
  >  
  > Note that at genesis, the validator set of the consumer chain matches the validator set of the provider chain.
- **Connection handshake**: A relayer is responsible for initiating the connection handshake (as defined in [ICS 3](../../core/ics-003-connection-semantics)). 
- **Channel handshake**: A relayer is responsible for initiating the channel handshake (as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics)). The channel handshake must be initiated on the consumer chain. The handshake consists of four messages that need to be received for a channel built on top of the expected clients. We omit the `ChanOpenAck` message since it is not relevant for the overview. 
  - *OnChanOpenInit*: On receiving the *FIRST* `ChanOpenInit` message, the consumer CCV module sets the status of its end of the CCV channel to `INITIALIZING`.
  - *OnChanOpenTry*: On receiving the *FIRST* `ChanOpenTry` message, the provider CCV module sets the status of its end of the CCV channel to `INITIALIZING`.
  - *OnChanOpenConfirm*: On receiving the *FIRST* `ChanOpenConfirm` message, the provider CCV module sets the status of its end of the CCV channel to `VALIDATING`.
- **Channel completion**: Once the provider chain sets the status of the CCV channel to `VALIDATING`, it provides a VSC (i.e., validator set change) to the consumer chain (see [next section](#validator-set-update)). On receiving the *FIRST* `VSCPacket`, the consumer CCV module sets the status of its end of the CCV channel to `VALIDATING`. 

Note that the "*FIRST*" keyword in the above description ensures the uniqueness of the IBC channel.

## Validator Set Update
[&uparrow; Back to Outline](#outline)

In the context of VSCs, the CCV module enables the following functionalities:
  - On the provider chain, 
    - **provide** VSCs to the consumer chain, for it to update its validators set according to the validator set of the provider chain;
    - **register** VSC maturity notifications from the consumer chain.
  - On the consumer chain,
    - **apply** the VSCs provided by the provider chain to the validator set of the consumer chain; 
    - **notify** the provider chain that the provided VSCs have matured.

These functionalities are depicted in the following Figure that shows an overview of the Validator Set Update operation of CCV. 

![Validator Set Update Overview](./figures/ccv-vsc-overview.png?raw=true)