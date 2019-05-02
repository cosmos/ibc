# 4: IBC Usecases

**This is a set of possible use cases for the IBC protocol.**

**For an architectural overview, see [here](./1_IBC_ARCHITECTURE.md).**

**For a broad set of protocol design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).**

**For definitions of terms used in IBC specifications, see [here](./3_IBC_TERMINOLOGY.md).**

This is a list of possible concrete application use-cases for the inter-blockchain communication protocol.

For each use case, we define the requirements of the involved chains, the high-level packet handling logic, the application properties maintained across a combined-state view of the involved chains, and a list of potential involved zones with different comparative advantages or other application features.

## Asset Transfer

Wherever compatible native asset representations exist, IBC can be used to transfer assets between two chains.

### Asset Types

#### Fungible Tokens

IBC can be used to transfer fungible tokens between chains.

Example representations: Bitcoin, ERC20, Cosmos SDK Coins.

The "source zone", which originally held all of the tokens balances, escrows and unescrows.

The "target zone", which originally held zero balance, mints & burns vouchers.

(note: could hybridize)

(better to hybridize totally & track supply throughput?)

#### Nonfungible Tokens

IBC can be used to transfer nonfungible tokens between chains.

Example representations: ERC721, Cosmos SDK NFT.

### Involved Zones

#### Vanilla Payments

A "vanilla payments" zone, such as the Cosmos Hub, may allow incoming & outgoing token transfers through IBC. Users might elect to keep assets on such a zone due to high security or high connectivity.

#### Shielded Payments

A "shielded payments" zone, such as the Zcash blockchain (pending [UITs](https://github.com/zcash/zcash/issues/830)), may allow incoming & outgoing token transfers through IBC. Tokens which are transferred to such a zone could then be shielded through the zero-knowledge circuit and held, transferred, traded, etc. Once users had accomplished their anonymity-requiring purposes, they could be transferred out and back over IBC to other zones.

#### Decentralized Exchange

A "decentralized exchange" zone may allow incoming & outgoing token transfers through IBC.

#### Decentralized Finance

A "decentralized finance" zone, such as the Ethereum blockchain, may allow incoming & outgoing token transfers though IBC.

## Multichain Contracts

IBC can be used to implement cross-chain contract calls.

### Decentralized data oracles

### Cross-chain multisignature accounts

### Cross-chain fee payment

## Interchain Collateralization

## Sharding

### Code Migration
