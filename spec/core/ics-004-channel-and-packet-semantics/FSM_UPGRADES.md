# Channel Upgradability Finite State Machines - WIP

This document is an attempt to abstract the [channel upgradability specs](https://github.com/cosmos/ibc/blob/main/spec/core/ics-004-channel-and-packet-semantics/UPGRADES.md) into finite state machines (FSMs).

According to the specs the channel upgradiblity handshake protocol defines 7 datagrams, exchanged between the parties during the upgrade process, and 5 subprotocols that are reproduced here for the reader's convenience.

- Datagrams:  `ChanUpgradeInit`, `ChanUpgradeTry`, `ChanUpgradeAck`, `ChanUpgradeConfirm`, `ChanUpgradeOpen`, `ChanUpgradeTimeout`, and `ChanUpgradeCancel`.
- Sub-protocols: `initUpgradeHandshake`, `startFlushUpgradeHandshake`, `openUpgradeHandshake`, `cancelChannelUpgrade`, and `timeoutChannelUpgrade`.

Each datagram and subprotocol in our system is linked to a specific function. When a datagram is received, it triggers the corresponding datagram function. This function, depending on the existing state, conditions, input, and flow, may then initiate a subprotocol function. In addition to these, there are utility functions that can be invoked either directly by a datagram function or through a subprotocol function. In our modeling, these utility functions will be represented as conditional elements, streamlining the process flow.

To further streamline the complexity of the protocol, we will conduct two distinct analyses. The first will focus on the 'happy paths', covering scenarios where operations proceed as expected. The second analysis will be dedicated exclusively to errors and timeouts.

To directly jump to the diagrams click one of the following:

- [HappyPathFSM](#happy-paths-finite-state-machine-diagram)
- [ErrorTimeoutFSM](#error-and-timeout-finite-state-machine-diagram)

## Finite state machine modeling

We consider a deterministic finite state machine as a 4-tuple (Q; C; Σ; δ) consisting of:

- a finite set of States Q
- a finite set of Conditions C
- a finite set of Accepted Inputs Σ
- a finite set of Accepted States Transition δ

### Q: States

We begin by defining each state, ensuring that these states are consistent across all models. For every state, we will detail the status of both Chain A and Chain B, specifically focusing on three key aspects: ChannelState, ProvableStore, and PrivateStore.

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
| q9.1    | OPEN or FLUSHING or FLUSHING_COMPLETE | OPEN or FLUSHING or FLUSHING_COMPLETE      | Chan.UpgradeErrorSet 0..1 | Chan.UpgradeErrorSet 0..1 |                                        |                                        |
| q9.2    | FLUSHING or FLUSHING_COMPLETE             | FLUSHING or FLUSHING_COMPLETE       | Chan.UpgradeErrorSet 0..1 | Chan.UpgradeErrorSet 0..1 |                                        |                                        |

Every time a state transition occurs, we should verify that in qX ChannelStateX, ProvableStateX and PrivateStoreX reflect what is written in this table.  

### Happy Paths

#### C: Conditions

Below we list all the conditions that are verified during the protocol execution. For a state transition to occur, cX must evaluate to True.

```typescript
- pc0: BothChannelEnds === OPEN  
```

We assume that the protocol is only started if pc0 evaluates to True.

```typescript
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
- c16: CounterPartyUpgradeSequence > Chan.upgradeSequence === True
```

#### Σ: Accepted Inputs

We proceed to identify all potential inputs in our system. For example, consider the format:

- ix.y: [Party, Condition, PreviousInput]: Datagram → subprotocolActivation -- extraDetails

In this context, 'ix.y' represents the input identifier. Here, 'x' corresponds to the specific associated function, and 'y' denotes one of the potential paths that this function can take.

The elements within the brackets — [Party, Condition, PreviousInput] — serve to define the scope of each input. 'Party' specifies the participants involved in the action, 'Condition' refers to any prerequisites that need to be met, and 'PreviousInput' indicates any prior inputs that are required for the current action.

Therefore, we can encapsulate the inputs in our system as follows:

```typescript
- i0.1: [Party, Condition, ]: ChanUpgradeInit --> initUpgradeHandshake

- i1.1: [Party, Condition, ]: ChanUpgradeTry --> initUpgradeHandshake
- i1.2: [Party, Condition, PreviousInput]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i1.3: [Party, Condition, ]: ChanUpgradeTry --> startFlushUpgradeHandshake

- i2.1: [Party, Condition, ]: ChanUpgradeAck --> startFlushtUpgradeHandshake

- i3.1: [Party, Condition, ]: ChanUpgradeConfirm
- i3.2: [Party, Condition, PreviousInput]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic

- i4.1: [Party, Condition, ]: ChanUpgradeOpen --> OpenUpgradeHandshake

- i5.1: [Party, Condition, PreviousInput]: PacketHandler
```

Below, we list the expanded representation of the protocol inputs.

```typescript
- i0.1: [A, (c0; c1; c2 ; c3), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0.1: [B, (c0; c1; c2 ; c3), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0.1: [A, (c0; c1; c2; c3; incrementUpgradeSeq), ]: ChanUpgradeInit --> initUpgradeHandshake
- i0.1: [B, (c0; c1; c2; c3; incrementUpgradeSeq), ]: ChanUpgradeInit --> initUpgradeHandshake

- i1.1: [B, (c1; c2; c3; c4) , ]: ChanUpgradeTry --> initUpgradeHandshake
- i1.1: [A, (c1; c2; c3; c4) , ]: ChanUpgradeTry --> initUpgradeHandshake
- i1.1: [A, (c1; c2; c3; c4; c16) , ]: ChanUpgradeTry --> initUpgradeHandshake -- SwitchToCounterPartyUpgSeq
- i1.1: [B, (c1; c2; c3; c4; c16) , ]: ChanUpgradeTry --> initUpgradeHandshake -- SwitchToCounterPartyUpgSeq
- i1.2: [A, (c5; c6; c7; c8; c9; c10), i1.1]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i1.2: [B, (c5; c6; c7; c8; c9; c10), i1.1]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i1.3: [A, (c5; c6; c7; c8; c9; c10), ]: ChanUpgradeTry --> startFlushUpgradeHandshake
- i1.3: [B, (c5; c6; c7; c8; c9; c10), ]: ChanUpgradeTry --> startFlushUpgradeHandshake

- i2.1: [A, (c7; c8; c6; c10; c11; c12), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i2.1: [B, (c7; c8; c6; c10; c11; c13), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i2.1: [B, (c7; c8; c6; c10; c11; c12), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i2.1: [A, (c7; c8; c6; c10; c11; c13), ]: ChanUpgradeAck --> startFlushtUpgradeHandshake

- i3.1: [A, (c7; c8; c11; c12), ]: ChanUpgradeConfirm
- i3.1: [B, (c7; c8; c11; c12), ]: ChanUpgradeConfirm
- i3.1: [B, (c7; c8; c11; c13; c14), ]: ChanUpgradeConfirm
- i3.1: [A, (c7; c8; c11; c13; c14), ]: ChanUpgradeConfirm
- i3.2: [A, , i3.1 ]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic
- i3.2: [B, , i3.1 ]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic

- i4.1: [A, c7 , ]: ChanUpgradeOpen --> OpenUpgradeHandshake
- i4.1: [B, c7 , ]: ChanUpgradeOpen --> OpenUpgradeHandshake

- i5.1: [A, (c11; c12; c14), ]: PacketHandler 
- i5.1: [B, (c11; c12; c14), ]: PacketHandler 
```

#### δ: Accepted State Transitions

In this section, we outline the state transitions within our protocol. These transitions describe how the protocol moves from one state to another based on various inputs. Each transition is formatted as follows:

```typescript
[initial_state] x [input[Party,Conditions,PreviousCall]] --> [final_state].
// or 
[initial_state] x [input[Party,Conditions,PreviousCall]] --> [intermediate_state] x [input[Party,Conditions,PreviousCall]] --> [final_state].
```

In these formats:

- `[initial_state]` and `[final_state]` denote the starting and ending states.
- `[intermediate_state]` is used when there's a step between the initial and final state.
- `[input[Party, Conditions, PreviousCall]]` specifies the input details triggering the transition.

The following are the defined state transitions for our protocol:

```typescript
- [`q0`] x [i0.1: [A,(c0; c1; c2 ; c3),]] -> [`q1.1`]
- [`q0`] x [i0.1: [B,(c0; c1; c2 ; c3), ]] -> [`q1.2`]

- [`q1.1`] x [i0.1: [A,(c0; c1; c2; c3; incrementUpgradeSeq), ]] -> [`q1.1`]
- [`q1.1`] x [i0.1: [B,(c0; c1; c2 ; c3), ]]  -> [`q2`]
- [`q1.1`] x [i1.1: [B, (c0; c1; c2; c3; c4), ]] -> [`q2`] x [i1.2: [B, (c4; c6; c7; c8; c9; c10), i1.1]] -> [`q3.2`]
- [`q1.1`] x [i1.1: [B, (c0; c1; c2; c3; c4; c16), ]] -> [`q2`] x [i1.2: [B, (c4; c6; c7; c8; c9; c10), i1.1]] -> [`q3.2`]

- [`q1.2`] x [i0.1: [B,(c0; c1; c2; c3; incrementUpgradeSeq), ]] -> [`q1.2`]
- [`q1.2`] x [i0.1: [A,(c0; c1; c2 ; c3), ]]  -> [`q2`]  
- [`q1.2`] x [i1.1: [A, (c0; c1; c2; c3; c4), ]] -> [`q2`] x [i1.2: [A, (c4; c6; c7; c8; c9; c10), i1.1]] -> [`q3.1`]
- [`q1.2`] x [i1.1: [A, (c0; c1; c2; c3; c4; c16), ]] -> [`q2`] x [i1.2: [A, (c4; c6; c7; c8; c9; c10), i1.1]] -> [`q3.1`]

- [`q2`] x [i1.3: [A, (c5; c6; c7; c8; c9; c10), ]] -> [`q3.1`]
- [`q2`] x [i1.3: [B, (c5; c6; c7; c8; c9; c10), ]] -> [`q3.2`]

- [`q3.1`] x [i2.1: [B, (c7; c8; c6; c10; c11; c13), ]] -> [`q4`]
- [`q3.1`] x [i2.1: [B, (c7; c8; c6; c10; c11; c12), ]] -> [`q5.2`]

- [`q3.2`] x [i2.1: [A, (c7; c8; c6; c10; c11; c12), ]] -> [`q4`]
- [`q3.2`] x [i2.1: [A, (c7; c8; c6; c10; c11; c13), ]] -> [`q5.1`]

- [`q4`] x [i3.1: [A, (c7; c8; c11; c13; c14), ]] -> [`q4`]
- [`q4`] x [i3.1: [B, (c7; c8; c11; c13; c14), ]] -> [`q4`]
- [`q4`] x [i3.1: [A, (c7; c8; c11; c12), ]] -> [`q5.1`]
- [`q4`] x [i5.1: [A, (c11; c12; c14), ]] -> [`q5.1`]
- [`q4`] x [i3.1: [B, (c7; c8; c11; c12), ]] -> [`q5.2`]
- [`q4`] x [i5.1: [B, (c11; c12; c14), ]] -> [`q5.2`]

- [`q5.1`] x [i3.1: [B, (c7; c8; c11; c13; c14), ]] -> [`q5.1`] x [i5.1: [B, (c11; c12; c14), ]] -> [`q6`]
- [`q5.1`] x [i3.1: [B, (c7; c8; c11; c12), ]] -> [`q6`] x [i3.2: [B, , i3.1 ]] -> [`q7.2`]

- [`q5.2`] x [i3.1: [A, (c7; c8; c11; c13; c14), ]] -> [`q5.2`] x [i5.1: [A, (c11; c12; c14), ]] -> [`q6`]
- [`q5.2`] x [i3.1: [A, (c7; c8; c11; c12), ]] -> [`q6`] x [i3.2: [A, , i3.1 ]] -> [`q7.1`]

- [`q6`] x [i4.1: [A, c7 ,]] -> [`q7.1`]
- [`q6`] x [i4.1: [B, c7 ,]] -> [`q7.2`]

- [`q7.1`] x [i4.1: [B, c7 ,]] -> [`q8`]
- [`q7.2`] x [i4.1: [A, c7 ,]] -> [`q8`]
```

This list details how our protocol responds to different inputs, transitioning between its various states.

#### Happy Paths Transition Matrix

The protocol encompasses three main flows:

0. A & B start the process (Crossing Hello).
1. A starts the process and B follows.
2. B starts the process and A follows.

To illustrate these flows, we use a state transition matrix. This matrix helps in visualizing the transitions between different states in each flow. The transitions are represented as follows:

- '1' indicates a direct, final transition.
- '(1)' denotes a transient state, which leads to another subsequent state.
- '1,(1)' signifies that both a final and a transient transition are possible from the current state.
- '1(qx)' indicates an indirect, final transition happening via state qx.

In the matrix:

- A cell marked with '1' shows a possible transition between states.
- Empty cells signify that no direct transition is possible between the respective states.

This matrix structure provides a clear overview of how each state can evolve into another, depending on the specific flow being followed.

##### Flow 0:  A & B start the process (Crossing Hello)

| States    | q0 | q1.1 | q1.2 | q2    | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6   | q7.1 | q7.2 | q8 |
|-----------|----|------|------|-------|------|------|----|------|------|------|------|------|----|
| **q0**    |    | 1    | 1    |       |      |      |    |      |      |      |      |      |    |
| **q1.1**  |    | 1    |      | 1,(1) |      |1(q2) |    |      |      |      |      |      |    |
| **q1.2**  |    |      | 1    | 1,(1) |1(q2) |      |    |      |      |      |      |      |    |
| **q2**    |    |      |      |       | 1    | 1    |    |      |      |      |      |      |    |
| **q3.1**  |    |      |      |       |      |      |  1 |      | 1    |      |      |      |    |
| **q3.2**  |    |      |      |       |      |      |  1 |  1   |      |      |      |      |    |
| **q4**    |    |      |      |       |      |      |  1 | 1    | 1    |      |      |      |    |
| **q5.1**  |    |      |      |       |      |      |    | 1    |      | 1,(1)|      |1(q6) |    |
| **q5.2**  |    |      |      |       |      |      |    |      |  1   | 1,(1)|1(q6) |      |    |
| **q6**    |    |      |      |       |      |      |    |      |      |      | 1    | 1    |    |
| **q7.1**  |    |      |      |       |      |      |    |      |      |      |      |      | 1  |
| **q7.2**  |    |      |      |       |      |      |    |      |      |      |      |      | 1  |

##### Flow 1: A starts the process and B follows

This flow describes the scenario where Party A initiates the process, followed by Party B's actions. The state transition matrix for this flow is:

| States    | q0 | q1.1 | q1.2 | q2    | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6   | q7.1| q7.2 | q8 |
|-----------|----|------|------|-------|------|------|----|------|------|------|-----|------|----|
| **q0**    |    | 1    |      |       |      |      |    |      |      |      |     |      |    |
| **q1.1**  |    | 1    |      |   (1) |      |1(q2) |    |      |      |      |     |      |    |
| **q1.2**  |    |      |      |       |      |      |    |      |      |      |     |      |    |
| **q2**    |    |      |      |       |      | 1    |    |      |      |      |     |      |    |
| **q3.1**  |    |      |      |       |      |      |    |      |      |      |     |      |    |
| **q3.2**  |    |      |      |       |      |      | 1  | 1    |      |      |     |      |    |
| **q4**    |    |      |      |       |      |      | 1  | 1    | 1    |      |     |      |    |
| **q5.1**  |    |      |      |       |      |      |    | 1    |      | 1,(1)|     |1(q6) |    |
| **q5.2**  |    |      |      |       |      |      |    |      |      | 1    |     |      |    |
| **q6**    |    |      |      |       |      |      |    |      |      |      | 1   |1     |    |
| **q7.1**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |
| **q7.2**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |

##### Flow 2: B starts the process and A follows

This flow represents the situation where Party B initiates the process, with Party A following suit. The state transition matrix for this flow is:

| States    | q0 | q1.1 | q1.2 | q2    | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6   | q7.1| q7.2 | q8 |
|-----------|----|------|------|-------|------|------|----|------|------|------|-----|------|----|
| **q0**    |    |      |  1   |       |      |      |    |      |      |      |     |      |    |
| **q1.1**  |    |      |      |       |      |      |    |      |      |      |     |      |    |
| **q1.2**  |    |      | 1    |  (1)  | 1(q2)|      |    |      |      |      |     |      |    |
| **q2**    |    |      |      |       |   1  |      |    |      |      |      |     |      |    |
| **q3.1**  |    |      |      |       |      |      |  1 |      | 1    |      |     |      |    |
| **q3.2**  |    |      |      |       |      |      |    |      |      |      |     |      |    |
| **q4**    |    |      |      |       |      |      | 1  | 1    | 1    |      |     |      |    |
| **q5.1**  |    |      |      |       |      |      |    |      |      | 1    |     |      |    |
| **q5.2**  |    |      |      |       |      |      |    |      | 1    | 1,(1)|1(q6)|      |    |
| **q6**    |    |      |      |       |      |      |    |      |      |      | 1   |1     |    |
| **q7.1**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |
| **q7.2**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |

#### Happy Paths Finite State Machine Diagram

Here we give a graphical representation of the "happy paths" finite state machine. When interpreting the diagram, consider the following elements:

- **Colors**: Each arrow color signifies a specific type of transition:
  - **Green Arrows**: These illustrate transitions happening in the flow1: A starts, B follows.
  - **Blue Arrows**: These illustrate transitions happening in the flow2: B starts, A follows.
  - **Black Arrows**: These represent transitions that can take place in Flow0 (CrossingHello) or any other flow.

- **Line Thickness**:
  - **Thick Lines**: A transition with a thicker line suggests a move into a transient state or a move from a transient state.
  - **Thin Lines**: A thinner line indicates a direct move to the final state.

[FSM](https://excalidraw.com/#json=A4nB3_iZeT5jdhsXRSz3x,aBm-OM6FI6Q545CCkwaCmg)
![Picture](img_fsm/FSM_Upgrades.png)

### Errors and Timeout Model

To simplify our approach, we are modeling protocol errors and timeouts separately. In this model, we introduce two new states: q9.1, which represents the timeout state, and q9.2, which signifies the error state. Both states, q9.1 and q9.2, are reached when Chain A or Chain B records an error receipt. This error receipt is generated in response to either an error or a timeout occurrence. Once in either q9.x state, the counterpart chain is able to reset its channelEnd back to the q0 parameters, while maintaining the Chan.UpgradeSequence number at its most recent, albeit unsuccessful, upgrade value. The key distinction between q9.1 and q9.2 lies in their triggers: q9.1 can be reached through a distinct datagram-associated function other than ChanUpgradeTimeout, whereas reaching q9.2 is exclusively triggered by a ChanUpgradeTimeout.

#### C: Errors and Timeout Conditions

We list here all the conditions involved in the state transitions related to timeout and errors.

```typescript
// Errors and timeout Conditions:
- c0: isAuthorizedUpgrader === True
- c5: Upg.UpgradeSet === True
- c7: VerifyChannelState === True
- c15: CounterPartyUpgradeSequence < Chan.upgradeSequence === True
- c17: Upg.TimeoutExpired === True
- c18: VerifyChannelUpgradeError === True
- c19: errorReceipt.sequence >= Chan.UpgradeSequence === True
- c20: isCompatibleUpgradeFields === False
- c21: CounterPartyTimeoutExpired === True
- c22: Chan.State !== FLUSHING_COMPLETE 
```

#### Σ: Error and Timeout Accepted Inputs

We can summarize the inputs as:

```typescript
- i0.1: [Party, , ]: ChanUpgradeInit --> initUpgradeHandshake

- i1.1: [Party, , ]: ChanUpgradeTry --> initUpgradeHandshake
- i1.2: [Party, Condition, PreviousInput]: ChanUpgradeTry --> startFlushUpgradeHandshake -- atomic
- i1.3: [Party, Condition, ]: ChanUpgradeTry --> startFlushUpgradeHandshake
- i1.4: [Party, Condition, ]: ChanUpgradeTry --> initUpgradeHandshake --> writeError
- i1.5: [Party, Condition, ]: ChanUpgradeTry --> writeError

- i2.1: [Party, Condition, ]: ChanUpgradeAck --> startFlushtUpgradeHandshake
- i2.2: [Party, Condition, ]: ChanUpgradeAck --> restoreChannel
- i2.3: [Party, Condition, ]: ChanUpgradeAck --> startFlushtUpgradeHandshake --> restoreChannel
- i2.4: [Party, Condition, ]: ChanUpgradeAck --> startFlushtUpgradeHandshake --> CallBackError

- i3.1: [Party, Condition, ]: ChanUpgradeConfirm
- i3.2: [Party, Condition, PreviousInput]: ChanUpgradeConfirm --> OpenUpgradeHandshake -- atomic
- i3.3: [Party, Condition, ]: ChanUpgradeConfirm --> restoreChannel

- i4.1: [Party, , ]: ChanUpgradeOpen --> OpenUpgradeHandshake

- i5.1: [Party, , PreviousInput]: PacketHandler

- i6.1: [Party, Condition, ] : ChanUpgradeCancel --> restoreChannel

- i7.1: [Party, Condition, ] : ChanUpgradeTimeout --> restoreChannel
```

Below, we list the expanded representation of the protocol inputs.

```typescript
- i1.4: [A or B, c15, ]: ChanUpgradeTry --> initUpgradeHandshake --> WriteError -- CounterPartySequenceOutOfSynch
- i1.5: [A or B, c15, ]: ChanUpgradeTry --> WriteError -- CounterPartySequenceOutOfSynch

- i2.2: [A, c20 , ]: ChanUpgradeAck --> restoreChannel -- NotCompatibleUpgradeFields 
- i2.2: [B, c20 , ]: ChanUpgradeAck --> restoreChannel -- NotCompatibleUpgradeFields 
- i2.3: [A or B, c21 , ]: ChanUpgradeAck --> restoreChannel -- CounterPartyTimeoutExceeded
- i2.4: [A or B, ,]: ChanUpgradeAck --> restoreChannel -- callBackError
- i2.4: [A, ,]: ChanUpgradeAck --> restoreChannel -- callBackError
- i2.4: [B, ,]: ChanUpgradeAck --> restoreChannel -- callBackError

- i3.3: [A or B, c21 , ]: ChanUpgradeConfirm --> restoreChannel -- CounterPartyTimeoutExceeded 
- i3.3: [A, c21 , ]: ChanUpgradeConfirm --> restoreChannel -- CounterPartyTimeoutExceeded 
- i3.3: [B, c21 , ]: ChanUpgradeConfirm --> restoreChannel -- CounterPartyTimeoutExceeded 

- i6.1: [A or B, (c0; c5; c22 ), ] : ChanUpgradeCancel --> restoreChannel --restoreWihtoutVerifyingError
- i6.1: [A or B, (c5; c18; c19), ] : ChanUpgradeCancel --> restoreChannel --restoreVerifyingError

- i7.1: [A or B, (c5; c7; c17), ] : ChanUpgradeTimeout --> restoreChannel
- i7.1: [A, (c5; c7; c17), ] : ChanUpgradeTimeout --> restoreChannel
- i7.1: [B, (c5; c7; c17), ] : ChanUpgradeTimeout --> restoreChannel
```

#### δ: Error and Timeout Accepted States Transition

```typescript
// Errors
- [`q1.1`] -> [`q2`] -> x[i1.4: [A or B, c15, ]] -> [`q9.1`]
- [`q1.2`] -> [`q2`] -> x[i1.4: [A or B, c15, ]] -> [`q9.1`]

- [`q2`] -> x[i1.5: [A or B, c15, ]] -> [`q9.1`]

- [`q3.1`] -> x[i2.2: [B, c20, ]] -> [`q9.1`]
- [`q3.1`] -> [`q4`] -> x[i2.3: [A or B, c21, ]] -> [`q9.1`]
- [`q3.1`] ->[`q4`] -> x[i2.4: [A or B, , ]] -> [`q9.1`]
- [`q3.1`] -> [`q5.2`] -> x[i2.4: [B, , ]] -> [`q9.1`]

- [`q3.2`] -> x[i2.2: [A, c20, ]] -> [`q9.1`]
- [`q3.2`] ->[`q4`] -> x[i2.3: [A or B, c21, ]] -> [`q9.1`]
- [`q3.2`] ->[`q4`] -> x[i2.4: [A or B, , ]] -> [`q9.1`]
- [`q3.2`] -> [`q5.1`] -> x[i2.4: [A, , ]] -> [`q9.1`]

- [`q4`] -> x[i3.3: [A or B, c21, ]] -> [`q9.1`]

- [`q5.1`] -> x[i3.3: [B, c21, ]] -> [`q9.1`]

- [`q5.2`] -> x[i3.3: [A, c21, ]] -> [`q9.1`]

- [`q9.1`] x [i6.1: [A or B, (c0; c5; c22), ]] -> [`q0`]
- [`q9.1`] x [i6.1: [A or B, (c5; c18; c19), ]] -> [`q0`]


// Timeout
- [`q3.1`] x [i7.1: [A, (c5; c17; c11), ]] -> [`q9.2`]

- [`q3.2`] x [i7.1: [B, (c5; c17; c11), ]] -> [`q9.2`]

- [`q4`] x [i7.1: [A or B, (c5; c17; c11), ]] -> [`q9.2`]

- [`q5.1`] x [i7.1: [B, (c5; c17; c11), ]] -> [`q9.2`]

- [`q5.2`] x [i7.1: [A, (c5; c17; c11), ]] -> [`q9.2`]

- [`q9.2`] x [i6.1: [A or B, (c0; c5; c22), ]] -> [`q0`]
- [`q9.2`] x [i6.1: [A or B, (c5; c18; c19), ]] -> [`q0`]
```

#### Error and Timeout Transition Matrix

| States    | q9.1 | q9.2 |
|-----------|------|------|
| **q0**    |      |      |
| **q1.1**  |      |      |
| **q1.2**  |      |      |
| **q2**    | 1    |      |
| **q3.1**  | 1    |  1   |
| **q3.2**  | 1    |  1   |
| **q4**    | 1    |  1   |
| **q5.1**  | 1    |  1   |
| **q5.2**  | 1    |  1   |
| **q6**    |      |      |
| **q7.1**  |      |      |
| **q7.2**  |      |      |
| **q8**    |      |      |
| **q9.1**  |      |      |

##### Flow 0 Including Error and Timeout

| States    | q0 | q1.1 | q1.2 | q2    | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6   | q7.1 | q7.2 | q8 | q9.1 | q9.2 |
|-----------|----|------|------|-------|------|------|----|------|------|------|------|------|----|------|------|
| **q0**    |    | 1    | 1    |       |      |      |    |      |      |      |      |      |    |      |      |
| **q1.1**  |    | 1    |      | 1,(1) |      |1(q2) |    |      |      |      |      |      |    |1(q2) |      |
| **q1.2**  |    |      | 1    | 1,(1) |1(q2) |      |    |      |      |      |      |      |    |1(q2) |      |
| **q2**    |    |      |      |       | 1    | 1    |    |      |      |      |      |      |    | 1    |      |
| **q3.1**  |    |      |      |       |      |      | 1  |      | 1    |      |      |      |    |1,1(q4),1(q5.2)| 1    |
| **q3.2**  |    |      |      |       |      |      | 1  | 1    |      |      |      |      |    |1,1(q4),1(q5.1)| 1    |
| **q4**    |    |      |      |       |      |      |  1 | 1    | 1    |      |      |      |    | 1    | 1    |
| **q5.1**  |    |      |      |       |      |      |    | 1    |      | 1,(1)|      |1(q6) |    | 1    | 1    |
| **q5.2**  |    |      |      |       |      |      |    |      | 1    | 1,(1)|1(q6) |      |    | 1    | 1    |
| **q6**    |    |      |      |       |      |      |    |      |      |      | 1    | 1    |    |      |      |
| **q7.1**  |    |      |      |       |      |      |    |      |      |      |      |      | 1  |      |      |
| **q7.2**  |    |      |      |       |      |      |    |      |      |      |      |      | 1  |      |      |
| **q8**    |    |      |      |       |      |      |    |      |      |      |      |      |    |      |      |
| **q9.1**  | 1  |      |      |       |      |      |    |      |      |      |      |      |    |      |      |
| **q9.2**  | 1  |      |      |       |      |      |    |      |      |      |      |      |    |      |      |

##### Flow 1 Including Error and Timeout: A starts the process and B follows

This flow describes the scenario where Party A initiates the process, followed by Party B's actions. The state transition matrix for this flow is:

| States    | q0 | q1.1 | q1.2 | q2    | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6   | q7.1| q7.2 | q8 | q9.1 | q9.2|
|-----------|----|------|------|-------|------|------|----|------|------|------|-----|------|----|------|------|
| **q0**    |    | 1    |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q1.1**  |    | 1    |      |   (1) |      |1(q2) |    |      |      |      |     |      |    |1(q2) |      |
| **q1.2**  |    |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q2**    |    |      |      |       |      | 1    |    |      |      |      |     |      |    |1     |      |
| **q3.1**  |    |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q3.2**  |    |      |      |       |      |      | 1  | 1    |      |      |     |      |    |1,1(q4),1(q5.1)| 1|
| **q4**    |    |      |      |       |      |      | 1  | 1    | 1    |      |     |      |    |1     | 1    |
| **q5.1**  |    |      |      |       |      |      |    | 1    |      | 1,(1)|     |1(q6) |    |1     | 1    |
| **q5.2**  |    |      |      |       |      |      |    |      |      | 1    |     |      |    |      |      |
| **q6**    |    |      |      |       |      |      |    |      |      |      | 1   |1     |    |      |      |
| **q7.1**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |      |      |
| **q7.2**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |      |      |
| **q8**    |    |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q9.1**  | 1  |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q9.2**  | 1  |      |      |       |      |      |    |      |      |      |     |      |    |      |      |

##### Flow 2 Including Error and Timeout: B starts the process and A follows

This flow describes the scenario where Party A initiates the process, followed by Party B's actions. The state transition matrix for this flow is:

| States    | q0 | q1.1 | q1.2 | q2    | q3.1 | q3.2 | q4 | q5.1 | q5.2 | q6   | q7.1| q7.2 | q8 | q9.1 | q9.2|
|-----------|----|------|------|-------|------|------|----|------|------|------|-----|------|----|------|------|
| **q0**    |    | 1    |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q1.1**  |    |      |      |       |      |      |    |      |      |      |     |      |    |1(q2) |      |
| **q1.2**  |    | 1    |      |   (1) |1(q2) |      |    |      |      |      |     |      |    |      |      |
| **q2**    |    |      |      |       |   1  |      |    |      |      |      |     |      |    |1     |      |
| **q3.1**  |    |      |      |       |      |      | 1  | 1    |      |      |     |      |    |1,1(q4),1(q5.2)| 1|
| **q3.2**  |    |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q4**    |    |      |      |       |      |      | 1  | 1    | 1    |      |     |      |    |1     | 1    |
| **q5.1**  |    |      |      |       |      |      |    |      |      | 1    |     |      |    |1     | 1    |
| **q5.2**  |    |      |      |       |      |      |    | 1    |      | 1,(1)|1(q6)|      |    |      |      |
| **q6**    |    |      |      |       |      |      |    |      |      |      | 1   |1     |    |      |      |
| **q7.1**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |      |      |
| **q7.2**  |    |      |      |       |      |      |    |      |      |      |     |      | 1  |      |      |
| **q8**    |    |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q9.1**  | 1  |      |      |       |      |      |    |      |      |      |     |      |    |      |      |
| **q9.2**  | 1  |      |      |       |      |      |    |      |      |      |     |      |    |      |      |

#### Error and Timeout Finite State Machine Diagram

Here we give a graphical representation of the "error and timeout" finite state machine. When interpreting the diagram, consider the following elements:

- **Colors**: Each arrow color signifies a specific type of transition:
  - **Orange Arrows**: These illustrate transitions caused by `ChanUpgradeTimeout`.
  - **Red Arrows**: These denote transitions due to errors. This includes errors arising from conditions other than `ChanUpgradeTimeout`.
  - **Yellow Arrows**: These are associated with error callbacks, signifying processes that handle errors after they occur.
  - **Black Arrows**: These represent standard transitions that take place without any errors.

- **Line Thickness**:
  - **Thick Lines**: A transition with a thicker line suggests a move to states `q9.1` or `q9.2`, which follows a standard transition indicated by a black arrow.
  - **Thin Lines**: A thinner line indicates a direct move to states `q9.1` or `q9.2` without preceding transitions.

[Errors_and_Timeout_FSM](https://excalidraw.com/#json=QuXnHZqsVbAHw44DMu1F4,ZbsDB4zjdPvSc90E-fYQxQ)
![Picture2](img_fsm/FSM_Upgrades_Error_Timeout.png)

## Invariants

// Todo.

- Upg.Version and Chan.UpgradeSequence MUST BE SET ON BOTH ENDS in q2.
Note that in some cases this state may be a transient state. E.g. In case we are in q1.1 and B call ChanUpgradeTry the ChanUpgradeTry will first execute an initUpgradeHandshake, write the ProvableStore and then execute a startFlushUpgradeHandshake. This means that we are going to store in the chain directly FLUSHING and the finalProvableStore modified by the startFlushUpgradeHandshake.

- Upg.Timeout MUST BE SET ON BOTH ENDS in q4 q5.1 or q5.2. The q4 state can be eventually skipped.

- State q6 is always reached even if only as a transient state.  

## What Could We Do Next?

Each state can be identified by its ChannelEnd(s),ProvableStore(s). We know all the inputs associated with each state. We know all admitted state transitions.

We could use this info to test the protocol in different ways:

1. Identify Invariant states and ensure they are always reached.
2. Reproduce ideal behavior and verify the protocol goes through only expected states.
3. Fuzzing the input and ensuring the protocol doesn't go out of the expected states.
4. Quint Spec. How? TBD.
