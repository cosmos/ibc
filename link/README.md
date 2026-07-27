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

Repository-wide black-box tests live in [`../e2e/`](../e2e/README.md), with the harness in
`../e2e/internal/harness` as a separate Go module. From the repository root,
`make doctor-e2e && make test-e2e` runs the Link smoke suite.

The accepted target for the harness is documented in [IBC Environment Architecture](HARNESS-ARCHITECTURE-DESIGN.md).
