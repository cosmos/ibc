<!-- SPDX-License-Identifier: Apache-2.0 -->

# besu-to-besu


Two independent single-validator Besu QBFT chains (A and B), the substrate for a
single IBC pair — and, optionally, the IBC attestation layer on top of them:
one **standalone attestor per chain**, neither holding a signing key, both
signing through a **remote signer**.

```
        chain A (41001)                     chain B (41002)

        ┌────────────┐                      ┌────────────┐
        │   besu-a   │                      │   besu-b   │
        └─────┬──────┘                      └─────┬──────┘
              │ watches                   watches │
              ▼                                   ▼
     ┌────────────────┐                  ┌────────────────┐
     │ attestor-41001 │                  │ attestor-41002 │
     └────────┬───────┘                  └───────┬────────┘
              │                                  │
              └────────── Sign (gRPC) ───────────┘
                               │
                               ▼
                         ┌───────────┐
                         │    kms    │  attestor-41001, attestor-41002
                         └───────────┘
```

## Prerequisites

`docker` (with the compose v2 plugin), `curl`, `perl`.

`cast` (foundry) does the BIP-39 derivation. If it isn't on `PATH`, `cast_cli`
falls back to running it inside `$FOUNDRY_IMAGE`, so a host install is optional.

## Usage

Run from `examples/besu-to-besu/`:

```bash
./setup.sh              # init + start + wait for RPC (chains only, no IBC)
./setup.sh init         # derive keys, render configs into chains/local/ (no containers)
./setup.sh start        # docker compose up both chains, wait for RPC
./setup.sh attestors    # the above, then kms + deploy core + both attestors
./setup.sh accounts     # print the funded addresses and their roles
./setup.sh status       # RPC endpoints, chain IDs, block heights
./setup.sh clean        # stop containers, remove volumes and chains/local/
```

The bare form stays chains-only on purpose: the
[CLI tutorial](../../docs/6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md)
runs `setup.sh` and then deploys IBC by hand with the `ibc` binary, so it needs
the chains empty.

Verify once the chains are up:

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
  http://localhost:8745   # → 0xa02a  (= 41002, chain B)
```

Each invocation writes a timestamped log file to `logs/`.

## Remote signer and standalone attestors

`./setup.sh attestors` adds the attestation layer. It derives one attestor key
per chain into `chains/local/kms/keys/`, runs `ibc deploy core` on both chains
through the one-shot `deployer` service, records the router addresses in
`chains/local/ibc.env`, and starts kms plus an attestor per chain. It ends with:

```
[10:38:06] Attestors:
[10:38:06]   attestor-41001  0x2F07c220dC62CE9531bC44B695A6D93578806d8d (chain 41001, signing through kms)
[10:38:06]   attestor-41002  0x1454168e82efA260f49a6F86612cc6414Ba633e9 (chain 41002, signing through kms)
```

There is no relayer here. These are attestors serving attestations; point your
own relayer at them with `type: remote` entries, or query them directly.

**Each attestor is a standalone external process** — its own container, its own
config, reachable over gRPC. `attestors[].type: local` in `config/attestor-*.yml`
means *that process* runs the attestor; it is a different axis from where the
key lives. A relayer reaches these with `type: remote` entries naming
`attestor-41001:3000` / `attestor-41002:3000`.

**No signing key is on disk in either attestor container.** Both are configured
with a `type: remote` signer pointing at
[cosmos/kms](https://github.com/cosmos/kms), which runs in gRPC-only mode and
serves both keys over its SignerService. The container holds `ibc.yml` and
nothing else:

```bash
docker compose exec attestor-41001 ls /home/ibc      # → ibc.yml
```

Ask an attestor what it is, and make it actually sign:

```bash
docker compose exec attestor-41001 /opt/ibc attestor info attestor-41001 --home /home/ibc
docker compose exec attestor-41001 /opt/ibc attestor latest-height attestor-41001 --home /home/ibc
docker compose exec attestor-41001 /opt/ibc attestor state-attestation attestor-41001 \
  --height <height> --home /home/ibc
```

The `signature` that comes back was produced inside kms. `state-attestation`
requires `--height`; `latest-height` gives you one that is attestable under the
configured `finalityOffset`.

### Registering a light client against these attestors

The addresses printed above are what `ibc deploy client --attestors` wants. You
have to pass the **address**, not the signer alias: a kms-held key has no
address the CLI can derive offline, and an alias backed by a remote signer
fails with `cannot derive an address for remote signer`. A client on one chain
authorizes the *counterparty's* attestor, because it verifies that chain:

```bash
# client on 41001 tracks 41002, so it authorizes attestor-41002's address
ibc deploy client --chain 41001 --counterparty-chain 41002 \
  --attestors 0x1454168e82efA260f49a6F86612cc6414Ba633e9 --threshold 1 --yes
```

`threshold 1` because each chain has exactly one attestor here.

### Keys

| kms key id       | Source                 | Used by        | Needs gas |
|------------------|------------------------|----------------|-----------|
| `attestor-41001` | `A_MNEMONIC` index 3   | attestor-41001 | no        |
| `attestor-41002` | `B_MNEMONIC` index 4   | attestor-41002 | no        |

The two sit at *different* indices because `A_MNEMONIC` and `B_MNEMONIC`
default to the same phrase, and a shared index would give two supposedly
independent attestors one address. Attestors sign attestations only and never
need a balance.

Both are `algorithm: secp256k1eth` in `config/kms.yaml`, not `secp256k1`. Only
that scheme signs a pre-hashed 32-byte digest and returns the 65-byte
recoverable signature `AttestationLightClient.sol` recovers an attestor address
from; the attestor rejects any other scheme at startup.

The key files are mode `0644`, unlike the `0600` Besu validator keys, because
the kms image runs unprivileged (uid 10001) and could not otherwise read them
through the bind mount on Linux.

The deployer is the one key kms does not hold: `ibc deploy` needs the raw
private key and rejects a remote signer outright (`deployer signer %q must be a
local key`).

### Ports

kms is deliberately **not** published. It serves plaintext gRPC and performs no
caller authentication at all, so anything that can reach it can sign with
either key. It stays on the compose network; use `docker compose exec kms` to
inspect it. A real deployment sets `tls_cert` / `tls_key` in `config/kms.yaml`
and puts network controls in front.

| Service        | gRPC (host)    |
|----------------|---------------:|
| attestor-41001 |           3001 |
| attestor-41002 |           3002 |
| kms            | not published  |

## Chains and accounts

| Chain | Chain ID | Validator source | Validator (default phrase) |
|-------|---------:|------------------|-----------------------------|
| A     |    41001 | `A_MNEMONIC` index 1 | `0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C` |
| B     |    41002 | `B_MNEMONIC` index 1 | `0x0D3eB21b6b21833A4939Cfff4810E9AE0758e12C` |

Each chain has its own mnemonic, but `B_MNEMONIC` defaults to the same phrase
as `A_MNEMONIC`, so out of the box both chains fund the same account set:
`FUNDED_ACCOUNTS` accounts (default 5) are funded with 1 000 000 ETH in each
chain's genesis; index 0 is the deployer, index 1 the validator.
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
├── setup.sh                    — entrypoint: init | start | attestors |
│                                 accounts | status | clean
├── docker-compose.yml          — besu-a, besu-b, kms, attestor-41001,
│                                 attestor-41002, deployer (profile: tools)
├── lib/
│   ├── common.sh               — logging, prerequisite checks, RPC waiter,
│   │                             render_template, cast_cli
│   ├── chains.sh               — derivation, QBFT extraData, rendering, start /
│   │                             wait / status / clean
│   └── ibc.sh                  — kms keys, `ibc deploy core`, ibc.env, the
│                                 attestors
├── config/                     — committed, no secrets. Bind-mounted verbatim:
│   ├── kms.yaml                — gRPC-only remote signer, 2 secp256k1eth keys
│   ├── attestor-41001.yml      — standalone attestor for chain A
│   ├── attestor-41002.yml      — standalone attestor for chain B
│   └── deploy.yml              — one-shot deployer, the only local key
└── chains/
    ├── besu.toml.tmpl          — rendered once per chain
    ├── el-genesis.json.tmpl    — ${CHAIN_ID}, ${QBFT_EXTRADATA}, ${GENESIS_ALLOC}
    └── local/                  — generated, gitignored:
        ├── chains.env          — addresses, deployer keys, chain IDs, RPC URLs
        ├── ibc.env             — router addresses, written by the deploy phase
        ├── kms/keys/*.hex      — the two attestor keys kms serves
        ├── deploy/keys/        — the two deployer keyfiles
        ├── deploy/deployments/ — one deployment manifest per chain
        ├── A/{besu.toml, el-genesis.json, key}
        └── B/{besu.toml, el-genesis.json, key}
```

The `config/*.yml` placeholders (`${A_ICS26_ROUTER}`) are expanded by the `ibc`
binary itself at config load, not by `render_template` — which is why these are
plain committed files rather than `.tmpl` files under `chains/`.
