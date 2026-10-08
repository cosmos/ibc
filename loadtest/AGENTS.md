<!-- SPDX-License-Identifier: Apache-2.0 -->

# Load Test Guide for AI Agents

This module (`github.com/cosmos/ibc/loadtest`) is a binary, `cmd/loadtest`, that drives load
against a harness environment. It consumes `../harness` (`environment`, `traffic`, `ibccli`)
directly; it is not a `go test` suite and must not grow `go test` entry points for scenarios.
`README.md` covers usage and the generator design.

- Layout: `internal/session` (start and tear down the environment, deployment, and relayer),
  `internal/relayerload` (head-paced IFT generator, receipt tracking, relay awaiting, report),
  `internal/attestorload` (attestor gRPC load), `internal/simpleload` (fixed-rate caller pool),
  and `internal/stats` (percentiles).
- The harness wall still applies: no `cli/internal` imports. Raw-transaction helpers the
  generator needs from `traffic` live in `harness/traffic/traffic_ift_load.go`. They are
  calldata, receipt-to-`PacketTx` parsing, signing, and terminal-state waiting. Keep signing
  keys inside `traffic`.
- Wait budgets derive from `environment.Timing` and route wait policies, never from literals
  tuned to Anvil.
- Every run must tear down on success, failure, and SIGINT: stop the relayer, close the
  Environment, then remove the deployment directory (`session.Close`).
- Unit tests cover the logic only (wallet scheduling, backpressure, nonces, percentiles,
  `SimpleLoad`) and need no Docker: `make -C loadtest test`. Lint with `make -C loadtest lint`.
- Verify a change by running the binary: a small relayer run
  (`--packets 40 --wallets 10 --per-block 5`) and `attestor --duration 5s`, then check that
  `docker ps --filter label=ibc-cli-e2e=true` is empty afterwards.
