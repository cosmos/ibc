# Upgrading Connections

## Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing connections to upgrade after the initial connection handshake.

## Motivation

As new features get added to IBC, chains may wish to take advantage of new connection features without abandoning the accumulated state and network effect(s) of an already existing connection. The upgrade protocol proposed would allow chains to renegotiate an existing connection to take advantage of new features without having to create a new connection, thus preserving all existing channels that built on top of the connection.

## Desired Properties

- Both chains MUST agree to the renegotiated connection parameters.
- Connection state and logic on both chains SHOULD either be using the old parameters or the new parameters, but MUST NOT be in an in-between state, e.g., it MUST NOT be possible for a chain to write state to an old proof path, while the counterparty expects a new proof path.
- The connection upgrade protocol is atomic, i.e., 
  - either it is unsuccessful and then the connection MUST fall-back to the original connection parameters; 
  - or it is successful and then both connection ends MUST adopt the new connection parameters and process IBC data appropriately.
- The connection upgrade protocol should have the ability to change all connection-related parameters; however the connection upgrade protocol MUST NOT be able to change the underlying `ClientState`.
The connection upgrade protocol MUST NOT modify the connection identifiers.

## Technical Specification

### Data Structures

The `ConnectionState` and `ConnectionEnd` are defined in [ICS-3](./README.md), they are reproduced here for the reader's convenience. `UPGRADE_INIT`, `UPGRADE_TRY` are additional states added to enable the upgrade feature.

#### **ConnectionState (reproduced from [ICS-3](README.md))**

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
  UPGRADE_INIT,
  UPGRADE_TRY,
}
```

- The chain that is proposing the upgrade should set the connection state from `OPEN` to `UPGRADE_INIT`
- The counterparty chain that accepts the upgrade should set the connection state from `OPEN` to `UPGRADE_TRY`

#### **ConnectionEnd (reproduced from [ICS-3](README.md))**

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

The desired property that the connection upgrade protocol MUST NOT modify the underlying clients or connection identifiers, means that only some fields of `ConnectionEnd` are upgradable by the upgrade protocol.

- `state`: The state is specified by the handshake steps of the upgrade protocol.

MAY BE MODIFIED:

- `counterpartyPrefix`: The prefix MAY be modified in the upgrade protocol. The counterparty must accept the new proposed prefix value, or it must return an error during the upgrade handshake.
- `version`: The version MAY be modified by the upgrade protocol. The same version negotiation that happens in the initial connection handshake can be employed for the upgrade handshake.
- `delayPeriodTime`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.
- `delayPeriodBlocks`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.

MUST NOT BE MODIFIED:

- `counterpartyConnectionIdentifier`: The counterparty connection identifier CAN NOT be modified by the upgrade protocol.
- `clientIdentifier`: The client identifier CAN NOT be modified by the upgrade protocol
- `counterpartyClientIdentifier`: The counterparty client identifier CAN NOT be modified by the upgrade protocol

NOTE: If the upgrade adds any fields to the `ConnectionEnd` these are by default modifiable, and can be arbitrarily chosen by an Actor (e.g. chain governance) which has permission to initiate the upgrade.

Modifiable fields may also be removed completely.

#### **UpgradeTimeout**

```typescript
interface UpgradeTimeout {
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

- `timeoutHeight`: Timeout height indicates the height at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original connection and the upgrade handshake is aborted.
- `timeoutTimestamp`: Timeout timestamp indicates the time on the counterparty at which the counterparty must no longer proceed with the upgrade handshake. The chains will then preserve their original connection and the upgrade handshake is aborted.

At least one of the timeoutHeight or timeoutTimestamp MUST be non-zero.

### Store Paths

#### Restore Connection Path

The chain must store the previous connection end so that it may restore it if the upgrade handshake fails. This may be stored in the private store.

```typescript
function connectionRestorePath(id: Identifier): Path {
    return "connections/{id}/restore"
}
```

#### UpgradeError Path

The upgrade error path is a public path that can signal an error of the upgrade to the counterparty. It does not store anything in the successful case, but it will store a sentinel abort value in the case that a chain does not accept the proposed upgrade.

```typescript
function connectionErrorPath(id: Identifier): Path {
    return "connections/{id}/upgradeError"

}
```

The UpgradeError MUST have an associated verification membership and non-membership functions added to the connection interface so that a counterparty may verify that chain has stored an error in the UpgradeError path.

```typescript
// ConnectionEnd VerifyConnectionUpgradeError method
function verifyConnectionUpgradeError(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  upgradeErrorReceipt: []byte, 
) {
    clientState = queryClientState(connection.clientIdentifier)
    // construct CommitmentPath
    path = applyPrefix(connection.counterpartyPrefix, connectionErrorPath(connection.counterpartyConnectionIdentifier))
    // verify upgradeErrorReceipt is stored under the constructed path
    // delay period is unnecessary for non-packet verification so pass in 0 for delay period fields
    return verifyMembership(clientState, height, 0, 0, proof, path, upgradeErrorReceipt)
}
```

```typescript
// ConnectionEnd VerifyConnectionUpgradeErrorAbsence method
function verifyConnectionUpgradeErrorAbsence(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
) {
    clientState = queryClientState(connection.clientIdentifier)
    // construct CommitmentPath
    path = applyPrefix(connection.counterpartyPrefix, connectionErrorPath(connection.counterpartyConnectionIdentifier))
    // verify upgradeError path is empty
    // delay period is unnecessary for non-packet verification so pass in 0 for delay period fields
    return verifyNonMembership(clientState, height, 0, 0, proof, path)
}
```

#### TimeoutPath

The timeout path is a public path set by the upgrade initiator to determine when the TRY step should timeout. It stores the `timeoutHeight` and `timeoutTimestamp` by which point the counterparty must have progressed to the TRY step. This path will be proven on the counterparty chain in case of a successful TRY, to ensure timeout has not passed. Or in the case of a timeout, in which case counterparty proves that the timeout has passed on its chain and restores the connection.

```typescript
function timeoutPath(id: Identifier) Path {
    return "connections/{id}/upgradeTimeout"
}
```

The timeout path MUST have associated verification methods on the connection interface in order for a counterparty to prove that a chain stored a particular `UpgradeTimeout`.

```typescript
// ConnectionEnd VerifyConnectionUpgradeTimeout method
function verifyConnectionUpgradeTimeout(
  connection: ConnectionEnd,
  height: Height,
  proof: CommitmentProof,
  upgradeTimeout: UpgradeTimeout, 
) {
    clientState = queryClientState(connection.clientIdentifier)
    // construct CommitmentPath
    path = applyPrefix(connection.counterpartyPrefix, connectionTimeoutPath(connection.counterpartyConnectionIdentifier))
    // marshal upgradeTimeout into bytes with standardized protobuf codec
    timeoutBytes = protobuf.marshal(upgradeTimeout)
    return verifyMembership(clientState, height, 0, 0, proof, path, timeoutBytes)
}
```

## Utility Functions

`restoreConnection()` is a utility function that allows a chain to abort an upgrade handshake in progress, and return the `connectionEnd` to its original pre-upgrade state while also setting the `errorReceipt`. A relayer can then send a `cancelUpgradeMsg` to the counterparty so that it can restore its `connectionEnd` to its pre-upgrade state as well. Once both connection ends are back to the pre-upgrade state, the connection will resume processing with its original connection parameters

```typescript
function restoreConnection() {
    // cancel upgrade
    // write an error receipt into the error path
    // and restore original connection
    errorReceipt = []byte{1}
    provableStore.set(errorPath(identifier), errorReceipt)
    originalConnection = privateStore.get(restorePath(identifier))
    provableStore.set(connectionPath(identifier), originalConnection)
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
    // caller should return as well
}
```

## Sub-Protocols

The Connection Upgrade process consists of three sub-protocols: `UpgradeConnectionHandshake`, `CancelConnectionUpgrade`, and `TimeoutConnectionUpgrade`. In the case where both chains approve of the proposed upgrade, the upgrade handshake protocol should complete successfully and the `ConnectionEnd` should upgrade successfully.

### Upgrade Handshake

The upgrade handshake defines four datagrams: *ConnUpgradeInit*, *ConnUpgradeTry*, *ConnUpgradeAck*, and *ConnUpgradeConfirm*

A successful protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)          | Posterior state (A, B)      |
| --------- | -------------------- | ---------------- | --------------------------- | --------------------------- |
| Actor     | `ConnUpgradeInit`    | A                | (OPEN, OPEN)                | (UPGRADE_INIT, OPEN)        |
| Actor     | `ConnUpgradeTry`     | B                | (UPGRADE_INIT, OPEN)        | (UPGRADE_INIT, UPGRADE_TRY) |
| Relayer   | `ConnUpgradeAck`     | A                | (UPGRADE_INIT, UPGRADE_TRY) | (OPEN, UPGRADE_TRY)         |
| Relayer   | `ConnUpgradeConfirm` | B                | (OPEN, UPGRADE_TRY)         | (OPEN, OPEN)                |

At the end of an upgrade handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded connection end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded connection parameters.

If a chain does not agree to the proposed counterparty `UpgradedConnection`, it may abort the upgrade handshake by writing an error receipt into the `errorPath` and restoring the original connection. The error receipt MAY be arbitrary bytes and MUST be non-empty.

`errorPath(id) => error_receipt`

A relayer may then submit a `CancelConnectionUpgradeMsg` to the counterparty. Upon receiving this message a chain must verify that the counterparty wrote a non-empty error receipt into its `UpgradeError` and if successful, it will restore its original connection as well thus cancelling the upgrade.

If an upgrade message arrives after the specified timeout, then the message MUST NOT execute successfully. Again a relayer may submit a proof of this in a `CancelConnectionUpgradeTimeoutMsg` so that counterparty cancels the upgrade and restores it original connection as well.

```typescript
function connUpgradeInit(
    identifier: Identifier,
    proposedUpgradeConnection: ConnectionEnd,
    counterpartyTimeoutHeight: Height,
    counterpartyTimeoutTimestamp: uint64,
) {
    // current connection must be OPEN
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(currentConnection.state == OPEN)

    // abort transaction if an unmodifiable field is modified
    // upgraded connection state must be in `UPGRADE_INIT`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgradeConnection.state == UPGRADE_INIT &&
        proposedUpgradeConnection.counterpartyConnectionIdentifier == currentConnection.counterpartyConnectionIdentifier &&
        proposedUpgradeConnection.clientIdentifier == currentConnection.clientIdentifier &&
        proposedUpgradeConnection.counterpartyClientIdentifier == currentConnection.counterpartyClientIdentifier
    )

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(counterpartyTimeoutHeight != 0 || counterpartyTimeoutTimestamp != 0)

    upgradeTimeout = UpgradeTimeout{
        timeoutHeight: counterpartyTimeoutHeight,
        timeoutTimestamp: counterpartyTimeoutTimestamp,
    }

    provableStore.set(timeoutPath(identifier), upgradeTimeout)
    provableStore.set(connectionPath(identifier), proposedUpgradeConnection)
    privateStore.set(restorePath(identifier), currentConnection)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ConnUpgradeInit` function. E.g. chain governance, permissioned actor, DAO, etc.
Access control on counterparty should inform choice of timeout values, i.e. timeout value should be large if counterparty's `UpgradeTry` is gated by chain governance.

```typescript
function connUpgradeTry(
    identifier: Identifier,
    proposedUpgradeConnection: ConnectionEnd,
    counterpartyConnection: ConnectionEnd,
    timeoutHeight: Height,
    timeoutTimestamp: uint64,
    proofConnection: CommitmentProof,
    proofUpgradeTimeout: CommitmentProof,
    proofHeight: Height
) {
    // current connection must be OPEN or UPGRADE_INIT (crossing hellos)
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(currentConnection.state == OPEN || currentConnection.state == UPGRADE_INIT)

    // abort transaction if an unmodifiable field is modified
    // upgraded connection state must be in `UPGRADE_TRY`
    // NOTE: Any added fields are by default modifiable.
    abortTransactionUnless(
        proposedUpgradeConnection.state == UPGRADE_TRY &&
        proposedUpgradeConnection.counterpartyConnectionIdentifier == currentConnection.counterpartyConnectionIdentifier &&
        proposedUpgradeConnection.clientIdentifier == currentConnection.clientIdentifier &&
        proposedUpgradeConnection.counterpartyClientIdentifier == currentConnection.counterpartyClientIdentifier
    )

    // construct upgrade timeout from timeoutHeight and timeoutTimestamp
    // so that we can prove they were set by counterparty
    upgradeTimeout = UpgradeTimeout{
        timeoutHeight: timeoutHeight,
        timeoutTimestamp: timeoutTimestamp,
    }

    // verify proofs of counterparty state
    abortTransactionUnless(verifyConnectionState(currentConnection, proofHeight, proofConnection, currentConnection.counterpartyConnectionIdentifier, proposedUpgradeConnection))
    abortTransactionUnless(verifyConnectionUpgradeTimeout(currentConnection, proofHeight, proofUpgradeTimeout,  upgradeTimeout))

    // verify that counterparty connection unmodifiable fields have not changed and counterparty state
    // is UPGRADE_INIT
    abortTransactionUnless(
        counterpartyConnection.state == UPGRADE_INIT &&
        counterpartyConnection.counterpartyConnectionIdentifier == identifier &&
        counterpartyConnection.clientIdentifier == currentConnection.counterpartyClientIdentifier &&
        counterpartyConnection.counterpartyClientIdentifier == currentConnection.clientIdentifier
    )

    if currentConnection.state == UPGRADE_INIT {
        // if there is a crossing hello, ie an UpgradeInit has been called on both connectionEnds,
        // then we must ensure that the proposedUpgrade by the counterparty is the same as the currentConnection
        // except for the connection state (upgrade connection will be in UPGRADE_TRY and current connection will be in UPGRADE_INIT)
        // if the proposed upgrades on either side are incompatible, then we will restore the connection and cancel the upgrade.
        currentConnection.state = UPGRADE_TRY
        if !currentConnection.IsEqual(proposedUpgradeConnection) {
            restoreConnection()
            return
        }
    } else if currentConnection.state == OPEN {
        // this is first message in upgrade handshake on this chain so we must store original connection in restore path
        // in case we need to restore connection later.
        privateStore.set(restorePath(identifier), currentConnection)
    } else {
        // abort transaction if current connection is not in INIT or OPEN
        abortTransactionUnless(false)
    }
    
    // either timeout height or timestamp must be non-zero
    // if the upgrade feature is implemented on the TRY chain, then a relayer may submit a TRY transaction after the timeout.
    // this will restore the connection on the executing chain and allow counterparty to use the CancelUpgradeMsg to restore their connection.
    if timeoutHeight == 0 && timeoutTimestamp == 0 {
        restoreConnection()
        return
    }
    
    // counterparty-specified timeout must not have exceeded
    if (currentHeight() > timeoutHeight && timeoutHeight != 0) ||
        (currentTimestamp() > timeoutTimestamp && timeoutTimestamp != 0) {
        restoreConnection()
        return
    }

    // verify chosen versions are compatible
    versionsIntersection = intersection(counterpartyConnection.version, proposedUpgradeConnection.version)
    version = pickVersion(versionsIntersection) // aborts transaction if there is no intersection

    // both connection ends must be mutually compatible.
    // this function has been left unspecified since it will depend on the specific structure of the new connection.
    // It is the responsibility of implementations to make sure that verification that the proposed new connections
    // on either side are correctly constructed according to the new version selected.
    if !IsCompatible(counterpartyConnection, proposedUpgradeConnection) {
        restoreConnection()
        return
    }

    provableStore.set(connectionPath(identifier), proposedUpgradeConnection)
}
```

NOTE: It is up to individual implementations how they will provide access-control to the `ConnUpgradeTry` function. E.g. chain governance, permissioned actor, DAO, etc. A chain may decide to have permissioned **or** permissionless `UpgradeTry`. In the permissioned case, both chains must explicitly consent to the upgrade, in the permissionless case; one chain initiates the upgrade and the other chain agrees to the upgrade by default. In the permissionless case, a relayer may submit the `ConnUpgradeTry` datagram.

```typescript
function connUpgradeAck(
    identifier: Identifier,
    counterpartyConnection: ConnectionEnd,
    proofConnection: CommitmentProof,
    proofHeight: Height
) {
    // current connection is in UPGRADE_INIT or UPGRADE_TRY (crossing hellos)
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(currentConnection.state == UPGRADE_INIT || currentConnection.state == UPGRADE_TRY)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyConnectionState(currentConnection, proofHeight, proofConnection, currentConnection.counterpartyConnectionIdentifier, counterpartyConnection))

    // counterparty must be in TRY state
    if counterpartyConnection.State != UPGRADE_TRY {
        restoreConnection()
        return
    }

    // verify connections are mutually compatible
    // this will also check counterparty chosen version is valid
    // this function has been left unspecified since it will depend on the specific structure of the new connection.
    // It is the responsibility of implementations to make sure that verification that the proposed new connections
    // on either side are correctly constructed according to the new version selected.
    if !IsCompatible(counterpartyConnection, connection) {
        restoreConnection()
        return
    }

    // upgrade is complete
    // set connection to OPEN and remove unnecessary state
    currentConnection.state = OPEN
    provableStore.set(connectionPath(identifier), currentConnection)
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

```typescript
function connUpgradeConfirm(
    identifier: Identifier,
    counterpartyConnection: ConnectionEnd,
    proofConnection: CommitmentProof,
    proofUpgradeError: CommitmentProof,
    proofHeight: Height,
) {
    // current connection is in UPGRADE_TRY
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(currentConnection.state == UPGRADE_TRY)

    // counterparty must be in OPEN state
    abortTransactionUnless(counterpartyConnection.State == OPEN)

    // verify proofs of counterparty state
    abortTransactionUnless(verifyConnectionState(currentConnection, proofHeight, proofConnection, currentConnection.counterpartyConnectionIdentifier, counterpartyConnection))

    // verify that counterparty did not restore the connection and store an upgrade error
    // in the connection upgradeError path
    abortTransactionUnless(verifyConnectionUpgradeErrorAbsence(currentConnection, proofHeight, proofUpgradeError))
    
    // upgrade is complete
    // set connection to OPEN and remove unnecessary state
    currentConnection.state = OPEN
    provableStore.set(connectionPath(identifier), currentConnection)
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

NOTE: Since the counterparty has already successfully upgraded and moved to `OPEN` in `ACK` step, we cannot restore the connection here. We simply verify that the counterparty has successfully upgraded and then upgrade ourselves.

### Cancel Upgrade Process

During the upgrade handshake a chain may cancel the upgrade by writing an error receipt into the error path and restoring the original connection to `OPEN`. The counterparty must then restore its connection to `OPEN` as well.

A connectionEnd may only cancel the upgrade during the upgrade negotiation process (TRY, ACK). An upgrade cannot be cancelled on one end once the other chain has already completed its upgrade and moved to `OPEN` since that will lead the connection to being in an invalid state.

A relayer can facilitate this by calling `CancelConnectionUpgrade`:

```typescript
function cancelConnectionUpgrade(
    identifier: Identifer,
    errorReceipt: []byte,
    proofUpgradeError: CommitmentProof,
    proofHeight: Height,
) {
    // current connection is in UPGRADE_INIT or UPGRADE_TRY
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(currentConnection.state == UPGRADE_INIT || currentConnection.state == UPGRADE_TRY)

    abortTransactionUnless(!isEmpty(errorReceipt))

    abortTransactionUnless(verifyConnectionUpgradeError(currentConnection, proofHeight, proofUpgradeError, errorReceipt))

    // cancel upgrade
    // and restore original connection
    // delete unnecessary state
    originalConnection = privateStore.get(restorePath(identifier))
    provableStore.set(connectionPath(identifier), originalConnection)

    // delete auxiliary upgrade state
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

### Timeout Upgrade Process

It is possible for the connection upgrade process to stall indefinitely on UPGRADE_TRY if the UPGRADE_TRY transaction simply cannot pass on the counterparty; for example, the upgrade feature may not be enabled on the counterparty chain.

In this case, we do not want the initializing chain to be stuck indefinitely in the `UPGRADE_INIT` step. Thus, the `UpgradeInit` message will contain a `TimeoutHeight` and `TimeoutTimestamp`. The counterparty chain is expected to reject `UpgradeTry` message if the specified timeout has already elapsed.

A relayer must then submit an `UpgradeTimeout` message to the initializing chain which proves that the counterparty is still in its original state. If the proof succeeds, then the initializing chain shall also restore its original connection and cancel the upgrade.

```typescript
function timeoutConnectionUpgrade(
    identifier: Identifier,
    counterpartyConnection: ConnectionEnd,
    proofConnection: CommitmentProof,
    proofHeight: Height,
) {
    // current connection must be in UPGRADE_INIT
    currentConnection = provableStore.get(connectionPath(identifier))
    abortTransactionUnles(currentConnection.state == UPGRADE_INIT)

    upgradeTimeout = provableStore.get(timeoutPath(identifier))

    // proof must be from a height after timeout has elapsed. Either timeoutHeight or timeoutTimestamp must be defined.
    // if timeoutHeight is defined and proof is from before timeout height
    // then abort transaction
    abortTransactionUnless(upgradeTimeout.timeoutHeight.IsZero() || proofHeight >= upgradeTimeout.timeoutHeight)
    // if timeoutTimestamp is defined then the consensus time from proof height must be greater than timeout timestamp
    abortTransactionUnless(upgradeTimeout.timeoutTimestamp.IsZero() || getTimestampAtHeight(currentConnection, proofHeight) >= upgradeTimeout.timestamp)

    // counterparty connection must be proved to still be in OPEN state or UPGRADE_INIT state (crossing hellos)
    abortTransactionUnless(counterpartyConnection.State === OPEN || counterpartyConnection.State == UPGRADE_INIT)
    abortTransactionUnless(verifyConnectionState(currentConnection, proofHeight, proofConnection, currentConnection.counterpartyConnectionIdentifier, counterpartyConnection))

    // we must restore the connection since the timeout verification has passed
    originalConnection = privateStore.get(restorePath(identifier))
    provableStore.set(connectionPath(identifier), originalConnection)

    // delete auxiliary upgrade state
    provableStore.delete(timeoutPath(identifier))
    privateStore.delete(restorePath(identifier))
}
```

Note that the timeout logic only applies to the INIT step. This is to protect an upgrading chain from being stuck in a non-OPEN state if the counterparty cannot execute the TRY successfully. Once the TRY step succeeds, then both sides are guaranteed to have the upgrade feature enabled. Liveness is no longer an issue, because we can wait until liveness is restored to execute the ACK step which will move the connection definitely into an OPEN state (either a successful upgrade or a rollback).

The TRY chain will receive the timeout parameters chosen by the counterparty on INIT, so that it can reject any TRY message that is received after the specified timeout. This prevents the handshake from entering into an invalid state, in which the INIT chain processes a timeout successfully and restores its connection to `OPEN` while the TRY chain at a later point successfully writes a `TRY` state.

### Migrations

A chain may have to update its internal state to be consistent with the new upgraded connection. In this case, a migration handler should be a part of the chain binary before the upgrade process so that the chain can properly migrate its state once the upgrade is successful. If a migration handler is necessary for a given upgrade but is not available, then th executing chain must reject the upgrade so as not to enter into an invalid state. This state migration will not be verified by the counterparty since it will just assume that if the connection is upgraded to a particular connection version, then the auxiliary state on the counterparty will also be updated to match the specification for the given connection version. The migration must only run once the upgrade has successfully completed and the new connection is `OPEN` (ie. on `ACK` and `CONFIRM`).
