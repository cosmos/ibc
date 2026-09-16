---
title: "Configuration"
description: "Configure chains, connections, attestors, signers, and storage in ibc.yml."
---

<!-- GEN:notice START -->

<!--
Tables between GEN markers on this page are generated from this
repository by docs/6-ibc-cli/tools/refgen.py. Do not edit inside them: the next
run overwrites whatever is there, so a hand edit looks like a fix and is not.
The prose around them is written by hand and is yours to change.

After changing cli/, proto/ or gen/, follow docs/6-ibc-cli/tools/AGENTS.md
before opening a pull request.
-->

<!-- GEN:notice END -->

The IBC CLI reads its configuration from `ibc.yml`. The file tells the CLI:

- which chains to connect to
- which clients form each connection
- which attestors to run or query
- which keys to use for deployment, relaying, and attestation
- where the relayer stores packet state

Run the following commands to create and validate the file:

```sh
ibc config new
ibc config validate              # structural checks and cross-references
ibc config validate relayer      # also check the file can run a relayer
ibc config validate attestor     # also check the file can run a local attestor
```

Every command loads the config file and runs the same structural checks: unknown fields are rejected, and cross-references (including a local attestor's `chainId` and `signer`) must resolve. `ibc config validate` reports those without starting a process. Pass `relayer` or `attestor` to add the sufficiency checks needed to run that process.

Values can contain `${VAR}`. The CLI replaces each variable from the environment before it parses the file. <!-- [file.go:L24](cli/internal/config/file.go#L24) -->

## Example config.yml

A working configuration, copied from the fixture the CLI's own tests load and validate. Every key below is one the relayer accepts today: if a key is renamed and this file is not updated, the Go test suite fails.

<!-- GEN:config:example START -->

```yaml
server:
  listenAddr: 0.0.0.0:3000
signers:
  - alias: "relayer-key"
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: "relayer-key-id"
  - alias: "attestor-dan-key"
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: "attestor-dan-key-id"
db:
  type: sqlite
  url: ibc.db
chains:
  - chainId: "1"
    evm:
      rpc: https://ethereum-rpc.example.com
      ws: wss://ethereum-rpc.example.com
      ics26Router: "0x0000000000000000000000000000000000000001"
  - chainId: "8453"
    evm:
      rpc: https://base-rpc.example.com
      ics26Router: "0x0000000000000000000000000000000000000001"
relayer:
  dispatchPollInterval: 3s
  chainOverrides:
    - chainId: "1"
      evm:
        gasFeeCapMultiplier: 1.5
        gasTipCapMultiplier: 1.5
      txSubmissionDelay: 2s
      packetBatchSize: 20
      packetBatchTimeout: 10s
    - chainId: "8453"
  connections:
    - alias: "eth-base"
      clientA:
        chainId: "1"
        signer: "relayer-key"
        clientId: "base-0"
        type: "attestation"
        autoRelay:
          enabled: false
      clientB:
        chainId: "8453"
        signer: "relayer-key"
        clientId: "ethereum-0"
        type: "attestation"
attestors:
  - name: "attestor-alice-base"
    type: remote
    grpc: attestor-alice.example.com:3000
  - name: "attestor-bob-base"
    type: remote
    grpc: attestor-bob.example.com:3000
  - name: "attestor-dan-base"
    chainId: "8453"
    type: local
    signer: "attestor-dan-key"
    finalityOffset: 1
  - name: "attestor-dan-ethereum"
    chainId: "1"
    type: local
    signer: "attestor-dan-key"
```

<!-- [sample.yml:L1](cli/internal/config/testdata/sample.yml#L1) -->

<!-- GEN:config:example END -->


The following fields in this file are references to other fields:

| Reference | Must match | Checked at |
| --- | --- | --- |
| `clientA.chainId` and `clientB.chainId` | A `chains[].chainId` value <!-- [config.go:L584-L590](cli/internal/config/config.go#L584-L590) --> | Config load |
| `clientA.signer` and `clientB.signer` | A `signers[].alias` value <!-- [config.go:L598-L606](cli/internal/config/config.go#L598-L606) --> | Config load |
| A local attestor's `signer` | A `signers[].alias` value <!-- [config.go:L546-L549](cli/internal/config/config.go#L546-L549) --> | Config load |
| A local attestor's `chainId` | A `chains[].chainId` value <!-- [config.go:L551-L554](cli/internal/config/config.go#L551-L554) --> | Config load; `evm.rpc` required to run |
| `chains[].deployer` | A `signers[].alias` value <!-- [config.go:L557-L564](cli/internal/config/config.go#L557-L564) --> | Config load |

For example, `signer: attestor-dan-key` selects the signer whose alias is `attestor-dan-key`. A local attestor's `chainId` must name a chain declared under `chains`; running the attestor also requires that chain's `evm.rpc` endpoint. Any unresolved reference fails at load — including on `ibc config validate` — before a relayer or attestor process starts.

## `server`

`server` sets the address for the relayer and attestor APIs. A process that runs both services uses one server.

<!-- GEN:config:server START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `listenAddr` | `string` | `0.0.0.0:3000` | Address the gRPC server binds. It serves the relayer and attestor APIs together. |

<!-- [config.go:L82](cli/internal/config/config.go#L82) -->

<!-- GEN:config:server END -->

Server reflection is always enabled. <!-- [bootstrap.go:L120](cli/internal/bootstrap/bootstrap.go#L120) --> <!-- [server.go:L103-L113](cli/internal/server/server.go#L103-L113) -->

## `logging`

`logging` sets how the process writes its own logs. Each key has a matching
flag, and the flag wins only when it is passed explicitly, so a value here
survives an ordinary invocation.

<!-- GEN:config:logging START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `level` | `string` | `info` | A slog level name (debug, info, warn, error). Empty defaults to info. |
| `json` | `bool` | optional | Emits logs as JSON instead of text. |

<!-- [config.go:L88](cli/internal/config/config.go#L88) -->

<!-- GEN:config:logging END -->

`--log-level` and `--log-json` override their keys for one run.
<!-- [config.go:L213-L222](cli/cmd/ibc/config.go#L213-L222) -->

## `db`

`db` configures the relayer's packet store. The store lets the relayer resume unfinished work after a restart.

<!-- GEN:config:db START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `type` | `sqlite` \| `postgres` | `sqlite` | Database backend. |
| `url` | `string` | `ibc.db` | File path for sqlite, connection string for postgres. `:memory:` is rejected. |

<!-- [config.go:L96](cli/internal/config/config.go#L96) -->

<!-- GEN:config:db END -->

`ibc relayer run` applies pending migrations at startup. Pass `--no-migrate` to disable automatic migration, or run `ibc migrate up` separately.

## `observability`

`observability` turns metrics on and says how they are served. With `metrics`
false nothing else in the block is read, and the process logs that observability
is disabled.

<!-- GEN:config:observability START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `metrics` | `bool` | `false` | Whether the process exports metrics. When false, the rest of this block is ignored. |
| `type` | `string` | `simple` | Which exporter serves the metrics. |
| `simpleMetricsListenAddr` | `string` | `0.0.0.0:9090` | Address the `simple` exporter serves metrics on. |
| `otelFile` | `string` | optional | OpenTelemetry configuration file, read when `type` is `otel`. `OTEL_CONFIG_FILE` overrides it, and one of the two is required. |

<!-- [config.go:L103](cli/internal/config/config.go#L103) -->

<!-- GEN:config:observability END -->

`simple` serves Prometheus metrics over HTTP on `simpleMetricsListenAddr`.
`otel` collects through OpenTelemetry instead and needs a configuration file,
from `otelFile` or from `OTEL_CONFIG_FILE`.
<!-- [config.go:L43-L47](cli/internal/config/config.go#L43-L47) -->

## `chains`

`chains` lists every chain used elsewhere in the file.

<!-- GEN:config:chains START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain's id, as the chain reports it. |
| `deployer` | `string` | optional | Optional signer alias used by `ibc deploy` for this chain. |
| `evm.rpc` | `string` | **required** | JSON-RPC endpoint for the chain. |
| `evm.ws` | `string` | optional | A websocket endpoint, required for chains sourcing auto-relayed routes. |
| `evm.ics26Router` | `string` | optional | Address of the ICS26 router on the chain. |

<!-- [config.go:L114](cli/internal/config/config.go#L114) -->

<!-- GEN:config:chains END -->

`ibc deploy core` deploys the router. <!-- [steps.go:L67-L97](cli/internal/deploy/steps.go#L67-L97) --> Omit `ics26Router` before deployment, then fill it in from the manifest, or let `ibc deploy render-config` write the finished blocks. <!-- [deploy.go:L544-L600](cli/cmd/ibc/deploy.go#L544-L600) -->

The deployer must be a local signer, because deployment requires direct access to the key. <!-- [deploy.go:L123-L155](cli/cmd/ibc/deploy.go#L123-L155) -->

## `relayer`

### Connections

`relayer.connections` selects the connections this process relays. Each entry identifies one client on each chain, and the relayer handles traffic in both directions.

<!-- GEN:config:relayer:connections START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `connections[].alias` | `string` | **required** | Name for the connection, unique in the file. |
| `connections[].clientA.chainId, connections[].clientB.chainId` | `string` | **required** | The chain this end's client lives on. |
| `connections[].clientA.signer, connections[].clientB.signer` | `string` | **required** | `signers` alias that submits relay transactions on this chain. |
| `connections[].clientA.clientId, connections[].clientB.clientId` | `string` | **required** | The light client's id on this chain. |
| `connections[].clientA.type, connections[].clientB.type` | `attestation` \| `remote` | **required** | Light client type. |
| `connections[].clientA.params, connections[].clientB.params` | `yaml.RawMessage` | optional | This client type's settings. |
| `connections[].clientA.autoRelay.enabled, connections[].clientB.autoRelay.enabled` | `bool` | optional | Whether the relayer carries packets leaving this end without being asked. |

<!-- [relayer.go:L51](cli/internal/config/relayer.go#L51) -->

<!-- GEN:config:relayer:connections END -->

Client identifiers are scoped to a chain, so both ends can use the same `clientId`, as in the example above. `ibc deploy client` does this by default. <!-- [deploy.go:L256-L262](cli/cmd/ibc/deploy.go#L256-L262) -->

The two client ends must belong to different chains. A client can appear in only one configured connection on a given chain. <!-- [relayer.go:L185-L192](cli/internal/config/relayer.go#L185-L192) -->

With `autoRelay.enabled` on an end, the relayer carries that end's outgoing packets without being asked. <!-- [set.go:L27-L39](cli/internal/relay/watcher/set.go#L27-L39) --> That end's chain needs `evm.ws`, and validation fails without it. <!-- [config.go:L511-L528](cli/internal/config/config.go#L511-L528) --> Unset and `false` are the same input. <!-- [relayer.go:L126-L142](cli/internal/config/relayer.go#L126-L142) -->

### Relay settings

The relayer uses these defaults unless you override them.

<!-- GEN:config:relayer START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `dispatchPollInterval` | `duration` | `1s` | How often the dispatcher polls the store for unfinished packets. |

<!-- [relayer.go:L28](cli/internal/config/relayer.go#L28) --> <!-- [dispatcher.go:L17](cli/internal/relay/dispatch/dispatcher.go#L17) -->

<!-- GEN:config:relayer END -->

<!-- GEN:config:relayer:chainOverrides START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainOverrides[].chainId` | `string` | **required** | The chain these settings apply to. |
| `chainOverrides[].txSubmissionDelay` | `duration` | `2s` | Minimum delay between two transaction submissions on the chain. |
| `chainOverrides[].packetBatchSize` | `int` | `50` | How many packets the relayer puts in one transaction. |
| `chainOverrides[].packetBatchTimeout` | `duration` | `3s` (receive and acknowledge), `1m` (timeout) | How long the relayer waits to fill a batch before submitting it. |
| `chainOverrides[].evm.gasFeeCapMultiplier` | `float64` | optional | Multiplies the fee cap the node suggests. |
| `chainOverrides[].evm.gasTipCapMultiplier` | `float64` | optional | Multiplies the tip cap the node suggests. |

<!-- [relayer.go:L35](cli/internal/config/relayer.go#L35) --> <!-- [evm.go:L26](cli/internal/txsubmitter/evm/evm.go#L26) --> <!-- [opts.go:L14](cli/internal/relay/pipeline/opts.go#L14) --> <!-- [opts.go:L15](cli/internal/relay/pipeline/opts.go#L15) --> <!-- [opts.go:L16](cli/internal/relay/pipeline/opts.go#L16) -->

<!-- GEN:config:relayer:chainOverrides END -->

Add an override only when a chain needs different behavior:

```yaml
relayer:
  dispatchPollInterval: 5s
  chainOverrides:
    - chainId: "41002"
      txSubmissionDelay: 3s
      packetBatchSize: 25
      packetBatchTimeout: 15s
      evm:
        gasFeeCapMultiplier: 1.2
        gasTipCapMultiplier: 1.1
```

Receive batches use the destination chain's settings. Acknowledgement and timeout batches use the source chain's settings. <!-- [opts.go:L34-L71](cli/internal/relay/pipeline/opts.go#L34-L71) -->

## `attestors`

`attestors` lists the attestors that the process runs or queries. A `local` attestor watches a configured chain and signs with a configured signer. A `remote` attestor runs elsewhere and is queried over gRPC.

### Local attestor

<!-- GEN:config:attestors:local START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain this attestor watches. |
| `name` | `string` | **required** | The attestor's own self-reported identity. Not required unique. |
| `type` | `local` | **required** | Whether this process runs the attestor or queries it. |
| `signer` | `string` | **required** | The signer used to sign attestations. |
| `finalityOffset` | `uint` | optional | Zero attests up to the chain's `finalized` tag; n > 0 attests up to `latest` - n instead. |

<!-- [config.go:L138](cli/internal/config/config.go#L138) -->

<!-- GEN:config:attestors:local END -->

Set `finalityOffset` according to the chain's finality model.

### Remote attestor

```yaml
attestors:
  - name: attestor-41002
    type: remote
    grpc: attestor.example.com:3000
```

<!-- GEN:config:attestors:remote START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `name` | `string` | **required** | The attestor's own self-reported identity. Not required unique. |
| `type` | `remote` | **required** | Whether this process runs the attestor or queries it. |
| `grpc` | `string` | **required** | Bare host:port. |

<!-- [config.go:L138](cli/internal/config/config.go#L138) -->

<!-- GEN:config:attestors:remote END -->

A remote entry does not set `chainId` or `signer`. The process obtains the attestor's chain and signing address from its `Info` RPC. <!-- [remote.go:L31-L51](cli/internal/service/attestor/remote.go#L31-L51) -->

Fields from the other attestor type are rejected. A remote attestor cannot set `chainId`, and a local attestor cannot set `grpc`. <!-- [config.go:L513-L536](cli/internal/config/config.go#L513-L536) -->

Local attestor names must be unique. Two local attestors for the same chain must also use different signers. <!-- [config.go:L381-L396](cli/internal/config/config.go#L381-L396) -->

## `signers`

`signers` defines the keys referenced elsewhere in the file. Each signer has a unique alias.

### Local signer

<!-- GEN:config:signers:local START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Unique name for a signer. |
| `type` | `local` | **required** | Whether the key is a file on disk or a key held by a remote signer. |
| `file` | `string` | **required** | Key file path for a local signer. |

<!-- [config.go:L163](cli/internal/config/config.go#L163) -->

<!-- GEN:config:signers:local END -->

For `file: relayer`, the CLI checks `relayer`, `relayer.json`, and the `keys` directory under the IBC home directory. The final path is typically `~/.ibc/keys/relayer.json`. <!-- [config.go:L591-L615](cli/internal/config/config.go#L591-L615) -->

### Remote signer

```yaml
signers:
  - alias: relayer
    type: remote
    grpc: signer.example.com:9090
    remoteKeyId: relayer-key-id
```

<!-- GEN:config:signers:remote START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Unique name for a signer. |
| `type` | `remote` | **required** | Whether the key is a file on disk or a key held by a remote signer. |
| `grpc` | `string` | **required** | Address for a remote signer. |
| `remoteKeyId` | `string` | **required** | KMS key ID for a remote signer. |

<!-- [config.go:L163](cli/internal/config/config.go#L163) -->

<!-- GEN:config:signers:remote END -->

The remote signer holds the key material and performs signing.

## Split the configuration by process

The complete example runs the relayer and both attestors together. In production, you can give each process only the blocks it uses:

- A standalone relayer needs `server`, `db`, `chains`, `relayer`, the attestors it queries, and its transaction signers.
- A standalone local attestor needs `server`, its chain, its `attestors` entry, and its attestation signer. It does not need `db` or `relayer`.

## Next steps

- [Run a standalone relayer](4-run-a-standalone-relayer.md)
- [Run a standalone attestor](3-run-a-standalone-attestor.md)
- [CLI commands](6-cli-commands.md) for command-line overrides
