# 4: IBC Use-cases

**This is a set of possible application-level use cases for the inter-blockchain communication protocol.**

**For definitions of terms used in IBC specifications, see [here](./1_IBC_TERMINOLOGY.md).**

**For an architectural overview, see [here](./2_IBC_ARCHITECTURE.md).**

**For a broad set of protocol design principles, see [here](./3_IBC_DESIGN_PRINCIPLES.md).**

**For a discussion of design patterns, see [here](./5_IBC_DESIGN_PATTERNS.md).**

This is a far-from-comprehensive list of possible concrete application use-cases for the inter-blockchain communication protocol (IBC), listed here for inspiration & with the intent of providing inspiration and a set of viewpoints from which to evaluate the design of the protocol.

For each use case, we define the requirements of the involved chains, the high-level packet handling logic, the application properties maintained across a combined-state view of the involved chains, and a list of potential involved zones with different comparative advantages or other application features.

## Asset transfer

Wherever compatible native asset representations exist, IBC can be used to transfer assets between two chains.

### Fungible tokens

IBC can be used to transfer fungible tokens between chains.

#### Representations

Bitcoin `UTXO`, Ethereum `ERC20`, Cosmos SDK `sdk.Coins`.

#### Implementation

Two chains elect to "peg" two semantically compatible fungible token denominations to each other, escrowing, unescrowing, minting, and burning as necessary when sending & handling IBC packets.

There may be a starting "source zone", which starts with the entire token balance, and "target zone", which starts with zero token balance, or two zones may both start off with nonzero balances of a token (perhaps originated on a third zone), or two zones may elect to combine the supply and render fungible two previously disparate tokens.

#### Invariants

Fungibility of any amount across all pegged representations, constant (or formulaic, in the case of a inflationary asset) total supply cumulative across chains, and tokens only exist in a spendable form on one chain at a time.

### Non-fungible tokens

IBC can be used to transfer non-fungible tokens between chains.

#### Representations

Ethereum `ERC721`, Cosmos SDK `sdk.NFT`.

#### Implementation

Two chains elect to "peg" two semantically compatible non-fungible token namespaces to each other, escrowing, unescrowing, creating, and destroying as necessary when sending & handling IBC packets.

There may be a starting "source zone" which starts with particular tokens and contains token-associated logic (e.g. breeding CryptoKitties, redeeming digital ticket), or the associated logic may be packaged along with the NFT in a format which all involved chains can understand.

#### Invariants

Any given non-fungible token exists uniquely on one chain, owned by a particular account, at any point in time, and can always be transferred back to the "source" zone to perform associated actions (e.g. breeding a CryptoKitty) if applicable.

### Involved zones

#### Vanilla payments

A "vanilla payments" zone, such as the Cosmos Hub, may allow incoming & outgoing fungible and/or non-fungible token transfers through IBC. Users might elect to keep assets on such a zone due to high security or high connectivity.

#### Shielded payments

A "shielded payments" zone, such as the Zcash blockchain (pending [UITs](https://github.com/zcash/zcash/issues/830)), may allow incoming & outgoing fungible and/or non-fungible token transfers through IBC. Tokens which are transferred to such a zone could then be shielded through the zero-knowledge circuit and held, transferred, traded, etc. Once users had accomplished their anonymity-requiring purposes, they could be transferred out and back over IBC to other zones.

#### Decentralised exchange

A "decentralised exchange" zone may allow incoming & outgoing fungible and/or non-fungible token transfers through IBC, and allow tokens stored on that zone to be traded with each other through a decentralised exchange protocol in the style of Uniswap or 0x (or future such protocols).

#### Decentralised finance

A "decentralised finance" zone, such as the Ethereum blockchain, may allow incoming & outgoing fungible and/or non-fungible token transfers though IBC, and allow tokens stored on that zone to interact with a variety of decentralised financial products: synthetic stablecoins, collateralised loans, liquidity pools, etc.

## Multichain contracts

IBC can be used to pass messages & data between contracts with logic split across several chains.

### Cross-chain contract calls

IBC can be used to execute arbitrary contract-to-contract calls between separate smart contract platform chains, with calldata and return data.

#### Representations

Contracts: Ethereum `EVM`, `WASM` (various), Tezos `Michelson`, Agoric `Jessie`.

Calldata: Ethereum `ABI`, generic serialisation formats such as RLP, Protobuf, or JSON.

#### Implementation

A contract on one zone which intends to call a contract on another zone must serialise the calldata and address of the destination contract in an IBC packet, which can be relayed through an IBC connection to the IBC handler on the destination chain, which will call the specified contract, executing any associated logic, and return the result of the call (if applicable) back in a second IBC packet to the calling contract, which will need to handle it asynchronously.

Implementing chains may elect to provide a "channel" object to contract developers, with a send end, receive end, configurable buffer size, etc. much like channels in multiprocess concurrent programming in languages such as Go or Haskell.

#### Invariants

Contract-dependent.

### Cross-chain fee payment

#### Representations

Same as "fungible tokens" as above.

#### Implementation

An account holding assets on one chain can be used to pay fees on another chain by sending tokens to an account on the first chain controlled by the validator set of the second chain and including a proof that tokens were so sent (on the first chain) in the transaction submitted to the second chain.

The funds can be periodically send back over the IBC connection from the first chain to the second chain for fee disbursement.

#### Invariants

Correct fees paid on one of two chains but not both.

### Interchain collateralisation

A subset of the validator set on one chain can elect to validate another chain and be held accountable for equivocation faults committed on that chain submitted over an IBC connection, and the second chain can delegate its validator update logic to the first chain through the same IBC connection.

#### Representations

ABCI `Evidence` and `ValidatorUpdate`.

#### Implementation

`ValidatorUpdate`s for a participating subset of the primary (collateralising) chain's validator set are relayed in IBC packets to the collateralised chain, which uses them directly to set its own validator set.

`Evidence` of any equivocations is relayed back from the collateralised chain to the primary chain so that the equivocating validator(s) can be slashed.

#### Invariants

Validators which commit an equivocation fault are slashable on at least one chain, and possibly the validator set of a collateralised chain is bound to the validator set of a primary (collateralising) chain.

## Sharding

IBC can be used to migrate smart contracts & data between blockchains with mutually comprehensible virtual machines & data formats, respectively.

### Code migration

#### Representations

Same as "cross-chain contract calls" above, with the additional requirement that all involved code be serialisable and mutually comprehensible (executable) by the involved chains.

#### Implementation

Participating chains migrate contracts, which they can all execute, between themselves according to a known balancing ("sharding") algorithm, perhaps designed to equalise load or achieve efficient locality for frequently-interacting contracts.

A routing system on top of core IBC will be required to correctly route cross-chain contract calls between contracts which may frequently switch chains.

#### Invariants

Semantics of code preserved, namespacing preserved by some sort of routing system.

### Data migration

IBC can be used to implement an arbitrary-depth multi-chain "cache" system where storage cost can be traded for access cost.

#### Representations

Generic serialisation formats, such as Amino, RLP, Protobuf, JSON.

#### Implementation

An arbitrary-depth IBC-connection-linked-list of chains, with the first chain optimised for compute and later chains optimised for cheaper storage, can implement a hierarchical cache, where data unused for a period of time on any chain is migrated to the next chain in the list. When data is necessary (e.g. for a contract call or storage access), if it not stored on the chain looking it up, it must be relayed over an IBC packet back to that chain (which can then re-cache it for some period).

#### Invariants

All data can be accessed on the primary (compute) chain when requested, with a known bound of necessary IBC hops.
