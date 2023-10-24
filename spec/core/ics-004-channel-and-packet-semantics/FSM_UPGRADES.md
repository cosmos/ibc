# Channel Upgradability Finite State Machines 

This document is an attempt to abstract the [channel upgradability specs](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md) into finite state machines (FMS). 

# Upgrade Handshake 
According to the [specs](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md#upgrade-handshake), we can model the upgrade handshake with 2 main flow namely, `UpgradeOk` and `UpgradeNotOk`. `UpgradeNotOk` can be further expanded in 3 subflows namely, `UpgradeCanceled`,`UpgradeExpired`, `UpgradeStaled`.  

- `UpgradeOk`
- `UpgradeNotOk`
    - `UpgradeCanceled`
    - `UpgradeExpired` 
    - `UpgradeStaled`

We now procede with the abstraction process of every flow identified. 

## UpgradeOk 
In this section we describe the happy path of the channel upgradability protocol, the `UpgradeOk`. 

### Definition Description
We have a working channel `Chan`. The channel works on top of an established connection between ChainA and ChainB and has two ends. We will call `ChanA` and `ChanB` the ends of the channel of the connected chains. 

For both chains we have a provable store `PS`. We define `PSa` and `PSb` as the ChainA and ChainB provable store.  

For the upgradability protocol the [`Upgrade`](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md#upgrade) type, that represent a particular upgrade attempt on a channel hand, has been introduced. We will call `UpgA` and `UpgB` the upgrade parameters at both `ChanA` and `ChanB` ends

For every of the defined structure we will have `UpgA.t0`,`UpgB.t0` the upgrade parameters at t0 (e.g. starting of the upgrade) and `UpgA.t1`,`UpgB.t1` the upgrade parameters at t1 (e.g proposed upgrade) and `UpgA.t2`,`UpgB.t2` the upgrade parameters at t2 (e.g finalized upgrade). This is valid for Chan too. 

We call infligh packets `InP` the packets that have been sent before an upgrade starts. `InP` needs to be cleared out for the succesfull execution of the channel upgrade protocol.  


## Formalization of the specs

**Definition**: 
- Chan: :: Chan.State ∈ (OPEN, FLUSHING, FLUSHING_COMPLETE), Chan.UpgradeFields, Chan.ChannelID, UpgradeSequence. 
- ChanA: Cannel State on Chain A.  
- ChanB: Cannel State on Chain B.  
- Upg: Upgrade type :: Upg.UpgradeFields, Upg.UpgradeTimeout, Upg.lastPacketSent. 
- UpgA: Upgrade type on ChainA.
- UpgB: Upgrade type on ChainB.
- PS: ProvableStore.
- PSa : ProvableStore on ChainA.
- PSb : ProvableStore on ChainB. 

**UpgradeOk Table** : describes what is used within the scope of the UpgradeOk flow. 

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


**Actors**: 
- Chain A: Ca  
- Chain B: Cb
- Relayer : R

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


**C**: 
- C0 = ChanA.t0.State === OPEN === ChanB.t0.State
- C1 = ChanA.t0.ChannelID === ChanA.t1.ChannelID === ChanA.t2.ChannelID
- C2 = Chan.t2.UpgradeFields === (UpgA.t2.UpgradeFields === UpgB.t2.UpgradeFields) ||  Chan.t0.UpgradeFields
- C3 = InP.processed(Chan.t0.UpgradeFields)
- C4 = UpgA.t2.UpgradeTimeout.notExpired === UpgB.t2.UpgradeTimeout.notExpired

**Σ**:
- ChanlUpgradeInit
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
- S2->S3_1 
- S2->S3_2
- S3_1->S4
- S3_2->S4
- S4->S5

**δ**: 
We define δ : C * Σ * Q = Q'  

| C (Conditions) | Σ (Input Symbols) | Q (Current State) | δ (Next State in Q') |
|----------------|-------------------|-------------------|----------------------|
|    C0;C1       | ChannelUpgradeInit; initUpgradeHandshake; isAuthorizedUpgrader|         S0        |           S1           |
|                |                   |                   |                      |
| ...            | ...               | ...               | ...                  |



## Upgrade Handshake - UpgradeNotOk

### Upgrade Handshake - UpgradeCanceled

### Upgrade Handshake - UpgradeExpired

### Upgrade Handshake - UpgradeStaled



# Personal Notes 

<details>
  <summary>Click to expand!</summary>

The content below is not to be considered.

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
