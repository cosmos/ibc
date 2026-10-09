<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Load tests

`github.com/cosmos/ibc/harness/cmd/loadtest` is a command-line binary that puts load on a local IBC
environment built by the [harness](../..): two managed EVM Chains (Anvil by default,
Besu with `--provider besu`) joined by an attested mesh with one Attestor per Client, the
traffic test applications, and the real `ibc` relayer with auto-relay. Every run starts a
fresh environment and tears it down at the end, including on Ctrl-C (unless
`--keep-on-failure` keeps a failed run's environment).

The scenarios are not `go test` tests and never run in CI; CI only builds the binary and
runs its unit tests (`make -C harness build test`).

## Running

Docker must be running. From the repository root:

```bash
make -C harness run-relayer                                       # burst preset
make -C harness run-relayer LOADTEST_FLAGS='--preset longrun'
make -C harness run-attestor
make -C harness run-relayer LOADTEST_FLAGS='--packets 40 --wallets 10 --per-block 5'   # smoke
make -C harness run-attestor LOADTEST_FLAGS='--duration 5s'                           # smoke
```

`run-*` builds `cli/bin/ibc` and `harness/bin/loadtest` first. To call the binary directly,
`make -C harness build` and run `harness/bin/loadtest <scenario> [flags]`
(`-h` lists every flag). The binary finds `cli/bin/ibc` through the harness source path it
was built from; `--ibc-bin` or `IBC_BIN` overrides it.

The report goes to stdout (text, or JSON with `--json`) and progress logs to stderr. The exit
status is 0 only when the scenario met its target, 1 when it did not or setup failed, and 2 on
bad usage; invalid flag values are rejected before anything starts.

### Common flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--provider` | `anvil` | Managed EVM provider: `anvil` or `besu`. |
| `--observability` | off | Export OTel telemetry from the relayer and attestors (see below). |
| `--ibc-bin` | `$IBC_BIN`, else `cli/bin/ibc` | The `ibc` binary under test. |
| `--timeout` | `30m` | Overall budget, setup and teardown included. |
| `--json` | off | Print the report as JSON instead of text. |
| `--keep-on-failure` | off | When the run fails, skip teardown: containers, the relayer, and the deployment directory (relayer config, database, `relayer.log`, path logged) stay up for inspection. Clean up with `make -C e2e clean`. |
| `-v` | off | Debug logging. |

## Relayer scenario

`loadtest relayer` sends IFT transfers in both directions (A to B and B to A, or one with
`--directions a-to-b|b-to-a`) and requires every packet to reach `SUCCEEDED` in the relayer
status API.

| Preset | Packets (total) | Wallets | Per block interval |
| --- | --- | --- | --- |
| `burst` (default) | 3000 | 200 | 150 |
| `longrun` | 8000 | 200 | 20 |

`--packets`, `--wallets` (at least 2), and `--per-block` override the preset. Packets split
evenly across directions.

The relayer runs with `ClearOnStart`, a 1s clear interval, and per-Chain packet batches of 64
with a 1s batch timeout.

How a direction runs:

1. **Wallets.** A pool of fresh keys. Each gets 1 native coin of gas and 1 IFT on both
   Chains before the run, from one payable shim call per 64 wallets, so a wallet can send in
   both directions. Wallet *i* sends to wallet *i+1* on the destination Chain.
2. **Pacing.** A ticker at the source Chain's block interval puts `--per-block` tokens into a
   bucket of that size, so the offered rate is `--per-block` transfers per block interval, not
   per mined block. Tokens still unused when the next tick arrives are counted as
   `backpressure` (no wallet was free to send them) and discarded.
3. **Senders.** One goroutine per wallet takes a token, claims one of the direction's packet
   slots, and submits a transfer through the harness sender, which waits for its receipt. So
   each wallet has at most one transaction in flight, which Besu's per-sender pool limit
   needs. A transfer that fails (send error, revert, or no receipt within the Chain's
   completion budget) counts as a `send_failure` and retires that wallet.
4. **Achieved pacing.** The receipts' block numbers show how the sends landed: blocks with at
   least one send, the span from the first to the last of them, and the min/p50/max sends
   per such block. Managed Anvil also mines on transaction arrival, so there sends spread
   over many small blocks.
5. **Relay sweep.** While sending runs, one loop per direction lists every packet of the
   direction's source client from the relayer's `Packets` RPC (1000 per page, following
   cursors) once per poll interval (the slower of the route's status poll and either Chain's
   block interval) and records each sent packet's state. It ends when sending finished and
   every sent packet is terminal (`SUCCEEDED`, `TIMED_OUT`, `REJECTED`, `RELAY_FAILED`), or at
   one deadline: the last send's receipt plus the route's completion budget once per relayer
   batch the direction's packets fill (`ceil(packets / 64)`, at least one), since under load a
   packet can queue behind every batch ahead of it. Packets still not terminal then are
   `unresolved`.
6. **Latency.** After the sweep, each succeeded packet's relay is measured from chain data:
   the receipts of its relayer-reported receive and acknowledgement transactions and the
   timestamps of the including blocks. Send to receive is the destination block of the
   receive minus the source block of the transfer; receive to acknowledgement and send to
   acknowledgement end at the source block of the acknowledgement. Block timestamps have
   one-second resolution, so the precision is the block time. With `--observability`, the
   relayer's own `relay_duration` metric (`send_to_recv`, `recv_to_ack`) is in Grafana.

The report has one overall line (pass or fail, packets, wall time from the first send to the
end of the last sweep, and succeeded packets per second) and, per direction: sends (target,
sent, send failures, backpressure, ticks, send phase), achieved pacing, packet outcomes
(succeeded, timed out, rejected, relay failed, unresolved, relay drain after the last send),
and p50/p90/p95/p99/max latency for submit to receipt (wall clock, from the senders) and send
to receive, receive to acknowledgement, and send to acknowledgement (block time). A direction
passes when it sent its whole target without failures and every packet succeeded.

## Attestor scenario

`loadtest attestor` offers gRPC requests at a fixed rate to the Attestor observing chain A.
No relayer runs.

| Phase | Request | `burst` preset |
| --- | --- | --- |
| `packets` | `PacketAttestation` of a random seeded packet at its send height | 30s at 500 rps, 32 callers |
| `state` | `StateAttestation` at a random height in `1..latest` | 30s at 100 rps, 16 callers |

`--phases`, `--duration`, `--packets-rps`, `--packets-concurrency`, `--state-rps`, and
`--state-concurrency` override the preset. The `packets` phase first seeds 10 IFT transfers
from chain A in one batch transaction and waits until the Attestor's latest height reaches
its block; the packets are never relayed.

A rate limiter opens one slot per `1/rps`. A slot starts a call only if a caller is free
(fewer than the phase's concurrency in flight); otherwise it counts as `dropped`, so
`offered = sent + dropped`. Each call has a 3s timeout from the run's context, not the
phase's: at the phase deadline no new calls start and the phase waits for those in flight.
Latency covers succeeded calls. A phase passes when at least one call succeeded and failed
plus timed-out calls are at most `--max-error-rate` (default 0.01) of the calls sent.

## Observability

`--observability` makes every managed `ibc` process (the relayer and attestors) push OTel
metrics to the local LGTM stack:

```bash
make -C cli/scripts/otel up    # Grafana on http://localhost:3001 (admin/admin)
make -C harness run-relayer LOADTEST_FLAGS='--observability'
make -C cli/scripts/otel down
```

The "IBC Local Metrics" dashboard is provisioned automatically when the stack starts. The
stack must be running before the load test starts. See
[cli/scripts/otel](../../../cli/scripts/otel/README.md).

## After a crash

A run that is killed hard (second Ctrl-C, `kill -9`) can leave containers and processes
behind. They carry the harness labels: `make -C e2e clean-dry-run`, then `make -C e2e clean`
(also `make -C harness clean`).
