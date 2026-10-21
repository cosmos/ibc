<!-- SPDX-License-Identifier: Apache-2.0 -->

# Repository guidance

- Nested `AGENTS.md` files hold area guidance. Before changing files under a directory, read each
  `AGENTS.md` between that directory and this one.
- `cli/`, `e2e/`, and `gen/go/solidity-abi/` are separate Go modules.
- `gen/` contains generated code only. Preserve its existing layout and do not add handwritten files,
  except `gen/README.md`, which documents the tree.
- Keep scripts that generate code into `gen/` in the root `scripts/` directory, not beside their
  output. Read [gen/README.md](gen/README.md) when regenerating those artifacts.
- New files need an SPDX header; `make lint-license` checks them and `make lint-fix-license` adds
  them. `make run-all-checks` runs the CLI, e2e, generated-code, and license checks together.
- Postgres tests and e2e runs use disposable Docker containers on this machine; run and rerun them
  without asking. Load tests (`E2E_LOAD=true`) also start a persistent observability stack.
