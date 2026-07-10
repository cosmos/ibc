# IBC Link

`WIP`

## Setup

```bash
# create new config if ~/.ibc/ibc.yml
ibc config new

# migrate the database
ibc migrate up
```

## E2E

Black-box e2e tests live in [`e2e/`](e2e/README.md) with their harness in `harness/` (separate Go
modules). `make doctor-e2e && make test-e2e` runs the smoke suite.
