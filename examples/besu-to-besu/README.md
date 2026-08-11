<!-- SPDX-License-Identifier: Apache-2.0 -->

# besu-to-besu


Two independent single-validator Besu QBFT chains (A and B), the substrate for a
single IBC pair.

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

| Chain | Chain ID | Validator source | Validator (default phrases) |
|-------|---------:|------------------|-----------------------------|
| A     |    41001 | `A_MNEMONIC` index 1 | `0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C` |
| B     |    41002 | `B_MNEMONIC` index 1 | `0x45A1eF7572a5B9998b46E54AA5Dce838965acB35` |

Each chain has its own mnemonic and every account on it derives from that phrase,
so the two chains share no accounts. `FUNDED_ACCOUNTS` accounts (default 5) are
derived per chain and funded with 1 000 000 ETH in that chain's genesis and
nowhere else; index 0 is that chain's deployer, index 1 its validator. Because
the deployers differ, a contract deployed to both chains will **not** land at the
same address — `CREATE` derives it from sender and nonce. Set `A_MNEMONIC` and
`B_MNEMONIC` to the same value if you want shared accounts and matching contract
addresses.


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
