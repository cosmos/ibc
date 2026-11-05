<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

- Interfaces listed in `.mockery.yaml` have generated mocks. After changing one, run
  `make codegen-mocks` from `cli/`; CI does not regenerate them.
- `make -C e2e test` defaults to `E2E_MODE=fast`, which skips tests that require Besu without
  failing; `E2E_MODE=complete` runs them. See
  [Running the acceptance tests](../e2e/README.md#running-the-acceptance-tests).

## Documentation generated from this code

The reference pages under `docs/ibc/05-ibc-cli/04-reference/` carry tables
generated from this directory. When changing commands or flags, configuration,
cited behavior, or build inputs represented on those pages, **the documentation
must be regenerated before the pull request is opened**, and the regenerated
pages belong in the same pull request as the code.

Follow [`docs/scripts/AGENTS.md`](../docs/scripts/AGENTS.md).
It covers what to run, what the generator refuses and why, and what you may
write yourself.

Never hand-edit between a `{/* GEN:... START */}` marker and its `END`: the
next run overwrites it.
