<!-- SPDX-License-Identifier: Apache-2.0 -->

# Harness Development Guide for AI Agents

This module (`github.com/cosmos/ibc/harness`) is the black-box harness, used by the e2e tests and
the load-test binary (`cmd/loadtest`). Its IBC CLI adapter observes the `ibc` binary only through
its public wire surface — CLI commands with JSON output, config YAML, and the status API — and
corroborates outcomes by reading chain state with its own clients.

- **The wall:** the harness never imports `cli/internal/...` or the stub's guts. It may import
  public IBC CLI command transport types, generated RPC clients, and the signer-keyfile package, while
  behavior remains observable only through the executable and HTTP surface.
- `traffic` is the workflow layer above `environment`: it deploys the test applications, writes the
  relayer configuration, starts the relayer and prover, creates and funds signers, and sends and awaits
  ICS20, GMP, and IFT packets. It never imports `testing`: functions that do I/O take a
  `context.Context`, failures are returned as errors, files go under a caller-supplied directory, and
  started processes are returned to the caller, who stops them before removing that directory and
  closing the Environment. E2e tests call it directly, through thin `testing.TB` wrappers in
  `e2e/internal/e2etest` only for the resource-owning calls; the load-test binary consumes it directly.
  Harness non-test code never imports `testing`.
- Wait budgets derive from the resolved Chain's `environment.Timing`, never from a literal tuned
  to instant Anvil. Launch-side readiness uses `chain/evm/poll.Until`; test-application effect and
  stability observation lives in `traffic`, with the bindings that interpret those effects.
- **Docker discipline.** Containers/networks carry the `ibc-cli-e2e=true` and
  `ibc-cli-run=<runid>` labels and the `ibc-cli-e2e-` name prefix, with pinned images. Anvil runs
  with `--entrypoint anvil` (PID 1, so `docker stop`'s SIGTERM reaches it and shutdown is prompt
  instead of waiting out the kill grace). Don't reintroduce a shell-wrapped entrypoint.
  StopNode/StartNode fault injection is docker pause/unpause; chain state stays in memory.
- **Load-test binary.** `cmd/loadtest` (packages under `internal/loadtest/`) drives load against a
  harness environment; `cmd/loadtest/README.md` covers usage and the design. It is not a
  `go test` suite and must not grow `go test` entry points for scenarios. `main` validates flags
  once at parse time, starts the `session` (environment, deployment, and optionally the
  relayer), runs the scenario, and closes the session; a scenario takes the session, writes its
  own report (text or JSON of the same structs), and returns an error (nil = pass), which `main`
  maps to the exit code. `relayerload` paces IFT transfers with a block-interval token ticker,
  one sender goroutine per wallet using `traffic.IFT.Transfer`, sweeps relayer status with
  `ibccli.Relayer.ClientPackets`, and measures relay latency from receipts and block
  timestamps; `attestorload` is attestor gRPC load over `simpleload`, a `rate.Limiter` with a
  try-acquire caller semaphore; `stats.Summarize` gives percentiles. Every run must tear down on
  success, failure, and SIGINT (stop the relayer, close the Environment, remove the deployment
  directory: `Session.Close`), except a failed run with `--keep-on-failure`. Unit tests cover
  the logic only (token pacing and backpressure, wallet retirement, sweep termination, latency
  arithmetic, rate accounting, reports, flag validation). Verify a change by running the binary
  (`make -C harness build`): a small relayer run (`--packets 40 --wallets 10 --per-block 5`) and
  `attestor --duration 5s`, then check that `docker ps --filter label=ibc-cli-e2e=true` is empty
  afterwards.
- Lint with `make -C harness lint` from the repository root; the shared root `.golangci.yml` covers the
  harness and excludes exported-doc mandates for this test-infrastructure surface.
