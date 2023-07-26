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
- Packets sent under the previously negotiated parameters must be processed under the previously negotiated parameters, packets sent under the newly negotiated parameters must be processed under the newly negotiated parameters. Thus, in-flight packets sent before the upgrade handshake is complete will be processed according to the original parameters.
- The channel upgrade protocol MUST NOT modify the channel identifiers.

## Technical Specification

### Data Structures

The `ChannelState` and `ChannelEnd` are defined in [ICS-4](./README.md), they are reproduced here for the reader's convenience. `FLUSHING` and `FLUSHCOMPLETE` are additional states added to enable the upgrade feature.

#### `ChannelState`

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  FLUSHING,
  FLUSHCOMPLETE,
}
```

- In ChanUpgradeInit, the initializing chain that is proposing the upgrade should store the channel upgrade
- The counterparty chain executing `ChanUpgradeTry` that accepts the upgrade should store the channel upgrade, set the channel state from `OPEN` to `FLUSHING`, and start the flushing timer by storing an upgrade timeout.
- Once the initiating chain verifies the counterparty is in `FLUSHING`, it must also move to `FLUSHING` unless all in-flight packets are already flushed on both ends, in which case it must move directly to `FLUSHINGCOMPLETE`. The initator will also store the counterparty timeout to ensure it does not move to `FLUSHCOMPLETE` after the counterparty timeout has passed.
- The counterparty chain must prove that the initiator is  also in `FLUSHING` or completed flushing in `FLUSHCOMPLETE`. The counterparty will store the initiator timeout to ensure it does not move to `FLUSHCOMPLETE` after the initiator timeout has passed.

`FLUSHING` is a "blocking" states in that they will prevent the upgrade handshake from proceeding until the in-flight packets on both channel ends are flushed. Once both sides have moved to `FLUSHCOMPLETE`, a relayer can prove this on both ends with `ChanUpgradeOpen` to open the channel on both sides with the new parameters.

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

- `state`: The state is specified by the handshake steps of the upgrade protocol and will be mutated in place during the handshake. It will be in `FLUSHING` mode when the channel end is flushing in-flight packets. The state will change to `FLUSHCOMPLETE` once there are no in-flight packets left and the channelEnd is ready to move to OPEN.
- `upgradeSequence`: The upgrade sequence will be incremented and agreed upon during the upgrade handshake and will be mutated in place.

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
- `ordering`: The ordering MAY be modified by the upgrade protocol so long as the new ordering is supported by underlying connection.
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

The upgrade contains the proposed upgrade for the channel end on the executing chain, the timeout for the upgrade attempt, and the last packet send sequence for the channel. The `lastPacketSent` allows the counterparty to know which packets need to be flushed before the channel can reopen with the newly negotiated parameters. Any packet sent to the channel end with a packet sequence above the `lastPacketSent` will be rejected until the upgrade is complete.

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

#### Channel Upgrade Path

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
function counterpartyLastPacketSequencePath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrades/counterpartyLastPacketSequence/ports/{portIdentifier}/channels/{channelIdentifier}"
}
```

#### CounterpartyUpgradeTimeout Path

The chain must store the counterparty's upgradeTimeout. This will be stored in the `counterpartyUpgradeTimeout` path on the private store

```typescript
function counterpartyUpgradeTimeout(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrades/counterpartyUpgradeTimeout/ports/{portIdentifier}/channels/{channelIdentifier}"
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

The channel upgrade process consists of the following sub-protocols: `initUpgradeHandshake`, `startFlushUpgradeHandshake`, `openUpgradeHandshake`, `cancelChannelUpgrade`, and `timeoutChannelUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the `ChannelEnd` should upgrade to the new parameters in OPEN state.

### Utility Functions

`initUpgradeHandshake` is a sub-protocol that will initialize the channel end for the upgrade handshake. It will validate the upgrade parameters and store the channel upgrade. All packet processing will continue according to the original channel parameters, as this is a signalling mechanism that can remain indefinitely. The new proposed upgrade will be stored in the provable store for counterparty verification. If it is called again before the handshake starts, then the current proposed upgrade will be replaced with the new one and the channel sequence will be incremented.

```typescript
// initUpgradeHandshake will verify that the channel is in the correct precondition to call the initUpgradeHandshake protocol
// it will verify the new upgrade field parameters, and make the relevant state changes for initializing a new upgrade:
// - store channel upgrade
// - incrementing upgrade sequence
function initUpgradeHandshake(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeFields: UpgradeFields,
): uint64 {
    // current channel must be OPEN
    // If channel already has an upgrade but isn't in FLUSHING, then this will override the previous upgrade attempt
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == OPEN)

    // new channel version must be nonempty
    abortTransactionUnless(proposedUpgradeFields.Version != "")

    // proposedConnection must exist and be in OPEN state for 
    // channel upgrade to be accepted
    proposedConnection = provableStore.Get(connectionPath(proposedUpgradeFields.connectionHops[0])
    abortTransactionUnless(proposedConnection != null && proposedConnection.state == OPEN)

    // new order must be supported by the new connection
    abortTransactionUnless(isSupported(proposedConnection, proposedUpgradeFields.ordering))

    // lastPacketSent and timeout will be filled when we move to FLUSHING
    upgrade = Upgrade{
        fields: proposedUpgradeFields,
    }

    // store upgrade in public store for counterparty proof verification
    provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)

    currentChannel.sequence = currentChannel.sequence + 1
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
    return currentChannel.sequence
}
```

`startFlushUpgradeHandshake` will set the counterparty last packet send and continue blocking the upgrade from continuing until all in-flight packets have been flushed. When the channel is in blocked mode, any packet receive above the counterparty last packet send will be rejected. It will verify the upgrade parameters and set the channel state to `FLUSHING` and block `sendPackets`. During this time; `receivePacket`, `acknowledgePacket` and `timeoutPacket` will still be allowed and processed according to the original channel parameters. The state machine will set a timer for how long the other side can take before it completes flushing and moves to `FLUSHCOMPLETE`. The new proposed upgrade will be stored in the public store for counterparty verification.

```typescript
// startFlushUpgradeSequence will verify that the channel is in a valid precondition for calling the startFlushUpgradeHandshake
// it will verify the proofs of the counterparty channel and upgrade
// it will verify that the upgrades on both ends are mutually compatible
// it will set the channel to desiredChannel state and move to flushing mode if we are not already in flushing mode
function startFlushUpgradeHandshake(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeFields: UpgradeFields,
    counterpartyChannel: ChannelEnd,
    counterpartyUpgrade: Upgrade,
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height
) {
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == OPEN || currentChannel.state == FLUSHING)

    // get underlying connection for proof verification
    connection = getConnection(currentChannel.connectionIdentifier)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    abortTransactionUnless(verifyChannelUpgrade(connection, proofHeight, proofUpgrade, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyUpgrade))

    // if the counterparty sequence is not equal to the current sequence, then either the counterparty chain is out-of-sync or
    // the message is out-of-sync and we write an error receipt with our own sequence so that the counterparty can update
    // their sequence as well. We must then increment our sequence so both sides start the next upgrade with a fresh sequence.
    if counterpartyUpgradeSequence != channel.upgradeSequence {
        // error on the higher sequence so that both chains move to a fresh sequence
        maxSequence = max(counterpartyUpgradeSequence, channel.upgradeSequence)
        currentChannel.UpgradeSequence = maxSequence
        provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)
        
        restoreChannel(portIdentifier, channelIdentifier)
        return
    }

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

    // only execute flushing state changes if it has not already occurred
    if currentChannel.state == OPEN {
        currentChannel.state = FLUSHING

        upgradeTimeout = getUpgradeTimeout(currentChannel.portIdentifier, currentChannel.channelIdentifier)
        // either timeout height or timestamp must be non-zero
        abortTransactionUnless(upgradeTimeout.timeoutHeight != 0 || upgradeTimeout.timeoutTimestamp != 0)

        lastPacketSendSequence = provableStore.get(nextSequenceSendPath(portIdentifier, channelIdentifier)) - 1

        upgrade = Upgrade{
            fields: proposedUpgradeFields,
            upgradeTimeout: upgradeTimeout,
            lastPacketSent: lastPacketSendSequence, 
        }

        // store upgrade in public store for counterparty proof verification
        provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
    }
}
```

`openUpgradeHandshake` will open the channel and switch the existing channel parameters to the newly agreed-upon uprade channel fields.

```typescript
// openUpgradeHandshake will switch the channel fields over to the agreed upon upgrade fields
// it will reset the channel state to OPEN
// it will delete auxilliary upgrade state
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
    provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)

    // delete auxilliary state
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyUpgradeTimeout(portIdentifier, channelIdentifier))
}
```

`restoreChannel` will write an error receipt, set the channel back to its original state and delete upgrade information when the executing channel needs to abort the upgrade handshake and return to the original parameters.

```typescript
// restoreChannel will restore the channel state to its pre-upgrade state and delete upgrade auxilliary state so that upgrade is aborted
// it write an error receipt to state so counterparty can restore as well.
// NOTE: this function signature may be modified by implementors to take a custom error
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

    // delete auxilliary state
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyLastPacketSequencePath(portIdentifier, channelIdentifier))
    privateStore.delete(channelCounterpartyUpgradeTimeout(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error since counterparty successfully restored previous channelEnd
    module.onChanUpgradeRestore(
        portIdentifer,
        channelIdentifier
    )
}
```

`pendingInflightPackets` will return the list of in-flight packet sequences sent from this `channelEnd`. This can be monitored since the packet commitments are deleted when the packet lifecycle is complete. Thus if the packet commitment exists on the sender chain, the packet lifecycle is incomplete. The pseudocode is not provided in this spec since it will be dependent on the state machine in-question. The ibc-go implementation will use the store iterator to implement this functionality. The function signature is provided below:

```typescript
// pendingInflightPacketSequences returns the packet sequences sent on this end that have not had their lifecycle completed
function pendingInflightPacketSequences(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
): [uint64]
```

`isAuthorizedUpgrader` will return true if the provided address is authorized to initialize, modify, and cancel upgrades. Chains may permission a set of addresses that can signal which upgrade a channel is willing to upgrade to.

```typescript
// isAuthorizedUpgrader
function isAuthorizedUpgrader(address: string): boolean
```

`getUpgradeTimeout` will return the upgrade timeout specified for the given channel. This may be a chain-wide parameter, or it can be a parameter chosen per channel. This is an implementation-level detail, so only the function signature is specified here. Note this should retrieve some stored timeout delta for the channel and add it to the current height and time to get the absolute timeout values.

```typescript
// getUpgradeTimeout
function getUpgradeTimeout(portIdentifier: string, channelIdentifier: string) UpgradeTimeout {
}
```

### Upgrade Handshake

The upgrade handshake defines seven datagrams: *ChanUpgradeInit*, *ChanUpgradeTry*, *ChanUpgradeAck*, *ChanUpgradeConfirm*, *ChanUpgradeOpen*, *ChanUpgradeTimeout*, and *ChanUpgradeCancel*

A successful protocol execution flows as follows (note that all calls are made through modules per [ICS 25](../ics-025-handler-interface)):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)                 | Posterior state (A, B)                                |
| --------- | -------------------- | ---------------- | ---------------------------------- | ----------------------------------------------------- |
| Actor     | `ChanUpgradeInit`    | A                | (OPEN, OPEN)                       | (OPEN, OPEN)                                          |
| Relayer   | `ChanUpgradeTry`     | B                | (OPEN, OPEN)                       | (OPEN, FLUSHING)                                      |
| Relayer   | `ChanUpgradeAck`     | A                | (OPEN, FLUSHING)                   | (FLUSHING/FLUSHCOMPLETE, FLUSHING)                    |
| Relayer   | `ChanUpgradeConfirm` | B                | (FLUSHING/FLUSHCOMPLETE, FLUSHING) | (FLUSHING/FLUSHCOMPLETE, FLUSHING/FLUSHCOMPLETE/OPEN) |

Once both states are in `FLUSHING` and both sides have stored each others upgrade timeouts, both sides can move to `FLUSHINGCOMPLETE` by clearing their in-flight packets. Once both sides have complete flushing, a relayer may submit a `ChanUpgradeOpen` message to both ends proving that the counterparty has also completed flushing in order to move the channelEnd to `OPEN`.

`ChanUpgradeOpen` is only necessary to call on chain B if the chain was not moved to `OPEN` on `ChanUpgradeConfirm` which may happen if all packets on both ends are already flushed.

At the end of a successful upgrade handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded channel end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded channel parameters.
- All packets sent before the handshake have been completely flushed (acked or timed out) with the old parameters.
- All packets sent after a channel end moves to OPEN will either timeout using new parameters on sending channelEnd or will be received by the counterparty using new parameters.

If a chain does not agree to the proposed counterparty upgraded `ChannelEnd`, it may abort the upgrade handshake by writing an `ErrorReceipt` into the `channelUpgradeErrorPath` and restoring the original channel. The `ErrorReceipt` must contain the current upgrade sequence on the erroring chain's channel end.

`channelUpgradeErrorPath(portID, channelID, sequence) => ErrorReceipt(sequence, msg)`

A relayer may then submit a `ChannelUpgradeCancelMsg` to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote an `ErrorReceipt` into its `channelUpgradeErrorPath` with a sequence greater than or equal to its own `ChannelEnd`'s upgrade sequence. If successful, it will restore its original channel as well, thus cancelling the upgrade.

If a chain does not reach FLUSHCOMPLETE within the counterparty specified timeout, then it MUST NOT move to FLUSHCOMPLETE and should instead abort the upgrade. A relayer may submit a proof of this to the counterparty chain in a `ChannelUpgradeTimeoutMsg` so that counterparty cancels the upgrade and restores its original channel as well.

```typescript
function chanUpgradeInit(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeFields: Upgrade,
    msgSender: string,
) {
    // chanUpgradeInit may only be called by addresses authorized by executing chain
    abortTransactionUnless(isAuthorizedUpgrader(msgSender))

    upgradeSequence = initUpgradeChannel(portIdentifier, channelIdentifier, proposedUpgradeFields)

    // call modules onChanUpgradeInit callback
    module = lookupModule(portIdentifier)
    version, err = module.onChanUpgradeInit(
        portIdentifier,
        channelIdentifier,
        proposedUpgrade.fields.ordering,
        proposedUpgrade.fields.connectionHops,
        upgradeSequence,
        proposedUpgrade.fields.version
    )
    // abort transaction if callback returned error
    abortTransactionUnless(err != nil)

    // replace channel upgrade version with the version returned by application
    // in case it was modified
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
    upgrade.fields.version = version
    provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.

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
    // current channel must be OPEN (i.e. not in FLUSHING)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == OPEN)

    // create upgrade fields for this chain from counterparty upgrade and relayer-provided information
    // version may be mutated by application callback
    upgradeFields = Upgrade{
        ordering: counterpartyUpgrade.fields.ordering,
        connectionHops: proposedConnectionHops,
        version: counterpartyUpgrade.fields.version,
    }

    existingUpgrade = publicStore.get(channelUpgradePath)

    // current upgrade either doesn't exist (non-crossing hello case), we initialize the upgrade with constructed upgradeFields
    // if it does exist, we are in crossing hellos and must assert that the upgrade fields are the same for crossing-hellos case
    if existingUpgrade == nil {
        // if the counterparty sequence is greater than the current sequence, we fast forward to the counterparty sequence
        // so that both channel ends are using the same sequence for the current upgrade
        // initUpgradeChannelHandshake will increment the sequence so after that call
        // both sides will have the same upgradeSequence
        if counterpartyUpgradeSequence > currentChannel.upgradeSequence {
            currentChannel.upgradeSequence = counterpartyUpgradeSequence - 1
        }

        initUpgradeChannelHandshake(portIdentifier, channelIdentifier, upgradeFields)
    }

    // get counterpartyHops for given connection
    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // construct counterpartyChannel from existing information and provided
    // counterpartyUpgradeSequence
    counterpartyChannel = ChannelEnd{
        state: OPEN,
        ordering: currentChannel.ordering,
        counterpartyPortIdentifier: portIdentifier,
        counterpartyChannelIdentifier: channelIdentifier,
        connectionHops: counterpartyHops,
        version: currentChannel.version,
        sequence: counterpartyUpgradeSequence,
    }

    // call startFlushUpgrade handshake to move channel to FLUSHING, which will block
    // upgrade from progressing to OPEN until flush completes on both ends
    startFlushUpgradeHandshake(portIdentifier, channelIdentifier, upgradeFields, counterpartyChannel, counterpartyUpgrade, proofChannel, proofUpgrade, proofHeight)

    // refresh currentChannel to get latest state
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeTry callback
    module = lookupModule(portIdentifier)
    version, err = module.onChanUpgradeTry(
        portIdentifier,
        channelIdentifer,
        proposedUpgradeChannel.ordering,
        proposedUpgradeChannel.connectionHops,
        currentChannel.sequence,
        proposedUpgradeChannel.counterpartyPortIdentifer,
        proposedUpgradeChannel.counterpartyChannelIdentifier,
        proposedUpgradeChannel.version
    )
    // abort the transaction if the callback returns an error and
    // there was no existing upgrade. This will allow the counterparty upgrade
    // to continue existing while this chain may add support for it in the future
    if err != nil && existingUpgrade == nil {
        abortTransactionUnless(false)
    }
    // if the callback returns an error while an existing upgrade is in place
    // or if the existing upgrade is not compatible with the counterparty upgrade
    // we must restore the channel so that a new upgrade attempt can be made
    if err != nil || existingUpgrade.fields != upgradeFields {
        restoreChannel(portIdentifier, channelIdentifier)
    }

    // replace channel version with the version returned by application
    // in case it was modified
    upgrade = publicStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
    upgrade.fields.version = version
    provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

NOTE: Implementations that want to explicitly permission upgrades should enforce crossing hellos. i.e. Both parties must have called ChanUpgradeInit with mutually compatible parameters in order for ChanUpgradeTry to succeed. Implementations that want to be permissive towards counterparty-initiated upgrades may allow moving from OPEN to FLUSHING without having an upgrade previously stored on the executing chain.

```typescript
function chanUpgradeAck(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyUpgrade: Upgrade,
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height
) {
    // current channel is OPEN or FLUSHING (crossing hellos)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == OPEN || currentChannel.state == FLUSHING)
    priorState = currentChannel.state

    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // construct counterpartyChannel from existing information
    counterpartyChannel = ChannelEnd{
        state: FLUSHING,
        ordering: currentChannel.ordering,
        counterpartyPortIdentifier: portIdentifier,
        counterpartyChannelIdentifier: channelIdentifier,
        connectionHops: counterpartyHops,
        version: currentChannel.version,
        sequence: channel.sequence,
    }

    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

    // prove counterparty and move our own state to flushing
    // if we are already at flushing, then no state changes occur
    // upgrade is blocked on this channelEnd from progressing until flush completes on both ends
    startFlushUpgradeHandshake(portIdentifier, channelIdentifier, upgrade.fields, counterpartyChannel, counterpartyUpgrade, proofChannel, proofUpgrade, proofHeight)

    timeout = counterpartyUpgrade.timeout
    
    // counterparty-specified timeout must not have exceeded
    // if it has, then restore the channel and abort upgrade handshake
    if (timeout.timeoutHeight != 0 && currentHeight() >= timeout.timeoutHeight) ||
          (timeout.timeoutTimestamp != 0 && currentTimestamp() >= timeout.timeoutTimestamp ) {
            restoreChannel(portIdentifier, channelIdentifier)
    }

     // in the crossing hellos case, the versions returned by both on TRY must be the same
    if priorState == FLUSHING {
        if upgrade.fields.version != counterpartyUpgrade.fields.version {
            restoreChannel(portIdentifier, channelIdentifier)
        }
    }

    // if there are no in-flight packets on our end, we can automatically go to FLUSHCOMPLETE
    // otherwise store counterparty timeout so packet handlers can check before going to FLUSHCOMPLETE
    if pendingInflightPackets(portIdentifier, channelIdentifier) == nil {
        currentChannel.state = FLUSHCOMPLETE
    } else {
        privateStore.set(counterpartyUpgradeTimeout(portIdentifier, channelIdentifier), timeout)
    }

    publicStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)

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
}
```

`chanUpgradeConfirm` is called on the chain which is on `FLUSHING` **after** `chanUpgradeAck` is called on the counterparty. This will inform the TRY chain of the timeout set on ACK by the counterparty. If the timeout has already exceeded, we will write an error receipt and restore. If packets on both sides have already been flushed and timeout is not exceeded, then we can open the channel. Otherwise, we set the counterparty timeout in the private store and wait for packet flushing to complete.

```typescript
function chanUpgradeConfirm(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannelState: state,
    counterpartyUpgrade: Upgrade,
    proofChannel: CommitmentProof,
    proofUpgrade: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in FLUSHING
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == FLUSHING)

    // counterparty channel is either FLUSHING or FLUSHCOMPLETE
    abortTransactionUnles(counterpartyChannelState == FLUSHING || counterpartyChannelState == FLUSHCOMPLETE)

    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    counterpartyChannel = ChannelEnd{
        state: counterpartyFlushState,
        ordering: currentChannel.ordering,
        counterpartyPortIdentifier: portIdentifier,
        counterpartyChannelIdentifier: channelIdentifier,
        connectionHops: counterpartyHops,
        version: currentChannel.version,
        sequence: channel.sequence,
    }

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    abortTransactionUnless(verifyChannelUpgrade(connection, proofHeight, proofUpgrade, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyUpgrade))

    timeout = counterpartyUpgrade.timeout
    
    // counterparty-specified timeout must not have exceeded
    // if it has, then restore the channel and abort upgrade handshake
    if (timeout.timeoutHeight != 0 && currentHeight() >= timeout.timeoutHeight) ||
          (timeout.timeoutTimestamp != 0 && currentTimestamp() >= timeout.timeoutTimestamp ) {
            restoreChannel(portIdentifier, channelIdentifier)
    }

    // if there are no in-flight packets on our end, we can automatically go to FLUSHCOMPLETE
    if pendingInflightPackets(portIdentifier, channelIdentifier) == nil {
        currentChannel.state = FLUSHCOMPLETE
        publicStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)
    } else {
        privateStore.set(counterpartyUpgradeTimeout(portIdentifier, channelIdentifier), timeout)
    }

    // if both chains are already in flushcomplete we can move to OPEN
    if currentChannel.state == FLUSHCOMPLETE && counterpartyChannelState == FLUSHCOMPLETE {
        openUpgradelHandshake(portIdentifier, channelIdentifier)
        module.onChanUpgradeOpen(portIdentifier, channelIdentifier)
    }
}
```

`chanUpgradeOpen` may only be called once both sides have moved to FLUSHCOMPLETE. If there exists unprocessed packets in the queue when the handshake goes into `FLUSHING` mode, then the packet handlers must move the channelEnd to `FLUSHCOMPLETE` once the last packet on the channelEnd has been processed.

```typescript
function chanUpgradeOpen(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannelState: ChannelState,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // currentChannel must have completed flushing
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == FLUSHCOMPLETE)

    // counterparty upgrade must not have passed on our chain
    connection = getConnection(currentChannel.connectionIdentifier)
    counterpartyHops = getCounterpartyHops(connection)

    // counterparty must be in OPEN or FLUSHCOMPLETE state
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
        }
    } else if counterpartyChannelState == FLUSHCOMPLETE {
        counterpartyChannel = ChannelEnd{
            state: FLUSHCOMPLETE,
            ordering: currentChannel.ordering,
            counterpartyPortIdentifier: portIdentifier,
            counterpartyChannelIdentifier: channelIdentifier,
            connectionHops: counterpartyHops,
            version: currentChannel.version,
            sequence: currentChannel.sequence,
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
    msgSender: string,
) {
    // current channel has an upgrade stored
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
    abortTransactionUnless(upgrade != nil)

    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    // if the msgSender is authorized to make and cancel upgrades AND the current channel has not already reached FLUSHCOMPLETE
    // then we can restore immediately without any additional checks
    // otherwise, we can only cancel if the counterparty wrote an error receipt during the upgrade handshake
    if !(isAuthorizedUpgrader(msgSender) && currentChannel.state != FLUSHCOMPLETE) {
        abortTransactionUnless(!isEmpty(errorReceipt))

        // If counterparty sequence is less than the current sequence, abort transaction since this error receipt is from a previous upgrade
        abortTransactionUnless(errorReceipt.Sequence >= currentChannel.sequence)

        // get underlying connection for proof verification
        connection = getConnection(currentChannel.connectionIdentifier)
        // verify that the provided error receipt is written to the upgradeError path with the counterparty sequence
        abortTransactionUnless(verifyChannelUpgradeError(connection, proofHeight, proofUpgradeError, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, errorReceipt))
    }

    // cancel upgrade and write error receipt
    restoreChannel(portIdentifier, channelIdentifier)
}
```

### Timeout Upgrade Process

It is possible for the channel upgrade process to stall indefinitely while trying to flush the existing packets. To protect against this, each chain sets a timeout when it moves into `FLUSHING`. If the counterparty has not completed flushing within the expected time window, then the relayer can submit a timeout message to restore the channel to OPEN with the original parameters. It will also write an error receipt so that the counterparty which has not moved to `FLUSHCOMPLETE` can also restore channel to OPEN with the original parameters.

```typescript
function timeoutChannelUpgrade(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // current channel must have an upgrade that is FLUSHING or FLUSHCOMPLETE
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
    abortTransactionUnless(upgrade != nil)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == FLUSHING || currentChannel.state == FLUSHCOMPLETE)

    upgradeTimeout = upgrade.timeout

    // proof must be from a height after timeout has elapsed. Either timeoutHeight or timeoutTimestamp must be defined.
    // if timeoutHeight is defined and proof is from before timeout height
    // then abort transaction
    abortTransactionUnless(upgradeTimeout.timeoutHeight.IsZero() || proofHeight >= upgradeTimeout.timeoutHeight)
    // if timeoutTimestamp is defined then the consensus time from proof height must be greater than timeout timestamp
    connection = queryConnection(currentChannel.connectionIdentifier)
    abortTransactionUnless(upgradeTimeout.timeoutTimestamp.IsZero() || getTimestampAtHeight(connection, proofHeight) >= upgradeTimeout.timestamp)

    // get underlying connection for proof verification
    connection = getConnection(currentChannel.connectionIdentifier)

    // counterparty channel must be proved to not have completed flushing after timeout has passed
    abortTransactionUnless(counterpartyChannel.state !== OPEN || counterpartyChannel.state == FLUSHCOMPLETE)
    abortTransactionUnless(counterpartyChannel.sequence === currentChannel.sequence)
    abortTransactionUnless(verifyChannelState(connection, proofHeight, proofChannel, currentChannel.counterpartyPortIdentifier, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

    // we must restore the channel since the timeout verification has passed
    // error receipt is written for this sequence, counterparty can call cancelUpgradeHandshake
    restoreChannel(portIdentifier, channelIdentifier)

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error since counterparty successfully restored previous channelEnd
    module.onChanUpgradeRestore(
        portIdentifer,
        channelIdentifier
    )
}
```

Both parties must not complete the upgrade handshake if the counterparty upgrade timeout has already passed. Even if both sides could have successfully moved to FLUSHCOMPLETE. This will prevent the channel ends from reaching incompatible states.

### Considerations

Note that a channel upgrade handshake may never complete successfully if the in-flight packets cannot successfully be cleared. This can happen if the timeout value of a packet is too large, or an acknowledgement never arrives, or if there is a bug that makes acknowledging or timing out a packet impossible. In these cases, some out-of-protocol mechanism (e.g. governance) must step in to clear the packets "manually" perhaps by forcefully clearing the packet commitments before restarting the upgrade handshake.

### Migrations

A chain may have to update its internal state to be consistent with the new upgraded channel. In this case, a migration handler should be a part of the chain binary before the upgrade process so that the chain can properly migrate its state once the upgrade is successful. If a migration handler is necessary for a given upgrade but is not available, then the executing chain must reject the upgrade so as not to enter into an invalid state. This state migration will not be verified by the counterparty since it will just assume that if the channel is upgraded to a particular channel version, then the auxilliary state on the counterparty will also be updated to match the specification for the given channel version. The migration must only run once the upgrade has successfully completed and the new channel is `OPEN` (ie. on `ChanUpgradeConfirm` or `ChanUpgradeOpen`).