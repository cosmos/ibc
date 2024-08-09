# Upgrading Channels

## Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing channels to upgrade after the initial channel handshake.

## Motivation

As new features get added to IBC, chains may wish to take advantage of new channel features without abandoning the accumulated state and network effect(s) of an already existing channel. The upgrade protocol proposed would allow chains to renegotiate an existing channel to take advantage of new features without having to create a new channel, thus preserving all existing packet state processed on the channel.

## Desired Properties

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

- In `ChanUpgradeInit`, the initializing chain that is proposing the upgrade should store the channel upgrade.
- The counterparty chain executing `ChanUpgradeTry` that accepts the upgrade should store the channel upgrade, set the channel state from `OPEN` to `FLUSHING`, and start the flushing timer by storing an upgrade timeout.
- Once the initiating chain verifies the counterparty is in `FLUSHING`, it must also move to `FLUSHING` unless all in-flight packets are already flushed on its end, in which case it must move directly to `FLUSHCOMPLETE`. The initiator will also store the counterparty timeout to ensure it does not move to `FLUSHCOMPLETE` after the counterparty timeout has passed.
- The counterparty chain must prove that the initiator is also in `FLUSHING` or completed flushing in `FLUSHCOMPLETE`. The counterparty will store the initiator timeout to ensure it does not move to `FLUSHCOMPLETE` after the initiator timeout has passed.

`FLUSHING` is a "blocking" state that prevents a channel end from advancing to `FLUSHCOMPLETE` unless the in-flight packets on its channel end are flushed and both channel ends have already moved to `FLUSHING`. Once both sides have moved to `FLUSHCOMPLETE`, a relayer can prove this on both ends with `ChanUpgradeOpen` to open the channel on both sides with the new parameters.

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

- `state`: The state is specified by the handshake steps of the upgrade protocol and will be mutated in place during the handshake. It will be in `FLUSHING` mode when the channel end is flushing in-flight packets. The state will change to `FLUSHCOMPLETE` once there are no in-flight packets left and the channelEnd is ready to move to `OPEN`.
- `upgradeSequence`: The upgrade sequence will be incremented and agreed upon during the upgrade handshake and will be mutated in place.

All other parameters will remain the same during the upgrade handshake until the upgrade handshake completes. When the channel is reset to `OPEN` on a successful upgrade handshake, the fields on the channel end will be switched over to the `UpgradeFields` specified in the `Upgrade`.

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

#### `Timeout`

```typescript
interface Timeout {
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
  timeout: Timeout
  nextSequenceSend: uint64
}
```

The upgrade contains the proposed upgrade for the channel end on the executing chain, the timeout for the upgrade attempt, and the next packet send sequence for the channel. The `nextSequenceSend` allows the counterparty to know which packets need to be flushed before the channel can reopen with the newly negotiated parameters. Any packet sent to the channel end with a packet sequence greater than or equal to the `nextSequenceSend` will be rejected until the upgrade is complete. The `nextSequenceSend` will also be used to set the new sequences for the counterparty when it opens for a new upgrade.

#### `ErrorReceipt`

```typescript
interface ErrorReceipt {
  sequence: uint64
  errorMsg: string
}
```

- `sequence` contains the `upgradeSequence` at which the error occurred.
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
  path = applyPrefix(
    connection.counterpartyPrefix, 
    channelUpgradePath(counterpartyPortIdentifier, counterpartyChannelIdentifier)
  )
  return verifyMembership(clientState, height, 0, 0, proof, path, upgrade)
}
```

#### CounterpartyUpgrade Path

The chain must store the counterparty upgrade on `chanUpgradeAck` and `chanUpgradeConfirm`. This will be stored in the `counterpartyUpgrade` path on the private store.

```typescript
function counterpartyUpgradePath(portIdentifier: Identifier, channelIdentifier: Identifier): Path {
    return "channelUpgrades/counterpartyUpgrade/ports/{portIdentifier}/channels/{channelIdentifier}"
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
  path = applyPrefix(
    connection.counterpartyPrefix, 
    channelUpgradeErrorPath(counterpartyPortIdentifier, counterpartyChannelIdentifier)
  )
  return verifyMembership(clientState, height, 0, 0, proof, path, upgradeErrorReceipt)
}
```

## Sub-Protocols

The channel upgrade process consists of the following sub-protocols: `initUpgradeHandshake`, `startFlushUpgradeHandshake`, `openUpgradeHandshake`, `cancelChannelUpgrade`, and `timeoutChannelUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the `ChannelEnd` should upgrade to the new parameters in `OPEN` state.

### Utility Functions

`initUpgradeHandshake` is a sub-protocol that will initialize the channel end for the upgrade handshake. It will validate the upgrade parameters and store the channel upgrade. All packet processing will continue according to the original channel parameters, as this is a signalling mechanism that can remain indefinitely. The new proposed upgrade will be stored in the provable store for counterparty verification. If it is called again before the handshake starts, then the current proposed upgrade will be replaced with the new one and the channel upgrade sequence will be incremented.

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
  abortTransactionUnless(channel.state === OPEN)

  // new channel version must be nonempty
  abortTransactionUnless(proposedUpgradeFields.Version !== "")

  // proposedConnection must exist and be in OPEN state for 
  // channel upgrade to be accepted
  proposedConnection = provableStore.get(connectionPath(proposedUpgradeFields.connectionHops[0]))
  abortTransactionUnless(proposedConnection !== null && proposedConnection.state === OPEN)

  // new order must be supported by the new connection
  abortTransactionUnless(isSupported(proposedConnection, proposedUpgradeFields.ordering))

  // nextSequenceSend and timeout will be filled when we move to FLUSHING
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

`isCompatibleUpgradeFields` will return true if two upgrade field structs are mutually compatible as counterparties, and false otherwise. The first field must be the upgrade fields on the executing chain, the second field must be the counterparty upgrade fields. This function will also check that the proposed connection hops exists, is `OPEN`, and is mutually compatible with the counterparty connection hops.

```typescript
function isCompatibleUpgradeFields(
  proposedUpgradeFields: UpgradeFields,
  counterpartyUpgradeFields: UpgradeFields,
): boolean {
  if (proposedUpgradeFields.ordering != counterpartyUpgradeFields.ordering) {
    return false
  }
  if (proposedUpgradeFields.version != counterpartyUpgradeFields.version) {
    return false
  }

  // connectionHops can change in a channel upgrade, however both sides must
  // still be each other's counterparty. Since connection hops may be provided
  // by relayer, we will abort to avoid changing state based on relayer-provided value
  // Note: If the proposed connection came from an existing upgrade, then the 
  // off-chain authority is responsible for replacing one side's upgrade fields
  // to be compatible so that the upgrade handshake can proceed
  proposedConnection = provableStore.get(connectionPath(proposedUpgradeFields.connectionHops[0]))
  if (proposedConnection == null || proposedConnection.state != OPEN) {
    return false
  }
  if (counterpartyUpgradeFields.connectionHops[0] != proposedConnection.counterpartyConnectionIdentifier) {
    return false
  }
  return true
}
```

`startFlushUpgradeHandshake` will block the upgrade from continuing until all in-flight packets have been flushed. It will set the channel state to `FLUSHING` and block `sendPacket`. During this time; `receivePacket`, `acknowledgePacket` and `timeoutPacket` will still be allowed and processed according to the original channel parameters. The state machine will set a timer for how long the other side can take before it completes flushing and moves to `FLUSHCOMPLETE`. The new proposed upgrade will be stored in the public store for counterparty verification.

```typescript
// startFlushUpgradeHandshake will verify that the channel
// is in a valid precondition for calling the startFlushUpgradeHandshake.
// it will set the channel to flushing state.
// it will store the nextSequenceSend and upgrade timeout in the upgrade state.
function startFlushUpgradeHandshake(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
) {
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state === OPEN)

  upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
  abortTransactionUnless(upgrade !== null)

  channel.state = FLUSHING

  upgradeTimeout = getUpgradeTimeout(channel.portIdentifier, channel.channelIdentifier)
  // either timeout height or timestamp must be non-zero
  abortTransactionUnless(upgradeTimeout.timeoutHeight != 0 || upgradeTimeout.timeoutTimestamp != 0)

  nextSequenceSend = provableStore.get(nextSequenceSendPath(portIdentifier, channelIdentifier))

  upgrade.timeout = upgradeTimeout
  upgrade.nextSequenceSend = nextSequenceSend
  
  // store upgrade in public store for counterparty proof verification
  provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
  provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

`openUpgradeHandshake` will open the channel and switch the existing channel parameters to the newly agreed-upon upgraded channel fields.

```typescript
// openUpgradeHandshake will switch the channel fields 
// over to the agreed upon upgrade fields.
// it will reset the channel state to OPEN.
// it will delete auxiliary upgrade state.
// caller must do all relevant checks before calling this function.
function openUpgradeHandshake(
  portIdentifier: Identifier,
  channelIdentifier: Identifier
) {
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

  // if channel order changed, we need to set
  // the recv and ack sequences appropriately
  if channel.order == "UNORDERED" && upgrade.fields.ordering == "ORDERED" {
    selfNextSequenceSend = provableStore.get(nextSequenceSendPath(portIdentifier, channelIdentifier))
    counterpartyUpgrade = privateStore.get(counterpartyUpgradePath(portIdentifier, channelIdentifier))

    // set nextSequenceRecv to the counterparty nextSequenceSend since all packets were flushed
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), counterpartyUpgrade.nextSequenceSend)
    // set nextSequenceAck to our own nextSequenceSend since all packets were flushed
    provableStore.set(nextSequenceAckPath(portIdentifier, channelIdentifier), selfNextSequenceSend)
  } else if channel.order == "ORDERED" && upgrade.fields.ordering == "UNORDERED" {
    // reset recv and ack sequences to 1 for UNORDERED channel
    provableStore.set(nextSequenceRecvPath(portIdentifier, channelIdentifier), 1)
    provableStore.set(nextSequenceAckPath(portIdentifier, channelIdentifier), 1)
  }

  // switch channel fields to upgrade fields
  // and set channel state to OPEN
  channel.ordering = upgrade.fields.ordering
  channel.version = upgrade.fields.version
  channel.connectionHops = upgrade.fields.connectionHops
  channel.state = OPEN
  provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)

  // IMPLEMENTATION DETAIL: Implementations may choose to prune stale acknowledgements and receipts at this stage
  // Since flushing has completed, any acknowledgement or receipt written before the chain went into flushing has
  // already been processed by the counterparty and can be removed.
  // Implementations may do this pruning work over multiple blocks for gas reasons. In this case, they should be sure
  // to only prune stale acknowledgements/receipts and not new ones that have been written after the channel has reopened.
  // Implementations may use the counterparty NextSequenceSend as a way to determine which acknowledgement/receipts
  // were already processed by counterparty when flushing completed

  // delete auxiliary state
  provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
  privateStore.delete(counterpartyUpgradePath(portIdentifier, channelIdentifier))
}
```

`restoreChannel` will write an `ErrorReceipt`, set the channel back to its original state and delete upgrade information when the executing channel needs to abort the upgrade handshake and return to the original parameters.

```typescript
// restoreChannel will restore the channel state to its pre-upgrade state
// and delete upgrade auxiliary state so that upgrade is aborted.
// it writes an error receipt to state so counterparty can restore as well.
// NOTE: this function signature may be modified by implementers to take a custom error
function restoreChannel(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
) {
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  errorReceipt = ErrorReceipt{
    channel.upgradeSequence,
    "upgrade handshake is aborted", // constant string changeable by implementation
  }
  provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
  channel.state = OPEN
  provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)

  // delete auxiliary state
  provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
  privateStore.delete(counterpartyUpgradePath(portIdentifier, channelIdentifier))
}
```

`pendingInflightPackets` will return the list of in-flight packet sequences sent from this `ChannelEnd`. This can be monitored since the packet commitments are deleted when the packet lifecycle is complete. Thus if the packet commitment exists on the sender chain, the packet lifecycle is incomplete. The pseudocode is not provided in this spec since it will be dependent on the state machine in-question. The ibc-go implementation will use the store iterator to implement this functionality. The function signature is provided below:

```typescript
// pendingInflightPacketSequences returns the packet sequences sent on 
// this end that have not had their lifecycle completed
function pendingInflightPacketSequences(
  portIdentifier: Identifier,
  channelIdentifier: Identifier
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
function getUpgradeTimeout(portIdentifier: string, channelIdentifier: string) Timeout {
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

**IMPORTANT:** Note it is important that the prior state before the channel upgrade process starts is that **both** channel ends are `OPEN`. Authorized upgraders are at risk of having the channel halt during the upgrade process if the prior state before channel upgrades on one of the ends is not `OPEN`.

Refer to the diagram below for a possible channel upgrade flow. Multiple channel states are shown on steps 5 and 7 where the channel end can move to either one of those possible states upon executing the handshake. Note that in this example, the channel end on chain B moves to `OPEN` with the new parameters on `ChanUpgradeConfirm` (step 7).

![Channel Upgrade Flow](channel-upgrade-flow.png)

Once both states are in `FLUSHING` and both sides have stored each others upgrade timeouts, both sides can move to `FLUSHCOMPLETE` by clearing their in-flight packets. Once both sides have complete flushing, a relayer may submit a `ChanUpgradeOpen` datagram to both ends proving that the counterparty has also completed flushing in order to move the channelEnd to `OPEN`.

`ChanUpgradeOpen` is only necessary to call on chain B if the chain was not moved to `OPEN` on `ChanUpgradeConfirm` which may happen if all packets on both ends are already flushed.

At the end of a successful upgrade handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded channel end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded channel parameters.
- All packets sent before the handshake have been completely flushed (acked or timed out) with the old parameters.
- All packets sent after a channel end moves to OPEN will either timeout using new parameters on sending channelEnd or will be received by the counterparty using new parameters.

If a chain does not agree to the proposed counterparty upgraded `ChannelEnd`, it may abort the upgrade handshake by writing an `ErrorReceipt` into the `channelUpgradeErrorPath` and restoring the original channel. The `ErrorReceipt` must contain the current upgrade sequence on the erroring chain's channel end.

`channelUpgradeErrorPath(portID, channelID) => ErrorReceipt(sequence, msg)`

A relayer may then submit a `ChanUpgradeCancel` datagram to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote an `ErrorReceipt` into its `channelUpgradeErrorPath` with a sequence greater than or equal to its own `ChannelEnd`'s upgrade sequence. If successful, it will restore its original channel as well, thus cancelling the upgrade.

If a chain does not reach `FLUSHCOMPLETE` within the counterparty specified timeout, then it MUST NOT move to `FLUSHCOMPLETE` and should instead abort the upgrade. A relayer may submit a proof of this to the counterparty chain in a `ChanUpgradeTimeout` datagram so that counterparty cancels the upgrade and restores its original channel as well.

```typescript
// Channel Ends on both sides **must** be OPEN before this function is called
// It is the responsibility of the authorized upgrader to ensure this is the case
function chanUpgradeInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proposedUpgradeFields: UpgradeFields,
  msgSender: string,
) {
  // chanUpgradeInit may only be called by addresses authorized by executing chain
  abortTransactionUnless(isAuthorizedUpgrader(msgSender))

  // if a previous upgrade attempt exists, then delete it and write error receipt, so
  // counterparty can abort it and move to next upgrade
  existingUpgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
  if existingUpgrade != null {
    provableStore.delete(channelUpgradePath(portIdentifier, channelIdentifier))
    errorReceipt = ErrorReceipt{
      channel.upgradeSequence,
      "abort the previous upgrade attempt so counterparty can accept the new one", // constant string changeable by implementation
    }
    provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
  }

  upgradeSequence = initUpgradeHandshake(portIdentifier, channelIdentifier, proposedUpgradeFields)

  // call modules onChanUpgradeInit callback
  // onChanUpgradeInit may return a new proposed version
  // if an error is returned the upgrade is not written
  // the callback MUST NOT write state, as all state transitions will occur once
  // the channel upgrade is complete.
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
  abortTransactionUnless(err === null)

  // replace channel upgrade version with the version returned by application
  // in case it was modified
  upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
  upgrade.fields.version = version
  provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `chanUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.

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
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state === OPEN)

  // construct counterpartyChannel from existing information and provided
  // counterpartyUpgradeSequence
  counterpartyChannel = ChannelEnd{
    state: OPEN,
    ordering: channel.ordering,
    counterpartyPortIdentifier: portIdentifier,
    counterpartyChannelIdentifier: channelIdentifier,
    connectionHops: counterpartyHops,
    version: channel.version,
    sequence: counterpartyUpgradeSequence,
  }

  // verify proofs of counterparty state
  abortTransactionUnless(
    verifyChannelState(
      connection,
      proofHeight,
      proofChannel,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyChannel
    )
  )
  abortTransactionUnless(
    verifyChannelUpgrade(
      connection,
      proofHeight,
      proofUpgrade,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyUpgrade
    )
  )

  existingUpgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
  if existingUpgrade != null {
    expectedUpgradeSequence = channel.UpgradeSequence
  } else {
    // at the end of the TRY step, the current upgrade sequence will be incremented in the non-crossing
    // hello case due to calling chanUpgradeInit, we should use this expected upgrade sequence for
    // sequence mismatch comparison
    expectedUpgradeSequence = channel.UpgradeSequence + 1
  }

  // NON CROSSING HELLO CASE:
  // if the counterparty sequence is less than or equal to the current sequence,
  // then either the counterparty chain is out-of-sync or the message
  // is out-of-sync and we write an error receipt with our sequence
  // so that the counterparty can abort their attempt and resync with our sequence.
  // When the next upgrade attempt is initiated, both sides will move to a fresh
  // never-before-seen sequence number
  // CROSSING HELLO CASE:
  // if the counterparty sequence is less than the current sequence,
  // then either the counterparty chain is out-of-sync or the message
  // is out-of-sync and we write an error receipt with our sequence minus one
  // so that the counterparty can update their sequence as well.
  // This will cause the outdated counterparty to upgrade the sequence
  // and abort their out-of-sync upgrade without aborting our own since
  // the error receipt sequence is lower than ours and higher than the counterparty.
  if counterpartyUpgradeSequence < expectedUpgradeSequence {
    errorReceipt = ErrorReceipt{
      expectedUpgradeSequence - 1,
      "sequence out of sync", // constant string changeable by implementation
    }
    provableStore.set(channelUpgradeErrorPath(portIdentifier, channelIdentifier), errorReceipt)
    return
  }
  
  // create upgrade fields for this chain from counterparty upgrade and 
  // relayer-provided information version may be mutated by application callback
  upgradeFields = Upgrade{
    ordering: counterpartyUpgrade.fields.ordering,
    connectionHops: proposedConnectionHops,
    version: counterpartyUpgrade.fields.version,
  }

  // current upgrade either doesn't exist (non-crossing hello case),
  // we initialize the upgrade with constructed upgradeFields
  // if it does exist, we are in crossing hellos and must assert
  // that the upgrade fields are the same for crossing-hellos case
  if (existingUpgrade == null) {
    initUpgradeHandshake(portIdentifier, channelIdentifier, upgradeFields)
  } else {
    // we must use the existing upgrade fields
    upgradeFields = existingUpgrade.fields
  }

  abortTransactionUnless(isCompatibleUpgradeFields(upgradeFields, counterpartyUpgradeFields))

  // if the counterparty sequence is greater than the current sequence,
  // we fast forward to the counterparty sequence so that both channel 
  // ends are using the same sequence for the current upgrade.
  // initUpgradeHandshake will increment the sequence so after that call
  // both sides will have the same upgradeSequence
  if (counterpartyUpgradeSequence > channel.upgradeSequence) {
    channel.upgradeSequence = counterpartyUpgradeSequence
  }
  provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)

  // get counterpartyHops for given connection
  connection = provableStore.get(connectionPath(channel.connectionHops[0]))
  counterpartyHops = [connection.counterpartyConnectionIdentifier]

  // call startFlushUpgradeHandshake to move channel to FLUSHING, which will block
  // upgrade from progressing to OPEN until flush completes on both ends
  startFlushUpgradeHandshake(portIdentifier, channelIdentifier)

  // refresh channel to get latest state
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))

  // call modules onChanUpgradeTry callback
  // onChanUpgradeTry may return a new proposed version
  // if an error is returned the upgrade is not written
  // the callback MUST NOT write state, as all state transitions will occur once
  // the channel upgrade is complete.
  module = lookupModule(portIdentifier)
  version, err = module.onChanUpgradeTry(
    portIdentifier,
    channelIdentifier,
    channel.upgradeSequence,
    upgradeFields.ordering,
    upgradeFields.connectionHops,
    upgradeFields.version
  )
  // abort the transaction if the callback returns an error and
  // there was no existing upgrade. This will allow the counterparty upgrade
  // to continue existing while this chain may add support for it in the future
  abortTransactionUnless(err === null)

  // replace channel version with the version returned by application
  // in case it was modified
  upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
  upgrade.fields.version = version
  provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), upgrade)
}
```

NOTE: Implementations that want to explicitly permission upgrades should enforce crossing hellos. i.e. Both parties must have called `ChanUpgradeInit` with mutually compatible parameters in order for `ChanUpgradeTry` to succeed. Implementations that want to be permissive towards counterparty-initiated upgrades may allow moving from `OPEN` to `FLUSHING` without having an upgrade previously stored on the executing chain.

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
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state == OPEN || channel.state == FLUSHING)

  connection = provableStore.get(connectionPath(channel.connectionHops[0]))
  counterpartyHops = [connection.counterpartyConnectionIdentifier]

  // construct counterpartyChannel from existing information
  counterpartyChannel = ChannelEnd{
    state: FLUSHING,
    ordering: channel.ordering,
    counterpartyPortIdentifier: portIdentifier,
    counterpartyChannelIdentifier: channelIdentifier,
    connectionHops: counterpartyHops,
    version: channel.version,
    sequence: channel.upgradeSequence,
  }

  // verify proofs of counterparty state
  abortTransactionUnless(
    verifyChannelState(
      connection,
      proofHeight,
      proofChannel,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyChannel
    )
  )
  abortTransactionUnless(
    verifyChannelUpgrade(
      connection,
      proofHeight,
      proofUpgrade,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyUpgrade
    )
  )

  existingUpgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

  // optimistically accept version that TRY chain proposes and pass this to callback for confirmation.
  // in the crossing hello case, we do not modify version that our TRY call returned and instead 
  // enforce that both TRY calls returned the same version
  if (channel.state == OPEN) {
    existingUpgrade.fields.version == counterpartyUpgrade.fields.version
  }
  // if upgrades are not compatible by ACK step, then we restore the channel
  if (!isCompatibleUpgradeFields(existingUpgrade.fields, counterpartyUpgrade.fields)) {
    restoreChannel(portIdentifier, channelIdentifier)
    return
  }

  if (channel.state == OPEN) {
    // prove counterparty and move our own state to flushing
    // if we are already at flushing, then no state changes occur
    // upgrade is blocked on this channelEnd from progressing until flush completes on its end
    startFlushUpgradeHandshake(portIdentifier, channelIdentifier)
    // startFlushUpgradeHandshake sets the timeout for the upgrade
    // so retrieve upgrade again here and use that timeout value
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))
    existingUpgrade.timeout = upgrade.timeout
  }

  timeout = counterpartyUpgrade.timeout
  
  // counterparty-specified timeout must not have exceeded
  // if it has, then restore the channel and abort upgrade handshake
  if ((timeout.timeoutHeight != 0 && currentHeight() >= timeout.timeoutHeight) ||
      (timeout.timeoutTimestamp != 0 && currentTimestamp() >= timeout.timeoutTimestamp )) {
        restoreChannel(portIdentifier, channelIdentifier)
        return
  }

  // if there are no in-flight packets on our end, we can automatically go to FLUSHCOMPLETE
  if (pendingInflightPackets(portIdentifier, channelIdentifier) == null) {
    channel.state = FLUSHCOMPLETE
  }
  // set counterparty upgrade
  privateStore.set(counterpartyUpgradePath(portIdentifier, channelIdentifier), counterpartyUpgrade)

  provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)

  // call modules onChanUpgradeAck callback
  // module can error on counterparty version
  // ACK should not change state to the new parameters yet
  // as that will happen on the onChanUpgradeOpen callback
  module = lookupModule(portIdentifier)
  err = module.onChanUpgradeAck(
    portIdentifier,
    channelIdentifier,
    counterpartyUpgrade.fields.version
  )
  // restore channel if callback returned error
  if (err != null) {
    restoreChannel(portIdentifier, channelIdentifier)
    return
  }

  // if no error, agree on final version
  provableStore.set(channelUpgradePath(portIdentifier, channelIdentifier), existingUpgrade)
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
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state === FLUSHING)

  // counterparty channel is either FLUSHING or FLUSHCOMPLETE
  abortTransactionUnless(counterpartyChannelState === FLUSHING || counterpartyChannelState === FLUSHCOMPLETE)

  connection = provableStore.get(connectionPath(channel.connectionHops[0]))
  counterpartyHops = [connection.counterpartyConnectionIdentifier]

  counterpartyChannel = ChannelEnd{
    state: counterpartyChannelState,
    ordering: channel.ordering,
    counterpartyPortIdentifier: portIdentifier,
    counterpartyChannelIdentifier: channelIdentifier,
    connectionHops: counterpartyHops,
    version: channel.version,
    sequence: channel.upgradeSequence,
  }

  // verify proofs of counterparty state
  abortTransactionUnless(
    verifyChannelState(
      connection,
      proofHeight,
      proofChannel,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyChannel
    )
  )
  abortTransactionUnless(
    verifyChannelUpgrade(
      connection,
      proofHeight,
      proofUpgrade, 
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyUpgrade
    )
  )

  existingUpgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

	// in the crossing-hello case it is possible that both chains execute the
	// INIT, TRY and CONFIRM steps without any of them executing ACK, therefore
	// we also need to check that the upgrades are compatible on this step
  if (!isCompatibleUpgradeFields(existingUpgrade.fields, counterpartyUpgrade.fields)) {
    restoreChannel(portIdentifier, channelIdentifier)
    return
  }

  timeout = counterpartyUpgrade.timeout
  
  // counterparty-specified timeout must not have exceeded
  // if it has, then restore the channel and abort upgrade handshake
  if ((timeout.timeoutHeight != 0 && currentHeight() >= timeout.timeoutHeight) ||
      (timeout.timeoutTimestamp != 0 && currentTimestamp() >= timeout.timeoutTimestamp)) {
        restoreChannel(portIdentifier, channelIdentifier)
        return
  }

  // if there are no in-flight packets on our end, we can automatically go to FLUSHCOMPLETE
  if (pendingInflightPackets(portIdentifier, channelIdentifier) == null) {
    channel.state = FLUSHCOMPLETE
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)
  }
  // set counterparty upgrade
  privateStore.set(counterpartyUpgradePath(portIdentifier, channelIdentifier), counterpartyUpgrade)

  // if both chains are already in flushcomplete we can move to OPEN
  if (channel.state == FLUSHCOMPLETE && counterpartyChannelState == FLUSHCOMPLETE) {
    openUpgradeHandshake(portIdentifier, channelIdentifier)
    // make application state changes based on new channel parameters
    module.onChanUpgradeOpen(portIdentifier, channelIdentifier)
  }
}
```

`chanUpgradeOpen` may only be called once both sides have moved to `FLUSHCOMPLETE`. If there exists unprocessed packets in the queue when the handshake goes into `FLUSHING` mode, then the packet handlers must move the channel end to `FLUSHCOMPLETE` once the last packet on the channel end has been processed.

```typescript
function chanUpgradeOpen(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelState: ChannelState,
  counterpartyUpgradeSequence: uint64,
  proofChannel: CommitmentProof,
  proofHeight: Height,
) {
  // channel must have completed flushing
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state === FLUSHCOMPLETE)

  // get connection for proof verification
  connection = provableStore.get(connectionPath(channel.connectionHops[0]))

  // counterparty must be in OPEN or FLUSHCOMPLETE state
  if (counterpartyChannelState == OPEN) {
    // get upgrade since counterparty should have upgraded to these parameters
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

    // get the counterparty's connection hops for the proposed upgrade connection
    proposedConnection = provableStore.get(connectionPath(upgrade.fields.connectionHops))
    counterpartyHops = [proposedConnection.counterpartyConnectionIdentifier]

    // The counterparty upgrade sequence must be greater than or equal to
    // the channel upgrade sequence. It should normally be equivalent, but
    // in the unlikely case that a new upgrade is initiated after it reopens,
    // then the upgrade sequence will be greater than our upgrade sequence.
    abortTransactionUnless(counterpartyUpgradeSequence >= channel.upgradeSequence)

    counterpartyChannel = ChannelEnd{
      state: OPEN,
      ordering: upgrade.fields.ordering,
      counterpartyPortIdentifier: portIdentifier,
      counterpartyChannelIdentifier: channelIdentifier,
      connectionHops: counterpartyHops,
      version: upgrade.fields.version,
      sequence: counterpartyUpgradeSequence,
    }
  } else if (counterpartyChannelState == FLUSHCOMPLETE) {
    counterpartyHops = [connection.counterpartyConnectionIdentifier]
    counterpartyChannel = ChannelEnd{
      state: FLUSHCOMPLETE,
      ordering: channel.ordering,
      counterpartyPortIdentifier: portIdentifier,
      counterpartyChannelIdentifier: channelIdentifier,
      connectionHops: counterpartyHops,
      version: channel.version,
      sequence: channel.upgradeSequence,
    }
  } else {
    abortTransactionUnless(false)
  }

  abortTransactionUnless(
    verifyChannelState(
      connection, 
      proofHeight, 
      proofChannel, 
      channel.counterpartyPortIdentifier, 
      channel.counterpartyChannelIdentifier, 
      counterpartyChannel
    )
  )

  // move channel to OPEN and adopt upgrade parameters
  openUpgradeHandshake(portIdentifier, channelIdentifier)

  // call modules onChanUpgradeOpen callback
  module = lookupModule(portIdentifier)
  // open callback must not return error since counterparty successfully upgraded
  // make application state changes based on new channel parameters
  module.onChanUpgradeOpen(
    portIdentifier,
    channelIdentifier
  )
}
```

### Cancel Upgrade Process

During the upgrade handshake a chain may cancel the upgrade by writing an error receipt into the upgrade error path and restoring the original channel to `OPEN`. The counterparty must then restore its channel to `OPEN` as well. A relayer can facilitate this by sending `ChanUpgradeCancel` datagram to the handler:

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
  abortTransactionUnless(upgrade !== null)

  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  // if the msgSender is authorized to make and cancel upgrades AND 
  // the current channel has not already reached FLUSHCOMPLETE,
  // then we can restore immediately without any additional checks
  // otherwise, we can only cancel if the counterparty wrote an
  // error receipt during the upgrade handshake
  if (!(isAuthorizedUpgrader(msgSender) && channel.state != FLUSHCOMPLETE)) {
    abortTransactionUnless(!isEmpty(errorReceipt))

    if channel.state == FLUSHCOMPLETE {
      // if the channel state is in FLUSHCOMPLETE, it can **only** be aborted if there
      // is an error receipt with the exact same sequence. This ensures that the counterparty
      // did not successfully upgrade and then cancel at a new upgrade to abort our own end,
      // leading to both channel ends being OPEN with different parameters
      abortTransactionUnless(errorReceipt.sequence == channel.upgradeSequence)
    } else {
      // If counterparty sequence is less than the current sequence,
      // abort transaction since this error receipt is from a previous upgrade
      abortTransactionUnless(errorReceipt.sequence >= channel.upgradeSequence)
    }
    // fastforward channel sequence to higher sequence so that we can start
    // new handshake on a fresh sequence
    channel.upgradeSequence = errorReceipt.sequence
    provableStore.set(channelPath(portIdentifier, channelIdentifier), channel)

    // get underlying connection for proof verification
    connection = provableStore.get(connectionPath(channel.connectionHops[0]))
    // verify that the provided error receipt is written to the upgradeError path with the counterparty sequence
    abortTransactionUnless(
      verifyChannelUpgradeError(
        connection,
        proofHeight,
        proofUpgradeError,
        channel.counterpartyPortIdentifier,
        channel.counterpartyChannelIdentifier,
        errorReceipt
      )
    )
  }

  // cancel upgrade and write error receipt
  restoreChannel(portIdentifier, channelIdentifier)
}
```

### Timeout Upgrade Process

It is possible for the channel upgrade process to stall indefinitely while trying to flush the existing packets. To protect against this, each chain sets a timeout when it moves into `FLUSHING`. If the counterparty has not completed flushing within the expected time window, then the relayer can submit a timeout message to restore the channel to `OPEN` with the original parameters. It will also write an error receipt so that the counterparty which has not moved to `FLUSHCOMPLETE` can also restore channel to `OPEN` with the original parameters.

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
  abortTransactionUnless(upgrade !== null)
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel.state === FLUSHING || channel.state === FLUSHCOMPLETE)

  upgradeTimeout = upgrade.timeout

  // proof must be from a height after timeout has elapsed. 
  // Either timeoutHeight or timeoutTimestamp must be defined.
  // if timeoutHeight is defined and proof is from before 
  // timeout height then abort transaction
  abortTransactionUnless(
    upgradeTimeout.timeoutHeight.IsZero() || 
    proofHeight >= upgradeTimeout.timeoutHeight
  )
  // if timeoutTimestamp is defined then the consensus time 
  // from proof height must be greater than timeout timestamp
  connection = provableStore.get(connectionPath(channel.connectionHops[0]))
  abortTransactionUnless(
    upgradeTimeout.timeoutTimestamp.IsZero() || 
    getTimestampAtHeight(connection, proofHeight) >= upgradeTimeout.timestamp
  )

  // counterparty channel must be proved to not have completed flushing after timeout has passed
  abortTransactionUnless(counterpartyChannel.state !== FLUSHCOMPLETE)
  // if counterparty channel state is OPEN, we should abort the tx
  // only if the counterparty has successfully completed upgrade
  if (counterpartyChannel.state == OPEN) {
    // get upgrade since counterparty should have upgraded to these parameters
    upgrade = provableStore.get(channelUpgradePath(portIdentifier, channelIdentifier))

    // get counterparty hops of the proposed connection
    proposedConnection = provableStore.get(connectionPath(upgrade.fields.connectionHops))
    counterpartyHops = [proposedConnection.counterpartyConnectionIdentifier]

    // check that the channel did not upgrade successfully
    if ((upgrade.fields.version == counterpartyChannel.version) &&
        (upgrade.fields.order == counterpartyChannel.order) &&
        (counterpartyHops == counterpartyChannel.connectionHops)) {
          // counterparty has already successfully upgraded so we cannot timeout
          abortTransactionUnless(false)
    }
  }
  abortTransactionUnless(counterpartyChannel.upgradeSequence >= channel.upgradeSequence)
  abortTransactionUnless(
    verifyChannelState(
      connection,
      proofHeight,
      proofChannel,
      channel.counterpartyPortIdentifier,
      channel.counterpartyChannelIdentifier,
      counterpartyChannel
    )
  )

  // we must restore the channel since the timeout verification has passed
  // error receipt is written for this sequence, counterparty can call cancelUpgradeHandshake
  restoreChannel(portIdentifier, channelIdentifier)
}
```

Both parties must not complete the upgrade handshake and move to `FLUSHCOMPLETE` if the counterparty upgrade timeout has already passed. This will prevent the channel ends from reaching incompatible states.

### Considerations

Note that a channel upgrade handshake may never complete successfully if the in-flight packets cannot successfully be cleared. This can happen if the timeout value of a packet is too large, or an acknowledgement never arrives, or if there is a bug that makes acknowledging or timing out a packet impossible. In these cases, some out-of-protocol mechanism (e.g. governance) must step in to clear the packets "manually" perhaps by forcefully clearing the packet commitments before restarting the upgrade handshake.

### Migrations

A chain may have to update its internal state to be consistent with the new upgraded channel. In this case, a migration handler should be a part of the chain binary before the upgrade process so that the chain can properly migrate its state once the upgrade is successful. If a migration handler is necessary for a given upgrade but is not available, then the executing chain must reject the upgrade so as not to enter into an invalid state. This state migration will not be verified by the counterparty since it will just assume that if the channel is upgraded to a particular channel version, then the auxiliary state on the counterparty will also be updated to match the specification for the given channel version. The migration must only run once the upgrade has successfully completed and the new channel is `OPEN` (ie. on `ChanUpgradeConfirm` or `ChanUpgradeOpen`).

## Example Implementations

- Implementation of channel upgrade in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

## History

Feb 1, 2024 - Spec as implemented in ibc-go

Jul 24, 2024 - [Add upgrade compatibility check in `chanUpgradeConfirm`](https://github.com/cosmos/ibc/pull/1127)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
