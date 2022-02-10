# Upgrading Connections

### Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing connections to upgrade after initial connection handshake.

### Motivation

As new features get added to IBC, chains may wish the take advantage of new connection features without abandoning the accumulated state and network effect(s) of an already existing connection. The upgrade protocol proposed would allow chains to renegotiate an existing connection to take advantage of new features without having to create a new connection, thus preserving all existing channels that built on top of the connection.

### Desired Properties

- Both chains must agree to the renegotiated connection parameters
- Connection state and logic on both chains should either be using the old parameters or the new parameters, but should not be in an in-between state. i.e. It should not be possible for a chain to write state to an old proof path, while the counterparty expects a new proof path.
- If the upgrade handshake is unsuccessful, the connection must fall-back to the original connection parameters
- If the upgrade handshake is successful, then both connection ends have adopted the new connection parameters and process IBC data appropriately.
- The connection upgrade protocol should have the ability to change all connection-related parameters; however the connection upgrade protocol MUST NOT be able to change the underlying `ClientState`.
- The connection upgrade protocol may not modify the connection identifiers.

## Technical Specification

### Data Structures

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
  UPGRADE_INIT,
  UPGRADE_TRY,
  UPGRADE_ERR,
}
```

- The chain that is proposing the upgrade should set the connection state from `OPEN` to `UPGRADE_INIT`
- The counterparty chain that accepts the upgrade should set the connection state from `OPEN` to `UPGRADE_TRY`

```typescript
interface ConnectionEnd {
  state: ConnectionState
  counterpartyConnectionIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string | []string
  delayPeriodTime: uint64
  delayPeriodBlocks: uint64
}
```

The desired property that the connection upgrade protocol may not modify the underlying clients or connection identifiers, means that only some fields are upgradable by the upgrade protocol.

- `state`: The state is specified by the handshake steps of the upgrade protocol.

CAN BE MODIFIED:
- `counterpartyPrefix`: The prefix MAY be modified in the upgrade protocol. The counterparty must accept the new proposed prefix value, or it must return an error during the upgrade handshake.
- `version`: The version MAY be modified by the upgrade protocol. The same version negotiation that happens in the initial connection handshake can be employed for the upgrade handshake.
- `delayPeriodTime`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.
- `delayPeriodBlocks`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.

CANNOT BE MODIFIED:
- `counterpartyConnectionIdentifier`: The counterparty connection identifier CAN NOT be modified by the upgrade protocol.
- `clientIdentifier`: The client identifier CAN NOT be modified by the upgrade protocol
- `counterpartyClientIdentifier`: The counterparty client identifier CAN NOT be modified by the upgrade protocol

NOTE: If the upgrade adds any fields to the `ConnectionEnd` these are by default modifiable, and can be arbitrarily chosen by an Actor (e.g. chain governance) which has permission to initiate the upgrade.

```typescript
interface UpgradeStatus {
    state: ConnectionState
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

- UpgradeStatus contains the state of the upgrade. If upgrade is successful it will contain the `ConnectionState` of the upgrading connection on the executing chain. If the executing chain wants to abort the upgrade, it will restore its previous connection under its connection path with state `OPEN` and write an `UpgradeStatus` with state `UPGRADE_ERR`.
- `timeoutHeight`: Timeout height indicates the height at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original connection and the upgrade handshake is aborted.
- `timeoutTimestamp`: Timeout timestamp indicates the time on the counterparty at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original connection and the upgrade handshake is aborted.

```typescript
interface UpgradeConnectionState {
    connection: ConnectionEnd
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

- `proposedConnectionState`: Proposed `ConnectionState` to replace the current `ConnectionState`, it MUST ONLY modify the fields that are permissible to 
- `timeoutHeight`: Timeout height indicates the height at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original connection and the upgrade handshake is aborted.
- `timeoutTimestamp`: Timeout timestamp indicates the time on the counterparty at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original connection and the upgrade handshake is aborted.

NOTE: One of the timeout height or timeout timestamp must be non-zero.

### Store Paths

##### Restore Connection Path

The chain must store the previous connection end so that it may restore it if the upgrade handshake fails. This may be stored in the private store.

```typescript
function restorePath(id: Identifier): Path {
    return "connections/{id}/restore"
}
```

##### UpgradeStatus Path

The upgrade status path is a public path that can signal the status of the upgrade to the counterparty. It does not store anything in the successful case, but it will store a sentinel abort value in the case that a chain does not accept the proposed upgrade.

```typescript
function statusPath(id: Identifier): Path {
    return "connections/{id}/upgradeStatus"

}
```

### Upgrade Handshake

The upgrade handshake defines four datagrams: *ConnUpgradeInit*, *ConnUpgradeTry*, *ConnUpgradeAck*, and *ConnUpgradeConfirm*

A successful protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)          | Posterior state (A, B)      |
| --------- | -------------------- | ---------------- | --------------------------- | --------------------------- |
| Actor     | `ConnUpgradeInit`    | A                | (OPEN, OPEN)                | (UPGRADE_INIT, OPEN)        |
| Actor     | `ConnUpgradeTry`     | B                | (UPGRADE_INIT, OPEN)        | (UPGRADE_INIT, UPGRADE_TRY) |
| Relayer   | `ConnUpgradeAck`     | A                | (UPGRADE_INIT, UPGRADE_TRY) | (OPEN, UPGRADE_TRY)         |
| Relayer   | `ConnUpgradeConfirm` | B                | (OPEN, UPGRADE_TRY)         | (OPEN, OPEN)                |

At the end of an opening handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded connection end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded connection parameters.

If a chain does not agree to the proposed counterparty `UpgradeConnectionState`, it may abort the upgrade handshake by writing `UPGRADE_ERR` as the state in the `UpgradeStatus` and storing
it under the status path and restore the original connection.

`statusPath(id) => UpgradeStatus{UPGRADE_ERR}`

A relayer may then submit a `CancelConnectionUpgradeMsg` to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote `UPGRADE_ERR` into its `UpgradeStatus` and if successful, it will restore its original connection as well thus cancelling the upgrade.

If an upgrade message arrives after the specified timeout, then the message MUST NOT execute successfully. Again a relayer may submit a proof of this in a `CancelConnectionUpgradeTimeoutMsg` so that counterparty cancels the upgrade and restores it original connection as well.

```typescript
function connUpgradeInit(
    identifier: Identifier,
    proposedUpgrade: UpgradeConnectionState
) {
    // current connection must be OPEN
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state == OPEN)

    // abort transaction if an unmodifiable field is modified
    // upgraded connection state must be in `UPGRADE_INIT`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgrade.connection.state == UPGRADE_INIT &&
        proposedUpgrade.connection.counterpartyConnectionIdentifier == currentConnection.counterpartyConnectionIdentifier &&
        proposedUpgrade.connection.clientIdentifier == currentConnection.clientIdentifier &&
        proposedUpgrade.connection.counterpartyClientIdentifier == currentConnection.counterpartyClientIdentifier &&
    )

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(proposedUpgrade.TimeoutHeight != 0 || proposedUpgrade.TimeoutTimestamp != 0)

    upgradeStatus = UpgradeStatus{
        state: UPGRADE_INIT,
        timeoutHeight: proposedUpgrade.timeoutHeight,
        timeoutTimestamp: proposedUpgrade.timeoutTimestamp,
    }

    provableStore.set(statusPath(identifier), upgradeStatus)
    provableStore.set(connectionPath(identifier), proposedUpgrade.connection)
    privateStore.set(restorePath(identifier), currentConnection)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ConnUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.
Access control on counterparty should inform choice of timeout values, i.e. timeout value should be large if counterparty's `UpgradeTry` is gated by chain governance.

```typescript
function connUpgradeTry(
    identifier: Identifier,
    proposedUpgrade: UpgradeConnectionState,
    counterpartyConnection: ConnectionEnd,
    counterpartyUpgradeStatus: UpgradeStatus,
    proofConnection: CommitmentProof,
    proofUpgradeStatus: CommitmentProof,
    proofHeight: Height
) {
    // current connection must be OPEN or UPGRADE_INIT (crossing hellos)
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state == (OPEN || UPGRADE_INIT))

    // abort transaction if an unmodifiable field is modified
    // upgraded connection state must be in `UPGRADE_TRY`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgrade.connection.state == UPGRADE_TRY &&
        proposedUpgrade.connection.counterpartyConnectionIdentifier == currentConnection.counterpartyConnectionIdentifier &&
        proposedUpgrade.connection.clientIdentifier == currentConnection.clientIdentifier &&
        proposedUpgrade.connection.counterpartyClientIdentifier == currentConnection.counterpartyClientIdentifier &&
    )

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(proposedUpgrade.TimeoutHeight != 0 || proposedUpgrade.TimeoutTimestamp != 0)

    // verify proofs of counterparty state
    abortTransactionUnless(currentConnection.client.verifyConnectionState(proofHeight, proofConnection, counterpartyConnection))
    abortTransactionUnless(currentConnection.client.verifyUpgradeStatus(proofHeight, proofUpgradeStatus, counterpartyUpgradeStatus))

    // verify that counterparty connection unmodifiable fields have not changed and counterparty state
    // is UPGRADE_INIT
    restoreConnectionUnless(
        counterpartyConnection.state == UPGRADE_INIT &&
        counterpartyConnection.counterpartyConnectionIdentifier == identifier &&
        counterpartyConnection.clientIdentifier == currentConnection.counterpartyClientIdentifier &&
        counterpartyConnection.counterpartyClientIdentifier == currentConnection.clientIdentifier
    )
    restoreConnectionUnless(counterpartyUpgradeStatus.state == UPGRADE_INIT)

    // counterparty-specified timeout must not have exceeded
    restoreConnectionUnless(
        counterparty.UpgradeStatus.TimeoutHeight < currentHeight() &&
        counterparty.UpgradeStatus.TimeoutTimestamp < currentTimestamp()
    )

    // verify chosen versions are compatible
    versionsIntersection = intersection(counterpartyConnection.version, proposedUpgrade.Connection.version)
    version = pickVersion(versionsIntersection) // throws if there is no intersection

    // both connection ends must be mutually compatible.
    restoreConnectionUnless(IsCompatible(counterpartyConnection, proposedUpgrade.Connection))

    upgradeStatus = UpgradeStatus{
        state: UPGRADE_TRY,
        timeoutHeight: proposedUpgrade.timeoutHeight,
        timeoutTimestamp: proposedUpgrade.timeoutTimestamp,
    }

    provableStore.set(statusPath(identifier), upgradeStatus)
    provableStore.set(connectionPath(identifier), proposedUpgrade.connection)
    privateStore.set(restorePath(identifier), currentConnection)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ConnUpgradeTry` function. E.g. chain governance, permissioned actor, DAO, etc.


```typescript
function onChanUpgradeAck(
    identifier: Identifier,
    counterpartyConnection: ConnectionEnd,
    counterpartyStatus: UpgradeStatus,
    proofConnection: CommitmentProof,
    proofUpgradeStatus: CommitmentProof,
    proofHeight: Height
) {
    // current connection is in UPGRADE_INIT
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state == UPGRADE_INIT)


    // verify proofs of counterparty state
    abortTransactionUnless(currentConnection.client.verifyConnectionState(proofHeight, proofConnection, counterpartyConnection))
    abortTransactionUnless(currentConnection.client.verifyUpgradeStatus(proofHeight, proofUpgradeStatus, counterpartyUpgradeStatus))

    // counterparty must be in TRY state
    restoreConnectionUnless(counterpartyStatus.state == UPGRADE_TRY)
    restoreConnectionUnless(counterpartyConnection.State == UPGRADE_TRY)

    // verify connections are mutually compatible
    // this will also check counterparty chosen version is valid
    restoreConnectionUnless(IsCompatible(counterpartyConnection, connection))

    // upgrade is complete
    // set connection to OPEN and remove unnecessary state
    currentConnection.state = OPEN
    provableStore.set(connectionPath(identifier), currentConnection)
    provableStore.delete(statusPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

```typescript
function onChanUpgradeConfirm(
    identifier: Identifier,
    counterpartyConnection: ConnectionEnd,
    proofConnection: CommitmentProof,
    proofHeight: Height,
) {
    // current connection is in UPGRADE_TRY
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state == UPGRADE_TRY)

    // counterparty must be in OPEN state
    abortTransactionUnless(counterpartyConnection.State == OPEN)

    // verify proofs of counterparty state
    abortTransactionUnless(currentConnection.client.verifyConnectionState(proofHeight, proofConnection, counterpartyConnection))
    
    // upgrade is complete
    // set connection to OPEN and remove unnecessary state
    currentConnection.state = OPEN
    provableStore.set(connectionPath(identifier), currentConnection)
    provableStore.delete(statusPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

Note: In the `TRY` and `ACK` steps, the chain has the ability to abort the upgrade process if the upgrade parameters are not suitable or the timeout has exceeded. In this case, the chain is expected to write `UPGRADE_ERR` as the state in the `UPGRADE_STATUS`, and restore the original connection with state `OPEN`. The counterparty can then verify that the upgrade status is `UPGRADE_ERR` and restore its original connection to `OPEN` as well, thus cancelling the upgrade.

```typescript
function restoreConnectionUnless(condition: bool) {
    if !condition {
        // cancel upgrade
        // set UpgradeStatus to `UPGRADE_ERR`
        // and restore original conneciton
        errStatus = UpgradeStatus{UPGRADE_ERR}
        provableStore.set(statusPath(identifier), errStatus)
        originalConnection = privateStore.get(restorePath(identifier))
        provableStore.set(connectionPath(identifier), originalConnection)
        privateStore.delete(restorePath(identifier))
        // caller should return as well
    } else {
        // caller should continue execution
    }
}
```

### Cancel Upgrade Process

As discussed, during the upgrade handshake a chain may cancel the upgrade by writing `UPGRADE_ERR` into the `UpgradeStatus` and restoring the original connection to `OPEN`. The counterparty must then restore its connection to `OPEN` as well. A relayer can facilitate this by calling `CancelConnectionUpgrade`

```typescript
function cancelConnectionUpgrade(
    identifier: Identifer,
    counterpartyUpgradeStatus: UpgradeStatus,
    proofUpgradeStatus: CommitmentProof,
    proofHeight: Height,
) {
    // current connection is in UPGRADE_INIT or UPGRADE_TRY
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state == (UPGRADE_INIT || UPGRADE_TRY))

    abortTransactionUnless(counterpartyUpgradeStatus.state == UPGRADE_ERR)

    abortTransactionUnless(currentConnection.client.verifyUpgradeStatus(proofHeight, proofUpgradeStatus, counterpartyUpgradeStatus))

    // cancel upgrade
    // and restore original conneciton
    // delete unnecessary state
    originalConnection = privateStore.get(restorePath(identifier))
    provableStore.set(connectionPath(identifier), originalConnection)
    provableStore.delete(statusPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

### Timeout Upgrade Process

It is possible for the connection upgrade process to timeout on TRY. This may be because of a liveness issue, or because the UPGRADE_TRY transaction simply cannot pass on the counterparty; for example, the upgrade feature may not be enabled on the counterparty chain.

In this case, we do not want the initializing chain to be stuck indefinitely in the `UPGRADE_INIT` step. Thus, the `UpgradeInit` message will contain a `TimeoutHeight` and `TimeoutTimestamp`. The counterparty chain is expected to reject `UpgradeTry` message if the specified timeout has already elapsed.

A relayer must then submit an `UpgradeTimeout` message to the initializing chain which proves that the counterparty is still in its original state. If the proof succeeds, then the initializing chain shall also restore its original connection and cancel the upgrade.

```typescript
function timeoutConnectionUpgrade(
    identifier: Identifier,
    counterpartyConnection: ConnectionEnd,
    proofConnection: CommitmentProof,
    proofHeight: Height,
) {
    upgradeStatus = provableStore.get(statusPath(identifier))

    // proof must be from a height after timeout has elapsed. Either timeoutHeight or timeoutTimestamp must be defined.
    // if timeoutHeight is defined and proof is from before timeout height
    // then abort transaction
    abortTransactionUnless(upgradeStatus.TimeoutHeight.IsZero() || proofHeight >= upgradeStatus.TimeoutHeight)
    // if timeoutTimestamp is defined then the consensus time from proof height must be greater than timeout timestamp
    connection = getConnection(identifer)
    consensusState = getConsensusState(connection.clientIdentifer, proofHeight)
    abortTransactionUnless(upgradeStatus.TimeoutTimestamp.IsZero() || consensusState.Timestamp >= upgradeStatus.Timestamp)

    // counterparty connection must be proved to still be in OPEN state
    abortTransactionUnless(counterpartyConnection.State === OPEN)
    abortTransactionUnless(connection.client.verifyConnectionState(proofHeight, proofConnection, counterpartyConnection))

    // we must restore the connection since the timeout verification has passed
    restoreConnectionUnless(false)
}
```

Note that the timeout logic only applies to the INIT step. This is to protect an upgrading chain from being stuck in a non-OPEN state if the counterparty cannot execute the TRY successfully. Once the TRY step succeeds, then both sides are guaranteed to have the upgrade feature enabled. Liveness is no longer an issue, because we can wait until liveness is restored to execute the ACK step which will move the connection definitely into an OPEN state (either a successful upgrade or a rollback).