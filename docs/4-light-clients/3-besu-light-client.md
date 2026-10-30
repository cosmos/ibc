---
title: The Besu QBFT and IBFT 2.0 light client
description: The Besu light client verifies sealed Besu headers and Ethereum storage proofs, so a connection between two Besu QBFT or IBFT 2.0 chains needs no attestors.
---

The Besu light client verifies the counterparty chain's own consensus. It accepts a Besu block header once enough of the validators it trusts have sealed it, and it then answers questions about packets with Ethereum storage proofs against that header. Trust in a connection using this client rests on the counterparty's validator set, not on off-chain signers.

The chain it verifies must run Besu with QBFT or IBFT 2.0 consensus. The [CLI tutorial](../6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md) pairs a Besu client with an attestation client. Each client implements the standard [light client interface](../2-how-ibc-works/4-clients-and-counterparties.md): update the client, verify membership, verify non-membership, and accept misbehaviour evidence. In IBC-solidity it is one contract, `BesuLightClient`, with the client type `besu`, for both protocols. They differ only in the form of the header their validators sign, and the relayer submits each header already in its chain's form, so everything below applies to both. Under the wrong form no seal recovers to a validator, so the counterparty's own sealed headers tell the CLI and the relayer which form the chain signs. A header converted to the other form hashes differently, so its seals recover to unrelated signers and the client rejects it.

## What the client trusts

The client state fixes what it verifies against for its whole life:

```solidity
struct ClientState {
    address ibcRouter;          // the counterparty ICS26Router whose storage is proven
    Height latestHeight;        // the highest Besu height accepted so far
    uint64 trustingPeriod;      // positive lifetime of a trusted state, in seconds, at most 30 days
    uint64 maxClockDrift;       // seconds a header may lead this chain's clock, at most 12
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

An update carries a Besu header in the form its validators signed, the header's commit seals, the height the client already trusts, and that height's consensus state.

Besu validators seal a block by signing a digest of its header, with the seals themselves left out: QBFT replaces them with an empty list, IBFT 2.0 drops them from the header's extra data. The relayer submits the header in that signing form with the seals alongside, so the digest is the hash of the submitted header. The client recovers the signer of every seal against it and applies two rules:

- **Trusted overlap.** At least `trustLevel` of the validators trusted at the trusted height, rounded up, must be among the signers, capped at two thirds of them rounded down, and always more than a third of them. This is what stops a new validator set the client has never heard of from feeding it headers. The contract accepts a trust level from one third to two thirds, fixed at deployment; `ibc deploy client besu` uses two thirds unless `--trust-level` sets another (see [Deploying](#deploying)). The cap keeps a single validator vote from stalling the client: after one removal or addition, every quorum of the new set still holds that many trusted validators. It lowers the requirement only when two thirds of the trusted set is not a whole number: at the default, two of four trusted validators must sign rather than three. The more-than-a-third floor takes precedence over the cap, and only raises the requirement at a trust level of exactly one third with a trusted set whose size is a multiple of three: of six trusted validators, three must sign rather than two.
- **Quorum.** At least two thirds of the header's own validator set must be among the signers, which is the same threshold Besu needs to produce the block.

The client requires the seals in ascending order of signer address, each with a recovery ID of 27 or 28. Besu orders them otherwise and encodes the recovery ID as 0 or 1, so the relayer recovers the signers with the digest under which they recover to the header's validators, which is the chain's consensus, then reorders the seals and adds 27 to each recovery ID before submitting them; the digest leaves the seals out, so neither change alters what was signed.

The header's state root must be neither zero nor the root of an empty trie. Neither can be the state of a chain with a deployed router, and no account proof verifies against either, so a consensus state holding one could never back a proof.

The header must also be at most `maxClockDrift` seconds ahead of this chain's clock, and the trusted state it builds on must be younger than `trustingPeriod`. An expired consensus state can no longer anchor an update or be used for packet proofs. Advancing an expired client requires redeployment; packets pending on it stay pending, and the relayer reports the expiry on every attempt until the connection is redeployed. Updates are submitted only alongside packets: keeping the relayer online does not refresh an idle client or prevent its expiry.

The header must be above the trusted height. Heights need not be consecutive. Every QBFT and IBFT 2.0 block is final. Each update must satisfy the quorum and overlap rules against the state it builds on, which can be one installed by an earlier update in the same transaction; the light client enforces those rules when the transaction is simulated or mined.

## Misbehaviour

The client freezes itself when it sees two validly sealed consensus states that cannot both be honest:

- **Double sign.** Two different consensus states at the same height.
- **Time non-monotonicity.** A consensus state whose timestamp is not below that of a higher one.
- **Sub-second block time.** Two consensus states whose timestamps differ by fewer seconds than their heights. Header timestamps are whole seconds, so the client does not support sub-second block periods.
- **Invalid validator set change.** Two consensus states whose validator sets differ by more validators than their heights do, counting validators in one set but not the other. Header voting adds or removes at most one validator a block, so a larger change cannot come from one chain. A validator-list override in Besu's genesis `transitions`, which can replace several validators in one block, is outside what the client supports and makes it freezable.

An update that would install such a state freezes the client instead and reports misbehaviour. The relayer simulates every transaction before submitting it and does not submit one in which an update reports misbehaviour, so each attempt fails with an error saying so instead of freezing the client. The check runs at simulation, so it cannot catch a conflicting state stored between simulation and inclusion. Anyone may also submit evidence through the router's `submitMisbehaviour`: either two stored heights that conflict in one of those ways, or two headers that each pass the same seal checks as an update. A header that conflicts with its own trusted state, or with the state already stored at its height, is enough on its own. Evidence that proves nothing reverts, and evidence is accepted while the client is paused.

A frozen client rejects every update, proof and further evidence with `FrozenClientState`, and cannot be unfrozen, so every relay attempt fails with that error. Deploy a new client to restore the connection. The relayer does not watch for or submit misbehaviour evidence itself.

## Membership and non-membership

The router stores every packet commitment, receipt and acknowledgement in one mapping. The client computes the storage slot of a path from the path itself and the mapping's fixed slot, and expects an Ethereum account proof for the router against the trusted state root followed by a storage proof for that slot against the proven storage root.

| Operation | What it verifies |
|---|---|
| **Verify membership** | The proof resolves the slot to exactly the 32-byte commitment the router asks about |
| **Verify non-membership** | The proof shows the slot is absent from the storage trie |

Both take the consensus state for the proof height, the account proof nodes and the storage proof nodes, which the relayer fetches with a single `eth_getProof` call per batch. The contract caches the proven storage root for the rest of the transaction, so only the first packet in a batch carries the account proof. The router hands the client the packet's path behind the counterparty's Merkle prefix, which for a Besu client must be `[counterpartyRouter, IBCSTORE_STORAGE_SLOT, ""]`: the counterparty router's 20-byte address, the 32-byte slot of the router's IBC store, and an empty element the router appends the path to. The client rejects a path naming any other router than its client state's `ibcRouter`, or any other slot. `ibc deploy client besu` registers that prefix, and `ibc deploy status` checks it.

## Deploying

```sh
ibc deploy client besu --chain 1 --counterparty-chain 2 --trusting-period "$TRUSTING_PERIOD" [--trust-level 2/3]
```

The CLI reads the counterparty's head and deploys the client with that header's timestamp, state root and validators as its first trusted state. The client's maximum clock drift is always 12 seconds, the most the contract accepts. If the head is further ahead than that of this chain's latest block, the CLI trusts an earlier header instead, stepping back one block per second of excess, which is enough because the client requires blocks at least a second apart. The counterparty core stack must already be deployed and its router address set in the counterparty chain's `evm.ics26Router` configuration. A local counterparty deployment manifest is not required. Set `TRUSTING_PERIOD` to a positive duration of at most 30 days, chosen for the counterparty's validator governance and key-retirement policy: the client relies on historical validators remaining trustworthy for that period. There is no universally safe finite default. The CLI truncates it to whole seconds and leaves the bounds to the contract, whose error it reports by name, such as `InvalidTrustingPeriod`. The deployment also fails if the trusted header is already older than the trusting period.

`--trust-level` is the fraction of the trusted validators that must seal an update, given as `n/d` from `1/3` to `2/3`. It defaults to `2/3`. Header voting changes one validator at a time, and the cap on the requirement keeps every header sealed by a bare quorum verifiable from its parent at any trust level, so the client follows header voting however far behind it is (see [Relaying](#relaying)). A lower level never requires more trusted validators to seal an update, and for most set sizes requires fewer.

The client is bound to this chain's router at deployment, so only calls routed through `ICS26Router` reach it, and that router's access manager decides who may [pause it](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#permissions-on-a-light-client). A paused client rejects updates and packet proofs but still accepts misbehaviour evidence. Rerunning the command with the same router, trusting period and trust level reports the client as already deployed. A different router, trusting period or trust level reports a conflict, so use a new client ID to deploy with different settings. If the manifest records the client but the chain no longer has it (for example after a devnet reset), the rerun deploys it again from the counterparty's current head. This needs a working host router: after a full reset, rerun `ibc deploy core` first.

## Relaying

A client end of type `besu` takes no parameters, for either consensus:

```yaml
clientA:
  chainId: "1"
  signer: "relayer-key"
  clientId: "besu-0"
  type: "besu"
```

The prover checks at startup that the client is bound to this chain's router and that the router the client proves is the counterparty chain's configured router; with a `remoteProver`, the ProverService is responsible for both. A client deployed from ibc-contracts before `dfba365f` has no router binding, takes updates in a form this relayer no longer sends, and fails the first check; replace it with a new client through the router's `migrateClient`. The relayer relays at the counterparty's latest height, or, when that header is further ahead than the client's `maxClockDrift` of this chain's latest block, at an earlier header chosen the same way as at deployment, so a counterparty clock running ahead delays relaying rather than stalling it. Gas is estimated against the latest block, so a chain that stops producing blocks holds relaying back until it produces another. The prover returns the updates from the client's latest height to that height, or none when that height is not above the client's latest. When too few trusted validators sealed that header, the prover steps to the furthest header it finds it can verify, by bisecting on whether enough trusted validators sealed each header, counting the signers it recovers from the commit seals. It repeats this from each step, returning one update per step, then the update to the target. When there is more than one, the relayer sends all but the last in transactions of their own, as [Relayer](../2-how-ibc-works/5-relayer.md) describes. It sends an update even when simulation shows the client holds it, since that may be a stalled transaction still pending, which only a resend replaces with one that carries the update; a duplicate lands as a no-op. Every update transaction leaves the client further along, so when a later one fails, the next attempt starts from the height the client reached. Header voting changes at most one validator a block, which at any trust level leaves every header verifiable from its parent, so the prover always reaches the target. Relaying runs concurrently, within one relayer as well as across several, so the client may already be past that height when the update is built. The client accepts updates only above its trusted height, so the prover then sends no update if the client stores a consensus state at that height, and otherwise fails the attempt before submitting; the next attempt relays at a newer height. Outside that guarantee the search is best effort and can stop at a header the client cannot verify from the one before, such as after a validator-list override in Besu's genesis `transitions` replaces several validators at once. The relayer then relays the packets sent up to the furthest height its updates reach, holds the rest, and logs once where the search stopped. A later header may still carry the client across, but the prover does not try every header; if the client stays stopped, replace it by deploying one from a fresh trusted header and pointing the client ID at it with the router's admin-only `migrateClient`. If the client's latest consensus state is older than the trusting period, the update fails simulation; an expired client can only be replaced by a new one. Packet proofs share one `eth_getProof` response across packets at the same height. No attestors are involved. Everything the relayer submits is verified by the light client when the transaction is simulated.

Visit the [Besu light client contract page](../5-ibc-solidity-contracts/7-besu-light-client.md) for more information on the Solidity implementation.
