# Upgrading Channels

### Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing channels to upgrade after the initial channel handshake.

### Motivation

As new features get added to IBC, chains may wish the take advantage of new channel features without abandoning the accumulated state and network effect(s) of an already existing channel. The upgrade protocol proposed would allow chains to renegotiate an existing channel to take advantage of new features without having to create a new channel, thus preserving all existing packet state processed on the channel.

### Desired Properties

- Both chains MUST agree to the renegotiated channel parameters.
- Channel state and logic on both chains SHOULD either be using the old parameters or the new parameters, but MUST NOT be in an in-between state, e.g., it MUST NOT be possible for an application to run v2 logic, while its counterparty is still running v1 logic.
- The channel upgrade protocol is atomic, i.e., 
  - either it is unsuccessful and then the channel MUST fall-back to the original channel parameters; 
  - or it is successful and then both channel ends MUST adopt the new channel parameters and the applications must process packet data appropriately.
- Packets sent under the previously negotiated parameters must be processed under the previously negotiated parameters, packets sent under the newly negotiated parameters must be processed under the newly negotiated parameters.
- The channel upgrade protocol should have the ability to change all channel-related parameters; however the channel upgrade protocol MUST NOT be able to change the underlying `ConnectionEnd`.
- The channel upgrade protocol MUST NOT modify the channel identifiers.

## Technical Specification

### Data Structures

The `ChannelState` and `ChannelEnd` are defined in [ICS-4](./README.md), they are reproduced here for the reader's convenience. `INITUPGRADE`, `TRYUPGRADE` are additional states added to enable the upgrade feature.

#### `ChannelState`

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  INITUPGRADE,
  BLOCKUPGRADE,
}
```

- The chain that is proposing the upgrade should set the channel state from `OPEN` to `INITUPGRADE`
- The counterparty chain that accepts the upgrade should set the channel state from `OPEN` to `BLOCKUPGRADE`

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
}
```

The desired property that the channel upgrade protocol MUST NOT modify the underlying clients or channel identifiers, means that only some fields of `ChannelEnd` are upgradable by the upgrade protocol.

- `state`: The state is specified by the handshake steps of the upgrade protocol and will be mutated in place during the handshake.

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

The chain must store the proposed upgrade until the upgrade handshake successfully. This must be stored in the publice store. It may be deleted once the upgrade is successful.

```typescript
function channelUpgradePath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrades/upgrades/ports/{portIdentifier}/channels/{channelIdentifier}"
 }
```

The upgrade path has an associated verification membership added to the connection interface so that a counterparty may verify that chain has stored the provided counterparty upgrade.

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

The chain must store the counterparty's last packet sequence on `blockUpgradeHandshake`. This will be stored in the `counterpartyLastPacketSequence` path on the private store/

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

The channel upgrade process consists of the following sub-protocols: `InitUpgradeHandshake`, `BlockUpgradeHandshake`, `OpenUpgradeHandshake`, `CancelChannelUpgrade`, and `TimeoutChannelUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the `ChannelEnd` should upgrade to the new parameters in OPEN state.

### Utility Functions

`initUpgradeChannelHandshake` is a subprotocol that will initialize the channel end for the upgrade handshake. It will verify the upgrade parameters and set the channel state to INITUPGRADE and block sendpackets. During this time; `receivePacket`, `acknowledgePacket` and `timeoutPacket` will still be allowed and processed according to the original channel parameters. The new proposed upgrade will be stored in the public store for counterparty verification.

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
    lastPacketSendSequence = publicStore.get(nextSequenceSendPath(portIdentifier, channelIdentifier)) - 1
    upgrade = Upgrade{
        fields: proposedFields,
        timeout: timeout,
        lastPacketSent: lastPacketSendSequence,
    }

    // store upgrade in public store for counterparty proof verification
    publicStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)

    currentChannel.sequence = currentChannel.sequence + 1
    currentChannel.state = INITUPGRADE
    publicStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    return currentChannel.sequence
}
```

`blockUpgradeHandshake` will set the counterparty last packet send and continue blocking the upgrade from continuing until all in-flight packets have been flushed. When the channel is in blocked mode, any packet receive above the counterparty last packet send will be rejected. 

```typescript
function blockUpgradeHandshake(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeFields: UpgradeFields,
    counterpartyChannel: ChannelEnd,
    counterpartyUpgrade: Upgrade,
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height
) {
    // current channel must be INITUPGRADE
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == INITUPGRADE)

    // connectionHops can change in a channelUpgrade, however both sides must still be each other's counterparty.
    proposedConnection = provableStore.get(connectionPath(proposedUpgradeFields.connectionHops[0])
    abortTransactionUnless(proposedConnection != null && proposedConnection.state == OPEN)
    abortTransactionUnless(counterpartyUpgrade.fields.connectionHops[0] == proposedConnection.counterpartyConnectionIdentifier)

    // get underlying connection for proof verification
    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    abortTransactionUnless(verifyChannelUpgrade(connection, proofHeight, proofUpgradeTimeout, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyUpgrade))

    currentChannel.state = BLOCKUPGRADE
    publicStore.set(channelPath(portIdentifier, channelIdentifier), channel)

    privateStore.set(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier), counterpartyUpgrade.lastPacketSent)
}
```

`restoreChannel` will write an error receipt, set the original channel and delete upgrade information when the executing channel needs to abort the upgrade handshake and return to the original parameters.

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
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
}
```

### Upgrade Handshake

The upgrade handshake defines four datagrams: *ChanUpgradeInit*, *ChanUpgradeTry*, *ChanUpgradeAck*, and *ChanUpgradeConfirm*

A successful protocol execution flows as follows (note that all calls are made through modules per [ICS 25](../ics-025-handler-interface)):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)          | Posterior state (A, B)      |
| --------- | -------------------- | ---------------- | --------------------------- | --------------------------- |
| Actor     | `ChanUpgradeInit`    | A                | (OPEN, OPEN)                | (INITUPGRADE, OPEN)        |
| Actor     | `ChanUpgradeTry`     | B                | (INITUPGRADE, OPEN)        | (INITUPGRADE, TRYUPGRADE) |
| Relayer   | `ChanUpgradeAck`     | A                | (INITUPGRADE, TRYUPGRADE) | (OPEN, TRYUPGRADE)         |
| Relayer   | `ChanUpgradeConfirm` | B                | (OPEN, TRYUPGRADE)         | (OPEN, OPEN)                |

At the end of an upgrade handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded channel end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded channel parameters.

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

    // if the counterparty sequence is not equal to the current sequence, then either the counterparty chain is out-of-sync or
    // the message is out-of-sync and we write an error receipt with our own sequence so that the counterparty can update
    // their sequence as well. We must then increment our sequence so both sides start the next upgrade with a fresh sequence.
    if counterpartyUpgradeSequence != channel.upgradeSequence {
        // error on the higher sequence so that both chains move to a fresh sequence
        maxSequence = max(counterpartySequence, channel.upgradeSequence)
        errorReceipt = ErrorReceipt{
            sequence: maxSequence,
            errorMsg: ""
        }
        provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
        provableStore.set(channelUpgradeSequencePath(portIdentifier, channelIdentifier), maxSequence)
        return
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
        counterpartyUpgradeSequence,
    }

    // call blockUpgrade handshake to move channel from INITUPGRADE to BLOCKUPGRADE
    blockUpgradeHandshake(portIdentifier, channelIdentifier, upgradeFields, counterpartyChannel, counterpartyUpgrade, proofChannel, proofUpgrade, proofHeight)

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
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofUpgradeSequence: CommitmentProof,
    proofHeight: Height
) {
    // current channel is in INITUPGRADE or TRYUPGRADE (crossing hellos)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == INITUPGRADE || currentChannel.state == TRYUPGRADE)

    // get underlying connection from the original channel for proof verification
    originalChannel = provableStore.get(channelRestorePath(portIdentifier, channelIdentifier))
    connection = getConnection(originalChannel.connectionIdentifier)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

    // verify that the counterparty sequence is the same as the current sequence to ensure that the proofs were
    // retrieved from the current upgrade attempt
    // since all proofs are retrieved from same proof height, and there can not be multiple upgrade states in the store for a given
    // channel at the same time
    sequence = provableStore.get(channelUpgradeSequencePath(portIdentifier, channelIdentifier))
    abortTransactionUnless(verifyUpgradeSequence(connection, proofHeight, proofUpgradeSequence, currentChannel.counterpartyPortIdentifier,
    currentChannel.counterpartyChannelIdentifier, sequence))

    // counterparty must be in TRY state
    if counterpartyChannel.State != TRYUPGRADE {
        restoreChannel(portIdentifier, channelIdentifier)
        return
    }

    // both channel ends must be mutually compatible.
    // this means that the ordering must be the same and 
    // any future introduced fields that must be compatible
    // should also be checked
    if counterpartyChannel.ordering != proposedUpgradeChannel.ordering {
        restoreChannel(portIdentifier, channelIdentifier)
        return
    }

    // call modules onChanUpgradeAck callback
    module = lookupModule(portIdentifier)
    err = module.onChanUpgradeAck(
        portIdentifier,
        channelIdentifier,
        counterpartyChannel.channelIdentifier,
        counterpartyChannel.version
    )
    // restore channel if callback returned error
    if err != nil {
        restoreChannel(portIdentifier, channelIdentifier)
        return
    }

    // upgrade is complete
    // set channel to OPEN and remove unnecessary state
    currentChannel.state = OPEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)
    provableStore.delete(channelUpgradeTimeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(channelRestorePath(portIdentifier, channelIdentifier))
}
```

```typescript
function chanUpgradeConfirm(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofUpgradeError: CommitmentProof,
    proofUpgradeSequence: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in TRYUPGRADE
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == TRYUPGRADE)

    // counterparty must be in OPEN state
    abortTransactionUnless(counterpartyChannel.State == OPEN)

    // get current sequence
    sequence = provableStore.get(channelUpgradeSequencePath(portIdentifier, channelIdentifier))

    // get underlying connection from the original channel for proof verification
    originalChannel = provableStore.get(channelRestorePath(portIdentifier, channelIdentifier))
    connection = getConnection(originalChannel.connectionIdentifier)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    // verify counterparty did not abort upgrade handshake by writing upgrade error
    // must have absent value at upgradeError path at the current sequence
    abortTransactionUnless(verifyUpgradeChannelErrorAbsence(connection, proofHeight, proofUpgradeError, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier))

    // verify that the counterparty sequence is the same as the current sequence to ensure that the proofs were
    // retrieved from the current upgrade attempt
    // since all proofs are retrieved from same proof height, and there can not be multiple upgrade states in the store for a given
    // channel at the same time
    abortTransactionUnless(verifyUpgradeSequence(connection, proofHeight, proofUpgradeSequence, currentChannel.counterpartyPortIdentifier,
    currentChannel.counterpartyChannelIdentifier, sequence))

    // call modules onChanUpgradeConfirm callback
    module = lookupModule(portIdentifier)
    // confirm callback must not return error since counterparty successfully upgraded
    module.onChanUpgradeConfirm(
        portIdentifer,
        channelIdentifier
    )
    
    // upgrade is complete
    // set channel to OPEN and remove unnecessary state
    currentChannel.state = OPEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)
    provableStore.delete(channelUpgradeTimeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(channelRestorePath(portIdentifier, channelIdentifier))
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
    abortTransactionUnless(channel.state == INITUPGRADE || channel.state == TRYUPGRADE)

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
    // and restore original conneciton
    // delete unnecessary state
    originalChannel = privateStore.get(channelRestorePath(portIdentifier, channelIdentifier))
    provableStore.set(channelPath(portIdentifier, channelIdentifier), originalChannel)

    // delete auxilliary upgrade state
    provableStore.delete(channelUpgradeTimeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(channelRestorePath(portIdentifier, channelIdentifier))

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

A relayer must then submit an `ChannelUpgradeTimeoutMsg` message to the initializing chain which proves that the counterparty is still in its original state. If the proof succeeds, then the initializing chain shall also restore its original channel and cancel the upgrade.

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
    originalChannel = privateStore.get(channelRestorePath(portIdentifier, channelIdentifier))
    provableStore.set(channelPath(portIdentifier, channelIdentifier), originalChannel)

    // delete auxilliary upgrade state
    provableStore.delete(channelUpgradeTimeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(channelRestorePath(portIdentifier, channelIdentifier))

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