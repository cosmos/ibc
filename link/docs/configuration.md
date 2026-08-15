<!-- SPDX-License-Identifier: Apache-2.0 -->

# Configuration reference

`ibc` reads a single YAML file (filename specified via `--config`, relative to `--home`; default
`~/.ibc/ibc.yml`). Env vars are expanded before parsing (`os.ExpandEnv`), so
`${VAR}` works anywhere in the file. Six top-level keys:

| Key         | Used by            | Purpose                                                       |
|-------------|--------------------|---------------------------------------------------------------|
| `server`    | relayer, attestor  | gRPC/HTTP listener address                                    |
| `db`        | relayer            | sqlite or postgres connection                                 |
| `chains`    | relayer, attestor  | chain configs common to both relayer and attestor             |
| `relayer`   | relayer            | connections to actively relay                                 |
| `attestors` | relayer, attestor  | attestors - local and remote                              |
| `signers`   | relayer, attestor  | signing backends referenced by client ends and local attestors |

Running the relayer with at least one `type: local` entry in `attestors` runs an attestor instance in-process ("dual mode").

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
chain used elsewhere in the config here first. `relayer.connections[].clientA/clientB.chainId`
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
| `dispatchPollInterval` | duration | How often the dispatcher polls storage for dispatchable packets. Defaults to 5s. |
| `chainOverrides`       | list     | Per-chain relaying overrides (see below).                                      |
| `connections`          | list     | Bidirectional connections to actively relay (see below).                       |

### `relayer.chainOverrides[]`

| Field                | Type     | Description |
|----------------------|----------|--------------|
| `chainId`            | string   | Must match a declared chain. |
| `txSubmissionDelay`  | duration | Minimum delay between tx submissions on this chain. |
| `packetBatchSize`    | int      | Max packets to batch into one recv/ack/timeout tx. |
| `packetBatchTimeout` | duration | Max time to wait for a batch to fill before flushing it anyway. |
| `evm`                | object   | `gasFeeCapMultiplier`, `gasTipCapMultiplier` — multipliers applied to the chain's suggested EIP-1559 fee cap/tip. |

### `relayer.connections[]`

One entry per IBC connection the relayer actively relays, in both
directions. Each connection has two client ends, `clientA` and `clientB`;
each end's counterparty is simply the connection's other end.

| Field     | Type   | Description                        |
|-----------|--------|-------------------------------------|
| `alias`   | string | Unique handle for this connection.  |
| `clientA` | object | One client end. See below.          |
| `clientB` | object | The other client end. See below.    |

#### `relayer.connections[].clientA` / `.clientB`

| Field         | Type   | Description |
|---------------|--------|--------------|
| `chainId`     | string | The chain this client end is registered on. |
| `signer`      | string | Signer submitting relay transactions on `chainId` — this end's own chain. Must match a `signers[].alias`. |
| `clientId`    | string | This end's on-chain client ID, on `chainId`. |
| `type`        | string | Only `attestation` is currently supported. |
| `autoRelay`   | object | `enabled` (bool), `lookback` (uint) — auto-relay settings for packets flowing FROM this end's chain TOWARD the counterparty end. |

`clientA` and `clientB` must be on different chains.

```yaml
relayer:
  connections:
    - alias: "eth-base"
      clientA:
        chainId: "1"
        signer: "relayer-key"
        clientId: "base-0"
        type: "attestation"
        autoRelay:
          enabled: true
          lookback: 100
      clientB:
        chainId: "8453"
        signer: "relayer-key"
        clientId: "ethereum-0"
        type: "attestation"
        autoRelay:
          enabled: true
          lookback: 100
```

`ibc config validate --live` additionally confirms each connection's
on-chain registered counterparty matches `clientA`/`clientB`, and runs the
same attestor-quorum resolution described above.

---

## `attestors`

Top-level list of attestors — both the ones this
process runs itself (`type: local`) and the ones it queries over gRPC
(`type: remote`).

| Field            | Type   | Description |
|------------------|--------|--------------|
| `name`           | string | Identifies the attestor. |
| `chainId`        | string | `local` only — which declared chain this attestor watches. Not set for `type: remote`; discovered live via its `Info` RPC instead. |
| `type`           | string | `local` or `remote`. |
| `signer`         | string | `local` only. Must reference a `signers[].alias`. |
| `finalityOffset` | uint   | `local` only. `0` (default): attest up to the chain's `"finalized"` RPC tag. `n > 0`: attest up to `"latest" - n` instead. |
| `grpc`           | string | `remote` only. Bare `host:port` (not a URL — a `://` here is rejected at validation). |

```yaml
attestors:
  - name: "eth-watcher"
    chainId: "1"
    type: local
    signer: "my-local-signer"
    finalityOffset: 1
  - name: "base-watcher"
    type: remote
    grpc: attestor.example.com:3000
```

---

## `signers`

Signing backends, referenced by alias from `relayer.connections[].clientA/clientB.signer`
and `attestors[].signer` (for `type: local` attestors). Each needs a unique `alias`.

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
- attestor sets — `--attestors` values may be attestor names, signer
  aliases, or raw addresses; aliases resolve through local key files (remote
  signers error — pass their address directly), and any value matching no
  alias passes through as an address, validated by the chain's deployment
  driver. When the flag is omitted,
  the set for a client tracking chain X defaults to every
  `attestors[]` entry with `chainId: X`. Reusing one attestor
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

