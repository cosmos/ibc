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

When changing commands, configuration, APIs, or cited behavior represented on
these pages, follow docs/6-ibc-cli/tools/AGENTS.md.
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
```

Populate the chains, signers, connections, and attestors you need before running the process-specific checks:

```sh
ibc config validate              # structural checks and cross-references
ibc config validate relayer      # also check the file can run a relayer
ibc config validate attestor     # also check the file can run a local attestor
```

`ibc config validate` rejects unknown fields and unresolved cross-references, including a local attestor's `chainId` and `signer`, without starting a process. Key-management commands and commands that write a draft configuration do not necessarily run these checks. Pass `relayer` or `attestor` to add the sufficiency checks needed to run that process.

Values can contain `${VAR}`. The CLI replaces each variable from the environment before it parses the file. <!-- [cli/internal/config/file.go: LoadFromFile] -->

## Example config.yml

Below is an example of a config file:

<!-- GEN:config:example START -->

```yaml
server:
  listenAddr: 0.0.0.0:3000
signers:
  - alias: "relayer-key"
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: "relayer-key-id"
  - alias: "attestor-dan-base-key"
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: "attestor-dan-base-key-id"
  - alias: "attestor-dan-ethereum-key"
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: "attestor-dan-ethereum-key-id"
db:
  type: sqlite
  url: ibc.db
chains:
  - chainId: "1"
    evm:
      rpc: https://ethereum-rpc.example.com
      ws: wss://ethereum-rpc.example.com
      ics26Router: "0x0000000000000000000000000000000000000001"
      ibcMulticall: "0x0000000000000000000000000000000000000002"
  - chainId: "8453"
    evm:
      rpc: https://base-rpc.example.com
      ics26Router: "0x0000000000000000000000000000000000000001"
      ibcMulticall: "0x0000000000000000000000000000000000000002"
relayer:
  dispatchPollInterval: 3s
  clearOnStart: false
  clearInterval: 10m
  chainOverrides:
    - chainId: "1"
      evm:
        pendingTxTimeout: 2m
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
    signer: "attestor-dan-base-key"
    finalityOffset: 1
  - name: "attestor-dan-ethereum"
    chainId: "1"
    type: local
    signer: "attestor-dan-ethereum-key"
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
| `relayer.connections[].<end>.signer` | References unknown signer … |

<!-- [config.go:L68](cli/internal/config/config.go#L68) -->

<!-- GEN:config:crossrefs END -->

For example, `signer: attestor-dan-base-key` selects the signer whose alias is `attestor-dan-base-key`. A local attestor's `chainId` must name a chain declared under `chains`; running the attestor also requires that chain's `evm.rpc` endpoint. `ibc config validate`, `ibc relayer run`, and `ibc attestor run` reject unresolved references.

## `server`

`server` sets the address for the relayer and attestor APIs. A process that runs both services uses one server.

<!-- GEN:config:server START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `listenAddr` | `string` | `0.0.0.0:3000` | Address the gRPC server binds. It serves the relayer and attestor APIs together. |

<!-- [config.go:L82](cli/internal/config/config.go#L82) -->

<!-- GEN:config:server END -->

Server reflection is always enabled. <!-- [cli/internal/bootstrap/bootstrap.go: rpcEnableReflection] --> <!-- [cli/internal/server/server.go: Server.Start] -->

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

See [Observability and metrics](8-observability-and-metrics.md) for metric names,
labels, and semantics.

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
| `evm.ibcMulticall` | `string` | **required** to relay | The batching contract address, required to run the relayer. |

<!-- [config.go:L119](cli/internal/config/config.go#L119) -->

<!-- GEN:config:chains END -->

`ibc deploy core` deploys the router and multicall. <!-- [cli/internal/deploy/steps.go: CoreSteps] --> Omit their addresses before deployment, then fill them in from the manifest, or let `ibc deploy render-config` write the finished blocks. <!-- [cli/cmd/ibc/deploy.go: renderedChain] -->

The deployer must be a local signer, because deployment requires direct access to the key. <!-- [cli/cmd/ibc/deploy.go: deployerKeyHex] -->

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
| `connections[].clientA.type, connections[].clientB.type` | `string` | **required** | The on-chain light client type: `attestation` or `besu`, or with a remoteProver any type, including ones the IBC CLI does not implement. |
| `connections[].clientA.remoteProver.url, connections[].clientB.remoteProver.url` | `string` | **required** in `remoteProver` | The ProverService endpoint: an http(s):// URL, or a bare host:port dialed as http://. Without a query or fragment. |
| `connections[].clientA.remoteProver.tls.caFile, connections[].clientB.remoteProver.tls.caFile` | `string` | optional | A PEM bundle verifying the server. Empty uses system roots. Read at startup, so a changed bundle needs a restart. |
| `connections[].clientA.remoteProver.tls.certFile, connections[].clientB.remoteProver.tls.certFile` | `string` | optional | The client certificate presented for mTLS. Required with keyFile. Omit both for one-way TLS. Read at startup, so a rotated certificate needs a restart. |
| `connections[].clientA.remoteProver.tls.keyFile, connections[].clientB.remoteProver.tls.keyFile` | `string` | optional | The private key for certFile. Required with certFile. |
| `connections[].clientA.remoteProver.tls.serverName, connections[].clientB.remoteProver.tls.serverName` | `string` | optional | Overrides the name verified against the server certificate. Needed when dialing an address that differs from the certificate's name. |
| `connections[].clientA.remoteProver.tls.insecureSkipVerify, connections[].clientB.remoteProver.tls.insecureSkipVerify` | `bool` | optional | Disables server certificate verification and logs a warning. Development only. Must not be combined with caFile. |
| `connections[].clientA.autoRelay.enabled, connections[].clientB.autoRelay.enabled` | `bool` | optional | Whether the relayer carries packets leaving this end without being asked. |

<!-- [relayer.go:L61](cli/internal/config/relayer.go#L61) -->

<!-- GEN:config:relayer:connections END -->

Client identifiers are scoped to a chain, so both ends can use the same `clientId`. The `ibc deploy client` subcommands do this by default. <!-- [cli/cmd/ibc/deploy.go: defaultClientID] -->

The two client ends must belong to different chains. A client can appear in only one configured connection on a given chain. <!-- [cli/internal/config/relayer.go: RelayerConfig.validateConnectionIdentities] -->

With `autoRelay.enabled` on an end, the relayer carries that end's outgoing packets without being asked. <!-- [cli/internal/relay/watcher/set.go: NewSetFromConfig] --> That end's chain needs `evm.ws`, and validation fails without it. <!-- [cli/internal/config/config.go: Config.validateAutoRelay] --> Unset and `false` are the same input. <!-- [cli/internal/config/relayer.go: RelayerConfig.AutoRelayConnections] -->

`remoteProver` is optional. Without it, the relayer proves an end with the built-in prover for its `type`; with it, the ProverService at `remoteProver.url` proves that end instead, and `type` may name a light client the IBC CLI does not implement. <!-- [cli/internal/relay/prover/prover.go: newProver] --> `ibc deploy render-config` never writes a `remoteProver` block, and keeps one you added. <!-- [cli/internal/config/patch.go: mergeConnections] -->

### Relay settings

The relayer uses these defaults unless you override them.

<!-- GEN:config:relayer START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `dispatchPollInterval` | `duration` | `1s` | How often the dispatcher polls the store for unfinished packets. |
| `clearOnStart` | `bool` | `true` | Runs a clearing pass at startup. Unset runs it. |
| `clearInterval` | `duration` | `5m` | How often a clearing pass runs; a chainOverrides entry wins. |

<!-- [relayer.go:L27](cli/internal/config/relayer.go#L27) --> <!-- [dispatcher.go:L17](cli/internal/relay/dispatch/dispatcher.go#L17) --> <!-- [relayer.go:L18](cli/internal/config/relayer.go#L18) --> <!-- [relayer.go:L24](cli/internal/config/relayer.go#L24) -->

<!-- GEN:config:relayer END -->

<!-- GEN:config:relayer:chainOverrides START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainOverrides[].chainId` | `string` | **required** | The chain these settings apply to. |
| `chainOverrides[].packetBatchSize` | `int` | `25` | How many packets the relayer puts in one transaction. |
| `chainOverrides[].packetBatchTimeout` | `duration` | `3s` (receive and acknowledge), `1m` (timeout) | How long the relayer waits to fill a batch before submitting it. |
| `chainOverrides[].clearInterval` | `duration` | optional | Overrides relayer.clearInterval for packets sourced from this chain. |
| `chainOverrides[].abandonUnrecoverablePackets` | `bool` | optional | Stops re-probing packets whose send log the endpoint will not serve. They are remembered but never looked at again, so turning it back off recovers them against an archive endpoint. |
| `chainOverrides[].evm.pendingTxTimeout` | `duration` | `2m` | Sets how long to wait before retrying an unmined transaction or reusing a stalled nonce. |

<!-- [relayer.go:L38](cli/internal/config/relayer.go#L38) --> <!-- [opts.go:L14](cli/internal/relay/pipeline/opts.go#L14) --> <!-- [opts.go:L15](cli/internal/relay/pipeline/opts.go#L15) --> <!-- [opts.go:L16](cli/internal/relay/pipeline/opts.go#L16) --> <!-- [evm.go:L26](cli/internal/txsubmitter/evm/evm.go#L26) -->

<!-- GEN:config:relayer:chainOverrides END -->

Each EVM submission estimates its fee cap as twice the latest block's base fee plus the node's suggested tip. Replacement attempts increase the fee cap and tip by 20%, rounded up with a minimum increase of one wei, within a ceiling of four times that fresh estimate. The ceiling stays fixed during a submission's retries and is recalculated from fresh node data on the next submission. <!-- [cli/internal/txsubmitter/evm/txops.go: estimateFees] --> <!-- [cli/internal/txsubmitter/evm/txops.go: bump] --> <!-- [cli/internal/txsubmitter/evm/evm.go: TxSubmitter.Submit] -->

Add an override only when a chain needs different behavior:

```yaml
relayer:
  dispatchPollInterval: 5s
  chainOverrides:
    - chainId: "41002"
      packetBatchSize: 25
      packetBatchTimeout: 15s
      evm:
        pendingTxTimeout: 3m
```

Receive batches use the destination chain's settings. Acknowledgement and timeout batches use the source chain's settings. <!-- [cli/internal/relay/pipeline/opts.go: OptionsFromConfig] -->

## `attestors`

`attestors` lists the attestors that the process runs or queries. A `local` attestor watches a configured chain and signs with a configured signer. A `remote` attestor runs elsewhere and is queried over gRPC.

### Local attestor

<!-- GEN:config:attestors:local START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain this attestor watches. |
| `name` | `string` | **required** | The attestor's own self-reported identity. For local attestors, names must be unique. |
| `type` | `local` | **required** | Whether this process runs the attestor or queries it. |
| `signer` | `string` | **required** | The signer used to sign attestations. |
| `finalityOffset` | `uint` | optional | Zero attests up to the chain's `finalized` tag; n > 0 attests up to `latest` - n instead. |

<!-- [config.go:L146](cli/internal/config/config.go#L146) -->

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
| `name` | `string` | **required** | The attestor's own self-reported identity. For local attestors, names must be unique. |
| `type` | `remote` | **required** | Whether this process runs the attestor or queries it. |
| `grpc` | `string` | **required** | Bare host:port, or an http(s):// URL without a query or fragment. https:// uses TLS. |
| `tls.caFile` | `string` | optional | A PEM bundle verifying the server. Empty uses system roots. Read at startup, so a changed bundle needs a restart. |
| `tls.certFile` | `string` | optional | The client certificate presented for mTLS. Required with keyFile. Omit both for one-way TLS. Read at startup, so a rotated certificate needs a restart. |
| `tls.keyFile` | `string` | optional | The private key for certFile. Required with certFile. |
| `tls.serverName` | `string` | optional | Overrides the name verified against the server certificate. Needed when dialing an address that differs from the certificate's name. |
| `tls.insecureSkipVerify` | `bool` | optional | Disables server certificate verification and logs a warning. Development only. Must not be combined with caFile. |

<!-- [config.go:L146](cli/internal/config/config.go#L146) -->

<!-- GEN:config:attestors:remote END -->

A remote entry does not set `chainId` or `signer`. The process obtains the attestor's chain and signing address from its `Info` RPC. <!-- [cli/internal/service/attestor/remote.go: queryAttestorInfo] -->

Fields from the other attestor type are rejected. A remote attestor cannot set `chainId`, and a local attestor cannot set `grpc`. <!-- [cli/internal/config/config.go: AttestorConfig.Validate] -->

Local attestor names must be unique. Two local attestors for the same chain must also use different signers. <!-- [cli/internal/config/config.go: Attestors.validateIdentities] -->

Use separate signing keys for independent clients and chains. See [Attestor keys](../4-light-clients/2-attestors.md#attestor-keys) for the signature replay risk.

## `signers`

`signers` defines the keys referenced elsewhere in the file. Each signer has a unique alias.

### Local signer

<!-- GEN:config:signers:local START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Unique name for a signer. |
| `type` | `local` | **required** | Whether the key is a file on disk or a key held by a remote signer. |
| `file` | `string` | **required** | Key file path for a local signer. |

<!-- [config.go:L175](cli/internal/config/config.go#L175) -->

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
| `grpc` | `string` | **required** | Address for a remote signer. An https:// URL uses TLS. |
| `remoteKeyId` | `string` | **required** | KMS key ID for a remote signer. |
| `tls.caFile` | `string` | optional | A PEM bundle verifying the server. Empty uses system roots. Read at startup, so a changed bundle needs a restart. |
| `tls.certFile` | `string` | optional | The client certificate presented for mTLS. Required with keyFile. Omit both for one-way TLS. Read at startup, so a rotated certificate needs a restart. |
| `tls.keyFile` | `string` | optional | The private key for certFile. Required with certFile. |
| `tls.serverName` | `string` | optional | Overrides the name verified against the server certificate. Needed when dialing an address that differs from the certificate's name. |
| `tls.insecureSkipVerify` | `bool` | optional | Disables server certificate verification and logs a warning. Development only. Must not be combined with caFile. |

<!-- [config.go:L175](cli/internal/config/config.go#L175) -->

<!-- GEN:config:signers:remote END -->

The remote signer holds the key material and performs signing.

## TLS for remote services

Remote signers, remote attestors, and remote provers all follow one rule: an `https://` address turns TLS on, and anything else connects in plaintext. That address is the signer's `grpc`, the attestor's `grpc`, or a client end's `remoteProver.url`. <!-- [cli/internal/config/tls.go: ResolveEndpoint] -->

```yaml
attestors:
  - name: attestor-41002
    type: remote
    grpc: https://attestor.example.com
    tls:
      caFile: /etc/ibc/tls/ca.crt
      certFile: /etc/ibc/tls/client.crt
      keyFile: /etc/ibc/tls/client.key
```

Without a `tls` block, TLS trusts the system's CAs and presents no client certificate. Add a `tls` block, with the keys listed in the remote attestor and remote signer tables, to change that; a client end's `remoteProver` block takes it as `tls`. A `tls` block on an address that isn't `https://` is rejected, since its settings would be ignored. <!-- [cli/internal/config/tls.go: validateEndpointTLS] -->

An `https://` address without a port uses 443, so `https://attestor.example.com` connects to port 443; write `https://host:port` for any other port. A bare `host:port` connects in plaintext: attestors and provers dial it as `http://host:port`, and signers pass it to gRPC as written. <!-- [cli/internal/config/tls.go: endpointURL] --> An attestor or prover URL may carry a path but not a query or fragment, since the RPC path is appended to it. For a signer, an `http://` or `https://` URL becomes the gRPC target `host:port`, with 80 or 443 when the port is omitted, and must not carry userinfo, a path, a query or a fragment, which a gRPC target can't express; other targets, such as `dns:///host:port`, are passed to gRPC as written. <!-- [cli/internal/network/tls.go: Endpoint.GRPCTarget] -->

Set `certFile` and `keyFile` together for mTLS. The CA bundle and client certificate are read once at startup, so restart the process after rotating either. A client certificate that has expired or is not yet valid fails validation. <!-- [cli/internal/config/tls.go: TLSClientConfig.TLSConfig] -->

TLS here covers outbound connections only. The CLI's own API server on `server.listenAddr` listens without TLS. <!-- [cli/internal/server/server.go: New] -->

## Split the configuration by process

The complete example runs the relayer and both attestors together. In production, you can give each process only the blocks it uses:

- A standalone relayer needs `server`, `db`, `chains`, `relayer`, the attestors it queries, and its transaction signers.
- A standalone local attestor needs `server`, its chain, its `attestors` entry, and its attestation signer. It does not need `db` or `relayer`.

## Next steps

- [Run a standalone relayer](4-run-a-standalone-relayer.md)
- [Run a standalone attestor](3-run-a-standalone-attestor.md)
- [CLI commands](6-cli-commands.md) for command-line overrides
