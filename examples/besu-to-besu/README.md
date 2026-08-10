<!-- SPDX-License-Identifier: Apache-2.0 -->

# besu-to-besu

Two single-validator Besu QBFT chains running side-by-side, intended as the
substrate for a single IBC pair:

```
       A  ◀──IBC──▶  B
```

Current scope is chain bring-up only: derive keys, render configs, boot both
chains, wait for them to produce blocks.

## No committed private keys

Every account — both validators and the pre-funded genesis allocs — is derived
from a **BIP-39 mnemonic** at init time. Nothing under `chains/local/` is
committed; only the two templates in `chains/` are.

**Each chain owns a mnemonic** and every account on that chain comes out of it.
`A_MNEMONIC` and `B_MNEMONIC` default to different (publicly known) BIP-39 test
vectors, so the two chains share no accounts at all:

| | Chain A | Chain B |
|---|---|---|
| phrase | `A_MNEMONIC` | `B_MNEMONIC` |
| funded accounts | 5 from `A_MNEMONIC`, on A only | 5 from `B_MNEMONIC`, on B only |
| deployer | index 0 | index 0 |
| validator | index 1 | index 1 |

Separate phrases per chain are deliberate:

- A QBFT light client trusts the counterparty's validator set *by address*. If A
  and B shared a validator, a header signed for one chain would be
  signature-valid against the other chain's client. Distinct phrases rule that
  out by construction.
- A validator is its chain's coinbase and pockets the priority fee. Keeping the
  deployer (index 0) off the validator slot (index 1) means an identical tx
  costs the deployer the same on both chains — otherwise it partly refunds
  itself on the chain it validates, which quietly breaks any test asserting
  exact gas spend.

Set `A_MNEMONIC` and `B_MNEMONIC` to the same value if you *want* the chains to
share accounts (derivation is cached, so that costs one pass, not two).

Because the deployer differs per chain, a contract deployed to both chains will
**not** land at the same address — `CREATE` derives it from sender and nonce.
Point both chains at one mnemonic if you need matching addresses.

`./setup.sh init` walks the mnemonics and, per chain:

1. Derives the validator key from that chain's phrase at its derivation index
   (`cast wallet private-key --mnemonic … --mnemonic-index …`).
2. Computes the QBFT genesis `extraData` for that validator — the RLP encoding
   of `[vanity, [validator], votes=[], round=0, committedSeals=[]]`.
3. Renders `besu.toml` and `el-genesis.json` from `chains/*.tmpl` and writes the
   raw key to `chains/local/<chain>/key`, which is what Besu reads.

So the mnemonics are the single source of truth: change one and the validator
address, the genesis `extraData`, and the allocs all move together. No manual
re-encoding of `extraData` when a key changes.

`A_MNEMONIC` and `B_MNEMONIC` default to standard, publicly known BIP-39 test
vectors. **Never point either of them at a phrase that holds real funds** —
derived keys are written to disk in plaintext for Besu to read.

## Layout

```
examples/besu-to-besu/
├── README.md
├── setup.sh                    — entrypoint: init | start | accounts | status | clean
├── docker-compose.yml          — besu-a + besu-b
├── lib/
│   ├── common.sh               — logging, prerequisite checks, RPC waiter,
│   │                             render_template, cast_cli (host cast or
│   │                             the foundry image)
│   └── chains.sh               — mnemonic derivation, QBFT extraData, config
│                                 rendering, start / wait / status / clean
└── chains/
    ├── besu.toml.tmpl          — rendered once per chain
    ├── el-genesis.json.tmpl    — ${CHAIN_ID}, ${QBFT_EXTRADATA}, ${GENESIS_ALLOC}
    └── local/                  — generated, gitignored:
        ├── chains.env          — funded addresses, deployer key, validators,
        │                         chain IDs, RPC URLs
        ├── A/{besu.toml, el-genesis.json, key}
        └── B/{besu.toml, el-genesis.json, key}
```

## Chains

| Chain | Chain ID | Validator source | Validator (default phrases) |
|-------|---------:|------------------|-----------------------------|
| A     |    41001 | `A_MNEMONIC` index 1 | `0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C` |
| B     |    41002 | `B_MNEMONIC` index 1 | `0x45A1eF7572a5B9998b46E54AA5Dce838965acB35` |

Neither validator is the deployer, and the two are different accounts. The
addresses are a *consequence* of the mnemonics, not a fixed fact — run
`./setup.sh accounts` to print the resolved set for whatever phrases are
configured, or read `chains/local/chains.env` after an init.

## Funded accounts

`FUNDED_ACCOUNTS` accounts (default 5) are derived **per chain** from that
chain's mnemonic and funded with 1 000 000 ETH in that chain's genesis — and
nowhere else. Chain A's accounts hold nothing on chain B and vice versa. Index 0
is that chain's deployer, index 1 its validator.

`init` logs the exact alloc set it wrote for each chain — these are the only
accounts that can pay for gas there, so it is the first thing to check when a tx
fails with *insufficient funds*:

```
[A] deriving accounts 0..4 from A_MNEMONIC...
[A] genesis funds 5 accounts with 1000000 ETH each:
  index 0  0x58A57ed9d8d624cBD12e2C467D34787555bB1b25 [deployer]
  index 1  0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C [validator]
  index 2  0xe42f4612e154153B68e241e8FDe337e0c4dD6bBD
  index 3  0x2F07c220dC62CE9531bC44B695A6D93578806d8d
  index 4  0x1454168e82efA260f49a6F86612cc6414Ba633e9
```

`./setup.sh accounts` reprints both chains' lists on demand without touching
anything. The same addresses land in `chains/local/chains.env`, namespaced per
chain:

```bash
source chains/local/chains.env
echo "$A_DEPLOYER_ADDR $A_DEPLOYER_PRIVKEY"
echo "$B_FUNDED_ADDR_3"        # indexed form — safe in any shell
for a in $A_FUNDED_ADDRS; do   # space-separated list — bash word-splitting
  cast balance "$a" --rpc-url "$A_RPC_URL" --ether
done
```

To fund more accounts, raise the count and rebuild (a genesis change needs the
chain data wiped):

```bash
./setup.sh clean && FUNDED_ACCOUNTS=10 ./setup.sh
```

A and B are fully independent: discovery is off and neither has bootnodes, so
they never gossip with each other. The only path between them is IBC.

## Ports

Both containers use the same internal ports (8545 RPC, 8546 WS, 9545 metrics).
Only the host-side mappings differ:

| Service | RPC (host) | WS (host) | Metrics (host) |
|---------|-----------:|----------:|---------------:|
| besu-a  |       8545 |      8546 |           9545 |
| besu-b  |       8745 |      8746 |           9745 |

## Usage

```bash
./setup.sh              # init + start + wait for RPC (end-to-end)
./setup.sh init         # derive keys, render chain configs into chains/local/
                        # (touches no containers)
./setup.sh start        # docker compose up both chains, wait for RPC
./setup.sh accounts     # print the funded addresses and their roles
./setup.sh status       # RPC endpoints, chain IDs, block heights
./setup.sh clean        # stop containers, remove volumes and chains/local/
```

`init` is idempotent — re-running it re-derives the same artefacts from the same
mnemonic. When the rendered genesis differs from the previous run it warns:
Besu will refuse to start against a database built on a different genesis, so
run `./setup.sh clean` first.

Quick smoke check once the chains are up:

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
  http://localhost:8745   # → 0xa02a  (= 41002, chain B)
```

A `logs/` directory is created on first run and each invocation writes a
timestamped log file there.

## Configuration

All optional; export before running `./setup.sh`.

| Variable | Default | Meaning |
|----------|---------|---------|
| `A_MNEMONIC` | `legal winner thank …` | phrase every chain A account derives from |
| `B_MNEMONIC` | `letter advice cage …` | phrase every chain B account derives from |
| `A_VALIDATOR_INDEX` | `1` | chain A's validator, within `A_MNEMONIC` |
| `B_VALIDATOR_INDEX` | `1` | chain B's validator, within `B_MNEMONIC` |
| `FUNDED_ACCOUNTS` | `5` | accounts derived per chain and funded in that chain's genesis |
| `GENESIS_BALANCE` | `0xd3c21bcecceda1000000` | wei per funded account (1M ETH) |
| `QBFT_BLOCK_PERIOD_SECONDS` | `2` | QBFT block time |
| `QBFT_EPOCH_LENGTH` | `30000` | QBFT epoch length |
| `QBFT_REQUEST_TIMEOUT_SECONDS` | `4` | QBFT round-change timeout |
| `BESU_IMAGE` | `hyperledger/besu:25.4.0` | Besu image |
| `FOUNDRY_IMAGE` | `ghcr.io/foundry-rs/foundry:latest` | fallback for key derivation |

Changing any of the first nine invalidates the on-disk chain data — run
`./setup.sh clean` before re-running.

```bash
# point chain B at your own phrase
./setup.sh clean
B_MNEMONIC="your twelve word phrase …" ./setup.sh

# make both chains share one account set (same deployer, same addresses)
./setup.sh clean
A_MNEMONIC="test test test test test test test test test test test junk" \
B_MNEMONIC="test test test test test test test test test test test junk" ./setup.sh
```

## Prerequisites

```bash
docker    # compose v2 plugin required
curl
perl
```

`cast` (foundry) is used for BIP-39 derivation. If it isn't on `PATH`,
`cast_cli` transparently falls back to running it inside `$FOUNDRY_IMAGE`, so a
host install is optional.

## What's not here

- IBC contracts, light clients, attestors, proof-api, relayer. This example
  stops at two bare chains; nothing is wired between them yet.
- Multi-validator QBFT. `qbft_extradata` encodes exactly one validator per
  chain; a validator set of two or more needs a real RLP encoder for the
  list-length prefix.
