<!-- SPDX-License-Identifier: Apache-2.0 -->

# Repository guidance

- `gen/` contains generated code only. Preserve its existing layout and do not add handwritten files,
  except `gen/README.md`, which documents the tree.
- Keep scripts that generate code into `gen/` in the root `scripts/` directory, not beside their
  output. Read [gen/README.md](gen/README.md) when regenerating those artifacts.
