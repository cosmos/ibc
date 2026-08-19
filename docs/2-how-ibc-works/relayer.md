---
title: "Relayer"
description: "The relayer decides when IBC packets move. The attestors and the light client decide what counts as true."
---

A relayer is the off-chain service that moves IBC packets between chains. It takes a packet already sent on a source chain, assembles the proof to update the destination chain's light client, and submits it to the destination router. It also delivers acknowledgements and timeouts from the destination chain back to the source chain.

A relayer is trusted for liveness alone: it decides when packets move, not what counts as true. Everything it carries is verified by the light client on the chain it delivers to, so an altered packet or proof would be rejected during verification.

## What a relayer does

Chains cannot interact with each other directly. Each one keeps a [light client](/how-ibc-works/clients-and-counterparties) of the other, and that client verifies a packet only against state it already holds. A relayer carries everything the other chain needs to accept a packet.

- **The packet**: The relayer delivers it to the destination chain's [router](/how-ibc-works/core-router-and-store).
- **The proof, and the client update it is checked against.** The relayer calls `updateClient` to advance the destination client's view, then submits the packet with a proof at that height. Every client type works this way, and what the update contains is the client's own business: for an [attestation light client](/light-clients/attestation-light-client) it is a quorum of attestor signatures.
- **The answer.** The acknowledgement the receiving application returned, whether it reports success or an error, goes back to the source chain. If the deadline passes with nothing received, a proof of that goes back instead.

Nothing on chain does any of this by itself, so a sent packet moves only when some relayer picks it up.

For a relayer on chains that use attestation light clients, that work divides into four duties:

- **Gather attestations**: ask the [attestors](/light-clients/attestors) a client trusts for a signed statement, and find a quorum among their answers.
- **Build the proofs**: turn that quorum into the state proof and the packet attestation the light client checks.
- **Submit packets, acknowledgements, and timeouts**: deliver the packet to the destination chain, and the acknowledgement or the timeout back to the source chain. Multiple packets for the same call get batched into one transaction.
- **Resubmit**: retry until each packet reaches a terminal state.

## Routes

A route pairs one source chain and client with one destination chain and client. A packet whose source client has no configured route is never relayed. One relayer process can serve many clients and many routes.

Routes come from the connections the relayer is configured with. Each connection names:

- A client end on each side, `clientA` and `clientB`. Each end is the other's counterparty, so neither restates the other.
- A signing key on each end, used to submit relay transactions on that end's own chain.

Relaying runs both ways, so one connection gives a route in each direction.

## What starts a relay

Once an application sends a packet to the router on the source chain, the router commits it and emits its `SendPacket` event, which means it is ready to be relayed. The relayer learns of that packet when a caller hands its API the source chain ID, the hash of the transaction that sent it, and which of that transaction's packets to relay.

When a request arrives, the relayer reads the packet events out of the transaction, records the request, and stores one packet for each send event on a configured client. The packets the request left out are stored too, and left alone until a later request selects them. Chain state holds only a 32-byte commitment, so a packet's contents live in the event the send emitted, which is why a request names a transaction. What the relayer stores is metadata, not the packet itself. Delivering the packet and delivering a timeout both read that transaction's events from the source chain again and pick out the sequence they need. So a source node that cannot serve the transaction stalls delivery, even for a packet the relayer already recorded.

## The relay pipeline

Every packet moves through the same fixed sequence of stages.

1. Check whether another party already delivered the packet or settled it.
2. Wait for the send to reach finality on the source chain.
3. Deliver the packet to the destination chain.
4. Wait for the acknowledgement, and for it to reach finality.
5. Deliver the acknowledgement back to the source chain.

A packet past its timeout with no receipt or acknowledgement gets a timeout delivered instead.

A packet that cannot finish in one pass stays in the store and is picked up on the next poll. That is how the relayer retries.

```mermaid
flowchart TB
    poll["Poll the store"]
    check["Check whether another party<br/>already delivered or settled it"]
    sendfin["Wait for send finality<br/>on the source chain"]
    branch{"Past its timeout with<br/>nothing received?"}
    recv["Deliver the packet<br/>to the destination chain"]
    ackfin["Wait for the acknowledgement<br/>and its finality"]
    ack["Deliver the acknowledgement<br/>to the source chain"]
    tofin["Wait for timeout finality<br/>on the destination chain"]
    to["Deliver the timeout<br/>to the source chain"]
    done(["Settled"])

    poll --> check --> sendfin --> branch
    branch -->|no| recv --> ackfin --> ack --> done
    branch -->|yes| tofin --> to --> done
    sendfin -.->|not final yet| poll
    ackfin -.->|no acknowledgement yet| poll
```

The two waiting periods are finality gates. Each gate asks what height can be proven right now, then compares. A send is gated on the destination client's attestors, because that client is the one that will verify the packet. The send transaction's height on the source chain must be at or below the height they can prove. An acknowledgement is gated the other way, on the source client's attestors, because the acknowledgement is proven back on the sending chain.

A timeout waits on nothing that was submitted, because no transaction carries it. Its gate reads the source client's attestors too, but compares a timestamp instead of a height. The destination chain's timestamp at the provable height must have passed the packet's timeout.

## Building a proof

What a proof contains depends on the client type it is built for. For [attestation light clients](/light-clients/attestation-light-client), a proof is a set of attestor signatures.

The relayer runs one proof generator per configured client. To build an attestation proof it queries the attestors from the client's set that its own configuration lists, all at once. It keeps the responses whose signatures check out, and groups them by byte-identical attestation data. A group that reaches the client's threshold in distinct signers becomes the proof.

It proves against the highest height a threshold of attestors have reached, so one lagging attestor cannot hold the quorum back. If the threshold is out of reach the packet stays pending, and relaying resumes when the quorum returns.

Relaying a batch produces a state proof for the client update, plus one attestation covering every packet in the batch, both at a single height.

## Submitting the transaction

The relayer batches by call: packets ready for the same call go out together as one transaction to the router's `multicall`. The transaction carries one `updateClient` call, then one call per packet. The update carries the state proof, and every packet call carries the same attestation. The client scans the attested packets for the one the call names. The update goes first because the client rejects a proof at any height it holds no consensus timestamp for.

Where a call goes depends on what it is:

- Packets go to the [router](/how-ibc-works/core-router-and-store) on the destination chain.
- Acknowledgements and timeouts go back to the router on the source chain, which is where their proofs are checked.

What each call does once it lands is covered in the [packet lifecycle](/how-ibc-works/packet-lifecycle).

A transaction that fails, or stays pending past a time limit, is cleared by the retry stage and delivered again on a later poll.

## Tracking a packet

The relayer's API reports one entry for each packet it stored from a send transaction: the state, the sequence, the source client, and the transaction hashes recorded on each leg.

A packet's state reads as one of:

- **Not selected**: the relay request left this packet out. A later request naming it moves it to pending.
- **Pending**: the packet is still somewhere in the pipeline.
- **Succeeded**: an acknowledgement came back and settled the packet.
- **Timed out**: a timeout settled it instead.
- **Rejected**: the acknowledgement that came back carried an error.
- **Relay failed**: the relayer could not take the packet into a pipeline at all, which means its own configuration is wrong rather than the delivery having failed.

The relayer records the stage a packet is entering before it runs that stage. Those stages stay internal: a packet reads Pending through all of them until it settles.

## What a relayer is trusted for

A relayer is trusted for liveness, and for nothing else. Every packet and proof is verified by the light client on the chain that receives it, so a relayer's own account of a packet never enters the check.

That bounds what a relayer cannot do:

- It cannot forge or alter a proof. Any change to the signed bytes breaks the digest the signatures are checked against.
- It cannot alter a packet's contents. The packet it submits has to hash to the commitment the source chain stored.

The only thing a relayer can do is withhold: refusing to deliver packets, acknowledgements, or timeouts. However, a withholding relayer is replaceable. Attestations are not tied to one relayer, so another party can query the same attestors and submit the same proofs. That party can be anyone, because the [deployment the IBC CLI performs](/ibc-solidity-contracts/permissions-and-upgrades) opens the router's relay calls to every address.

A relayer is a courier: it can be slow, and it can be replaced, but it cannot alter what it carries.
