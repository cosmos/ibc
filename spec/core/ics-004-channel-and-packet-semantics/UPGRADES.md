# Upgrading Channels

### Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing channels to upgrade after the initial channel handshake.

### Motivation

As new features get added to IBC, chains may wish the take advantage of new channel features without abandoning the accumulated state and network effect(s) of an already existing channel. The upgrade protocol proposed would allow chains to renegotiate an existing channel to take advantage of new features without having to create a new channel, thus preserving all existing channels that built on top of the channel.

### Desired Properties

- Both chains MUST agree to the renegotiated channel parameters.
- Channel state and logic on both chains SHOULD either be using the old parameters or the new parameters, but MUST NOT be in an in-between state, e.g., it MUST NOT be possible for a chain to write state to an old proof path, while the counterparty expects a new proof path.
- The channel upgrade protocol is atomic, i.e., 
  - either it is unsuccessful and then the channel MUST fall-back to the original channel parameters; 
  - or it is successful and then both channel ends MUST adopt the new channel parameters and process IBC data appropriately.
- The channel upgrade protocol should have the ability to change all channel-related parameters; however the channel upgrade protocol MUST NOT be able to change the underlying `ClientState`.
The channel upgrade protocol MUST NOT modify the channel identifiers.

## Technical Specification

### Data Structures

The `ChannelState` and `ChannelEnd` are defined in [ICS-3](./README.md), they are reproduced here for the reader's convenience. `UPGRADE_INIT`, `UPGRADE_TRY`, `UPGRADE_ERR` are additional states added to enable the upgrade feature.

```typescript
enum ChannelState {
  INIT,
  TRYOPEN,
  OPEN,
  UPGRADE_INIT,
  UPGRADE_TRY,
  UPGRADE_ERR,
}
```

- The chain that is proposing the upgrade should set the channel state from `OPEN` to `UPGRADE_INIT`
- The counterparty chain that accepts the upgrade should set the channel state from `OPEN` to `UPGRADE_TRY`

```typescript
interface ChannelEnd {
  state: ChannelState
  counterpartyChannelIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string | []string
  delayPeriodTime: uint64
  delayPeriodBlocks: uint64
}
```

The desired property that the channel upgrade protocol MUST NOT modify the underlying clients or channel identifiers, means that only some fields of `ChannelEnd` are upgradable by the upgrade protocol.

- `state`: The state is specified by the handshake steps of the upgrade protocol.

MAY BE MODIFIED:
- `counterpartyPrefix`: The prefix MAY be modified in the upgrade protocol. The counterparty must accept the new proposed prefix value, or it must return an error during the upgrade handshake.
- `version`: The version MAY be modified by the upgrade protocol. The same version negotiation that happens in the initial channel handshake can be employed for the upgrade handshake.
- `delayPeriodTime`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.
- `delayPeriodBlocks`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.

MUST NOT BE MODIFIED:
- `counterpartyChannelIdentifier`: The counterparty channel identifier CAN NOT be modified by the upgrade protocol.
- `clientIdentifier`: The client identifier CAN NOT be modified by the upgrade protocol
- `counterpartyClientIdentifier`: The counterparty client identifier CAN NOT be modified by the upgrade protocol

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
function restorePath(id: Identifier): Path {
    return "channels/{id}/restore"
}
```

#### UpgradeError Path

The upgrade error path is a public path that can signal an error of the upgrade to the counterparty. It does not store anything in the successful case, but it will store a sentinel abort value in the case that a chain does not accept the proposed upgrade.

```typescript
function errorPath(id: Identifier): Path {
    return "channels/{id}/upgradeError"

}
```

The UpgradeError MUST have an associated verification function added to the channel and client interfaces so that a counterparty may verify that chain has stored an error in the UpgradeError path.

```typescript
// Channel VerifyUpgradeError method
function verifyUpgradeError(
  channel: ChannelEnd,
  height: Height,
  proof: CommitmentProof,
  upgradeErrorReceipt: []byte, 
) {
    client = queryClient(channel.clientIdentifier)
    client.verifyUpgradeError(height, channel.counterpartyPrefix, proof, channel.counterpartyChannelIdentifier, upgradeErrorReceipt)
}
```

```typescript
// Client VerifyUpgradeError
function verifyUpgradeError(
    clientState: ClientState,
    height: Height,
    prefix: CommitmentPrefix,
    proof: CommitmentProof,
    counterpartyChannelIdentifier: Identifier,
    upgradeErrorReceipt []byte,
) {
    path = applyPrefix(prefix, errorPath(counterpartyChannelIdentifier))
    abortTransactionUnless(!clientState.frozen)
    return clientState.verifiedRoots[height].verifyMembership(path, upgradeErrorReceipt, proof)
}
```

#### TimeoutPath

The timeout path is a public path set by the upgrade initiator to determine when the TRY step should timeout. It stores the `timeoutHeight` and `timeoutTimestamp` by which point the counterparty must have progressed to the TRY step. This path will be proven on the counterparty chain in case of a successful TRY, to ensure timeout has not passed. Or in the case of a timeout, in which case counterparty proves that the timeout has passed on its chain and restores the channel.

```typescript
function timeoutPath(id: Identifier) Path {
    return "channels/{id}/upgradeTimeout"
}
```

The timeout path MUST have associated verification methods on the channel and client interfaces in order for a counterparty to prove that a chain stored a particular `UpgradeTimeout`.

```typescript
// Channel VerifyUpgradeTimeout method
function verifyUpgradeTimeout(
  channel: ChannelEnd,
  height: Height,
  proof: CommitmentProof,
  upgradeTimeout: UpgradeTimeout, 
) {
    client = queryClient(channel.clientIdentifier)
    client.verifyUpgradeTimeout(height, channel.counterpartyPrefix, proof, channel.counterpartyChannelIdentifier, upgradeTimeout)
}
```

```typescript
// Client VerifyUpgradeTimeout
function verifyUpgradeTimeout(
    clientState: ClientState,
    height: Height,
    prefix: CommitmentPrefix,
    proof: CommitmentProof,
    counterpartyChannelIdentifier: Identifier,
    upgradeTimeout: UpgradeTimeout,
) {
    path = applyPrefix(prefix, timeoutPath(counterpartyChannelIdentifier))
    abortTransactionUnless(!clientState.frozen)
    timeoutBytes = protobuf.marshal(upgradeTimeout)
    return clientState.verifiedRoots[height].verifyMembership(path, timeoutBytes, proof)
}
```

## Sub-Protocols

The Channel Upgrade process consists of three sub-protocols: `UpgradeHandshake`, `CancelChannelUpgrade`, and `TimeoutChannelUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the ChannelEnd should upgrade successf

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
function connUpgradeInit(
    identifier: Identifier,
    proposedUpgradeChannel: ChannelEnd,
    counterpartyTimeoutHeight: Height,
    counterpartyTimeoutTimestamp: uint64,
) {
    // current channel must be OPEN
    currentChannel = provableStore.get(channelPath(identifier))
    abortTransactionUnless(channel.state == OPEN)

    // abort transaction if an unmodifiable field is modified
    // upgraded channel state must be in `UPGRADE_INIT`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgradeChannel.state == UPGRADE_INIT &&
        proposedUpgradeChannel.counterpartyChannelIdentifier == currentChannel.counterpartyChannelIdentifier &&
        proposedUpgradeChannel.clientIdentifier == currentChannel.clientIdentifier &&
        proposedUpgradeChannel.counterpartyClientIdentifier == currentChannel.counterpartyClientIdentifier
    )

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(counterpartyTimeoutHeight != 0 || counterpartyTimeoutTimestamp != 0)

    upgradeTimeout = UpgradeTimeout{
        timeoutHeight: counterpartyTimeoutHeight,
        timeoutTimestamp: counterpartyTimeoutTimestamp,
    }

    provableStore.set(timeoutPath(identifier), upgradeTimeout)
    provableStore.set(channelPath(identifier), proposedUpgrade.channel)
    privateStore.set(restorePath(identifier), currentChannel)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.
Access control on counterparty should inform choice of timeout values, i.e. timeout value should be large if counterparty's `UpgradeTry` is gated by chain governance.

```typescript
function connUpgradeTry(
    identifier: Identifier,
    proposedUpgrade: UpgradeChannelState,
    counterpartyChannel: ChannelEnd,
    timeoutHeight: Height,
    timeoutTimestamp: uint64,
    UpgradeTimeout: UpgradeTimeout,
    proofChannel: CommitmentProof,
    proofUpgradeTimeout: CommitmentProof,
    proofHeight: Height
) {
    // current channel must be OPEN or UPGRADE_INIT (crossing hellos)
    currentChannel = provableStore.get(channelPath(identifier))
    abortTransactionUnless(currentChannel.state == OPEN || currentChannel.state == UPGRADE_INIT)

    if currentChannel.state == UPGRADE_INIT {
        // if there is a crossing hello, ie an UpgradeInit has been called on both channelEnds,
        // then we must ensure that the proposedUpgrade by the counterparty is the same as the currentChannel
        // except for the channel state (upgrade channel will be in UPGRADE_TRY and current channel will be in UPGRADE_INIT)
        // if the proposed upgrades on either side are incompatible, then we will restore the channel and cancel the upgrade.
        currentChannel.state = UPGRADE_TRY
        restoreChannelUnless(currentChannel.IsEqual(proposedUpgrade.channel))
    } else {
        // this is first message in upgrade handshake on this chain so we must store original channel in restore path
        // in case we need to restore channel later.
        privateStore.set(restorePath(identifier), currentChannel)
    }

    // abort transaction if an unmodifiable field is modified
    // upgraded channel state must be in `UPGRADE_TRY`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgrade.channel.state == UPGRADE_TRY &&
        proposedUpgrade.channel.counterpartyChannelIdentifier == currentChannel.counterpartyChannelIdentifier &&
        proposedUpgrade.channel.clientIdentifier == currentChannel.clientIdentifier &&
        proposedUpgrade.channel.counterpartyClientIdentifier == currentChannel.counterpartyClientIdentifier
    )

    
    // either timeout height or timestamp must be non-zero
    // if the upgrade feature is implemented on the TRY chain, then a relayer may submit a TRY transaction after the timeout.
    // this will restore the channel on the executing chain and allow counterparty to use the CancelUpgradeMsg to restore their channel.
    restoreChannelUnless(timeoutHeight != 0 || timeoutTimestamp != 0)
    upgradeTimeout = UpgradeTimeout{
        timeoutHeight: timeoutHeight,
        timeoutTimestamp: timeoutTimestamp,
    }

    // verify that counterparty channel unmodifiable fields have not changed and counterparty state
    // is UPGRADE_INIT
    restoreChannelUnless(
        counterpartyChannel.state == UPGRADE_INIT &&
        counterpartyChannel.counterpartyChannelIdentifier == identifier &&
        counterpartyChannel.clientIdentifier == currentChannel.counterpartyClientIdentifier &&
        counterpartyChannel.counterpartyClientIdentifier == currentChannel.clientIdentifier
    )

    // counterparty-specified timeout must not have exceeded
    restoreChannelUnless(
        timeoutHeight < currentHeight() &&
        timeoutTimestamp < currentTimestamp()
    )

    // verify chosen versions are compatible
    versionsIntersection = intersection(counterpartyChannel.version, proposedUpgrade.Channel.version)
    version = pickVersion(versionsIntersection) // aborts transaction if there is no intersection

    // both channel ends must be mutually compatible.
    // this function has been left unspecified since it will depend on the specific structure of the new channel.
    // It is the responsibility of implementations to make sure that verification that the proposed new channels
    // on either side are correctly constructed according to the new version selected.
    restoreChannelUnless(IsCompatible(counterpartyChannel, proposedUpgrade.Channel))

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(currentChannel, proofHeight, proofChannel, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    abortTransactionUnless(verifyUpgradeTimeout(currentChannel, proofHeight, proofUpgradeTimeout, currentChannel.counterpartyChannelIdentifier, upgradeTimeout))
 
    provableStore.set(channelPath(identifier), proposedUpgrade.channel)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ChanUpgradeTry` function. E.g. chain governance, permissioned actor, DAO, etc. A chain may decide to have permissioned **or** permissionless `UpgradeTry`. In the permissioned case, both chains must explicitly consent to the upgrade, in the permissionless case; one chain initiates the upgrade and the other chain agrees to the upgrade by default. In the permissionless case, a relayer may submit the `ChanUpgradeTry` datagram.


```typescript
function onChanUpgradeAck(
    identifier: Identifier,
    counterpartyChannel: ChannelEnd,
    counterpartyStatus: UpgradeError,
    proofChannel: CommitmentProof,
    proofUpgradeError: CommitmentProof,
    proofHeight: Height
) {
    // current channel is in UPGRADE_INIT or UPGRADE_TRY (crossing hellos)
    currentChannel = provableStore.get(channelPath(identifier))
    abortTransactionUnless(currentChannel.state == UPGRADE_INIT || currentChannel.state == UPGRADE_TRY)

    // counterparty must be in TRY state
    restoreChannelUnless(counterpartyChannel.State == UPGRADE_TRY)

    // verify channels are mutually compatible
    // this will also check counterparty chosen version is valid
    // this function has been left unspecified since it will depend on the specific structure of the new channel.
    // It is the responsibility of implementations to make sure that verification that the proposed new channels
    // on either side are correctly constructed according to the new version selected.
    restoreChannelUnless(IsCompatible(counterpartyChannel, channel))

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(currentChannel, proofHeight, proofChannel, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))

    // upgrade is complete
    // set channel to OPEN and remove unnecessary state
    currentChannel.state = OPEN
    provableStore.set(channelPath(identifier), currentChannel)
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

```typescript
function onChanUpgradeConfirm(
    identifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in UPGRADE_TRY
    currentChannel = provableStore.get(channelPath(identifier))
    abortTransactionUnless(channel.state == UPGRADE_TRY)

    // counterparty must be in OPEN state
    abortTransactionUnless(counterpartyChannel.State == OPEN)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyChannelState(currentChannel, proofHeight, proofChannel, currentChannel.counterpartyChannelIdentifier, counterpartyChannel))
    
    // upgrade is complete
    // set channel to OPEN and remove unnecessary state
    currentChannel.state = OPEN
    provableStore.set(channelPath(identifier), currentChannel)
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

```typescript
function restoreChannelUnless(condition: bool) {
    if !condition {
        // cancel upgrade
        // write an error receipt into the error path
        // and restore original channel
        errorReceipt = []byte{1}
        provableStore.set(errorPath(identifier), errorReceipt)
        originalChannel = privateStore.get(restorePath(identifier))
        provableStore.set(channelPath(identifier), originalChannel)
        provableStore.delete(timeoutPath(identifier))
        privateStore.delete(restorePath(identifier))
        // caller should return as well
    } else {
        // caller should continue execution
    }
}
```

### Cancel Upgrade Process

During the upgrade handshake a chain may cancel the upgrade by writing an error receipt into the error path and restoring the original channel to `OPEN`. The counterparty must then restore its channel to `OPEN` as well. A relayer can facilitate this by calling `CancelChannelUpgrade`:

```typescript
function cancelChannelUpgrade(
    identifier: Identifer,
    errorReceipt: []byte,
    counterpartyUpgradeError: UpgradeError,
    proofUpgradeError: CommitmentProof,
    proofHeight: Height,
) {
    // current channel is in UPGRADE_INIT or UPGRADE_TRY
    currentChannel = provableStore.get(channelPath(identifier))
    abortTransactionUnless(channel.state == UPGRADE_INIT || channel.state == UPGRADE_TRY)

    abortTransactionUnless(!isEmpty(errorReceipt))

    abortTransactionUnless(verifyUpgradeError(currentChannel, proofHeight, proofUpgradeError, currentChannel.counterpartyChannelIdentifier, counterpartyUpgradeError))

    // cancel upgrade
    // and restore original conneciton
    // delete unnecessary state
    originalChannel = privateStore.get(restorePath(identifier))
    provableStore.set(channelPath(identifier), originalChannel)

    // delete auxilliary upgrade state
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

### Timeout Upgrade Process

It is possible for the channel upgrade process to stall indefinitely on UPGRADE_TRY if the UPGRADE_TRY transaction simply cannot pass on the counterparty; for example, the upgrade feature may not be enabled on the counterparty chain.

In this case, we do not want the initializing chain to be stuck indefinitely in the `UPGRADE_INIT` step. Thus, the `UpgradeInit` message will contain a `TimeoutHeight` and `TimeoutTimestamp`. The counterparty chain is expected to reject `UpgradeTry` message if the specified timeout has already elapsed.

A relayer must then submit an `UpgradeTimeout` message to the initializing chain which proves that the counterparty is still in its original state. If the proof succeeds, then the initializing chain shall also restore its original channel and cancel the upgrade.

```typescript
function timeoutChannelUpgrade(
    identifier: Identifier,
    counterpartyChannel: ChannelEnd,
    proofChannel: CommitmentProof,
    proofHeight: Height,
) {
    // current channel must be in UPGRADE_INIT
    currentChannel = provableStore.get(channelPath(identifier))
    abortTransactionUnles(currentChannel.state == UPGRADE_INIT)

    upgradeTimeout = provableStore.get(timeoutPath(identifier))

    // proof must be from a height after timeout has elapsed. Either timeoutHeight or timeoutTimestamp must be defined.
    // if timeoutHeight is defined and proof is from before timeout height
    // then abort transaction
    abortTransactionUnless(upgradeTimeout.timeoutHeight.IsZero() || proofHeight >= upgradeTimeout.timeoutHeight)
    // if timeoutTimestamp is defined then the consensus time from proof height must be greater than timeout timestamp
    consensusState = queryConsensusState(currentChannel.clientIdentifer, proofHeight)
    abortTransactionUnless(upgradeTimeout.timeoutTimestamp.IsZero() || consensusState.getTimestamp() >= upgradeTimeout.timestamp)

    // counterparty channel must be proved to still be in OPEN state
    abortTransactionUnless(counterpartyChannel.State === OPEN)
    abortTransactionUnless(channel.client.verifyChannelState(proofHeight, proofChannel, counterpartyChannel))

    // we must restore the channel since the timeout verification has passed
    restoreChannelUnless(false)
}
```

Note that the timeout logic only applies to the INIT step. This is to protect an upgrading chain from being stuck in a non-OPEN state if the counterparty cannot execute the TRY successfully. Once the TRY step succeeds, then both sides are guaranteed to have the upgrade feature enabled. Liveness is no longer an issue, because we can wait until liveness is restored to execute the ACK step which will move the channel definitely into an OPEN state (either a successful upgrade or a rollback).

The TRY chain will receive the timeout parameters chosen by the counterparty on INIT, so that it can reject any TRY message that is received after the specified timeout. This prevents the handshake from entering into an invalid state, in which the INIT chain processes a timeout successfully and restores its channel to `OPEN` while the TRY chain at a later point successfully writes a `TRY` state.

### Migrations

A chain may have to update its internal state to be consistent with the new upgraded channel. In this case, a migration handler should be a part of the chain binary before the upgrade process so that the chain can properly migrate its state once the upgrade is successful. If a migration handler is necessary for a given upgrade but is not available, then th executing chain must reject the upgrade so as not to enter into an invalid state. This state migration will not be verified by the counterparty since it will just assume that if the channel is upgraded to a particular channel version, then the auxilliary state on the counterparty will also be updated to match the specification for the given channel version. The migration must only run once the upgrade has successfully completed and the new channel is `OPEN` (ie. on `ACK` and `CONFIRM`).