# Upgrading Connections

### Synopsis

This standard document specifies the interfaces and state machine logic that IBC implementations must implement in order to enable existing connections to upgrade after initial connection handshake.

### Motivation

As new features get added to IBC, chains may wish the take advantage of new connection features without abandoning the accumulated state and network effect(s) of an already existing connection. The upgrade protocol proposed would allow chains to renegotiate an existing connection to take advantage of new features without having to create a new connection.

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

```typescript
interface UpgradeStatus {
    state: ConnectionState
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

The desired property that the connection upgrade protocol may not modify the underlying clients or connection identifiers, means that only some fields are upgradable by the upgrade protocol.

- `state`: The state is specified by the handshake steps of the upgrade protocol.
- `counterpartyConnectionIdentifier`: The counterparty connection identifier CAN NOT be modified by the upgrade protocol.
- `counterpartyPrefix`: The prefix MAY be modified in the upgrade protocol. The counterparty must accept the new proposed prefix value, or it must return an error during the upgrade handshake.
- `clientIdentifier`: The client identifier CAN NOT be modified by the upgrade protocol
- `counterpartyClientIdentifier`: The counterparty client identifier CAN NOT be modified by the upgrade protocol
- `version`: The version MAY be modified by the upgrade protocol. The same version negotiation that happens in the initial connection handshake can be employed for the upgrade handshake.
- `delayPeriodTime`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.
- `delayPeriodBlocks`: The delay period MAY be modified by the upgrade protocol. The counterparty MUST accept the new proposed value or return an error during the upgrade handshake.

```typescript
interface UpgradeConnectionState {
    connection: ConnectionEnd
    timeoutHeight: Height
    timeoutTimestamp: uint64
}
```

- `proposedConnectionState`: Proposed `ConnectionState` to replace the current `ConnectionState`, it MUST ONLY modify the fields that are permissable to 
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
function upgradePath(id: Identifier): Path {
    return "connections/{id}/upgradeStatus"

}
```

### Upgrade Handshake

The upgrade handshake defines four datagrams: *ConnUpgradeInit*, *ConnUpgradeTry*, *ConnUpgradeAck*, and *ConnUpgradeConfirm*

A successful protocol execution flows as follows (note that all calls are made through modules per ICS 25):

| Initiator | Datagram             | Chain acted upon | Prior state (A, B)          | Posterior state (A, B)      |
| --------- | -------------------- | ---------------- | --------------------------- | --------------------------- |
| Actor     | `ConnUpgradeInit`    | A                | (OPEN, OPEN)                | (UPGRADE_INIT, OPEN)        |
| Relayer   | `ConnUpgradeTry`     | B                | (UPGRADE_INIT, OPEN)        | (UPGRADE_INIT, UPGRADE_TRY) |
| Relayer   | `ConnUpgradeAck`     | A                | (UPGRADE_INIT, UPGRADE_TRY) | (OPEN, UPGRADE_TRY)         |
| Relayer   | `ConnUpgradeConfirm` | B                | (OPEN, UPGRADE_TRY)         | (OPEN, OPEN)                |

At the end of an opening handshake between two chains implementing the sub-protocol, the following properties hold:

- Each chain is running their new upgraded connection end and is processing upgraded logic and state according to the upgraded parameters.
- Each chain has knowledge of and has agreed to the counterparty's upgraded connection parameters.

If a chain does not agree to the proposed counterparty `UpgradeConnectionState`, it may abort the upgrade handshake by writing a sentinel abort value into the upgrade-failed connection path.

`connection/{identifier}/proposedUpgradeFailed` => `[]byte(0x1)`.

The counterparty receiving proof of this must cancel the upgrade and resume the 

If an upgrade message arrives after the specified timeout, then the message MUST NOT execute successfully.

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
    abortTransactionUnless(
        proposedUpgrade.connection.state == UPGRADE_INIT &&
        proposedUpgrade.connection.counterpartyConnectionIdentifier == currentConnection.counterpartyConnectionIdentifier &&
        proposedUpgrade.connection.clientIdentifier == currentConnection.clientIdentifier &&
        proposedUpgrade.connection.counterpartyClientIdentifier == currentConnection.counterpartyClientIdentifier &&
    )

    // either timeout height or timestamp must be non-zero
    abortTransactionUnless(proposedUpgrade.TimeoutHeight != 0 || proposedUpgrade.TimeoutTimestamp != 0)

    provableStore.set(connectionPath(identifier), proposedUpgrade)
    privateStore.set(restorePath(identifier), currentConnection)
}
```

```typescript
function connUpgradeTry(
    identifier: Identifier,
    proposedUpgrade
)
