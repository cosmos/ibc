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
If the provided version string is empty, `onChanUpgradeInit` should return 
a default version string or an error if the provided version is invalid.
Note that if the upgrade provides an empty string, this is an indication to upgrade
to the default version which MAY be a new default from when the channel was first initiated.
If there is no default version string for the application,
it should return an error if provided version is empty string.

`onChanUpgradeInit` is also responsible for making sure that the application is recoverable to its pre-upgrade state. The application may either store any new metadata in separate paths, or store the previous metadata under a different path so it can be restored.

```typescript
function onChanUpgradeInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
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
`onChanUpgradeTry` may also perform custom initialization logic.

`onChanUpgradeTry` is also responsible for making sure that the application is recoverable to its pre-upgrade state. The application may either store any new metadata in separate paths, or store the previous metadata under a different path so it can be restored.

```typescript
function onChanUpgradeTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) => (version: string, err: Error) {
    // defined by the module
}
```

#### **OnChanUpgradeAck**

`onChanUpgradeAck` will error if the counterparty selected version string
is invalid to abort the handshake. It may also perform custom ACK logic.

After `onChanUpgradeAck` returns, the application upgrade is complete on this end so any 
auxilliary data stored for the purposes of recovery is no longer needed and may be deleted.

The application MUST have its state fully migrated to start processing packet data according to the new application parameters by the time the callback returns.

```typescript
function onChanUpgradeAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier, 
  counterpartyVersion: string) {
    // defined by the module
}
```

#### **OnChanUpgradeConfirm**

`onChanUpgradeConfirm` will perform custom CONFIRM logic. It MUST NOT error since the counterparty has already approved the handshake, and transitioned to using the new upgrade parameters.

After `onChanUpgradeConfirm` returns, the application upgrade is complete so any 
auxilliary data stored for the purposes of recovery is no longer needed and may be deleted.

The application MUST have its state fully migrated to start processing packet data according to the new application parameters by the time the callback returns.

```typescript
function onChanUpgradeConfirm(
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