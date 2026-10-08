<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Load tests

`github.com/cosmos/ibc/harness/cmd/loadtest` is a command-line binary that puts load on a local IBC
environment built by the [harness](../..): two managed EVM Chains (Anvil by default,
Besu with `--provider besu`) joined by an attested mesh with one Attestor per Client, the
traffic test applications, and the real `ibc` relayer with auto-relay. Every run starts a
fresh environment and tears it down at the end, including on Ctrl-C.

The scenarios are not `go test` tests and never run in CI; CI only builds the binary and
runs its unit tests (`make -C harness build test`).

## Running

Docker must be running. From the repository root:

```bash
make -C harness run-relayer                                       # burst preset
make -C harness run-relayer LOADTEST_FLAGS='--preset longrun'
make -C harness run-attestor
make -C harness run-relayer LOADTEST_FLAGS='--packets 40 --wallets 10 --per-block 5'   # smoke
```

`run-*` builds `cli/bin/ibc` and `harness/bin/loadtest` first. To call the binary directly,
`make -C harness build` and run `harness/bin/loadtest <scenario> [flags]`
(`-h` lists every flag). The binary finds `cli/bin/ibc` through the harness source path it
was built from; `--ibc-bin` or `IBC_BIN` overrides it.

The text report goes to stdout and progress logs to stderr. The exit status is 0 only when
the scenario met its target, 1 when it did not or setup failed, and 2 on bad usage.

### Common flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--provider` | `anvil` | Managed EVM provider: `anvil` or `besu`. |
| `--observability` | off | Export OTel telemetry from the relayer and attestors (see below). |
| `--ibc-bin` | `$IBC_BIN`, else `cli/bin/ibc` | The `ibc` binary under test. |
| `--timeout` | `30m` | Overall budget, setup and teardown included. |
| `--json` | off | Print the report as JSON instead of text. |
| `--keep-on-failure` | off | Keep the deployment directory (relayer config, database, `relayer.log`) when the run fails; its path is logged. Containers are still removed. |
| `-v` | off | Debug logging. |

## Relayer scenario

`loadtest relayer` sends IFT transfers in both directions (A to B and B to A, or one with
`--directions a-to-b|b-to-a`) and requires every packet to reach `SUCCEEDED` in the relayer
status API.

| Preset | Packets (total) | Wallets | Per block |
| --- | --- | --- | --- |
| `burst` (default) | 3000 | 200 | 150 |
| `longrun` | 8000 | 200 | 20 |

`--packets`, `--wallets`, and `--per-block` override the preset. Packets split evenly across
directions.

The relayer runs with `ClearOnStart`, a 1s clear interval, and per-Chain packet batches of 64
with a 1s batch timeout.

How the generator works, per direction:

1. **Wallets.** A pool of fresh keys. Each gets 10 native coins of gas and 10 IFT on both
   Chains before the run, so a wallet can send in both directions. Wallet *i* sends to wallet
   *i+1* on the destination Chain.
2. **Pacing.** It subscribes to new heads over the source Chain's WebSocket. On a head it
   dispatches up to `--per-block` transfers, at most once per Chain block interval. Managed
   Anvil mixes interval and on-arrival mining, so every submitted transaction mines its own
   block. Pacing on heads alone would feed back on itself; the block-interval debounce keeps it
   per block.
3. **One transaction in flight per wallet per Chain.** Besu limits transactions per sender in
   its pool, so a wallet sends again only after its previous transaction was mined. When
   fewer wallets are free than a dispatch wants, the shortfall is counted as `backpressure`
   and nothing is queued; later heads send the rest.
4. **Open loop.** Transactions are signed locally with tracked nonces (fetched once, and
   fetched again after a send error or dropped transaction) and submitted without waiting
   for receipts. Gas is estimated once with 50% headroom. Fees follow the harness sender:
   `2 x baseFee + tip`.
5. **Receipts.** On each head it fetches the receipts of every new block, falling back to
   per-transaction lookups where `eth_getBlockReceipts` is unavailable, and matches its own
   transactions. Packets are located from the `SendPacket` events in those receipts, not
   from relayer discovery. A transaction still unmined after the Chain's completion budget is
   counted as `dropped`.
6. **Relay.** Packets stream to a bounded worker pool, 8 per direction, as soon as they are
   located. Each worker waits for the packet's terminal relayer state. It polls no faster
   than one block. Its budget is the route's completion budget once per relayer batch the
   direction's packets fill. Both values come from the Chains' `environment.Timing`.

The report gives per-direction and total counts: transactions attempted, sent, included,
reverted, and dropped; send errors; backpressure; packets succeeded, timed out, rejected,
relay-failed, and unresolved. It also gives send-phase and relay-drain durations, plus
p50/p90/p95/p99 latency for send to inclusion and inclusion to observed `SUCCEEDED`. The
second latency is observed by polling, so it is quantized by the poll interval, which is one
block. Precise relay lifecycle timestamps are FOU-1740.

## Attestor scenario

`loadtest attestor` seeds 10 ICS20 transfers from chain A, waits until the relayer delivers
them, and then offers gRPC requests at a fixed rate to the Attestor observing chain A:

| Phase | Request | `burst` preset |
| --- | --- | --- |
| `packets` | `PacketAttestation` of a random seeded packet at its send height | 30s at 500 rps, 32 callers |
| `state` | `StateAttestation` at a random height in `1..latest` | 30s at 100 rps, 16 callers |

`--phases`, `--duration`, `--packets-rps`, `--packets-concurrency`, `--state-rps`, and
`--state-concurrency` override the preset. Each call has a 3s timeout. When every caller is
busy, an offered call is counted as `dropped`. A phase fails if no request succeeded.

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
