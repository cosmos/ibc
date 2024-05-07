---
ics: 20
title: Fungible Token Transfer
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
version compatibility: ibc-go v7.0.0 (ics20-1 supported only)
author: Christopher Goes <cwgoes@interchain.berlin>
created: 2019-07-15 
modified: 2024-03-05
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

Only one packet data type is required: `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, and receiving account or `FungibleTokenPacketDataV2` which specifies multiple tokens being sent between sender and receiver along with a forwarding path that can forward tokens further beyond the initial receiving chain. A v2 supporting chain can optionally convert a v1 packet for channels that are still on version 1.

```typescript
interface FungibleTokenPacketData {
  denom: string
  amount: uint256
  sender: string
  receiver: string
  memo: string
}

interface FungibleTokenPacketDataV2 {
  tokens: []Token
  sender: string
  receiver: string
  memo: string
  forwardingPath: ForwardingInfo // a list of forwarding info determining where the tokens must be forwarded next
}

interface ForwardingInfo {
  hops: []Hop
  memo: string,
}

interface Hop {
  portID: string,
  channelId: string,
}

interface Token {
  denom: string // base denomination
  trace: []string
  amount: uint256
}
```

As tokens are sent across chains using the ICS 20 protocol, they begin to accrue a record of channels for which they have been transferred across. This information is encoded into the `trace` field in the token. 

The ICS 20 token traces are represented by a list of the form `{ics20Port}/{ics20Channel}`, where `ics20Port` and `ics20Channel` are an ICS 20 port and channel on the current chain for which the funds exist. The port and channel pair indicate which channel the funds were previously sent through. Implementations are responsible for correctly parsing the IBC trace information and encoding it into the final on-chain denomination so that the same base denominations sent through different paths are not treated as being fungible.

A sending chain may be acting as a source or sink zone. When a chain is sending tokens across a port and channel which are not equal to the last prefixed port and channel pair, it is acting as a source zone. When tokens are sent from a source zone, the destination port and channel will be prepended to the trace (once the tokens are received) adding another hop to a tokens record. When a chain is sending tokens across a port and channel which are equal to the last prefixed port and channel pair, it is acting as a sink zone. When tokens are sent from a sink zone, the first element of the trace, which was the last port and channel pair added to the trace is removed (once the tokens are received), undoing the last hop in the tokens record. A more complete explanation is [present in the ibc-go implementation](https://github.com/cosmos/ibc-go/blob/457095517b7832c42ecf13571fee1e550fec02d0/modules/apps/transfer/keeper/relay.go#L18-L49).

The following sequence diagram exemplifies the multi-chain token transfer dynamics. This process encapsulates the intricate steps involved in transferring tokens in a cycle that begins and ends on the same chain, traversing through Chain A, Chain B, and Chain C. The order of operations is meticulously outlined as `A -> B -> C -> A -> C -> B -> A`.

The forwarding path in the `v2` packet tells the receiving chain where to send the tokens to next. This must be constructed as a list of portID/channelID pairs with each element concatenated as `portID/channelID`. This allows users to automatically route tokens through the interchain. A common usecase might be to unwind the trace of the tokens back to the original source chain before sending it forward to the final intended destination.

Here are examples of the transfer packet data:

```typescript

// V1 example of transfer packet data
FungibleTokenPacketData {
  denom: "transfer/channel-1/transfer/channel-4/uatom",
  amount: 500,
  sender: cosmosexampleaddr1,
  receiver: cosmosexampleaddr2,
  memo: "exampleMemo",
}

// V2 example of transfer packet data
FungibleTokenPacketDataV2 {
  tokens: [
    Token{
      denom: uatom,
      amount: 500,
      trace: ["transfer/channel-1", "transfer/channel-4"],
    },
    Token{
      denom: btc,
      amount: 7,
      trace: ["transfer/channel-3"],
    }
  ],
  sender: cosmosexampleaddr1,
  receiver: cosmosexampleaddr2,
  memo: "",
  forwardingPath: {[{"transfer", "channel-7"}, {"transfer", "channel-13"}], , "swap: {...}"}, // provide hops in order and the memo intended for final hop
}
```

![Transfer Example](source-and-sink-zones.png)

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

The fungible token transfer bridge module tracks escrow addresses associated with particular channels in state. Fields of the `ModuleState` are assumed to be in scope.

```typescript
interface ModuleState {
  channelEscrowAddresses: Map<Identifier, string>
  channelForwardingAddresses: Map<Identifier, string>
}
```

### Sub-protocols

The sub-protocols described herein should be implemented in a "fungible token transfer bridge" module with access to a bank module and to the IBC routing module.

#### Port & channel setup

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialised) to bind to the appropriate port and create an escrow address (owned by the module).

```typescript
function setup() {
  capability = routingModule.bindPort("transfer", ModuleCallbacks{
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
- The version string is `ics20-1` or `ics20-2`.

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
  // assert that version is "ics20-1" or "ics20-2" or empty
  // if empty, we return the default transfer version to core IBC
  // as the version for this channel
  abortTransactionUnless(version === "ics20-2" || version === "ics20-1" || version === "")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress(portIdentifier, channelIdentifier)
  if version == "" {
    // default to latest supported version
    return "ics20-2", nil
  }
  // If the version is not empty and is among those supported, we return the version
  return version, nil 
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
  // assert that version is "ics20-1" or "ics20-2" 
  abortTransactionUnless(counterpartyVersion === "ics20-1" || counterpartyVersion === "ics20-2")
  // allocate an escrow address
  channelEscrowAddresses[channelIdentifier] = newAddress(portIdentifier, channelIdentifier)
  // return the same version as counterparty version so long as we support it
  return counterpartyVersion, nil
}
```

```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  counterpartyVersion: string) {
  // port has already been validated
  // assert that counterparty selected version is the same as our version
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  abortTransactionUnless(counterpartyVersion === channel.version)
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

Note: `constructOnChainDenom` is a helper function that will construct the local on-chain denomination for the bridged token. It **must** encode the trace and base denomination to ensure that tokens coming over different paths are not treated as fungible. The original trace and denomination must be retrievable by the state machine so that they can be passed in their original forms when constructing a new IBC path for the bridged token. The ibc-go implementation handles this by creating a local denomination: `hash(trace+base_denom)`.

`sendFungibleTokens` must be called by a transaction handler in the module which performs appropriate signature checks, specific to the account owner on the host state machine.

```typescript
function sendFungibleTokens(
  tokens: []Token,
  sender: string,
  receiver: string,
  memo: string,
  forwardingPath: ForwardingInfo,
  sourcePort: string,
  sourceChannel: string,
  timeoutHeight: Height,
  timeoutTimestamp: uint64, // in unix nanoseconds
): uint64 {
  // memo and forwardingPath cannot both be non-empty
  abortTransactionUnless(memo != "" && forwardingPath != nil)
  for token in tokens {
    prefix = "{sourcePort}/{sourceChannel}/"
    // we are the source if the denomination is not prefixed
    source = token.trace[0] != prefix
    onChainDenom = constructOnChainDenom(token.trace, token.denom)
      
    if source {
      // determine escrow account
      escrowAccount = channelEscrowAddresses[sourceChannel]
      // escrow source tokens (assumed to fail if balance insufficient)
      bank.TransferCoins(sender, escrowAccount, onChainDenom, token.amount)
    } else {
      // receiver is source chain, burn vouchers
      bank.BurnCoins(sender, onChainDenom, token.amount)
    }
  }

  channel = provableStore.get(channelPath(sourcePort, sourceChannel))
  // getAppVersion returns the transfer version that is embedded in the channel version
  // as the channel version may contain additional app or middleware version(s)
  transferVersion = getAppVersion(channel.version)
  if transferVersion == "ics20-1" {
    abortTransactionUnless(len(tokens) == 1)
    token = tokens[0]
    // abort if forwardingPath defined
    abortTransactionUnless(forwardingPath == nil)
    // create v1 denom of the form: port1/channel1/port2/channel2/port3/channel3/denom
    v1Denom = constructOnChainDenom(token.trace, token.denom)
    // v1 packet data does not support forwardingPath fields
    data = FungibleTokenPacketData{v1Denom, token.amount, sender, receiver, memo}
  } else if transferVersion == "ics20-2" {
    // create FungibleTokenPacket data
    data = FungibleTokenPacketDataV2{tokens, sender, receiver, memo, forwardingPath}
  } else {
    // should never be reached as transfer version must be negotiated to be either
    // ics20-1 or ics20-2 during channel handshake
    abortTransactionUnless(false)
  }

  // send packet using the interface defined in ICS4
  sequence = handler.sendPacket(
    getCapability("port"),
    sourcePort,
    sourceChannel,
    timeoutHeight,
    timeoutTimestamp,
    MarshalJSON(data) // json-marshalled bytes of packet data
  )

  return sequence
}
```

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```typescript
function onRecvPacket(packet: Packet) {
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  // getAppVersion returns the transfer version that is embedded in the channel version
  // as the channel version may contain additional app or middleware version(s)
  transferVersion = getAppVersion(channel.version)
  var tokens []Token
  var receiver string // address to send tokens to on this chain
  var finalReceiver string // final intended address in forwarding case
  if transferVersion == "ics20-1" {
     FungibleTokenPacketData data = UnmarshalJSON(packet.data)
     trace, denom = parseICS20V1Denom(data.denom)
     token = Token{
       denom: denom
       trace: trace
       amount: packet.amount
     }
     tokens = []Token{token}
     receiver = data.receiver
  } else if transferVersion == "ics20-2" {
    FungibleTokenPacketDataV2 data = UnmarshalJSON(packet.data)
    tokens = data.tokens
    // if we need to forward the tokens onward
    // overwrite the receiver to temporarily send to the channel escrow address of the intended receiver
    if len(forwardingPath.hops) > 0 {
      // memo must be empty
      abortTransactionUnless(memo == "")
      if channelForwardingAddress[packet.destinationChannel] == "" {
        channelForwardingAddress[packet.destinationChannel] = newAddress()
      }
      receiver = channelForwardingAddresses[packet.destinationChannel]
      finalReceiver = data.receiver
    } else {
      receiver = data.receiver
    }
  } else {
    // should never be reached as transfer version must be negotiated to be either
    // ics20-1 or ics20-2 during channel handshake
    abortTransactionUnless(false)
  }

  assert(packet.sender !== "")
  assert(receiver !== "")
    
  // construct default acknowledgement of success
  FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, null}

  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  receivedTokens = []Token
  for token in tokens {
    assert(token.denom !== "")
    assert(token.amount > 0)
    
    // we are the source if the packets were prefixed by the sending chain
    source = token.trace[0] == prefix
    var onChainTrace []string
    if source {
      // since we are receiving back to source we remove the prefix from the trace
      onChainTrace = token.trace[1:]
      onChainDenom = constructOnChainDenom(onChainTrace, token.denom)
      // receiver is source chain: unescrow tokens
      // determine escrow account
      escrowAccount = channelEscrowAddresses[packet.destChannel]
      // unescrow tokens to receiver (assumed to fail if balance insufficient)
      err = bank.TransferCoins(escrowAccount, receiver, onChainDenom, token.amount)
      if (err != nil) {
        ack = FungibleTokenPacketAcknowledgement{false, "transfer coins failed"}
        // break out of for loop on first error
        break
      }
    } else {
      // since we are receiving to a new sink zone we prepend the prefix to the trace
      prefix = "{packet.destPort}/{packet.destChannel}/"
      onChainTrace = append([]string{prefix}, token.trace...)
      onChainDenom = constructOnChainDenom(onChainTrace, token.denom)
      // sender was source, mint vouchers to receiver (assumed to fail if balance insufficient)
      err = bank.MintCoins(receiver, onChainDenom, token.amount)
      if (err !== nil) {
        ack = FungibleTokenPacketAcknowledgement{false, "mint coins failed"}
        // break out of for loop on first error
        break
      }
    }

    // add the received token to the received tokens list
    recvToken = Token{
      denom: token.denom,
      trace: onChainTrace,
      amount: token.amount,
    }
    receivedTokens = append(receivedTokens, recvToken)
  }

  // if there is an error ack return immediately and do not forward further
  if !ack.Success() {
    return ack
  }

  // if acknowledgement is successful and forwarding path set
  // then start forwarding
  if len(forwardingPath.hops) > 0 {
    //check that next channel supports token forwarding
    channel = provableStore.get(channelPath(forwardingPath.hops[0].portID, forwardingPath.hops[0].channelID))
    if channel.version != "ics20-2" && len(forwardingPath.hops) > 1 {
      ack = FungibleTokenPacketAcknowledgement(false, "next hop in path cannot support forwarding onward")
      return ack
    }
    memo = ""
    nextForwardingPath = ForwardingPath{
      hops: forwardingPath.hops[1:]
      memo: forwardingPath.memo
    }
    if forwardingPath.hops == 1 {
      // we're on the last hop, we can set memo and clear
      // the next forwardingPath
      memo = forwardingPath.memo
      nextForwardingPath = nil
    }
    // send the tokens we received above to the next port and channel
    // on the forwarding path
    // and reduce the forwardingPath by the first element
    nextPacketSequence = sendFungibleTokens(
      receivedTokens,
      receiver, // sender of next packet
      finalReceiver, // receiver of next packet
      memo,
      nextForwardingPath,
      forwardingPath.hops[0].portID,
      forwardingPath.hops[0].channelID,
      Height{},
      currentTime() + DefaultHopTimeoutPeriod,
    )
    // store packet for future sending ack
    privateStore.set(packetForwardPath(forwardingPath.hops[0].portID, forwardingPath.hops[0].channelID, nextPacketSequence), packet)
    // use async ack until we get successful acknowledgement from further down the line.
    return nil
  }

  return ack
}
```

`onAcknowledgePacket` is called by the routing module when a packet sent by this module has been acknowledged.

```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // check if the packet that was sent is from a previously forwarded packet
  prevPacket = privateStore.get(packetForwardPath(packet.sourcePort, packet.sourceChannel))

  if prevPacket != nil {
    if acknowledgement.success {
      FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{true, "forwarded packet succeeded"}
      handler.writeAcknowledgement(
        prevPacket,
        ack,
      )
    } else {
      // the forwarded packet has failed, thus the funds have been refunded to the forwarding address.
      // we must revert the changes that came from successfully receiving the tokens on our chain
      // before propogating the error acknowledgement back to original sender chain
      revertInFlightChanges(packet, prevPacket)
      // write error acknowledgement
      FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{false, "forwarded packet failed"}
      handler.writeAcknowledgement(
        prevPacket,
        ack,
      )
  } else {
    // if the transfer failed, refund the tokens
    if !(acknowledgement.success) {
      refundTokens(packet)
    }
  }
}
```

`onTimeoutPacket` is called by the routing module when a packet sent by this module has timed-out (such that it will not be received on the destination chain).

```typescript
function onTimeoutPacket(packet: Packet) {
  // check if the packet was sent is from a previously forwarded packet
  prevPacket = privateStore.get(packetForwardPath(packet.sourcePort, packet.sourceChannel))

  if prevPacket != nil {
    // the forwarded packet has failed, thus the funds have been refunded to the forwarding address.
    // we must revert the changes that came from successfully receiving the tokens on our chain
    // before propogating the error acknowledgement back to original sender chain
    revertInFlightChanges(packet, prevPacket)
    // write error acknowledgement
    FungibleTokenPacketAcknowledgement ack = FungibleTokenPacketAcknowledgement{false, "forwarded packet failed"}
    handler.writeAcknowledgement(
      prevPacket,
      ack,
    )
  } else {
    // the packet timed-out, so refund the tokens
    refundTokens(packet)
  }
}
```

##### Helper functions

```typescript
function isSource(portId: string, channelId: string, token: Token) {
  firstTrace = "{portId}/{channelId}"
  return token.trace[0] == firstTrace
}
```

`refundTokens` is called by both `onAcknowledgePacket`, on failure, and `onTimeoutPacket`, to refund escrowed tokens to the original sender.

```typescript
function refundTokens(packet: Packet) {
  channel = provableStore.get(channelPath(portIdentifier, channelIdentifier))
  // getAppVersion returns the transfer version that is embedded in the channel version
  // as the channel version may contain additional app or middleware version(s)
  transferVersion = getAppVersion(channel.version)
  if transferVersion == "ics20-1" {
     FungibleTokenPacketData data = UnmarshalJSON(packet.data)
     trace, denom = parseICS20V1Denom(data.denom)
     token = Token{
       denom: denom
       trace: trace
       amount: packet.amount
     }
     tokens = []Token{token}
  } else if transferVersion == "ics20-2" {
    FungibleTokenPacketDataV2 data = UnmarshalJSON(packet.data)
    tokens = data.tokens
  } else {
    // should never be reached as transfer version must be negotiated to be either
    // ics20-1 or ics20-2 during channel handshake
    abortTransactionUnless(false)
  }

  prefix = "{packet.sourcePort}/{packet.sourceChannel}/"
  for token in tokens {
    // we are the source if the denomination is not prefixed
    source = token.trace[0] != prefix
    onChainDenom = constructOnChainDenom(token.trace, token.denom)
    if source {
      // sender was source chain, unescrow tokens back to sender
      escrowAccount = channelEscrowAddresses[packet.sourceChannel]
      bank.TransferCoins(escrowAccount, data.sender, onChainDenom, token.amount)
    } else {
      // receiver was source chain, mint vouchers back to sender
      bank.MintCoins(data.sender, onChainDenom, token.amount)
    }
  }
}
```

```typescript
// revertInFlightChanges reverts the receive packet and send packet
// that occurs in the middle chains during a packet forwarding
// If an error occurs further down the line, the state changes
// on this chain must be reverted before sending back the error acknowledgement
// to ensure atomic packet forwarding
function revertInFlightChanges(sentPacket: Packet, receivedPacket: Packet) {
  forwardEscrow = channelEscrowAddresses[sentPacket.sourceChannel]
  reverseEscrow = channelEscrowAddresses[receivedPacket.destChannel]
  // the token on our chain is the token in the sentPacket
  for token in sentPacket.tokens {
    // check if the packet we sent out was sending as source or not
    // in this case we escrowed the outgoing tokens
    if isSource(sentPacket.sourcePort, sentPacket.sourceChannel, token) {
      // check if the packet we received was a source token for our chain
      if isSource(receivedPacket.destinationPort, receivedPacket.desinationChannel, token) {
        // receive sent tokens from the received escrow to the forward escrow account
        // so we must send the tokens back from the forward escrow to the original received escrow account
        bank.TransferCoins(forwardEscrow, reverseEscrow, token.denom, token.amount)
      } else {
        // receive minted vouchers and sent to the forward escrow account
        // so we must remove the vouchers from the forward escrow account and burn them
        bank.BurnCoins(forwardEscrow, token.denom, token.amount)
      }
    } else {
      // in this case we burned the vouchers of the outgoing packets
      // check if the packet we received was a source token for our chain
      // in this case, the tokens were unescrowed from the reverse escrow account
      if isSource(receivedPacket.destinationPort, receivedPacket.desinationChannel, token) {
        // in this case we must mint the burned vouchers and send them back to the escrow account
        bank.MintCoins(reverseEscrow, token.denom, token.amount)
      }
      // if it wasn't a source token on receive, then we simply had minted vouchers and burned them in the receive.
      // So no state changes were made, and thus no reversion is necessary
    }
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

Sep 22, 2023 - Support for multi-token packets

March 5, 2024 - Support for ics20-2

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
