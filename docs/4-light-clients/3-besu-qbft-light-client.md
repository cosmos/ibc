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
    uint64 trustingPeriod;  // positive lifetime of a trusted state, in seconds
    uint64 maxClockDrift;   // seconds a header may lead this chain's clock
}
```

For every height it has accepted, the client trusts one consensus state: the header's timestamp, its state root, and the validator set sealed in the header.

```solidity
struct ConsensusState {
    uint64 timestamp;
    bytes32 stateRoot;
    address[] validators;
}
```

The contract stores only the hash of each consensus state. Whoever submits an update or a proof must send the full consensus state it relies on, and the contract checks that it hashes to what it stored. The relayer rebuilds consensus states from the counterparty header alone, so rebuilding never depends on state the counterparty node may have pruned.

<Warning>
Packet proofs are the only reads that need historical state. Besu's default Bonsai storage answers `eth_getProof` only for roughly the last 512 blocks, so packets whose proof height has fallen out of that window fail with an error naming the height until the relayer picks a newer height. Run the counterparty node with a larger `--bonsai-historical-block-limit`, or with archive storage, if relaying may lag that far.
</Warning>

## What an update proves

An update carries a raw Besu header, the height the client already trusts, and that height's consensus state.

Besu validators seal a block by signing a digest of its header, with the seals themselves left out. The client rebuilds that digest, recovers the signer of every seal, and applies two rules:

- **Trusted overlap.** More than one third of the validators trusted at the trusted height must be among the signers. This is what stops a new validator set the client has never heard of from feeding it headers.
- **Quorum.** At least two thirds of the header's own validator set must be among the signers, which is the same threshold Besu needs to produce the block.

The header must also be at most `maxClockDrift` seconds ahead of this chain's clock, and the trusted state it builds on must be younger than `trustingPeriod`. An expired consensus state can no longer anchor an update or be used for packet proofs. Advancing an expired client requires redeployment; packets pending on it stay pending, and the relayer reports the expiry on every attempt until the connection is redeployed. Updates are submitted only alongside packets: keeping the relayer online does not refresh an idle client or prevent its expiry.

Heights need not be consecutive. Every QBFT block is final, and the relayer submits at most one client update alongside the packets. The target header must satisfy the quorum and overlap rules directly against the trusted state; the light client enforces those rules when the transaction is simulated or mined. If validator turnover prevents a direct update, submission fails; automatic catch-up through intermediate updates is not supported.

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

The CLI reads the counterparty's header at `--height` (default: its head) and deploys the client with that header's timestamp, state root and validators as its first trusted state. The counterparty core stack must already be deployed and its router address set in the counterparty chain's `evm.ics26Router` configuration. A local counterparty deployment manifest is not required. `--trusting-period` is required. Set `TRUSTING_PERIOD` to a duration chosen for the counterparty's validator governance and key-retirement policy: the client relies on historical validators remaining trustworthy for that period. There is no universally safe finite default. The trusting period must be positive; zero is rejected. The CLI also refuses a trusted state that is already older than the trusting period by this chain's block time, since such a client could never be updated. `--max-clock-drift` defaults to one minute; both durations must be whole seconds. The relayer proves the counterparty's latest header, which can lead this chain's latest block time by up to about one block time of this chain plus clock skew, so set the drift above that. An update beyond it fails simulation and is retried. The attestation-only flags are rejected when set.

The client's role manager is this chain's router, so only calls routed through `ICS26Router` reach it. Rerunning the command with the same trust settings reports the client as already deployed; different trust settings report a conflict, so use a new client ID to deploy with different settings. A rerun reads the counterparty header again before that check, so omit `--height` on reruns once the original height has aged past the trusting period. If the manifest records the client but the chain no longer has it (for example after a devnet reset), the rerun deploys it again from a freshly read counterparty header. This needs a working host router: after a full reset, rerun `ibc deploy core` first.

## Relaying

A client end of type `besu-qbft` takes no parameters:

```yaml
clientA:
  chainId: "1"
  signer: "relayer-key"
  clientId: "besu-0"
  type: "besu-qbft"
```

The relayer checks at startup that the router the client proves is the counterparty chain's configured router. It relays at the counterparty's latest height: the client update payload encodes a single update from the client's latest height to that height, and is empty when they are the same. Relaying runs concurrently, within one relayer as well as across several, so the client may already be past that height when the update is built. The update then still starts from the client's latest height: the client verifies it in full, then installs it as a historical consensus state or no-ops if it already stores that height. If the validator set changed too much between the two heights, the update fails simulation and the next attempt uses the newer head. Packet proofs share one `eth_getProof` response across packets at the same height. No attestors are involved. Everything the relayer submits is verified by the light client when the transaction is simulated.

Misbehaviour handling is not part of this client: a conflicting consensus state for a height the client already stores is rejected, and the client keeps working.
