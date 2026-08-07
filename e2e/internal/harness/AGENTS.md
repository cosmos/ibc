# E2E Harness Development Guide for AI Agents

This internal package tree of the e2e module is the black-box e2e harness. Its Link adapter observes the `ibc` binary only through
its public wire surface — CLI commands with JSON output, config YAML, and the status API — and
corroborates outcomes by reading chain state with its own clients.

- **The wall:** the harness never imports `link/internal/...` or the stub's guts. It may import
  public Link command transport types, generated RPC clients, and the signer-keyfile package, while
  behavior remains observable only through the executable and HTTP surface.
- Wait budgets derive from the resolved Chain's `environment.Timing`, never from a literal tuned
  to instant Anvil. Launch-side readiness uses `chain/evm/poll.Until`; test-application effect and
  stability observation lives with the e2e-only bindings that interpret those effects.
- **Docker discipline.** Containers/networks carry the `ibc-link-e2e=true` and
  `ibc-link-run=<runid>` labels and the `ibc-link-e2e-` name prefix, with pinned images. Anvil runs
  with `--entrypoint anvil` (PID 1, so `docker stop`'s SIGTERM reaches it and shutdown is prompt
  instead of waiting out the kill grace). Don't reintroduce a shell-wrapped entrypoint.
  StopNode/StartNode fault injection is docker pause/unpause; chain state stays in memory.
- Lint with `make lint-e2e` from the repository root; the shared root `.golangci.yml` covers the
  harness and excludes exported-doc mandates for this internal test surface.
