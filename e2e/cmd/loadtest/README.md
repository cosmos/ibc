<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Load tests

`e2e/cmd/loadtest` is a command-line binary that puts load on a local IBC environment built by
the e2e [harness](../../internal/harness/AGENTS.md): two managed EVM Chains (Anvil by default,
Besu with `--provider besu`) joined by an attested mesh with one Attestor per Client, the
traffic test applications, and the real `ibc` relayer with auto-relay. Every run starts a
fresh environment and tears it down at the end, including on Ctrl-C (unless
`--keep-on-failure` keeps a failed run's environment).

The scenarios are not `go test` tests and never run in CI; CI only builds the binary and
runs its unit tests (`make -C e2e test-harness`).

## Running

Docker must be running. From the repository root:

```bash
make -C e2e run-relayer                                       # burst
make -C e2e run-relayer LOADTEST_FLAGS='--packets 8000 --per-block 20'    # long run
make -C e2e run-attestor
make -C e2e run-relayer LOADTEST_FLAGS='--packets 40 --wallets 10 --per-block 5'   # smoke
make -C e2e run-attestor LOADTEST_FLAGS='--duration 5s'                           # smoke
```

`run-*` builds `cli/bin/ibc` and `e2e/bin/loadtest` first. To call the binary directly,
`make -C e2e build-loadtest` and run `e2e/bin/loadtest <scenario> [flags]`
(`-h` lists every flag). The binary finds `cli/bin/ibc` through the harness source path it
was built from; `--ibc-bin` or `IBC_BIN` overrides it.

The report goes to stdout (text, or JSON with `--json`) and progress logs to stderr. The exit
status is 0 only when the scenario met its target, 1 when it did not or setup failed, and 2 on
bad usage; invalid flag values are rejected before anything starts. A run that is interrupted
or exceeds `--timeout` still reports what it measured.

### Common flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--provider` | `anvil` | Managed EVM provider: `anvil` or `besu`. |
| `--observability` | off | Export OTel telemetry from the relayer and attestors (see below). |
| `--ibc-bin` | `$IBC_BIN`, else `cli/bin/ibc` | The `ibc` binary under test. |
| `--timeout` | `30m` | Overall budget, setup and teardown included. |
| `--json` | off | Print the report as JSON instead of text. |
| `--keep-on-failure` | off | When setup or the run fails, skip teardown: containers, the relayer, and the deployment directory (relayer config, database, `relayer.log`, path logged) stay up for inspection. Clean up with `make -C e2e clean`. |
| `-v` | off | Debug logging. |

## Relayer scenario

`loadtest relayer` sends IFT transfers in both directions (A to B and B to A, or one with
`--directions a-to-b|b-to-a`) and requires every packet to reach `SUCCEEDED` in the relayer
status API. The defaults are a short, dense burst: `--packets 3000` (split evenly across
directions), `--wallets 200`, and `--per-block 150`. `--packets 8000 --per-block 20` is a
sustained, light run. Only the selected directions' routes are deployed and relayed.

The relayer runs with `ClearOnStart`, a 1s clear interval, and per-Chain packet batches of 64
with a 1s batch timeout.

How a direction runs:

1. **Wallets.** A pool of fresh keys. Each gets 1 native coin of gas and 1 IFT on the source
   Chain of every direction before the run, from one payable shim call per 64 wallets.
   Wallet *i* sends to wallet *i+1* on the destination Chain (itself in a pool of one).
2. **Pacing.** A ticker at the source Chain's block interval puts `--per-block` tokens into a
   bucket of that size, so the offered rate is `--per-block` transfers per block interval, not
   per mined block. Tokens still unused when the next tick arrives are counted as
   `backpressure` (no wallet was free to send them) and discarded.
3. **Senders.** One goroutine per wallet takes a token, claims one of the direction's packet
   slots, and submits a transfer through the harness sender, which waits for its receipt. So
   each wallet has at most one transaction in flight, which Besu's per-sender pool limit
   needs. A transfer that fails (send error, revert, or no receipt within the Chain's
   completion budget) counts as a `send_failure` and retires that wallet; one cut short by an
   interruption does not count.
4. **Achieved pacing.** The receipts' block numbers show how the sends landed: blocks with at
   least one send, the span from the first to the last of them, and the min/p50/max sends
   per such block. Managed Anvil also mines on transaction arrival, so there sends spread
   over many small blocks.
5. **Relay sweep.** While sending runs, one loop per direction lists every packet of the
   direction's source client from the relayer's `Packets` RPC (1000 per page, following
   cursors) once per poll interval (the slower Chain's block interval) and records each sent
   packet's state. It ends when sending finished and every sent packet is terminal
   (`SUCCEEDED`, `TIMED_OUT`, `REJECTED`, `RELAY_FAILED`). It gives up when relaying stalls:
   no packet sent and none newly terminal for the route's completion budget (20 block
   intervals of each Chain: 40s on Anvil, 80s on Besu). Packets still not terminal then are
   `unresolved`.
6. **Latency.** After the sweep, each succeeded packet's relay is measured from chain data:
   the receipts of its relayer-reported receive and acknowledgement transactions and the
   timestamps of the including blocks. Send to receive is the destination block of the
   receive minus the source block of the transfer; receive to acknowledgement and send to
   acknowledgement end at the source block of the acknowledgement. Block timestamps have
   one-second resolution, so the precision is the block time. Latency is reported, not
   judged: a failed chain read skips the packets that need it and shows as `latency error`.
   With `--observability`, the relayer's own `relay_duration` metric (`send_to_recv`,
   `recv_to_ack`) is in Grafana.

The report has one overall line (pass or fail, packets, wall time from the first send to the
end of the last sweep, and succeeded packets per second) and, per direction: sends (target,
sent, send failures, backpressure, ticks, send phase), achieved pacing, packet outcomes
(succeeded, timed out, rejected, relay failed, unresolved, relay drain after the last send),
and p50/p90/p95/p99/max latency for submit to receipt (wall clock, from the senders) and send
to receive, receive to acknowledgement, and send to acknowledgement (block time). A direction
passes when it sent its whole target without failures and every packet succeeded.

## Attestor scenario

`loadtest attestor` sends gRPC requests at a fixed rate to the Attestor observing chain A.
No relayer runs.

| Phase | Request | Default |
| --- | --- | --- |
| `packets` | `PacketAttestation` of a random seeded packet at its send height | 30s at 500 rps |
| `state` | `StateAttestation` at a random height in `1..latest` | 30s at 100 rps |

`--phases` lists the phases to run, in order (a phase may repeat); `--duration`,
`--packets-rps`, and `--state-rps` shape them. The `packets` phase first deploys the
applications (once), seeds 10 IFT transfers from chain A in one batch transaction, and waits
until the Attestor's latest height reaches its block; the packets are never relayed. A
`state`-only run deploys nothing.

The load is open loop: a ticker starts one call per `1/rps` whether or not earlier calls have
returned, so an attestor that cannot keep up shows as latency and timeouts, not as fewer
calls. Each call has a 3s timeout. At the phase deadline no new calls start and the phase
waits for those in flight. Latency covers succeeded calls. A phase passes when at least one
call succeeded and failed plus timed-out calls are at most `--max-error-rate` (default 0.01)
of the calls sent. A phase that cannot start, or an interruption, ends the run; an
interrupted phase is reported with the calls it sent, over the time it ran, and fails.

## Observability

`--observability` makes every managed `ibc` process (the relayer and attestors) push OTel
metrics to the local LGTM stack:

```bash
make -C cli/scripts/otel up    # Grafana on http://localhost:3001 (admin/admin)
make -C e2e run-relayer LOADTEST_FLAGS='--observability'
make -C cli/scripts/otel down
```

The "IBC Local Metrics" dashboard is provisioned automatically when the stack starts. The
stack must be running before the load test starts. See
[cli/scripts/otel](../../../cli/scripts/otel/README.md).

## After a crash

A run that is killed hard (second Ctrl-C, `kill -9`) can leave containers and processes
behind. They carry the harness labels: `make -C e2e clean-dry-run`, then `make -C e2e clean`.
