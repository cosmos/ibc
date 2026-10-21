<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

- `cli/` is its own Go module. Run `make lint` here when changing Go code or inputs that affect
  linting.
- Interfaces listed in `.mockery.yaml` have generated mocks. After changing one, run
  `make codegen-mocks` from `cli/`; CI does not regenerate them.
- Black-box tests for the CLI transport contract run from the repository root with
  `make -C e2e test`, using `E2E_FLAGS` to select tests. The default `E2E_MODE=fast` skips tests
  that require Besu; `E2E_MODE=complete` runs them.

## Documentation generated from this code

When changing commands or flags, configuration, cited behavior, or build inputs represented in the
CLI reference pages, follow [`docs/6-ibc-cli/tools/AGENTS.md`](../docs/6-ibc-cli/tools/AGENTS.md).
