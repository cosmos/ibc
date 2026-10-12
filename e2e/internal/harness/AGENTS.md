<!-- SPDX-License-Identifier: Apache-2.0 -->

# Harness Development Guide for AI Agents

This internal package tree of the e2e module is the black-box harness. Its IBC CLI adapter observes the `ibc` binary only through
its public wire surface — CLI commands with JSON output, config YAML, and the status API — and
corroborates outcomes by reading chain state with its own clients.

- **The wall:** the harness never imports `cli/internal/...` or the stub's guts. It may import
  public IBC CLI command transport types, generated RPC clients, and the signer-keyfile package, while
  behavior remains observable only through the executable and HTTP surface.
- `traffic` is the workflow layer above `environment`: it deploys the test applications, writes the
  relayer configuration, starts the relayer and prover, creates and funds signers, and sends and awaits
  ICS20, GMP, and IFT packets. Like all harness non-test code, it never imports `testing`: functions
  that do I/O take a `context.Context`, failures are returned as errors, files go under a
  caller-supplied directory, and started processes are returned to the caller, who stops them
  before removing that directory and closing the Environment. E2e tests call it directly (`e2e/README.md` describes the few `testing.TB` wrappers in
  `e2e/internal/e2etest`).
- Wait budgets derive from the resolved Chain's `environment.Timing`, never from a literal tuned
  to instant Anvil. Launch-side readiness uses `chain/evm/poll.Until`; test-application effect and
  stability observation lives in `traffic`, with the bindings that interpret those effects.
- **Docker discipline.** Containers/networks carry the `ibc-cli-e2e=true` and
  `ibc-cli-run=<runid>` labels and the `ibc-cli-e2e-` name prefix, with pinned images. Anvil runs
  with `--entrypoint anvil` (PID 1, so `docker stop`'s SIGTERM reaches it and shutdown is prompt
  instead of waiting out the kill grace). Don't reintroduce a shell-wrapped entrypoint.
  StopNode/StartNode fault injection is docker pause/unpause; chain state stays in memory.
- Lint with `make -C e2e lint` from the repository root; the shared root `.golangci.yml` covers the
  harness and excludes exported-doc mandates for this internal test surface.
