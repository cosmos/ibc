---
title: The Besu QBFT light client
description: The Besu QBFT light client verifies sealed Besu headers and Ethereum storage proofs, so a connection between two Besu QBFT chains needs no attestors.
---

The Besu QBFT light client verifies the counterparty chain's own consensus. It accepts a Besu block header once enough of the validators it trusts have sealed it, and it then answers questions about packets with Ethereum storage proofs against that header. Trust in a connection using this client rests on the counterparty's validator set, not on off-chain signers.

A connection carries one of these clients on each side when both chains run Besu with the QBFT consensus engine. Each client implements the standard [light client interface](../2-how-ibc-works/4-clients-and-counterparties.md): update the client, verify membership, and verify non-membership. In IBC-solidity it is the `BesuQBFTLightClient` contract.

## What the client trusts

The client state fixes what it verifies against for its whole life:

```solidity
struct ClientState {
    address ibcRouter;      // the counterparty ICS26Router whose storage is proven
    Height latestHeight;    // the highest Besu height accepted so far
    uint64 trustingPeriod;  // seconds a trusted state stays usable; 0 never expires
    uint64 maxClockDrift;   // seconds a header may lead this chain's clock
}
```

For every height it has accepted, the client trusts one consensus state: the header's timestamp, the storage root of the counterparty router at that height, and the validator set sealed in the header.

```solidity
struct ConsensusState {
    uint64 timestamp;
    bytes32 storageRoot;
    address[] validators;
}
```

The contract stores only the hash of each consensus state. Whoever submits an update or a proof must send the full consensus state it relies on, and the contract checks that it hashes to what it stored. The relayer keeps a bounded cache of recent consensus states, always retaining its latest verified anchor, and rebuilds missing states from the counterparty chain: the timestamp and validators come from the header, and the storage root comes from `eth_getProof` for the router at that height.

<Warning>
Besu's default Bonsai storage answers `eth_getProof` only for roughly the last 512 blocks. A relayer that has been idle longer than that cannot rebuild the consensus state it must resend, and stops with an error naming the height. Run the counterparty node with a larger `--bonsai-historical-block-limit`, or with archive storage, if the relayer may pause for long.
</Warning>

## What an update proves

An update carries a raw Besu header, the height the client already trusts, that height's consensus state, and an account proof for the counterparty router against the header's state root.

Besu validators seal a block by signing a digest of its header, with the seals themselves left out. The client rebuilds that digest, recovers the signer of every seal, and applies two rules:

- **Trusted overlap.** More than one third of the validators trusted at the trusted height must be among the signers. This is what stops a new validator set the client has never heard of from feeding it headers.
- **Quorum.** At least two thirds of the header's own validator set must be among the signers, which is the same threshold Besu needs to produce the block.

The header must also be at most `maxClockDrift` seconds ahead of this chain's clock, and the trusted state it builds on must be younger than `trustingPeriod`. A trusted state that has expired can no longer anchor an update, and the client has to be deployed again.

Heights need not be consecutive. Every QBFT block is final, so the relayer updates straight to the newest header the client will accept. When validators rotate so far between two updates that the overlap rule fails, the relayer walks forward through intermediate headers, each accepted by the set before it, and submits them in one transaction ahead of the packets.

## Membership and non-membership

The router stores every packet commitment, receipt and acknowledgement in one mapping. The client computes the storage slot of a path from the path itself and the mapping's fixed slot, and expects an Ethereum storage proof for that slot against the trusted storage root.

| Operation | What it verifies |
|---|---|
| **Verify membership** | The proof resolves the slot to exactly the 32-byte commitment the router asks about |
| **Verify non-membership** | The proof shows the slot is absent from the storage trie |

Both take the consensus state for the proof height along with the proof nodes, which the relayer fetches with a single `eth_getProof` call per batch. The counterparty's Merkle prefix must be the single empty element, which is what `ibc deploy client` registers.

## Deploying

```sh
ibc deploy client --chain 1 --counterparty-chain 2 --type besu-qbft [--height N] [--trusting-period 336h] [--max-clock-drift 60s]
```

The CLI reads the counterparty's header at `--height` (default: its head) and the counterparty router's storage root at that height, and deploys the client with that as its first trusted state. The counterparty core stack must already be deployed, because its router address is read from the manifest. `--trusting-period` defaults to never expiring and `--max-clock-drift` to one minute; both must be whole seconds. The attestation-only flags do not apply.

The client's role manager is this chain's router, so only calls routed through `ICS26Router` reach it. Rerunning the command reuses the recorded parameters and reports the client as already deployed. Explicitly supplying different trust settings reports a conflict; use a new client ID to deploy with different settings.

## Relaying

A client end of type `besu-qbft` takes no parameters:

```yaml
clientA:
  chainId: "1"
  signer: "relayer-key"
  clientId: "besu-0"
  type: "besu-qbft"
```

The relayer reads the client's state from its chain, checks that the router it proves is the counterparty chain's configured router, and warms the consensus state it trusts. It then produces, for each batch, the updates that bring the client to the batch's height and one storage proof per packet. No attestors are involved.

Misbehaviour handling is not part of this client: a conflicting consensus state for a height the client already stores is rejected, and the client keeps working.
