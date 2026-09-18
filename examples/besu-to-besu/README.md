<!-- SPDX-License-Identifier: Apache-2.0 -->

# besu-to-besu

Two independent single-validator Besu QBFT chains and the IBC services that move
a token between them, with every signature produced by a remote signer. One
command brings up the chains, deploys IBC on both, and relays a transfer from A
to B.

```
        chain A (41001)                     chain B (41002)

        ┌────────────┐                      ┌────────────┐
        │   besu-a   │                      │   besu-b   │
        └─────┬──────┘                      └─────┬──────┘
              │ watches                   watches │
              ▼                                   ▼
        ┌────────────┐                      ┌────────────┐
        │ attestor-a │                      │ attestor-b │
        └─────┬──────┘                      └─────┬──────┘
              │                                   │
              └──────────▶┌───────────┐◀──────────┘
                          │  relayer  │
                          └─────┬─────┘
                                │
              ┌─────────────────┴─────────────────┐
              │  Sign (gRPC)                      │
              ▼                                   │
        ┌───────────┐                             │
        │    kms    │◀────────────────────────────┘
        └───────────┘   attestor-a, attestor-b,
                        relayer-a, relayer-b
```

Each attestor watches one chain and signs attestations about it, nothing else.
The relayer is the only process wired to everything: both attestors over gRPC,
and an RPC *and* websocket connection to both chains. A transfer from A to B
goes:

1. `ift send` on chain A burns 0.5 DEMO and emits a packet. Nothing hands that
   packet over — the relayer is subscribed to chain A's `SendPacket` logs and
   picks it up itself.
2. The relayer pulls an attestation over chain A's state from attestor-a.
3. The relayer submits the packet to chain B with that attestation as its proof.
   Chain B's light client verifies it against the attestor it authorizes —
   attestor-a, the counterparty's, not its own.

**None of those signatures is produced locally.** Both attestors and the relayer
are configured with `type: remote` signers pointing at
[cosmos/kms](https://github.com/cosmos/kms), which holds all four keys and
serves them over gRPC.

## Quick start

Needs `docker` (with the compose v2 plugin), `curl`, and `perl`. Everything else
is pulled from public GHCR images — nothing is built, no `docker login`. `cast`
(foundry) does the BIP-39 derivation; if it isn't on `PATH`, `cast_cli` runs it
inside `$FOUNDRY_IMAGE` instead, so a host install is optional.

```bash
cd examples/besu-to-besu
./setup.sh
```

Takes about two minutes on a warm cache, and ends with:

```
[14:14:15] Waiting for the relayer to subscribe to SendPacket on both chains...
[14:14:15] Relayer is watching chain A and chain B.
[14:14:17] [A] minting 1000000000000000000 DEMO to 0x58A57ed9...
[14:14:20] [A] sending 500000000000000000 DEMO to 0x58A57ed9... on chain B over ibc-41001-41002...
[14:14:22]       sent in 0xbc60671b3e081d770e8704f0e6c4c25ba9fe0bfeae0a1d99418dd17dc1a5f146
[14:14:22]       waiting for the relayer to deliver it — chain B's balance 0 -> 500000000000000000...
[14:14:38]       relayed A -> B: 0x58A57ed9... holds 500000000000000000 DEMO on chain B
[14:14:40] DEMO held by the deployer: chain A 1500000000000000000, chain B 500000000000000000
```

That `relayed` line is the whole point, and nothing asked for it: the send
returns a transaction hash the script only ever prints. The balance on chain B
moves because the relayer saw the packet on its own subscription, attestor-a
signed through kms, and the relayer assembled that attestation into a proof
chain B's light client accepted.

`roundtrip` sends the same tokens straight back afterwards. The return leg needs
no mint — it spends what the first leg delivered — so chain B ends where it
started and chain A ends holding everything it minted:

```bash
./setup.sh roundtrip    # phase 4 alone when the stack is already up
```

```
[14:13:27]       relayed A -> B: 0x58A57ed9... holds 500000000000000000 DEMO on chain B
[14:13:29] [B] sending 500000000000000000 DEMO to 0x58A57ed9... on chain A over ibc-41001-41002...
[14:13:48]       relayed B -> A: 0x58A57ed9... holds 1000000000000000000 DEMO on chain A
[14:13:50] DEMO held by the deployer: chain A 1000000000000000000, chain B 0
```

Worth running at least once: A → B only proves attestor-a and chain B's client.
The return leg is what exercises attestor-b and chain A's client, so a one-way
demo leaves half the stack unverified.

`roundtrip` is also the command for iterating on the relay itself. It checks
whether the stack is usable — all four values in `ibc.env`, both IFT tokens in
the manifests, and all six containers healthy — and if so runs phase 4 alone, in
about 45 seconds instead of two minutes. If any of that is missing or the
containers are stopped, it brings the stack up first.

```bash
./setup.sh clean        # stop containers and remove chains/local/
```

Three commands, on purpose. Re-running any of them against a live stack is safe
and idempotent — every deploy step re-checks on-chain state and reports
`skipped` — so it relays another transfer rather than rebuilding anything.

Each invocation writes a timestamped log to `logs/`. Use `docker compose`
directly to poke at a running stack:

```bash
docker compose ps
docker compose logs -f relayer
docker compose exec attestor-a /opt/ibc attestor info attestor-a --home /home/ibc
```

## The four phases

They always run together; the names are internal, not subcommands.

| Phase      | What it does                                                     |
|------------|------------------------------------------------------------------|
| `start`    | derive every key, render the chain configs into `chains/local/`, `docker compose up` both chains, wait for RPC |
| `deploy`   | `ibc deploy core` + `client` on each chain (writing `ibc.env`), then GMP, an IFT token per chain, and the bridge |
| `services` | `docker compose up` kms, both attestors, relayer                 |
| `transfer` | wait until the relayer is subscribed on both chains, mint IFT on A, send it to B, assert the balance moved — and with `roundtrip`, send it back |

## What makes this example different

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

Relaying is **hands-off**. Both ends of the connection set
`autoRelay.enabled: true`, so the relayer discovers packets on its own
`SendPacket` websocket subscription rather than being handed a transaction
hash. The demo never calls `ibc relayer relay`; see
[Relaying an IFT transfer](#relaying-an-ift-transfer).

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

Every key here is derived from a public BIP-39 test vector and printed in the
logs. Local devnet only — never send real funds to any address this prints.

## Signing keys

The init phase derives four keys from the same mnemonics and writes them to
`chains/local/kms/keys/<id>.hex`, one per `grpc.keys` entry in `config/kms.yaml`.
Every run prints them.

| kms key id   | Source                     | Used by    | Needs gas |
|--------------|----------------------------|------------|-----------|
| `relayer-a`  | `A_MNEMONIC` index 2       | relayer    | yes, on A |
| `relayer-b`  | `B_MNEMONIC` index 2       | relayer    | yes, on B |
| `attestor-a` | `A_MNEMONIC` index 3       | attestor-a | no        |
| `attestor-b` | `B_MNEMONIC` index 4       | attestor-b | no        |

Two relayer keys rather than one shared key because `A_MNEMONIC` and
`B_MNEMONIC` are independently overridable: one key would only be funded on
chain A the moment they differ. The attestors sit at *different* indices for the
opposite reason — the two mnemonics default to the same phrase, and a shared
index would give two supposedly independent attestors one address.

All four are `algorithm: secp256k1eth` in `config/kms.yaml`, not `secp256k1`. Only
that scheme signs a pre-hashed 32-byte digest and returns the 65-byte
recoverable signature `AttestationLightClient.sol` recovers an attestor address
from; the attestor rejects any other scheme at startup.

The keys are written mode `0644`, unlike the `0600` Besu validator keys, because
the kms image runs unprivileged (uid 10001) and could not otherwise read them
through the bind mount on Linux.

## Deploying

The deploy phase puts IBC on both chains through the one-shot `deployer`
compose service — the same `ibc` image with [config/deploy.yml](config/deploy.yml)
mounted. Running it in a container means no Go toolchain on the host, and
`besu-a` / `besu-b` resolve exactly as they do for the real services. Per chain:

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
private key and rejects a `type: remote` signer outright (`deployer signer %q
must be a local key`).

Deployment writes a manifest per chain to `chains/local/deploy/deployments/`,
and `deploy` then writes `chains/local/ibc.env`. Two things are read back out of
those manifests: the router address (`.core.router`), which goes into `ibc.env`,
and each chain's IFT address (the `.tokens[]` entry matching `IFT_SYMBOL`),
which every command in the transfer phase needs for `--ift`. The client id is
not among them — it is passed to `deploy client` explicitly, so it is known
before anything runs and needs no parsing.

To run an `ibc` command by hand against the same config, pass the uid and gid
`setup.sh` exports — the service writes to host-owned bind mounts, and the
compose default of `65532` cannot on Linux:

```bash
DEPLOY_UID=$(id -u) DEPLOY_GID=$(id -g) \
  docker compose run --rm deployer query ift balance --chain 41002 \
    --ift <address> --address <holder> --home /home/ibc
```

## Relaying an IFT transfer

The last phase is the end-to-end assertion for everything above it, and this is
what it runs:

```
ibc deploy gmp    --chain <id>                       # IFT rides on ICS27-GMP
ibc deploy ift    --chain <id> --name --symbol       # one token per chain
ibc deploy ift-bridge --chain-a A --ift-a … --chain-b B --ift-b … --client-id …
ibc tx ift mint   --chain A --ift … --from deployer-a --to <sender>   --amount 1e18
ibc tx ift send   --chain A --ift … --from deployer-a --to <receiver> --amount 5e17 \
                  --client-id ibc-41001-41002
ibc query ift balance --chain B --ift … --address <receiver>
```

`roundtrip` then repeats the last two with A and B swapped, and no mint.

`deploy gmp` is not optional — `deploy ift` refuses without it (`no gmp
deployment recorded for chain <id>`).

**There is no relay step.** That list has no `relayer relay` in it because both
ends of the connection in [config/ibc.yml](config/ibc.yml) set
`autoRelay.enabled: true`. That is what puts a packet watcher on the chain: it
subscribes to `SendPacket` logs for the client ids it relays, records every
matching packet, and the dispatch loop collects them within
`dispatchPollInterval` (3s here). `autoRelay` is set per *end* and covers
packets flowing **from** that end's chain, so enabling it on both ends is what
makes A → B and B → A both automatic.

That subscription is why each chain carries `evm.ws` next to `evm.rpc`.
Enabling `autoRelay` without a websocket endpoint is a config error rather than
a silent fallback to polling:

```
relayer.connections[0].clientA.autoRelay: requires chains[41001].evm.ws
```

**The relayer has to already be listening when the packet is sent.** A watcher
subscribes from wherever its chain is at that moment and never looks backwards,
so a packet emitted while the relayer was down is not discovered at all. The
transfer phase therefore waits for both subscriptions — the two `Subscribed to
send packets` lines in the relayer's log — before it mints anything. It does
not just trust the compose healthcheck, which only proves port 3000 is open,
and the relayer opens that port before it starts its watchers.

`ibc relayer relay` has not gone away; it is now the escape hatch rather than
the happy path, for a packet that predates the subscription or one on a route
with `autoRelay` off. It runs inside the `relayer` container, since the command
dials the relayer's own gRPC and only its config describes it:

```bash
docker compose exec relayer /opt/ibc relayer relay \
  --tx-hash <the send tx> --chain-id 41001 --home /home/ibc
```

The balance wait is on a *delta*, not an absolute value — a second run starts
with the first run's tokens already in place, and an absolute check would pass
before the new packet ever landed.

Direction is a parameter to the same helper, because the two legs prove
different halves of the stack: A → B rests on attestor-a and chain B's client,
B → A on attestor-b and chain A's client. Both directions were registered back
in the deploy phase — `deploy client` runs on each chain authorizing the
counterparty's attestor, and `deploy ift-bridge` registers a bridge on both
tokens — so the return leg needs no extra setup.

`tx ift` and `deploy` both need the raw private key, so both run as the
`deployer` service against `config/deploy.yml`. The transfer defaults are
overridable: `IFT_NAME`, `IFT_SYMBOL`, `IFT_MINT_AMOUNT`, `IFT_SEND_AMOUNT`,
`IFT_RELAY_TIMEOUT` (120), `IFT_POLL_INTERVAL` (3), and
`WATCHER_READY_TIMEOUT` (60) for the subscription wait. The last three are
whole seconds with no unit suffix — `3`, not `3s`. Changing `IFT_NAME` or
`IFT_SYMBOL` against a stack that already has a token mints a second one rather
than replacing the first; run `./setup.sh clean` first.

## Ports

Both chains use the same internal ports (8545 RPC, 8546 WS, 9545 metrics). Only
the host-side mappings differ:

| Service | RPC (host) | WS (host) | Metrics (host) |
|---------|-----------:|----------:|---------------:|
| besu-a  |       8545 |      8546 |           9545 |
| besu-b  |       8745 |      8746 |           9745 |

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
  http://localhost:8745   # → 0xa02a  (= 41002, chain B)
```

Those WS mappings are for host-side tooling; the relayer never uses them. It
reaches the same endpoints across the compose network as `ws://besu-a:8546` and
`ws://besu-b:8546`, which is what `chains[].evm.ws` in `config/ibc.yml` points
at and what auto-relay needs. Both are already enabled in `besu.toml.tmpl`
(`rpc-ws-enabled = true`), so nothing extra has to be turned on.

The IBC services all listen on 3000 internally:

| Service    | gRPC (host) |
|------------|------------:|
| relayer    |        3000 |
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
`tls_cert` / `tls_key` in `config/kms.yaml` and puts network controls in front.

## Configuration

| Variable | Default | Effect |
|----------|---------|--------|
| `IBC_IMAGE` | `ghcr.io/cosmos/ibc:main` | relayer, attestors, deployer |
| `KMS_IMAGE` | `ghcr.io/cosmos/kms:latest` | the remote signer |
| `BESU_IMAGE` | `hyperledger/besu:25.4.0` | both chains |
| `A_MNEMONIC` / `B_MNEMONIC` | a public test vector | every account on that chain |
| `FUNDED_ACCOUNTS` | `5` | accounts pre-funded in each genesis |
| `GENESIS_BALANCE` | 1 000 000 ETH | hex wei per funded account |
| `IFT_*` | see above | the token and the transfer |
| `WATCHER_READY_TIMEOUT` | `60` | seconds to wait for the relayer's subscriptions |
| `QBFT_BLOCK_PERIOD_SECONDS` etc. | `2` | consensus tunables |

Changing any chain-shaping variable invalidates the on-disk chain data: run
`./setup.sh clean` first, or Besu refuses to start against a genesis that no
longer matches its database.

To run local relayer or attestor changes, build the image and point `IBC_IMAGE`
at it:

```bash
docker build -t ibc:local --target target-builder ../../cli
IBC_IMAGE=ibc:local ./setup.sh
```

## Troubleshooting

**`unknown command "deploy"`, or attestors restart-looping on `no attestations
provided`.** `IBC_IMAGE` is too old. It must come from a commit that has both
the `deploy` command and the unified top-level `attestors[]` list — older
`attestor.attestations[]` entries carry no `type` and no `grpc`, so they cannot
express a standalone external attestor. Check the tag first; rebuild one with
`gh workflow run ibc-cli-build.yml --ref <branch>`, which tags the image after
the ref it runs on.

**An IBC service exits at startup.** All four values in `chains/local/ibc.env`
are required, and each is missed at a different stage:

| Symptom on startup                               | Cause                       |
|--------------------------------------------------|-----------------------------|
| `.clientId required`                             | `*_CLIENT_ID` empty         |
| `invalid ics26 router address "" for chain <id>` | `*_ICS26_ROUTER` empty      |
| `no contract code at given address`              | router set but not deployed |

**The relayer exits with `relayer.connections[0].clientA.autoRelay: requires
chains[41001].evm.ws`.** An `autoRelay` end whose chain has no websocket
endpoint. Both chains in `config/ibc.yml` need `evm.ws` alongside `evm.rpc`;
auto-relay has no polling fallback.

**`the relayer did not subscribe on both chains within 60s`.** The container is
healthy but its watchers never came up, so `docker compose logs relayer` and
look for a websocket dial failure against `besu-a:8546` / `besu-b:8546`. A
chain that is up on RPC can still be refusing WS if `besu.toml` was edited.

**The transfer times out.** First ask whether the relayer ever saw the packet:

```bash
docker compose exec relayer /opt/ibc relayer packets --tx-hash <hash> --home /home/ibc
```

No row at all means the watcher missed the `SendPacket` — most likely the
packet was emitted before the subscription was live, which is exactly what the
`WATCHER_READY_TIMEOUT` wait exists to prevent. A `pending` row means discovery
worked and delivery did not, so the problem is downstream:
`docker compose logs relayer attestor-a attestor-b`. An attestor that cannot
reach kms, or a client authorizing the wrong attestor address, both surface
there.


## Layout

```
examples/besu-to-besu/
├── README.md
├── setup.sh                    — entrypoint: the demo, or `clean`
├── docker-compose.yml          — besu-a, besu-b, kms, attestor-a, attestor-b,
│                                 relayer, deployer (profile: tools)
├── lib/
│   ├── common.sh               — logging, prerequisite checks, image pull,
│   │                             RPC waiter, render_template, cast_cli
│   ├── chains.sh               — derivation, QBFT extraData, rendering, start /
│   │                             wait / status / clean
│   └── ibc.sh                  — kms key derivation, deployment, ibc.env,
│                                 the watcher wait, the IFT transfer
├── config/                     — committed, no secrets. Bind-mounted verbatim:
│   ├── kms.yaml                — gRPC-only remote signer, 4 secp256k1eth keys
│   ├── attestor-a.yml          — standalone attestor for chain A
│   ├── attestor-b.yml          — standalone attestor for chain B
│   ├── ibc.yml                 — relayer: both attestors as type: remote,
│   │                             autoRelay + evm.ws on both connection ends
│   └── deploy.yml              — one-shot deployer, the only local key
└── chains/
    ├── besu.toml.tmpl          — rendered once per chain
    ├── el-genesis.json.tmpl    — ${CHAIN_ID}, ${QBFT_EXTRADATA}, ${GENESIS_ALLOC}
    └── local/                  — generated, gitignored:
        ├── chains.env          — addresses, deployer keys, chain IDs, RPC URLs
        ├── ibc.env             — router addresses and client ids, written by
        │                         the deploy phase
        ├── kms/keys/*.hex      — the four signing keys kms serves
        ├── deploy/keys/        — the two deployer keyfiles
        ├── deploy/deployments/ — one deployment manifest per chain
        ├── data/{A,B}/         — each chain's Besu database
        ├── relayer/            — the relayer's ibc.db and its wal, mounted at
        │                         /data (db.url in config/ibc.yml)
        ├── A/{besu.toml, el-genesis.json, key}
        └── B/{besu.toml, el-genesis.json, key}
```


The `config/*.yml` placeholders are expanded by the `ibc` binary itself (`os.ExpandEnv` at
config load), not by `render_template` — which is why these are plain committed
files rather than `.tmpl` files under `chains/`.
