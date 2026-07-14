# IBC Link

`WIP`

## Setup

```bash
# build the CLI
make build

# create a new config at ~/.ibc/ibc.yml
./bin/ibc config new

# migrate the database
./bin/ibc migrate up
```

## E2E

Black-box e2e tests live in [`e2e/`](e2e/README.md) with their harness in `harness/` (separate Go
modules). `make doctor-e2e && make test-e2e` runs the smoke suite.

The accepted target for the harness is documented in [IBC Environment Architecture](HARNESS-ARCHITECTURE-DESIGN.md).
