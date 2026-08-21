---
title: "CLI commands"
description: "Every ibc command, grouped by what you are doing with it."
---

The `ibc` binary does three jobs: it deploys IBC onto a chain, it relays packets, and it attests to chain heights. Its commands are grouped here by task, because that is the order a reader meets them. Every command also appears in [All commands](#all-commands) for lookup.

Commands are grouped into subcommands, and any of them prints its own flags with `--help`.

## Flags every command accepts

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

## Set up a config and keys

Start here on a new machine. `config new` writes a file with the defaults filled in, and the rest of this group fills in what the defaults cannot know.

<!-- GEN:cli:task:setup START -->

| Command | What it does |
|---|---|
| `ibc config new` | Create new config file |
| `ibc config add-chain` | Add a chain entry to the config |
| `ibc config validate` | Validate the config |
| `ibc keys new` | Saves key into `<ibc-home>/keys/<name>` or prints to stdout if name is not provided |
| `ibc keys import` | Import a private key into `<ibc-home>/keys/<name>` |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:task:setup END -->

`config new` refuses to overwrite an existing file, so it is safe to run twice. `--out` prints the file to stdout instead of writing it. <!-- [config.go:L95-L112](link/cmd/ibc/config.go#L95-L112) -->

`config validate` resolves every cross-reference in the file and stops at the first one that does not resolve. `--live` adds the checks that need the chains. `--strict` fails on a key the config package does not recognize, which catches a misspelling that would otherwise be ignored.

### `ibc config add-chain`

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

## Deploy IBC

Each of these commands deploys one layer onto one chain, named by `--chain`, and records what it deployed in that chain's manifest.

<!-- GEN:cli:task:deploy START -->

| Command | What it does |
|---|---|
| `ibc deploy core` | Deploy the core IBC routing stack on one chain |
| `ibc deploy client` | Deploy and register a light client tracking a counterparty chain |
| `ibc deploy gmp` | Deploy the ICS27-GMP app on one chain |
| `ibc deploy ift` | Deploy an IFT token on one chain |
| `ibc deploy ift-bridge` | Register both sides of an IFT bridge between two chains' tokens |
| `ibc deploy render-config` | Project two deployment manifests into config sections for relaying between them (stdout) |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:task:deploy END -->

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

`--dry-run` prints the steps a command would take and submits nothing. It is the cheapest way to see what a deployment will do before it spends gas.

`--manifest-dir` holds one manifest per chain, recording every address the deployment created. `deploy show` prints one, and `deploy render-config` reads two.

Manifests are machine-generated. Every deploy command reads the manifest, decides from it what is already done, and writes it back, which is what makes a rerun continue rather than start over. <!-- [steps.go:L73-L95](link/internal/deploy/steps.go#L73-L95) --> A hand edit is lost on the next run.

> **Warning:** Do not edit a manifest by hand. The next deploy command overwrites it, and until then the deployment decides what to skip from values that no longer match the chain.

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

The client lives on `--chain` and watches `--counterparty-chain`. `--attestors` therefore names the attestors watching the counterparty, since those are the signatures this client verifies.

Each `--attestors` value is resolved three ways in order. An attestor name resolves through that entry's signer, a signer alias resolves to its address, and anything else passes through as an address. <!-- [attestors.go:L15-L30](link/cmd/ibc/attestors.go#L15-L30) --> A `remote` attestor's name is rejected, since its address is not known from the config, so pass the address instead.

Left out, `--attestors` becomes every attestor the config lists for the counterparty chain. If the config lists none, the command fails rather than deploying a client with an empty attestor set. <!-- [attestors.go:L32-L42](link/cmd/ibc/attestors.go#L32-L42) -->

`--height` and `--timestamp` set the point the client starts trusting. Left out, both come from the counterparty's current head. A head of zero on a fresh chain becomes one, since a client rejects zero as an initial trusted height. <!-- [deploy.go:L286-L299](link/cmd/ibc/deploy.go#L286-L299) -->

Rerunning `deploy client` with the same `--client-id` continues that client, which is how a deployment interrupted halfway finishes. Rerunning with different attestors or a different threshold under the same id fails and lists the differences, because those values are fixed when the client is constructed. <!-- [steps.go:L126-L138](link/internal/deploy/steps.go#L126-L138) -->

Deploying another client pair means passing a new `--client-id` and a matching `--counterparty-client-id`.

### `ibc deploy ift`

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

A symbol need not be unique, so two chains can each hold a token called `DEMO`. They are separate tokens until `deploy ift-bridge` links them.

### `ibc deploy ift-bridge`

Registers both sides of a bridge, so the token on chain A and the token on chain B become two ends of one asset.

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

This command names both chains itself, so it does not take `--chain`. The send call constructor flags exist for reusing a constructor already deployed; left out, the command deploys or reuses one on each chain.

## Run a relayer or an attestor

Both processes run in the foreground and serve gRPC on `server.listenAddr`.

<!-- GEN:cli:task:run START -->

| Command | What it does |
|---|---|
| `ibc relayer run` | Run the relayer |
| `ibc attestor run` | Run the attestor |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:task:run END -->

One process can be both. `relayer run` serves the relayer API and also runs every attestor the config marks `local`. <!-- [bootstrap.go:L59-L79](link/internal/bootstrap/bootstrap.go#L59-L79) -->

`relayer run` applies pending database migrations at startup. `--no-migrate` skips that, for an operator who runs `migrate up` separately.

## Move packets

Nothing watches the chains, so a packet moves when something asks for it. These commands are the asking.

<!-- GEN:cli:task:move START -->

| Command | What it does |
|---|---|
| `ibc tx ift mint` | Mint `--amount` of the IFT token at `--ift` to `--to`. The `--from` signer must be the token's owner. |
| `ibc tx ift send` | Initiate a cross-chain transfer of `--amount` of the IFT token at `--ift`, over the bridge registered for `--client-id`, to `--to` on the counterparty chain. |
| `ibc relayer relay` | Trigger relaying of the packets emitted by a source transaction |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:task:move END -->

`tx ift mint` and `tx ift send` submit transactions to a chain. `relayer relay` calls a running relayer's API, so it needs a relayer to be up.

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

The send commits a packet and emits it. Relaying it is a separate step, which `relayer relay` starts.

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

The command asks for every packet in that transaction, and the relayer takes the ones it has a configured client and route for. <!-- [relayer.go:L114-L120](link/cmd/ibc/relayer.go#L114-L120) --> `--host` dials an address directly, for a relayer other than the one this config describes.

## Inspect what is there

<!-- GEN:cli:task:inspect START -->

| Command | What it does |
|---|---|
| `ibc relayer status` | Query per-packet relay status for a source transaction |
| `ibc deploy status` | Verify recorded deployments against live chain state |
| `ibc deploy show` | Print the recorded deployment manifest for a chain |
| `ibc query ift balance` | Query an address's IFT token balance |
| `ibc attestor info` | Query a local attestor's identity |
| `ibc attestor latest-height` | Query a local attestor's latest attestable height |
| `ibc attestor state-attestation` | Query a local attestor for a state attestation at `--height` |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:task:inspect END -->

`deploy show` prints what a deployment recorded. `deploy status` goes further and checks those records against the chains, which is what catches a manifest describing contracts that are no longer there.

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

### Flags every IFT query accepts

<!-- GEN:cli:group-flags:query-ift START -->

| Flag | Type | Default | Description |
|---|---|---|---|
| `--chain` | `string` | **required** | chain ID the IFT token is deployed on |
| `--ift` | `string` | **required** | IFT token address |

<!-- [main.go:L114](link/cmd/ibc/main.go#L114) -->

<!-- GEN:cli:group-flags:query-ift END -->

The attestor queries take an attestor name as an argument rather than a flag. Their `--host` flag dials an address directly, instead of resolving that name through the config.

## Maintain a deployment

<!-- GEN:cli:task:maintain START -->

| Command | What it does |
|---|---|
| `ibc migrate up` | Migrate DB up |
| `ibc migrate down` | Migrate DB down |
| `ibc migrate status` | Print migration status |
| `ibc keys list` | Lists every key from `<ibc-home>/keys/` |
| `ibc keys show` | Show key details from `<ibc-home>/keys/<name>`; optionally print the private key |

<!-- [main.go:L58](link/cmd/ibc/main.go#L58) -->

<!-- GEN:cli:task:maintain END -->

`keys show --private` prints a private key to the terminal.

> **Warning:** Do not run `keys show --private` on a shared or recorded terminal. The private key it prints controls every asset its address holds.

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
