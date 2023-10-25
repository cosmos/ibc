# Channel Upgradability Finite State Machines 

This document is an attempt to abstract the [channel upgradability specs](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md) into finite state machines (FSMs). 

# Channel Upgradability Protocol 
According to the [specs](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md#upgrade-handshake), we can model the channel upgradability protocol with 2 main flow namely, `UpgradeOk` and `UpgradeNotOk`. `UpgradeOk` can be expanded in 2 subflows namely `UpgradeOkCrossingHello` and `UpgradeOkNotCrossingHello`. `UpgradeNotOk` can be further expanded in 3 subflows namely `UpgradeCanceled`,`UpgradeExpired`, `UpgradeStaled`.  

- `UpgradeOk`
    - `UpgradeOkNotCrossingHello`
    - `UpgradeOkCrossingHello`
- `UpgradeNotOk`
    - `UpgradeCanceled`
    - `UpgradeExpired` 
    - `UpgradeStaled`

We now procede with the abstraction process of every flow identified. 

## UpgradeOkNotCrossingHello
In this section we describe the happy path of the channel upgradability protocol, the `UpgradeOkNotCrossingHello`.   

### FSM High Level Representation

[UpgradeOkNotCrossingHello High Level Representation](https://excalidraw.com/#json=dU82X90B_i7qlEAdFY00R,T_dyyFxOX32glPaWvch-cg)

![Picture](img_fsm/UpgradeOkNotCrossingHello_HighLevel.png){width=100%}

## Formalization of the specs - WIP 

The content of this section may be completely modified. I'm trying to understand how to better defines all the conditions and invariants needed for every state transition. 

### Admitted Flow
Here we list all the possible flows. 

1. `S0 -> S1 -> S2 -> S3_1 -> S4 -> S5_1 ->S6`  
2. `S0 -> S1 -> S2 -> S3_2 -> S5_1 ->S6`
3. `S0 -> S1 -> S2 -> S3_2 -> S5_2 ->S6`

### States Description

**State Table**

| StateID| Description                                                  |
|--------|--------------------------------------------------------------|
| S0    | A:OPEN; B:OPEN :: The channel is ready to be upgraded        |
| S1    | A:OPEN; B:OPEN :: Chain A has started the process            |
| S2    | A:OPEN; B:FLUSHING            |
| S3_1  | A:FLUSHING; B:FLUSHING        |
| S3_2  | A:FLUSHING_COMPLETE; B:FLUSHING |
| S4    | A:FLUSHING; B:FLUSHING_COMPLETE |
| S5_1  | A:FLUSHING_COMPLETE; B:FLUSHING_COMPLETE                  |
| S5_2  | A:FLUSHING_COMPLETE; B:OPEN                  |
| S6    | A:OPEN; B:OPEN                  |

**F:Admitted State Transition**:
- S0->S1 
- S1->S2
- S2->S3_1 
- S2->S3_2
- S3_1->S4
- S3_2->S5_1
- S3_2->S5_2
- S5_1->S6
- S5_2->S6

### Definition Description
We have a working channel `Chan`. The channel works on top of an established connection between ChainA and ChainB and has two ends. We will call `ChanA` and `ChanB` the ends of the channel of the connected chains. 

For both chains we have a provable store `PS`. We define `PSa` and `PSb` as the ChainA and ChainB provable store.  

For the upgradability protocol the [`Upgrade`](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md#upgrade) type, that represent a particular upgrade attempt on a channel hand, has been introduced. We will call `Upg` the upgrade parameters store in `Chan` and `UpgA` and `UpgB` the upgrade parameters at both `ChanA` and `ChanB` ends

We call infligh packets `InP` the packets that have been sent before an upgrade starts. `InP` needs to be cleared out for the succesfull execution of the channel upgrade protocol.  


**Actors**: 
- Chain A: A  
- Chain B: B
- Relayer : R
- Relayer for A: R:A
- Relayer for B: R:B

**Definition**: 
- Chan: Channel :: Chan.State, Chan.UpgradeFields, Chan.ChannelID, Chan.UpgradeSequence. 
- Chan.State ∈ (OPEN, FLUSHING, FLUSHING_COMPLETE)
- ChanA: Cannel State on Chain A.  
- ChanB: Cannel State on Chain B.  
- Upg: Upgrade type :: Upg.UpgradeFields, Upg.UpgradeTimeout, Upg.lastPacketSent. 
- UpgA: Upgrade type on ChainA.
- UpgB: Upgrade type on ChainB.
- PS: ProvableStore.
- PSa : ProvableStore on ChainA.
- PSb : ProvableStore on ChainB. 

### Conditions

Notes: 
- For now conditions may be duplicated. This needs other pass to be cleared out
- Need to review all conditions to ensure nothing is missing 
- Need to understand if there is a better way to express conditions 

**C:Conditions**: 
- C0 = Chan.State === ChanA.State === OPEN === ChanB.State
- C1 = Chan.ChannelID === CONSTANT 
- C2 = isAuthorizedUpgrader() === True
- C3 = ChanA.UpgradeSequence.isIncremented() === True
- C4 = UpgA.UpgradeFields.areSet() === True
- C5 = (ProposedConnection.State===OPEN) && (ProposedConnection !== null)
- C6 = isSupported(UpgA.UpgradeFields.ordering) === True
- C7 = UpgA.isStored(PSa) === True
- C8 = ChanA.UpgradeSequence.isStored(PSa) === True
- C9 = UpgB.isStored(PSb) !== True 
- C10 = ChanB.UpgradeSequence === ChanA.UpgradeSequence
- C11 = VerifyChanA.State() === True 
- C12 = VerifyChanA.Upgrade === True 
- C13 = (Chan.UpgradeTimeout != 0) || (Chan.UpgradeTimestamp != 0) 
- C13 = UpgA.lastPacketSequence.isSet() === True
- C14 = UpgA.isStored(PSa) === True 
- C15 = ChanB.State.isSet(FLUSHING) === True 
- C16 = ChanB.isStored(PSb) === True 
- C17 = (ChanB.State === FLUSHING) && (ChanA.State === OPEN) 
- C18 = VerifyChanB.State() === True 
- C19 = VerifyChanB.Upgrade === True 
- C20 = UpgB.UpgradeFields === UpgA.UpgradeFields 
- C21 = UpgB.Timeout.isExpired() !== True
- C22 = InP.exist()===True
- C23 = InP.exist()!==True 
- C24 = ChanA.State.isSet(FLUSHING) === True 
- C25 = ChanA.isStored(PSa) === True 
- C26 = ChanA.State.isSet(FLUSHING_COMPLETE) === True 
- C27 = UpgA.Timeout.isExpired() !== True
- C28 = (ChanB.State === FLUSHING) && (ChanA.State === FLUSHING)
- C29 = ChanB.State.isSet(FLUSHING_COMPLETE) === True 
- C30 = ChanB.isStored(PSb) === True 
- C31 = (ChanA.State === FLUSHING_COMPLETE) && (ChanB.State === FLUSHING)
- C32 = ChanB.State.isSet(OPEN) === True 
- C33 = (ChanA.State === FLUSHING_COMPLETE) && (ChanB.State === FLUSHING_COMPLETE)
- C34 = (ChanA.State === FLUSHING_COMPLETE) && (ChanB.State === OPEN)


|Initiator| Q | Q'   | C (Conditions)                      | Σ (Input Symbols)  | 
|---------|---|------|-------------------------------------|--------------------|
|A        | S0| S1   | C0;C1;C2;C3;C4;C5;C6;C7;C8          | ChanUpgradeInit    |
|R:B      | S1| S2   | C0;C1;C9;C10;C11;C12;C13;C14;C15;C16| ChanUpgradeTry     | 
|R:A      | S2| S3_1 | C1;C17;C18;C19;C20;C21;C22;C24;C25  | ChanUpgradeAck     |
|R:A      | S2| S3_2 | C1;C17;C18;C19;C20;C21;C23;C25;C26  | ChanUpgradeAck     |
|R:B      | S3_1|S4  | C27;C28;C11;C12;C23;C29;C30         | ChanUpgradeConfirm |
|A        | S4|S5_1  | C21;C23;C26;C25                     | ?                  |
|R:B      | S3_2|S5_1| C27;C31;C23;C11;C29;C30             | ChanUpgradeConfirm |
|R:B      | S3_2|S5_2| C27;C31;C23;C11;C32;C30             | ChanUpgradeConfirm |
|R:A:B    | S5_1|S6  | C21;C27;C33                         | ChanUpgradeOpen    |
|R:A      | S5_2|S6  | C21;C27;C34                         | ChanUpgradeOpen    |

DRAW FSM INCLUDING CONDITIONS 


## Upgrade Handshake - UpgradeNotOk

### Upgrade Handshake - UpgradeCanceled

### Upgrade Handshake - UpgradeExpired

### Upgrade Handshake - UpgradeStaled



# Personal Notes 

<details>
  <summary>Click to expand!</summary>

The content below is not to be considered.


## Finite State Machine Modeling 

We consider a deterministic finite state machine as a 5-tuple (Q; C; Σ; δ; F) consisting of: 
- a finite set of States Q
- a finite set of Invariant Conditions C
- a finite set of Accepted Inputs Σ
- a finite set of Accepted States Transition F
- a finite set of Transition Functions δ

Where 

**Q**: 
| State  | Liveness Time | Description     |
|-------|---------------|-----------------|
| S0    |        t0     | The channel is ready to be upgraded |
| S1    |        t1     | Chain A has started the process  |
| S2    |        t1     | Chain B Goes from OPEN to FLUSHING |
| S3_1  |        t1     | Chain A Goes from OPEN to FLUSHING  |
| S3_2  |        t1     | Chain A Goes from OPEN to FLUSHING COMPLETE |
| S4    |        t1     | Chain A and Chain B are in FLUSHING COMPLETE |
| S5    |        t2     | Channel Updated |

**Σ**:
- ChanlUpgradeInit --> isAuthorizedUpgrader; initUpgradeHandshake :: Modify Upg.UpgradeFields 
- ChanlUpgradeTry
- ChanlUpgradeAck
- ChanlUpgradeConfirm
- ChanlUpgradeOpen

- initUpgradeHandshake
- isCompatibleUpgradeFields
- startFlushUpgradeHandshake
- openUpgradeHandshake
- pendingInflightPackets
- isAuthorizedUpgrader

**F**:
- S0->S1 
- S1->S2
- S1->S1
- S2->S3_1 
- S2->S3_2
- S3_1->S4
- S3_2->S4
- S4->S5

**C**: 
- C0 = Chan.State === ChanA.t0.State === OPEN === ChanB.t0.State
- C1 = ChanA.t0.ChannelID === ChanA.t1.ChannelID === ChanA.t2.ChannelID === ChanB.t0.ChannelID === ChanB.t1.ChannelID === ChanB.t2.ChannelID
- C2 = Chan.t2.UpgradeFields === (UpgA.t2.UpgradeFields === UpgB.t2.UpgradeFields) ||  Chan.t0.UpgradeFields
- C3 = InP.processed(Chan.t0.UpgradeFields)
- C4 = UpgA.t2.UpgradeTimeout.notExpired === UpgB.t2.UpgradeTimeout.notExpired
- C5 = (Upg.UpgradeTimeout.timeoutHeight || Upg.UpgradeTimeout.timeoutTimestamp) != 0
- C6 = UpgA.t1.isStored(PSa) 
- C7 = isAuthorizedUpgrader === True 
- C8 = Chan.t1.UpgradeSequence === Chan.t0.UpgradeSequence+1 
- C9 = Chan.t1.UpgradeFields.UpgradeVersion != Null
- C10 = Chan.t1.UpgradeFields.ProposedConnection != Null && Chan.t1.UpgradeFields.ProposedConnection === OPEN 
- C11 = initUpgradeHandshake has been executed. 
- C12 = Upg.isNotSet 
- C13 = Upg.isSet 
- C14 = Upg.Ordering.isSupported
- C15 = Chan.isStored(PS)

**δ**: 
We define δ : C * Σ * Q = Q'  


```typescript
// initUpgradeHandshake will verify that the channel is in the
// correct precondition to call the initUpgradeHandshake protocol.
// it will verify the new upgrade field parameters, and make the
// relevant state changes for initializing a new upgrade:
// - store channel upgrade
// - incrementing upgrade sequence
function initUpgradeHandshake(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proposedUpgradeFields: UpgradeFields,
): uint64 {
  // current channel must be OPEN
  // If channel already has an upgrade but isn't in FLUSHING,
  // then this will override the previous upgrade attempt
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state == OPEN)

  // new channel version must be nonempty
  abortTransactionUnless(proposedUpgradeFields.Version !== "")

  // proposedConnection must exist and be in OPEN state for 
  // channel upgrade to be accepted
  proposedConnection = provableStore.get(connectionPath(proposedUpgradeFields.connectionHops[0])
  abortTransactionUnless(proposedConnection !== null && proposedConnection.state === OPEN)

  // new order must be supported by the new connection
  abortTransactionUnless(isSupported(proposedConnection, proposedUpgradeFields.ordering))

  // lastPacketSent and timeout will be filled when we move to FLUSHING
  upgrade = Upgrade{
    fields: proposedUpgradeFields,
  }

  // store upgrade in provable store for counterparty proof verification
  provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)

  channel.upgradeSequence = channel.upgradeSequence + 1
  provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
  return channel.upgradeSequence
}
```
```typescript
function chanUpgradeInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proposedUpgradeFields: UpgradeFields,
  msgSender: string,
) {
  // chanUpgradeInit may only be called by addresses authorized by executing chain
  abortTransactionUnless(isAuthorizedUpgrader(msgSender))

  upgradeSequence = initUpgradeHandshake(portIdentifier, channelIdentifier, proposedUpgradeFields)

  // call modules onChanUpgradeInit callback
  module = lookupModule(portIdentifier)
  version, err = module.onChanUpgradeInit(
    portIdentifier,
    channelIdentifier,
    upgradeSequence,
    proposedUpgradeFields.ordering,
    proposedUpgradeFields.connectionHops,
    proposedUpgradeFields.version
  )
  // abort transaction if callback returned error
  abortTransactionUnless(err === nil)

  // replace channel upgrade version with the version returned by application
  // in case it was modified
  upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
  upgrade.fields.version = version
  provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

For every of the defined structure we will have `UpgA.t0`,`UpgB.t0` the upgrade parameters at t0 (e.g. starting of the upgrade) and `UpgA.t1`,`UpgB.t1` the upgrade parameters at t1 (e.g proposed upgrade) and `UpgA.t2`,`UpgB.t2` the upgrade parameters at t2 (e.g finalized upgrade). This is valid for Chan too. 



**UpgradeOkNotCrossingHello Table** : describes what is used within the scope of the UpgradeOk flow. 

| Category             | Item                         | Used in UpgradeOk |
|----------------------|------------------------------|:-----------------:|
| **Datagrams**        | *ChanUpgradeInit*            |         1         |
|                      | *ChanUpgradeTry*             |         1         |
|                      | *ChanUpgradeAck*             |         1         |
|                      | *ChanUpgradeConfirm*         |         1         |
|                      | *ChanUpgradeOpen*            |         1         |
|                      | *ChanUpgradeTimeout*         |                   |
|                      | *ChanUpgradeCancel*          |                   |
| **Utility Functions**| `initUpgradeHandshake`       |         1         |
|                      | `isCompatibleUpgradeFields`  |         1         |
|                      | `startFlushUpgradeHandshake` |         1         |
|                      | `openUpgradeHandshake`       |         1         |
|                      | `restoreChannel`             |                   |
|                      | `pendingInflightPackets`     |         1         |
|                      | `isAuthorizedUpgrader`       |         1         |
|                      | `getUpgradeTimeout`          |                   |
| **Functions**        | ChanlUpgradeInit             |         1         |
|                      | ChanlUpgradeTry              |         1         |
|                      | ChanlUpgradeAck              |         1         |
|                      | ChanlUpgradeConfirm          |         1         |
|                      | ChanlUpgradeOpen             |         1         |
|                      | CancelChannelUpgrade         |                   |
|                      | TimeoutChannelUpgrade        |                   |


**Sets**
- Q=
- C= {C0=(Ca && Cb on CUPo || Ca && Cb on CUPn); C1=(CI before == CI after); C2=Ca/Cb has stored Cx/Cy change in PSa/PSb; C3=InP; C4= !InP; C5= Ca/Cb has stored Timeout in PSa/PSb; C6 = Ca/Cb has stored ErrorMessage in PSa/PSb}
- δ = {`initUpgradeHandshake`, `StartFlushingUpgradeHandshake`, `openUpgradeHandshake`, `cancelUpgradeHandshake` , `timeoutChannelUpgrade`. 
}

- Σ = {Set Cx/Cy; Set Cx/Cy/CPU in PSa/PSb; Set Timeout in PSa/PSb; Ca/Cb set ErrorMessage in PSa/PSb; Ca/Cb send `ChannelUpgradeInit` datagram; }

**Final States Description**
| State | Description | PostConditions| 
|-------|-------------|--|
|   5_1 | Succesful| Ca && Cb are on CUPn |
|   5_2 | Unsuccesful| Ca && Cb are on CUPo | 


**Condition Table**
| Condition ID | Condition |  
|-------|-------------|
|C0|(Ca && Cb on CUPo || Ca && Cb on CUPn)|
|C1|CI before == CI after|
|C2|Ca/Cb has stored CUPn in PSa/PSb|
|C3|Ca/Cb has stored Cx/Cy change in PSa/PSb|
|C4|InP|
|C5|!InP|
|C6|Ca/Cb has stored Timeout in PSa/PSb|
|C7|Timeout Expired
|C8|Timeout Not Expired 
|C9|Ca/Cb has stored ErrorMessage in PSa/PSb|


Given the 
- List of State Transition Pairs `STp`={`1->2`; `2->3`; `2->4`; `3->5.a(1)`; `3->5.a(2)`; `3->4`; `4->5.b`; `4->5.c`}

**State Transition Table**
| State Transition Pair | Condition To Hold | State Transition Function | Inputs | Codebase Location | 
|-------|-------------|----------|---------|---|
|S0->S1|PC0;C0;C1|Ca:`initUpgradeHandshake`|Ca sets CPUn in PSa| [codebase_location](TBD)|
|S1->S2|PC0;C1;CaC2;CbC2;CbC3|Cb:`startFlushingUpgradeHandshake`|Cb sets Cy change (OPEN -> FLUSHING) in PSb; Cb sets Timeout in PSb; Cb sets CUPn in PSb|[codebase_location](TBD)|
|S2->S3_1`|C1;CbC3;C4;C8|Ca:`startFlushingUpgradeHandshake`|Ca sets Cx change (OPEN -> FLUSHING) in PSa; Ca sets Timeout in PSa|[codebase_location](TBD)|
|S2->S3_1|C1;CbC3;C5;C8|Ca: ChanUpgradAck|Ca sets Cx change (OPEN -> FLUSHING_COMPLETE) in PSa|


|`3->5.a(1)`|ID3;ID4;ID6;ID7;ID8;ID9|UserCall:`execute_issue`|`pallets/issue/src/lib.rs`,line `265`|
|`3->5.a(2)`|ID3;ID4;ID6;ID7;ID9|VaultCall:`execute_issue`|`pallets/issue/src/lib.rs`,line `265`|
|`4->5.b`|ID3;ID5;ID6|VaultCall`cancel_issue`|`pallets/issue/src/lib.rs`,line `293`|
|`4->5.c`|ID3;ID5;ID6;ID10|VaultCall`cancel_issue`|`pallets/issue/src/lib.rs`,line `293`|


**Flows State Machine**
To modify the draw, visit this website [drawio](https://app.diagrams.net/), select file-->Open_From --> [Issue-State-Machine](imgs/Simplified_Issue-State-Machine.drawio)

Consider that this scenario only includes the flows where: 
- User send request_issue(X).
- User send on Stellar (X+y) with y ∈ R.
- y=0.

![State Machine Chart](imgs/Issue_y=0.jpg)

**Possible Flows** 
1. `1 -> 2 -> 3 -> 5.a(1)`  
2. `1 -> 2 -> 3 -> 5.a(2)`  
3. `1 -> 2 -> 3 -> 4 -> 5.b`
4. `1 -> 2 -> 4 -> 5.c`

**Possible Flows with condition and state transitions functions** 
1. 1 (ID1;ID2):`request_issue`-> 2 (ID3):`User send Tx on Stellar with Memo=IssueID` -> 3 (ID3;ID4;ID6;ID7;ID8;ID9):`User call execute_issue` -> 5.a  
2. 1 (ID1;ID2):`request_issue`-> 2 (ID3):`User send Tx on Stellar with Memo=IssueID` -> 3 (ID3;ID4;ID6;ID7;ID9):`Vault call execute_issue` -> 5.a  
3. 1 (ID1;ID2):`request_issue`-> 2 (ID3):`User send Tx on Stellar with Memo=IssueID` -> 3 (ID3;ID5):`HG Expires` -> 4 (ID3;ID5;ID6): `Vault Call cancel_issue` -> 5.b
4. 1 (ID1;ID2):`request_issue`-> 2 (ID3,ID5):`HG Expires` -> 4 (ID3;ID5;ID6;ID10):`Vault Call cancel_issue` -> 5.c


The Upgrade Handshake protocol starts when either ChainA or ChainB send trought R a `ChannelUpgradeInit` datagram. We will assume that ChainA is the starting chain and that C0 Holds. Thus we can model the start of the protocol as follow:
- PCO holds. 
- Ca send `ChannelUpgradeInit` datagram to Cb using relayer R. 
 

TBD
## Abstraction Process Description

The abstraction process follows the next described steps: 
1. Identify the protocol main flows and subflows defined in the specs. 
2. For each flow or subflow 
    1. Idenfity involved actors.
    2. Idenfify needed definitions.  
    3. Identify set of inputs.  
    4. Identify the state transitions functions. 
    5. Identify invariant conditions. 
    6. Identify states with preconditions and postconditions.  
    7. Idenfify possible state transitions pairs.  
    8. Draw FSM.  


- CUPo: Channel Old Upgrade Parameters 
- CUPn: Channel New Upgrade Parameters 
- CI: Channel Identifiers
- PSa: Provable Store Chain A
- PSb: Provable Store Chain B
- InP: Inflight Packets 
- TO: Timeout
- TOE : Timeout Experied
- !TOE : Timeout Not Experied


</details>
