## ZKPs on the interchain (brainstorming document)

### ZKPs for cross-chain communication

- Slotting in to replace existing verification primitives which could be done without ZK.
- Primarily: Zero-knowledge light clients (light client covers wide category)
- Not part of the state machine itself.
- Examples: Celo light client, Coda light client (could also be used over IBC easily)

### ZKPs as part of a state machine on the interchain

#### A shielded pool on the interchain / B - Many shielded pools on the interchain

- Alteration of Sapling circuit for multi-asset shielded pool
- Chain in question accepts incoming (transparent) tokens over IBC
- Tokens can be "shielded" into the shielded pool
- Tokens can be "unshielded" then sent out to other chains (transparently)
- All denominations share the same anonymity set
- Can map between denominations, e.g. ERC20 <> UIT, easily

##### Complexities

- Asset-specific rules (e.g. 10% fee) hard to enforce privately.
  - Possible to do a sort of P2SH thing where the script hash is a verification key for a circuit
  - However, if a 10% fee is sent to an address, that address knows all the amounts
- Accounts systems (required for e.g. delegation) are hard to implement in a circuit (no serious designs yet).
  - Not as conducive as UTXO system
  - Notions of "balance in account", "slashing" hard to port to UTXOs
- Ideally most of the value & transfers are in a single shielded pool, but:
  - Then you have to move out every time you want to do something, tx costs
  - There might not be enough at stake to secure it
  - May be harder to upgrade, add custom asset-specific rules

#### A shielded pool across the interchain

- Daira scaling proposal for Zcash.
