---
ics: 20
title: Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
version compatibility: ibc-go v7.0.0
author: Christopher Goes <cwgoes@interchain.berlin>
created: 2019-07-15 
modified: 2020-02-24
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the transfer of fungible tokens over an IBC channel between two modules on separate chains. The state machine logic presented allows for safe multi-chain denomination handling with permissionless channel opening. This logic constitutes a "fungible token transfer bridge module", interfacing between the IBC routing module and an existing asset tracking module on the host state machine.

### Motivation

Users of a set of chains connected over the IBC protocol might wish to utilise an asset issued on one chain on another chain, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing chain. This application-layer standard describes a protocol for transferring fungible tokens between chains connected with IBC which preserves asset fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

### Definitions

The IBC handler interface & IBC routing module interface are as defined in [ICS 25](../../core/ics-025-handler-interface) and [ICS 26](../../core/ics-026-routing-module), respectively.

### Desired Properties

- Preservation of fungibility (two-way peg).
- Preservation of total supply (constant or inflationary on a single source chain & module).
- Permissionless token transfers, no need to whitelist connections, modules, or denominations.
- Symmetric (all chains implement the same logic, no in-protocol differentiation of hubs & zones).
- Fault containment: prevents Byzantine-inflation of tokens originating on chain `A`, as a result of chain `B`'s Byzantine behaviour (though any users who sent tokens to chain `B` may be at risk).

## Technical Specification

### Data Structures

Only one packet data type is required: `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, and receiving account. Additional fields may include the `memo` and `forwardingPath`. If an implementation does not recognize these fields, and receives a packet with these fields populated then it will error and the packet will timeout on the sender chain.

```typescript
interface FungibleTokenPacketData {
  denom: string
  amount: uint256
  sender: string
  receiver: string
  memo: string
  forwardingPath: string
}
```


As tokens are sent across chains using the ICS 20 protocol, they begin to accrue a record of channels for which they have been transferred across. This information is encoded into the `denom` field. 

The ICS 20 token denominations are represented by the form `{ics20Port}/{ics20Channel}/{denom}`, where `ics20Port` and `ics20Channel` are an ICS 20 port and channel on the current chain for which the funds exist. The prefixed port and channel pair indicate which channel the funds were previously sent through. Implementations are responsible for correctly parsing the IBC trace information from the base denomination. The way the reference ICS 20 implementation in ibc-go handles this is by taking advantage of the fact that it automatically generates channel identifiers with the format `channel-{n}`, where `n` is a integer greater or equal than 0. It can then correctly parse out the IBC trace information from the base denom which may have slashes, but will not have a substring of the form `{transfer-port-name}/channel-{n}`. If this assumption is broken, the trace information will be parsed incorrectly (i.e. part of the base denom will be misinterpreted as trace information). Thus chains must make sure that base denominations do not have the ability to create arbitrary prefixes that can mock the ICS 20 logic.

A sending chain may be acting as a source or sink zone. When a chain is sending tokens across a port and channel which are not equal to the last prefixed port and channel pair, it is acting as a source zone. When tokens are sent from a source zone, the destination port and channel will be prefixed onto the denomination (once the tokens are received) adding another hop to a tokens record. When a chain is sending tokens across a port and channel which are equal to the last prefixed port and channel pair, it is acting as a sink zone. When tokens are sent from a sink zone, the last prefixed port and channel pair on the denomination is removed (once the tokens are received), undoing the last hop in the tokens record. A more complete explanation is [present in the ibc-go implementation](https://github.com/cosmos/ibc-go/blob/457095517b7832c42ecf13571fee1e550fec02d0/modules/apps/transfer/keeper/relay.go#L18-L49).

The memo is not parsed by the ICS20 application and may be used for external users or higher-level middleware and smart contracts. If the memo is intended to be used within the state machine, it must be in JSON dictionary format with the relevant information keyed for the intended user.

The forwardingPath of the packet data is the path to the final destination of the tokens after they reach the immediate destination chain specified by the packet's channel identifiers. The forwardingPath may be specified as a list of port/channel identifier tuples prefixed by `channels:` or a list of chainIDs that will be resolved with an on-chain registry prefixed by `chains:`. The chainID specification may be unsupported, in which case the implementation should return an error acknowledgement. A special keyword: `origin` may be prefixed to the forwardingInfo list to instruct the forwarding logic to first unwind the token path to the origin chain before continuing with forwarding. If the specified forwardingPath is supported, the implementation will forward the path along one hop forward and asynchronously write the acknowledgement once the forwarded packet has returned.

Example valid forwardingPaths:
- `channels:transfer/channel-3/transfer/channel-2/transfer/channel-300`: On each receiving chain, the implementation will pop a (srcPortID, srcChannelID) from the front of the list, resolve to a channel, and then forward the tokens along. Once the forwardingPath is empty, it will send to specified receiver which must be a valid address at the final destination.
- `channels:origin/transfer/channel-4`: This forwardingPath is similar to the above, but it will first unwind the tokens to their origin chain before executing the logic described above.
- `channels:origin`: This will simply unwind the tokens to their origin chain and send to the intended receiver.
- `chains:cosmoshub/osmosis/juno`: On each receiving chain the implementation must first consult its local chain registry to resolve the chainID to a source port and source channel, then it will forward the tokens along that channel. Once the forwardingPath is empty, it will send to specified receiver which must be a valid address at the final destination.
- `chains:origin/juno/osmosis`: Similar to the previous logic, it will first unwind to the origin before resolving the first chainID and forwarding along the returned channel to forward along the rest of the path.
- `chains:origin`: The tokens will be unwound to the origin and sent to intended receiver. Note this does not require any read from a chain registry.

The acknowledgement data type describes whether the transfer succeeded or failed, and the reason for failure (if any).

```typescript
type FungibleTokenPacketAcknowledgement = FungibleTokenPacketSuccess | FungibleTokenPacketError;

interface FungibleTokenPacketSuccess {
  // This is binary 0x01 base64 encoded
  result: "AQ=="
}

interface FungibleTokenPacketError {
  error: string
}
```

Note that both the `FungibleTokenPacketData` as well as `FungibleTokenPacketAcknowledgement` must be JSON-encoded (not Protobuf encoded) when they serialized into packet data. Also note that `uint256` is string encoded when converted to JSON, but must be a valid decimal number of the form `[0-9]+`.

The fungible token transfer bridge module tracks escrow addresses associated with particular channels in state. Fields of the `ModuleState` are assumed to be in scope. Implementations that support token forwarding must maintain a separate mapping of addresses for forwarding tokens along a channel.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
  forwardEscrowAddresses: Map<Identifier, string>
}
```

### Store paths

In order to support the token forwarding feature, implementations must store the incoming packet against the corresponding outgoing packet identifiers through the `packetForwardPath` store path in the private store.

```typescript
function packetForwardPath(portID: string, channelID: string, sequence: uint64): string {
  return "packetForwardPath/{portID}/{channelID}/{sequence}"
}
```

### Utility Functions

### ics20Prefix

`ics20Prefix` returns the ICS20 trace encoded in the denomination. It does this by checking if the second element of the slash-split denomination is prefixed by the special keyword "channel-". If this is true, then the two elements comprise a (portID, channelID) tuple.

```typescript
function ics20Prefix(denomination: string): (prefix: string) {
  elems = split(denomination, "/")
  ics20Prefix = ""
  if len(elems) > 2 {
    for i = 1; i < len(elems); i + 2 {
      if hasPrefix(elems[i], "channel-") {
        ics20Prefix = ics20Prefix + elems[i-1] + elems[i]
      } else {
        break
      }
    }
  }
  return ics20Prefix
}
```

#### getNextHop

`getNextHop` returns the next port and channel identifier to send the tokens along from the specified forwardingPath and denomination. The function takes in the denomination since the forwardingPath may instruct first sending the tokens back to originating chain which will require reading the ICS20 trace encoded in the denomination. Note the denomination here should be the denomination in the incoming packet before the receive is complete.

```typescript
function getNextHop(
  forwardPath: string,
  denomination: string): (forwardPort: string, forwardChannel: string) {
    if hasPrefix(forwardPath, "channels:origin") || hasPrefix(forwardPath, "chains:origin") {
      abortTransactionUnless(ics20Prefix(denomination) == "")
      elems = split(denomination, "/")
      // next hop towards origin is at the start of the denomination
      // as portID/channelID
      return elems[0], elems[1]
    }
    if hasPrefix(forwardPath, "chains:") {
      chainList = removePrefix(forwardPath, "chains:")
      chainsArr = split(chainList, "/")
      // chain registry is not in scope for this spec
      // implementations may use an on-chain registry to resolve
      // chainIDs to (portID, channelID) pairs.
      // if it is unsupported, implementation will error for 
      // chainID-specified forwardingPaths.
      return registry.ResolveToPortAndChannel(chainsArr[0])
    } else if hasPrefix(forwardPath, "channels:") {
      channelList = removePrefix(forwardPath, "channels:")
      channelArr = split(channelList, "/")
      // return first (portID, channelID) pair
      return channelArr[0], channelArr[1]
    } else {
      // invalid forwarding path
      abortTransactionUnless(false)
    }
  }
```

#### pruneHop

`pruneHop` prunes the first hop of the forwardingPath and returns the truncated path. This must be called before forwarding the packet along so that the front of the forwardingPath is now the next hop that the destination chain must forward to in an outgoing packet. If we are still unwinding the origin, then the origin keyword will only be removed once the denomination is fully unprefixed by ICS20. Note the denomination passed in must be the denomination received on the executing chain, not the denomination specified in the incoming packet.

```typescript
function pruneHop(forwardingPath: string, denomination: string): (newForwardingPath: string) {
  if hasPrefix(forwardPath, "channels:origin") || hasPrefix(forwardPath, "chains:origin") {
    // if there is no ICS20 prefix on the denomination after the receive,
    // then we are already at the origin chain and can remove it from the forwardingPath.
    if (ics20Prefix(denomination) == "") {
      // this helper function will remove the origin keyword from the forwardPath
      removeStr(forwardingPath, "origin")
    } else {
      // still need to unwind to origin, so leave forwardPath as-is.
      return forwardPath
    }
  } else {
    if hasPrefix(forwardingPath, "chains:") {
      chainList = removePrefix(forwardPath, "chains:")
      chainArr = split(forwardPath, "/")
      // if there's only one chain left, we've finished forwarding so we can return empty string
      if len(chainArr) == 1 {
        return ""
      }
      return "chains:" + join(chainArr[1:], "/")
    } else if hasPrefix(forwardingPath, "channels:") {
        channelList = removePrefix(forwardPath, "channels:")
        channelArr = split(forwardPath, "/")
        // if there's only one (portID, channelID) pair left, we've finished forwarding so we can return empty string
        if len(channelArr) == 2 {
          return ""
        }
        return "channels:" + join(forwardPath[2:], "/")
    } else {
      abortTransactionUnless(false)
    }
  }
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "fungible token transfer bridge" module with access to a bank module and to the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("bank", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
  claimCapability("port", capability)
}
```

Once the `setup` function has been called, channels can be created through the IBC routing module between instances of the fungible token transfer module on separate chains.

An administrator (with the permissions to create connections & channels on the host state machine) is responsible for setting up connections to other state machines & creating channels
to other instances of this module (or another module supporting this interface) on other chains. This specification defines packet handling semantics only, and defines them in such a fashion
that the module itself doesn't need to worry about what connections or channels might or might not exist at any point in time.

#### Routing module callbacks

##### Channel lifecycle management

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The channel being created is unordered.
- The version string is `ics20-1`.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics20-1" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics20-1" || version === "")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
  return "ics20-1", nil
}
```

```typescript
function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) => (version: string, err: Error) {
  // only unordered channels allowed
  abortTransactionUnless(order === UNORDERED)
  // assert that version is "ics20-1"
  abortTransactionUnless(counterpartyVersion === "ics20-1")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress()
  // return version that this chain will use given the
  // counterparty version
  return "ics20-1", nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  // port has already been validated
  // assert that counterparty selected version is "ics20-1"
  abortTransactionUnless(counterpartyVersion === "ics20-1")
}
```

```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated, version has already been validated
}
```

```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // always abort transaction
    abortTransactionUnless(FALSE)
}
```

```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```

##### Packet relay

In plain English, between chains `A` and `B`:

- When acting as the source zone, the bridge module escrows an existing local asset denomination on the sending chain and mints vouchers on the receiving chain.
- When acting as the sink zone, the bridge module burns local vouchers on the sending chains and unescrows the local asset denomination on the receiving chain.
- When a packet times-out, local assets are unescrowed back to the sender or vouchers minted back to the sender appropriately.
- Acknowledgement data is used to handle failures, such as invalid denominations or invalid destination accounts. Returning
  an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending chain
  to take appropriate action based on the nature of the failure.

`sendFungibleTokens` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function sendFungibleTokens(
  denomination: string,
  amount: uint256,
  sender: string,
  receiver: string,
  memo: string,
  forwardingPath: string,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64): uint64 {
    prefix = "{sourcePort}/{sourceChannel}/"
    // we are the source if the denomination is not prefixed
    source = denomination.slice(0, len(prefix)) !== prefix
    if source {
      // if the denomination is not prefixed by an IBC trace at all, then the forwardingPath must not try to route to origin
      // since we are already at the origin chain.
      if ics20Prefix(denomination) == "" {
        // if the forwardingPath wants to unwind to origin chain but we are already origin, then abort
        abortTransactionUnless(!hasPrefix(forwardingPath, "channels:origin") && !hasPrefix(forwardingPath, "chains:origin"))
      }

      // determine escrow account
      escrowAccount = channelEscrowAddresses[sourceChannel]
      // escrow source tokens (assumed to fail if balance insufficient)
      bank.TransferCoins(sender, escrowAccount, denomination, amount)
    } else {
      // receiver is source chain, burn vouchers
      bank.BurnCoins(sender, denomination, amount)
    }

    // create FungibleTokenPacket data
    data = FungibleTokenPacketData{denomination, amount, sender, receiver}

    // send packet using the interface defined in ICS4
    sequence = handler.sendPacket(
      getCapability("port"),
      sourcePort,
      sourceChannel,
      timeoutHeight,
      timeoutTimestamp,
      data,
      memo,
      forwardingPath
    )

    return sequence
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  FungibleTokenPacketData data = unmarshal(packet.data)
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the packets were prefixed by the sending chain
  source = data.denom.slice(0, len(prefix)) === prefix
  // if we are forwarding the tokens along, we will move the coins to the forwardEscrowAddress to the source chain
  // otherwise send directy to receiver defined by packet
  if forwardingPath != "" {
    // pass in denomination before receive to getNextHop
    // i.e. denomination in packet
    forwardPort, forwardChannel = getNextHop(forwardingPath, packet.denomination)
    receiver = forwardEscrowAddress[forwardChannel]
  } else {
    receiver = data.receiver
  }
  if source {
    // receiver is source chain: unescrow tokens
    // determine escrow account
    escrowAccount = channelEscrowAddresses[packet.destChannel]
    denomination = data.denom.slice(len(prefix))
    // unescrow tokens to receiver (assumed to fail if balance insufficient)
    err = bank.TransferCoins(escrowAccount, receiver, denomination, data.amount)
    if (err !== nil) {
      return FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
    }
  } else {
    prefix = "{packet.destPort}/{packet.destChannel}/"
    denomination = prefix + data.denom
    // sender was source, mint vouchers to receiver (assumed to fail if balance insufficient)
    err = bank.MintCoins(receiver, denomination, data.amount)
    if (err !== nil)
      return FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
  }
  if forwardingPath != "" {
    // pruneHop will prune the first hop from the path
    // if the forwardingPath only contains one hop this will return
    // an empty string
    // pass in denomination after receive to pruneHop and the outgoing send
    forwardingPath = pruneHop(forwardingPath, denomination)
    forwardPacketSequence = sendFungibleTokens(
      denomination,
      amount,
      receiver, // the receiver of the old packet is the sender of the new one
      data.receiver, // final receiver is passed through
      memo,
      forwardingPath,
      forwardPort, // retrieved from above
      forwardChannel, // retrieved from above
      timeoutHeight: Height{}, // timeoutHeight unspecified for forwarding packets
      timeoutTimestamp: timeoutTimestamp, // use the same timeout
    )
    // store the original packet keyed on the forwarding packet
    store.set(packetForwardPath(forwardPort, forwardPacket, forwardPacketSequence), packet)
  } else {
    return ack
  }
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // if the transfer failed, refund the tokens
  if (!acknowledgement.success) {
    refundTokens(packet)
  }
  // if this packet was forwarded from a previously received packet
  // then we need to send the acknowledgement in reverse
  reversePacket = store.get(packetForwardPath(packet.sourcePort, packet.sourceChannel, packet.Sequence))
  if reversePacket != nil {
    FungibleTokenPacketAcknowledgement ack = unmarshal(acknowledgement)
    // prefix with forwardingPath so we know which chain it errored on
    ack.error = prefix(ack.error, reversePacket.sourcePort + "/" + reversePacket.sourceChannel)
    // remove stored forwarding info
    store.delete(packetForwardPath(packet.sourcePort, packet.sourceChannel, packet.Sequence))
    // propogate acknowledgement back to sender chain
    writeAcknowledgement(packet, acknowledgement)
  }
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: Packet) {
  // the packet timed-out, so refund the tokens
  refundTokens(packet)
  // if this packet was forwarded from a previously received packet
  // then we need to send notification of timeout in reverse
  reversePacket = store.get(packetForwardPath(packet.sourcePort, packet.sourceChannel, packet.Sequence))
  if reversePacket != nil {
    ack = FungibleTokenPacketAcknowledgement{false, "forwarding packet timed out"}
    // prefix with forwardingPath so we know which chain it timed out on
    ack.error = prefix(ack.error, reversePacket.sourcePort + "/" + reversePacket.sourceChannel)
    // remove stored forwarding info
    store.delete(packetForwardPath(packet.sourcePort, packet.sourceChannel, packet.Sequence))
    // propogate timeout acknowledgement back to sender chain
    writeAcknowledgement(packet, ack)
  }
}
```

`refundTokens` is called by both `onAcknowledgePacket`, on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original sender.

```typescript
function refundTokens(packet: Packet) {
  FungibleTokenPacketData data = packet.data
  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  // we are the source if the denomination is not prefixed
  source = data.denom.slice(0, len(prefix)) !== prefix
  if source {
    // sender was source chain, unescrow tokens back to sender
    escrowAccount = channelEscrowAddresses[packet.srcChannel]
    bank.TransferCoins(escrowAccount, data.sender, data.denom, data.amount)
  } else {
    // receiver was source chain, mint vouchers back to sender
    bank.MintCoins(data.sender, data.denom, data.amount)
  }
}
```

```typescript
function onTimeoutPacketClose(packet: Packet) {
  // can't happen, only unordered channels allowed
}
```

#### Using the Memo Field

Note: Since earlier versions of this specification did not include a `memo` field, implementations must ensure that the new packet data is still compatible with chains that expect the old packet data. A legacy implementation MUST be able to unmarshal a new packet data with an empty string memo into the legacy `FungibleTokenPacketData` struct. Similarly, an implementation supporting `memo` must be able to unmarshal a legacy packet data into the current struct with the `memo` field set to the empty string.

The `memo` field is not used within transfer, however it may be used either for external off-chain users (i.e. exchanges) or for middleware wrapping transfer that can parse and execute custom logic on the basis of the passed in memo. If the memo is intended to be parsed and interpreted by higher-level middleware, then these middleware are advised to namespace their additions to the memo string so that they do not overwrite each other. Chains should ensure that there is some length limit on the entire packet data to ensure that the packet does not become a DOS vector. However, these do not need to be protocol-defined limits. If the receiver cannot accept a packet because of length limitations, this will lead to a timeout on the sender side.

Memos that are intended to be read by higher level middleware for custom execution must be structured so that different middleware can read relevant data in the memo intended for them without interfering with data intended for other middlewares.

Thus, for any memo that is meant to be interpreted by the state machine; it is recommended that the memo is a JSON object with each middleware reserving a key that it can read into and retrieve relevant data. This way the memo can be constructed to pass in information such that multiple middleware can read the memo without interference from each other.

Example:

```json
{
  "wasm": {
    "address": "contractAddress",
    "arguments": "marshalledArguments",
  },
  "callback": "contractAddress",
  "router": "routerArgs",
}
```

Here, the "wasm", "callback", and "router" fields are all intended for separate middlewares that will exclusively read those fields respectively in order to execute their logic. This allows multiple modules to read from the memo. Middleware should take care to reserve a unique key so that they do not accidentally read data intended for a different module. This issue can be avoided by some off-chain registry of keys already in-use in the JSON object.

#### Reasoning

##### Correctness

This implementation preserves both fungibility & supply.

Fungibility: If tokens have been sent to the counterparty chain, they can be redeemed back in the same denomination & amount on the source chain.

Supply: Redefine supply as unlocked tokens. All send-recv pairs sum to net zero. Source chain can change supply.

##### Multi-chain notes

This specification does not directly handle the "diamond problem", where a user sends a token originating on chain A to chain B, then to chain D, and wants to return it through D -> C -> A — since the supply is tracked as owned by chain B (and the denomination will be "{portOnD}/{channelOnD}/{portOnB}/{channelOnB}/denom"), chain C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central chains in the network topology.

In order to track all of the denominations moving around the network of chains in various paths, it may be helpful for a particular chain to implement a registry which will track the "global" source chain for each denomination. End-user service providers (such as wallet authors) may want to integrate such a registry or keep their own mapping of canonical source chains and human-readable names in order to improve UX.

#### Optional addenda

- Each chain, locally, could elect to keep a lookup table to use short, user-friendly local denominations in state which are translated to and from the longer denominations when sending and receiving packets. 
- Additional restrictions may be imposed on which other machines may be connected to & which channels may be established.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

This initial standard uses version "ics20-1" in the channel handshake.

A future version of this standard could use a different version in the channel handshake,
and safely alter the packet data format & packet handler semantics.

## Example Implementations

- Implementation of ICS 20 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).
- Implementation of ICS 20 in Rust can be found in [ibc-rs repository](https://github.com/cosmos/ibc-rs).

## History

Jul 15, 2019 - Draft written

Jul 29, 2019 - Major revisions; cleanup

Aug 25, 2019 - Major revisions, more cleanup

Feb 3, 2020 - Revisions to handle acknowledgements of success & failure

Feb 24, 2020 - Revisions to infer source field, inclusion of version string

July 27, 2020 - Re-addition of source field

Nov 11, 2022 - Addition of a memo field

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
