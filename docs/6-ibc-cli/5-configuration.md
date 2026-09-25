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

Values can contain `${VAR}`. The CLI replaces each variable from the environment before it parses the file. <!-- [cli/internal/config/file.go: LoadFromFile] -->

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
  clearOnStart: false
  clearInterval: 10m
  chainOverrides:
    - chainId: "1"
      evm:
        gasFeeCapMultiplier: 1.5
        gasTipCapMultiplier: 1.5
      txSubmissionDelay: 2s
      packetBatchSize: 20
      packetBatchTimeout: 10s
      clearInterval: 15m
      abandonUnrecoverablePackets: true
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


Some fields name something declared elsewhere in the file. The loader checks each of these when it reads the config:

<!-- GEN:config:crossrefs START -->

| Key | What the loader enforces |
|---|---|
| `attestors[].chainId` | Attestor … references unknown chain … |
| `attestors[].signer` | Attestor … references unknown signer … |
| `chains[].deployer` | Chain … references unknown signer … |
| `connections[].<end>.autoRelay` | Requires chains[…].evm.ws |
| `relayer.chainOverrides[].chainId` | … not declared in top-level chains |
| `relayer.connections[].<end>.chainId` | … not declared in top-level chains |

<!-- [config.go:L68](cli/internal/config/config.go#L68) -->

<!-- GEN:config:crossrefs END -->

For example, `signer: attestor-dan-key` selects the signer whose alias is `attestor-dan-key`. A local attestor's `chainId` must name a chain declared under `chains`; running the attestor also requires that chain's `evm.rpc` endpoint. Any unresolved reference fails at load — including on `ibc config validate` — before a relayer or attestor process starts.

## `server`

`server` sets the address for the relayer and attestor APIs. A process that runs both services uses one server.

<!-- GEN:config:server START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `listenAddr` | `string` | `0.0.0.0:3000` | Address the gRPC server binds. It serves the relayer and attestor APIs together. |

<!-- [config.go:L82](cli/internal/config/config.go#L82) -->

<!-- GEN:config:server END -->

Server reflection is always enabled. <!-- [cli/internal/bootstrap/bootstrap.go: rpcEnableReflection] --> <!-- [cli/internal/server/server.go: Server.start] -->

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
<!-- [cli/cmd/ibc/config.go: setupHomeWithConfig] -->

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
| `metrics` | `bool` | `false` | Enables metric collection and export. When false, the rest of this block is ignored. |
| `type` | `simple` \| `otel` | `simple` | Selects simple Prometheus export or an OTEL configuration file. |
| `simpleMetricsListenAddr` | `string` | `0.0.0.0:9090` | The Prometheus listener address in simple mode. |
| `otelFile` | `string` | **required** for `otel` | The OTEL configuration file, overridden by OTEL_CONFIG_FILE. One of the two is required. |

<!-- [config.go:L102](cli/internal/config/config.go#L102) -->

<!-- GEN:config:observability END -->

`simple` serves Prometheus metrics over HTTP on `simpleMetricsListenAddr`.
`otel` collects through OpenTelemetry instead and needs a configuration file,
from `otelFile` or from `OTEL_CONFIG_FILE`.
<!-- [cli/internal/config/config.go: Observability.ConfigFile] -->

## `chains`

`chains` lists every chain used elsewhere in the file.

<!-- GEN:config:chains START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain's id, as the chain reports it. |
| `deployer` | `string` | optional | Optional signer alias used by `ibc deploy` for this chain. |
| `evm.rpc` | `string` | **required** | JSON-RPC endpoint for the chain. |
| `evm.ws` | `string` | optional | A websocket endpoint, required for chains sourcing auto-relayed routes. |
| `evm.ics26Router` | `string` | **required** to run | Address of the ICS26 router on the chain. |

<!-- [config.go:L119](cli/internal/config/config.go#L119) -->

<!-- GEN:config:chains END -->

`ibc deploy core` deploys the router. <!-- [cli/internal/deploy/steps.go: RunSteps] --> Omit `ics26Router` before deployment, then fill it in from the manifest, or let `ibc deploy render-config` write the finished blocks. <!-- [cli/cmd/ibc/deploy.go: renderedChain] -->

The deployer must be a local signer, because deployment requires direct access to the key. <!-- [cli/cmd/ibc/deploy.go: resolveDeployerAlias] -->

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
| `connections[].clientA.params, connections[].clientB.params` | block | optional | This client type's settings: empty for `attestation`, and `{url: <ProverService endpoint>}` for `remote`, where it is required. |
| `connections[].clientA.autoRelay.enabled, connections[].clientB.autoRelay.enabled` | `bool` | optional | Whether the relayer carries packets leaving this end without being asked. |

<!-- [relayer.go:L69](cli/internal/config/relayer.go#L69) -->

<!-- GEN:config:relayer:connections END -->

Client identifiers are scoped to a chain, so both ends can use the same `clientId`, as in the example above. `ibc deploy client` does this by default. <!-- [cli/cmd/ibc/deploy.go: deployClient] -->

The two client ends must belong to different chains. A client can appear in only one configured connection on a given chain. <!-- [cli/internal/config/relayer.go: RelayerConfig.validateConnectionIdentities] -->

With `autoRelay.enabled` on an end, the relayer carries that end's outgoing packets without being asked. <!-- [cli/internal/relay/watcher/set.go: NewSetFromConfig] --> That end's chain needs `evm.ws`, and validation fails without it. <!-- [cli/internal/config/config.go: Config.validateAutoRelay] --> Unset and `false` are the same input. <!-- [cli/internal/config/relayer.go: RelayerConfig.Validate] -->

### Relay settings

The relayer uses these defaults unless you override them.

<!-- GEN:config:relayer START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `dispatchPollInterval` | `duration` | `1s` | How often the dispatcher polls the store for unfinished packets. |
| `clearOnStart` | `bool` | `true` | Runs a clearing pass at startup. Unset runs it. |
| `clearInterval` | `duration` | `5m` | How often a clearing pass runs; a chainOverrides entry wins. |

<!-- [relayer.go:L34](cli/internal/config/relayer.go#L34) --> <!-- [dispatcher.go:L17](cli/internal/relay/dispatch/dispatcher.go#L17) --> <!-- [relayer.go:L19](cli/internal/config/relayer.go#L19) --> <!-- [relayer.go:L22](cli/internal/config/relayer.go#L22) -->

<!-- GEN:config:relayer END -->

<!-- GEN:config:relayer:chainOverrides START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainOverrides[].chainId` | `string` | **required** | The chain these settings apply to. |
| `chainOverrides[].txSubmissionDelay` | `duration` | `2s` | Minimum delay between two transaction submissions on the chain. |
| `chainOverrides[].packetBatchSize` | `int` | `50` | How many packets the relayer puts in one transaction. |
| `chainOverrides[].packetBatchTimeout` | `duration` | `3s` (receive and acknowledge), `1m` (timeout) | How long the relayer waits to fill a batch before submitting it. |
| `chainOverrides[].clearInterval` | `duration` | optional | Overrides relayer.clearInterval for packets sourced from this chain. |
| `chainOverrides[].abandonUnrecoverablePackets` | `bool` | optional | Stops re-probing packets whose send log the endpoint will not serve. They are remembered but never looked at again, so turning it back off recovers them against an archive endpoint. |
| `chainOverrides[].evm.gasFeeCapMultiplier` | `float64` | optional | Multiplies the fee cap the node suggests. |
| `chainOverrides[].evm.gasTipCapMultiplier` | `float64` | optional | Multiplies the tip cap the node suggests. |

<!-- [relayer.go:L45](cli/internal/config/relayer.go#L45) --> <!-- [evm.go:L26](cli/internal/txsubmitter/evm/evm.go#L26) --> <!-- [opts.go:L14](cli/internal/relay/pipeline/opts.go#L14) --> <!-- [opts.go:L15](cli/internal/relay/pipeline/opts.go#L15) --> <!-- [opts.go:L16](cli/internal/relay/pipeline/opts.go#L16) -->

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

Receive batches use the destination chain's settings. Acknowledgement and timeout batches use the source chain's settings. <!-- [cli/internal/relay/pipeline/opts.go: Options] -->

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

<!-- [config.go:L143](cli/internal/config/config.go#L143) -->

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

<!-- [config.go:L143](cli/internal/config/config.go#L143) -->

<!-- GEN:config:attestors:remote END -->

A remote entry does not set `chainId` or `signer`. The process obtains the attestor's chain and signing address from its `Info` RPC. <!-- [cli/internal/service/attestor/remote.go: queryAttestorInfo] -->

Fields from the other attestor type are rejected. A remote attestor cannot set `chainId`, and a local attestor cannot set `grpc`. <!-- [cli/internal/config/config.go: AttestorConfig.Validate] -->

Local attestor names must be unique. Two local attestors for the same chain must also use different signers. <!-- [cli/internal/config/config.go: Attestors.validateIdentities] -->

## `signers`

`signers` defines the keys referenced elsewhere in the file. Each signer has a unique alias.

### Local signer

<!-- GEN:config:signers:local START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Unique name for a signer. |
| `type` | `local` | **required** | Whether the key is a file on disk or a key held by a remote signer. |
| `file` | `string` | **required** | Key file path for a local signer. |

<!-- [config.go:L168](cli/internal/config/config.go#L168) -->

<!-- GEN:config:signers:local END -->

For `file: relayer`, the CLI checks `relayer`, `relayer.json`, and the `keys` directory under the IBC home directory. The final path is typically `~/.ibc/keys/relayer.json`. <!-- [cli/internal/config/file.go: KeyFileFallbacks] -->

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

<!-- [config.go:L168](cli/internal/config/config.go#L168) -->

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
