---
title: "CLI commands"
description: "IBC CLI command reference"
---

<!-- SPDX-License-Identifier: Apache-2.0 -->

The IBC CLI contains commands for configuring, deploying, running, and monitoring IBC deployments, relayers, and attestors.

Commands are grouped here the way the binary groups them, so any of them prints its own flags with `--help`. The sections run in the order a reader meets them: `config` and `keys`, then `deploy`, then `relayer` and `attestor`, then `tx` and `query`, then `migrate`.

## Global flags

<!-- GEN:cli:global-flags START -->

| Flag | Default | Description |
|---|---|---|
| `--config <string>` | `ibc.yml` | Config file relative to home. |
| `--db <string>` |  | Database URL override. |
| `--home <string>` | `~/.ibc` | IBC home directory. |
| `--log-json` |  | Enable JSON logging. |
| `-q, --quiet` |  | Quiet mode. |

<!-- [flags.go:L38](link/internal/config/flags.go#L38) -->

<!-- GEN:cli:global-flags END -->

`--home` is where the config file, the keystore, the manifests, and the relayer's database live. A command that reads the config changes into that directory first, so a config naming `keys/alice.json` finds `~/.ibc/keys/alice.json`. <!-- [config.go:L142-L160](link/cmd/ibc/config.go#L142-L160) -->

## `config`

The config is a YAML file describing every chain, connection, attestor, and signer the other commands use.

### `ibc config add-chain`

<!-- GEN:cli:cmd:config-add-chain START -->

Add a chain entry to the config.

| Flag | Default | Description |
|---|---|---|
| `--chain-id <string>` | required | Chain ID. |
| `--deployer <string>` |  | Signer alias used by ibc deploy for this chain. |
| `--router <string>` | left blank, fill in via render-config | Ics26Router address. |
| `--rpc <string>` | required | Chain RPC URL. |
| `--ws <string>` |  | Chain websocket URL, required for chains sourcing auto-relayed routes. |

<!-- [main.go:L69](link/cmd/ibc/main.go#L69) -->

<!-- GEN:cli:cmd:config-add-chain END -->

```bash
ibc config add-chain --chain-id 41001 --rpc http://localhost:8545
```

Leave `--router` out until the chain has a router to point at. `ibc deploy render-config` prints the finished chain entries, with the router addresses filled in from the manifests.

### `ibc config new`

<!-- GEN:cli:cmd:config-new START -->

Create new config file.

| Flag | Default | Description |
|---|---|---|
| `--out` |  | Output the config to stdout. |

<!-- [main.go:L65](link/cmd/ibc/main.go#L65) -->

<!-- GEN:cli:cmd:config-new END -->

The command refuses to overwrite an existing file, so it is safe to run twice. <!-- [config.go:L95-L112](link/cmd/ibc/config.go#L95-L112) -->

### `ibc config validate`

<!-- GEN:cli:cmd:config-validate START -->

Validate the config.

| Flag | Default | Description |
|---|---|---|
| `--live` |  | Extra validation checks. |
| `--strict` |  | Fail on unknown fields in the config file. |

<!-- [main.go:L66](link/cmd/ibc/main.go#L66) -->

<!-- GEN:cli:cmd:config-validate END -->

`--live` adds the checks that need the chains. `--strict` catches a misspelled key that would otherwise be ignored.

## `keys`

Keys live in `<ibc-home>/keys/`, one file each. These can be named by signers in the config file. <!-- [keys.go:L157-L162](link/cmd/ibc/keys.go#L157-L162) -->

### `ibc keys import`

<!-- GEN:cli:cmd:keys-import START -->

Import a private key into `<ibc-home>/keys/<name>`.

| Flag | Default | Description |
|---|---|---|
| `--private-key <string>` | required | Hex-encoded private key. |
| `-p, --populate-config` |  | Write key reference to the config. |

<!-- [main.go:L84](link/cmd/ibc/main.go#L84) -->

<!-- GEN:cli:cmd:keys-import END -->

### `ibc keys list`

<!-- GEN:cli:cmd:keys-list START -->

Lists every key from `<ibc-home>/keys/`.

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:keys-list END -->

### `ibc keys new`

<!-- GEN:cli:cmd:keys-new START -->

Saves key into `<ibc-home>/keys/<name>` or prints to stdout if name is not provided.

| Flag | Default | Description |
|---|---|---|
| `-p, --populate-config` |  | Write key reference to the config. |

<!-- [main.go:L86](link/cmd/ibc/main.go#L86) -->

<!-- GEN:cli:cmd:keys-new END -->

### `ibc keys show`

<!-- GEN:cli:cmd:keys-show START -->

Show key details from `<ibc-home>/keys/<name>`; optionally print the private key.

| Flag | Default | Description |
|---|---|---|
| `--private` |  | Show private key. |

<!-- [main.go:L83](link/cmd/ibc/main.go#L83) -->

<!-- GEN:cli:cmd:keys-show END -->

> **Warning:** Do not run `keys show --private` on a shared or recorded terminal. The private key it prints controls every asset its address holds.

## `deploy`

Deploying a working connection between two chains takes `deploy core` on both, then `deploy client` on both, each client tracking the other chain. The [tutorial](2-tutorial-deploy-ibc-and-send-a-token.md) walks that through end to end.

`--chain` is required by the commands that provision something, and the table cannot show it: they check it themselves rather than declaring it required. <!-- [deploy.go:L315-L317](link/cmd/ibc/deploy.go#L315-L317) -->

`--dry-run` prints the steps a command would take and submits nothing.

Manifests are machine-generated. A command that provisions something reads the manifest, decides from it what is already done, and writes it back. <!-- [steps.go:L73-L95](link/internal/deploy/steps.go#L73-L95) -->

> **Warning:** Do not edit a manifest by hand. A deploy command decides what to skip from it, and an edit that disagrees with the chain either is overwritten or makes the next run fail.

### `ibc deploy client`

<!-- GEN:cli:cmd:deploy-client START -->

Deploy and register a light client tracking a counterparty chain.

| Flag | Default | Description |
|---|---|---|
| `--attestors <strings>` | configured attestations for the tracked chain | Attestors for the new client: addresses, attestation names, or signer aliases. |
| `--client-id <string>` | `link-<a>-<b>`, chain ids sorted | Client id. |
| `--counterparty-chain <string>` | required | Counterparty chain id the client tracks. |
| `--counterparty-client-id <string>` | `link-<a>-<b>`, chain ids sorted | Counterparty's client id. |
| `--height <uint>` | counterparty head | Initial trusted height. |
| `--threshold <uint8>` | `1` | Attestation signature threshold. |
| `--timestamp <uint>` | counterparty head | Initial trusted timestamp seconds. |
| `--type <string>` | `attestation` | Light client type. |
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L155](link/cmd/ibc/main.go#L155) -->

<!-- GEN:cli:cmd:deploy-client END -->

```bash
ibc deploy client --chain 41001 --counterparty-chain 41002 --threshold 1 --yes
```

The client lives on `--chain` and watches `--counterparty-chain`. `--attestors` names the attestors watching the counterparty, since those are the signatures this client verifies. For `remote` attestors, pass the address instead of names. <!-- [attestors.go:L15-L30](link/cmd/ibc/attestors.go#L15-L30) --> Left out entirely, the command fails when the config lists no attestors for the counterparty chain. <!-- [attestors.go:L32-L42](link/cmd/ibc/attestors.go#L32-L42) -->

Rerunning with the same `--client-id` continues that client. Rerunning with different attestors or a different threshold under the same id fails and lists the differences, because those values are fixed when the client is constructed. <!-- [steps.go:L126-L138](link/internal/deploy/steps.go#L126-L138) -->

### `ibc deploy core`

<!-- GEN:cli:cmd:deploy-core START -->

Deploy the core IBC routing stack on one chain.

| Flag | Default | Description |
|---|---|---|
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:deploy-core END -->

### `ibc deploy gmp`

<!-- GEN:cli:cmd:deploy-gmp START -->

Deploy the ICS27-GMP app on one chain.

| Flag | Default | Description |
|---|---|---|
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:deploy-gmp END -->

### `ibc deploy ift`

<!-- GEN:cli:cmd:deploy-ift START -->

Deploy an IFT token on one chain.

| Flag | Default | Description |
|---|---|---|
| `--name <string>` | required | ERC20 token name. |
| `--owner <string>` | `deployer` | Token owner address. |
| `--symbol <string>` | required | ERC20 token symbol (need not be unique). |
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L178](link/cmd/ibc/main.go#L178) -->

<!-- GEN:cli:cmd:deploy-ift END -->

```bash
ibc deploy ift --chain 41001 --name "Demo Token" --symbol DEMO --yes
```

### `ibc deploy ift-bridge`

<!-- GEN:cli:cmd:deploy-ift-bridge START -->

Register both sides of an IFT bridge between two chains' tokens.

| Flag | Default | Description |
|---|---|---|
| `--chain-a <string>` | required | First chain id. |
| `--chain-b <string>` | required | Second chain id. |
| `--client-id <string>` | `link-<a>-<b>` | Client id the bridge relays over. |
| `--ift-a <string>` | required | IFT token address on chain A. |
| `--ift-b <string>` | required | IFT token address on chain B. |
| `--send-call-constructor-a <string>` | deploy or reuse the EVM constructor | Send call constructor address on chain A. |
| `--send-call-constructor-b <string>` | deploy or reuse the EVM constructor | Send call constructor address on chain B. |
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L184](link/cmd/ibc/main.go#L184) -->

<!-- GEN:cli:cmd:deploy-ift-bridge END -->

```bash
ibc deploy ift-bridge --chain-a 41001 --ift-a 0xTokenOnA --chain-b 41002 --ift-b 0xTokenOnB --yes
```

### `ibc deploy render-config`

<!-- GEN:cli:cmd:deploy-render-config START -->

Project two deployment manifests into config sections for relaying between them (stdout).

| Flag | Default | Description |
|---|---|---|
| `--signer-a <string>` |  | Signers[] alias submitting relay txs on chainA. |
| `--signer-b <string>` |  | Signers[] alias submitting relay txs on chainB. |
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L172](link/cmd/ibc/main.go#L172) -->

<!-- GEN:cli:cmd:deploy-render-config END -->

### `ibc deploy show`

<!-- GEN:cli:cmd:deploy-show START -->

Print the recorded deployment manifest for a chain.

| Flag | Default | Description |
|---|---|---|
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:deploy-show END -->

### `ibc deploy status`

<!-- GEN:cli:cmd:deploy-status START -->

Verify recorded deployments against live chain state.

| Flag | Default | Description |
|---|---|---|
| `--chain <string>` |  | Chain ID for the chain being deployed to. |
| `--deployer <string>` |  | Signer alias override for deployment transactions. |
| `--dry-run` |  | Print the step plan without submitting transactions. |
| `--manifest-dir <string>` | `deployments` | Manifest directory relative to home. |
| `--yes` |  | Skip confirmation prompts. |

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:deploy-status END -->

## `relayer`

The relayer runs in the foreground and serves gRPC on `server.listenAddr`.

### `ibc relayer relay`

<!-- GEN:cli:cmd:relayer-relay START -->

Trigger relaying of the packets emitted by a source transaction.

| Flag | Default | Description |
|---|---|---|
| `--chain-id <string>` | required | Source chain id. |
| `--host <string>` |  | Dial this address instead of resolving from config. |
| `--tx-hash <string>` | required | Source transaction hash. |

<!-- [main.go:L93](link/cmd/ibc/main.go#L93) -->

<!-- GEN:cli:cmd:relayer-relay END -->

```bash
ibc relayer relay --chain-id 41001 --tx-hash 0xSendTxHash
```

The command asks for every packet in that transaction. The relayer takes the ones it has a configured client and route for.

### `ibc relayer run`

<!-- GEN:cli:cmd:relayer-run START -->

Run the relayer.

| Flag | Default | Description |
|---|---|---|
| `--no-migrate` |  | Skip database migrations. |

<!-- [main.go:L92](link/cmd/ibc/main.go#L92) -->

<!-- GEN:cli:cmd:relayer-run END -->

One process can be both. `relayer run` serves the relayer API and also runs every attestor the config marks `local`. <!-- [bootstrap.go:L59-L79](link/internal/bootstrap/bootstrap.go#L59-L79) --> It applies pending database migrations at startup, and `--no-migrate` skips that.

### `ibc relayer packets`

<!-- GEN:cli:cmd:relayer-packets START -->

List the packets the relayer is aware of, most recent first.

| Flag | Default | Description |
|---|---|---|
| `--all` |  | Follow every page and print the combined result. |
| `--chain-id <string>` |  | Source chain id. |
| `--cursor <string>` |  | Next_cursor from a previous response, to resume paging. |
| `--destination-chain-id <string>` |  | Destination chain id. |
| `--destination-client-id <string>` |  | Destination client id. |
| `--host <string>` |  | Dial this address instead of resolving from config. |
| `--limit <uint32>` | 100, max 1000 | Maximum packets to return. |
| `--sequence <uint>` |  | Packet sequence number. |
| `--source-client-id <string>` |  | Source client id. |
| `--state <string>` |  | Relay state (not-selected, pending, rejected, relay-failed, succeeded, timed-out). |
| `--tx-hash <string>` |  | Source transaction hash. |

<!-- [main.go:L102](link/cmd/ibc/main.go#L102) -->

<!-- GEN:cli:cmd:relayer-packets END -->

```bash
ibc relayer packets --chain-id 41001 --tx-hash 0xSendTxHash
```

Every filter flag is optional, and they combine. Results are paged: `--all` follows the pages and prints the combined result, and `--cursor` resumes from a previous one. [API](7-api.md) lists the states `--state` accepts.

## `attestor`

The attestor also runs in the foreground and serves gRPC on `server.listenAddr`. The query commands take an attestor's name as an argument rather than a flag, and `--host` dials an address directly instead of resolving that name through the config.

### `ibc attestor info`

<!-- GEN:cli:cmd:attestor-info START -->

Query a local attestor's identity.

| Flag | Default | Description |
|---|---|---|
| `--host <string>` |  | Dial this address instead of resolving from config. |

<!-- [main.go:L121](link/cmd/ibc/main.go#L121) -->

<!-- GEN:cli:cmd:attestor-info END -->

### `ibc attestor latest-height`

<!-- GEN:cli:cmd:attestor-latest-height START -->

Query a local attestor's latest attestable height.

| Flag | Default | Description |
|---|---|---|
| `--host <string>` |  | Dial this address instead of resolving from config. |

<!-- [main.go:L121](link/cmd/ibc/main.go#L121) -->

<!-- GEN:cli:cmd:attestor-latest-height END -->

### `ibc attestor run`

<!-- GEN:cli:cmd:attestor-run START -->

Run the attestor.

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:attestor-run END -->

### `ibc attestor state-attestation`

<!-- GEN:cli:cmd:attestor-state-attestation START -->

Query a local attestor for a state attestation at `--height`.

| Flag | Default | Description |
|---|---|---|
| `--height <uint>` |  | Height to attest. |
| `--host <string>` |  | Dial this address instead of resolving from config. |

<!-- [main.go:L124](link/cmd/ibc/main.go#L124) -->

<!-- GEN:cli:cmd:attestor-state-attestation END -->

## `tx`

These commands submit transactions to a chain.

### `ibc tx ift mint`

<!-- GEN:cli:cmd:tx-ift-mint START -->

Mint `--amount` of the IFT token at `--ift` to `--to`. The `--from` signer must be the token's owner.

| Flag | Default | Description |
|---|---|---|
| `--amount <string>` | required | Amount to mint, in the token's base unit. |
| `--to <string>` | required | Recipient address, or a configured signer alias. |
| `--chain <string>` | required | Chain ID the IFT token is deployed on. |
| `--from <string>` | required | Signer alias to submit the transaction with. |
| `--ift <string>` | required | IFT token address. |

<!-- [main.go:L210](link/cmd/ibc/main.go#L210) -->

<!-- GEN:cli:cmd:tx-ift-mint END -->

### `ibc tx ift send`

<!-- GEN:cli:cmd:tx-ift-send START -->

Initiate a cross-chain transfer of `--amount` of the IFT token at `--ift`, over the bridge registered for `--client-id`, to `--to` on the counterparty chain.

| Flag | Default | Description |
|---|---|---|
| `--amount <string>` | required | Amount to send, in the token's base unit. |
| `--client-id <string>` | required | Client id the bridge is registered for. |
| `--timeout <duration>` | `15m0s` | Relative send timeout. |
| `--to <string>` | required | Receiver address on the counterparty chain, or a configured signer alias. |
| `--chain <string>` | required | Chain ID the IFT token is deployed on. |
| `--from <string>` | required | Signer alias to submit the transaction with. |
| `--ift <string>` | required | IFT token address. |

<!-- [main.go:L215](link/cmd/ibc/main.go#L215) -->

<!-- GEN:cli:cmd:tx-ift-send END -->

```bash
ibc tx ift send --chain 41001 --ift 0xTokenOnA --client-id link-41001-41002 \
  --to 0xReceiver --amount 500000000000000000 --from deployer
```

The send commits a packet and emits it. Relaying can then be called with `relayer relay`.

## `query`

These commands read chain state and submit nothing.

### `ibc query ift balance`

<!-- GEN:cli:cmd:query-ift-balance START -->

Query an address's IFT token balance.

| Flag | Default | Description |
|---|---|---|
| `--address <string>` | required | Account address, or a configured signer alias. |
| `--chain <string>` | required | Chain ID the IFT token is deployed on. |
| `--ift <string>` | required | IFT token address. |

<!-- [main.go:L135](link/cmd/ibc/main.go#L135) -->

<!-- GEN:cli:cmd:query-ift-balance END -->

## `migrate`

These commands move the schema of the relayer's database up and down.

### `ibc migrate down`

<!-- GEN:cli:cmd:migrate-down START -->

Migrate DB down.

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:migrate-down END -->

`migrate down` reverses one migration, the most recent. <!-- [store_postgres.go:L110-L114](link/internal/store/store_postgres.go#L110-L114) -->

### `ibc migrate status`

<!-- GEN:cli:cmd:migrate-status START -->

Print migration status.

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:migrate-status END -->

### `ibc migrate up`

<!-- GEN:cli:cmd:migrate-up START -->

Migrate DB up.

<!-- [main.go:L53](link/cmd/ibc/main.go#L53) -->

<!-- GEN:cli:cmd:migrate-up END -->

Migrations run against the `db` block, so `--db` picks a different database for one invocation.

## Next steps

- [Configuration](5-configuration.md) for the file these commands read.
- [API](7-api.md) for the gRPC calls behind `relayer relay`, `relayer packets`, and the attestor queries.
