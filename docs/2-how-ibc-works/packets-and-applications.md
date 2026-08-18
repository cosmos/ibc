---
title: "Packets and applications"
description: "A packet is what IBC moves between two chains, and an application is what gives its content meaning."
---

An application is the actor on one chain that wants to talk to another chain over IBC. They communicate by sending and receiving packets across an IBC connection. This page covers a packet's fields, the payload inside it, what an application is, ports, and the callbacks every application implements.

A packet is like a piece of mail. The packet is the envelope with routing information, and the payload inside is the letter. The application that is the recipient of the packet opens it and decides what to do with the information.

## The packet

A packet is one unit of information a [relayer](/how-ibc-works/relayer) carries from one chain to another. It holds what an application wants to send, together with the routing information IBC needs to deliver it and verify it.

```solidity
struct Packet {
    uint64 sequence;
    string sourceClient;
    string destClient;
    uint64 timeoutTimestamp;
    Payload[] payloads;
}
```

The first four fields are the routing information. The last one is the application's content. This is similar to the same split TCP makes between a header and a payload.

- `sequence`: the packet's number on its source client. It comes from a counter that starts at 1 and rises by one with every send, so both chains can refer to the same packet.
- `sourceClient` and `destClient`: the two ends of the route. `sourceClient` is the sending chain's [light client](/how-ibc-works/clients-and-counterparties) of the destination; `destClient` is the destination chain's client of the source.
- `timeoutTimestamp`: the deadline in unix seconds, after which the packet can no longer be received. A relayer times it out on the source chain instead. Sending and receiving check it against the local clock , and a timeout checks it against the destination chain's clock as the light client proves it , and on send the router requires it to be in the future and no more than one day out.
- `payloads`: the application's content. The field is a list, and the router carries exactly one payload per packet.

## The payload

A payload is an application's message as raw bytes, plus information naming the application at each end and saying how to read those bytes.

```solidity
struct Payload {
    string sourcePort;
    string destPort;
    string version;
    string encoding;
    bytes value;
}
```

- `sourcePort` and `destPort`: the ports of the sending and the receiving application.
- `version`: the version of the application protocol that `value` follows.
- `encoding`: the format the value bytes are written in.
- `value`: the message itself, opaque to IBC, the way a network carries a TCP payload without reading it.

IBC moves `value` but never interprets it. The application registered on `destPort` is what gives those bytes their meaning.

An application also decides which versions and encodings it answers to.

## Applications

Applications are the business logic that give meaning to packets. They are the logic at each end of an IBC connection that wants to communicate.

An application owns the `value` field of the payload: what goes in, how it is encoded, and what happens at each step of the packet's life. IBC core delivers packets but never interprets them, so the application is responsible for the meaning.

Sending an IBC packet begins inside the application. An application runs its own logic, then hands the router a send message with its source client, a timeout, and its payload.

### Sending a packet

An application does not build a full packet. It submits a smaller message to the router on the source chain and the router completes the rest.

```solidity
struct MsgSendPacket {
    string sourceClient;
    uint64 timeoutTimestamp;
    Payload payload;
}
```

The application is responsible for supplying the source client, the timeout, and the payload. When the application calls the router, IBC core derives `destClient` from the source client's counterparty, assigns the next `sequence`, assembles the `Packet`, and commits it. The application owns its data and the route it wants; the router owns sequence numbering and the destination side of the route.

### Ports and registration

An application is registered with the IBC router on its chain, under a port. Each port identifies one application, and ports are how the router knows which application a packet is for and how to route it.

A port is either the application's own address as a hex string, which anyone may claim, or a readable name like GMP's `gmpport`, which the [access manager](/ibc-solidity-contracts/permissions-and-upgrades#the-access-manager-and-its-roles) gates. Once registered, the port identifier remains with that application permanently.

Every payload names two ports:
- `destPort` says which application the payload is for.
- `sourcePort` says who sent it, and only the application registered there may send on it.

## Application callbacks

To take part in the packet flow, an application implements three callbacks. The IBC router calls them at the right moments, and an application accepts them only from the router.

```solidity
interface IIBCApp {
    function onRecvPacket(IIBCAppCallbacks.OnRecvPacketCallback calldata msg_)
        external returns (bytes memory);
    function onAcknowledgementPacket(IIBCAppCallbacks.OnAcknowledgementPacketCallback calldata msg_)
        external;
    function onTimeoutPacket(IIBCAppCallbacks.OnTimeoutPacketCallback calldata msg_)
        external;
}
```

Each callback handles one moment in a packet's life.

- `onRecvPacket` runs on the destination chain when a packet arrives. The application decodes the payload, acts on it, and returns an acknowledgement. If it reverts with a reason, the router writes a universal error acknowledgement in its place and the packet still settles. If it reverts with no reason (like running out of gas), the whole receive fails and the packet stays undelivered.
- `onAcknowledgementPacket` runs on the source chain once the destination's acknowledgement is proven. The application reads the result and settles accordingly.
- `onTimeoutPacket` runs on the source chain when a packet's deadline passes without delivery. The application releases or reverses whatever it staged on send, or tells the sender so it can.

An acknowledgement is non-empty bytes the receiving application chooses, other than the one value reserved for errors. IBC proves them and carries them back to the sender without reading them. The sending application gets either those bytes or the one reserved error value the router writes when a receive fails.

Across these callbacks, an application carries a clear set of responsibilities:

- Sending: encode `value` and set the version and encoding.
- On receipt: validate the version and encoding, decode `value`, act, and return an acknowledgement.
- On the return leg: handle either the acknowledgement or the timeout of the packet.

The router handles routing, recording, and verification. The application handles meaning. For the order these callbacks fire in, and what the router does between them, see the [packet lifecycle](/how-ibc-works/packet-lifecycle).
