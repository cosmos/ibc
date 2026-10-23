<!-- SPDX-License-Identifier: Apache-2.0 -->

# E2E harness

- Chain transaction wait budgets derive from the resolved Chain's `environment.Timing`, never from a literal tuned to instant Anvil. Provider and process startup and shutdown use separate lifecycle deadlines.
- **Docker discipline.** Containers carry `chain/evm/container.Labels` so `e2e/scripts/clean.sh` can find them. Anvil runs with `--entrypoint anvil` (PID 1, so `docker stop`'s SIGTERM reaches it and shutdown is prompt instead of waiting out the kill grace). Don't reintroduce a shell-wrapped entrypoint. StopNode/StartNode fault injection is docker pause/unpause; chain state stays in memory.
