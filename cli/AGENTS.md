<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

- Interfaces listed in `.mockery.yaml` have generated mocks. After changing one, run
  `make codegen-mocks` from `cli/`; CI does not regenerate them.
- `make -C e2e test` defaults to `E2E_MODE=fast`, which skips tests that require Besu without
  failing; `E2E_MODE=complete` runs them. See
  [Running the acceptance tests](../e2e/README.md#running-the-acceptance-tests).

## Documentation generated from this code

When changing commands or flags, configuration, cited behavior, or build inputs represented in the
CLI reference pages, follow [`docs/6-ibc-cli/tools/AGENTS.md`](../docs/6-ibc-cli/tools/AGENTS.md).
