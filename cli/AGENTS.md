<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

- Run the relevant tests and `make lint` from this directory before finishing.
- `gen/` at the repository root holds generated code only, and its layout is fixed by an ADR — don't
  move it or put anything handwritten there. Generator scripts belong in the root `scripts/`
  directory, not beside their output. The one exception is `gen/README.md`, which documents the tree.
- From the repository root, keep `make -C e2e test` green when changing the CLI transport contract
  exercised by the repository-wide black-box tests.

## Documentation generated from this code

When changing commands or flags, configuration, cited behavior, or build inputs represented in the
CLI reference pages, follow [`docs/6-ibc-cli/tools/AGENTS.md`](../docs/6-ibc-cli/tools/AGENTS.md).

Never hand-edit between a `<!-- GEN:... START -->` marker and its `END`: the
next run overwrites it.
