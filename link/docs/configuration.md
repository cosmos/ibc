# Configuration reference

`ibc` reads a single YAML file (filename specified via `--config`, relative to `--home`; default
`~/.ibc/ibc.yml`). Env vars are expanded before parsing (`os.ExpandEnv`), so
`${VAR}` works anywhere in the file. Six top-level keys:

| Key         | Used by            | Purpose                                                          |
|-------------|--------------------|-------------------------------------------------------------------|
| `server`    | relayer, attestor  | gRPC/HTTP listener address                                       |
| `db`        | relayer            | sqlite or postgres connection                                    |
| `chains`    | relayer, attestor  | chain configs common to both relayer and attestor                |
| `relayer`   | relayer            | connections to actively relay                                    |
| `attestors` | relayer, attestor  | every attestor in play — this process's own and remote ones      |
| `signers`   | relayer, attestor  | signing backends referenced by client ends and local attestors   |

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

| Field     | Type   | Description |
|-----------|--------|-------------|
| `chainId` | string | Unique chain identifier (e.g. `"11155111"` for an EVM chain ID). |
| `evm`     | object | EVM-specific connection details. Currently the only supported chain type. |

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
each end's counterparty is simply the connection's other end — there are no
separate counterparty fields to keep in sync, unlike the old `clients[]` +
`routesToRelay[]` shape.

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
| `attestorSet` | object | Required for `attestation` clients. See below. |
| `autoRelay`   | object | `enabled` (bool), `lookback` (uint) — auto-relay settings for packets flowing FROM this end's chain TOWARD the counterparty end. |

`clientA` and `clientB` must be on different chains.

#### `relayer.connections[].clientA/clientB.attestorSet`

| Field                             | Type            | Description |
|-----------------------------------|-----------------|--------------|
| `threshold`                       | int             | Minimum number of attestor signatures required. |
| `counterpartyChainFinalityOffset` | uint            | Blocks to wait on the *counterparty* chain before treating state final enough to relay. Should match the corresponding attestor's `finalityOffset`. |
| `attestors`                       | list of string  | Aliases into the top-level `attestors[]` list — not embedded objects. |

```yaml
relayer:
  connections:
    - alias: "eth-base"
      clientA:
        chainId: "1"
        signer: "relayer-key"
        clientId: "base-0"
        type: "attestation"
        attestorSet:
          threshold: 1
          counterpartyChainFinalityOffset: 1
          attestors: ["base-watcher"]   # watches chain 8453, this end's counterparty
        autoRelay:
          enabled: true
          lookback: 100
      clientB:
        chainId: "8453"
        signer: "relayer-key"
        clientId: "ethereum-0"
        type: "attestation"
        attestorSet:
          threshold: 1
          counterpartyChainFinalityOffset: 1
          attestors: ["eth-watcher"]    # watches chain 1, this end's counterparty
        autoRelay:
          enabled: true
          lookback: 100
```

`ibc config validate --live` additionally queries each connection's two
chains' routers to confirm the on-chain registered counterparty actually
matches `clientA`/`clientB`, in both directions — catching a config that
names two clients as counterparties when the chains themselves disagree.

---

## `attestors`

Unified, top-level list of every attestor in play — both the ones this
process runs itself (`type: local`) and the ones it queries over gRPC
(`type: remote`). Replaces the old two-concept split between the top-level
`attestor.attestations[]` block (this process's self-description) and
`relayer.clients[].attestorSet.attestors[]` (embedded attestor references).

Needed standalone (`ibc attestor run`, which only ever serves the
`type: local` subset of this list) or alongside a relayer in the same
process (`ibc relayer run` — `type: local` entries then run in-process and
resolve locally instead of over gRPC; populate `relayer.connections[]` too).

| Field            | Type   | Description |
|------------------|--------|--------------|
| `alias`          | string | Unique handle referenced by `relayer.connections[].clientA/clientB.attestorSet.attestors[]`. |
| `name`           | string | The attestor's own self-reported identity. NOT required unique — a local and a remote attestor may share a name. |
| `chainId`        | string | Which declared chain this attestor watches. |
| `type`           | string | `local` or `remote`. |
| `signer`         | string | `local` only. Must reference a `signers[].alias`. |
| `finalityOffset` | uint   | `local` only. `0` (default): attest up to the chain's `"finalized"` RPC tag. `n > 0`: attest up to `"latest" - n` instead. Use this where `"finalized"` is slow or unsupported (e.g. Ethereum PoS lags ~12-15 min behind head). |
| `grpc`           | string | `remote` only. Bare `host:port` (not a URL — a `://` here is rejected at validation). |

```yaml
attestors:
  - alias: "eth-watcher"
    name: "eth-watcher"
    chainId: "1"
    type: local
    signer: "my-local-signer"
    finalityOffset: 1
  - alias: "base-watcher"
    name: "base-watcher"
    chainId: "8453"
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
