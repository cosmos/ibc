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

Run the following commands to create and check the file:

```sh
ibc config new
ibc config validate
```

Values can contain `${VAR}`. The CLI replaces each variable from the environment before it parses the file. Use environment variables for values such as database connection strings. <!-- [config.go:L169](link/internal/config/config.go#L169) -->

## Complete example

This configuration connects two local EVM chains with attestation clients. One process runs the relayer and one attestor for each chain.

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
      ics26Router: "0x64A6714075b7590f8b07D07a5B431409337de29B"
    deployer: deployer

  - chainId: "41002"
    evm:
      rpc: http://localhost:8745
      ics26Router: "0x64A6714075b7590f8b07D07a5B431409337de29B"
    deployer: deployer

relayer:
  connections:
    - alias: 41001-41002
      clientA:
        chainId: "41001"
        signer: relayer
        clientId: link-41001-41002
        type: attestation
      clientB:
        chainId: "41002"
        signer: relayer
        clientId: link-41001-41002
        type: attestation

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

The names in this file are references:

| Reference | Must match |
| --- | --- |
| `clientA.chainId`, `clientB.chainId`, and a local attestor's `chainId` | A `chains[].chainId` value <!-- [config.go:L300-L319](link/internal/config/config.go#L300-L319) --> |
| `clientA.signer` and `clientB.signer` | A `signers[].alias` value <!-- [config.go:L323-L336](link/internal/config/config.go#L323-L336) --> |
| A local attestor's `signer` | A `signers[].alias` value <!-- [config.go:L232-L239](link/internal/config/config.go#L232-L239) --> |
| `chains[].deployer` | A local `signers[].alias` value <!-- [config.go:L241-L248](link/internal/config/config.go#L241-L248) --> |

For example, `signer: attestor-41001` selects the signer whose alias is `attestor-41001`. An unresolved reference causes validation to fail.

## `server`

`server` sets the address for the relayer and attestor APIs. A process that runs both services uses one server.

<!-- GEN:config:server START -->

<!-- GEN:config:server END -->

The port serves gRPC, gRPC-Web, and Connect. Server reflection is enabled. <!-- [bootstrap.go:L120](link/internal/bootstrap/bootstrap.go#L120) --> <!-- [server.go:L103-L113](link/internal/server/server.go#L103-L113) -->

## `db`

`db` configures the relayer's packet store. The store lets the relayer resume unfinished work after a restart.

<!-- GEN:config:db START -->

<!-- GEN:config:db END -->

`ibc relayer run` applies pending migrations at startup. Pass `--no-migrate` to disable automatic migration, or run `ibc migrate up` separately.

## `chains`

`chains` lists every chain used elsewhere in the file.

<!-- GEN:config:chains START -->

<!-- GEN:config:chains END -->

`ibc deploy core` deploys the router. You can omit `ics26Router` before deployment, then add the deployed address from the manifest. `ibc deploy render-config` renders the completed chain blocks for both sides of a connection. <!-- [main.go:L74-L79](link/cmd/ibc/main.go#L74-L79) -->

The deployer must be a local signer, because deployment requires direct access to the key. <!-- [deploy.go:L123-L155](link/cmd/ibc/deploy.go#L123-L155) -->

## `relayer`

### Connections

`relayer.connections` selects the connections this process relays. Each entry identifies one client on each chain, and the relayer handles traffic in both directions.

<!-- GEN:config:relayer:connections START -->

<!-- GEN:config:relayer:connections END -->

Client identifiers are scoped to a chain, so both ends can use the same `clientId`, as in the example above. `ibc deploy client` does this by default. <!-- [deploy.go:L256-L262](link/cmd/ibc/deploy.go#L256-L262) -->

The two client ends must belong to different chains. A client can appear in only one configured connection on a given chain. <!-- [relayer.go:L149-L176](link/internal/config/relayer.go#L149-L176) -->

### Relay settings

The relayer uses these defaults unless you override them.

<!-- GEN:config:relayer START -->

<!-- GEN:config:relayer END -->

{G:config:relayer:chainOverrides}

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

Receive batches use the destination chain's settings. Acknowledgement and timeout batches use the source chain's settings. <!-- [opts.go:L34-L71](link/internal/relay/pipeline/opts.go#L34-L71) -->

## `attestors`

`attestors` lists the attestors that the process runs or queries. A `local` attestor watches a configured chain and signs with a configured signer. A `remote` attestor runs elsewhere and is queried over gRPC.

### Local attestor

<!-- GEN:config:attestors:local START -->

<!-- GEN:config:attestors:local END -->

Set `finalityOffset` according to the chain's finality model. Do not copy the example value into a production configuration without confirming that it is safe for the chain.

### Remote attestor

```yaml
attestors:
  - name: attestor-41002
    type: remote
    grpc: attestor.example.com:3000
```

<!-- GEN:config:attestors:remote START -->

<!-- GEN:config:attestors:remote END -->

A remote entry does not set `chainId` or `signer`. The process obtains the attestor's chain and signing address from its `Info` RPC. <!-- [remote.go:L31-L51](link/internal/service/attestor/remote.go#L31-L51) -->

Fields from the other attestor type are rejected. A remote attestor cannot set `chainId`, and a local attestor cannot set `grpc`. <!-- [config.go:L513-L536](link/internal/config/config.go#L513-L536) -->

Local attestor names must be unique. Two local attestors for the same chain must also use different signers. <!-- [config.go:L474-L503](link/internal/config/config.go#L474-L503) -->

## `signers`

`signers` defines the keys referenced elsewhere in the file. Each signer has a unique alias.

### Local signer

<!-- GEN:config:signers:local START -->

<!-- GEN:config:signers:local END -->

For `file: relayer`, the CLI checks `relayer`, `relayer.json`, and the `keys` directory under the IBC home directory. The final path is typically `~/.ibc/keys/relayer.json`. <!-- [config.go:L591-L615](link/internal/config/config.go#L591-L615) -->

### Remote signer

```yaml
signers:
  - alias: relayer
    type: remote
    grpc: signer.example.com:9090
    remoteKeyId: relayer-key-id
```

<!-- GEN:config:signers:remote START -->

<!-- GEN:config:signers:remote END -->

The remote signer holds the key material and performs signing.

## Split the configuration by process

The complete example runs the relayer and both attestors together. In production, you can give each process only the blocks it uses:

- A standalone relayer needs `server`, `db`, `chains`, `relayer`, the attestors it queries, and its transaction signers.
- A standalone local attestor needs `server`, its chain, its `attestors` entry, and its attestation signer. It does not need `db` or `relayer`.

## Next steps

- [Run a standalone relayer](/ibc-cli/run-a-standalone-relayer)
- [Run a standalone attestor](/ibc-cli/run-a-standalone-attestor)
- [CLI commands](/ibc-cli/cli-commands) for command-line overrides
