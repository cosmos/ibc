# Application Upgrade Callbacks

## Synopsis

This standard document specifies the interfaces and state machine logic that IBC applications must implement in order to enable existing channels to upgrade their applications after the initial channel handshake.

### Motivation

As new features get added to IBC applications, chains may wish to take advantage of new application features without abandoning the accumulated state and network effect(s) of an already existing channel. The upgrade protocol proposed would allow applications to renegotiate an existing channel to take advantage of new features without having to create a new channel, thus preserving all existing application state while upgrading to new application logic.

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
  onChanUpgradeInit: onChanUpgradeInit, // read-only
  onChanUpgradeTry: onChanUpgradeTry, // read-only
  onChanUpgradeAck: onChanUpgradeAck, // read-only
  onChanUpgradeOpen: onChanUpgradeOpen
}
```

### **OnChanUpgradeInit**

`onChanUpgradeInit` will verify that the upgrade parameters 
are valid and perform any custom `UpgradeInit` logic.
It may return an error if the chosen parameters are invalid 
in which case the upgrade handshake is aborted.
The callback is provided the new upgrade parameters. It may perform the necessary checks to ensure that it can support the new channel parameters. If upgrading the application from the previous parameters to the new parameters is not supported, it must return an error.

`onChanUpgradeInit` may return a modified version to be stored in the upgrade. This may occur if the application needs to store some metadata in the version string as part of its channel negotiation.

If an error is returned, then core IBC will abort the handshake.

`onChanUpgradeInit` MUST NOT write any state changes as this will be done only once the upgrade is completed and confirmed to succeed on both sides

```typescript
function onChanUpgradeInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proposedOrdering: ChannelOrder,
  proposedConnectionHops: [Identifier],
  proposedVersion: string) => (version: string, err: Error) {
    // defined by the module
} (version: string)
```

### **OnChanUpgradeTry**

`onChanUpgradeTry` will verify the upgrade-chosen parameters from the counterparty. 
If the upgrade-chosen parameters are unsupported by the application, the callback must return an error to abort the handshake. 
The try callback may return a modified version, in case it needs to add some metadata to the version string.
This will be stored as the final proposed version of the upgrade by core IBC.

`onChanUpgradeTry` MUST NOT write any state changes as this will be done only once the upgrade is completed and confirmed to succeed on both sides.

```typescript
function onChanUpgradeTry(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  proposedOrdering: ChannelOrder,
  proposedConnectionHops: [Identifier],
  proposedVersion: string) => (version: string, err: Error) {
    // defined by the module
} (version: string)
```

### **OnChanUpgradeAck**

`onChanUpgradeAck` will error if the counterparty selected version string
is unsupported. If an error is returned by the callback, core IBC will abort the handshake.

`onChanUpgradeAck` MUST NOT write any state changes as this will be done only once the upgrade is completed and confirmed to succeed on both sides.

```typescript
function onChanUpgradeAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string) => Error {
    // defined by the module
}
```

### **OnChanUpgradeOpen**

`onChanUpgradeOpen` is called after the upgrade is complete and both sides are guaranteed to move to the new channel parameters. Thus, the application may now perform any state migrations necessary to start supporting packet processing according to the new channel parameters.

```typescript
function onChanUpgradeOpen(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}
```
