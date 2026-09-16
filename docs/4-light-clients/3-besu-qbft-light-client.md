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

For every height it has accepted, the client trusts one consensus state: the header's timestamp, its state root, and the validator set sealed in the header.

```solidity
struct ConsensusState {
    uint64 timestamp;
    bytes32 storageRoot;
    address[] validators;
}
```

The contract stores only the hash of each consensus state. Whoever submits an update or a proof must send the full consensus state it relies on, and the contract checks that it hashes to what it stored. The relayer caches the consensus states it has verified or submitted and rebuilds missing ones from the counterparty header alone, so rebuilding never depends on state the counterparty node may have pruned.

<Warning>
Packet proofs are the only reads that need historical state. Besu's default Bonsai storage answers `eth_getProof` only for roughly the last 512 blocks, so packets whose proof height has fallen out of that window fail with an error naming the height until the relayer picks a newer height. Run the counterparty node with a larger `--bonsai-historical-block-limit`, or with archive storage, if relaying may lag that far.
</Warning>

## What an update proves

An update carries a raw Besu header, the height the client already trusts, and that height's consensus state.

Besu validators seal a block by signing a digest of its header, with the seals themselves left out. The client rebuilds that digest, recovers the signer of every seal, and applies two rules:

- **Trusted overlap.** More than one third of the validators trusted at the trusted height must be among the signers. This is what stops a new validator set the client has never heard of from feeding it headers.
- **Quorum.** At least two thirds of the header's own validator set must be among the signers, which is the same threshold Besu needs to produce the block.

The header must also be at most `maxClockDrift` seconds ahead of this chain's clock, and the trusted state it builds on must be younger than `trustingPeriod`. A trusted state that has expired can no longer anchor an update, and the client has to be deployed again.

Heights need not be consecutive. Every QBFT block is final, so the relayer updates straight to the newest header the client will accept. When validators rotate so far between two updates that the overlap rule fails, the relayer bisects the intermediate headers for the newest one the trusted set still accepts, then repeats from that header until it reaches the target. The resulting updates travel as consecutive `updateClient` calls in the same transaction as the packets, each trusting the one before it.

## Membership and non-membership

The router stores every packet commitment, receipt and acknowledgement in one mapping. The client computes the storage slot of a path from the path itself and the mapping's fixed slot, and expects an Ethereum account proof for the router against the trusted state root followed by a storage proof for that slot against the proven storage root.

| Operation | What it verifies |
|---|---|
| **Verify membership** | The proof resolves the slot to exactly the 32-byte commitment the router asks about |
| **Verify non-membership** | The proof shows the slot is absent from the storage trie |

Both take the consensus state for the proof height, the account proof nodes and the storage proof nodes, which the relayer fetches with a single `eth_getProof` call per batch. The contract caches the proven storage root for the rest of the transaction, so only the first packet in a batch carries the account proof. The counterparty's Merkle prefix must be the single empty element, which is what `ibc deploy client` registers.

## Deploying

```sh
ibc deploy client --chain 1 --counterparty-chain 2 --type besu-qbft [--height N] --trusting-period "$TRUSTING_PERIOD" [--max-clock-drift 60s]
```

The CLI reads the counterparty's header at `--height` (default: its head) and deploys the client with that header's timestamp, state root and validators as its first trusted state. The counterparty core stack must already be deployed, because its router address is read from the manifest. `--trusting-period` is required for new clients. Set `TRUSTING_PERIOD` to a duration chosen for the counterparty's validator governance and key-retirement policy: the client relies on historical validators remaining trustworthy for that period. There is no universally safe finite default. Explicit `--trusting-period 0` disables expiry and requires trusting historical validator keys indefinitely. `--max-clock-drift` defaults to one minute; both durations must be whole seconds. The attestation-only flags do not apply.

The client's role manager is this chain's router, so only calls routed through `ICS26Router` reach it. Rerunning the command without trust flags reuses the recorded parameters and reports the client as already deployed. Explicitly supplying different trust settings reports a conflict; use a new client ID to deploy with different settings.

## Relaying

A client end of type `besu-qbft` takes no parameters:

```yaml
clientA:
  chainId: "1"
  signer: "relayer-key"
  clientId: "besu-0"
  type: "besu-qbft"
```

The relayer reads the client's state from its chain, checks that the router it proves is the counterparty chain's configured router, and warms the consensus state it trusts. The state proof is the chain of updates that brings the client to the target height, empty when the client already stores it. Packet proofs share one `eth_getProof` response across packets at the same height. No attestors are involved.

Misbehaviour handling is not part of this client: a conflicting consensus state for a height the client already stores is rejected, and the client keeps working.
