## ZKPs on the interchain (brainstorming document)

### ZKPs for cross-chain communication

No (new) state at all.

- Compute compression.
- Slotting in to replace existing verification primitives which could be done without ZK.
- Primarily: Zero-knowledge light clients (light client covers wide category.
- Not part of the state machine itself.
- Examples: Celo light client, Coda light client (could also be used over IBC easily)
- Celo light client: header verification only.
- Coda light client: full state transition verification.
- There is design space in between (verify part of state transition, verify some invariants).
- Light client isn't a great term (category too wide), we need better ones.

### ZKPs as part of a state machine on the interchain

No cross-chain state.

#### A shielded pool on the interchain / many shielded pools on the interchain

- Alteration of Sapling circuit for multi-asset shielded pool.
- Chain in question accepts incoming (transparent) tokens over IBC.
- Tokens can be "shielded" into the shielded pool.
- Tokens can be "unshielded" then sent out to other chains (transparently).
- All denominations share the same anonymity set.
- Can map between denominations, e.g. ERC20 <> UIT, easily.

##### Complexities

- Asset-specific rules (e.g. 10% fee) hard to enforce privately.
  - Possible to do a sort of P2SH thing where the script hash is a verification key for a circuit.
  - However, if a 10% fee is sent to an address, that address knows all the amounts.
- Accounts systems (required for e.g. delegation) are hard to implement in a circuit (no serious designs yet).
  - Not as conducive as UTXO system.
  - Notions of "balance in account", "slashing" hard to port to UTXOs.
- Ideally most of the value & transfers are in a single shielded pool, but:
  - Then you have to move out every time you want to do something, tx costs.
  - There might not be enough at stake to secure it.
  - May be harder to upgrade, add custom asset-specific rules.
- Costs of inflation for proof-of-stake
  - Have single shielded pool, pool delegates to a validator (known supply) - how to choose which validator? How to distribute rewards?
  - Try to implement PoS logic in circuit.
    - Need accounts system or large modifications.
    - Should proof-of-stake really be private? Knowledge of e.g. if there is a validator > 33% seems useful to have.
- Private governance might be desirable / interesting.
  - Less transparent? Pretty separate problem from vote buying (possible either way).
  - Maybe there are ways to make vote buying expensive, e.g. same spending/viewing/voting key, so if you prove vote the receipient can spend - but always another ZKP is possible.

#### A shielded exchange on the interchain

- Zexe-style? Any implementers?
- Onther Tech: https://github.com/Onther-Tech/zk-dex.
- What is shielded / unshielded here?
- Any complexities from the multi-chain use case?
- Mostly not different than many ERC20 token contracts & an Ethereum DEX.

### ZKPs as part of many state machines on the interchain

Cross-chain state.

#### A shielded pool across the interchain

- Shard the state of the circuit across many chains.
- Needs finality on each chain, some transactions must wait a block or two to be sequenced.
- Shared anonymity set across all chains (presumably multi-asset), can scale.
- Daira scaling proposal for Zcash.
  - Shard the note (commitment) set & nullifier set.
  - Synchronisation of the commitment tree (subtrees) after each block.
- Accept incoming transfers in zero-knowledge (w/finality).

##### Complexities

- Exposed to consensus fault / halt risk of _any_ of the chains.
- Chains must agree on circuit construction.
- Finality is required, if finality is high-latency UX may be problematic.
- Have to balance state across chains (depends on specifics).
- Would like some way to limit risk.
  - Give a validator a viewing key, they can see amounts.
  - Rotate the key, stake the validator, slash if revealed.
  - Alternative: some sort of MPC, sharded shares of secret.
