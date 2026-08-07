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
| `autoRelay`   | object | `enabled` (bool), `lookback` (uint) — auto-relay settings for packets flowing FROM this end's chain TOWARD the counterparty end. |

`clientA` and `clientB` must be on different chains.

There's no `attestorSet` here — for `attestation` clients, the relayer
resolves the required attestor quorum *live* at startup (and on every
`--live` config validate) rather than from a static declaration:

1. It queries `clientId`'s own on-chain attestation light client for the
   authoritative registered attestor addresses and minimum required
   signature count.
2. For every entry in the top-level `attestors[]` list, it asks that
   attestor (in-process for `type: local`, over its `Info` RPC for
   `type: remote`) which chain it watches and which address it signs with.
3. It keeps only the entries that watch this end's counterparty chain *and*
   whose address is actually in the on-chain registered set, then requires
   at least the on-chain minimum count of them.

This removes a class of config drift that a static `attestorSet` couldn't
catch: a declared threshold or attestor reference that disagreed with what
the chain actually enforces used to surface only when a relay attempt failed
on submission; now it's caught at startup instead.

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

`ibc config validate --live` additionally queries each connection's two
chains' routers to confirm the on-chain registered counterparty actually
matches `clientA`/`clientB`, in both directions — catching a config that
names two clients as counterparties when the chains themselves disagree —
and runs the same attestor-quorum resolution described above, so a `--live`
pass means the relayer will actually be able to build its proof generators
on real startup, not just that the YAML shape is well-formed.

---

## `attestors`

Unified, top-level list of every attestor in play — both the ones this
process runs itself (`type: local`) and the ones it queries over gRPC
(`type: remote`). Every entry describes exactly one attestor identity;
which client ends it's actually authorized for is resolved live (see above),
never declared here.

Needed standalone (`ibc attestor run`, which only ever serves the
`type: local` subset of this list) or alongside a relayer in the same
process (`ibc relayer run` — `type: local` entries then run in-process and
resolve locally instead of over gRPC).

| Field            | Type   | Description |
|------------------|--------|--------------|
| `name`           | string | Identifies the attestor. For `type: local`, must be unique among this process's own local entries (they all share one dispatch table). For `type: remote`, it's whatever name that attestor's own operator assigned it — NOT required unique across different remote attestors, since a caller always asks a specific `grpc` endpoint for a specific name, never looks a name up across endpoints. |
| `chainId`        | string | `local` only — which declared chain this attestor watches. Not set for `type: remote`; discovered live via its `Info` RPC instead. |
| `type`           | string | `local` or `remote`. |
| `signer`         | string | `local` only. Must reference a `signers[].alias`. |
| `finalityOffset` | uint   | `local` only. `0` (default): attest up to the chain's `"finalized"` RPC tag. `n > 0`: attest up to `"latest" - n` instead. Use this where `"finalized"` is slow or unsupported (e.g. Ethereum PoS lags ~12-15 min behind head). A remote attestor's finality offset is discovered the same way as its chain and address — via `Info`, not configured here. |
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
