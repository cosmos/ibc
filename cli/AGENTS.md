<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

- Run the relevant tests from this directory. Run `make lint` here when changing Go code or inputs
  that affect linting.
- From the repository root, keep `make -C e2e test` green when changing the CLI transport contract
  exercised by the repository-wide black-box tests.

## Documentation generated from this code

When changing commands or flags, configuration, cited behavior, or build inputs represented in the
CLI reference pages, follow [`docs/6-ibc-cli/tools/AGENTS.md`](../docs/6-ibc-cli/tools/AGENTS.md).

Never hand-edit between a `<!-- GEN:... START -->` marker and its `END`: the
next run overwrites it.
