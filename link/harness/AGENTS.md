# E2E Harness Development Guide for AI Agents

This module is the black-box e2e harness for IBC Link. It observes the `ibc` binary only through
its public wire surface — CLI commands with JSON output, config YAML, and the status API — and
corroborates outcomes by reading chain state with its own clients.

- **The wall:** this module never imports `link/internal/...` or the stub's guts. Its go.mod has no
  requirement on the link module; keep it that way. The single declaration of the wire contract is
  `ibclink/wire`.
- **Two wait primitives only.** `onchain.Await` for effect waits (budget-bounded, retries through
  transient probe errors); `internal/poll.Until` for launch-side readiness (aborts on the first
  probe error). Never hand-roll a ticker loop; never sleep. Every wait budget derives from the
  chain's `topology.TimingProfile`, never a literal tuned to instant Anvil. The one deliberate
  exception is `wait.go`'s `waitPacketStable`, a stability assertion — the condition must hold at
  every sample across the settle window, so it is not a wait-until and fits neither primitive.
- **Docker discipline.** Containers/networks carry the `ibc-link-e2e=true` and
  `ibc-link-run=<runid>` labels and the `ibc-link-e2e-` name prefix, with pinned images. Anvil runs
  with `--entrypoint anvil` (PID 1, so `docker stop`'s SIGTERM reaches it and it dumps its
  `--state` file — what makes StopNode/StartNode fault injection survive restarts). Don't
  reintroduce a shell-wrapped entrypoint.
- Lint with `make lint-e2e` from `link/`; the harness uses `link/harness/.golangci.yml`, which
  mirrors the root config minus exported-doc mandates (everything here is internal surface).
