---
title: The Besu QBFT and IBFT 2.0 light clients
description: The Besu light clients verify sealed Besu headers and Ethereum storage proofs, so a connection between two Besu QBFT or IBFT 2.0 chains needs no attestors.
---

The Besu light client verifies the counterparty chain's own consensus. It accepts a Besu block header once enough of the validators it trusts have sealed it, and it then answers questions about packets with Ethereum storage proofs against that header. Trust in a connection using this client rests on the counterparty's validator set, not on off-chain signers.

The chain it verifies must run Besu with QBFT or IBFT 2.0 consensus. The [CLI tutorial](../6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md) pairs a Besu client with an attestation client. Each client implements the standard [light client interface](../2-how-ibc-works/4-clients-and-counterparties.md): update the client, verify membership, verify non-membership, and accept misbehaviour evidence. In IBC-solidity it comes as two contracts, `BesuQBFTLightClient` and `BesuIBFT2LightClient`, both with the client type `besu`. They differ only in the digest commit seals sign, so everything below applies to both. Under the wrong digest no seal recovers to a validator, so the counterparty's own sealed headers tell the CLI which contract to deploy and the relayer which digest to use. A client deployed by other means for the wrong consensus rejects every update, because the seals recover to the wrong signers under its digest.

## What the client trusts

The client state fixes what it verifies against for its whole life:

```solidity
struct ClientState {
    address ibcRouter;          // the counterparty ICS26Router whose storage is proven
    Height latestHeight;        // the highest Besu height accepted so far
    uint64 trustingPeriod;      // positive lifetime of a trusted state, in seconds
    uint64 maxClockDrift;       // seconds a header may lead this chain's clock
    bool isFrozen;              // set for good once misbehaviour is proven
    TrustThreshold trustLevel;  // fraction of trusted validators that must seal an update
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

Besu validators seal a block by signing a digest of its header, with the seals themselves left out: QBFT replaces them with an empty list, IBFT 2.0 drops them from the header's extra data. The client rebuilds that digest, recovers the signer of every seal, and applies two rules:

- **Trusted overlap.** At least `trustLevel` of the validators trusted at the trusted height, rounded up, must be among the signers. This is what stops a new validator set the client has never heard of from feeding it headers. The contract accepts a trust level from one third to one, fixed at deployment; `ibc deploy client besu` uses two thirds unless `--trust-level` sets another (see [Deploying](#deploying)).
- **Quorum.** At least two thirds of the header's own validator set must be among the signers, which is the same threshold Besu needs to produce the block.

The client requires the seals in ascending order of signer address. Besu does not order them that way, so the relayer recovers the signers with the digest under which they recover to the header's validators, which is the chain's consensus, and reorders the seals before submitting the header; the digest leaves the seals out, so reordering does not change what was signed.

The header must also be at most `maxClockDrift` seconds ahead of this chain's clock, and the trusted state it builds on must be younger than `trustingPeriod`. An expired consensus state can no longer anchor an update or be used for packet proofs. Advancing an expired client requires redeployment; packets pending on it stay pending, and the relayer reports the expiry on every attempt until the connection is redeployed. Updates are submitted only alongside packets: keeping the relayer online does not refresh an idle client or prevent its expiry.

The header must be above the trusted height. Heights need not be consecutive. Every QBFT and IBFT 2.0 block is final. Each update must satisfy the quorum and overlap rules against the state it builds on, which can be one installed by an earlier update in the same transaction; the light client enforces those rules when the transaction is simulated or mined.

## Misbehaviour

The client freezes itself when it sees two validly sealed consensus states that cannot both be honest:

- **Double sign.** Two different consensus states at the same height.
- **Time non-monotonicity.** A consensus state whose timestamp is not below that of a higher one.

An update that would install such a state freezes the client instead and reports misbehaviour. The relayer simulates every transaction before submitting it and does not submit one in which an update reports misbehaviour, so each attempt fails with an error saying so instead of freezing the client. The check runs at simulation, so it cannot catch a conflicting state stored between simulation and inclusion. Anyone may also submit evidence through the router's `submitMisbehaviour`: either two stored heights whose timestamps run backwards, or two headers that each pass the same seal checks as an update. Evidence that proves nothing reverts.

A frozen client rejects every update, proof and further evidence with `FrozenClientState`, and cannot be unfrozen, so every relay attempt fails with that error. Deploy a new client to restore the connection. The relayer does not watch for or submit misbehaviour evidence itself.

## Membership and non-membership

The router stores every packet commitment, receipt and acknowledgement in one mapping. The client computes the storage slot of a path from the path itself and the mapping's fixed slot, and expects an Ethereum account proof for the router against the trusted state root followed by a storage proof for that slot against the proven storage root.

| Operation | What it verifies |
|---|---|
| **Verify membership** | The proof resolves the slot to exactly the 32-byte commitment the router asks about |
| **Verify non-membership** | The proof shows the slot is absent from the storage trie |

Both take the consensus state for the proof height, the account proof nodes and the storage proof nodes, which the relayer fetches with a single `eth_getProof` call per batch. The contract caches the proven storage root for the rest of the transaction, so only the first packet in a batch carries the account proof. The counterparty's Merkle prefix must be the single empty element, which is what `ibc deploy client` registers.

## Deploying

```sh
ibc deploy client besu --chain 1 --counterparty-chain 2 --trusting-period "$TRUSTING_PERIOD" [--max-clock-drift 60s] [--trust-level 2/3]
```

The CLI reads the counterparty's head and deploys the client with that header's timestamp, state root and validators as its first trusted state. The header's commit seals recover to its validators under only one of the two digests, which tells the CLI whether to deploy the QBFT or the IBFT 2.0 client. The counterparty core stack must already be deployed and its router address set in the counterparty chain's `evm.ics26Router` configuration. A local counterparty deployment manifest is not required. Set `TRUSTING_PERIOD` to a positive duration chosen for the counterparty's validator governance and key-retirement policy: the client relies on historical validators remaining trustworthy for that period. There is no universally safe finite default. `--max-clock-drift` defaults to one minute; both durations are truncated to whole seconds. The relayer proves the counterparty's latest header, which can lead this chain's latest block time by up to about one block time of this chain plus clock skew, so set the drift above that. An update beyond it fails simulation and is retried. The deployment itself applies the same two checks to the counterparty head, so it fails if the head is already older than the trusting period or further ahead than the drift.

`--trust-level` is the fraction of the trusted validators that must seal an update, given as `n/d` from `1/3` to `1`. It defaults to `2/3`. Header voting changes one validator at a time, and Besu imports a block once a bare quorum seals it, so at the default a validator removal or addition can leave a header with too few of the validators the client trusts. The client then cannot follow header voting past that change until a later header carries enough trusted seals (see [Relaying](#relaying)). If the client must follow every single validator change, deploy it with `--trust-level 1/2`: at one half every header verifies from its parent, and one half is the highest level with that guarantee. It is never less secure than one third: forging an update takes at least as many compromised trusted validators, two of four at either level and three of five instead of two.

The client's role manager is this chain's router, so only calls routed through `ICS26Router` reach it. Rerunning the command with the same router, trusting period, clock drift and trust level reports the client as already deployed. A different router, trusting period, clock drift or trust level, or a counterparty whose consensus has changed, reports a conflict, so use a new client ID to deploy with different settings. If the manifest records the client but the chain no longer has it (for example after a devnet reset), the rerun deploys it again from the counterparty's current head. This needs a working host router: after a full reset, rerun `ibc deploy core` first.

## Relaying

A client end of type `besu` takes no parameters, for either consensus:

```yaml
clientA:
  chainId: "1"
  signer: "relayer-key"
  clientId: "besu-0"
  type: "besu"
```

The prover checks at startup that the router the client proves is the counterparty chain's configured router; with a `remoteProver`, the ProverService is responsible for it. The relayer relays at the counterparty's latest height: the prover returns the updates from the client's latest height to that height, or none when that height is not above the client's latest. When too few trusted validators sealed that header, the prover steps to the furthest header it finds it can verify, by bisecting on whether enough trusted validators sealed each header, counting the signers it recovers from the commit seals. It repeats this from each step, returning one update per step, then the update to the target. When there is more than one, the relayer sends all but the last in transactions of their own, as [Relayer](../2-how-ibc-works/5-relayer.md) describes. It sends an update even when simulation shows the client holds it, since that may be a stalled transaction still pending, which only a resend replaces with one that carries the update; a duplicate lands as a no-op. Every update transaction leaves the client further along, so when a later one fails, the next attempt starts from the height the client reached. Header voting changes at most one validator a block, which at a trust level of at most one half leaves every header verifiable from its parent, so the prover always reaches the target. Relaying runs concurrently, within one relayer as well as across several, so the client may already be past that height when the update is built. The client accepts updates only above its trusted height, so the prover then sends no update if the client stores a consensus state at that height, and otherwise fails the attempt before submitting; the next attempt relays at a newer height. Outside that guarantee the search is best effort and can stop at a header the client cannot verify from the one before: after a block replaces several validators at once, as a validator-list override in Besu's genesis `transitions` can, or at a trust level above one half, such as the default two thirds, where a single removal or addition can leave too few trusted signers. The relayer then relays the packets sent up to the furthest height its updates reach, holds the rest, and logs once where the search stopped. A later header may still carry the client across, but the prover does not try every header; if the client stays stopped, replace it by deploying one from a fresh trusted header and pointing the client ID at it with the router's admin-only `migrateClient`. If the client's latest consensus state is older than the trusting period, the update fails simulation; an expired client can only be replaced by a new one. Packet proofs share one `eth_getProof` response across packets at the same height. No attestors are involved. Everything the relayer submits is verified by the light client when the transaction is simulated.
