---
ics: 27
title: Interchain Account
stage: Draft
category: IBC/TAO
author: Tony Yun <yunjh1994@everett.zone>, Dogemos <dogemos@dogemos.com>
created: 2019-08-01
modified: 2019-09-24
---

## **Synopsis**

This standard document specifies packet data structure, state machine handling logic, and encoding details for the account management system over an IBC channel between separate chains.

### **Motivation**

On Ethereum, there are two types of accounts: externally owned accounts, controlled by private keys, and contract accounts, controlled by their contract code [[ref](https://github.com/ethereum/wiki/wiki/White-Paper)]. Similar to Ethereum's CA(contract accounts), Interchain accounts are managed by another chain(zone) while retaining all the capabilities of a normal account (i.e. stake, send, vote, etc). While an Ethereum CA's contract logic is performed within Ethereum's EVM, Interchain accounts are managed by another chain via IBC in a trustless way.

### **Definitions**

The IBC handler interface & IBC relayer module interface are as defined in [ICS 25](https://github.com/cosmos/ics/blob/bez/15-ics-cosmos-signed-messages/spec/ics-025-handler-interface) and [ICS 26](https://github.com/cosmos/ics/blob/bez/15-ics-cosmos-signed-messages/spec/ics-026-relayer-module), respectively.

### **Desired Properties**

- Permissionless
- Fault containment: Interchain account must follow rules of its dwelling chain, even in times of byzantine behavior by the counterparty chain (the chain that manages the account)
- The chain(that controls the account) requesting a specific transaction from must process the results asynchronously and according to the chain's logic.
- Sending and receiving transactions will be processed in an ordered manner.

## **Technical Specification**

### **Data Structures**

`RegisterIBCAccountPacketData` is used for counterparty chain to register an account. Interchain account's address is defined deterministically with channel path and salt. Process of determining address is influenced by [EIP-1014](https://github.com/ethereum/EIPs/blob/master/EIPS/eip-1014.md). It allows interactions to be made with addresses that do not exist yet on-chain. These specificities will allow more offchain services to build on top of the data structure.

```typescript
interface RegisterIBCAccountPacketData {
  salt: Uint8Array
}
```

`RunTxPacketData` is used to run tx for interchain account. Tx bytes is dependent on app, and it is formed with minimal data set except data for validating signatures.
```typescript
interface RunTxPacketData {
  txBytes: Uint8Array
}
```

The interchain acccount bridge module tracks the addresses that have been registered on particular ports/channels in state. Path is a string which is {path_identifier/channel_identifier}. Fields of the ModuleState are assumed to be in scope.

```typescript
type Path = string

interface ModuleState {
    addressesRegisteredChannel: Map<string, Path>
}
```

### **Subprotocols**

The subprotocols described herein should be implemented in a "interchain-account-bridge" module with access to a router and codec (decoder or unmarshaller) for app and to the IBC relayer module.

### **Port & channel setup**

The `setup` function must be called exactly once when the module is created (perhaps when the blockchain itself is initialized) to bind to the appropriate port and create an escrow address (owned by the module).
```typescript
function setup() {
  relayerModule.bindPort("interchain-account", ModuleCallbacks{
    onChanOpenInit,
    onChanOpenTry,
    onChanOpenAck,
    onChanOpenConfirm,
    onChanCloseInit,
    onChanCloseConfirm,
    onSendPacket,
    onRecvPacket,
    onTimeoutPacket,
    onAcknowledgePacket,
    onTimeoutPacketClose
  })
}
```
Once the `setup` function has been called, channels can be created through the IBC relayer module between instances of the interchain account module on separate chains.

### **Routing module callbacks**

### **Channel lifecycle management**

Both machines `A` and `B` accept new channels from any module on another machine, if and only if:

- The other module is bound to the "interchain account" port.
- The channel being created is ordered.
- The version string is empty.
```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // only allow channels to "interchain-account" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "interchain-account")
  // version not used at present
  abortTransactionUnless(version === "")
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
  version: string,
  counterpartyVersion: string) {
  // only ordered channels allowed
  abortTransactionUnless(order === ORDERED)
  // version not used at present
  abortTransactionUnless(version === "")
  abortTransactionUnless(counterpartyVersion === "")
  // only allow channels to "interchain-account" port on counterparty chain
  abortTransactionUnless(counterpartyPortIdentifier === "interchain-account")
}
```
```typescript
function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
  // version not used at present
  abortTransactionUnless(version === "")
  // port has already been validated
}
```
```typescript
function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // accept channel confirmations, port has already been validated
}
```
```typescript
function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```
```typescript
function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
  // no action necessary
}
```
### **Packet relay**

In plain English, between chains `A` and `B`. It will describe only the case that chain A wants to register an Interchain account on chain B and control it. Moreover, this system can also be applied the other way around. In these set of examples, we follow Cosmos-SDK's modularity rule and its `msg` system.
```typescript
interface ChainAccountTx {
  msgs: Msg[]
}
```
```typescript    
function onSendPacket(packet: Packet) {  
  // Sending chain can do preliminary work (i.e. create a log of transactions sent) if needed.
}
```
```typescript
function onRecvPacket(packet: Packet): bytes {
  if (packet.data is RunTxPacketData) {
    ChainAccountTx data = {}
    // Decode transaction request
    codec.unmarshal(packet.data.txBytes, &data)
      
    signers = []
    for (const msg of data.msgs) {
      signers.push(msg.getSigners())  
    }
      
    path = "{packet.sourcePort}/{packet.sourceChannel}"
      
    // Check tx has right permissions
    for (const signer of signers){
      abortTransactionUnless(path === addressesRegisteredChannel[signer])
    }
    
    // Get handler from router
    handler = router(msg.route())
    abortTransactionUnless(handler != null)
        
    // Execute handler with msg
    result = handler(msg)
        
    if (result.code === 0) {
        // Return acknowledgement byte as 0x0 if tx succeeds.
        return 0x0
    } else {
        // Return acknowledgement byte as error code if tx fails.
        return binary.littleEndian.encode(result.code)
    }
  }
    
  if (packet.data is RegisterIBCAccountPacketData) {
    RegisterIBCAccountPacketData data = packet.data
      
    path = "{packet.sourcePort}/{packet.sourceChannel}"
    address = sha256(path + packet.salt)
      
    // Should not block even if there is normal account,
    // because attackers can distrupt to create an ibc managed account
    // by sending some assets to estimated address in advance.
    // And IBC managed account has no public key, but its sequence is 1.
    // It can be mark for Interchain account, becuase normal account can't be sequence 1 without publish public key.
    account = accountKeeper.getAccount(account)
    if (account != null) {
      abortTransactionUnless(account.sequence === 1 && account.pubKey == null)
    } else {
      accountKeeper.newAccount(address)
    }
      
    addressesRegisteredChannel[signer] = path
        
    // set account's sequence to 1
    accountKeeper.setAccount(address, {
      ...,
      sequence: 1,
      ...,
    })
      
    // Return acknowledgement byte as 0x0 if account creation succeeds.
    return 0x0
  } 
    
  return 0x
}
```
```typescript
function onAcknowledgePacket(
  packet: Packet,
  acknowledgement: bytes) {
  // Receiving chain should handle this event as if the tx in packet has failed if ack bytes is not 0x0.
}
```
```typescript
function onTimeoutPacket(packet: Packet) {
  // Receiving chain should handle this event as if the tx in packet has failed
}
```
```typescript
function onTimeoutPacketClose(packet: Packet) {
  // nothing is necessary
}
```

## **Backwards Compatibility**

(discussion of compatibility or lack thereof with previous standards)

## **Forwards Compatibility**

(discussion of compatibility or lack thereof with expected future standards)

## **Example Implementation**

(link to or description of concrete example implementation)

## **Other Implementations**

(links to or descriptions of other implementations)

## **History**

(changelog and notable inspirations / references)

## **Copyright**

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).