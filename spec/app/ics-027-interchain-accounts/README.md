---
ics: 27
title: Interchain Accounts
stage: Draft
category: IBC/APP
requires: 25, 26
kind: instantiation
version compatibility:
author: Tony Yun <tony@chainapsis.com>, Dogemos <josh@tendermint.com>, Sean King <sean@interchain.io>
created: 2019-08-01
modified: 2020-07-14
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the account management system over an IBC channel between separate chains.

### Motivation

ICS-27 Interchain Accounts outlines a cross-chain account management protocol built upon IBC. ICS-27 enabled chains can programmatically create accounts on other ICS-27 enabled chains & control these accounts via IBC transactions (instead of signing with a private key). Interchain accounts retain all of the capabilities of a normal account (i.e. stake, send, vote) but instead are managed by a separate chain via IBC in a way such that the owner account on the controller chain retains full control over any interchain account(s) it registers on host chain(s). 

### Definitions 

- `Host Chain`: The chain where the interchain account is registered. The host chain listens for IBC packets from a controller chain which contain instructions (e.g. cosmos SDK messages) that the interchain account will execute.
- `Controller Chain`: The chain registering and controlling an account on a host chain. The controller chain sends IBC packets to the host chain to control the account.
- `Interchain Account`: An account on a host chain. An interchain account has all the capabilities of a normal account. However, rather than signing transactions with a private key, a controller chain will send IBC packets to the host chain which signals what transactions the interchain account must execute. 
- `Interchain Account Owner`: An account on the controller chain. Every interchain account on a host chain has a respective owner account on the controller chain. 

The IBC handler interface & IBC relayer module interface are as defined in [ICS-25](../../core/ics-025-handler-interface) and [ICS-26](../../core/ics-026-routing-module), respectively.

`channelCapabilityPath` is as defined in [ICS 4](../../core/ics-004-channel-and-packet-semantics).

`claimCapability` is as defined in [ICS 5](../../core/ics-005-port-allocation).

### Desired properties

- Permissionless: An interchain account may be created by any actor without the approval of a third party (e.g. chain governance). Note: Individual implementations may implement their own permissioning scheme, however the protocol must not require permissioning from a trusted party to be secure.
- Fault isolation: A controller chain must not be able to control accounts registered by other controller chains. For example, in the case of a fork attack on a controller chain, only the interchain accounts registered by the forked chain will be vulnerable.
- The ordering of transactions sent to an interchain account on a host chain must be maintained. Transactions must be executed by an interchain account in the order in which they are sent by the controller chain.
- If a channel closes, the controller chain must be able to regain access to registered interchain accounts by simply opening a new channel.
- Each interchain account is owned by a single account on the controller chain. Only the owner account on the controller chain is authorized to control the interchain account. The controller chain is responsible for enforcing this logic.
- The controller chain must store the account address of any owned interchain accounts registered on host chains.
- A host chain must have the ability to limit interchain account functionality on its chain as necessary (e.g. a host chain can decide that interchain accounts registered on the host chain cannot take part in staking).

## Technical specification

### General design 

A chain can utilize one or both parts of the interchain accounts protocol (*controlling* and *hosting*). A controller chain that registers accounts on other host chains (that support interchain accounts) does not necessarily have to allow other controller chains to register accounts on its chain, and vice versa. 

This specification defines the general way to register an interchain account and send tx bytes to be executed on behalf of the owner account. The host chain is responsible for deserializing and executing the tx bytes and the controller chain must know how the host chain will handle the tx bytes in advance of sending a packet, thus this must be negotiated during channel creation.

### Controller chain contract

#### **RegisterInterchainAccount**

`RegisterInterchainAccount` is the entry point to registering an interchain account.
It generates a new controller portID using the owner account address.
It will bind to the controller portID and
call 04-channel `ChanOpenInit`. An error is returned if the controller portID is already in use.
A `ChannelOpenInit` event is emitted which can be picked up by an offchain process such as a relayer.
The account will be registered during the `OnChanOpenTry` step on the host chain.
This function must be called after an `OPEN` connection is already established with the given connection identifier.
The caller must provide the complete channel version. This MUST include the ICA version with complete metadata and it MAY include 
versions of other middleware that is wrapping ICA on both sides of the channel. Note this will require contextual information
on what middleware is enabled on either end of the channel. Thus it is recommended that an ICA-auth application construct the ICA
version automatically and allow for users to optionally enable additional middleware versioning.

```typescript
function RegisterInterchainAccount(connectionId: Identifier, owner: string, version: string) returns (error) {
}
```

#### **SendTx**

`SendTx` is used to send an IBC packet containing instructions (messages) to an interchain account on a host chain for a given interchain account owner.

```typescript
function SendTx(
  capability: CapabilityKey, 
  connectionId: Identifier,
  portId: Identifier, 
  icaPacketData: InterchainAccountPacketData, 
  timeoutTimestamp uint64
): uint64 {
  // check if there is a currently active channel for
  // this portId and connectionId, which also implies an 
  // interchain account has been registered using 
  // this portId and connectionId
  activeChannelID, found = GetActiveChannelID(portId, connectionId)
  abortTransactionUnless(found)

  // validate timeoutTimestamp
  abortTransactionUnless(timeoutTimestamp <= currentTimestamp())

  // validate icaPacketData
  abortTransactionUnless(icaPacketData.type == EXECUTE_TX)
  abortTransactionUnless(icaPacketData.data != nil)

  // send icaPacketData to the host chain on the active channel
  sequence = handler.sendPacket(
    capability,
    portId, // source port ID
    activeChannelID, // source channel ID 
    0,
    timeoutTimestamp,
    protobuf.marshal(icaPacketData) protobuf-marshalled bytes of packet data
  )

  return sequence
}
```

### Host chain contract

#### **RegisterInterchainAccount**

`RegisterInterchainAccount` is called on the `OnChanOpenTry` step during the channel creation handshake.

```typescript
function RegisterInterchainAccount(counterpartyPortId: Identifier, connectionID: Identifier) returns (nil) {
  // checks to make sure the account has not already been registered
  // creates a new address on chain deterministically given counterpartyPortId and underlying connectionID
  // calls SetInterchainAccountAddress()
}
```

#### **AuthenticateTx**

`AuthenticateTx` is called before `ExecuteTx`.
`AuthenticateTx` checks that the signer of a particular message is the interchain account associated with the counterparty portID of the channel that the IBC packet was sent on.

```typescript
function AuthenticateTx(msgs []Any, connectionId string, portId string) returns (error) {
  // GetInterchainAccountAddress(portId, connectionId)
  // if interchainAccountAddress != msgSigner return error
}
```

#### **ExecuteTx**

Executes each message sent by the owner account on the controller chain.

```typescript
function ExecuteTx(sourcePort: Identifier, channel Channel, msgs []Any) returns (resultString, error) {
  // validate each message
  // retrieve the interchain account for the given channel by passing in source port and channel's connectionID
  // verify that interchain account is authorized signer of each message
  // execute each message
  // return result of transaction
}
```

### Utility functions

```typescript
// Sets the active channel for a given portID and connectionID.
function SetActiveChannelID(portId: Identifier, connectionId: Identifier, channelId: Identifier) returns (error){
}

// Returns the ID of the active channel for a given portID and connectionID, if present.
function GetActiveChannelID(portId: Identifier, connectionId: Identifier) returns (Identifier, boolean){
}

// Stores the address of the interchain account in state.
function SetInterchainAccountAddress(portId: Identifier, connectionId: Identifier, address: string) returns (string) {
}

// Retrieves the interchain account from state.
function GetInterchainAccountAddress(portId: Identifier, connectionId: Identifier) returns (string, bool){
}
```

### Register & controlling flows

#### Register account flow

To register an interchain account we require an off-chain process (relayer) to listen for `ChannelOpenInit` events with the capability to finish a channel creation handshake on a given connection. 

1. The controller chain binds a new IBC port with the controller portID for a given *interchain account owner address*.

This port will be used to create channels between the controller & host chain for a specific owner/interchain account pair. Only the account with `{owner-account-address}` matching the bound port will be authorized to send IBC packets over channels created with the controller portID. It is up to each controller chain to enforce this port registration and access on the controller side. 

2. The controller chain emits an event signaling to open a new channel on this port given a connection. 
3. A relayer listening for `ChannelOpenInit` events will continue the channel creation handshake.
4. During the `OnChanOpenTry` callback on the host chain an interchain account will be registered and a mapping of the interchain account address to the owner account address will be stored in state (this is used for authenticating transactions on the host chain at execution time). 
5. During the `OnChanOpenAck` callback on the controller chain a record of the interchain account address registered on the host chain during `OnChanOpenTry` is set in state with a mapping from portID -> interchain account address. See [metadata negotiation](#metadata-negotiation) section below for how to implement this.
6. During the `OnChanOpenAck` & `OnChanOpenConfirm` callbacks on the controller & host chains respectively, the [active-channel](#active-channels) for this interchain account/owner pair, is set in state.

#### Active channels

The controller and host chain must keep track of an `active-channel` for each registered interchain account. The `active-channel` is set during the channel creation handshake process. This is a safety mechanism that allows a controller chain to regain access to an interchain account on a host chain in case of a channel closing. 

An example of an active channel on the controller chain can look like this:

```typescript
{
  // Controller Chain
  SourcePortId: `icacontroller-<owner-account-address>`,
  SourceChannelId: `<channel-id>`,
  // Host Chain
  CounterpartyPortId: `icahost`,
  CounterpartyChannelId: `<channel-id>`,
}
```

In the event of a channel closing, the active channel may be replaced by starting a new channel handshake with the same port identifiers on the same underlying connection of the original active channel. ICS-27 channels can only be closed in the event of a timeout (if the implementation uses ordered channels) or in the unlikely event of a light client attack. Controller chains must retain the ability to open new ICS-27 channels and reset the active channel for a particular portID (containing `{owner-account-address}`) and connectionID pair.

The controller and host chains must verify that any new channel maintains the same metadata as the previous active channel to ensure that the parameters of the interchain account remain the same even after replacing the active channel. The `Address` of the metadata should not be verified since it is expected to be empty at the INIT stage, and the host chain will regenerate the exact same address on TRY, because it is expected to generate the interchain account address deterministically from the controller portID and connectionID (both of which must remain the same).

#### **Metadata negotiation**

ICS-27 takes advantage of [ICS-04 channel version negotiation](../../core/ics-004-channel-and-packet-semantics/README.md#versioning) to negotiate metadata and channel parameters during the channel handshake. The metadata will contain the encoding format along with the transaction type so that the counterparties can agree on the structure and encoding of the interchain transactions. The metadata sent from the host chain on the TRY step will also contain the interchain account address, so that it can be relayed to the controller chain. At the end of the channel handshake, both the controller and host chains will store a mapping of the controller chain portID to the newly registered interchain account address ([account registration flow](#register-account-flow)). 

ICS-04 allows for each channel version negotiation to be application-specific. In the case of interchain accounts, the channel version will be a string of a JSON struct containing all the relevant metadata intended to be relayed to the counterparty during the channel handshake step ([see summary below](#metadata-negotiation-summary)).

Combined with the one channel per interchain account approach, this method of metadata negotiation allows us to pass the address of the interchain account back to the controller chain and create a mapping from controller portID -> interchain account address during the `OnChanOpenAck` callback. As outlined in the [controlling flow](#controlling-flow), a controller chain will need to know the address of a registered interchain account in order to send transactions to the account on the host chain.

#### **Metadata negotiation summary**

`interchain-account-address` is the address of the interchain account registered on the host chain by the controller chain.

- **INIT**

Initiator: Controller

Datagram: ChanOpenInit

Chain Acted Upon: Controller

Version: 

```json
{
  "Version": "ics27-1",
  "ControllerConnectionId": "self_connection_id",
  "HostConnectionId": "counterparty_connection_id",
  "Address": "",
  "Encoding": "requested_encoding_type",
  "TxType": "requested_tx_type",
}
```

Comments: The address is left empty since this will be generated and relayed back by the host chain. The connection identifiers must be included to ensure that if a new channel needs to be opened (in case active channel times out), then we can ensure that the new channel is opened on the same connection. This will ensure that the interchain account is always connected to the same counterparty chain.

- **TRY**

Initiator: Relayer

Datagram: ChanOpenTry

Chain Acted Upon: Host

Version: 

```json
{
  "Version": "ics27-1",
  "ControllerConnectionId": "counterparty_connection_id",
  "HostConnectionId": "self_connection_id",
  "Address": "interchain_account_address",
  "Encoding": "negotiated_encoding_type",
  "TxType": "negotiated_tx_type",
}
```

Comments: The ICS-27 application on the host chain is responsible for returning this version given the counterparty version set by the controller chain in INIT. The host chain must agree with the single encoding type and a single tx type that is requested by the controller chain (ie. included in counterparty version). If the requested encoding or tx type is not supported, then the host chain must return an error and abort the handshake.
The host chain must also generate the interchain account address and populate the address field in the version with the interchain account address string.

- **ACK** 

Initiator: Relayer

Datagram: ChanOpenAck

Chain Acted Upon: Controller

CounterpartyVersion: 

```json
{
  "Version": "ics27-1",
  "ControllerConnectionId": "self_connection_id",
  "HostConnectionId": "counterparty_connection_id",
  "Address": "interchain_account_address",
  "Encoding": "negotiated_encoding_type",
  "TxType": "negotiated_tx_type",
}
```

Comments: On the ChanOpenAck step, the ICS27 application on the controller chain must verify the version string chosen by the host chain on ChanOpenTry. The controller chain must verify that it can support the negotiated encoding and tx type selected by the host chain. If either is unsupported, then it must return an error and abort the handshake.
If both are supported, then the controller chain must store a mapping from the channel's portID to the provided interchain account address and return successfully.

#### Controlling flow

Once an interchain account is registered on the host chain a controller chain can begin sending instructions (messages) to the host chain to control the account. 

1. The controller chain calls `SendTx` and passes message(s) that will be executed on the host side by the associated interchain account (determined by the controller side port identifier)

Cosmos SDK pseudo-code example:

```golang
interchainAccountAddress := GetInterchainAccountAddress(portId)
msg := &banktypes.MsgSend{FromAddress: interchainAccountAddress, ToAddress: ToAddress, Amount: amount}
icaPacketData = InterchainAccountPacketData{
  Type: types.EXECUTE_TX,
  Data: serialize(msg),
  Memo: "memo",
}

// Sends the message to the host chain, where it will eventually be executed 
SendTx(ownerAddress, connectionId, portID, data, timeout)
```

2. The host chain upon receiving the IBC packet will call `DeserializeTx`. 
    
3. The host chain will then call `AuthenticateTx` and `ExecuteTx` for each message and return an acknowledgment containing a success or error.  

Messages are authenticated on the host chain by taking the controller side port identifier and calling `GetInterchainAccountAddress(controllerPortId)` to get the expected interchain account address for the current controller port. If the signer of this message does not match the expected account address then authentication will fail.

### Packet Data

`InterchainAccountPacketData` contains an array of messages that an interchain account can execute and a memo string that is sent to the host chain as well as the packet `type`. ICS-27 version 1 has only one type `EXECUTE_TX`.

```proto
message InterchainAccountPacketData  {
  enum type
  bytes data = 1;
  string memo = 2;
}
```

The acknowledgment packet structure is defined as in [ics4](https://github.com/cosmos/ibc-go/blob/main/proto/ibc/core/channel/v1/channel.proto#L135-L148). If an error occurs on the host chain the acknowledgment contains the error message.

```proto
message Acknowledgement {
  // response contains either a result or an error and must be non-empty
  oneof response {
    bytes  result = 21;
    string error  = 22;
  }
}
```

### Custom logic

ICS-27 relies on [ICS-30 middleware architecture](../ics-030-middleware) to provide the option for application developers to apply custom logic on the success or fail of ICS-27 packets. 

Controller chains will wrap `OnAcknowledgementPacket` & `OnTimeoutPacket` to handle the success or fail cases for ICS-27 packets. 

### Port & channel setup

The interchain account module on a host chain must always bind to a port with the id `icahost`. Controller chains will bind to ports dynamically, as specified in the identifier format [section](#identifier-formats).

The example below assumes a module is implementing the entire `InterchainAccountModule` interface. The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialized) to bind to the appropriate port.

```typescript
function setup() {
  capability = routingModule.bindPort("icahost", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onChanUpgradeInit, // read-only
    onChanUpgradeTry, // read-only
    onChanUpgradeAck, // read-only
    onChanUpgradeOpen,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
  claimCapability("port", capability)
}
```

Once the `setup` function has been called, channels can be created via the IBC routing module.

### Channel lifecycle management

An interchain account module will accept new channels from any module on another machine, if and only if:

- The channel being created is ordered.
- The channel initialization step is being invoked from the controller chain.

```typescript
// Called on Controller Chain by InitInterchainAccount
function onChanOpenInit(
  capability: CapabilityKey,
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string
): (version: string, err: Error) {
  // validate port format
  abortTransactionUnless(validateControllerPortParams(portIdentifier))
  // only allow channels to be created on the "icahost" port on the counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "icahost")

  if version != "" {
    // validate metadata
    metadata = UnmarshalJSON(version)
    abortTransactionUnless(metadata.Version === "ics27-1")
    // all elements in encoding list and tx type list must be supported
    abortTransactionUnless(IsSupportedEncoding(metadata.Encoding))
    abortTransactionUnless(IsSupportedTxType(metadata.TxType))

    // connectionID and counterpartyConnectionID is retrievable in Channel
    abortTransactionUnless(metadata.ControllerConnectionId === connectionId)
    abortTransactionUnless(metadata.HostConnectionId === counterpartyConnectionId)
  } else {
    // construct default metadata
    metadata = {
      Version: "ics27-1",
      ControllerConnectionId: connectionId,
      HostConnectionId: counterpartyConnectionId,
      // implementation may choose a default encoding and TxType
      // e.g. DefaultEncoding=protobuf, DefaultTxType=sdk.MultiMsg
      Encoding: DefaultEncoding,
      TxType: DefaultTxType,
    }
    version = marshalJSON(metadata)
  }
  
  // only open the channel if:
  // - there is no active channel already set (with status OPEN)
  // OR
  // - there is already an active channel (with status CLOSED) AND
  // the metadata matches exactly the existing metadata in the 
  // version string of the active channel AND the ordering of the 
  // new channel matches the ordering of the active channel.
  activeChannelId, activeChannelFound = GetActiveChannelID(portId, connectionId)
  if activeChannelFound {
    activeChannel = provableStore.get(channelPath(portId, activeChannelId))
    abortTransactionUnless(channel !== null)
    abortTransactionUnless(activeChannel.state === CLOSED)
    previousOrder = activeChannel.order
    abortTransactionUnless(previousOrder === order)
    previousMetadata = UnmarshalJSON(activeChannel.version)
    abortTransactionUnless(previousMetadata === metadata)
  } else {
    // claim channel capability
    claimCapability(channelCapabilityPath(portIdentifier, channelIdentifier), capability)
  }

  return version, nil
}
```

```typescript
// Called on Host Chain by Relayer
function onChanOpenTry(
  capability: CapabilityKey,
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string
): (version: string, err: Error) {
  // validate port ID
  abortTransactionUnless(portIdentifier === "icahost")
  // create the interchain account with the counterpartyPortIdentifier
  // and the underlying connectionID on the host chain.
  address = RegisterInterchainAccount(counterpartyPortIdentifier, connectionID)

  cpMetadata = UnmarshalJSON(counterpartyVersion)
  abortTransactionUnless(cpMetadata.Version === "ics27-1")
  // If encoding or txType requested by initializing chain is not supported by host chain then
  // fail handshake and abort transaction
  abortTransactionUnless(IsSupportedEncoding(cpMetadata.Encoding))
  abortTransactionUnless(IsSupportedTxType(cpMetadata.TxType))

  // connectionID and counterpartyConnectionID is retrievable in Channel
  abortTransactionUnless(cpMetadata.ControllerConnectionId === counterpartyConnectionId)
  abortTransactionUnless(cpMetadata.HostConnectionId === connectionId)
  
  metadata = {
    "Version": "ics27-1",
    "ControllerConnectionId": cpMetadata.ControllerConnectionId,
    "HostConnectionId": cpMetadata.HostConnectionId,
    "Address": address,
    "Encoding": cpMetadata.Encoding,
    "TxType": cpMetadata.TxType,
  }

  return string(MarshalJSON(metadata)), nil
}
```

```typescript
// Called on Controller Chain by Relayer
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier,
  counterpartyVersion: string
) {
  // validate counterparty metadata decided by host chain
  metadata = UnmarshalJSON(version)
  abortTransactionUnless(metadata.Version === "ics27-1")
  abortTransactionUnless(IsSupportedEncoding(metadata.Encoding))
  abortTransactionUnless(IsSupportedTxType(metadata.TxType))
  abortTransactionUnless(metadata.ControllerConnectionId === connectionId)
  abortTransactionUnless(metadata.HostConnectionId === counterpartyConnectionId)
  
  // state change to keep track of successfully registered interchain account
  SetInterchainAccountAddress(portID, metadata.Address)
  // set the active channel for this owner/interchain account pair
  SetActiveChannelID(portIdentifier, metadata.ControllerConnectionId, channelIdentifier)
}
```

```typescript
// Called on Host Chain by Relayer
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier
) {
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel !== null)

  // set the active channel for this owner/interchain account pair
  SetActiveChannelID(channel.counterpartyPortIdentifier, channel.connectionHops[0], channelIdentifier)
}
```

```typescript
// The controller portID must have the format: `icacontroller-{ownerAddress}`
function validateControllerPortParams(portIdentifier: Identifier) {
  split(portIdentifier, "-")
  abortTransactionUnless(portIdentifier[0] === "icacontroller")
  abortTransactionUnless(IsValidAddress(portIdentifier[1]))
}
```

### Closing handshake

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
 	// disallow user-initiated channel closing for interchain account channels
  abortTransactionUnless(FALSE)
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
}
```

### Upgrade handshake

```typescript
// Called on Controller Chain by Authority
function onChanUpgradeInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  order: ChannelOrder,
  connectionHops: [Identifier],
  upgradeSequence: uint64,
  version: string
): (version: string, err: Error) {
  // new version proposed in the upgrade
  abortTransactionUnless(version !== "")
  metadata = UnmarshalJSON(version)

  // retrieve the existing channel version.
  // In ibc-go, for example, this is done using the GetAppVersion 
  // function of the ICS4Wrapper interface.
  // See https://github.com/cosmos/ibc-go/blob/ac6300bd857cd2bd6915ae51e67c92848cbfb086/modules/core/05-port/types/module.go#L128-L132
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel !== null)
  currentMetadata = UnmarshalJSON(channel.version)

  // validate metadata
  abortTransactionUnless(metadata.Version === "ics27-1")
  // all elements in encoding list and tx type list must be supported
  abortTransactionUnless(IsSupportedEncoding(metadata.Encoding))
  abortTransactionUnless(IsSupportedTxType(metadata.TxType))

  // the interchain account address on the host chain
  // must remain the same after the upgrade.
  abortTransactionUnless(currentMetadata.Address === metadata.Address)

  // at the moment it is not supported to perform upgrades that
  // change the connection ID of the controller or host chains.
  // therefore these connection IDs much remain the same as before.
  abortTransactionUnless(currentMetadata.ControllerConnectionId === metadata.ControllerConnectionId)
  abortTransactionUnless(currentMetadata.HostConnectionId === metadata.HostConnectionId)
  // the proposed connection hop must not change
  abortTransactionUnless(currentMetadata.ControllerConnectionId === connectionHops[0])
  
  version = marshalJSON(metadata)
  return version, nil
}
```

```typescript
// Called on Host Chain by Relayer
function onChanUpgradeTry(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  order: ChannelOrder,
  connectionHops: [Identifier],
  upgradeSequence: uint64,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string
): (version: string, err: Error) {
  // validate port ID
  abortTransactionUnless(portIdentifier === "icahost")

  // upgrade version proposed by counterparty
  abortTransactionUnless(counterpartyVersion !== "")

  // retrieve the existing channel version.
  // In ibc-go, for example, this is done using the GetAppVersion 
  // function of the ICS4Wrapper interface.
  // See https://github.com/cosmos/ibc-go/blob/ac6300bd857cd2bd6915ae51e67c92848cbfb086/modules/core/05-port/types/module.go#L128-L132
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel !== null)
  currentMetadata = UnmarshalJSON(channel.version)

  // validate metadata
  abortTransactionUnless(metadata.Version === "ics27-1")
  // all elements in encoding list and tx type list must be supported
  abortTransactionUnless(IsSupportedEncoding(metadata.Encoding))
  abortTransactionUnless(IsSupportedTxType(metadata.TxType))

  // the interchain account address on the host chain
  // must remain the same after the upgrade.
  abortTransactionUnless(currentMetadata.Address === metadata.Address)

  // at the moment it is not supported to perform upgrades that
  // change the connection ID of the controller or host chains.
  // therefore these connection IDs much remain the same as before.
  abortTransactionUnless(currentMetadata.ControllerConnectionId === metadata.ControllerConnectionId)
  abortTransactionUnless(currentMetadata.HostConnectionId === metadata.HostConnectionId)
  // the proposed connection hop must not change
  abortTransactionUnless(currentMetadata.HostConnectionId === connectionHops[0])

  return counterpartyVersion, nil
}
```

```typescript
// Called on Controller Chain by Relayer
function onChanUpgradeAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyVersion: string
): Error {
  // final upgrade version proposed by counterparty
  abortTransactionUnless(counterpartyVersion !== "")
  metadata = UnmarshalJSON(counterpartyVersion)

  // retrieve the existing channel version.
  // In ibc-go, for example, this is done using the GetAppVersion 
  // function of the ICS4Wrapper interface.
  // See https://github.com/cosmos/ibc-go/blob/ac6300bd857cd2bd6915ae51e67c92848cbfb086/modules/core/05-port/types/module.go#L128-L132
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(channel !== null)
  currentMetadata = UnmarshalJSON(channel.version)

  // validate metadata
  abortTransactionUnless(metadata.Version === "ics27-1")
  // all elements in encoding list and tx type list must be supported
  abortTransactionUnless(IsSupportedEncoding(metadata.Encoding))
  abortTransactionUnless(IsSupportedTxType(metadata.TxType))

  // the interchain account address on the host chain
  // must remain the same after the upgrade.
  abortTransactionUnless(currentMetadata.Address === metadata.Address)

  // at the moment it is not supported to perform upgrades that
  // change the connection ID of the controller or host chains.
  // therefore these connection IDs much remain the same as before.
  abortTransactionUnless(currentMetadata.ControllerConnectionId === metadata.ControllerConnectionId)
  abortTransactionUnless(currentMetadata.HostConnectionId === metadata.HostConnectionId)

  return nil
}
```

```typescript
// Called on Controller and Host Chains by Relayer
function onChanUpgradeOpen(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // no-op
} 
```

### Packet relay

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
// Called on Host Chain by Relayer
function onRecvPacket(packet Packet) {
  ack = NewResultAcknowledgement([]byte{byte(1)})

	// only attempt the application logic if the packet data
	// was successfully decoded
  switch data.Type {
  case types.EXECUTE_TX:
  msgs, err = types.DeserializeTx(data.Data)
  if err != nil {
    return NewErrorAcknowledgement(err)
  }

  // ExecuteTx calls the AuthenticateTx function defined above 
  result, err = ExecuteTx(ctx, packet.SourcePort, packet.DestinationPort, packet.DestinationChannel, msgs)
  if err != nil {
    // NOTE: The error string placed in the acknowledgement must be consistent across all
    // nodes in the network or there will be a fork in the state machine. 
    return NewErrorAcknowledgement(err)
  }

  // return acknowledgement containing the transaction result after executing on host chain
  return NewAcknowledgement(result)

  default:
    return NewErrorAcknowledgement(ErrUnknownDataType)
  }
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
// Called on Controller Chain by Relayer
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes
) {
  // call underlying app's OnAcknowledgementPacket callback 
  // see ICS-30 middleware for more information
}
```

```typescript
// Called on Controller Chain by Relayer
function onTimeoutPacket(packet: Packet) {
  // call underlying app's OnTimeoutPacket callback 
  // see ICS-30 middleware for more information
}
```

Note that interchain accounts controller modules should not execute any logic upon packet receipt, i.e. the `OnRecvPacket` callback should not be called, and in case it is called, it should simply return an error acknowledgement:

```typescript
// Called on Controller Chain by Relayer
function onRecvPacket(packet Packet) {
  return NewErrorAcknowledgement(ErrInvalidChannelFlow)
}
```

### Identifier formats

These are the default formats that the port identifiers on each side of an interchain accounts channel. The controller portID **must** include the owner address so that when a message is sent to the controller module, the sender of the message can be verified against the portID before sending the ICA packet. The controller chain is responsible for proper access control to ensure that the sender of the ICA message has successfully authenticated before the message reaches the controller module.

Controller Port Identifier: optional prefix `icacontroller-` + mandatory `{owner-account-address}`

Host Port Identifier: `icahost`

The `icacontroller-` prefix on the controller port identifier is optional and host chains **must** not enforce that the counterparty port identifier includes it. Controller chains may decide to include it and validate that it is present in their own port identifier.

## Example Implementations

- Implementation of ICS 27 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).

## Future Improvements

A future version of interchain accounts may be greatly simplified by the introduction of an IBC channel type that is ORDERED but does not close the channel on timeouts, and instead proceeds to accept and receive the next packet. If such a channel type is made available by core IBC, Interchain accounts could require the use of this channel type and remove all logic and state pertaining to "active channels". The metadata format can also be simplified to remove any reference to the underlying connection identifiers.

The "active channel" setting and unsetting is currently necessary to allow interchain account owners to create a new channel in case the current active channel closes during channel timeout. The connection identifiers are part of the metadata to ensure that any new channel that gets opened are established on top of the original connection. All of this logic becomes unnecessary once the channel is ordered **and** unclosable, which can only be achieved by the introduction of a new channel type to core IBC.

## History

Aug 1, 2019 - Concept discussed

Sep 24, 2019 - Draft suggested

Nov 8, 2019 - Major revisions

Dec 2, 2019 - Minor revisions (Add more specific description & Add interchain account on Ethereum)

July 14, 2020 - Major revisions

April 27, 2021 - Redesign of ics27 specification

November 11, 2021 - Update with latest changes from implementation

December 14, 2021 - Revisions to spec based on audits and maintainer reviews

August 1, 2023 - Implemented channel upgrades callbacks
    
## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
