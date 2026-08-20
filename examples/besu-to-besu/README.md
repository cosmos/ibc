<!-- SPDX-License-Identifier: Apache-2.0 -->

# besu-to-besu


Two independent single-validator Besu QBFT chains (A and B), the substrate for a
single IBC pair, plus the IBC Link services that relay between them:

```
besu-a ◀──┐                                    ┌──▶ besu-b
          │  attestor-a          attestor-b    │
          └──   (41001)            (41002)   ──┘
                    │                  │
                    ├──── ibc-link ────┤        relayer, no attestation key
                    │                  │
                    └────── kms ───────┘        every signing key lives here
```

Each attestor is a **standalone external process**: its own container, its own
config, reached by the relayer over gRPC as a `type: remote` attestor. The
relayer never loads an attestor config and cannot produce an attestation itself,
which is the difference from dual mode (one process running the relayer and a
`type: local` attestor together).

No signing key is on disk in any attestor or relayer container.
[cosmos/kms](https://github.com/cosmos/kms) runs in gRPC-only mode — no chains,
no validators, no privval dial-out — and serves all four keys over its
SignerService. Each service addresses the key it is allowed to use by id
(`type: remote`, `remoteKeyId:`), and the private key never leaves kms.

## Prerequisites

`docker` (with the compose v2 plugin), `curl`, `perl`.

`cast` (foundry) does the BIP-39 derivation. If it isn't on `PATH`, `cast_cli`
falls back to running it inside `$FOUNDRY_IMAGE`, so a host install is optional.

Nothing is built from source. The images are pulled from GHCR and are public, so
no `docker login` is needed:

| Service                          | Image        | Override    |
|----------------------------------|--------------|-------------|
| ibc-link, attestor-a/b, deployer | `ghcr.io/cosmos/ibc:main`     | `IBC_IMAGE` |
| kms                              | `ghcr.io/cosmos/kms:latest`   | `KMS_IMAGE` |
| besu-a, besu-b                   | `hyperledger/besu:25.4.0`     | `BESU_IMAGE` |

`IBC_IMAGE` must be built from a commit that has both the `deploy` command and
the unified top-level `attestors[]` list. Older tags fail late and obscurely —
`unknown command "deploy"` two phases in, or attestors restart-looping on `no
attestations provided`, because `attestor.attestations[]` entries carry no `type`
and no `grpc` and so cannot express a standalone external attestor. If you see
either, check the tag first. Rebuild one with `gh workflow run
ibc-link-build.yml --ref <branch>` — the image is tagged after the ref it runs
on.

To run local relayer or attestor changes, build the image and point `IBC_IMAGE`
at it:

```bash
docker build -t ibc-link:local --target target-builder ../../link
IBC_IMAGE=ibc-link:local ./setup.sh
```

## Usage

Run from `examples/besu-to-besu/`:

```bash
./setup.sh              # the demo: init + start + deploy + link
./setup.sh clean        # stop containers, remove volumes and chains/local/
```

Two commands, on purpose. The five phases always run together:

| Phase      | What it does                                                     |
|------------|-------------------------------------------------------------------|
| `init`     | derive every key, render the chain configs into `chains/local/`  |
| `start`    | `docker compose up` both chains, wait for RPC                    |
| `deploy`   | `ibc deploy core` + `client` on each chain (writing `link.env`), then GMP, an IFT token per chain, and the bridge |
| `link`     | `docker compose up` kms, both attestors, ibc-link                |
| `transfer` | mint IFT on A, send it to B, relay it, assert the balance moved  |

Use `docker compose` directly to poke at a running stack (`ps`, `logs -f
ibc-link`, `exec attestor-a /opt/ibc attestor info attestor-a --home /home/ibc`).

A bare `./setup.sh` runs all five phases and ends with half a token having
crossed from chain A to chain B.

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
chain's genesis; index 0 is the deployer, index 1 the validator.
Set `A_MNEMONIC` and `B_MNEMONIC` to different phrases if you want the chains
to have fully independent account sets instead.

## Signing keys

The init phase derives four keys from the same mnemonics and writes them to
`chains/local/kms/keys/<id>.hex`, one per `grpc.keys` entry in `link/kms.yaml`.
Every run prints them.

| kms key id   | Source                     | Used by    | Needs gas |
|--------------|----------------------------|------------|-----------|
| `relayer-a`  | `A_MNEMONIC` index 2       | ibc-link   | yes, on A |
| `relayer-b`  | `B_MNEMONIC` index 2       | ibc-link   | yes, on B |
| `attestor-a` | `A_MNEMONIC` index 3       | attestor-a | no        |
| `attestor-b` | `B_MNEMONIC` index 4       | attestor-b | no        |

Two relayer keys rather than one shared key because `A_MNEMONIC` and
`B_MNEMONIC` are independently overridable: one key would only be funded on
chain A the moment they differ. The attestors sit at *different* indices for the
opposite reason — the two mnemonics default to the same phrase, and a shared
index would give two supposedly independent attestors one address.

All four are `algorithm: secp256k1eth` in `link/kms.yaml`, not `secp256k1`. Only
that scheme signs a pre-hashed 32-byte digest and returns the 65-byte
recoverable signature `AttestationLightClient.sol` recovers an attestor address
from; link rejects any other scheme at startup.

The keys are written mode `0644`, unlike the `0600` Besu validator keys, because
the kms image runs unprivileged (uid 10001) and could not otherwise read them
through the bind mount on Linux. Every one is a public BIP-39 test-vector key.

## Deploying

The deploy phase puts IBC on both chains through the one-shot `deployer`
compose service — the same `ibc` image with [link/deploy.yml](link/deploy.yml)
mounted, run as `docker compose run --rm deployer`. Running it in a container
means no Go toolchain on the host, and `besu-a` / `besu-b` resolve exactly as
they do for the real services. Per chain:

```
ibc keys import ecdsa deployer-<a|b>   # index 0 of that chain's mnemonic
ibc deploy core   --chain <id>
ibc deploy client --chain <id> --counterparty-chain <other> \
                  --attestors <counterparty's attestor> --threshold 1
```

A client on chain A tracks chain B, so it verifies attestations *about* B and
authorizes **B's** attestor — the counterparty's, not its own. `threshold 1`
because each chain has exactly one attestor here.

The deployer is the one key that cannot live in kms: `ibc deploy` needs the raw
private key and rejects a `type: remote` signer outright. That is why
`link/deploy.yml` is separate from `link/ibc.yml` — it keeps the only local key
in the example out of the relayer's config and out of the relayer's process.

Deployment writes a manifest per chain to `chains/local/deploy/deployments/`,
and `deploy` then writes `chains/local/link.env`. Only the router addresses are
read back out of the manifests (`.core.router`); the client id is passed to
`deploy client` explicitly, so it is known before anything runs and needs no
parsing.

All four values are required to start, and each is missed at a different stage:

| Symptom on startup                              | Cause                        |
|-------------------------------------------------|------------------------------|
| `.clientId required`                            | `*_CLIENT_ID` empty          |
| `invalid ics26 router address "" for chain <id>` | `*_ICS26_ROUTER` empty      |
| `no contract code at given address`             | router set but not deployed  |

Re-running `./setup.sh` against a live stack is safe: an already-imported
deployer key is skipped, and `ibc deploy` skips steps whose artefacts already
exist, so the same addresses come back out. `./setup.sh clean` resets everything.

## Relaying an IFT transfer

The last phase is the end-to-end assertion for everything above it. A balance
only moves on chain B if both attestors signed through kms and the relayer
assembled their attestations into a proof the light client accepted.

```
ibc deploy gmp    --chain <id>                       # IFT rides on ICS27-GMP
ibc deploy ift    --chain <id> --name --symbol       # one token per chain
ibc deploy ift-bridge --chain-a A --ift-a … --chain-b B --ift-b … --client-id …
ibc tx ift mint   --chain A --ift … --from deployer-a --to <sender>   --amount 1e18
ibc tx ift send   --chain A --ift … --from deployer-a --to <receiver> --amount 5e17 \
                  --client-id link-41001-41002
ibc relayer relay --tx-hash <the send tx> --chain-id A
ibc query ift balance --chain B --ift … --address <receiver>
```

`deploy gmp` is not optional — `deploy ift` refuses without it (`no gmp
deployment recorded for chain <id>`).

**The relay step is explicit.** `relayer.connections[].autoRelay` exists in the
config schema and validates, but nothing in `link` reads `.Enabled` or
`.Lookback` yet, so a packet sits unrelayed until it is handed to the relayer by
transaction hash. That is why `link/ibc.yml` carries no `autoRelay` block: it
would only imply a behaviour that is not wired up. `relayer relay` runs inside
the `ibc-link` container, since the command dials the relayer's own gRPC and
only its config describes it.

`tx ift` and `deploy` both need the raw private key, so both run as the
`deployer` service against `link/deploy.yml`. The transfer defaults are
overridable: `IFT_NAME`, `IFT_SYMBOL`, `IFT_MINT_AMOUNT`, `IFT_SEND_AMOUNT`,
`IFT_RELAY_TIMEOUT` (120s), `IFT_POLL_INTERVAL` (3s).

## Ports

Both containers use the same internal ports (8545 RPC, 8546 WS, 9545 metrics).
Only the host-side mappings differ:

| Service | RPC (host) | WS (host) | Metrics (host) |
|---------|-----------:|----------:|---------------:|
| besu-a  |       8545 |      8546 |           9545 |
| besu-b  |       8745 |      8746 |           9745 |

The link services all listen on 3000 internally:

| Service    | gRPC (host) |
|------------|------------:|
| ibc-link   |        3000 |
| attestor-a |        3010 |
| attestor-b |        3011 |
| kms        |  not published |

Every host mapping binds to `127.0.0.1` — the JSON-RPC endpoints are
unauthenticated and each node holds the key that signs every block on its chain,
so they stay off the LAN. Container-to-container traffic goes over the
`besu-besu-net` compose network and does not depend on these mappings.

`kms` is deliberately absent from that table. It serves plaintext gRPC and
performs no caller authentication or authorization at all, so anything that can
reach it can sign with any of the four keys. It stays on the compose network
only; use `docker compose exec kms ...` to inspect it. A real deployment sets
`tls_cert` / `tls_key` in `link/kms.yaml` and puts network controls in front.

## Layout

```
examples/besu-to-besu/
├── README.md
├── setup.sh                    — entrypoint: init | start | link | accounts |
│                                 signers | status | clean
├── docker-compose.yml          — besu-a, besu-b, kms, attestor-a, attestor-b,
│                                 ibc-link
├── lib/
│   ├── common.sh               — logging, prerequisite checks, RPC waiter,
│   │                             render_template, cast_cli
│   ├── chains.sh               — derivation, QBFT extraData, rendering, start /
│   │                             wait / status / clean
│   └── link.sh                 — kms key derivation, deployment, link.env
├── link/                       — committed, no secrets. Bind-mounted verbatim:
│   ├── kms.yaml                — gRPC-only remote signer, 4 secp256k1eth keys
│   ├── attestor-a.yml          — standalone attestor for chain A
│   ├── attestor-b.yml          — standalone attestor for chain B
│   ├── ibc.yml                 — relayer: both attestors as type: remote
│   └── deploy.yml              — one-shot deployer, the only local key
└── chains/
    ├── besu.toml.tmpl          — rendered once per chain
    ├── el-genesis.json.tmpl    — ${CHAIN_ID}, ${QBFT_EXTRADATA}, ${GENESIS_ALLOC}
    └── local/                  — generated, gitignored:
        ├── chains.env          — addresses, deployer keys, chain IDs, RPC URLs
        ├── link.env            — router addresses and client ids, written by
        │                         the deploy phase
        ├── kms/keys/*.hex      — the four signing keys kms serves
        ├── deploy/keys/        — the two deployer keyfiles
        ├── deploy/deployments/ — one deployment manifest per chain
        ├── A/{besu.toml, el-genesis.json, key}
        └── B/{besu.toml, el-genesis.json, key}
```

The `link/*.yml` placeholders are expanded by link itself (`os.ExpandEnv` at
config load), not by `render_template` — which is why these are plain committed
files rather than `.tmpl` files under `chains/`.
