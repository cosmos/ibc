---
title: "Configuration"
description: "Every key in ibc.yml, the one file the relayer, the attestor, and the deployment engine all read."
---

The IBC CLI reads one YAML file. By default it is `ibc.yml` inside the IBC home directory, `~/.ibc`, and `--home` and `--config` move both. The relayer, the attestor, and `ibc deploy` all read the same file, so one file describes a whole deployment.

Values may contain `${VAR}`, which is expanded from the environment before the file is parsed. That is how a key or a connection string stays out of the file. <!-- [config.go:L169](link/internal/config/config.go#L169) -->

`ibc config new` writes a file with the defaults filled in, and `ibc config validate` checks one. Run `validate` before starting a process: it resolves every cross-reference described below and names the first one that does not resolve. <!-- [config.go:L190-L224](link/internal/config/config.go#L190-L224) -->

The file has six top-level blocks, and not every process reads all of them. <!-- [config.go:L37-L44](link/internal/config/config.go#L37-L44) -->

| Block | Read by | Purpose |
|---|---|---|
| `server` | relayer, attestor | The address the gRPC server binds |
| `db` | relayer | Where the relayer stores the packets it is tracking |
| `chains` | relayer, attestor, deploy | Every chain the rest of the file refers to |
| `relayer` | relayer | The connections to relay, and per-chain relay settings |
| `attestors` | relayer, attestor | Attestors this process runs, and attestors it queries |
| `signers` | relayer, attestor, deploy | The keys, under the aliases the rest of the file uses |

## How the blocks refer to each other

Four of the blocks name entries in the others, and a name that does not resolve is a startup error rather than a silent fallback.

- Each connection names two chain IDs and two client IDs, and both chains must appear in `chains`. <!-- [config.go:L300-L319](link/internal/config/config.go#L300-L319) -->
- Each client end names a signer alias, which must appear in `signers`. That signer submits the relay transactions on that end's chain. <!-- [config.go:L323-L336](link/internal/config/config.go#L323-L336) -->
- Each local attestor names a chain ID and a signer alias. <!-- [config.go:L232-L239](link/internal/config/config.go#L232-L239) -->
- A chain's `deployer` names a signer alias, which `ibc deploy` submits with. <!-- [config.go:L241-L248](link/internal/config/config.go#L241-L248) -->

A process only needs the blocks it uses. An attestor never opens a database, so an attestor-only file can leave out `db` and inherit its defaults. It needs no `relayer` block either.

## `server`

The gRPC server serves the relayer API and the attestor API on one address, so a process running both parts listens once.

<!-- GEN:config:server START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `listenAddr` | `string` | `0.0.0.0:3000` | Address the gRPC server binds. It serves the relayer and attestor APIs together. |

<!-- [config.go:L47](link/internal/config/config.go#L47) -->

<!-- GEN:config:server END -->

```yaml
server:
  listenAddr: 0.0.0.0:3000
```

One port serves gRPC, gRPC-Web, and Connect, and reflection is always registered, so a client needs no proto files. <!-- [bootstrap.go:L120](link/internal/bootstrap/bootstrap.go#L120) --> <!-- [server.go:L103-L113](link/internal/server/server.go#L103-L113) -->

## `db`

The relayer stores every packet it is tracking, which is what lets it restart without losing work. Nothing else in the file affects that store.

<!-- GEN:config:db START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `type` | `sqlite` \| `postgres` | `sqlite` | Database backend. |
| `url` | `string` | `ibc.db` | File path for sqlite, connection string for postgres. `:memory:` is rejected. |

<!-- [config.go:L52](link/internal/config/config.go#L52) -->

<!-- GEN:config:db END -->

```yaml
db:
  type: sqlite
  url: ibc.db
```

`ibc relayer run` applies pending migrations at startup unless `--no-migrate` is passed. `ibc migrate up` applies them separately, against the same `db` block.

## `chains`

A list. Each entry is one chain, named by the ID the chain reports, and every other block refers to a chain by that ID.

<!-- GEN:config:chains START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain's id, as the chain reports it. |
| `evm` | block | optional | EVM connection details for the chain. See the table below. |
| `deployer` | `string` | optional | Optional signer alias used by `ibc deploy` for this chain. |

<!-- [config.go:L115](link/internal/config/config.go#L115) -->

<!-- GEN:config:chains END -->

### `chains[].evm`

<!-- GEN:config:chains:evm START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `rpc` | `string` | **required** | JSON-RPC endpoint for the chain. |
| `ics26Router` | `string` | optional | Address of the ICS26 router on the chain. |

<!-- [config.go:L133](link/internal/config/config.go#L133) -->

<!-- GEN:config:chains:evm END -->

```yaml
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
```

`ics26Router` is the address `ibc deploy core` deploys. Leave it blank when creating the file, and fill it in from the manifest afterwards. `ibc deploy render-config` prints the finished blocks for two chains, including their router addresses. <!-- [main.go:L74-L79](link/cmd/ibc/main.go#L74-L79) -->

A `deployer` alias must name a `local` signer, because deployment needs the raw key rather than a remote signing call. <!-- [deploy.go:L123-L155](link/cmd/ibc/deploy.go#L123-L155) --> `deploy show`, `deploy status`, and `deploy render-config` read nothing on chain and work without one.

## `relayer`

The relayer relays over the connections listed here, and over no others. A packet on a client this block does not name is not relayed by this process.

<!-- GEN:config:relayer START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `dispatchPollInterval` | `duration` | `5s` | How often the dispatcher polls the store for unfinished packets. |
| `chainOverrides` | list | optional | Per-chain relay settings. See the table below. |
| `connections` | list | optional | The connections this relayer relays over. See the table below. |

<!-- [relayer.go:L29](link/internal/config/relayer.go#L29) --> <!-- [dispatcher.go:L17](link/internal/relay/dispatch/dispatcher.go#L17) -->

<!-- GEN:config:relayer END -->

### `relayer.chainOverrides`

A list, one entry per chain whose relay behavior differs from the defaults. Every key is optional, so an override may set one and leave the rest.

<!-- GEN:config:relayer:chainOverrides START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain these settings apply to. |
| `evm` | block | optional | EVM fee settings for the chain. See the table below. |
| `txSubmissionDelay` | `duration` | `2s` | Minimum delay between two transaction submissions on the chain. |
| `packetBatchSize` | `int` | `50` | How many packets the relayer puts in one transaction. |
| `packetBatchTimeout` | `duration` | `10s` (receive and acknowledge), `1m` (timeout) | How long the relayer waits to fill a batch before submitting it. |

<!-- [relayer.go:L36](link/internal/config/relayer.go#L36) --> <!-- [evm.go:L26](link/internal/txsubmitter/evm/evm.go#L26) --> <!-- [opts.go:L14](link/internal/relay/pipeline/opts.go#L14) --> <!-- [opts.go:L15](link/internal/relay/pipeline/opts.go#L15) --> <!-- [opts.go:L16](link/internal/relay/pipeline/opts.go#L16) -->

<!-- GEN:config:relayer:chainOverrides END -->

Batching is per direction, not per chain. Receive batching follows the destination chain, and acknowledgement and timeout batching follow the source chain. So an override on one chain shapes the traffic arriving there and the traffic leaving it differently. <!-- [opts.go:L34-L71](link/internal/relay/pipeline/opts.go#L34-L71) -->

### `relayer.chainOverrides[].evm`

<!-- GEN:config:relayer:evm START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `gasFeeCapMultiplier` | `float64` | optional | Multiplies the fee cap the node suggests. |
| `gasTipCapMultiplier` | `float64` | optional | Multiplies the tip cap the node suggests. |

<!-- [relayer.go:L45](link/internal/config/relayer.go#L45) -->

<!-- GEN:config:relayer:evm END -->

Both multipliers apply to what the node suggests for the next transaction. Left unset, the relayer submits the node's suggestion unchanged. <!-- [evm.go:L229-L237](link/internal/txsubmitter/evm/evm.go#L229-L237) -->

### `relayer.connections`

A list. Each entry is one connection, and the relayer relays it in both directions.

<!-- GEN:config:relayer:connections START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Name for the connection, unique in the file. |
| `clientA` | block | **required** | One end of the connection. See the table below. |
| `clientB` | block | **required** | The other end, on a different chain. Same keys as `clientA`. |

<!-- [relayer.go:L53](link/internal/config/relayer.go#L53) -->

<!-- GEN:config:relayer:connections END -->

### A client end

`clientA` and `clientB` take the same keys. Each describes one side of the connection: a light client on one chain, tracking the other side as its counterparty.

<!-- GEN:config:relayer:clientEnd START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** | The chain this end's client lives on. |
| `signer` | `string` | **required** | `signers` alias that submits relay transactions on this chain. |
| `clientId` | `string` | **required** | The light client's id on this chain. |
| `type` | `attestation` | **required** | Light client type. |

<!-- [relayer.go:L61](link/internal/config/relayer.go#L61) -->

<!-- GEN:config:relayer:clientEnd END -->

```yaml
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
```

Both ends carry the same `clientId` here, which is normal: client ids are per-chain, so one name identifies the connection on both sides. That is what `ibc deploy client` produces by default. <!-- [deploy.go:L256-L262](link/cmd/ibc/deploy.go#L256-L262) -->

The two ends must be on different chains, and no two connections may name the same client on the same chain. <!-- [relayer.go:L149-L155](link/internal/config/relayer.go#L149-L155) --> <!-- [relayer.go:L173-L176](link/internal/config/relayer.go#L173-L176) -->

<!-- TODO(autorelay): a clientEnd also parses an autoRelay block, which nothing outside the config package reads at ibc@951c1a9. It is left out of the table above on purpose. Add it, and a sentence here, once a consumer lands. -->

## `attestors`

A list. Each entry is one attestor, and `type` decides what the entry means.

A `local` attestor is one this process runs and signs with. A `remote` attestor is one this process queries over gRPC. Both kinds appear in the same list, and a relayer reads it to decide who to ask for attestations. <!-- [config.go:L57-L59](link/internal/config/config.go#L57-L59) -->

<!-- GEN:config:attestors START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `chainId` | `string` | **required** for `local` | The chain this attestor watches. |
| `name` | `string` | **required** | The attestor's own self-reported identity. Not required unique. |
| `type` | `local` \| `remote` | **required** | Whether this process runs the attestor or queries it. |
| `signer` | `string` | **required** for `local` | The signer used to sign attestations. |
| `finalityOffset` | `uint` | `local` only | Zero attests up to the chain's `finalized` tag; n > 0 attests up to `latest` - n instead. |
| `grpc` | `string` | **required** for `remote` | Bare host:port. |

<!-- [config.go:L64](link/internal/config/config.go#L64) -->

<!-- GEN:config:attestors END -->

```yaml
attestors:
  - name: attestor-41001
    type: local
    chainId: "41001"
    signer: attestor-41001
    finalityOffset: 1
  - name: attestor-41002
    type: remote
    grpc: 127.0.0.1:3001
```

A `remote` entry carries no `chainId` because the process asks the attestor. It queries the `Info` RPC at that address and takes the chain and the signing address from the response. <!-- [remote.go:L31-L51](link/internal/service/attestor/remote.go#L31-L51) -->

Setting a key that belongs to the other kind is an error, not an ignored line. A `remote` entry with a `chainId`, or a `local` entry with a `grpc` address, fails validation. <!-- [config.go:L513-L536](link/internal/config/config.go#L513-L536) -->

Two local attestors may not share a name, and two local attestors on one chain may not share a signer. The same signer backing local attestors on two different chains is allowed. <!-- [config.go:L474-L503](link/internal/config/config.go#L474-L503) -->

## `signers`

A list of the keys this process can sign with, each under an alias. Every alias the rest of the file uses, for a relay transaction, an attestation, or a deployment, resolves here.

<!-- GEN:config:signers START -->

| Key | Type | Default or required | Description |
|---|---|---|---|
| `alias` | `string` | **required** | Unique name for a signer. |
| `type` | `local` \| `remote` | **required** | Whether the key is a file on disk or a key held by a remote signer. |
| `file` | `string` | **required** for `local` | Key file path for a local signer. |
| `grpc` | `string` | **required** for `remote` | Address for a remote signer. |
| `remoteKeyId` | `string` | **required** for `remote` | KMS key ID for a remote signer. |

<!-- [config.go:L89](link/internal/config/config.go#L89) -->

<!-- GEN:config:signers END -->

```yaml
signers:
  - alias: relayer
    type: local
    file: relayer
  - alias: deployer
    type: remote
    grpc: cosmos-kms.example.com:9090
    remoteKeyId: deployer-key-id
```

A `local` signer reads a key file. The path is tried as written, then with `.json` appended, then under `keys/`, so `alice` resolves to `~/.ibc/keys/alice.json`. Validation fails if none of those paths exists. <!-- [config.go:L591-L615](link/internal/config/config.go#L591-L615) -->

A `remote` signer holds no key material. It names a gRPC address and a key ID, and the signing happens in the remote signer.

## Next steps

- [CLI commands](/ibc-cli/cli-commands) for the flags that override these values on one invocation.
- [Run a standalone attestor](/ibc-cli/run-a-standalone-attestor) for an attestor-only file.
- [Run a standalone relayer](/ibc-cli/run-a-standalone-relayer) for a relayer-only file.
