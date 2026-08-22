---
title: The attestation light client
description: The attestation light client accepts a claim about the counterparty chain once a quorum of a fixed attestor set has signed it.
---

The attestation light client is a light client that trusts a fixed set of off-chain signers, called attestors. It accepts a claim about the counterparty chain once a threshold of them has signed that claim.

Unlike a light client that directly verifies the counterparty chain's consensus, this one checks signatures from a known attestor set. It holds no view of that chain of its own, and instead accepts a claim carrying enough signatures from that set. Trust in a connection using this client rests on the attestor keys.

A connection carries one of these clients on each side, each registered with its own chain's router. The attestation light client implements the standard [light client interface](/how-ibc-works/clients-and-counterparties) for IBC: update the client, verify membership, and verify non-membership. In IBC-solidity, it is the `AttestationLightClient` contract.

## The attestor set and threshold

Each side's client fixes its own list of attestor addresses and its own signature threshold when it is deployed. That list and that number are the trust root of the connection.

The client state is four fields:

```solidity
struct ClientState {
    address[] attestorAddresses;
    uint8 minRequiredSigs;
    uint64 latestHeight;
    bool isFrozen;
}
```

Anything off chain can read that state through `getClientState`.

The latest height is the highest height the client knows, and the frozen flag records whether the client has stopped verifying for good. Beside that state the client keeps two more records: a lookup of which addresses belong to the set, and one trusted timestamp per height.

That timestamp is this client's [consensus state](/how-ibc-works/clients-and-counterparties), and it is the whole of what the client has accepted as true about the counterparty chain. Nothing else is retained, so the client can answer a question about a commitment only from a claim signed for that question.

The set and the threshold hold for the life of the client. Deployment fixes them along with an initial height and timestamp, and rejects an empty set, a zero threshold, a threshold larger than the set, and duplicate addresses.

Each attestor adds two assumptions of its own: that its chain RPC endpoint reports accurate state, and that its signing key stays secret. [Attestors](/light-clients/attestors) covers how attestors run and how their keys are held.

## What an attestation claims

An attestation is one of two signed statements about the counterparty chain:

1. A **State Attestation**: height `H` had timestamp `T`.

2. A **Packet Attestation**: at height `H`, the specified commitment paths held certain values.

Those paths are records in the counterparty chain's [store](/how-ibc-works/core-router-and-store), which its router writes as packets move.

An attestor reads the block at the height it is attesting and takes the timestamp from that block's header, so two honest attestors reading the same block report the same number.

Attestors sign those two statements, and the client builds its entire view of the counterparty chain from them.

A claim reaches the client in one envelope, carrying the encoded attestation data and the signatures over it.

```solidity
struct AttestationProof {
    bytes attestationData;  // one of the two attestations below, ABI-encoded
    bytes[] signatures;     // 65 bytes each
}

struct StateAttestation {
    uint64 height;
    uint64 timestamp;
}

struct PacketAttestation {
    uint64 height;
    PacketCompact[] packets;
}

struct PacketCompact {
    bytes32 path;        // the commitment path, hashed
    bytes32 commitment;  // what that path held
}
```

The two attestation types both rely on the height. A state attestation lands a height and its timestamp, and a packet attestation is accepted only at a height the client already holds a timestamp for. One state attestation therefore serves every packet check at that height, which is why a relayer sends the update ahead of the packet calls it carries.

## How a quorum is verified

A quorum is the client's signature threshold for attestations. It is the number of distinct attestors from its set that signed the same bytes. The threshold is `minRequiredSigs` in the client state, set when the client is deployed, and it can be anywhere from one signature to the whole set. No function changes it afterwards, so a different threshold means deploying a new client.

Attestors sign a digest of the attestation data, tagged with whether it is a state or packet attestation: `sha256(typeTag || sha256(attestationData))`. That tag is the only thing bound beyond the data itself.

The client first requires at least the threshold number of signatures. Each signature it was given must then pass the client's own checks, or the whole call reverts:

- It is of the fixed signature length.
- It recovers to an address in the configured attestor set.
- The recovered signer has not been counted already.

What a passing call proves is that a threshold of known keys signed those exact bytes.

<Warning>
Do not reuse an attestor address across clients. A signature from that key verifies on every client that trusts it.
</Warning>

## Updates and proof checks

A verified quorum feeds three operations, one for each question this client answers with an attestation.

| Operation | What it verifies | What it then does |
|---|---|---|
| **Update the client** | A quorum over a state attestation, carrying a nonzero height and timestamp | Stores that timestamp for that height |
| **Verify membership** | A quorum over a packet attestation, attesting exactly the height the router asked about | Hashes the requested path, finds the entry whose path hash and commitment both match, and returns that height's timestamp |
| **Verify non-membership** | The same quorum and height checks | Requires the queried path to appear in the attested list with a zero commitment |

An update stores a height the client did not have, and the two proof checks read a height it already holds. The latest height advances if an incoming update is for a higher height. An update for a lower height is still accepted but does not advance the latest height, so updates can arrive out of order. [AttestationLightClient](/ibc-solidity-contracts/attestation-light-client) carries the function signatures and return values.

To attest non-membership, attestors sign the path with a zero commitment, which attests that no commitment for that path was stored as of that height.

## Freezing

If two conflicting timestamps for the same height are presented, the client freezes permanently.

Two timestamps for one height mean the attestor set has contradicted itself. Either the counterparty chain reorged and that height no longer holds the block they signed for, or a quorum signed a timestamp that was never true. The client cannot tell which happened, and it cannot tell which of the two timestamps was honest, so it freezes permanently.

A frozen client refuses updates and proof checks, while its getters and role administration keep working. There is no way to unfreeze an attestation light client, so recovery means deploying a new client with its own attestor set. The router's admin-gated [client migration](/how-ibc-works/clients-and-counterparties) can swap that new client in under the same identifier.

<Warning>
Attest only heights that cannot be reorged. A reorg reaches the client as two quorums signing different timestamps for one height, which freezes it permanently. The [finality offset](/light-clients/attestors) is the setting that holds an attestor back: left unset it signs no further than the chain's own finalized block, and a positive value keeps it that many blocks behind the head.
</Warning>

## What the attestor set controls

An honest attestor checks every claim against its own view of the chain before signing, so its signature means it saw that claim there. That is what stops a relayer having packets attested at its own choosing.

At the threshold the signatures are the truth. At quorum, a malicious set can sign commitments for transfers that never happened, or freeze the client for good with two timestamps for one height.

The set and the threshold are therefore the security parameter: they fix how many keys an attacker needs, and who holds them decides how hard those are to get. For this reason, it is important to take care when selecting the attestors and their threshold.

## When attestors go quiet or disagree

Anything short of a quorum costs liveness, never safety. Attestation is pull-based, so an attestor that cannot be reached drops out of the count, and height selection picks the highest height a threshold of attestors have reached.

Below the threshold, proof generation fails and the packet stays pending. Timeouts stall the same way, because absence needs a signed attestation like any other claim. Nothing is lost while it lasts, since the client does not expire.

Disagreement stalls a connection identically. Only byte-identical attestation data aggregates, so a proof exists once one group of attestors reaches the threshold on its own.
