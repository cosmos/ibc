<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

- Run the relevant tests from this directory. Run `make lint` here when changing Go code or inputs
  that affect linting.
- When changing the CLI transport contract, run affected black-box tests from the repository root
  with `make -C e2e test`, using `E2E_FLAGS` to select tests. Run the full suite for shared transport
  changes that can affect multiple flows.

## Documentation generated from this code

When changing commands or flags, configuration, cited behavior, or build inputs represented in the
CLI reference pages, follow [`docs/6-ibc-cli/tools/AGENTS.md`](../docs/6-ibc-cli/tools/AGENTS.md).
