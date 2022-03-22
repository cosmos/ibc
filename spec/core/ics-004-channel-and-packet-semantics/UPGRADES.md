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
- The channel upgrade protocol should have the ability to change all channel-related parameters; however the channel upgrade protocol MUST NOT be able to change the underlying `ConnectionEnd`.
The channel upgrade protocol MUST NOT modify the channel identifiers.

## Technical Specification

### Data Structures

The `ChannelState` and `ChannelEnd` are defined in [ICS-4](./README.md), they are reproduced here for the reader's convenience. `UPGRADE_INIT`, `UPGRADE_TRY` are additional states added to enable the upgrade feature.

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  UPGRADE_INIT,
  UPGRADE_TRY,
}
```

- The chain that is proposing the upgrade should set the channel state from `OPEN` to `UPGRADE_INIT`
- The counterparty chain that accepts the upgrade should set the channel state from `OPEN` to `UPGRADE_TRY`

```typescript
interface ChannelEnd {
  state: ChannelState
  ordering: ChannelOrder
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  connectionHops: [Identifier]
  version: string
}
```

The desired property that the channel upgrade protocol MUST NOT modify the underlying clients or channel identifiers, means that only some fields of `ChannelEnd` are upgradable by the upgrade protocol.

- `state`: The state is specified by the handshake steps of the upgrade protocol.

MAY BE MODIFIED:
- `version`: The version MAY be modified by the upgrade protocol. The same version negotiation that happens in the initial channel handshake can be employed for the upgrade handshake.
- `ordering`: The ordering MAY be modified by the upgrade protocol. However, it MUST be the case that the previous ordering is a valid subset of the new ordering. Thus, the only supported change is from stricter ordering rules to less strict ordering. Ex: Switching from ORDERED to UNORDERED is supported, switching from UNORDERED to ORDERED is **unsupported**.
- `connectionHops`: The connectionHops MAY be modified by the upgrade protocol.

MUST NOT BE MODIFIED:
- `counterpartyChannelIdentifier`: The counterparty channel identifier MAY NOT be modified by the upgrade protocol.
- `counterpartyPortIdentifier`: The counterparty port identifier MAY NOT be modified by the upgrade protocol

NOTE: If the upgrade adds any fields to the `ChannelEnd` these are by default modifiable, and can be arbitrarily chosen by an Actor (e.g. chain governance) which has permission to initiate the upgrade.

```typescript
interface UpgradeTimeout {
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

- `timeoutHeight`: Timeout height indicates the height at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original channel and the upgrade handshake is aborted.
- `timeoutTimestamp`: Timeout timestamp indicates the time on the counterparty at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original channel and the upgrade handshake is aborted.

At least one of the timeoutHeight or timeoutTimestamp MUST be non-zero.

### Store Paths

#### Restore Channel Path

The chain must store the previous channel end so that it may restore it if the upgrade handshake fails. This may be stored in the private store.

```typescript
function restorePath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrade/ports/{portIdentifier}/channels/{channelIdentifier}/restore"
}
```

#### UpgradeError Path

The upgrade error path is a public path that can signal an error of the upgrade to the counterparty. It does not store anything in the successful case, but it will store a sentinel abort value in the case that a chain does not accept the proposed upgrade.

```typescript
function errorPath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrade/ports/{portIdentifier}/channels/{channelIdentifier}/upgradeError"

}
```

The UpgradeError MUST have an associated verification membership and nonmembership function added to the connection interface so that a counterparty may verify that chain has stored an error in the UpgradeError path.

```typescript
// Connection VerifyChannelUpgradeError method
function verifyChannelUpgradeError(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  upgradeErrorReceipt: []byte, 
) {
    client = queryClient(connection.clientIdentifier)
    path = applyPrefix(connection.counterpartyPrefix, channelErrorPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    client.verifyMembership(height, 0, 0, proof, path, upgradeErrorReceipt)
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
    client = queryClient(connection.clientIdentifier)
    path = applyPrefix(connection.counterpartyPrefix, channelErrorPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    client.verifyNonMembership(height, 0, 0, proof, path)
}
```

#### TimeoutPath

The timeout path is a public path set by the upgrade initiator to determine when the TRY step should timeout. It stores the `timeoutHeight` and `timeoutTimestamp` by which point the counterparty must have progressed to the TRY step. This path will be proven on the counterparty chain in case of a successful TRY, to ensure timeout has not passed. Or in the case of a timeout, in which case counterparty proves that the timeout has passed on its chain and restores the channel.

```typescript
function timeoutPath(portIdentifier: Identifier, channelIdentifier: Identifier) Path {
    return "channelUpgrade/ports/{portIdentifier}/channelIdentifier/{channelIdentifier}/upgradeTimeout"
}
```

The timeout path MUST have associated verification membership method on the connection interface in order for a counterparty to prove that a chain stored a particular `UpgradeTimeout`.

```typescript
// Connection VerifyChannelUpgradeTimeout method
function verifyChannelUpgradeTimeout(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  upgradeTimeout: UpgradeTimeout, 
) {
    client = queryClient(connection.clientIdentifier)
    path = applyPrefix(connection.counterpartyPrefix, channelTimeoutPath(counterpartyPortIdentifier, counterpartyChannelIdentifier))
    client.verifyChannelUpgradeTimeout(height, 0, 0, proof, path, upgradeTimeout)
}
```

## Sub-Protocols

The Channel Upgrade process consists of three sub-protocols: `UpgradeChannelHandshake`, `CancelChannelUpgrade`, and `TimeoutChannelUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the ChannelEnd should upgrade successfully.

### Utility Functions

`restoreConnectionUnless()` is a utility function that allows a chain to abort an upgrade handshake in progress, and return the `channelEnd` to its original pre-upgrade state while also setting the `errorReceipt`. A relayer can then send a `cancelUpgradeMsg` to the counterparty so that it can restore its `channelEnd` to its pre-upgrade state as well. Once both channel ends are back to the pre-upgrade state, packet processing will resume with the original channel and application parameters.

```typescript
function restoreChannel() {
    // cancel upgrade
    // write an error receipt into the error path
    // and restore original channel
    errorReceipt = []byte{1}
    provableStore.set(errorPath(portIdentifier, channelIdentifier), errorReceipt)
    originalChannel = privateStore.get(restorePath(portIdentifier, channelIdentifier))
    provableStore.set(channelPath(portIdentifier, channelIdentifier), originalChannel)
    provableStore.delete(timeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(restorePath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error and it must successfully restore
    // application to its pre-upgrade state
    module.onChanUpgradeRestore(
        portIdentifier,
        channelIdentifier
    )
    // caller should return as well
}
```

### Upgrade Handshake

The upgrade handshake defines four datagrams: *ChanUpgradeInit*, *ChanUpgradeTry*, *ChanUpgradeAck*, and *ChanUpgradeConfirm*

A successful protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)          | Posterior state (A, B)      |
| --------- | -------------------- | ---------------- | --------------------------- | --------------------------- |
| Actor     | `ChanUpgradeInit`    | A                | (OPEN, OPEN)                | (UPGRADE_INIT, OPEN)        |
| Actor     | `ChanUpgradeTry`     | B                | (UPGRADE_INIT, OPEN)        | (UPGRADE_INIT, UPGRADE_TRY) |
| Relayer   | `ChanUpgradeAck`     | A                | (UPGRADE_INIT, UPGRADE_TRY) | (OPEN, UPGRADE_TRY)         |
| Relayer   | `ChanUpgradeConfirm` | B                | (OPEN, UPGRADE_TRY)         | (OPEN, OPEN)                |

At the end of an opening handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded channel end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded channel parameters.

If a chain does not agree to the proposed counterparty `UpgradedChannel`, it may abort the upgrade handshake by writing an error receipt into the `errorPath` and restoring the original channel. The error receipt MAY be arbitrary bytes and MUST be non-empty.

`errorPath(id) => error_receipt`

A relayer may then submit a `CancelChannelUpgradeMsg` to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote a non-empty error receipt into its `UpgradeError` and if successful, it will restore its original channel as well thus cancelling the upgrade.

If an upgrade message arrives after the specified timeout, then the message MUST NOT execute successfully. Again a relayer may submit a proof of this in a `CancelChannelUpgradeTimeoutMsg` so that counterparty cancels the upgrade and restores it original channel as well.

```typescript
function chanUpgradeInit(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeChannel: ChannelEnd,
    counterpartyTimeoutHeight: Height,
    counterpartyTimeoutTimestamp: uint64,
) {
    // current channel must be OPEN
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == OPEN)

    // abort transaction if an unmodifiable field is modified
    // upgraded channel state must be in `UPGRADE_INIT`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgradeChannel.state == UPGRADE_INIT &&
        proposedUpgradeChannel.counterpartyPortIdentier == currentChannel.counterpartyPortIdentifier &&
        proposedUpgradeChannel.counterpartyChannelIdentifier == currentChannel.counterpartyChannelIdentifier
    )

    // current ordering must be a valid ordering of packets
    // in the proposed ordering
    // e.g. ORDERED -> UNORDERED, ORDERED -> DAG
    abortTransactionUnless(
        currentChannel.ordering.subsetOf(proposedUpgradeChannel.ordering)
    )

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(counterpartyTimeoutHeight != 0 || counterpartyTimeoutTimestamp != 0)

    upgradeTimeout = UpgradeTimeout{
        timeoutHeight: counterpartyTimeoutHeight,
        timeoutTimestamp: counterpartyTimeoutTimestamp,
    }

    // call modules onChanUpgradeInit callback
    module = lookupModule(portIdentifier)
    version, err = module.onChanUpgradeInit(
        proposedUpgradeChannel.order,
        proposedUpgradeChannel.connectionHops,
        proposedUpgradeChannel.portIdentifier,
        proposedUpgradeChannel.channelIdentifer,
        proposedUpgradeChannel.counterpartyPortIdentifer,
        proposedUpgradeChannel.counterpartyChannelIdentifier,
        proposedUpgradeChannel.version
    )
    // abort transaction if callback returned error
    abortTransactionUnless(err != nil)

    // replace channel version with the version returned by application
    // in case it was modified
    proposedUpgradeChannel.version = version

    provableStore.set(timeoutPath(portIdentifier, channelIdentifier), upgradeTimeout)
    provableStore.set(channelPath(portIdentifier, channelIdentifier), proposedUpgradeChannel)
    privateStore.set(restorePath(portIdentifier, channelIdentifier), currentChannel)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.
Access control on counterparty should inform choice of timeout values, i.e. timeout value should be large if counterparty's `UpgradeTry` is gated by chain governance.

```typescript
function chanUpgradeTry(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    proposedUpgradeChannel: ChannelEnd,
    timeoutHeight: Height,
    timeoutTimestamp: uint64,
    UpgradeTimeout: UpgradeTimeout,
    proofChannel: CommitmentProof,
    proofUpgradeTimeout: CommitmentProof,
    proofHeight: Height
) {
    // current channel must be OPEN or UPGRADE_INIT (crossing hellos)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == OPEN || currentChannel.state == UPGRADE_INIT)

    // abort transaction if an unmodifiable field is modified
    // upgraded channel state must be in `UPGRADE_TRY`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgradeChannel.state == UPGRADE_TRY &&
        proposedUpgradeChannel.counterpartyPortIdentifier == currentChannel.counterpartyPortIdentifier &&
        proposedUpgradeChannel.counterpartyChannelIdentifier == currentChannel.counterpartyChannelIdentifier
    )

    // current ordering must be a valid ordering of packets
    // in the proposed ordering
    // e.g. ORDERED -> UNORDERED, ORDERED -> DAG
    abortTransactionUnless(
        currentChannel.ordering.subsetOf(proposedUpgradeChannel.ordering)
    )

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(currentChannel, proofHeight, proofChannel, currentChannel.counterpartyChannelIdentifier, proposedUpgradeChannel))
    abortTransactionUnless(verifyUpgradeTimeout(currentChannel, proofHeight, proofUpgradeTimeout, currentChannel.counterpartyChannelIdentifier, upgradeTimeout))

    if currentChannel.state == UPGRADE_INIT {
        // if there is a crossing hello, ie an UpgradeInit has been called on both channelEnds,
        // then we must ensure that the proposedUpgrade by the counterparty is the same as the currentChannel
        // except for the channel state (upgrade channel will be in UPGRADE_TRY and current channel will be in UPGRADE_INIT)
        // if the proposed upgrades on either side are incompatible, then we will restore the channel and cancel the upgrade.
        currentChannel.state = UPGRADE_TRY
        if !currentChannel.IsEqual(proposedUpgradeChannel) {
            restoreChannel()
            return
        }
    } else if currentChannel.state == OPEN {
        // this is first message in upgrade handshake on this chain so we must store original channel in restore path
        // in case we need to restore channel later.
        privateStore.set(restorePath(portIdentifier, channelIdentifier), currentChannel)
    } else {
        // abort transaction if current channel is not in state: UPGRADE_INIT or OPEN
        abortTransactionUnless(false)
    }

    // either timeout height or timestamp must be non-zero
    // if the upgrade feature is implemented on the TRY chain, then a relayer may submit a TRY transaction after the timeout.
    // this will restore the channel on the executing chain and allow counterparty to use the CancelUpgradeMsg to restore their channel.
    if timeoutHeight == 0 && timeoutTimestamp == 0 {
        restoreChannel()
        return
    }
    upgradeTimeout = UpgradeTimeout{
        timeoutHeight: timeoutHeight,
        timeoutTimestamp: timeoutTimestamp,
    }

    // counterparty-specified timeout must not have exceeded
    if (currentHeight() > timeoutHeight && timeoutHeight != 0) ||
        (currentTimestamp() > timeoutTimestamp && timeoutTimestamp != 0) {
        restoreChannel()
        return
    }

    // both channel ends must be mutually compatible.
    // this function has been left unspecified since it will depend on the specific structure of the new channel.
    // It is the responsibility of implementations to make sure that verification that the proposed new channels
    // on either side are correctly constructed according to the new version selected.
    if !IsCompatible(counterpartyChannel, proposedUpgradeChannel) {
        restoreChannel()
        return
    }

    // call modules onChanUpgradeTry callback
    module = lookupModule(portIdentifier)
    version, err = module.onChanUpgradeTry(
        proposedUpgradeChannel.order,
        proposedUpgradeChannel.connectionHops,
        proposedUpgradeChannel.portIdentifier,
        proposedUpgradeChannel.channelIdentifer,
        proposedUpgradeChannel.counterpartyPortIdentifer,
        proposedUpgradeChannel.counterpartyChannelIdentifier,
        proposedUpgradeChannel.version
    )
    // restore channel if callback returned error
    if err != nil {
        restoreChannel()
        return
    }

    // replace channel version with the version returned by application
    // in case it was modified
    proposedUpgradeChannel.version = version
 
    provableStore.set(channelPath(portIdentifier, channelIdentifier), proposedUpgradeChannel)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeTry` function. E.g. chain governance, permissioned actor, DAO, etc. A chain may decide to have permissioned **or** permissionless `UpgradeTry`. In the permissioned case, both chains must explicitly consent to the upgrade, in the permissionless case; one chain initiates the upgrade and the other chain agrees to the upgrade by default. In the permissionless case, a relayer may submit the `ChanUpgradeTry` datagram.


```typescript
function chanUpgradeAck(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofHeight: Height
) {
    // current channel is in UPGRADE_INIT or UPGRADE_TRY (crossing hellos)
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(currentChannel.state == UPGRADE_INIT || currentChannel.state == UPGRADE_TRY)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(currentChannel, proofHeight, proofChannel, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

    // counterparty must be in TRY state
    if counterpartyChannel.State != UPGRADE_TRY {
        restoreChannel()
        return
    }

    // verify channels are mutually compatible
    // this will also check counterparty chosen version is valid
    // this function has been left unspecified since it will depend on the specific structure of the new channel.
    // It is the responsibility of implementations to make sure that verification that the proposed new channels
    // on either side are correctly constructed according to the new version selected.
    if !IsCompatible(counterpartyChannel, channel) {
        restoreChannel()
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
        restoreChannel()
        return
    }

    // upgrade is complete
    // set channel to OPEN and remove unnecessary state
    currentChannel.state = OPEN
    provableStore.set(channelPath(portIdentifier, channelIdentifier), currentChannel)
    provableStore.delete(timeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(restorePath(portIdentifier, channelIdentifier))
}
```

```typescript
function chanUpgradeConfirm(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in UPGRADE_TRY
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == UPGRADE_TRY)

    // counterparty must be in OPEN state
    abortTransactionUnless(counterpartyChannel.State == OPEN)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(currentChannel, proofHeight, proofChannel, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

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
    provableStore.delete(timeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(restorePath(portIdentifier, channelIdentifier))
}
```


### Cancel Upgrade Process

During the upgrade handshake a chain may cancel the upgrade by writing an error receipt into the error path and restoring the original channel to `OPEN`. The counterparty must then restore its channel to `OPEN` as well. A relayer can facilitate this by calling `CancelChannelUpgrade`:

```typescript
function cancelChannelUpgrade(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    errorReceipt: []byte,
    proofUpgradeError: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in UPGRADE_INIT or UPGRADE_TRY
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnless(channel.state == UPGRADE_INIT || channel.state == UPGRADE_TRY)

    abortTransactionUnless(!isEmpty(errorReceipt))

    abortTransactionUnless(verifyUpgradeError(currentChannel, proofHeight, proofUpgradeError, currentChannel.counterpartyChannelIdentifier, errorReceipt))

    // cancel upgrade
    // and restore original conneciton
    // delete unnecessary state
    originalChannel = privateStore.get(restorePath(portIdentifier, channelIdentifier))
    provableStore.set(channelPath(portIdentifier, channelIdentifier), originalChannel)

    // delete auxilliary upgrade state
    provableStore.delete(timeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(restorePath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error since counterparty successfully upgraded
    module.onChanUpgradeRestore(
        portIdentifer,
        channelIdentifier
    )
}
```

### Timeout Upgrade Process

It is possible for the channel upgrade process to stall indefinitely on UPGRADE_TRY if the UPGRADE_TRY transaction simply cannot pass on the counterparty; for example, the upgrade feature may not be enabled on the counterparty chain.

In this case, we do not want the initializing chain to be stuck indefinitely in the `UPGRADE_INIT` step. Thus, the `UpgradeInit` message will contain a `TimeoutHeight` and `TimeoutTimestamp`. The counterparty chain is expected to reject `UpgradeTry` message if the specified timeout has already elapsed.

A relayer must then submit an `UpgradeTimeout` message to the initializing chain which proves that the counterparty is still in its original state. If the proof succeeds, then the initializing chain shall also restore its original channel and cancel the upgrade.

```typescript
function timeoutChannelUpgrade(
    portIdentifier: Identifier,
    channelIdentifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // current channel must be in UPGRADE_INIT
    currentChannel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
    abortTransactionUnles(currentChannel.state == UPGRADE_INIT)

    upgradeTimeout = provableStore.get(timeoutPath(portIdentifier, channelIdentifier))

    // proof must be from a height after timeout has elapsed. Either timeoutHeight or timeoutTimestamp must be defined.
    // if timeoutHeight is defined and proof is from before timeout height
    // then abort transaction
    abortTransactionUnless(upgradeTimeout.timeoutHeight.IsZero() || proofHeight >= upgradeTimeout.timeoutHeight)
    // if timeoutTimestamp is defined then the consensus time from proof height must be greater than timeout timestamp
    connection = queryConnection(currentChannel.connectionIdentifier)
    abortTransactionUnless(upgradeTimeout.timeoutTimestamp.IsZero() || getTimestampAtHeight(connection, proofHeight) >= upgradeTimeout.timestamp)

    // counterparty channel must be proved to still be in OPEN state
    abortTransactionUnless(counterpartyChannel.State === OPEN)
    abortTransactionUnless(channel.client.verifyChannelState(proofHeight, proofChannel, counterpartyChannel))

    // we must restore the channel since the timeout verification has passed
    originalChannel = privateStore.get(restorePath(portIdentifier, channelIdentifier))
    provableStore.set(channelPath(portIdentifier, channelIdentifier), originalChannel)

    // delete auxilliary upgrade state
    provableStore.delete(timeoutPath(portIdentifier, channelIdentifier))
    privateStore.delete(restorePath(portIdentifier, channelIdentifier))

    // call modules onChanUpgradeRestore callback
    module = lookupModule(portIdentifier)
    // restore callback must not return error since counterparty successfully upgraded
    module.onChanUpgradeRestore(
        portIdentifer,
        channelIdentifier
    )
}
```

Note that the timeout logic only applies to the INIT step. This is to protect an upgrading chain from being stuck in a non-OPEN state if the counterparty cannot execute the TRY successfully. Once the TRY step succeeds, then both sides are guaranteed to have the upgrade feature enabled. Liveness is no longer an issue, because we can wait until liveness is restored to execute the ACK step which will move the channel definitely into an OPEN state (either a successful upgrade or a rollback).

The TRY chain will receive the timeout parameters chosen by the counterparty on INIT, so that it can reject any TRY message that is received after the specified timeout. This prevents the handshake from entering into an invalid state, in which the INIT chain processes a timeout successfully and restores its channel to `OPEN` while the TRY chain at a later point successfully writes a `TRY` state.

### Migrations

A chain may have to update its internal state to be consistent with the new upgraded channel. In this case, a migration handler should be a part of the chain binary before the upgrade process so that the chain can properly migrate its state once the upgrade is successful. If a migration handler is necessary for a given upgrade but is not available, then th executing chain must reject the upgrade so as not to enter into an invalid state. This state migration will not be verified by the counterparty since it will just assume that if the channel is upgraded to a particular channel version, then the auxilliary state on the counterparty will also be updated to match the specification for the given channel version. The migration must only run once the upgrade has successfully completed and the new channel is `OPEN` (ie. on `ACK` and `CONFIRM`).