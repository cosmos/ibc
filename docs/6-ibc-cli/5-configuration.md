---
title: "Configuration"
description: "Configure chains, connections, attestors, signers, and storage in ibc.yml."
---

The IBC CLI reads its configuration from `ibc.yml`. The file tells the CLI:

- which chains to connect to
- which clients form each connection
- which attestors to run or query
- which keys to use for deployment, relaying, and attestation
- where the relayer stores packet state

Run the following commands to create and validate the file:

```sh
ibc config new
ibc config validate # basic config correctness
ibc config validate relayer # extra relayer config validation
ibc config validate attestor # if you want to run attestor only
```

Values can contain `${VAR}`. The CLI replaces each variable from the environment before it parses the file. <!-- [config.go:L169](cli/internal/config/config.go#L169) -->

## Example config.yml

This configuration comes from the tutorial. In this setup, a single process runs the relayer and one attestor for each chain.

```yaml
server:
  listenAddr: 0.0.0.0:3000

db:
  type: sqlite
  url: ibc.db

chains:
  - chainId: "41001"
    evm:
      rpc: http://localhost:8545
      ws: ws://localhost:8546
      ics26Router: "0x64A6714075b7590f8b07D07a5B431409337de29B"
    deployer: deployer

  - chainId: "41002"
    evm:
      rpc: http://localhost:8745
      ws: ws://localhost:8746
      ics26Router: "0x64A6714075b7590f8b07D07a5B431409337de29B"
    deployer: deployer

relayer:
  connections:
    - alias: 41001-41002
      clientA:
        chainId: "41001"
        signer: relayer
        clientId: cli-41001-41002
        type: attestation
        autoRelay:
          enabled: true
      clientB:
        chainId: "41002"
        signer: relayer
        clientId: cli-41001-41002
        type: attestation
        autoRelay:
          enabled: true

attestors:
  - name: attestor-41001
    type: local
    chainId: "41001"
    signer: attestor-41001
    finalityOffset: 1

  - name: attestor-41002
    type: local
    chainId: "41002"
    signer: attestor-41002
    finalityOffset: 1

signers:
  - alias: deployer
    type: local
    file: deployer

  - alias: relayer
    type: local
    file: relayer

  - alias: attestor-41001
    type: local
    file: attestor-41001

  - alias: attestor-41002
    type: local
    file: attestor-41002
```

The following fields in this file are references to other fields:

| Reference | Must match |
| --- | --- |
| `clientA.chainId` and `clientB.chainId` | A `chains[].chainId` value <!-- [config.go:L300-L319](cli/internal/config/config.go#L300-L319) --> |
| `clientA.signer` and `clientB.signer` | A `signers[].alias` value <!-- [config.go:L323-L336](cli/internal/config/config.go#L323-L336) --> |
| A local attestor's `signer` | A `signers[].alias` value <!-- [config.go:L556-L559](cli/internal/config/config.go#L556-L559) --> |
| A local attestor's `chainId` | A `chains[].chainId` value <!-- [config.go:L561-L564](cli/internal/config/config.go#L561-L564) --> |
| `chains[].deployer` | A `signers[].alias` value <!-- [config.go:L241-L248](cli/internal/config/config.go#L241-L248) --> |

For example, `signer: attestor-41001` selects the signer whose alias is `attestor-41001`. `ibc config validate` reports an unresolved reference, including a local attestor whose `chainId` is not in `chains`. <!-- [config.go:L561-L564](cli/internal/config/config.go#L561-L564) --> `ibc config validate relayer` and `ibc config validate attestor` add the checks needed to run those processes.

## `server`

`server` sets the address for the relayer and attestor APIs. A process that runs both services uses one server.

<!-- GEN:config:server START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `listenAddr` | `string` | `0.0.0.0:3000` | Address the gRPC server binds. It serves the relayer and attestor APIs together. |

<!-- [config.go:L48](cli/internal/config/config.go#L48) -->

<!-- GEN:config:server END -->

Server reflection is always enabled. <!-- [bootstrap.go:L120](cli/internal/bootstrap/bootstrap.go#L120) --> <!-- [server.go:L103-L113](cli/internal/server/server.go#L103-L113) -->

## `db`

`db` configures the relayer's packet store. The store lets the relayer resume unfinished work after a restart.

<!-- GEN:config:db START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `type` | `sqlite` \| `postgres` | `sqlite` | Database backend. |
| `url` | `string` | `ibc.db` | File path for sqlite, connection string for postgres. `:memory:` is rejected. |

<!-- [config.go:L53](cli/internal/config/config.go#L53) -->

<!-- GEN:config:db END -->

`ibc relayer run` applies pending migrations at startup. Pass `--no-migrate` to disable automatic migration, or run `ibc migrate up` separately.

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

<!-- [config.go:L116](cli/internal/config/config.go#L116) -->

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

<!-- [relayer.go:L56](cli/internal/config/relayer.go#L56) -->

<!-- GEN:config:relayer:connections END -->

Client identifiers are scoped to a chain, so both ends can use the same `clientId`, as in the example above. `ibc deploy client` does this by default. <!-- [deploy.go:L256-L262](cli/cmd/ibc/deploy.go#L256-L262) -->

The two client ends must belong to different chains. A client can appear in only one configured connection on a given chain. <!-- [relayer.go:L149-L176](cli/internal/config/relayer.go#L149-L176) -->

With `autoRelay.enabled` on an end, the relayer carries that end's outgoing packets without being asked. <!-- [set.go:L27-L39](cli/internal/relay/watcher/set.go#L27-L39) --> That end's chain needs `evm.ws`, and validation fails without it. <!-- [config.go:L237-L264](cli/internal/config/config.go#L237-L264) --> Unset and `false` are the same input. <!-- [relayer.go:L118-L135](cli/internal/config/relayer.go#L118-L135) -->

### Relay settings

The relayer uses these defaults unless you override them.

<!-- GEN:config:relayer START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `dispatchPollInterval` | `duration` | `1s` | How often the dispatcher polls the store for unfinished packets. |

<!-- [relayer.go:L32](cli/internal/config/relayer.go#L32) --> <!-- [dispatcher.go:L17](cli/internal/relay/dispatch/dispatcher.go#L17) -->

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

<!-- [relayer.go:L39](cli/internal/config/relayer.go#L39) --> <!-- [evm.go:L26](cli/internal/txsubmitter/evm/evm.go#L26) --> <!-- [opts.go:L14](cli/internal/relay/pipeline/opts.go#L14) --> <!-- [opts.go:L15](cli/internal/relay/pipeline/opts.go#L15) --> <!-- [opts.go:L16](cli/internal/relay/pipeline/opts.go#L16) -->

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

<!-- [config.go:L65](cli/internal/config/config.go#L65) -->

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

<!-- [config.go:L65](cli/internal/config/config.go#L65) -->

<!-- GEN:config:attestors:remote END -->

A remote entry does not set `chainId` or `signer`. The process obtains the attestor's chain and signing address from its `Info` RPC. <!-- [remote.go:L31-L51](cli/internal/service/attestor/remote.go#L31-L51) -->

Fields from the other attestor type are rejected. A remote attestor cannot set `chainId`, and a local attestor cannot set `grpc`. <!-- [config.go:L513-L536](cli/internal/config/config.go#L513-L536) -->

Local attestor names must be unique. Two local attestors for the same chain must also use different signers. <!-- [config.go:L474-L503](cli/internal/config/config.go#L474-L503) -->

## `signers`

`signers` defines the keys referenced elsewhere in the file. Each signer has a unique alias.

### Local signer

<!-- GEN:config:signers:local START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Unique name for a signer. |
| `type` | `local` | **required** | Whether the key is a file on disk or a key held by a remote signer. |
| `file` | `string` | **required** | Key file path for a local signer. |

<!-- [config.go:L90](cli/internal/config/config.go#L90) -->

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

<!-- [config.go:L90](cli/internal/config/config.go#L90) -->

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
