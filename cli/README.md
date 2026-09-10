<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

IBC CLI deploys IBC onto a chain and runs the relayer and attestor
processes that carry packets across it.

See [`docs/6-ibc-cli/`](../docs/6-ibc-cli) for guides on operating IBC CLI.
Start with [Deploy IBC and send a token](../docs/6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md),
which brings up two chains, deploys the stack on both, and moves a token
between them.

## Prerequisites

- [Go](https://go.dev/doc/install) 1.26.4 or later

## Support

| | Supported |
| --- | --- |
| Chain types | `evm` |
| Light client types | `attestation` |

## CLI Commands

The `ibc` binary covers the whole lifecycle: creating a configuration and keys,
deploying IBC and its applications onto chains, running the relayer and attestor, and sending transactions and queries against what you
deployed. See [CLI commands](../docs/6-ibc-cli/6-cli-commands.md) for the full
reference.

```bash
# build the CLI
make build

# create a new config at ~/.ibc/ibc.yml
./bin/ibc config new

# migrate the database (skip if running migrations on relayer startup.
# `relayer run` migrates on startup automatically unless passed --no-migrate)
./bin/ibc migrate up

# generate a new local signing key, or import an existing private key
./bin/ibc keys new ecdsa <name>
./bin/ibc keys import ecdsa <name> --private-key <hex>

# deploy IBC between two configured chains (idempotent; a rerun continues a partial deployment)
./bin/ibc deploy core --chain <chainA> --yes
./bin/ibc deploy core --chain <chainB> --yes
./bin/ibc deploy client --chain <chainA> --counterparty-chain <chainB> --yes
./bin/ibc deploy client --chain <chainB> --counterparty-chain <chainA> --yes

# deploy the GMP app on each chain, so contracts can be called across the connection
./bin/ibc deploy gmp --chain <chainA> --yes
./bin/ibc deploy gmp --chain <chainB> --yes

# deploy an IFT token on each chain, then register the bridge between the two
./bin/ibc deploy ift --name <name> --symbol <symbol> --chain <chainA> --yes
./bin/ibc deploy ift --name <name> --symbol <symbol> --chain <chainB> --yes
./bin/ibc deploy ift-bridge --chain-a <chainA> --ift-a <addressA> --chain-b <chainB> --ift-b <addressB> --yes

# mint the token, then send it across a connection
./bin/ibc tx ift mint --chain <chainA> --ift <addressA> --to <recipient> --amount <baseUnits> --from <signer>
./bin/ibc tx ift send --chain <chainA> --ift <addressA> --client-id <clientID> --to <receiver> --amount <baseUnits> --from <signer>

# run the relayer and/or attestor after populating config
./bin/ibc relayer run
./bin/ibc attestor run
```

## Configuration

See [Configuration](../docs/6-ibc-cli/5-configuration.md) for the full 
configuration reference, or [`internal/config/ibc.yml`](internal/config/ibc.yml) 
for a worked example.

## E2E

Repository-wide black-box tests live in [`../e2e/`](../e2e/README.md), with the
harness in `../e2e/internal/harness` as a separate Go module. From the
repository root, `make -C e2e doctor && make -C e2e test` runs the suite.
