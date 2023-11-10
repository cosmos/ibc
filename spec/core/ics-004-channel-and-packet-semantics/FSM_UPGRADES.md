# Channel Upgradability Finite State Machines - WIP 

This document is an attempt to abstract the [channel upgradability specs](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md) into finite state machines (FSMs). 

According to the specs the channel upgradiblity handshake protocol defines 5 subprotocols and 7 datagrams, exchanged between the parties during the upgrading process, that are reproduced here for the reader's convenience. 
 
- Sub-protocols: `initUpgradeHandshake`, `startFlushUpgradeHandshake`, `openUpgradeHandshake`, `cancelChannelUpgrade`, and `timeoutChannelUpgrade`. 
- Datagrams:  `ChanUpgradeInit`,`ChanUpgradeTry`, `ChanUpgradeAck`, `ChanUpgradeConfirm`, `ChanUpgradeOpen`, `ChanUpgradeTimeout`, and `ChanUpgradeCancel`. 

Every defined datagram and subprotocol has an associated function. Once a datagram is received it will activate a datagram-function call which in turn may activate a subprotocol-function, based on the current state, conditions, input and flow. Additionally, we have some utility functions that can be activated by a datagram-function or subprotocol-function call. We will model utility functions as conditions.   

## Finite state machine modeling

We consider a deterministic finite state machine as a 4-tuple (Q; C; Σ; δ) consisting of: 

- a finite set of States Q
- a finite set of Conditions C
- a finite set of Accepted Inputs Σ
- a finite set of Accepted States Transition δ

### Q: States 

We start defining each state. For every state we list the status of Chain A and Chain B: ChannelState,ProvableStore,PrivateStore. 

| State | ChannelState A      | ChannelState B      | ProvableStore A                                                | ProvableStore B                                                | Private Store A                        | Private Store B                        |
|-------|---------------------|---------------------|----------------------------------------------------------------|----------------------------------------------------------------|----------------------------------------|----------------------------------------|
| q0    | OPEN                | OPEN                |                                                                |                                                                |                                        |                                        |
| q1.1  | OPEN                | OPEN                | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.VersionSet;      |                                                                |                                        |                                        |
| q1.2  | OPEN                | OPEN                |                                                                | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.VersionSet;      |                                        |                                        |
| q2    | OPEN                | OPEN                | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.VersionSet; | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.VersionSet; |                                        |                                        |
| q3.1  | FLUSHING            | OPEN                | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.VersionSet;      |                                        |                                        |
| q3.2  | OPEN                | FLUSHING            | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.VersionSet;      | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; |                                        |                                        |
| q4    | FLUSHING            | FLUSHING            | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; | Priv.Upg.CounterParty TimeoutSet 0..1; | Priv.Upg.CounterParty TimeoutSet 0..1; |
| q5.1  | FLUSHING_COMPLETE   | FLUSHING            | Upg.UpgradeSet; Chan.UpgradeSequenceSet; Upg.TimeoutSet; Upg.VersionSet; | Upg.UpgradeSet; Chan.UpgradeSequenceSet; Upg.TimeoutSet; Upg.VersionSet; | Priv.Upg.CounterParty TimeoutSet 0..1| Priv.Upg.CounterParty TimeoutSet 0..1       |
| q5.2  | FLUSHING            | FLUSHING_COMPLETE   | Upg.UpgradeSet; Chan.UpgradeSequenceSet; Upg.TimeoutSet; Upg.VersionSet; | Upg.UpgradeSet; Chan.UpgradeSequenceSet; Upg.TimeoutSet; Upg.VersionSet; | Priv.Upg.CounterParty TimeoutSet 0..1      | Priv.Upg.CounterParty TimeoutSet 0..1                                       |
| q6    | FLUSHING_COMPLETE   | FLUSHING_COMPLETE   | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; |Priv.Upg.CounterParty TimeoutSet 0..1 | Priv.Upg.CounterParty TimeoutSet 0..1|
| q7.1  | OPEN                | FLUSHING_COMPLETE   | Chan.UpgradeSequenceSet; Chan.VersionSet; Chan.ConnectionHopsSet; Chan.OrderingSet; | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; |                                        | Priv.Upg.CounterParty TimeoutSet 0..1       |
| q7.2  | FLUSHING_COMPLETE   | OPEN                | Chan.UpgradeSequenceSet; Upg.UpgradeSet; Upg.TimeoutSet; Upg.VersionSet; | Chan.UpgradeSequenceSet; Chan.VersionSet; Chan.ConnectionHopsSet; Chan.OrderingSet; | Priv.Upg.CounterParty TimeoutSet 0..1    |                                        |
| q8    | OPEN                | OPEN                | Chan.UpgradeSequenceSet; Chan.VersionSet; Chan.ConnectionHopsSet; Chan.OrderingSet; | Chan.UpgradeSequenceSet; Chan.VersionSet; Chan.ConnectionHopsSet; Chan.OrderingSet; |                                        |                                        |
| q9    | OPEN or FLUSHING                | OPEN or FLUSHING                | Chan.UpgradeErrorSet 0..1 | Chan.UpgradeErrorSet 0..1 |                                        |                                        |

Every time a state transition occurs, we should verify that in qX ChannelStateX, ProvableStateX and PrivateStoreX reflect what is written in this table.  

### C: Conditions 

Below we list all the conditions that are verified during the protocol execution. In order for a state transition to occur, cX must evaluate to True. We assume that the protocol is only started if pc0 evaluates to True. 

- pc0: BothChannelEnds === OPEN  

- c0: isAuthorizedUpgrader === True
- c1: proposedUpgradeFields.Version !==""
- c2: proposedConnection !== null && proposedConnection.state === OPEN
- c3: proposedConnection.supports(prposedUpgradeFields.ordering) === True
- c4: Upg.UpgradeSet === False
- c5: Upg.UpgradeSet === True
- c6: isCompatibleUpgradeFields === True 
- c7: VerifyChannelState === True
- c8: VerifyChannelUpgrade === True
- c9: CounterPartyUpgradeSequence >= Chan.upgradeSequence === True
- c10: Upg.TimeoutHeight !== 0 || Upg.TimeoutTimestamp !== 0 
- c11: CounterPartyTimeoutNotExpired === True
- c12: PendingInflightsPacket !== True
- c13: PendingInflightsPacket === True
- c14: Priv.Upg.CounterPartyTimeoutSet === True
- c15: CounterPartyUpgradeSequence < Chan.upgradeSequence === True
- c16: CounterPartyUpgradeSequence > Chan.upgradeSequence === True
- c17: Upg.TimeoutExpired === True
- c18: VerifyChannelUpgradeError === True
- c19: errorReceipt.sequence >= Chan.UpgradeSequence === True
- c20: isCompatibleUpgradeFields === False
- c21: CounterPartyTimeoutExpired === True

### Σ: Accepted Inputs

We now identify all the possible inputs. 
Given: 

- ix: [Party, Condition, PreviosInput]: Datagram --> subprotocolActivation -- extraDetails 

Each input identifier ix corresponds to a specific action, and the placeholders [Party, Condition, PreviousInput] capture who is involved in the action, any conditions that must be satisfied, and any previous input that must have occurred, respectively. 

Thus we can summarize the inputs as: 

- i0: [Party, , ]: ChanUpgradeInit --> initUpgradeHandshake
- i1: [Party, , ]: ChanUpgradeTry --> initUpgradeHandshake
- i2: [Party, Condition, PreviousInput]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i3: [Party, Condition, ]: ChanUpgradeTry --> startFlushUpgradeHandshake
- i4: [Party, Condition, ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i5: [Party, Condition, ]: ChanUpgradeConfirm
- i6: [Party, , PreviousInput]: PacketHandler 
- i7: [Party, Condition, PreviousInput]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic
- i8: [Party, , ]: ChanUpgradeOpen --> OpenUpgradeHandshake
- i9: [Party,Conditions , ]: ChanUpgradeCancel --> restoreChannel
- i10: [Party,Conditions , ]: ChanUpgradeTimeout --> restoreChannel

Below, we list the expanded representation of the protocol inputs. 
Note that the column previous state, "ix" indicates a previous input. When we express this as i5(c13) we assume that this is the input i5 having the same Party of the new input that is enforcing the c13 condition. 

- i0: [A, (c0; c1; c2 ; c3), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0: [B, (c0; c1; c2 ; c3), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0: [A & B, (c0; c1  c2; c3), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0: [A, (c0; c1; c2; c3 incrementUpgradeSeq), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0: [B, (c0; c1; c2; c3 incrementUpgradeSeq), ]: ChanUpgradeInit --> initUpgradeHandshake
- i1: [B, (c1; c2; c3; c4) , ]: ChanUpgradeTry --> initUpgradeHandshake
- i1: [A, (c1; c2; c3; c4) , ]: ChanUpgradeTry --> initUpgradeHandshake
- i1: [A, (c1; c2; c3; c4; c16) , ]: ChanUpgradeTry --> initUpgradeHandshake -- SwitchToCounterPartyUpgSeq
- i1: [B, (c1; c2; c3; c4; c16) , ]: ChanUpgradeTry --> initUpgradeHandshake -- SwitchToCounterPartyUpgSeq
- i2: [A, (c5; c6; c7; c8; c9; c10), i1]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i2: [B, (c5; c6; c7; c8; c9; c10), i1]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i3: [A, (c5; c6; c7; c8; c9; c10), ]: ChanUpgradeTry --> startFlushUpgradeHandshake
- i3: [B, (c5; c6; c7; c8; c9; c10), ]: ChanUpgradeTry --> startFlushUpgradeHandshake
- i4: [A, (c7; c8; c6; c10; c11; c12), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i4: [B, (c7; c8; c6; c10; c11; c13), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i4: [B, (c7; c8; c6; c10; c11; c12), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i4: [A, (c7; c8; c6; c10; c11; c13), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i5: [A, (c7; c8; c11; c12), ]: ChanUpgradeConfirm
- i5: [B, (c7; c8; c11; c12), ]: ChanUpgradeConfirm
- i5: [B, (c7; c8; c11; c13; c14), ]: ChanUpgradeConfirm
- i5: [A, (c7; c8; c11; c13; c14), ]: ChanUpgradeConfirm
- i6: [A, (c11; c12; c14), ]: PacketHandler 
- i6: [B, (c11; c12; c14), ]: PacketHandler 
- i7: [A, , i5(c12)]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic
- i7: [B, , i5(c12)]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic
- i8: [A, c7 , ]: ChanUpgradeOpen --> OpenUpgradeHandshake
- i8: [B, c7 , ]: ChanUpgradeOpen --> OpenUpgradeHandshake
- i9: [A or B, c15, ]: WriteError   ChanUpgradeCancel --> restoreChannel
- i10: [A or B, (c5; c17 )] : ChanUpgradeTimeout --> restoreChannel
- i10: [A, (c5; c17 )] : ChanUpgradeTimeout --> restoreChannel
- i10: [B, (c5; c17 )] : ChanUpgradeTimeout --> restoreChannel

Note that the current model do not represent the following cases:

- ChanUpgradeAck --> restoreChannel - c20 : UpgradeFields are not compatible
- ChanUpgradeAck --> restoreChannel - c21: TimeoutCounterParty exceeded 
- ChanUpgradeConfirm --> restoreChannel - c21: TimeoutCounterParty exceeded

### δ: Accepted States Transition

We model the accepted state transition as: 

1. [initial_state] x [input[Party,Conditions,PreviousCall]] -> [final_state]. 
2. [initial_state] x [input[Party,Conditions,PreviousCall]] -> [intermediate_state] x [input[Party,Conditions,PreviousCall]] -> [final_state]. 

---

- [`q0`] x [i0: [A,(c0; c1; c2 ; c3),]] -> [`q1.1`]
- [`q0`] x [i0: [B,(c0; c1; c2 ; c3), ]] -> [`q1.2`]
- [`q0`] x [i0: [A & B, (c0; c1; c2; c3), ]] -> [`q2`]
  
- [`q1.1`] x [i0: [A,(c0; c1; c2; c3; incrementUpgradeSeq), ]] -> [`q1.1`]
- [`q1.1`] x [i1: [B, (c0; c1; c2; c3; c4), ]] -> [`q2`] x [i2: [B, (c4; c6; c7; c8; c9; c10), i1]] -> [`q3.2`]
  
- [`q1.2`] x [i0: [B,(c0; c1; c2; c3; incrementUpgradeSeq), ]] -> [`q1.2`]
- [`q1.2`] x [i1: [A, (c0; c1; c2; c3; c4), ]] -> [`q2`] x [i2: [A, (c4; c6; c7; c8; c9; c10), i1]] -> [`q3.1`]
  
- [`q2`] x [i3: [A, (c5; c6; c7; c8; c9; c10), ]] -> [`q3.1`]
- [`q2`] x [i3: [B, (c5; c6; c7; c8; c9; c10), ]] -> [`q3.2`]
- [`q2`] x [i9: [A or B, c15, ]] -> [`q9`]
  
- [`q3.1`] x [i4: [A, (c7; c8; c6; c10; c11; c13), ]] -> [`q4`]
- [`q3.1`] x [i4: [A, (c7; c8; c6; c10; c11; c12), ]] -> [`q5.2`]
- [`q3.1`] x [i10: [A, (c5; c17; c11), ]] -> [`q9`]
  
- [`q3.2`] x [i4: [B, (c7; c8; c6; c10; c11; c12), ]] -> [`q4`]
- [`q3.2`] x [i4: [B, (c7; c8; c6; c10; c11; c13), ]] -> [`q5.1`]
- [`q3.2`] x [i10: [B, (c5; c17; c11), ]] -> [`q9`]
  
- [`q4`] x [i5: [A, (c7; c8; c11; c13; c14), ]] -> [`q4`]
- [`q4`] x [i5: [B, (c7; c8; c11; c13; c14), ]] -> [`q4`]
- [`q4`] x [i5: [A, (c7; c8; c11; c12), ]] -> [`q5.1`]
- [`q4`] x [i6: [A, (c11; c12; c14), ]] -> [`q5.1`]
- [`q4`] x [i5: [B, (c7; c8; c11; c12), ]] -> [`q5.2`]
- [`q4`] x [i6: [B, (c11; c12; c14), ]] -> [`q5.2`]
- [`q4`] x [i10: [A or B, (c5; c17; c11), ]] -> [`q9`]
  
- [`q5.1`] x [i5: [B, (c7; c8; c11; c13; c14), ]] -> [`q5.1`]
- [`q5.1`] x [i6: [B, (c11; c12), i5(c13)]] -> [`q6`]
- [`q5.1`] x [i5: [B, (c7; c8; c11; c12), ]] -> [`q6`] x [i7: [B, , i5(c12)]] -> [`q7.2`]
- [`q5.1`] x [i10: [A, (c5; c17; c11), ]] -> [`q9`]
  
- [`q5.2`] x [i5: [A, (c7; c8; c11; c13; c14), ]] -> [`q5.2`]
- [`q5.2`] x [i6: [A, (c11; c12), i5(c13)]] -> [`q6`]
- [`q5.2`] x [i5: [A, (c7; c8; c11; c12), ]] -> [`q6`] x [i7: [A, , i5(c12)]] -> [`q7.1`]
- [`q5.2`] x [i10: [B, (c5; c17; c11), ]] -> [`q9`]
  
- [`q6`] x [i8: [A, c7 ,]] -> [`q7.1`]
- [`q6`] x [i8: [B, c7 ,]] -> [`q7.2`]
  
- [`q7.1`] x [i8: [B, c7 ,]] -> [`q8`]
- [`q7.2`] x [i8: [A, c7 ,]] -> [`q8`]
  
- [`q9`] x [i9: [A or B, c15, ]] -> [`q0`]

### Finite State Machine Diagram

Here we give a graphical representation of the finite state machine. 

[FSM](https://excalidraw.com/#json=1KZM_2-MAfy7B0kELqjY9,fqGksHyqY8lww2B7SSpEcg)
![Picture](img_fsm/FSM_Upgrades.png)

We remember that the FSM do not represent the following cases:

- ChanUpgradeAck --> restoreChannel - c20 : UpgradeFields are not compatible
- ChanUpgradeAck --> restoreChannel - c21: TimeoutCounterParty exceeded 
- ChanUpgradeConfirm --> restoreChannel - c21: TimeoutCounterParty exceeded

All of these procedure will bring back the channel to its original parameters. 

### Flows

The protocol defines 3 Main possible flows: 

- A & B start the process (Crossing Hello). 
- A starts the process and B follows.
- B starts the process and A follows.

To describe the different flows we will write the state transition matrix. The state transition matrix has '1' indicating a possible transition and empty cells where no transition occurs. Empty cells indicate no direct transition is possible between those states.

#### Flow 0:  A & B start the process (Crossing Hello)

| States    | q0 | q1.1 | q1.2 | q2 | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6 | q7.1 | q7.2 | q8 | q9 |
|-----------|----|------|------|----|------|------|----|------|------|----|------|------|----|----|
| **q0**    |    | 1    | 1    | 1  |      |      |    |      |      |    |      |      |    |    |
| **q1.1**  |    | 1    |      | 1  |      |      |    |      |      |    |      |      |    |    |
| **q1.2**  |    |      | 1    | 1  |      |      |    |      |      |    |      |      |    |    |
| **q2**    |    |      |      |    | 1    | 1    |    |      |      |    |      |      |    |1   |
| **q3.1**  |    |      |      |    |      |      | 1  |      | 1    |    |      |      |    |1   |
| **q3.2**  |    |      |      |    |      |      | 1  | 1    |      |    |      |      |    |1   |
| **q4**    |    |      |      |    |      |      |    | 1    | 1    |    |      |      |    |1   |
| **q5.1**  |    |      |      |    |      |      |    | 1    |      | 1  |      |      |    |1   |
| **q5.2**  |    |      |      |    |      |      |    |      |  1   | 1  |      |      |    |1   |
| **q6**    |    |      |      |    |      |      |    |      |      |    | 1    |  1   |    |    |
| **q7.1**  |    |      |      |    |      |      |    |      |      |    |      |      | 1  |    |
| **q7.2**  |    |      |      |    |      |      |    |      |      |    |      |      | 1  |    |
| **q8**    |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q9**    | 1  |      |      |    |      |      |    |      |      |    |      |      |    |    |

#### Flow 1: A starts the process and B follows

| States    | q0 | q1.1 | q1.2 | q2 | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6 | q7.1 | q7.2 | q8 | q9 |
|-----------|----|------|------|----|------|------|----|------|------|----|------|------|----|----|
| **q0**    |    | 1    |      |    |      |      |    |      |      |    |      |      |    |    |
| **q1.1**  |    | 1    |      | 1  |      |      |    |      |      |    |      |      |    |    |
| **q1.2**  |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q2**    |    |      |      |    |      | 1    |    |      |      |    |      |      |    |1   |
| **q3.1**  |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q3.2**  |    |      |      |    |      |      | 1  | 1    |      |    |      |      |    |1   |
| **q4**    |    |      |      |    |      |      | 1  | 1    | 1    |    |      |      |    |1   |
| **q5.1**  |    |      |      |    |      |      |    | 1    |      | 1  |      |      |    |1   |
| **q5.2**  |    |      |      |    |      |      |    |      | 1    | 1  |      |      |    |1   |
| **q6**    |    |      |      |    |      |      |    |      |      |    | 1    | 1    |    |    |
| **q7.1**  |    |      |      |    |      |      |    |      |      |    |      |      | 1  |    |
| **q7.2**  |    |      |      |    |      |      |    |      |      |    |      |      | 1  |    |
| **q8**    |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q9**    | 1  |      |      |    |      |      |    |      |      |    |      |      |    |    |

#### Flow 2: B starts the process and A follows

| States    | q0 | q1.1 | q1.2 | q2 | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6 | q7.1 | q7.2 | q8 | q9 |
|-----------|----|------|------|----|------|------|----|------|------|----|------|------|----|----|
| **q0**    |    |      |  1   |    |      |      |    |      |      |    |      |      |    |    |
| **q1.1**  |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q1.2**  |    |  1   |      |  1 |      |      |    |      |      |    |      |      |    |    |
| **q2**    |    |      |      |    |   1  |      |    |      |      |    |      |      |    |1   |
| **q3.1**  |    |      |      |    |      |      |  1 |      | 1    |    |      |      |    |1   |
| **q3.2**  |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q4**    |    |      |      |    |      |      | 1  | 1    | 1    |    |      |      |    |1   |
| **q5.1**  |    |      |      |    |      |      |    | 1    |      | 1  |      |      |    |1   |
| **q5.2**  |    |      |      |    |      |      |    |      | 1    | 1  |      |      |    |1   |
| **q6**    |    |      |      |    |      |      |    |      |      |    | 1    | 1    |    |    |
| **q7.1**  |    |      |      |    |      |      |    |      |      |    |      |      | 1  |    |
| **q7.2**  |    |      |      |    |      |      |    |      |      |    |      |      | 1  |    |
| **q8**    |    |      |      |    |      |      |    |      |      |    |      |      |    |    |
| **q9**    | 1  |      |      |    |      |      |    |      |      |    |      |      |    |    |

#### SubFlows 

Any main flow can be further divided into multiple flows. As example: 

A starts B follows: 

1. q0 q1.1 q2 q3.2 q4 (q4) q5.2 q6 q7.1 q8    
2. q0 q1.1 q2 q3.2 q5.1 q.5.1 q6 q7.1 q8       
3. q0 q1.1 q2 q3.2 q5.1 q6 q7.2 q8 

But we may have more. TBC if we want to. 

## Invariant 

// To be improved. 

- Upg.Version and Chan.UpgradeSequence MUST BE SET ON BOTH ENDS in q2. 
Note that in some cases this state may be a transient state. E.g. In case we are in q1.1 and B call ChanUpgradeTry the ChanUpgradeTry will first execute an initUpgradeHandshake, write the ProvableStore and then execute a startFlushUpgradeHandshake. This means that we are going to store in chain directly FLUSHING and the finalProvableStore modified by the startFlushUpgradeHandshake.

- Upg.Timeout MUST BE SET ON BOTH ENDS in q4 or q5.1 or q5.2. The q4 state can be eventually skipped. 

- State q6 is always reached. This can be even a transient state.  

## What Could We Do Next? 

Each state can be identified by its ChannelEnd(s),ProvableStore(s). We know all the inputs associated with each state. We know all admitted state transitions.

We could use these info to test the protocol in different ways: 

1. Identify Invariant states and ensure they are always reached. 
2. Reproduce ideal behavior and verify the protocol go trhough only expected states. 
3. Fuzzing the input and ensure the protocol don't go out of the expected states. 
4. Quint Spec. How? TBD

## Questions 

1. By specs: "If a chain does not agree to the proposed counterparty upgraded ChannelEnd, it may abort the upgrade handshake by writing an ErrorReceipt into the channelUpgradeErrorPath and restoring the original channel. The ErrorReceipt must contain the current upgrade sequence on the erroring chain's channel end.

channelUpgradeErrorPath(portID, channelID) => ErrorReceipt(sequence, msg)

A relayer may then submit a ChanUpgradeCancel datagram to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote an ErrorReceipt into its channelUpgradeErrorPath with a sequence greater than or equal to its own ChannelEnd's upgrade sequence. If successful, it will restore its original channel as well, thus cancelling the upgrade."

However in the `chanUpgradeTry` is when this can happen and we have that:

```typescript
if counterpartyUpgradeSequence < channel.upgradeSequence {     
    errorReceipt = ErrorReceipt{
      channel.upgradeSequence - 1,
      "sequence out of sync", // constant string changable by implementation
    }
    provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
    return
  }
```

However, here we don't directly restore the channel. Let's say is Chain A the one going through ChanUpgradeTry. When Chain A stores the error message an event is triggered. The relayer process the event and submit a ChanUpgradeCancel to ChainB which trigger the restoreChannel. In restoreChannel Chain B will write an error message too. Thus at this point the relayer will process the event an submit a ChanUpgradeCancel and call restoreChannel. 
Another event will triggered, thus the relayers should be able to understand that he already sent ChanUpgradeCancel to both chains. Is that the case? Otherwise how and when do we restore Chain A channelEnd? 

2. Q: In case the Priv.Upg.CountePartyTimeout is not written, the ChanUpgradeOpen will anyway try to delete the Priv.Upg.CountePartyTimeout variable. What happens if this variable has not been set? Is this memory location accessible?  

A: The keeper instatiate all the potential necessary memory when is initialized. Thus it will try to del an area of memory that has been allocation on the keepr initialization. No problem here.