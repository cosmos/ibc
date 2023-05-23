# Upgrading Channels

### Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing channels to upgrade after the initial channel handshake.

### Motivation

As new features get added to IBC, chains may wish to take advantage of new channel features without abandoning the accumulated state and network effect(s) of an already existing channel. The upgrade protocol proposed would allow chains to renegotiate an existing channel to take advantage of new features without having to create a new channel, thus preserving all existing packet state processed on the channel.

### Desired Properties

- Both chains MUST agree to the renegotiated channel parameters.
- Channel state and logic on both chains SHOULD either be using the old parameters or the new parameters, but MUST NOT be in an in-between state, e.g., it MUST NOT be possible for an application to run v2 logic, while its counterparty is still running v1 logic.
- The channel upgrade protocol is atomic, i.e., 
  - either it is unsuccessful and then the channel MUST fall-back to the original channel parameters; 
  - or it is successful and then both channel ends MUST adopt the new channel parameters and the applications must process packet data appropriately.
- Packets sent under the previously negotiated parameters must be processed under the previously negotiated parameters, packets sent under the newly negotiated parameters must be processed under the newly negotiated parameters.
- The channel upgrade protocol MUST NOT modify the channel identifiers.

## Technical Specification

### Data Structures

The `ChannelState` and `ChannelEnd` are defined in [ICS-4](./README.md), they are reproduced here for the reader's convenience. `INITUPGRADE`, `TRYUPGRADE`, `ACKUPGRADE` are additional states added to enable the upgrade feature.

#### `ChannelState`

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  INITUPGRADE,
  TRYUPGRADE,
  ACKUPGRADE,
}
```

- The chain that is proposing the upgrade should set the channel state from `OPEN` to `INITUPGRADE`
- The counterparty chain that accepts the upgrade should set the channel state from `OPEN` to `TRYUPGRADE`
- Once the initiating chain verifies the counterparty is in `TRYUPGRADE`, it must move to `ACKUPGRADE` in the case where there still exist in-flight packets on **both ends** or complete the upgrade and move to `OPEN`
- The `TRYUPGRADE` chain must prove the counterparty is in `ACKUPGRADE` or completed the upgrade in `OPEN` AND have no in-flight packets on **both ends** before it can complete the upgrade and move to `OPEN`.
- The `ACKUPGRADE` chain may OPEN once in-flight packets on **both ends** have been flushed.

Both `TRYUPGRADE` and `ACKUPGRADE` are "blocking" states in that they will prevent the upgrade handshake from proceeding until the in-flight packets on both channel ends are flushed. The `TRYUPGRADE` state must additionally prove the counterparty state before proceeding to open, while the `ACKUPGRADE` state may move to `OPEN` unilaterally once packets are flushed on both ends.

#### `ChannelEnd`

```typescript
interface ChannelEnd {
  state: ChannelState
  ordering: ChannelOrder
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  connectionHops: [Identifier]
  version: string
  upgradeSequence: uint64
  flushStatus: FlushStatus
}
```

- `state`: The state is specified by the handshake steps of the upgrade protocol and will be mutated in place during the handshake.
- `upgradeSequence`: The upgrade sequence will be incremented and agreed upon during the upgrade handshake and will be mutated in place.

```typescript
enum FlushStatus {
    NOTINFLUSH
    FLUSHING
    FLUSHCOMPLETE
}
```

FlushStatus will be in `NOTINFLUSH` state when the channel is not in an upgrade handshake. It will be in `FLUSHING` mode when the channel end is flushing in-flight packets. The FlushStatus will change to `FLUSHCOMPLETE` once there are no in-flight packets left and the channelEnd is ready to move to OPEN.

All other parameters will remain the same during the upgrade handshake until the upgrade handshake completes. When the channel is reset to `OPEN` on a successful upgrade handshake, the fields on the channel end will be switched over to the `UpgradeFields` specified in the upgrade.

#### `UpgradeFields`

```typescript
interface UpgradeFields {
    version: string
    ordering: ChannelOrder
    connectionHops: [Identifier]
}
```

MAY BE MODIFIED:
- `version`: The version MAY be modified by the upgrade protocol. The same version negotiation that happens in the initial channel handshake can be employed for the upgrade handshake.
- `ordering`: The ordering MAY be modified by the upgrade protocol.
- `connectionHops`: The connectionHops MAY be modified by the upgrade protocol.

MUST NOT BE MODIFIED:
- `counterpartyChannelIdentifier`: The counterparty channel identifier MUST NOT be modified by the upgrade protocol.
- `counterpartyPortIdentifier`: The counterparty port identifier MUST NOT be modified by the upgrade protocol

NOTE: If the upgrade adds any fields to the `ChannelEnd` these are by default modifiable, and can be arbitrarily chosen by an Actor (e.g. chain governance) which has permission to initiate the upgrade.

#### `UpgradeTimeout`

```typescript
interface UpgradeTimeout {
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

- `timeoutHeight`: Timeout height indicates the height at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original channel and the upgrade handshake is aborted.
- `timeoutTimestamp`: Timeout timestamp indicates the time on the counterparty at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original channel and the upgrade handshake is aborted.

At least one of the `timeoutHeight` or `timeoutTimestamp` MUST be non-zero.

#### `Upgrade`

The upgrade type will represent a particular upgrade attempt on a channel end.

```typescript
interface Upgrade {
    fields: UpgradeFields
    timeout: UpgradeTimeout
    lastPacketSent: uint64
}
```

The upgrade contains the proposed upgrade for the channel end on the executing chain, the timeout for the upgrade attempt, and the last packet send sequence for the channel. The `lastPacketSent` allows the counterparty to know which packets need to be flushed before the channel can reopen with the newly negotiated parameters.

#### `ErrorReceipt`

```typescript
interface ErrorReceipt {
    sequence: uint64
    errorMsg: string
}
```

- `sequence` contains the sequence at which the error occurred. Both chains are expected to increment to the next sequence after the upgrade is aborted.
- `errorMsg` contains an arbitrary string which chains may use to provide additional information as to why the upgrade was aborted.

### Store Paths

#### Upgrade Channel Path

The chain must store the proposed upgrade upon initiating an upgrade. The proposed upgrade must be stored in the provable store. It may be deleted once the upgrade is successful or has been aborted.

```typescript
function channelUpgradePath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrades/upgrades/ports/{portIdentifier}/channels/{channelIdentifier}"
 }
```

The upgrade path has an associated membership verification method added to the connection interface so that a counterparty may verify that chain has stored and committed to a particular set of upgrade parameters.

```typescript
// Connection VerifyChannelUpgrade method
function verifyChannelUpgrade(
    connection: ConnectionEnd,
    height: Height,
    proof: CommitmentProof,
    counterpartyPortIdentifier: Identifier,
    counterpartyChannelIdentifier: Identifier,
    upgrade: Upgrade
) {
    clientState = queryClientState(connection.clientIdentifier)
    path = applyPrefix(connection.counterpartyPrefix, channelUpgradePath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    return verifyMembership(clientState, height, 0, 0, proof, path, upgrade)
}
```

#### CounterpartyLastPacketSequence Path

The chain must store the counterparty's last packet sequence on `startFlushUpgradeHandshake`. This will be stored in the `counterpartyLastPacketSequence` path on the private store.

```typescript
function channelCounterpartyLastPacketSequencePath(portIdentifier: Identifier, channelIdentifier: ChannelIdentifier): Path {
    return "channelUpgrades/counterpartyLastPacketSequence/ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

#### Upgrade Error Path

The upgrade error path is a public path that can signal an error of the upgrade to the counterparty for the given upgrade attempt. It does not store anything in the successful case, but it will store the `ErrorReceipt` in the case that a chain does not accept the proposed upgrade.

```typescript
function channelUpgradeErrorPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrades/upgradeError/ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

The upgrade error MUST have an associated verification membership and non-membership function added to the connection interface so that a counterparty may verify that chain has stored a non-empty error in the upgrade error path.

```typescript
// Connection VerifyChannelUpgradeError method
function verifyChannelUpgradeError(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  upgradeErrorReceipt: ErrorReceipt
) {
    clientState = queryClientState(connection.clientIdentifier)
    path = applyPrefix(connection.counterpartyPrefix, channelUpgradeErrorPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    return verifyMembership(clientState, height, 0, 0, proof, path, upgradeErrorReceipt)
}
```

```typescript
// Connection VerifyChannelUpgradeErrorAbsence method
function verifyChannelUpgradeErrorAbsence(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
) {
    clientState = queryClientState(connection.clientIdentifier)
    path = applyPrefix(connection.counterpartyPrefix, channelUpgradeErrorPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    return verifyNonMembership(clientState, height, 0, 0, proof, path)
}
```

## Sub-Protocols

The channel upgrade process consists of the following sub-protocols: `InitUpgradeHandshake`, `StartFlushUpgradeHandshake`, `OpenUpgradeHandshake`, `CancelChannelUpgrade`, and `TimeoutChannelUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the `ChannelEnd` should upgrade to the new parameters in OPEN state.

### Utility Functions

`initUpgradeChannelHandshake` is a sub-protocol that will initialize the channel end for the upgrade handshake. It will validate the upgrade parameters and set the channel state to INITUPGRADE, blocking `sendPacket` from processing outbound packets on the channel end. During this time; `receivePacket`, `acknowledgePacket` and `timeoutPacket` will still be allowed and processed according to the original channel parameters. The new proposed upgrade will be stored in the provable store for counterparty verification.

```typescript
function initUpgradeChannelHandshake(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedFields: UpgradeFields,
    timeout: UpgradeTimeout
): uint64 {
    // current channel must be OPEN
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == OPEN)

    // new channel version must be nonempty
    abortTransactionUnless(proposedFields.Version != "")

    // proposedConnection must exist and be in OPEN state for 
    // channel upgrade to be accepted
    proposedConnection = provableStore.Get(connectionPath(proposedFields.connectionHops[0])
    abortTransactionUnless(proposedConnection != null && proposedConnection.state == OPEN)

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(timeout.timeoutHeight != 0 || timeout.timeoutTimestamp != 0)

    // get last packet sent on channel and set it in the upgrade struct
    // last packet sent is the nextSequenceSend on the channel minus 1
    lastPacketSendSequence = provableStore.get(nextSequenceSendPath(portIdentifier, channelIdentifier)) - 1
    upgrade = Upgrade{
        fields: proposedFields,
        timeout: timeout,
        lastPacketSent: lastPacketSendSequence,
    }

    // store upgrade in public store for counterparty proof verification
    provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)

    currentChannel.sequence = currentChannel.sequence + 1
    currentChannel.state = INITUPGRADE
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    return currentChannel.sequence
}
```

`startFlushUpgradeHandshake` will set the counterparty last packet send and continue blocking the upgrade from continuing until all in-flight packets have been flushed. When the channel is in blocked mode, any packet receive above the counterparty last packet send will be rejected. It will verify the upgrade parameters and set the channel state to one of the flushing states (`TRYUPGRADE` or `ACKUPGRADE`) passed in by caller, set the `FlushStatus` to `FLUSHING` and block sendpackets. During this time; `receivePacket`, `acknowledgePacket` and `timeoutPacket` will still be allowed and processed according to the original channel parameters. The new proposed upgrade will be stored in the public store for counterparty verification.

```typescript
function startFlushUpgradeHandshake(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeFields: UpgradeFields,
    counterpartyChannel: ChannelEnd,
    counterpartyUpgrade: Upgrade,
    channelState: ChannelState,
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height
) {
    abortTransactionUnless(channelState == TRYUPGRADE || channelState == ACKUPGRADE)

    // current channel must be OPEN
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == OPEN)

    // get underlying connection for proof verification
    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    abortTransactionUnless(verifyChannelUpgrade(connection, proofHeight, proofUpgrade, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyUpgrade))

    // proposed ordering must be the same as the counterparty proposed ordering
    if proposedUpgradeFields.ordering != counterpartyUpgradeFields.ordering {
        restoreChannel(portIdentifier, channelIdentifier)
    }

    // connectionHops can change in a channelUpgrade, however both sides must still be each other's counterparty.
    proposedConnection = provableStore.get(connectionPath(proposedUpgradeFields.connectionHops[0])
    if (proposedConnection == null || proposedConnection.state != OPEN) {
        restoreChannel(portIdentifier, channelIdentifier)
    }
    if (counterpartyUpgrade.fields.connectionHops[0] != proposedConnection.counterpartyConnectionIdentifier) {
        restoreChannel(portIdentifier, channelIdentifier)
    }

    currentChannel.state = channelState
    currentChannel.flushState = FLUSHING

    // if there are no in-flight packets on our end, we can automatically go to FLUSHCOMPLETE
    if pendingInflightPackets(portIdentifier, channelIdentifier) == nil {
        currentChannel.flushState = FLUSHCOMPLETE
    }

    publicStore.set(channelPath(portIdentifier, channelIdentifier), channel)

    privateStore.set(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier), counterpartyUpgrade.lastPacketSent)
}
```

`openUpgradeHandshake` will open the channel and switch the existing channel parameters to the newly agreed-upon uprade channel fields.

```typescript
// caller must do all relevant checks before calling this function
function openUpgradeHandshake(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
) {
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

    // switch channel fields to upgrade fields
    // and set channel state to OPEN
    currentChannel.ordering = upgrade.fields.ordering
    currentChannel.version = upgrade.fields.version
    currentChannel.connectionHops = upgrade.fields.connectionHops
    currentchannel.state = OPEN
    currentChannel.flushStatus = NOTINFLUSH
    provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)

    // delete auxilliary state
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier))
}
```

`restoreChannel` will write an error receipt, set the channel back to its original state and delete upgrade information when the executing channel needs to abort the upgrade handshake and return to the original parameters.

```typescript
// restoreChannel signature may be modified to take a custom error
function restoreChannel(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
) {
    channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    errorReceipt = ErrorReceipt{
        channel.sequence,
        "upgrade handshake is aborted", // constant string changable by implementation
    }
    provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
    channel.state = OPEN
    channel.flushStatus = NOTINFLUSH
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)

    // delete auxilliary state
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier))
}
```

`pendingInflightPackets` will return the list of in-flight packet sequences sent from this `channelEnd`. This can be monitored since the packet commitments are deleted when the packet lifecycle is complete. Thus if the packet commitment exists on the sender chain, the packet lifecycle is incomplete. The pseudocode is not provided in this spec since it will be dependent on the state machine in-question. The ibc-go implementation will use the store iterator to implement this functionality. The function signature is provided below:

```typescript
// pendingInflightPacketSequences returns the packet sequences sent on this end that have not had their lifecycle completed
function pendingInflightPacketSequences(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
) [uint64]
```

### Upgrade Handshake

The upgrade handshake defines four datagrams: *ChanUpgradeInit*, *ChanUpgradeTry*, *ChanUpgradeAck*, and *ChanUpgradeOpen*

A successful protocol execution flows as follows (note that all calls are made through modules per [ICS 25](../ics-025-handler-interface)):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)            | Posterior state (A, B)    |
| --------- | -------------------- | ---------------- | ----------------------------- | ------------------------- |
| Actor     | `ChanUpgradeInit`    | A                | (OPEN, OPEN)                  | (INITUPGRADE, OPEN)       |
| Actor     | `ChanUpgradeTry`     | B                | (INITUPGRADE, OPEN)           | (INITUPGRADE, TRYUPGRADE) |
| Relayer   | `ChanUpgradeAck`     | A                | (INITUPGRADE, TRYUPGRADE)     | (ACKUPGRADE, TRYUPGRADE)  |

Once both states are in `ACKUPGRADE` and `TRYUPGRADE` respectively, both sides must move to `FLUSHINGCOMPLETE` respectively by clearing their in-flight packets. Once both sides have complete flushing, a relayer may submit a `ChanUpgradeOpen` message to both ends proving that the counterparty has also completed flushing in order to move the channelEnd to `OPEN`.

`ChanUpgradeOpen` is only necessary to call on chain A if the chain was not moved to `OPEN` on `ChanUpgradeAck` which may happen if all packets on both ends are already flushed.

At the end of a successful upgrade handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded channel end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded channel parameters.
- All packets sent before the handshake have been completely flushed (acked or timed out) with the old parameters.
- All packets sent after a channel end moves to OPEN will either timeout using new parameters on sending channelEnd or will be received by the counterparty using new parameters.

If a chain does not agree to the proposed counterparty upgraded `ChannelEnd`, it may abort the upgrade handshake by writing an `ErrorReceipt` into the `channelUpgradeErrorPath` and restoring the original channel. The `ErrorReceipt` must contain the current upgrade sequence on the erroring chain's channel end.

`channelUpgradeErrorPath(portID, channelID, sequence) => ErrorReceipt(sequence, msg)`

A relayer may then submit a `ChannelUpgradeCancelMsg` to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote an `ErrorReceipt` into its `channelUpgradeErrorPath` with a sequence greater than or equal to its own `ChannelEnd`'s upgrade sequence. If successful, it will restore its original channel as well, thus cancelling the upgrade.

If an upgrade message arrives after the specified timeout, then the message MUST NOT execute successfully. Again a relayer may submit a proof of this in a `ChannelUpgradeTimeoutMsg` so that counterparty cancels the upgrade and restores its original channel as well.

```typescript
function chanUpgradeInit(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeFields: Upgrade,
    timeout: UpgradeTimeout,
) {
    upgradeSequence = initUpgradeChannel(portIdentifier, channelIdentifier, proposedUpgradeFields, timeout)

    // call modules onChanUpgradeInit callback
    module = lookupModule(portIdentifier)
    version, err = module.onChanUpgradeInit(
        proposedUpgrade.fields.ordering,
        proposedUpgrade.fields.connectionHops,
        portIdentifier,
        channelIdentifier,
        upgradeSequence,
        proposedUpgrade.fields.version
    )
    // abort transaction if callback returned error
    abortTransactionUnless(err != nil)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.
Access control on counterparty should inform choice of timeout values, i.e. timeout value should be large if counterparty's `ChanUpgradeTry` is gated by chain governance.

```typescript
function chanUpgradeTry(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyUpgrade: Upgrade,
    counterpartyUpgradeSequence: uint64,
    proposedConnectionHops: [Identifier],
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height
) {
    // current channel must be OPEN or INITUPGRADE (crossing hellos)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == OPEN || currentChannel.state == INITUPGRADE)

    // create upgrade fields for this chain from counterparty upgrade and relayer-provided information
    // version may be mutated by application callback
    upgradeFields = Upgrade{
        ordering: counterpartyUpgrade.fields.ordering,
        connectionHops: proposedConnectionHops,
        version: counterpartyUpgrade.fields.version,
    }

    // either timeout height or timestamp must be non-zero
    // if the upgrade feature is implemented on the TRY chain, then a relayer may submit a TRY transaction after the timeout.
    // this will restore the channel on the executing chain and allow counterparty to use the ChannelUpgradeCancelMsg to restore their channel.
    timeout = counterpartyUpgrade.timeout
    abortTransactionUnless(timeout.timeoutHeight != 0 || timeout.timeoutTimestamp != 0)
    // counterparty-specified timeout must not have exceeded
    abortTransactionUnless(
        (currentHeight() > timeout.timeoutHeight && timeout.timeoutHeight != 0) ||
        (currentTimestamp() > timeout.timeoutTimestamp && timeout.timeoutTimestamp != 0)
    )

    // if OPEN, then initialize handshake with upgradeFields
    // otherwise, assert that the upgrade fields are the same for crossing-hellos case
    if currentChannel.state == OPEN {
        // if the counterparty sequence is greater than the current sequence, we fast forward to the counterparty sequence
        // so that both channel ends are using the same sequence for the current upgrade
        // initUpgradeChannelHandshake will increment the sequence so after that call
        // both sides will have the same upgradeSequence
        if counterpartyUpgradeSequence > channel.upgradeSequence {
            channel.upgradeSequence = counterpartyUpgradeSequence - 1
        }

        initUpgradeChannelHandshake(portIdentifier, channelIdentifier, upgradeFields, counterpartyUpgrade.timeout)
    } else if currentChannel.state == INITUPGRADE {
        existingUpgrade = publicStore.get(channelUpgradePath)
        abortTransactionUnless(existingUpgrade.fields == upgradeFields)
    }

    // get counterpartyHops for given connection
    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // construct counterpartyChannel from existing information and provided
    // counterpartyUpgradeSequence
    counterpartyChannel = ChannelEnd{
        state: INITUPGRADE,
        ordering: currentChannel.ordering,
        counterpartyPortIdentifier: portIdentifier,
        counterpartyChannelIdentifier: channelIdentifier,
        connectionHops: counterpartyHops,
        version: currentChannel.version,
        sequence: counterpartyUpgradeSequence,
    }

    // call startFlushUpgrade handshake to move channel from INITUPGRADE to TRYUPGRADE and start flushing
    // upgrade is blocked on this channelEnd from progressing until flush completes on both ends
    startFlushUpgradeHandshake(portIdentifier, channelIdentifier, upgradeFields, counterpartyChannel, counterpartyUpgrade, TRYUPGRADE, proofChannel, proofUpgrade, proofHeight)

    // if the counterparty sequence is not equal to the current sequence, then either the counterparty chain is out-of-sync or
    // the message is out-of-sync and we write an error receipt with our own sequence so that the counterparty can update
    // their sequence as well. We must then increment our sequence so both sides start the next upgrade with a fresh sequence.
    if counterpartyUpgradeSequence != channel.upgradeSequence {
        // error on the higher sequence so that both chains move to a fresh sequence
        maxSequence = max(counterpartyUpgradeSequence, channel.upgradeSequence)
        errorReceipt = ErrorReceipt{
            sequence: maxSequence,
            errorMsg: ""
        }
        provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
        provableStore.set(channelUpgradeSequencePath(portIdentifier, channelIdentifier), maxSequence)
        return
    }

    // refresh currentChannel to get latest state
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeTry callback
    module = lookupModule(portIdentifier)
    version, err = module.onChanUpgradeTry(
        proposedUpgradeChannel.ordering,
        proposedUpgradeChannel.connectionHops,
        portIdentifier,
        channelIdentifer,
        currentChannel.sequence,
        proposedUpgradeChannel.counterpartyPortIdentifer,
        proposedUpgradeChannel.counterpartyChannelIdentifier,
        proposedUpgradeChannel.version
    )
    // restore channel if callback returned error
    if err != nil {
        restoreChannel(portIdentifier, channelIdentifier)
        return
    }

    // replace channel version with the version returned by application
    // in case it was modified
    upgrade = publicStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
    upgrade.fields.version = version
    provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeTry` function. E.g. chain governance, permissioned actor, DAO, etc. A chain may decide to have permissioned **or** permissionless `ChanUpgradeTry`. In the permissioned case, both chains must explicitly consent to the upgrade; in the permissionless case, one chain initiates the upgrade and the other chain agrees to the upgrade by default. In the permissionless case, a relayer may submit the `ChanUpgradeTry` datagram.

```typescript
function chanUpgradeAck(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyUpgradeSequence: uint64,
    counterpartyFlushStatus: FlushStatus,
    counterpartyUpgrade: Upgrade,
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height
) {
    // current channel is in INITUPGRADE or TRYUPGRADE (crossing hellos)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == INITUPGRADE || currentChannel.state == TRYUPGRADE)

    // counterparty flush status must be FLUSHING or FLUSHINGCOMPLETE
    abortTransactionUnless(counterpartyFlushStatus == FLUSHING || counterpartyFlushStatus == FLUSHCOMPLETE)

    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // construct counterpartyChannel from existing information and provided
    // counterpartyUpgradeSequence
    counterpartyChannel = ChannelEnd{
        state: TRYUPGRADE,
        ordering: currentChannel.ordering,
        counterpartyPortIdentifier: portIdentifier,
        counterpartyChannelIdentifier: channelIdentifier,
        connectionHops: counterpartyHops,
        version: currentChannel.version,
        sequence: counterpartyUpgradeSequence,
        flushState: counterpartyFlushStatus,
    }

    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

    // in the crossing hellos case, the versions returned by both on TRY must be the same
    if currentChannel.state == TRYUPGRADE {
        if upgrade.fields.version != counterpartyUpgrade.fields.version {
            restoreChannel(portIdentifier, channelIdentifier)
        }
    }

    // prove counterparty and move our own state to ACKUPGRADE and start flushing
    // upgrade is blocked on this channelEnd from progressing until flush completes on both ends
    startFlushUpgradeHandshake(portIdentifier, channelIdentifier, upgrade.fields, counterpartyChannel, counterpartyUpgrade, ACKUPGRADE, proofChannel, proofUpgrade, proofHeight)

    // call modules onChanUpgradeAck callback
    // module can error on counterparty version
    // ACK should not change state to the new parameters yet
    // as that will happen on the onChanUpgradeOpen callback
    module = lookupModule(portIdentifier)
    err = module.onChanUpgradeAck(
        portIdentifier,
        channelIdentifier,
        counterpartyUpgrade.version
    )
    // restore channel if callback returned error
    if err != nil {
        restoreChannel(portIdentifier, channelIdentifier)
        return
    }

    // if no error, agree on final version
    upgrade.version = counterpartyUpgrade.version
    provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)

    // refresh channel
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))

    // if both sides have already flushed then open the upgrade handshake immediately
    if  currentChannel.state == FLUSHCOMPLETE && counterpartyFlushStatus == FLUSHCOMPLETE {
        openChannelHandshake(portIdentifier, channelIdentifier)
        module.onChanUpgradeOpen(portIdentifier, channelIdentifier)
    }
}
```

```typescript
function chanUpgradeOpen(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannelState: ChannelState,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // if packet commitments are not empty then abort the transaction
    abortTransactionUnless(pendingInflightPackets(portIdentifier, channelIdentifier))

    // currentChannel must be in TRYUPGRADE or ACKUPGRADE and have completed flushing
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == TRYUPGRADE || currentChannel.state == ACKUPGRADE)
    abortTransactionUnless(currentChannel.flushStatus == FLUSHCOMPLETE)

    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // counterparty must be in OPEN, TRYUPGRADE, ACKUPGRADE state
    if counterpartyChannelState == OPEN {
        // get upgrade since counterparty should have upgraded to these parameters
        upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

        counterpartyChannel = ChannelEnd{
            state: OPEN,
            ordering: upgrade.fields.ordering,
            counterpartyPortIdentifier: portIdentifier,
            counterpartyChannelIdentifier: channelIdentifier,
            connectionHops: upgrade.fields.connectionHops,
            version: upgrade.fields.version,
            sequence: currentChannel.sequence,
            flushStatus: NOTINFLUSH
        }
    } else if counterpartyChannelState == TRYUPGRADE {
        // MsgUpgradeAck must already have been executed before we can OPEN
        // so abort if currentState is not ACKUPGRADE
        abortTransactionUnless(currentChannel.state == ACKUPGRADE)
        counterpartyChannel = ChannelEnd{
            state: TRYUPGRADE,
            ordering: currentChannel.ordering,
            counterpartyPortIdentifier: portIdentifier,
            counterpartyChannelIdentifier: channelIdentifier,
            connectionHops: counterpartyHops,
            version: currentChannel.version,
            sequence: currentChannel.sequence,
            flushStatus: FLUSHCOMPLETE
        }
    } else if counterpartyChannelState == ACKUPGRADE {
        counterpartyChannel = ChannelEnd{
            state: ACKUPGRADE,
            ordering: currentChannel.ordering,
            counterpartyPortIdentifier: portIdentifier,
            counterpartyChannelIdentifier: channelIdentifier,
            connectionHops: counterpartyHops,
            version: currentChannel.version,
            sequence: currentChannel.sequence,
            flushStatus: FLUSHCOMPLETE
        }
    } else {
        abortTransactionUnless(false)
    }

    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

    // move channel to OPEN and adopt upgrade parameters
    openChannelHandshake(portIdentifier, channelIdentifier)

    // call modules onChanUpgradeConfirm callback
    module = lookupModule(portIdentifier)
    // confirm callback must not return error since counterparty successfully upgraded
    module.onChanUpgradeOpen(
        portIdentifer,
        channelIdentifier
    )
}
```

### Cancel Upgrade Process

During the upgrade handshake a chain may cancel the upgrade by writing an error receipt into the upgrade error path and restoring the original channel to `OPEN`. The counterparty must then restore its channel to `OPEN` as well. A relayer can facilitate this by sending `ChannelUpgradeCancelMsg` to the handler:

```typescript
function cancelChannelUpgrade(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    errorReceipt: ErrorReceipt,
    proofUpgradeError: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in INITUPGRADE or TRYUPGRADE
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == INITUPGRADE || currentChannel.state == TRYUPGRADE)

    abortTransactionUnless(!isEmpty(errorReceipt))

    // get current sequence
    // If counterparty sequence is less than the current sequence, abort transaction since this error receipt is from a previous upgrade
    // Otherwise, set the sequence to counterparty's error sequence+1 so that both sides start with a fresh sequence
    currentSequence = provableStore.get(channelUpgradeSequencePath(portIdentifier, channelIdentifier))
    abortTransactionUnless(errorReceipt.Sequence >= currentSequence)
    provableStore.set(channelUpgradeSequencePath(portIdentifier, channelIdentifier), errorReceipt.Sequence+1)

    // get underlying connection for proof verification
    connection = getConnection(currentChannel.connectionIdentifier)
    // verify that the provided error receipt is written to the upgradeError path with the counterparty sequence
    abortTransactionUnless(verifyChannelUpgradeError(connection, proofHeight, proofUpgradeError, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, errorReceipt))

    // cancel upgrade
    // and restore original channel
    // delete unnecessary state
    currentChannel.state = OPEN
    currentChannel.flushStatus = NOTINFLUSH
    provableStore.set(channelPath(portIdentifier, channelIdentifier), originalChannel)

    // delete auxilliary state
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error since counterparty successfully restored previous channelEnd
    module.onChanUpgradeRestore(
        portIdentifer,
        channelIdentifier
    )
}
```

### Timeout Upgrade Process

It is possible for the channel upgrade process to stall indefinitely on TRYUPGRADE if the TRYUPGRADE transaction simply cannot pass on the counterparty; for example, the upgrade feature may not be enabled on the counterparty chain.

In this case, we do not want the initializing chain to be stuck indefinitely in the `INITUPGRADE` step. Thus, the `ChannelUpgradeInitMsg` message will contain a `TimeoutHeight` and `TimeoutTimestamp`. The counterparty chain is expected to reject `ChannelUpgradeTryMsg` message if the specified timeout has already elapsed.

A relayer must then submit an `ChannelUpgradeTimeoutMsg` message to the initializing chain which proves that the counterparty is still in its original state. If the proof succeeds, then the initializing chain shall also restore its original channel to `OPEN` and cancel the upgrade.

```typescript
function timeoutChannelUpgrade(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannel: ChannelEnd,
    prevErrorReceipt: ErrorReceipt, // optional
    proofChannel: CommitmentProof,
    proofErrorReceipt: CommitmentProof,
    proofHeight: Height,
) {
    // current channel must be in INITUPGRADE
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnles(currentChannel.state == INITUPGRADE)

    upgradeTimeout = provableStore.get(timeoutPath(portIdentifier, channelIdentifier))

    // proof must be from a height after timeout has elapsed. Either timeoutHeight or timeoutTimestamp must be defined.
    // if timeoutHeight is defined and proof is from before timeout height
    // then abort transaction
    abortTransactionUnless(upgradeTimeout.timeoutHeight.IsZero() || proofHeight >= upgradeTimeout.timeoutHeight)
    // if timeoutTimestamp is defined then the consensus time from proof height must be greater than timeout timestamp
    connection = queryConnection(currentChannel.connectionIdentifier)
    abortTransactionUnless(upgradeTimeout.timeoutTimestamp.IsZero() || getTimestampAtHeight(connection, proofHeight) >= upgradeTimeout.timestamp)

    // get underlying connection for proof verification
    connection = getConnection(currentChannel.connectionIdentifier)

    // counterparty channel must be proved to still be in OPEN state or INITUPGRADE state (crossing hellos)
    abortTransactionUnless(counterpartyChannel.State === OPEN || counterpartyChannel.State == INITUPGRADE)
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

    // Error receipt passed in is either nil or it is a stale error receipt from a previous upgrade
    if prevErrorReceipt == nil {
        abortTransactionUnless(verifyErrorReceiptAbsence(connection, proofHeight, proofErrorReceipt, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier))
    } else {
        // timeout for this sequence can only succeed if the error receipt written into the error path on the counterparty
        // was for a previous sequence by the timeout deadline.
        sequence = provableStore.get(channelUpgradeSequencePath(portIdentifier, channelIdentifier))
        abortTransactionUnless(sequence > prevErrorReceipt.sequence)
        abortTransactionUnless(verifyErrorReceipt(connection, proofHeight, proofErrorReceipt, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, prevErrorReceipt))
    }

    // we must restore the channel since the timeout verification has passed
    currentChannel.state = OPEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)

    // delete auxilliary state
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error since counterparty successfully restored previous channelEnd
    module.onChanUpgradeRestore(
        portIdentifer,
        channelIdentifier
    )
}
```

Note that the timeout logic only applies to the INIT step. This is to protect an upgrading chain from being stuck in a non-OPEN state if the counterparty cannot execute the TRY successfully. Once the TRY step succeeds, then both sides are guaranteed to have the upgrade feature enabled. Liveness is no longer an issue, because we can wait until liveness is restored to execute the ACK step which will move the channel definitely into an OPEN state (either a successful upgrade or a rollback).

The error receipt on the counterparty may be empty (either because an upgrade error did not occur in the past, or a previous attempt was pruned), or it may have an outdated sequence (in this case the counterparty errored, our side executed a `ChanUpgradeCancel`, and then subsequently executed `INIT`). In the case where the error receipt is empty, the relayer is expected to submit an absence proof in the timeout message. In the case where the error receipt is for an outdated sequence, the relayer is expected to submit an existence proof in the timeout message. In this case, the handler will assert that the counterparty sequence is outdated **and** the upgrade timeout has passed on the counterparty by the proof height; thus proving that the counterparty did not receive a timeout message within the valid window.

The TRY chain will receive the timeout parameters chosen by the counterparty on INIT, so that it can reject any TRY message that is received after the specified timeout. This prevents the handshake from entering into an invalid state, in which the INIT chain processes a timeout successfully and restores its channel to `OPEN` while the TRY chain at a later point successfully writes a `TRY` state.

### Migrations

A chain may have to update its internal state to be consistent with the new upgraded channel. In this case, a migration handler should be a part of the chain binary before the upgrade process so that the chain can properly migrate its state once the upgrade is successful. If a migration handler is necessary for a given upgrade but is not available, then the executing chain must reject the upgrade so as not to enter into an invalid state. This state migration will not be verified by the counterparty since it will just assume that if the channel is upgraded to a particular channel version, then the auxilliary state on the counterparty will also be updated to match the specification for the given channel version. The migration must only run once the upgrade has successfully completed and the new channel is `OPEN` (ie. on `ACK` and `CONFIRM`).