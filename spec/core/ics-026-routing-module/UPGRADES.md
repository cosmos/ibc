# Application Upgrade Callbacks

## Synopsis

This standard document specifies the interfaces and state machine logic that IBC applications must implement in order to enable existing channels to upgrade their applications after the initial channel handshake.

### Motivation

As new features get added to IBC applications, chains may wish the take advantage of new application features without abandoning the accumulated state and network effect(s) of an already existing channel. The upgrade protocol proposed would allow applications to renegotiate an existing channel to take advantage of new features without having to create a new channel, thus preserving all existing application state while upgradng to new application logic.


### Desired Properties

- Both applications MUST agree to the renegotiated application parameters.
- Application state and logic on both chains SHOULD either be using the old parameters or the new parameters, but MUST NOT be in an in-between state, e.g., it MUST NOT be possible for an application to run v2 logic, while its counterparty is still running v1 logic.
- The application upgrade protocol is atomic, i.e., 
  - either it is unsuccessful and then the application MUST fall-back to the original application parameters; 
  - or it is successful and then both applications MUST adopt the new application parameters and the applications must process packet data appropriately.
- The application must be able to maintain several different supported versions such that one channel may be on version `v1` and another channel may be on version `v2` and the application can handle the channel state and logic accordingly depending on the application version for the respective channel.

The application upgrade protocol MUST NOT modify the channel identifiers.

## Technical Specification

In order to support channel upgrades, the application must implement the following interface:

```typescript
interface ModuleUpgradeCallbacks {
  onChanUpgradeInit: onChanUpgradeInit,
  onChanUpgradeTry: onChanUpgradeTry,
  onChanUpgradeAck: onChanUpgradeAck,
  onChanUpgradeConfirm: onChanUpgradeConfirm,
  onChanUpgradeRestore: onChanUpgradeRestore
}
```

#### **OnChanUpgradeInit**

`onChanUpgradeInit` will verify that the upgrade parameters 
are valid and perform any custom `UpgradeInit` logic.
It may return an error if the chosen parameters are invalid 
in which case the upgrade handshake is aborted.
The callback is provided both the previous version of the channel and the new proposed version. It may perform the necessary logic and state changes necessary to upgrade the channel from the previous version to the new version. If upgrading the application from the previous version to the new version is not supported, it must return an error.

If an error is returned, then core IBC will revert any changes made by `onChanUpgradeInit` and abort the handshake.

`onChanUpgradeInit` is also responsible for making sure that the application is recoverable to its pre-upgrade state. The application may either store any new metadata in separate paths, or store the previous metadata under a different path so it can be restored.

```typescript
function onChanUpgradeInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  upgradeSequence: uint64,
  proposedOrdering: ChannelOrder,
  proposedConnectionHops: [Identifier],
  proposedVersion: string) => (version: string, err: Error) {
    // defined by the module
}
```

#### **OnChanUpgradeTry**

`onChanUpgradeTry` will verify the upgrade-chosen parameters and perform custom `TRY` logic. 
If the upgrade-chosen parameters are invalid, the callback must return an error to abort the handshake. 
If the counterparty-chosen version is not compatible with this modules
supported versions, the callback must return an error to abort the handshake. 
If the versions are compatible, the try callback must select the final version
string and return it to core IBC.
If upgrading from the previous version to the final new version is not supported, it must return an error.
`onChanUpgradeTry` may also perform custom initialization logic.

If an error is returned, then core IBC will revert any changes made by `onChanUpgradeTry` and abort the handshake.

`onChanUpgradeTry` is also responsible for making sure that the application is recoverable to its pre-upgrade state. The application may either store any new metadata in separate paths, or store the previous metadata under a different path so it can be restored.

```typescript
function onChanUpgradeTry(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  upgradeSequence: uint64,
  proposedOrdering: ChannelOrder,
  proposedConnectionHops: [Identifier],
  proposedVersion: string) => (version: string, err: Error) {
    // defined by the module
}
```

#### **OnChanUpgradeAck**

`onChanUpgradeAck` will error if the counterparty selected version string
is invalid. If an error is returned by the callback, core IBC will revert any changes made by `onChanUpgradeAck` and abort the handshake.

The `onChanUpgradeAck` callback may also perform custom ACK logic.

After `onChanUpgradeAck` returns successfully, the application upgrade is complete on this end so any 
auxilliary data stored for the purposes of recovery is no longer needed and may be deleted.

If the callback returns successfully, the application MUST have its state fully migrated to start processing packet data according to the new application parameters.

```typescript
function onChanUpgradeAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string) => Error {
    // defined by the module
}
```

#### **OnChanUpgradeOpen**

`onChanUpgradeOpen` will perform custom OPEN logic. It MUST NOT error since the counterparty has already approved the handshake, and transitioned to using the new upgrade parameters.

After `onChanUpgradeOpen` returns, the application upgrade is complete so any 
auxilliary data stored for the purposes of recovery is no longer needed and may be deleted.

The application MUST have its state fully migrated to start processing packet data according to the new application parameters by the time the callback returns.

```typescript
function onChanUpgradeOpen(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}
```

#### **OnChanUpgradeRestore**

`onChanUpgradeRestore` will be called on `cancelChannelUpgrade` and `timeoutChannelUpgrade` to restore the application to its pre-upgrade state.

After the upgrade restore callback is returned, the application must have any application metadata back to its pre-upgrade state. Any temporary metadata stored for the purpose of transitioning to the upgraded state may be deleted.

The application MUST have its state fully migrated to start processing packet data according to the original application parameters by the time the callback returns.

```typescript
function onChanUpgradeRestore(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}
```