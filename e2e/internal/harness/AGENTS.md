# E2E Harness Development Guide for AI Agents

This repository-internal module is the black-box e2e harness. Its Link adapter observes the `ibc` binary only through
its public wire surface — CLI commands with JSON output, config YAML, and the status API — and
corroborates outcomes by reading chain state with its own clients.

- **The wall:** this module never imports `link/internal/...` or the stub's guts. Its go.mod has no
  requirement on the link module; keep it that way. The single declaration of the wire contract is
  `ibclink/wire`.
- Wait budgets derive from the resolved Chain's `environment.Timing`, never from a literal tuned
  to instant Anvil. Launch-side readiness uses `internal/poll.Until`; test-application effect and
  stability observation lives with the e2e-only bindings that interpret those effects.
- **Docker discipline.** Containers/networks carry the `ibc-link-e2e=true` and
  `ibc-link-run=<runid>` labels and the `ibc-link-e2e-` name prefix, with pinned images. Anvil runs
  with `--entrypoint anvil` (PID 1, so `docker stop`'s SIGTERM reaches it and it dumps its
  `--state` file — what makes StopNode/StartNode fault injection survive restarts). Don't
  reintroduce a shell-wrapped entrypoint.
- Lint with `make lint-e2e` from the repository root; the harness uses `e2e/internal/harness/.golangci.yml`, which
  mirrors the root config minus exported-doc mandates (everything here is internal surface).
