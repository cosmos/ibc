# IBC Link

`WIP`

## Setup

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

# run the relayer and/or attestor after populating config
./bin/ibc relayer run
./bin/ibc attestor run

# deploy IBC between two configured chains (idempotent; rerun to repair)
./bin/ibc deploy core --chain <chainA> --yes
./bin/ibc deploy core --chain <chainB> --yes
./bin/ibc deploy client --chain <chainA> --counterparty-chain <chainB> --yes
./bin/ibc deploy client --chain <chainB> --counterparty-chain <chainA> --yes
```

## Configuration

See [`docs/configuration.md`](docs/configuration.md) for the full config
reference, or [`internal/config/ibc.yml`](internal/config/ibc.yml) for a
worked example.

## E2E

Repository-wide black-box tests live in [`../e2e/`](../e2e/README.md), with the harness in
`../e2e/internal/harness` as a separate Go module. From the repository root,
`make doctor-e2e && make test-e2e` runs the Link smoke suite.

The accepted target for the harness is documented in [IBC Environment Architecture](HARNESS-ARCHITECTURE-DESIGN.md).
