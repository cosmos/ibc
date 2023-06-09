---
ics: 18
title: Relayer Algorithms
stage: draft
category: IBC/TAO
kind: interface
requires: 24, 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## Synopsis

Relayer algorithms are the "physical" connection layer of IBC — off-chain processes responsible for relaying data between two chains running the IBC protocol by scanning the state of each chain, constructing appropriate datagrams, and executing them on the opposite chain as allowed by the protocol.

### Motivation

In the IBC protocol, a blockchain can only record the *intention* to send particular data to another chain — it does not have direct access to a network transport layer. Physical datagram relay must be performed by off-chain infrastructure with access to a transport layer such as TCP/IP. This standard defines the concept of a *relayer* algorithm, executable by an off-chain process with the ability to query chain state, to perform this relay.

### Definitions

A *relayer* is an off-chain process with the ability to read the state of and submit transactions to some set of ledgers utilising the IBC protocol.

### Desired Properties

- No exactly-once or deliver-or-timeout safety properties of IBC should depend on relayer behaviour (assume Byzantine relayers).
- Packet relay liveness properties of IBC should depend only on the existence of at least one correct, live relayer.
- Relaying should be permissionless, all requisite verification should be performed on-chain.
- Requisite communication between the IBC user and the relayer should be minimised.
- Provision for relayer incentivisation should be possible at the application layer.

## Technical Specification

### Basic relayer algorithm

The relayer algorithm is defined over a set `C` of chains implementing the IBC protocol. Each relayer may not necessarily have access to read state from and write datagrams to all chains in the interchain network (especially in the case of permissioned or private chains) — different relayers may relay between different subsets.

`pendingDatagrams` calculates the set of all valid datagrams to be relayed from one chain to another based on the state of both chains. The relayer must possess prior knowledge of what subset of the IBC protocol is implemented by the blockchains in the set for which they are relaying (e.g. by reading the source code). An example is defined below.

`submitDatagram` is a procedure defined per-chain (submitting a transaction of some sort). Datagrams can be submitted individually as single transactions or atomically as a single transaction if the chain supports it.

`relay` is called by the relayer every so often — no more frequently than once per block on either chain, and possibly less frequently, according to how often the relayer wishes to relay.

Different relayers may relay between different chains — as long as each pair of chains has at least one correct & live relayer and the chains remain live, all packets flowing between chains in the network will eventually be relayed.

```typescript
function relay(C: Set<Chain>) {
  for (const chain of C)
    for (const counterparty of C)
      if (counterparty !== chain) {
        const datagrams = chain.pendingDatagrams(counterparty)
        for (const localDatagram of datagrams[0])
          chain.submitDatagram(localDatagram)
        for (const counterpartyDatagram of datagrams[1])
          counterparty.submitDatagram(counterpartyDatagram)
      }
}
```

### Packets, acknowledgements, timeouts

#### Relaying packets in an ordered channel

Packets in an ordered channel can be relayed in either an event-based fashion or a query-based fashion.
For the former, the relayer should watch the source chain for events emitted whenever packets are sent,
then compose the packet using the data in the event log. For the latter, the relayer should periodically
query the send sequence on the source chain, and keep the last sequence number relayed, so that any sequences
in between the two are packets that need to be queried & then relayed. In either case, subsequently, the relayer process
should check that the destination chain has not yet received the packet by checking the receive sequence, and then relay it.

#### Relaying packets in an unordered channel

Packets in an unordered channel can be relayed in an event-based fashion.
The relayer should watch the source chain for events emitted whenever packets
are sent, then compose the packet using the data in the event log. Subsequently,
the relayer should check whether the destination chain has received the packet
already by querying for the presence of an acknowledgement at the packet's sequence
number, and if one is not yet present the relayer should relay the packet.

#### Relaying acknowledgements

Acknowledgements can be relayed in an event-based fashion. The relayer should
watch the destination chain for events emitted whenever packets are received & acknowledgements
are written, then compose the acknowledgement using the data in the event log,
check whether the packet commitment still exists on the source chain (it will be
deleted once the acknowledgement is relayed), and if so relay the acknowledgement to
the source chain.

#### Relaying timeouts (ordinary case, no TIMEOUT receipt)

Timeout relay is slightly more complex since there is no specific event emitted when
a packet times-out - it is simply the case that the packet can no longer be relayed,
since the timeout height or timestamp has passed on the destination chain. The relayer
process must elect to track a set of packets (which can be constructed by scanning event logs),
and as soon as the height or timestamp of the destination chain exceeds that of a tracked
packet, check whether the packet commitment still exists on the source chain (it will
be deleted once the timeout is relayed), and if so relay a timeout to the source chain.

#### Relaying timeouts for channels that write TIMEOUT receipts

Some channel types (e.g. ORDERED_ALLOW_TIMEOUT), can only timeout a packet if a timeout receipt
is written on the destination chain. This requires a relayer to first attempt a receive on the destination chain
even if the packet is already timed out, before they can relay a timeout to the sending chain. Thus on these channels,
relayers must check if packet has already been received on the destination chain by querying the packet receipt path.
If a value does not already exist, then attempt to receive the packet on the destination chain. If a timeout receipt
is written, then relay the timeout with a proof of the timeout receipt back to the sender chain.

### Pending datagrams

`pendingDatagrams` collates datagrams to be sent from one machine to another. The implementation of this function will depend on the subset of the IBC protocol supported by both machines & the state layout of the source machine. Particular relayers will likely also want to implement their own filter functions in order to relay only a subset of the datagrams which could possibly be relayed (e.g. the subset for which they have been paid to relay in some off-chain manner).

An example implementation which performs unidirectional relay between two chains is outlined below. It can be altered to perform bidirectional relay by switching `chain` and `counterparty`.
Which relayer process is responsible for which datagrams is a flexible choice - in this example, the relayer process relays all handshakes which started on `chain` (sending datagrams to both chains), relays all packets sent from `chain` to `counterparty`, and relays all acknowledgements of packets sent from `counterparty` to `chain`.

```typescript
function pendingDatagrams(chain: Chain, counterparty: Chain): List<Set<Datagram>> {
  const localDatagrams = []
  const counterpartyDatagrams = []

  // ICS2 : Clients
  // - Determine if light client needs to be updated (local & counterparty)
  height = chain.latestHeight()
  client = counterparty.queryClientConsensusState(chain)
  if client.height < height {
    header = chain.latestHeader()
    counterpartyDatagrams.push(ClientUpdate{chain, header})
  }
  counterpartyHeight = counterparty.latestHeight()
  client = chain.queryClientConsensusState(counterparty)
  if client.height < counterpartyHeight {
    header = counterparty.latestHeader()
    localDatagrams.push(ClientUpdate{counterparty, header})
  }

  // ICS3 : Connections
  // - Determine if any connection handshakes are in progress
  connections = chain.getConnectionsUsingClient(counterparty)
  for (const localEnd of connections) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    if (localEnd.state === INIT && remoteEnd === null)
      // Handshake has started locally (1 step done), relay `connOpenTry` to the remote end
      counterpartyDatagrams.push(ConnOpenTry{
        desiredIdentifier: localEnd.counterpartyConnectionIdentifier,
        counterpartyConnectionIdentifier: localEnd.identifier,
        counterpartyClientIdentifier: localEnd.clientIdentifier,
        counterpartyPrefix: localEnd.commitmentPrefix,
        clientIdentifier: localEnd.counterpartyClientIdentifier,
        version: localEnd.version,
        counterpartyVersion: localEnd.version,
        proofInit: localEnd.proof(height),
        proofConsensus: localEnd.client.consensusState.proof(),
        proofHeight: height,
        consensusHeight: localEnd.client.height,
      })
    else if (localEnd.state === INIT && remoteEnd.state === TRYOPEN)
      // Handshake has started on the other end (2 steps done), relay `connOpenAck` to the local end
      localDatagrams.push(ConnOpenAck{
        identifier: localEnd.identifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(counterpartyHeight),
        proofConsensus: remoteEnd.client.consensusState.proof(),
        proofHeight: counterpartyHeight,
        consensusHeight: remoteEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake has confirmed locally (3 steps done), relay `connOpenConfirm` to the remote end
      counterpartyDatagrams.push(ConnOpenConfirm{
        identifier: remoteEnd.identifier,
        proofAck: localEnd.proof(height),
        proofHeight: height,
      })
  }

  // ICS4 : Channels & Packets
  // - Determine if any channel handshakes are in progress
  // - Determine if any packets, acknowledgements, or timeouts need to be relayed
  channels = chain.getChannelsUsingConnections(connections)
  for (const localEnd of channels) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    // Deal with handshakes in progress
    if (localEnd.state === INIT && remoteEnd === null)
      // Handshake has started locally (1 step done), relay `chanOpenTry` to the remote end
      counterpartyDatagrams.push(ChanOpenTry{
        order: localEnd.order,
        connectionHops: localEnd.connectionHops.reverse(),
        portIdentifier: localEnd.counterpartyPortIdentifier,
        channelIdentifier: localEnd.counterpartyChannelIdentifier,
        counterpartyPortIdentifier: localEnd.portIdentifier,
        counterpartyChannelIdentifier: localEnd.channelIdentifier,
        version: localEnd.version,
        counterpartyVersion: localEnd.version,
        proofInit: localEnd.proof(height),
        proofHeight: height,
      })
    else if (localEnd.state === INIT && remoteEnd.state === TRYOPEN)
      // Handshake has started on the other end (2 steps done), relay `chanOpenAck` to the local end
      localDatagrams.push(ChanOpenAck{
        portIdentifier: localEnd.portIdentifier,
        channelIdentifier: localEnd.channelIdentifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(counterpartyHeight),
        proofHeight: counterpartyHeight,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake has confirmed locally (3 steps done), relay `chanOpenConfirm` to the remote end
      counterpartyDatagrams.push(ChanOpenConfirm{
        portIdentifier: remoteEnd.portIdentifier,
        channelIdentifier: remoteEnd.channelIdentifier,
        proofAck: localEnd.proof(height),
        proofHeight: height
      })

    // Deal with packets
    // First, scan logs for sent packets and relay all of them
    sentPacketLogs = queryByTopic(height, "sendPacket")
    for (const logEntry of sentPacketLogs) {
      // relay packet with this sequence number
      packetData = Packet{logEntry.sequence, logEntry.timeoutHeight, logEntry.timeoutTimestamp,
                          localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketRecv{
        packet: packetData,
        proof: packet.proof(),
        proofHeight: height,
      })
    }

    // Then, scan logs for acknowledgements, relay back to sending chain
    recvPacketLogs = queryByTopic(height, "writeAcknowledgement")
    for (const logEntry of recvPacketLogs) {
      // relay packet acknowledgement with this sequence number
      packetData = Packet{logEntry.sequence, logEntry.timeoutHeight, logEntry.timeoutTimestamp,
                          localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketAcknowledgement{
        packet: packetData,
        acknowledgement: logEntry.acknowledgement,
        proof: packet.proof(),
        proofHeight: height,
      })
    }
  }

  return [localDatagrams, counterpartyDatagrams]
}
```

Relayers may elect to filter these datagrams in order to relay particular clients, particular connections, particular channels, or even particular kinds of packets, perhaps in accordance with the fee payment model (which this document does not specify, as it may vary).

### Ordering constraints

There are implicit ordering constraints imposed on the relayer process determining which datagrams must be submitted in what order. For example, a header must be submitted to finalise the stored consensus state & commitment root for a particular height in a light client before a packet can be relayed. The relayer process is responsible for frequently querying the state of the chains between which they are relaying in order to determine what must be relayed when.

### Bundling

If the host state machine supports it, the relayer process can bundle many datagrams into a single transaction, which will cause them to be executed in sequence, and amortise any overhead costs (e.g. signature checks for fee payment).

### Race conditions

Multiple relayers relaying between the same pair of modules & chains may attempt to relay the same packet (or submit the same header) at the same time. If two relayers do so, the first transaction will succeed and the second will fail. Out-of-band coordination between the relayers or between the actors who sent the original packets and the relayers is necessary to mitigate this. Further discussion is out of scope of this standard.

### Incentivisation

The relay process must have access to accounts on both chains with sufficient balance to pay for transaction fees. Relayers may employ application-level methods to recoup these fees, such by including a small payment to themselves in the packet data — protocols for relayer fee payment will be described in future versions of this ICS or in separate ICSs.

Any number of relayer processes may be safely run in parallel (and indeed, it is expected that separate relayers will serve separate subsets of the interchain). However, they may consume unnecessary fees if they submit the same proof multiple times, so some minimal coordination may be ideal (such as assigning particular relayers to particular packets or scanning mempools for pending transactions).

### Events

As specified in [ICS 24](../../core/ics-024-host-requirements/README.md#event-logging-system),
the host state machine MUST provide an event logging system that can make event logs available 
to relayers (by streaming and/or querying). This section specifies the events emitted by code
IBC that relayers need to use to complete client, connection and channel lifecycles, relay and 
timeout packets.

Events emitted by core IBC are JSON-encoded objects following the schema:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "type",
    "attributes"
  ],
  "properties": {
    "$schema": {
      "type": "string",
      "pattern": "^(\\.\\./)+events\\.schema\\.json$"
    },
    "type": {
      "type": "string",
      "description": "Type of event (e.g. create_client, send_packet)"
    },
    "attributes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "key",
          "value"
        ],
        "properties": {
          "key": {
            "type": "string",
            "description": "Event attribute key (e.g. client_id, packet_timeout_timestamp)"
          },
          "value": {
            "type": "string",
            "description": "Event attribute value (e.g. 07-tendermint-1, 1686296228260054000)"
          },
        }
      }
    }
  }
}
```

#### Client lifecycle

Format of the event emitted when a client is created in the `clientCreate` handler:

```json
{
  "type": "create_client",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "client_type",
      "value": "{clientType}"
    },
    {
      "key": "consensus_height",
      "value": "{consensusHeight}"
    }
  ]
}
```

Format of the event emitted when a client is updated with new `ClientState` 
and `ConsensusState` information in the `clientUpdate` handler:

```json
{
  "type": "update_client",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "client_type",
      "value": "{clientType}"
    },
    {
      "key": "consensus_heights",
      "value": "{string.Join(consensusHeights, ',')"
    },
    {
      "key": "header",
      "value": "{hex.encode(protobuf.marshal(header))"
    }
  ]
}
```

Format of the event emitted when a client is frozen due to misbehaviour in 
`updateClient` handler:

```json
{
  "type": "client_misbehaviour",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "client_type",
      "value": "{clientType}"
    }
  ]
}
```

Format of the event emitted when a non-active (i.e. frozen or expired) client is 
sustituted with a new active client:

```json
{
  "type": "update_client_proposal",
  "attributes": [
    {
      "key": "subject_client_id",
      "value": "{subjectClientId}"
    },
    {
      "key": "client_type",
      "value": "{substituteClientState.clientType}"
    }
  ]
}
```

Format of the event emitted when an IBC client breaking upgrade is scheduled:

```json
{
  "type": "upgrade_client_proposal",
  "attributes": [
    {
      "key": "title",
      "value": "{title}"
    },
    {
      "key": "upgrade_plan_height",
      "value": "{plan.height}"
    }
  ]
}
```

#### Connection lifecycle

Format of the event emitted when a connection end in `INIT` state is successfully added in 
the `connOpenInit` handler:

```json
{
  "type": "connection_open_init",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionId}"
    },
    {
      "key": "counterparty_client_id",
      "value": "{counterparty.clientId}"
    },
    {
      "key": "counterparty_connection_id",
      "value": "{counterparty.connectionId}"
    }
  ]
}
```

Format of the event emitted when a connection end in `TRY` state is is successfully added in 
the `connOpenTry` handler:

```json
{
  "type": "connection_open_try",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionId}"
    },
    {
      "key": "counterparty_client_id",
      "value": "{counterparty.clientId}"
    },
    {
      "key": "counterparty_connection_id",
      "value": "{counterparty.connectionId}"
    }
  ]
}
```

Format of the event emitted when an existing connection end in `INIT` is successfully updated 
to `OPEN` state in the `connOpenAck` handler:

```json
{
  "type": "connection_open_ack",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionId}"
    },
    {
      "key": "counterparty_client_id",
      "value": "{counterparty.clientId}"
    },
    {
      "key": "counterparty_connection_id",
      "value": "{counterparty.connectionId}"
    }
  ]
}
```

Format of the event emitted when an existing connection end in `TRY` state is successfully updated 
to `OPEN` state in the `connOpenConfirm` handler:

```json
{
  "type": "connection_open_confirm",
  "attributes": [
    {
      "key": "client_id",
      "value": "{clientId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionId}"
    },
    {
      "key": "counterparty_client_id",
      "value": "{counterparty.clientId}"
    },
    {
      "key": "counterparty_connection_id",
      "value": "{counterparty.connectionId}"
    }
  ]
}
```

#### Channel lifecycle

Format of the event emitted when a channel end in `INIT` state is successfully added in 
the `handleChanOpenInit` datagram handler:

```json
{
  "type": "channel_open_init",
  "attributes": [
    {
      "key": "port_id",
      "value": "{portId}"
    },
    {
      "key": "channel_id",
      "value": "{channelId}"
    },
    {
      "key": "counterparty_port_id",
      "value": "{counterparty.portId}"
    },
    {
      "key": "counterparty_channel_id",
      "value": "{counterparty.channelId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    },
    {
      "key": "version",
      "value": "{version}"
    }
  ]
}
```

Format of the event emitted when a channel end in `TRY` state is successfully added in 
the `handleChanOpenTry` datagram handler:

```json
{
  "type": "channel_open_try",
  "attributes": [
    {
      "key": "port_id",
      "value": "{portId}"
    },
    {
      "key": "channel_id",
      "value": "{channelId}"
    },
    {
      "key": "counterparty_port_id",
      "value": "{counterparty.portId}"
    },
    {
      "key": "counterparty_channel_id",
      "value": "{counterparty.channelId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    },
    {
      "key": "version",
      "value": "{version}"
    }
  ]
}
```

Format of the event emitted when an existing channel end in `INIT` is successfully updated 
to `OPEN` state in the `handleChanOpenAck` datagram handler:

```json
{
  "type": "channel_open_ack",
  "attributes": [
    {
      "key": "port_id",
      "value": "{portId}"
    },
    {
      "key": "channel_id",
      "value": "{channelId}"
    },
    {
      "key": "counterparty_port_id",
      "value": "{counterparty.portId}"
    },
    {
      "key": "counterparty_channel_id",
      "value": "{counterparty.channelId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

Format of the event emitted when a channel end in `INIT` state is successfully added in 
the `handleChanOpenConfirm` datagram handler:

```json
{
  "type": "channel_open_confirm",
  "attributes": [
    {
      "key": "port_id",
      "value": "{portId}"
    },
    {
      "key": "channel_id",
      "value": "{channelId}"
    },
    {
      "key": "counterparty_port_id",
      "value": "{counterparty.portId}"
    },
    {
      "key": "counterparty_channel_id",
      "value": "{counterparty.channelId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

Format of the event emitted when a channel end state is successfully updated to `CLOSED` state in 
the `chanCloseInit` handler:

```json
{
  "type": "channel_close_init",
  "attributes": [
    {
      "key": "port_id",
      "value": "{portId}"
    },
    {
      "key": "channel_id",
      "value": "{channelId}"
    },
    {
      "key": "counterparty_port_id",
      "value": "{counterparty.portId}"
    },
    {
      "key": "counterparty_channel_id",
      "value": "{counterparty.channelId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

Format of the event emitted when a channel end state is successfully updated to `CLOSED` state in 
the `chanCloseConfirm` handler:

```json
{
  "type": "channel_close_confirm",
  "attributes": [
    {
      "key": "port_id",
      "value": "{portId}"
    },
    {
      "key": "channel_id",
      "value": "{channelId}"
    },
    {
      "key": "counterparty_port_id",
      "value": "{counterparty.portId}"
    },
    {
      "key": "counterparty_channel_id",
      "value": "{counterparty.channelId}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

### Packet send

Format of the event emitted when a module sends a packet:

```json
{
  "type": "send_packet",
  "attributes": [
    {
      "key": "packet_data_hex",
      "value": "{hex.encode(data)}"
    },
    {
      "key": "packet_timeout_height",
      "value": "{timeoutHeight}"
    },
    {
      "key": "packet_timeout_timestamp",
      "value": "{timeoutTimestamp}"
    },
    {
      "key": "packet_sequence",
      "value": "{sequence}"
    },
    {
      "key": "packet_src_port",
      "value": "{sourcePort}"
    },
    {
      "key": "packet_src_channel",
      "value": "{sourceChannel}"
    },
    {
      "key": "packet_dst_port",
      "value": "{destinationPort}"
    },
    {
      "key": "packet_dst_channel",
      "value": "{destinationChannel}"
    },
    {
      "key": "packet_channel_ordering",
      "value": "{ordering.string()}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

### Packet relay

Format of the event emitted when a module receives a packet in the `recvPacket` handler:

```json
{
  "type": "recv_packet",
  "attributes": [
    {
      "key": "packet_data_hex",
      "value": "{hex.encode(data)}"
    },
    {
      "key": "packet_timeout_height",
      "value": "{timeoutHeight}"
    },
    {
      "key": "packet_timeout_timestamp",
      "value": "{timeoutTimestamp}"
    },
    {
      "key": "packet_sequence",
      "value": "{sequence}"
    },
    {
      "key": "packet_src_port",
      "value": "{sourcePort}"
    },
    {
      "key": "packet_src_channel",
      "value": "{sourceChannel}"
    },
    {
      "key": "packet_dst_port",
      "value": "{destinationPort}"
    },
    {
      "key": "packet_dst_channel",
      "value": "{destinationChannel}"
    },
    {
      "key": "packet_channel_ordering",
      "value": "{ordering.string()}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

Format of the event emitted when a module writes an acknowledgement after receiving a packet
in the `handlePacketRecv` datagram handler:

```json
{
  "type": "write_acknowledgement",
  "attributes": [
    {
      "key": "packet_data_hex",
      "value": "{hex.encode(data)}"
    },
    {
      "key": "packet_timeout_height",
      "value": "{timeoutHeight}"
    },
    {
      "key": "packet_timeout_timestamp",
      "value": "{timeoutTimestamp}"
    },
    {
      "key": "packet_sequence",
      "value": "{sequence}"
    },
    {
      "key": "packet_src_port",
      "value": "{sourcePort}"
    },
    {
      "key": "packet_src_channel",
      "value": "{sourceChannel}"
    },
    {
      "key": "packet_dst_port",
      "value": "{destinationPort}"
    },
    {
      "key": "packet_dst_channel",
      "value": "{destinationChannel}"
    },
    {
      "key": "packet_ack_hex",
      "value": "{hex.encode(ack)}"
    },
    {
      "key": "packet_channel_ordering",
      "value": "{ordering.string()}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

Format of the event emitted when a module acknowledges a packet in the `acknowledgePacket` handler:

```json
{
  "type": "acknowledge_packet",
  "attributes": [
    {
      "key": "packet_timeout_height",
      "value": "{timeoutHeight}"
    },
    {
      "key": "packet_timeout_timestamp",
      "value": "{timeoutTimestamp}"
    },
    {
      "key": "packet_sequence",
      "value": "{sequence}"
    },
    {
      "key": "packet_src_port",
      "value": "{sourcePort}"
    },
    {
      "key": "packet_src_channel",
      "value": "{sourceChannel}"
    },
    {
      "key": "packet_dst_port",
      "value": "{destinationPort}"
    },
    {
      "key": "packet_dst_channel",
      "value": "{destinationChannel}"
    },
    {
      "key": "packet_channel_ordering",
      "value": "{ordering.string()}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

### Packet timeouts

Format of the event emitted when  `timeoutPacket` or `timeoutOnClose` handlers:

```json
{
  "type": "timeout_packet",
  "attributes": [
    {
      "key": "packet_timeout_height",
      "value": "{timeoutHeight}"
    },
    {
      "key": "packet_timeout_timestamp",
      "value": "{timeoutTimestamp}"
    },
    {
      "key": "packet_sequence",
      "value": "{sequence}"
    },
    {
      "key": "packet_src_port",
      "value": "{sourcePort}"
    },
    {
      "key": "packet_src_channel",
      "value": "{sourceChannel}"
    },
    {
      "key": "packet_dst_port",
      "value": "{destinationPort}"
    },
    {
      "key": "packet_dst_channel",
      "value": "{destinationChannel}"
    },
    {
      "key": "packet_channel_ordering",
      "value": "{ordering.string()}"
    },
    {
      "key": "connection_id",
      "value": "{connectionHops[0]}"
    }
  ]
}
```

## Backwards Compatibility

Not applicable. The relayer process is off-chain and can be upgraded or downgraded as necessary.

## Forwards Compatibility

Not applicable. The relayer process is off-chain and can be upgraded or downgraded as necessary.

## Example Implementations

- Implementation of ICS 18 in Go can be found in [cosmos/relayer repository](https://github.com/cosmos/relayer).
- Implementation of ICS 18 in Rust can be found in [informalsystems/hermes repository](https://github.com/informalsystems/hermes).

## History

Mar 30, 2019 - Initial draft submitted

Apr 15, 2019 - Revisions for formatting and clarity

Apr 23, 2019 - Revisions from comments; draft merged

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
