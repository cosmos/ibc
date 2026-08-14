<!-- SPDX-License-Identifier: Apache-2.0 -->

# besu-to-besu


Two independent single-validator Besu QBFT chains (A and B), the substrate for a
single IBC pair.

> **DEMO KEYS — LOCAL DEVNET ONLY.** Every private key this example derives and
> prints (to the console, the log files, and `chains/local/chains.env`) comes
> from a publicly known BIP-39 test vector. They are worthless by design.
> Never send real funds to any address here, never point `A_MNEMONIC`/
> `B_MNEMONIC` at a phrase holding real funds, and never reuse these keys or
> this key-handling approach outside this local example. See `setup.sh`'s
> header for the full disclaimer.

## Prerequisites

`docker` (with the compose v2 plugin), `curl`, `perl`.

`cast` (foundry) does the BIP-39 derivation. If it isn't on `PATH`, `cast_cli`
falls back to running it inside `$FOUNDRY_IMAGE`, so a host install is optional.

## Usage

Run from `examples/besu-to-besu/`:

```bash
./setup.sh              # init + start + wait for RPC (end-to-end)
./setup.sh init         # derive keys, render configs into chains/local/ (no containers)
./setup.sh start        # docker compose up both chains, wait for RPC
./setup.sh accounts     # print the funded addresses and their roles
./setup.sh status       # RPC endpoints, chain IDs, block heights
./setup.sh clean        # stop containers, remove volumes and chains/local/
```

Verify once the chains are up:

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
  http://localhost:8745   # → 0xa02a  (= 41002, chain B)
```

Each invocation writes a timestamped log file to `logs/`.

## Chains and accounts

| Chain | Chain ID | Validator source | Validator (default phrase) |
|-------|---------:|------------------|-----------------------------|
| A     |    41001 | `A_MNEMONIC` index 1 | `0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C` |
| B     |    41002 | `B_MNEMONIC` index 1 | `0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C` |

Each chain has its own mnemonic, but `B_MNEMONIC` defaults to the same phrase
as `A_MNEMONIC`, so out of the box both chains fund the same account set:
`FUNDED_ACCOUNTS` accounts (default 5) are funded with 1 000 000 ETH in each
chain's genesis; index 0 is the deployer, index 1 the validator. Since the
deployer and its nonce sequence match on both chains, identical `CREATE`
deployments (core router, IFT tokens, ...) land at the same address on A and B.

One key therefore works everywhere: import it once and reuse the same signer
alias as deployer, relayer, and attestor signer on both chains. This is safe
because the attestation light client this example bridges over trusts an
explicitly configured attestor address set (`deploy client --attestors`), not
either chain's own block-validator identity — sharing the validator carries no
cross-chain signature-confusion risk.

Set `A_MNEMONIC` and `B_MNEMONIC` to different phrases if you want the chains
to have fully independent account sets instead.


## Ports

Both containers use the same internal ports (8545 RPC, 8546 WS, 9545 metrics).
Only the host-side mappings differ:

| Service | RPC (host) | WS (host) | Metrics (host) |
|---------|-----------:|----------:|---------------:|
| besu-a  |       8545 |      8546 |           9545 |
| besu-b  |       8745 |      8746 |           9745 |

Every host mapping binds to `127.0.0.1` — the JSON-RPC endpoints are
unauthenticated and each node holds the key that signs every block on its chain,
so they stay off the LAN. Container-to-container traffic goes over the
`besu-besu-net` compose network and does not depend on these mappings.

## Layout

```
examples/besu-to-besu/
├── README.md
├── setup.sh                    — entrypoint: init | start | accounts | status | clean
├── docker-compose.yml          — besu-a + besu-b
├── lib/
│   ├── common.sh               — logging, prerequisite checks, RPC waiter,
│   │                             render_template, cast_cli
│   └── chains.sh               — derivation, QBFT extraData, rendering, start /
│                                 wait / status / clean
└── chains/
    ├── besu.toml.tmpl          — rendered once per chain
    ├── el-genesis.json.tmpl    — ${CHAIN_ID}, ${QBFT_EXTRADATA}, ${GENESIS_ALLOC}
    └── local/                  — generated, gitignored:
        ├── chains.env          — addresses, deployer keys, chain IDs, RPC URLs
        ├── A/{besu.toml, el-genesis.json, key}
        └── B/{besu.toml, el-genesis.json, key}
```
