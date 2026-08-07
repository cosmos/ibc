# Configuration reference

`ibc` reads a single YAML file (filename specified via `--config`, relative to `--home`; default
`~/.ibc/ibc.yml`). Env vars are expanded before parsing (`os.ExpandEnv`), so
`${VAR}` works anywhere in the file. Six top-level keys:

| Key        | Used by            | Purpose                                                         |
|------------|--------------------|-----------------------------------------------------------------|
| `server`   | relayer, attestor  | gRPC/HTTP listener address                                      |
| `db`       | relayer            | sqlite or postgres connection                                   |
| `chains`   | relayer, attestor  | chain configs common to both relayer and attestor               |
| `relayer`  | relayer            | configures routes to relay                                      |
| `attestor` | attestor           | which chains to attest and with which signer                    |
| `signers`  | relayer, attestor  | signing backends referenced in the relayer and attestor configs |

Running the relayer with the `attestor` block populated runs an attestor instance in-process.

---

## `server`

| Field        | Type   | Description |
|--------------|--------|-------------|
| `listenAddr` | string | Address the gRPC/HTTP server binds to (e.g. `0.0.0.0:3000`). Serves the relayer/attestor API over gRPC, gRPC-Web, and Connect on the same port, with reflection always on. |

```yaml
server:
  listenAddr: 0.0.0.0:3000
```

## `db`

| Field  | Type   | Description |
|--------|--------|-------------|
| `type` | string | `sqlite` or `postgres` |
| `url`  | string | sqlite: a file path. postgres: a `postgres://` connection string. |

```yaml
db:
  type: sqlite
  url: ibc.db
```

## `chains`

Chains the attestor and relayer can reference by `chainId`. Declare every
chain used elsewhere in the config here first. `relayer.clients[].chainId`
and `relayer.chainOverrides[].chainId` are validated against this list.

| Field      | Type   | Description |
|------------|--------|-------------|
| `chainId`  | string | Unique chain identifier (e.g. `"11155111"` for an EVM chain ID). |
| `evm`      | object | EVM-specific connection details. Currently the only supported chain type. |
| `deployer` | string | Optional. Signer alias (from `signers`) used by `ibc deploy` to sign deployment transactions on this chain. Must be a local ECDSA signer. |

### `chains[].evm`

| Field         | Type   | Description |
|---------------|--------|-------------|
| `rpc`         | string | HTTP(S) JSON-RPC endpoint. |
| `ics26Router` | string | ICS26 router contract address, hex-encoded with `0x` prefix. |

```yaml
chains:
  - chainId: "1"
    evm:
      rpc: https://ethereum-rpc.example.com
      ics26Router: "0x0000000000000000000000000000000000000000"
```

---

## `relayer`

| Field                  | Type     | Description                                                                    |
|------------------------|----------|--------------------------------------------------------------------------------|
| `dispatchPollInterval` | duration | How often the dispatcher polls storage for unfinished packets. Defaults to 5s. |
| `chainOverrides`       | list     | Per-chain relaying overrides (see below).                                      |
| `clients`              | list     | Light clients to relay for (see below).                                        |
| `routesToRelay`        | list     | Which client pairs to actively relay, and with which signer (see below).       |

### `relayer.chainOverrides[]`

| Field                | Type     | Description |
|----------------------|----------|--------------|
| `chainId`            | string   | Must match a declared chain. |
| `txSubmissionDelay`  | duration | Minimum delay between tx submissions on this chain. |
| `packetBatchSize`    | int      | Max packets to batch into one recv/ack/timeout tx. |
| `packetBatchTimeout` | duration | Max time to wait for a batch to fill before flushing it anyway. |
| `evm`                | object   | `gasFeeCapMultiplier`, `gasTipCapMultiplier` — multipliers applied to the chain's suggested EIP-1559 fee cap/tip. |

### `relayer.clients[]`

One entry per light client the relayer knows about. **Both sides of a
connection must be configured** — a client's counterparty must also appear
as its own entry, referencing back.

| Field                  | Type   | Description |
|------------------------|--------|--------------|
| `alias`                | string | Unique handle referenced by `routesToRelay`. |
| `clientId`             | string | This client's on-chain ID, on `chainId`. |
| `chainId`              | string | The chain this client is registered on. |
| `counterpartyChainId`  | string | The chain this client tracks. |
| `counterpartyClientId` | string | The client ID on `counterpartyChainId` that tracks `chainId` back. |
| `type`                 | string | Only `attestation` is currently supported. |
| `attestorSet`          | object | Required for `attestation` clients. See below. |

#### `relayer.clients[].attestorSet`

| Field                             | Type | Description                                                                                                                         |
|-----------------------------------|------|-------------------------------------------------------------------------------------------------------------------------------------|
| `counterpartyChainFinalityOffset` | uint | Blocks to wait on the *counterparty* chain before treating state final enough to relay. Should match attestor's finality offset. |
| `threshold`                       | int  | Minimum number of attestor signatures required.                                                                                     |
| `attestors`                       | list | See below.                                                                                                                          |

#### `relayer.clients[].attestorSet.attestors[]`

| Field  | Type   | Description |
|--------|--------|--------------|
| `name` | string | Must match an `attestor.attestations[].name` — either in this process's own `attestor` block (`type: local`) or on the remote attestor being queried (`type: remote`). |
| `type` | string | `local` or `remote`. |
| `grpc` | string | Required for `remote`. Bare `host:port` (not a URL — a `://` here is rejected at validation). |

```yaml
relayer:
  clients:
    - alias: "eth-to-base"
      clientId: "base-0"
      chainId: "1"
      counterpartyChainId: "8453"
      counterpartyClientId: "ethereum-0"
      type: "attestation"
      attestorSet:
        counterpartyChainFinalityOffset: 1
        threshold: 1
        attestors:
          - name: "attestor-base"   # watches chain 8453, this client's counterparty
            type: remote
            grpc: attestor.example.com:3000
```

### `relayer.routesToRelay[]`

Packets sent from `sourceClient` are relayed through the full packet
lifecycle (recv, ack, timeout) using the given signer aliases.

| Field              | Type   | Description |
|--------------------|--------|--------------|
| `sourceClient`      | string | Must match a `clients[].alias`. |
| `sourceSignerAlias` | string | Signer submitting txs on the source chain (e.g. the ack). |
| `destSignerAlias`   | string | Signer submitting txs on the destination chain (e.g. the recv). |

---

## `signers`

Signing backends, referenced by alias from `relayer.routesToRelay` and
`attestor.attestations`. Each needs a unique `alias`.

| Field         | Type   | Description |
|---------------|--------|-------------|
| `alias`       | string | Unique name referenced elsewhere in the config. |
| `type`        | string | `local` or `remote`. |
| `file`        | string | Required for `local`. Path to a keyfile (see `ibc keys new`/`ibc keys import`). Relative paths also try `<path>.json` and `keys/<path>` as fallbacks. |
| `grpc`        | string | Required for `remote`. gRPC address of a cosmos/KMS-compatible remote signer. |
| `remoteKeyId` | string | Required for `remote`. Key ID on the remote signer. |

```yaml
signers:
  - alias: "eth-relayer-key"
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: "eth-relayer-key-id"
  - alias: "my-local-signer"
    type: local
    file: keys/my-key.json
```

## `attestor`

Only needed when this process should attest — standalone (`ibc attestor
run`) or alongside a relayer in the same process (populate this
block *and* `relayer`, run `ibc relayer run`; `attestorSet.attestors[].type:
local` entries then resolve against this block directly instead of over
gRPC).

### `attestor.attestations[]`

Each `name` and `signer` must be unique across the list — the same signer
can't back two attestations in one process.

| Field            | Type   | Description                                                                                                                                                                                                       |
|------------------|--------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `chainId`        | string | Which declared chain this attestation watches.                                                                                                                                                                    |
| `name`           | string | Referenced by `relayer.clients[].attestorSet.attestors[].name` (for both remote and local).                                                                                                                       |
| `signer`         | string | Must reference a `signers[].alias`.                                                                                                                                                                               |
| `finalityOffset` | uint   | `0` (default): attest up to the chain's `"finalized"` RPC tag. `n > 0`: attest up to `"latest" - n` instead. Use this where `"finalized"` is slow or unsupported (e.g. Ethereum PoS lags ~12-15 min behind head). |

```yaml
attestor:
  attestations:
    - chainId: chain-a
      name: attestation-a
      signer: my-local-signer
```

## Deployment

`ibc deploy` provisions IBC on a chain and records what it deployed. Two
pieces tie into the rest of the config:

- `chains[].deployer` — the signer alias `ibc deploy` uses to sign
  deployment transactions on that chain. Must reference a `local` ECDSA
  signer in `signers` (deployment tooling needs the raw key, not just a
  remote signing call). Overridable per-invocation with `--deployer`.
  `deploy status` and `deploy render-config` are read-only and work without
  a configured deployer.
- `--manifest-dir` (default `deployments`, relative to `--home`) — where
  `ibc deploy` writes one JSON manifest per chain recording what was
  deployed (router address, registered clients). Manifests are
  machine-generated: `ibc deploy` reads and rewrites them on every run to
  stay idempotent, so hand edits are lost and can desync the recorded state
  from what's actually on chain.
- attestor sets — `--attestors` values may be attestation names, signer
  aliases, or raw addresses; aliases resolve through local key files (remote
  signers error — pass their address directly), and any value matching no
  alias passes through as an address, validated by the chain's deployment
  driver. When the flag is omitted,
  the set for a client tracking chain X defaults to every
  `attestor.attestations[]` entry with `chainId: X`. Reusing one attestor
  set for both directions of a connection is discouraged — configure
  distinct sets per tracked chain.

A connection is two mirrored `deploy client` invocations (one per chain,
each tracking the other). Both default to the same client id
(`link-<chainA>-<chainB>`; ids are per-chain namespaces, so the shared name
is unambiguous) and so pair up without explicit id flags. Rerunning with
the same id continues a completed or partially-deployed connection;
rerunning with different attestors/threshold under the same id fails —
deployed client parameters are constructor-fixed — with the differences
listed, as does a client registered on-chain but missing from the manifest
(a deployment interrupted before its record was written; the deployed
parameters cannot be recovered from chain). In either case, pass a new
`--client-id` (and matching `--counterparty-client-id`) to deploy a new
client pair.

