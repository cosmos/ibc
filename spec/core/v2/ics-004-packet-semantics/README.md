---
ics: 4
title: Packet Semantics
stage: draft
category: IBC/TAO
kind: instantiation
requires: 2, 24, packet-data 
version compatibility: ibc-go v10.0.0
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

TODO : 

- Resolve Need discussion
- Improve Sketchs  
- Improve conditions set / think about more condition an proper presentation 
- Review Ack and Timeout carefully 
- FROM Race condition UP to END 

## Synopsis 

This standard defines the channel and packet semantics necessary for state machines implementing the Inter-Blockchain Communication (IBC) protocol  version 2 to enable secure, verifiable, and efficient cross-chain messaging.

It specifies the mechanisms to create channels and register them between two distinct state machines (blockchains) where a channel serves as a semantic link between the chains and their counterparty light client representation, ensuring that both chains can process and verify packets exchanged between them.

The standard then details the processes for transmitting, receiving, acknowledging, and timing out data packets. The packet-flow semantics guarantee exactly-once packet delivery between chains, utilizing on-chain light clients for state verification and providing efficient routing of packet data to specific IBC applications.

### Motivation

The motivation for this specification is to formalize the semantics for both packet handling and channel creation and registration in the IBC version 2 protocol. These are fundamental components for enabling reliable, secure, and verifiable communication between independent blockchains. The document focuses on the mechanisms to establish the root of trust for starting the communication between two distinct chains, routing, verification, and application-level delivery guarantees required by the IBC protocol. 

This specification focuses on defining the mechanisms for creating channels, securely registering them between chains, and ensuring that packets sent across these channels are processed consistently and verifiably. By utilizing on-chain light clients for state verification, it enables chains to exchange data without requiring synchronous communication, ensuring that all packets are delivered exactly once, even in the presence of network delays or reordering.

To standardize both channel creation and packet flow semantics, this document also defines the pre-conditions, error conditions, and post-conditions for each defined function handler. By using a well-defined packet interface and clear handling processes, ICS-04 aims to ensure consistency and security across distinct implementations of the protocol ensuring reliability and security and tries not to impose constraints on the internal workings of the state machines.  

### Definitions

`get`, `set`, `delete`, and module-system related primitives are as defined in [ICS 24](../ics-024-host-requirements).

A `channel` is a data structure that facilitates exactly-once packet delivery between two blockchains acting as a communication pipeline between specific modules registered on separate chains, allowing for secure, verifiable transmission of packets. Channels can be created and registered to establish a semantic link between two chains and their respective light clients, ensuring that both chains can process and verify the packets exchanged. To establish the root of trust for secure interchain communication with a counterparty chain, each chain MUST register a channel maintaining the necessary counterparty information, such as the channel identifier of the counterparty chain, the light client identifier of the counterparty chain and the path used to store packet flow messages.

```typescript
interface Channel {
    clientId: bytes // local light client id of the counterparty chain. 
    counterpartyChannelId: bytes // counterparty channel id.  
    keyPrefix: CommitmentPrefix //  key path that the counterparty will use to prove its store packet flow messages.
}
```

The `Packet`, `Payload`, `Encoding` and the `Acknowledgement` interfaces are as defined in [packet specification](https://github.com/cosmos/ibc/blob/c7b2e6d5184b5310843719b428923e0c5ee5a026/spec/core/v2/ics-004-packet-semantics/PACKET.md). 

For convenience, following we recall their structures.  

A `Packet`, in the interblockchain communication protocol, is a particular interface defined as follows:

```typescript
interface Packet {
    sourceChannelId: bytes, // channel identifier on the source chain. 
    destChannelId: bytes, // channel identifier on the dest chain.
    sequence: uint64, // number that corresponds to the order of sent packets.
    timeout: uint64, // indicates the UNIX timestamp in seconds and is encoded in LittleEndian. It must be passed on the destination chain and once elapsed, will no longer allow the packet processing, and will instead generate a time-out.
    data: [Payload] // data 
}
```

The `Payload` is a particular interface defined as follows:

```typescript
interface Payload {
    sourcePort: bytes, // identifies the source application port
    destPort: bytes, // identifies the dest application port 
    version: string, // application version
    encoding: Encoding, // used encoding - allows the specification of custom data encoding among those agreed in the `Encoding` enum
    appData: bytes, // app specific data 
}

enum Encoding {
  NO_ENCODING_SPECIFIED,
    PROTO_3,
    JSON,
    RLP,
    BCS,
}
```

Note that a `Packet` is never directly serialised. Rather it is an intermediary structure used in certain function calls that may need to be created or processed by modules interacting with the IBC handler.

When the array of payloads, passed-in the packet, is populated with multiple values, the system will handle the packet as a multi-data packet. The multi-data packet handling logic is out of the scope of the current version of this spec. 

An `OpaquePacket` is a packet, but cloaked in an obscuring data type by the host state machine, such that a module cannot act upon it other than to pass it to the IBC handler. The IBC handler can cast a `Packet` to an `OpaquePacket` and vice versa.

```typescript
type OpaquePacket = object
```

The protocol introduces standardized packet receipts that will serve as sentinel values for the receiving chain to explicitly write to its store the outcome of a `receivePacket`.

```typescript
enum PacketReceipt {
  SUCCESSFUL_RECEIPT = byte{0x01},
}
```

The `Acknowledgement` is a particular interface defined as follows: 

```typescript
interface Acknowledgement {
    appAcknowledgement: [bytes] // array of bytes. Each element of the array contains an acknowledgement from a specific application  
}
```

// NEED DISCUSSION, Can we delete the SENTINEL_ACKNOWLEDGMENT?

An application may not need to return an acknowledgment with after processing relevant data. In this case, it is advised to return a sentinel acknowledgement value `SENTINEL_ACKNOWLEDGMENT`, which will be the single byte in the byte array: `bytes(0x01)`. 

When the receiver chain returns this `SENTINEL_ACKNOWLEDGMENT` it allows the sender chain to still call the `acknowledgePacket` handler, e.g. to delete the packet commitment, without triggering the `onAcknowledgePacket` callback.   

> **Example**: In the multi-data packet world, if a packet within 3 payloads intended for 3 different application is sent out, the expectation is that each payload is processed in the same order in which it was placed in the packet. Similarly, the `appAcknowledgement` array is expected to be populated within the same order. 

- The `IBCRouter` contains a mapping from the application `portId` and the supported callbacks and as well as a mapping from `channelId` to the underlying client.

```typescript
type IBCRouter struct {
    callbacks: (portId,version) -> [Callback] // The double key (portId,version) clearly separate the appVxCallbacks from appVyCallbacks simplifying the routing to app process   
    clients: clientId -> Client // The IBCRouter stores the client under the clientId key
}
```

The registration of the application callbacks in the local `IBCRouter`, is responsibility of the chain modules. 
The registration of the client in the local `IBCRouter` is responsibility of the ICS-02 initialise client procedure. 

> **Note** The proper configuration of the IBCRouter is a prerequisite for starting the stream of packets.  

- The `MAX_TIMEOUT_DELTA` is intendend as the max, absolute, difference between currentTimestamp and timeoutTimestamp that can be given in input to `sendPacket`. 

```typescript
const MAX_TIMEOUT_DELTA = Implementation specific  // We recommend MAX_TIMEOUT_DELTA = TDB 
```

Additionally the ICS-04 specification defines a set of conditions that the implementations of the IBC protocol version 2 MUST adhere to. These conditions ensure the proper execution of the function handlers by establishing requirements before execution `pre-conditions`, the conditions that MUST trigger errors during execution `error-conditions`, expected outcomes after succesful execution `post-conditions-on-success`, and expected outcomes after error execution `post-conditions-on-error`.

### Desired Properties

#### Efficiency

- The speed of packet transmission and confirmation should be limited only by the speed of the underlying chains.
- Proofs should be batchable where possible.

#### Exactly-once delivery

- IBC packets sent on one end of a channel should be delivered exactly once to the other end.
- No network synchrony assumptions should be required for exactly-once safety. If one or both of the chains halt, packets may be delivered no more than once, and once the chains resume packets should be able to flow again.

#### Ordering

- IBC version 2 supports only *unordered* communications, thus, packets may be sent and received in any order. Unordered packets, have individual timeouts specified in seconds UNIX timestamp.

#### Permissioning

- Channels should be permissioned to the application registered on the local router. Thus only the modules registered on the local router, and so associated with the channel, should be able to send or receive on it.

#### Fungibility conservation 

> **Example**: An application may wish to allow a single tokenized asset to be transferred between and held on multiple blockchains while preserving fungibility and conservation of supply. The application can mint asset vouchers on chain `B` when a particular IBC packet is committed to chain `B`, and require outgoing sends of that packet on chain `A` to escrow an equal amount of the asset on chain `A` until the vouchers are later redeemed back to chain `A` with an IBC packet in the reverse direction. This ordering guarantee along with correct application logic can ensure that total supply is preserved across both chains and that any vouchers minted on chain `B` can later be redeemed back to chain `A`.

## Technical Specification

### Preliminaries

#### Store paths

The ICS-04 use the protocol paths, defined in [ICS-24](../ics-024-host-requirements/README.md), `packetCommitmentPath`, `packetRecepitPath` and `packetAcknowledgementPath`. The paths MUST be used as the referece locations in the provableStore to prove respectilvey the packet commitment, the receipt and the acknowledgment to the counterparty chain. 

Thus, Constant-size commitments to packet data fields are stored under the packet sequence number:

// NEED DISCUSSION -- we could use "commitments/{sourceId}/{sequence}" or "0x01/{sourceId}/{sequence}". For now we keep going with more or less standard paths 

```typescript
function packetCommitmentPath(channelSourceId: bytes, sequence: BigEndianUint64): Path {
    return "commitments/channels/{channelSourceId}/sequences/{sequence}"
}
```

Absence of the path in the store is equivalent to a zero-bit.

Packet receipt data are stored under the `packetReceiptPath`. In the case of a successful receive, the destination chain writes a sentinel success value of `SUCCESSFUL_RECEIPT`. 

```typescript
function packetReceiptPath(channelDestId: bytes, sequence: BigEndianUint64): Path {
    return "receipts/channels/{channelDestId}/sequences/{sequence}"
}
```

Packet acknowledgement data are stored under the `packetAcknowledgementPath`:

```typescript
function packetAcknowledgementPath(channelSourceId: bytes, sequence: BigEndianUint64): Path {
    return "acks/channels/{channelSourceId}/sequences/{sequence}"
}
```

#### Private Utility Store

Additionally, the ICS-04 defines the following variables:  `nextSequenceSend` , `channelPath` and `channelCreator`. These variables are defined for the IBC handler and meant to be used locally in the chain, thus, as long as they maintain the semantic value defined with the IBC protocol, the specification of their structure can be arbitrary changed by implementors at their conveinience.  

- The `nextSequenceSend`  tracks the sequence number for the next packet to be sent for a given source channelId.
- The `channelCreator` tracks the channels creator address given the channelId.
- The `storedChannels` tracks the channels paired with the other chains.

```typescript
type nextSequenceSend : channelId -> BigEndianUint64 
type channelCreator : channelId -> address 
type storedChannels : channelId -> Channel

function getChannel(channelId: bytes): Channel {
    return storedChannels[channelId]
}
```

### Sub-protocols

#### Setup

To start the secure packet stream between the chains, chain `A` and chain `B` MUST execute the setup following this set of procedures:  

| **Procedure**               | **Responsible**     | **Outcome**                                                                |
|-----------------------------|---------------------|-----------------------------------------------------------------------------|
| **Channel Creation**         | Relayer   | A channel is created and linked to an underlying light client on both chains.           |
| **Channel Registration**     | Relayer             | Registers the `counterpartyChannelId` on both chains, linking the channels.  |

> **Note** The relayer is required to execute `createClient` (as defined in ICS-02) before calling `createChannel`, since the `clientId` input parameter MUST be known. The `createClient` message (as defined in ICS-02) may be bundled with the `createChannel` message in a single multiMsgTx. The setup procedure is a prerequisite for starting the packet stream. If any of the steps has been missed, this would result in an incorrect setup error during the packet handlers execution. 

Below we provide the setup sequence diagram. 

```mermaid
---
title: Setup with `createClient` and `createChannel` bundle together.  
---
sequenceDiagram  
    Participant Chain A
    Participant Relayer 
    Participant Chain B
    Relayer ->> Chain A : createClient(B chain) + createChannel
    Chain A ->> Relayer : clientId= X , channelId = Y
    Relayer ->> Chain B : createClient(A chain) + createChannel
    Chain B ->> Relayer : clientId= Z , channelId = W
    Relayer ->> Chain A : registerChannel(channelId = Y, counterpartyChannelId = W)
    Relayer ->> Chain B : registerChannel(channelId = W, counterpartyChannelId = Y) 
```

```mermaid
---
title: Setup with light client previously created.  
---
sequenceDiagram
    Participant B LightClient - clientId=x  
    Participant Chain A
    Participant Relayer 
    Participant Chain B
    Participant A LightClient - clientId=z   
    Relayer ->> Chain A : createChannel(B LightClient)
    Chain A ->> Relayer : clientId= x , channelId = y
    Relayer ->> Chain B : createChannel ()
    Chain B ->> Relayer : clientId= z , channelId = w
    Relayer ->> Chain A : registerChannel(channelId = y, counterpartyChannelId = w)
    Relayer ->> Chain B : registerChannel(channelId = w, counterpartyChannelId = y) 
```

Once the set up is executed the system should be in a similar state: 

![Setup Final State](setup_final_state.png)

While the client creation is defined by [ICS-2](../ics-002-client-semantics/README.md), the channel creation and registration procedures are defined by ICS-04 and are detailed below.

##### Channel creation 

The channel creation process enables the creation of the two channel ends that can be linked to establishes the communication pathway between two chains. 

###### Conditions Table  

| **Condition Type**            | **Description**  | **Code Checks** | 
|-------------------------------|------------------| ----------------|
| **pre-conditions**            | - The used clientId exist. `createClient` has been called at least once.| |
| **error-conditions**           | - Incorrect clientId.<br> - Unexpected keyPrefix format.<br> - Invalid channelId .<br> | - `client==null`.<br> - `isFormatOk(counterpartyKeyPrefix)==False`.<br> - `validatedChannelId(channelId)==False`.<br> - `getChannel(channelId)!=null`.<br>  |
| **post-conditions (success)**  | - A channel is set in store and it's accessible with key channelId.<br> - The creator is set in store and it's accessible with key channelId.<br> - nextSequenceSend is initialized.<br> - client is stored in the router.<br> - an event with relevant fields is emitted | - `storedChannel[channelId]!=null`.<br> - `channelCreator[channelId]!=null`.<br> - `router[channelId]!=null`.<br> - `nextSequenceSend[channelId]==1` |
| **post-conditions (error)**    | - None of the post-conditions (success) is true.<br>| - `storedChannel[channelId]==null`.<br> - `channelCreator[channelId]==null`.<br> - `router[channelId]==null`.<br> - `nextSequenceSend[channelId]!=1`|

###### Pseudo-Code 

```typescript
function createChannel(
    clientId: bytes,  
    counterpartyKeyPrefix: CommitmentPrefix): bytes {

        // Implementation-Specific Input Validation 
        // All implementations MUST ensure the inputs value are properly validated and compliant with this specification 
        client=getClient(clientId)
        assert(client!==null)
        assert(isFormatOk(counterpartyKeyPrefix))

        // Channel Checks
        channelId = generateIdentifier() 
        abortTransactionUnless(validateChannelIdentifier(channelId))
        abortTransactionUnless(getChannel(channelId)) === null)
        
        // Channel manipulation
        channel = Channel{
            clientId: clientId,
            counterpartyChannelId: "",  // This field it must be a blank field during the creation as it may be not known at the creation time. 
            keyPrefix: counterpartyKeyPrefix
        }

        // Local stores 
        // Store channel info 
        storedChannels[channelId]=channel
        // Store creator address info 
        channelCreator[channelId]=msg.signer()
        // Initialise the nextSequenceSend 
        nextSequenceSend[channelId]=1
        
        // Event Emission 
    emitLogEntry("createChannel", {
      channelId: channelId, 
      channel: channel, 
      creatorAddress: msg.signer(),
    })

    return channelId
}
```

##### Channel registration and counterparty idenfitifcation  

Each IBC chain MUST have the ability to idenfity its counterparty to ensure valid communication. While a client can prove any key/value path on the counterparty, knowing which identifier the counterparty uses when it sends messages to us is essential to prevent confusion between messages intended for different chains. 

To enable mutual and verifiable identification, IBC version 2 introduces a `registerChannel` procedure. The channel registration procedure ensures both chains have a mutually recognized channel that facilitates the packet transmission.

This process stores the `counterpartyChannelId` in the local channel structure, ensuring both chains have mirrored <channel, channel> pairs. With the correct registration, the unique clients on each side provide an authenticated stream of packet data. Social consensus outside the protocol is relied upon to ensure only valid <channel, channel> pairs are used, representing connections between the correct chains. 

###### Conditions Table 

| **Condition Type**            | **Description** | **Code Checks** |
|-------------------------------|-----------------------------------|----------------------------|
| **pre-conditions**            | - The `createChannel` has been called at least once| |
| **error-conditions**           | - Incorrect channelId.<br> - Creator authentication failed | - `validatedChannelId(channelId)==False`.<br> - `getChannel(channelId)==null`.<br> - `channelCreator[channelId]!=msg.signer()`.<br> |
| **post-conditions (success)**  | - The channel in store contains the counterpartyChannelId information and it's accessible with key channelId.<br> An event with relevant information has been emitted | - `storedChannel[channelId].counterpartyChannelId!=""`.<br> |
| **post-conditions (error)**    | - On the first call, the channel in store contains the counterpartyChannelId as an empty field.<br> | - `storedChannel[channelId].counterpartyChannelId==""` |
 
###### Pseudo-Code 

```typescript
function registerChannel(
    channelId: bytes, // local chain channel identifier
    counterpartyChannelId: bytes, // the counterparty's channel identifier
    authentication: data, // implementation-specific authentication data
) {
    // Implementation-Specific Input Validation 
    // All implementations MUST ensure the inputs value are properly validated and compliant with this specification

    // Channel Checks
    abortTransactionUnless(validatedIdentifier(channelId))
    channel=getChannel(channelId) 
    abortTransactionUnless(channel !== null)
    
    // Creator Address Checks
    abortTransactionUnless(msg.signer()===channelCreator[channelId])

    // Channel manipulation
    channel.counterpartyChannelId=counterpartyChannelId

    // Local Store
    storedChannels[channelId]=channel

    // log that a packet can be safely sent
    // Event Emission 
    emitLogEntry("registerChannel", {
      channelId: channelId, 
      channel: channel, 
      creatorAddress: msg.signer(),
    })
}
```

The protocol uses as an authentication mechanisms checking that the `registerChannel` message is sent by the same relayer that initialized the client such that the `msg.signer()==channelCreator[channelId]`. This would make the client and channel parameters completely initialized by the relayer. Thus, users must verify that the client is pointing to the correct chain and that the counterparty identifier is correct as well before using the <channel,channel> pair.

Thus, once two chains have set up clients, created channel and registered channels for each other with specific Identifiers, they can send IBC packets using the packet interface defined before and the packet handlers that the ICS-04 defines below. The packets will be addressed **directly** with the channels that have semantic link to the underlying light clients. Thus there are **no** more handshakes necessary. Instead the packet sender must be capable of providing the correct <channel,channel> pair. If the setup has been executed correctly, then the correctness and soundness properties of IBC holds and the IBC packet flow is guaranteed to succeed. If a user sends a packet with the wrong destination channel, then as we will see it will be impossible for the intended destination to correctly verify the packet, thus, the packet will simply time out.

#### Packet Flow Function Handlers 

In the IBC protocol version 2, the packet flow is managed by four key function handlers, each of which is responsible for a distinct stage in the packet lifecycle:

- `sendPacket`
- `receivePacket`
- `acknowledgePacket`
- `timeoutPacket`

Note that the execution of the four handler above described, upon a unique packet, cannot be combined in any arbitrary order. We provide the three possible example scenarios described with sequence diagrmas. 

---

Scenario execution with synchronous acknowledgement `A` to `B` - set of actions: `sendPacket` -> `receivePacket` -> `acknowledgePacket`  

```mermaid
sequenceDiagram
    participant B Light Client 
    participant Chain A    
    participant Relayer 
    participant Chain B
    participant A Light Client
    Chain A ->> Chain A : sendPacket
    Chain A --> Chain A : app execution
    Chain A --> Chain A : packetCommitment 
    Relayer ->> Chain B: relayPacket
    Chain B ->> Chain B: receivePacket
    Chain B -->> A Light Client: verifyMembership(packetCommitment)
    Chain B --> Chain B : app execution
    Chain B --> Chain B: writeAck
    Chain B --> Chain B: writePacketReceipt
    Relayer ->> Chain A: relayAck
    Chain A ->> Chain A : acknowldgePacket
    Chain A -->> B Light Client: verifyMembership(packetAck)
    Chain A --> Chain A : app execution
    Chain A --> Chain A : Delete packetCommitment 
```

---

Scenario execution with asynchronous acknowledgement `A` to `B` - set of actions: `sendPacket` -> `receivePacket` -> `acknowledgePacket`  

Note that the key difference with the synchronous scenario is that the receivePacket writes only the packetReceipt and not the acknowledgement. The acknowledgement is instead written asynchronously for effect of the application callback call to the core function `writeAcknowledgement`, call that happens after the `receivePacket` execution.  

```mermaid
sequenceDiagram
    participant B Light Client 
    participant Chain A    
    participant Relayer 
    participant Chain B
    participant A Light Client
    Chain A ->> Chain A : sendPacket
    Chain A --> Chain A : app execution
    Chain A --> Chain A : packetCommitment 
    Relayer ->> Chain B: relayPacket
    Chain B ->> Chain B: receivePacket
    Chain B -->> A Light Client: verifyMembership(packetCommitment)
    Chain B --> Chain B : app execution
    Chain B --> Chain B: writePacketReceipt
    Chain B --> Chain B : app execution - async ack processing
    Chain B --> Chain B: writeAck
    Relayer ->> Chain A: relayAck
    Chain A ->> Chain A : acknowldgePacket
    Chain A -->> B Light Client: verifyMembership(packetAck)
    Chain A --> Chain A : app execution
    Chain A --> Chain A : Delete packetCommitment 
```

---

Scenario timeout execution `A` to `B` - set of actions: `sendPacket` -> `timeoutPacket`  

```mermaid
sequenceDiagram
    participant B Light Client     
    participant Chain A
    participant Relayer 
    participant Chain B
    participant A Light Client
    Chain A ->> Chain A : sendPacket
    Chain A --> Chain A : app execution
    Chain A --> Chain A : packetCommitment 
    Chain A ->> Chain A : TimeoutPacket
    Chain A -->> B Light Client: verifyNonMembership(PacketReceipt)
    Chain A --> Chain A : app execution
    Chain A --> Chain A : Delete packetCommitment 
```

---

Given a configuration where we are sending a packet from `A` to `B` then chain `A` can call either, `sendPacket`,`acknowledgePacket` or `timeoutPacket` while chain `B` can only execute the `receivePacket` handler. 
The `acknowledgePacket` is not a valid action if `receivePacket` has not been executed. `timeoutPacket` is not a valid action if `receivePacket` occurred.

##### Sending packets

>**Note** Prerequisites: The `IBCRouter`s and the `channel`s have been properly configured on both chains.

The `sendPacket` function is called by the IBC handler when an IBC packet is submitted to the newtwork in order to send *data* in the form of an IBC packet. ∀ `Payload` included in the `packet.data`, which may refer to a different application, the application specific callbacks are retrieved from the IBC router and the `onSendPacket` is the then triggered on the specified application. The `onSendPacket` executes the application logic. Once all payloads contained in the `packet.data` have been acted upon, the packet commitment is generated and the sequence number bound to the `channelSourceId` is incremented. 

The `sendPacket` core function MUST execute the applications logic atomically triggering the `onSendPacket` callback ∀ application contained in the `packet.data` payload.

The IBC handler performs the following steps in order:

- Checks that the underlying clients is valid. 
- Checks that the timeout specified has not already passed on the destination chain
- Executes the `onSendPacket` ∀ Payload included in the packet. 
- Stores a constant-size commitment to the packet data & packet timeout
- Increments the send sequence counter associated with the channel
- Returns the sequence number of the sent packet

Note that the full packet is not stored in the state of the chain - merely a short hash-commitment to the data & timeout value. The packet data can be calculated from the transaction execution and possibly returned as log output which relayers can index.

###### Conditions Table 

| **Condition Type**            |**Description** | **Code Checks**|
|-------------------------------|--------------------------------------------------------|------------------------|
| **pre-conditions**            | - Chains `A` and `B` MUST be in a setup final state.<br> |                     |
| **Error-Conditions**           | - Incorrect setup (includes invalid client and invalid channelId).<br> - Invalid timeoutTimestamp.<br> - Unsuccessful payload execution. | - `getChannel(sourceChannelId)==null`.<br> , -`router[sourceChannelId]==null`.<br> - `timeoutTimestamp==0`.<br> - `timeoutTimestamp < currentTimestamp()`.<br> - `timeoutTimestamp > currentTimestamp() + MAX_TIMEOUT_DELTA`.<br> - `success=onSendPacket(..), success==False`.<br> |
| **Post-Conditions (Success)**  | - All the applications contained in the payload have properly terminated the `onSendPacket` callback execution.<br> - The packetCommitment has been generated and stored under the right packetCommitmentPath.<br> - The sequence number bound to sourceId MUST has been incremented by 1.<br> | An event with relevant information has been emitted | 
| **Post-Conditions (Error)**    | - If one payload fails, then all state changes happened on the successful application execution must be reverted.<br> - No packetCommitment has been generated.<br> - The sequence number bound to sourceId MUST be unchanged. | |

###### Pseudo-Code 

The ICS04 provides an example pseudo-code that enforce the above described conditions so that the following sequence of steps must occur for a packet to be sent from module *1* on machine *A* to module *2* on machine *B*, starting from scratch.
 
```typescript
function sendPacket(
    sourceChannelId: bytes, 
    timeoutTimestamp: uint64,
    payloads: []byte
    ) : BigEndianUint64 {

    // Setup checks - channel and client 
    channel = getChannel(sourceChannelId)
    assert(channel !== null)
    client = router.clients[channel.clientId]
    assert(client !== null)
    
    // timeoutTimestamp checks
    // disallow packets with a zero timeoutTimestamp
    assert(timeoutTimestamp !== 0) 
    // disallow packet with timeoutTimestamp less than currentTimestamp and timeoutTimestamp value bigger than currentTimestamp + MaxTimeoutDelta 
    assert(currentTimestamp() < timeoutTimestamp < currentTimestamp() + MAX_TIMEOUT_DELTA) 
    
    
    // retrieve sequence
    sequence = nextSequenceSend[sourecChannelId]
    // Check that the Sequence has been correctly initialized before hand. 
    abortTransactionUnless(sequence!==0) 
    
    // Executes Application logic ∀ Payload
    // Currently we support only len(payloads)==1 
    payload=payloads[0]
    cbs = router.callbacks[payload.sourcePort,payload.version]
    success = cbs.onSendPacket(sourceChannelId,channel.counterpartyChannelId,payload)
    // IMPORTANT: if the onSendPacket fails, the transaction is aborted and the potential state changes are reverted. 
    // This ensure that the post conditions on error are always respected. 
    // payload execution check  
    abortUnless(success)

    // Construct the packet
    packet = Packet {
            sourceId: sourceChannelId,
            destId: channel.counterpartyChannelId, 
            sequence: sequence,
            timeoutTimestamp: timeoutTimestamp, 
            payloads: payloads
            }

    // store packet commitment using commit function defined in [packet specification](https://github.com/cosmos/ibc/blob/c7b2e6d5184b5310843719b428923e0c5ee5a026/spec/core/v2/ics-004-packet-semantics/PACKET.md)
    commitment=commitV2Packet(packet) 
    provableStore.set(packetCommitmentPath(sourceChannelId, sequence),commitment)
    
    // increment the sequence. Thus there are monotonically increasing sequences for packet flow for a given clientId
    nextSequenceSend[sourceChannelId]=sequence+1
    
    // log that a packet can be safely sent
    // Event Emission 
    emitLogEntry("sendPacket", {
      sourceId: sourceChannelId, 
      destId: channel.counterpartyChannelId, 
      sequence: sequence,
      packet: packet,
      timeoutTimestamp: timeoutTimestamp, 
    })
    
    return sequence
}
```

##### Receiving packets

The `recvPacket` function is called by the IBC handler in order to receive an IBC packet sent on the corresponding client on the counterparty chain.

Atomically in conjunction with calling the core `receivePacket`, the modules/application referred in the `packet.data` payload MUST execute the specific application logic callaback.

The IBC handler performs the following steps in order:

- Checks that the client is valid
- Checks that the timeout timestamp is not yet passed on the receiving chain 
- Checks the inclusion proof of packet data commitment in the sender chain's state
- Sets a store path to indicate that the packet has been received
- If the flows supports synchronous acknowledgement, it writes the acknowledgement into the receiver provableStore. 

We pass the address of the `relayer` that signed and submitted the packet to enable a module to optionally provide some rewards. This provides a foundation for fee payment, but can be used for other techniques as well (like calculating a leaderboard).

###### Conditions Table 

| **Condition Type**            | **Description** | **Code Checks** |
|-------------------------------|-----------------------------------------------|-----------------------------------------------|
| **pre-conditions**            | - Chain `A` MUST have stored the packetCommitment under the keyPrefix registered in the chain `B` channelEnd.<br> - TimeoutTimestamp MUST not have elapsed yet on the receiving chain.<br> - PacketReceipt for the specific keyPrefix and sequence MUST be empty (e.g. `receivePacket` has not been called yet). | |
| **Error-Conditions**           | - Packet Errors: invalid packetCommitment, packetReceipt already exists.<br> - Invalid timeoutTimestamp.<br> - Unsuccessful payload execution. | |
| **Post-Conditions (Success)**  | - All the applications pointed in the payload have properly terminated the `onReceivePacket` callback execution.<br> - The packetReceipt has been written.<br> - The acknowledgement has been written. | |
| **Post-Conditions (Error)**    | - If one payload fails, then all state changes happened on the successful `onReceivePacket` application callback execution MUST be reverted.<br> - If timeoutTimestamp has elapsed then no state changes occurred. // NEED DISCUSSION (Is this ok? Shall we write the `timeout_sentinel_receipt`?) | |
                                                                                                                          
###### Pseudo-Code 

The ICS-04 provides an example pseudo-code that enforce the above described conditions so that the following sequence of steps SHOULD occur for a packet to be received from module *1* on machine *A* to module *2* on machine *B*.
 
```typescript
function recvPacket(
  packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string // Need discussion 
  ) {

    // Channel and Client Checks
    channel = getChannel(packet.channelDestId)     
    assert(channel !== null)
    client = router.clients[channel.clientId]  
    assert(client !== null)
    
    //assert(packet.sourceId == channel.counterpartyChannelId) This should be always true, redundant // NEED DISCUSSION 

    // verify timeout
    assert(packet.timeoutTimestamp === 0)  
    assert(currentTimestamp() < packet.timeoutTimestamp)

    // verify the packet receipt for this packet does not exist already 
    packetReceipt = provableStore.get(packetReceiptPath(packet.channelDestId, packet.sequence))
    abortUnless(packetReceipt === null)

    //////// verify commitment 
    
    // 1. retrieve keys 
    packetPath = packetCommitmentPath(packet.channelDestId, packet.sequence)
    merklePath = applyPrefix(channel.keyPrefix, packetPath)
    
    // 2. reconstruct commit value based on the passed-in packet  
    commit = commitV2Packet(packet) 
    
    // 3. call client verify memership 
    assert(client.verifyMembership(
        client.clientState 
        proofHeight,
        proof,
        merklePath,
        commit))

    
    // Executes Application logic ∀ Payload
    payload=packet.data[0]
    cbs = router.callbacks[payload.destPort,payload.version]
    ack,success = cbs.onReceivePacket(packet.channelDestId,payload)
    abortTransactionUnless(success)
    if ack != nil {
        // NOTE: Synchronous ack. 
        writeAcknowledgement(packet, ack)
    }
    // NOTE No ack || Asynchronous ack. 
    //else: ack is nil and won't be written || ack is nil and will be written asynchronously 

    // Provable Stores 
    // we must set the receipt so it can be verified on the other side
    // it's the sentinel success receipt: []byte{0x01}
    provableStore.set(
        packetReceiptPath(packet.channelDestId, packet.sequence),
        SUCCESSFUL_RECEIPT
    )

    // log that a packet has been received
    // Event Emission
    emitLogEntry("recvPacket", {
      data: packet.data
      timeoutTimestamp: packet.timeoutTimestamp,
      sequence: packet.sequence,
      sourceId: packet.channelSourceId,
      destId: packet.channelDestId,
      relayer: relayer 
    })
    
}
```

##### Writing acknowledgements

> NOTE: The system handles synchronous and asynchronous acknowledgement logic. 

The `writeAcknowledgement` function can be called either synchronously by the IBC handler during the `receivePacket` execution or it can be called asynchronously by an application callback. 

Writing acknowledgements ensures that application modules callabacks have been triggered and have returned their specific acknowledgment in order to write data which resulted from processing an IBC packet that the sending chain can then verify. Writing acknowledgement serves as a sort of "execution receipt" or "RPC call response".

`writeAcknowledgement` can be called either in a `receivePacket`, or after the `receivePacket` execution in a later on application callback. Given that the `receivePacket` logic is always execute before the `writeAcknowledgement` it *does not* check if the packet being acknowledged was actually received, because this would result in proofs being verified twice for acknowledged packets. This aspect of correctness is the responsibility of the IBC handler.

The IBC handler performs the following steps in order:

- Checks that an acknowledgement for this packet has not yet been written
- Sets the opaque acknowledgement value at a store path unique to the packet

###### Conditions Table 

| **Condition Type**            | **Description** | **Code Checks** |
|-------------------------------|------------|------------|
| **pre-conditions**            | - `receivePacket` has been called on chain `B`.<br> - `onReceivePacket` application callback has been executed.<br> - `writeAcknowledgement` has not been called yet | `provableStore.get(packetAcknowledgementPath(packet.channelDestId, packet.sequence) === null` |
| **Error-Conditions**           | - acknowledgement is empty.<br> - The `packetAcknowledgementPath` stores already a value. | - `len(acknowledgement) === 0`.<br> - `provableStore.get(packetAcknowledgementPath(packet.channelDestId, packet.sequence) !== null` |
| **Post-Conditions (Success)**  | - The opaque acknowledgement has been written at `packetAcknowledgementPath`. | - `provableStore.get(packetAcknowledgementPath(packet.channelDestId, packet.sequence) !== null` |
| **Post-Conditions (Error)**    | - No value is stored at the `packetAcknowledgementPath`. | - `provableStore.get(packetAcknowledgementPath(packet.channelDestId, packet.sequence) === null` |

```typescript
function writeAcknowledgement(
  packet: Packet,
  acknowledgement: Acknowledgement) {
    // acknowledgement must not be empty
    abortTransactionUnless(len(acknowledgement) !== 0)

    // cannot already have written the acknowledgement
    abortTransactionUnless(provableStore.get(packetAcknowledgementPath(packet.channelDestId, packet.sequence) === null))

    // create the acknowledgement coomit using the function defined in [packet specification](https://github.com/cosmos/ibc/blob/c7b2e6d5184b5310843719b428923e0c5ee5a026/spec/core/v2/ics-004-packet-semantics/PACKET.md)
    commit=commitV2Acknowledgment(acknowledgement)
    
    provableStore.set(
    packetAcknowledgementPath(packet.channelDestId, packet.sequence),commit)

    // log that a packet has been acknowledged
    // Event Emission
    emitLogEntry("writeAcknowledgement", {
      sequence: packet.sequence,
      sourceId: packet.channelSourceId,
      destId: packet.channelDestId,
      timeoutTimestamp: packet.timeoutTimestamp,
      data: packet.data,
      acknowledgement
    })
}
```

##### Processing acknowledgements

The `acknowledgePacket` function is called by the IBC handler to process the acknowledgement of a packet previously sent by the source chain that has been received on the destination chain. The `acknowledgePacket` also cleans up the packet commitment, which is no longer necessary since the packet has been received and acted upon.

The IBC hanlder MUST atomically trigger the callbacks execution of appropriate application acknowledgement-handling logic in conjunction with calling `acknowledgePacket`.

###### Conditions Table  

Given that at this point of the packet flow, chain `B` has sucessfully received a packet, the pre-conditions defines what MUST be accomplished before chain `A` can properly execute the `acknowledgePacket` for the IBC v2 packet. 

| **Condition Type** | **Description** | **Code Checks** |
|-------------------------------|---------------------------------|---------------------------------|
| **pre-conditions**            | - chain `B` has successfully received a packet and has written the acknowledgment.<br> - PacketCommitment has not been cleared out yet. |- `provableStore.get(packetCommitmentPath(packet.channelSourceId, packet.sequence)) ===  commitV2Packet(packet)`.<br> - `verifyMembership(packetacknowledgementPath,...,) ==  True` |
| **Error-Conditions**           | - PacketCommitment already cleared out.<br> - Unset Acknowledgment.<br> - Unsuccessful payload execution. | - `provableStore.get(packetCommitmentPath(packet.channelSourceId, packet.sequence)) ===  null`.<br> - `verifyMembership(packetacknowledgementPath,...,) ==  False`.<br> - `OnAcknowledgePacket(packet.channelSourceId,payload, acknowledgement) == False` | 
| **Post-Conditions (Success)**  | - All the applications pointed in the payload have properly terminated the `onAcknowledgePacket` callback execution.<br> - The packetCommitment has been cleared out. | - `provableStore.get(packetCommitmentPath(packet.channelSourceId, packet.sequence)) === null` |
| **Post-Conditions (Error)**    | - If one payload fails, then all state changes that happened on the successful `onAcknowledgePacket` application callback execution are reverted.<br> - The packetCommitment has not been cleared out.<br> | - `provableStore.get(packetCommitmentPath(packet.channelSourceId, packet.sequence)) ===  commitV2Packet(packet)` |
                                                                                                                
###### Pseudo-Code 

The ICS04 provides an example pseudo-code that enforce the above described conditions so that the following sequence of steps must occur for a packet to be acknowledged from module *1* on machine *A* to module *2* on machine *B*.

// NEED DISCUSSION:   What to do with the relayer? Do we want to keep it? // We pass the `relayer` address just as in [Receiving packets](#receiving-packets) to allow for possible incentivization here as well.

```typescript
function acknowledgePacket(
    packet: OpaquePacket,
    acknowledgement: Acknowledgement,
    proof: CommitmentProof,
    proofHeight: Height,
    relayer: string
) {

    // Channel and Client Checks
    channel = getChannel(packet.channelSourceId)
    assert(channel !== null)
    client = router.clients[channel.clientId]
    assert(client !== null)
    
    //assert(packet.destId == channel.counterpartyChannelId) // Tautology
   
    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(packet.channelSourceId, packet.sequence)) ===  commitV2Packet(packet))

    // verify that the acknowledgement exist at the desired path  
    ackPath = packetAcknowledgementPath(packet.channelDestId, packet.sequence)
    merklePath = applyPrefix(channel.keyPrefix, ackPath)
    assert(client.verifyMembership(
        client.clientState
        proofHeight,
        proof,
        merklePath,
        acknowledgement
    ))
     
    if(acknowledgement!= SENTINEL_ACKNOWLEDGEMENT){ // Do we want this? 
        // Executes Application logic ∀ Payload
        payload=packet.data[0]
        cbs = router.callbacks[payload.sourcePort,payload.version]
        success= cbs.OnAcknowledgePacket(packet.channelSourceId,payload, acknowledgement)
        abortUnless(success) 
    }

    channelStore.delete(packetCommitmentPath(packet.channelSourceId, packet.sequence))
    
    // Event Emission // Check fields
    emitLogEntry("acknowledgePacket", {
      sequence: packet.sequence,
      sourceId: packet.channelSourceId,
      destId: packet.channelDestId,
      timeoutTimestamp: packet.timeoutTimestamp,
      data: packet.data,
      acknowledgement
    })
}
```

##### Acknowledgement Envelope

The acknowledgement returned from the remote chain is defined as arbitrary bytes in the IBC protocol. This data
may either encode a successful execution or a failure (anything besides a timeout). There is no generic way to
distinguish the two cases, which requires that any client-side packet visualiser understands every app-specific protocol
in order to distinguish the case of successful or failed relay. In order to reduce this issue, we offer an additional
specification for acknowledgement formats, which [SHOULD](https://www.ietf.org/rfc/rfc2119.txt) be used by the
app-specific protocols.

```proto
message Acknowledgement {
  oneof response {
    bytes result = 21;
    string error = 22;
  }
}
```

If an application uses a different format for acknowledgement bytes, it MUST not deserialise to a valid protobuf message
of this format. Note that all packets contain exactly one non-empty field, and it must be result or error.  The field
numbers 21 and 22 were explicitly chosen to avoid accidental conflicts with other protobuf message formats used
for acknowledgements. The first byte of any message with this format will be the non-ASCII values `0xaa` (result)
or `0xb2` (error).

#### Timeouts

Application semantics may require some timeout: an upper limit to how long the chain will wait for a transaction to be processed before considering it an error. Since the two chains have different local clocks, this is an obvious attack vector for a double spend - an attacker may delay the relay of the receipt or wait to send the packet until right after the timeout - so applications cannot safely implement naive timeout logic themselves.

Note that in order to avoid any possible "double-spend" attacks, the timeout algorithm requires that the destination chain is running and reachable. One can prove nothing in a complete network partition, and must wait to connect; the timeout must be proven on the recipient chain, not simply the absence of a response on the sending chain.

##### Sending end

The `timeoutPacket` function is called by the IBC hanlder by the chain that attempted to send a packet to a counterparty module,
where the timeout height or timeout timestamp has passed on the counterparty chain without the packet being committed, to prove that the packet
can no longer be executed and to allow the calling module to safely perform appropriate state transitions.

Calling modules MAY atomically execute appropriate application timeout-handling logic in conjunction with calling `timeoutPacket`.

The `timeoutPacket` checks the absence of the receipt key (which will have been written if the packet was received). 
We pass the `relayer` address just as in [Receiving packets](#receiving-packets) to allow for possible incentivization here as well.

###### Conditions Table  

| **Condition Type**            | **Description**                                                                                                                               |
|-------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| **pre-conditions**            | - PacketReceipt MUST be empty.<br> - PacketCommitment has not been cleared out yet. |
| **Error-Conditions**           | - PacketCommitment already cleared out.<br> - PacketReceipt is not empty.<br> - Unsuccessful payload execution. |
| **Post-Conditions (Success)**  | - All the applications pointed in the payload have properly terminated the `onTimeoutPacket` callback execution, reverting the state changes occurred in `onSendPacket`.<br> - The packetCommitment has been cleared out. |
| **Post-Conditions (Error)**    | - If one payload fails, then all state changes that happened on the successful `onTimeoutPacket` application callback execution MUST be reverted.<br> - Note that here we may get stuck if one `onTimeoutPacket` application always fails.<br> - The packetCommitment has not been cleared out. |

###### Pseudo-Code 

```typescript
function timeoutPacket(
    packet: OpaquePacket,
    proof: CommitmentProof,
    proofHeight: Height,
    relayer: string
) { 
    // Channel and Client Checks
    channel = getChannel(packet.channelSourceId)
    client = router.clients[channel.clientId]

    assert(client !== null)
    
    //assert(packet.destId == channel.counterpartyChannelId)

    // verify we sent the packet and haven't cleared it out yet
    assert(provableStore.get(packetCommitmentPath(packet.channelSourceId, packet.sequence))
           === commitV2Packet(packet))

    // get the timestamp from the final consensus state in the channel path
    var proofTimestamp
    proofTimestamp = client.getTimestampAtHeight(proofHeight)
    assert(err != nil)

    // check that timeout height or timeout timestamp has passed on the other end
    asert(packet.timeoutTimestamp > 0 && proofTimestamp >= packet.timeoutTimestamp)

    // verify there is no packet receipt --> receivePacket has not been called 
    receiptPath = packetReceiptPath(packet.channelDestId, packet.sequence)
    merklePath = applyPrefix(channel.keyPrefix, receiptPath)
    assert(client.verifyNonMembership(
        client.clientState,
        proofHeight,
        proof,
        merklePath
    ))

    payload=packet.data[0]
    cbs = router.callbacks[payload.sourcePort,payload.version]
    success=cbs.OnTimeoutPacket(packet.channelSourceId,payload)
    abortUnless(success)

    channelStore.delete(packetCommitmentPath(packet.channelSourceId, packet.sequence))
    
    // Event Emission // See fields
    emitLogEntry("timeoutPacket", {
      sequence: packet.sequence,
      sourceId: packet.channelSourceId,
      destId: packet.channelDestId,
      timeoutTimestamp: packet.timeoutTimestamp,
      data: packet.data,
      acknowledgement
    })
}
```

##### Cleaning up state

Packets MUST be acknowledged or timed-out in order to be cleaned-up. 

#### Reasoning about race conditions

TODO 

##### Identifier allocation

There is an unavoidable race condition on identifier allocation on the destination chain. Modules would be well-advised to utilise pseudo-random, non-valuable identifiers. Managing to claim the identifier that another module wishes to use, however, while annoying, cannot man-in-the-middle a handshake since the receiving module must already own the port to which the handshake was targeted.

##### Timeouts / packet confirmation

There is no race condition between a packet timeout and packet confirmation, as the packet will either have passed the timeout height prior to receipt or not.

##### Clients unreachability with in-flight packets

If a client has been frozen while packets are in-flight, the packets can no longer be received on the destination chain and can be timed-out on the source chain.

### Properties & Invariants

- Packets are delivered exactly once, assuming that the chains are live within the timeout window, and in case of timeout can be timed-out exactly once on the sending chain.

## Backwards Compatibility

TODO Mmmm ..Not applicable.

## Forwards Compatibility

Future updates to this specification will enable the IBC protocol version 2 to process multiple payloads within a single IBC packet atomically, reducing the number of packet flows. 

## Example Implementations

- Implementation of ICS 04 in Go can be found in [ibc-go repository](https://github.com/cosmos/ibc-go).
- Implementation of ICS 04 in Rust can be found in [ibc-rs repository](https://github.com/cosmos/ibc-rs).

## History

Oct X, 2024 - [Draft submitted](https://github.com/cosmos/ibc/pull/1148)

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
