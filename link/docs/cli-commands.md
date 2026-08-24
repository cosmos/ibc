---
title: "CLI commands"
description: "Every ibc command, by the part of the system it drives."
---

The IBC CLI contains commands for configuring, deploying, running, and monitoring IBC deployments, relayers, and attestors.

Commands are grouped here the way the binary groups them, so any of them prints its own flags with `--help`. The sections run in the order a reader meets them: `config` and `keys`, then `deploy`, then `relayer` and `attestor`, then `tx` and `query`, then `migrate`.

Every command also appears in [All commands](#all-commands) for lookup.

## Global flags

<!-- GEN:cli:global-flags START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--config` | `string` | `ibc.yml` | Config file relative to home |
| `--db` | `string` |  | Database URL override |
| `--home` | `string` | `~/.ibc` | IBC home directory |
| `--log-json` | `bool` |  | Enable JSON logging |
| `-q, --quiet` | `bool` |  | Quiet mode |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:global-flags END -->

`--home` is where the config file, the keystore, the manifests, and the relayer's database live. A command that reads the config changes into that directory first, so a config naming `keys/alice.json` finds `~/.ibc/keys/alice.json`. <!-- [config.go:L142-L160](link/cmd/ibc/config.go#L142-L160) -->

`--db` overrides the `db` block of the config for one invocation. A URL beginning `postgres://` selects postgres, and anything else is treated as a sqlite path. <!-- [config.go:L709-L715](link/internal/config/config.go#L709-L715) -->

## `config`

One YAML file describes every chain, connection, attestor, and signer the other commands use.

<!-- GEN:cli:group:config START -->

| Command | What it does |
|---|---|
| `ibc config add-chain` | Add a chain entry to the config |
| `ibc config new` | Create new config file |
| `ibc config validate` | Validate the config |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:config END -->

`config new` refuses to overwrite an existing file, so it is safe to run twice. `--out` prints the file to stdout instead of writing it. <!-- [config.go:L95-L112](link/cmd/ibc/config.go#L95-L112) -->

`config validate` is used to validate the file. `--live` adds the checks that need the chains. `--strict` fails on a key the config package does not recognize, which catches a misspelling that would otherwise be ignored.

### `ibc config add-chain`

Add chain entries to the config file.

<!-- GEN:cli:flags:config-add-chain START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain-id` | `string` | **required** | chain ID |
| `--deployer` | `string` |  | signer alias used by ibc deploy for this chain |
| `--router` | `string` | left blank, fill in via render-config | ics26Router address |
| `--rpc` | `string` | **required** | chain RPC URL |

<!-- [main.go:L74](link/cmd/ibc/main.go#L74) -->

<!-- GEN:cli:flags:config-add-chain END -->

```bash
ibc config add-chain --chain-id 41001 --rpc http://localhost:8545
```

Leave `--router` out until the chain has a router to point at. `ibc deploy render-config` prints the finished chain entries, with the router addresses filled in from the manifests.

## `keys`

Keys live in `<ibc-home>/keys/`, one file each. These can be named by signers in the config file. <!-- [config.go:L591-L615](link/internal/config/config.go#L591-L615) -->

<!-- GEN:cli:group:keys START -->

| Command | What it does |
|---|---|
| `ibc keys import` | Import a private key into `<ibc-home>/keys/<name>` |
| `ibc keys list` | Lists every key from `<ibc-home>/keys/` |
| `ibc keys new` | Saves key into `<ibc-home>/keys/<name>` or prints to stdout if name is not provided |
| `ibc keys show` | Show key details from `<ibc-home>/keys/<name>`; optionally print the private key |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:keys END -->

`keys show --private` prints a private key to the terminal.


## `deploy`

Each of these commands deploys one layer onto `--chain`, and records what it deployed in that chain's manifest.

<!-- GEN:cli:group:deploy START -->

| Command | What it does |
|---|---|
| `ibc deploy client` | Deploy and register a light client tracking a counterparty chain |
| `ibc deploy core` | Deploy the core IBC routing stack on one chain |
| `ibc deploy gmp` | Deploy the ICS27-GMP app on one chain |
| `ibc deploy ift` | Deploy an IFT token on one chain |
| `ibc deploy ift-bridge` | Register both sides of an IFT bridge between two chains' tokens |
| `ibc deploy render-config` | Project two deployment manifests into config sections for relaying between them (stdout) |
| `ibc deploy show` | Print the recorded deployment manifest for a chain |
| `ibc deploy status` | Verify recorded deployments against live chain state |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:deploy END -->

Deploying a working connection between two chains takes `deploy core` on both, then `deploy client` on both, each client tracking the other chain. The [tutorial](/ibc-cli/tutorial-deploy-ibc-and-send-a-token) walks that through end to end.

### Flags every deploy command accepts

<!-- GEN:cli:group-flags:deploy START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain` | `string` |  | chain ID for the chain being deployed to |
| `--deployer` | `string` |  | signer alias override for deployment transactions |
| `--dry-run` | `bool` |  | print the step plan without submitting transactions |
| `--manifest-dir` | `string` | `deployments` | manifest directory relative to home |
| `--yes` | `bool` |  | skip confirmation prompts |

<!-- [main.go:L133](link/cmd/ibc/main.go#L133) -->

<!-- GEN:cli:group-flags:deploy END -->

`--dry-run` prints the steps a command would take and submits nothing.

`--manifest-dir` holds one generated manifest per chain, recording the addresses the deployment created.

### `ibc deploy client`

Deploys a light client on `--chain` that tracks `--counterparty-chain`, then registers it on the router.

<!-- GEN:cli:flags:deploy-client START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--attestors` | `strings` | configured attestations for the tracked chain | attestors for the new client: addresses, attestation names, or signer aliases |
| `--client-id` | `string` | `link-<a>-<b>`, chain ids sorted | client id |
| `--counterparty-chain` | `string` | **required** | counterparty chain id the client tracks |
| `--counterparty-client-id` | `string` | `link-<a>-<b>`, chain ids sorted | counterparty's client id |
| `--height` | `uint` | counterparty head | initial trusted height |
| `--threshold` | `uint8` | `1` | attestation signature threshold |
| `--timestamp` | `uint` | counterparty head | initial trusted timestamp seconds |
| `--type` | `string` | `attestation` | light client type |

<!-- [main.go:L140](link/cmd/ibc/main.go#L140) -->

<!-- GEN:cli:flags:deploy-client END -->

```bash
ibc deploy client --chain 41001 --counterparty-chain 41002 --threshold 1 --yes
```

The client lives on `--chain` and watches `--counterparty-chain`. `--attestors` names the attestors watching the counterparty, since those are the signatures this client verifies.

For `remote` attestors, pass the address instead of names. <!-- [attestors.go:L15-L30](link/cmd/ibc/attestors.go#L15-L30) -->

### `ibc deploy ift`

Deploy an IFT contract.

<!-- GEN:cli:flags:deploy-ift START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--name` | `string` | **required** | ERC20 token name |
| `--owner` | `string` | `deployer` | token owner address |
| `--symbol` | `string` | **required** | ERC20 token symbol (need not be unique) |

<!-- [main.go:L163](link/cmd/ibc/main.go#L163) -->

<!-- GEN:cli:flags:deploy-ift END -->

```bash
ibc deploy ift --chain 41001 --name "Demo Token" --symbol DEMO --yes
```

### `ibc deploy ift-bridge`

Registers both sides of a bridge.

<!-- GEN:cli:flags:deploy-ift-bridge START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain-a` | `string` | **required** | first chain id |
| `--chain-b` | `string` | **required** | second chain id |
| `--client-id` | `string` | `link-<a>-<b>` | client id the bridge relays over |
| `--ift-a` | `string` | **required** | IFT token address on chain A |
| `--ift-b` | `string` | **required** | IFT token address on chain B |
| `--send-call-constructor-a` | `string` | deploy or reuse the EVM constructor | send call constructor address on chain A |
| `--send-call-constructor-b` | `string` | deploy or reuse the EVM constructor | send call constructor address on chain B |

<!-- [main.go:L169](link/cmd/ibc/main.go#L169) -->

<!-- GEN:cli:flags:deploy-ift-bridge END -->

```bash
ibc deploy ift-bridge --chain-a 41001 --ift-a 0xTokenOnA --chain-b 41002 --ift-b 0xTokenOnB --yes
```

## `relayer`

These commands are for running the relayer which serves gRPC on `server.listenAddr`.

<!-- GEN:cli:group:relayer START -->

| Command | What it does |
|---|---|
| `ibc relayer relay` | Trigger relaying of the packets emitted by a source transaction |
| `ibc relayer run` | Run the relayer |
| `ibc relayer status` | Query per-packet relay status for a source transaction |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:relayer END -->

`relayer run` serves the relayer API and also runs every attestor the config marks `local`. <!-- [bootstrap.go:L59-L79](link/internal/bootstrap/bootstrap.go#L59-L79) -->


### `ibc relayer relay`

Asks a running relayer to deliver the packets a transaction emitted.

<!-- GEN:cli:flags:relayer-relay START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain-id` | `string` | **required** | source chain id |
| `--host` | `string` |  | dial this address instead of resolving from config |
| `--tx-hash` | `string` | **required** | source transaction hash |

<!-- [main.go:L96](link/cmd/ibc/main.go#L96) -->

<!-- GEN:cli:flags:relayer-relay END -->

```bash
ibc relayer relay --chain-id 41001 --tx-hash 0xSendTxHash
```

The command asks for every packet in that transaction. The relayer takes the ones it has a configured client and route for.

### `ibc relayer status`

<!-- GEN:cli:flags:relayer-status START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain-id` | `string` | **required** | source chain id |
| `--host` | `string` |  | dial this address instead of resolving from config |
| `--tx-hash` | `string` | **required** | source transaction hash |

<!-- [main.go:L96](link/cmd/ibc/main.go#L96) -->

<!-- GEN:cli:flags:relayer-status END -->

```bash
ibc relayer status --chain-id 41001 --tx-hash 0xSendTxHash
```

The output carries one entry per packet in the transaction, each with its state and the transactions seen for it so far. [API](/ibc-cli/api) lists the states.

## `attestor`

The attestor also runs in the foreground and serves gRPC on `server.listenAddr`.

<!-- GEN:cli:group:attestor START -->

| Command | What it does |
|---|---|
| `ibc attestor info` | Query a local attestor's identity |
| `ibc attestor latest-height` | Query a local attestor's latest attestable height |
| `ibc attestor run` | Run the attestor |
| `ibc attestor state-attestation` | Query a local attestor for a state attestation at `--height` |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:attestor END -->

The query commands take an attestor's name as an argument rather than a flag. Their `--host` flag dials an address directly, instead of resolving that name through the config.

## `tx`

These commands submit transactions to a chain.

<!-- GEN:cli:group:tx START -->

| Command | What it does |
|---|---|
| `ibc tx ift mint` | Mint `--amount` of the IFT token at `--ift` to `--to`. The `--from` signer must be the token's owner. |
| `ibc tx ift send` | Initiate a cross-chain transfer of `--amount` of the IFT token at `--ift`, over the bridge registered for `--client-id`, to `--to` on the counterparty chain. |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:tx END -->

### Flags every IFT transaction accepts

<!-- GEN:cli:group-flags:tx-ift START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain` | `string` | **required** | chain ID the IFT token is deployed on |
| `--from` | `string` | **required** | signer alias to submit the transaction with |
| `--ift` | `string` | **required** | IFT token address |

<!-- [main.go:L188](link/cmd/ibc/main.go#L188) -->

<!-- GEN:cli:group-flags:tx-ift END -->

### `ibc tx ift send`

Sends tokens to the counterparty chain over a registered bridge.

<!-- GEN:cli:flags:tx-ift-send START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--amount` | `string` | **required** | amount to send, in the token's base unit |
| `--client-id` | `string` | **required** | client id the bridge is registered for |
| `--timeout` | `duration` | `15m0s` | relative send timeout |
| `--to` | `string` | **required** | receiver address on the counterparty chain, or a configured signer alias |

<!-- [main.go:L200](link/cmd/ibc/main.go#L200) -->

<!-- GEN:cli:flags:tx-ift-send END -->

```bash
ibc tx ift send --chain 41001 --ift 0xTokenOnA --client-id link-41001-41002 \
  --to 0xReceiver --amount 500000000000000000 --from deployer
```

The send commits a packet and emits it. Relaying can then be called with `relayer relay`.

## `query`

These commands read chain state and submit nothing.

<!-- GEN:cli:group:query START -->

| Command | What it does |
|---|---|
| `ibc query ift balance` | Query an address's IFT token balance |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:query END -->

### Flags every IFT query accepts

<!-- GEN:cli:group-flags:query-ift START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain` | `string` | **required** | chain ID the IFT token is deployed on |
| `--ift` | `string` | **required** | IFT token address |

<!-- [main.go:L114](link/cmd/ibc/main.go#L114) -->

<!-- GEN:cli:group-flags:query-ift END -->

## `migrate`

These commands move the schema of the relayer's database up and down.

<!-- GEN:cli:group:migrate START -->

| Command | What it does |
|---|---|
| `ibc migrate down` | Migrate DB down |
| `ibc migrate status` | Print migration status |
| `ibc migrate up` | Migrate DB up |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:group:migrate END -->

Migrations run against the `db` block, so `--db` picks a different database for one invocation. `migrate down` reverses one migration, the most recent. <!-- [store_postgres.go:L110-L114](link/internal/store/store_postgres.go#L110-L114) -->

## Flags on the remaining commands

The commands above are the ones whose flags need explaining. The rest take one or two flags each, and they are all here.

<!-- GEN:cli:remaining-flags START -->

| Command | Flag | Type | Default | Description |
|---|---|---|---|---|
| `ibc attestor info` | `--host` | `string` |  | dial this address instead of resolving from config |
| `ibc attestor latest-height` | `--host` | `string` |  | dial this address instead of resolving from config |
| `ibc attestor state-attestation` | `--height` | `uint` |  | height to attest |
| `ibc attestor state-attestation` | `--host` | `string` |  | dial this address instead of resolving from config |
| `ibc config new` | `--out` | `bool` |  | output the config to stdout |
| `ibc config validate` | `--live` | `bool` |  | extra validation checks |
| `ibc config validate` | `--strict` | `bool` |  | fail on unknown fields in the config file |
| `ibc deploy render-config` | `--signer-a` | `string` |  | signers[] alias submitting relay txs on chainA |
| `ibc deploy render-config` | `--signer-b` | `string` |  | signers[] alias submitting relay txs on chainB |
| `ibc keys import` | `--populate-config` | `bool` |  | append the resulting key as a signers entry in the config file |
| `ibc keys import` | `--private-key` | `string` |  | hex-encoded private key |
| `ibc keys new` | `--populate-config` | `bool` |  | append the resulting key as a signers entry in the config file |
| `ibc keys show` | `--private` | `bool` |  | show private key |
| `ibc query ift balance` | `--address` | `string` | **required** | account address, or a configured signer alias |
| `ibc relayer run` | `--no-migrate` | `bool` |  | skip database migrations |
| `ibc tx ift mint` | `--amount` | `string` | **required** | amount to mint, in the token's base unit |
| `ibc tx ift mint` | `--to` | `string` | **required** | recipient address, or a configured signer alias |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:remaining-flags END -->

## All commands

Cobra also generates `completion` and `help`, which are not listed here.

<!-- GEN:cli:all-commands START -->

| Command | What it does |
|---|---|
| `ibc attestor info` | Query a local attestor's identity |
| `ibc attestor latest-height` | Query a local attestor's latest attestable height |
| `ibc attestor run` | Run the attestor |
| `ibc attestor state-attestation` | Query a local attestor for a state attestation at `--height` |
| `ibc config add-chain` | Add a chain entry to the config |
| `ibc config new` | Create new config file |
| `ibc config validate` | Validate the config |
| `ibc deploy client` | Deploy and register a light client tracking a counterparty chain |
| `ibc deploy core` | Deploy the core IBC routing stack on one chain |
| `ibc deploy gmp` | Deploy the ICS27-GMP app on one chain |
| `ibc deploy ift` | Deploy an IFT token on one chain |
| `ibc deploy ift-bridge` | Register both sides of an IFT bridge between two chains' tokens |
| `ibc deploy render-config` | Project two deployment manifests into config sections for relaying between them (stdout) |
| `ibc deploy show` | Print the recorded deployment manifest for a chain |
| `ibc deploy status` | Verify recorded deployments against live chain state |
| `ibc keys import` | Import a private key into `<ibc-home>/keys/<name>` |
| `ibc keys list` | Lists every key from `<ibc-home>/keys/` |
| `ibc keys new` | Saves key into `<ibc-home>/keys/<name>` or prints to stdout if name is not provided |
| `ibc keys show` | Show key details from `<ibc-home>/keys/<name>`; optionally print the private key |
| `ibc migrate down` | Migrate DB down |
| `ibc migrate status` | Print migration status |
| `ibc migrate up` | Migrate DB up |
| `ibc query ift balance` | Query an address's IFT token balance |
| `ibc relayer relay` | Trigger relaying of the packets emitted by a source transaction |
| `ibc relayer run` | Run the relayer |
| `ibc relayer status` | Query per-packet relay status for a source transaction |
| `ibc tx ift mint` | Mint `--amount` of the IFT token at `--ift` to `--to`. The `--from` signer must be the token's owner. |
| `ibc tx ift send` | Initiate a cross-chain transfer of `--amount` of the IFT token at `--ift`, over the bridge registered for `--client-id`, to `--to` on the counterparty chain. |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:all-commands END -->

## Next steps

- [Configuration](/ibc-cli/configuration) for the file these commands read.
- [API](/ibc-cli/api) for the gRPC calls behind `relayer relay`, `relayer status`, and the attestor queries.
