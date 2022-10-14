<!-- omit in toc -->
# CCV: Technical Specification
[&uparrow; Back to main document](./README.md)

<!-- omit in toc -->
## Outline
- [Placing CCV within an ABCI Application](#placing-ccv-within-an-abci-application)
  - [Implemented Interfaces](#implemented-interfaces)
  - [Interfacing Other Modules](#interfacing-other-modules)
- [Data Structures and Methods](#data-structures-and-methods)

## Placing CCV within an ABCI Application
[&uparrow; Back to Outline](#outline)

Before describing the data structures and sub-protocols of the CCV protocol, we provide a short overview of the interfaces the CCV module implements and the interactions with the other ABCI application modules.

### Implemented Interfaces

- CCV is an **ABCI application module**, which means it MUST implement the logic to handle some of the messages received from the consensus engine via ABCI, 
  e.g., `InitChain`, `BeginBlock`, `EndBlock` (for more details, take a look at the [ABCI specification](https://github.com/tendermint/spec/tree/v0.7.1/spec/abci)). 
  In this specification we define the following methods that handle messages that are of particular interest to the CCV protocol:
  - `InitGenesis()` -- Called when the chain is first started, on receiving an `InitChain` message from the consensus engine. 
    This is also where the application can inform the underlying consensus engine of the initial validator set.
  - `BeginBlock()` -- Contains logic that is automatically triggered at the beginning of each block. 
  - `EndBlock()` -- Contains logic that is automatically triggered at the end of each block. 
    This is also where the application can inform the underlying consensus engine of changes in the validator set.

- CCV is an **IBC module**, which means it MUST implement the module callbacks interface defined in [ICS 26](../../core/ics-026-routing-module/README.md#module-callback-interface). The interface consists of a set of callbacks for 
  - channel opening handshake, which we describe in the [Initialization](./methods.md#initialization) section;
  - channel closing handshake, which we describe in the [Consumer Chain Removal](./methods.md#consumer-chain-removal) section;
  - and packet relay, which we describe in the [Packet Relay](./methods.md#packet-relay) section.

### Interfacing Other Modules

- As an ABCI application module, the CCV module interacts with the underlying consensus engine through ABCI:
  - On the provider chain,
    - it initializes the application (e.g., binds to the expected IBC port) in the `InitGenesis()` method.
  - On the consumer chain,
    - it initializes the application (e.g., binds to the expected IBC port, creates a client of the provider chain) in the `InitGenesis()` method;
    - it provides the validator updates in the `EndBlock()` method.

- As an IBC module, the CCV module interacts with Core IBC for functionalities regarding
  - port allocation ([ICS 5](../../core/ics-005-port-allocation)) via `portKeeper`;
  - channels and packet semantics ([ICS 4](../../core/ics-004-channel-and-packet-semantics)) via `channelKeeper`;
  - connection semantics ([ICS 3](../../core/ics-003-connection-semantics)) via `connectionKeeper`;
  - client semantics ([ICS 2](../../core/ics-002-client-semantics)) via `clientKeeper`.

- The consumer CCV module interacts with the IBC Token Transfer module ([ICS 20](../ics-020-fungible-token-transfer/README.md)) via `transferKeeper`.

- For the [Initialization sub-protocol](#initialization), the provider CCV module interacts with a Governance module by handling governance proposals to add new consumer chains. 
  If such proposals pass, then all validators on the provider chain MUST validate the consumer chain at spawn time; 
  otherwise they get slashed. 
  For an example of how governance proposals work, take a look at the [Governance module documentation](https://docs.cosmos.network/v0.45/modules/gov/) of Cosmos SDK.

- The consumer pre-CCV module (i.e., the CCV module with `preCCV == true`) interacts with a Staking module on the consumer chain. 
  Note that once `preCCV` is set to `false`, the Staking module MUST no longer provide validator updates to the underlying consensus engine. 
  For an example of how staking works, take a look at the [Staking module documentation](https://docs.cosmos.network/v0.45/modules/staking/) of Cosmos SDK. 
  The interaction is defined by the following interface:
  ```typescript 
  interface StakingKeeper {
    // replace the validator set with valset
    ReplaceValset(valset: [ValidatorUpdate])
  }
  ```

- The provider CCV module interacts with a Staking module on the provider chain. 
  For an example of how staking works, take a look at the [Staking module documentation](https://docs.cosmos.network/v0.45/modules/staking/) of Cosmos SDK. 
  The interaction is defined by the following interface:
  ```typescript 
  interface StakingKeeper {
    // get UnbondingPeriod from the provider Staking module 
    UnbondingTime(): Duration

    // get validator updates from the provider Staking module
    GetValidatorUpdates(): [ValidatorUpdate]

    // request the Staking module to put on hold 
    // the completion of an unbonding operation
    PutUnbondingOnHold(id: uint64)

    // notify the Staking module of an unboding operation that
    // has matured from the perspective of the consumer chains 
    UnbondingCanComplete(id: uint64)
  }
  ```

- The provider CCV module interacts with a Slashing module on the provider chain. 
  For an example of how slashing works, take a look at the [Slashing module documentation](https://docs.cosmos.network/v0.45/modules/slashing/) of Cosmos SDK. 
  The interaction is defined by the following interface:
  ```typescript 
  interface SlashingKeeper {
    // query the Slashing module for the slashing factor, 
    // which may be different for downtime infractions
    GetSlashFactor(downtime: Bool): int64

    // request the Slashing module to slash a validator
    Slash(valAddress: string, 
          infractionHeight: int64, 
          power: int64, 
          slashFactor: int64)

    // query the Slashing module for the jailing time, 
    // which may be different for downtime infractions
    GetJailTime(downtime: Bool): int64

    // request the Slashing module to jail a validator until time
    JailUntil(valAddress: string, time: uint64)
  }
  ``` 

- The following hook enables the provider CCV module to register operations to be execute when certain events occur within the provider Staking module:
  ```typescript
  // invoked by the Staking module after 
  // initiating an unbonding operation
  function AfterUnbondingInitiated(opId: uint64);
  ```

- The consumer CCV module defines the following hooks that enable other modules to register operations to execute when certain events have occurred within CCV:
  ```typescript
  // invoked after a new validator is added to the validator set
  function AfterCCValidatorBonded(valAddress: string);

  // invoked after a validator is removed from the validator set
  function AfterCCValidatorBeginUnbonding(valAddress: string);
  ```

## Data Structures and Methods
[&uparrow; Back to Outline](#outline)

The remainder of this technical specification is split into [Data Structures](./data_structures.md) and [Methods](./methods.md).

