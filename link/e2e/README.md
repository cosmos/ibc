# E2E Test Surface

The runnable test surface is intentionally small. The goal is the black-box harness shape: start
local chains, compile a topology into an `ibc` config, drive the SUT through public CLI/HTTP
contracts, and verify outcomes from chain state or status JSON.

## Suites

| Suite | Package | What it proves |
| --- | --- | --- |
| Setup | `./setup` | Config validation, live RPC checks, deploy metadata, and on-chain fixture verification. |
| Relayer flow | `./ibclink` | IFT/GMP happy paths plus pending/status and restart recovery behavior. |
| External chain | `./external` | Relaying through an RPC the harness does not own. |
| Negative flow | `./negative` | Representative fault, timeout/refund, and error-ack cases. |

## Setup

All `make` targets run from `link/`. You need Go and Docker.

```sh
make doctor-e2e
make build build-stub
```

`make build` produces the real `bin/ibc`, `make build-stub` the stub `bin/ibc-stub`; the harness
routes each wire command per the routing table in `../harness/ibclink/runner.go` (`IBC_BIN` /
`IBC_STUB_BIN` override the paths). `forge` is needed only for `make fixtures`.

## Running

- `make test-e2e` — smoke suite in the default instant-Anvil lane; the loop while editing setup or
  relayer flows.
- `make test-e2e E2E_LANE=anvil-interval` — same flows on 2s-block Anvil (per-chain timing
  profiles instead of instant mining).
- `make test-e2e E2E_LANE=besu` — portable tests against Besu.
- `make test-e2e E2E_PKGS=./negative` — representative negative/fault cases.
- `make test-e2e E2E_PKGS=./ibclink E2E_FLAGS='-run TestIFTTransfer -count=1'` — one local loop
  (`E2E_PKGS` is relative to `e2e`).

The runner also accepts `-e2e.lane` through `E2E_FLAGS`.
`e2etest.RequireAnvilLane(t)` dedups Anvil-pinned tests to a single run during the Anvil lane pass.
A test that pins a lane must carry the matching gate so it runs once, in that lane's pass, instead
of redundantly in every lane. Chain operations negotiate their capabilities at runtime.

Debugging: `KEEP_AFTER_TEST=1` leaves everything running for inspection — chains, the harness
workdir (sqlite, compiled config, logs), and the relayer daemon (its `*-daemon-N.pid` file in the
workdir names the process to sweep). `E2E_ARTIFACT_DIR=.e2e-artifacts` writes `diagnostics.txt`
per env; failed runs include the full chain/stub/status capture. After a hard crash:
`make clean-e2e-dry-run`, then `make clean-e2e`.

## Writing A Test

Use linear tests with `e2etest` for startup and `require.NoError` at the call site:

```go
func TestIFTTransfer(t *testing.T) {
    run := e2etest.Start(t, e2etest.SelectedTopology(t))
    ctx := t.Context()

    out, err := run.IFT(ctx, harness.IFT{
        Route:  topology.RouteAtoB,
        Amount: big.NewInt(1_000),
    })
    require.NoError(t, err)
    require.NoError(t, out.VerifyComplete(ctx))
}
```

- Each test gets a fresh environment; for table-driven cases, start one inside each `t.Run`
  subtest (no shared-env reset yet — see `docs/reset-design.md`).
- Assert packet outcomes through their harness methods (`VerifyComplete`, `VerifyPendingStable`,
  `VerifyTimedOut`, `VerifyErrorAck`), never via `harness/ibclink/wire` or the daemon directly.
- Receivers and GMP targets are plain strings (`harness.IFT.Receiver`, `harness.GMP.Target`); leave
  them empty to default.
- Per-chain lifecycle, faults, and block control hang off `run.Chain(id)` handles:
  `.WithPausedMining`, `.Mine`, `.AdvanceTime`, `.StopNode`/`.StartNode`, and `.EVM()` for the
  concrete `*evm.EVMClient`. A capability the chain does not advertise fails on use with
  `harness.ErrCapabilityMissing` (match with `errors.Is`), so these are safe to call and gate on.
- Use `e2etest.StartHarness` when the test is about validate/deploy only (chains up, config
  compiled, driver ready, no daemon).

## Picking A Topology

A topology is a shape (which chains and routes — the test's choice) bound by a lane
(which node backends, timing, chain ids — the runner's choice). The `topology.TwoEVM()` shape carries
the auto routes `topology.RouteAtoB` / `topology.RouteBtoA`. Lanes are `topology.Anvil`,
`topology.AnvilInterval`, and `topology.Besu` — one per `E2E_LANE` value.

- `e2etest.SelectedTopology(t)` — the selected lane bound to the two-EVM shape; use it for portable
  relayer tests. A test that needs manual relaying derives its variant at the point of use, e.g.
  `e2etest.SelectedTopology(t).WithManualRelay(topology.RouteAtoB)`.
- `topology.Anvil(topology.TwoEVM())` + `e2etest.RequireAnvilLane(t)` — pin tests that need instant
  Anvil's pause/mine/advance-time controls or node stop/restart behavior.
- Ad-hoc composition (`./external`, the cross-route test) — build `ChainSpec`s directly for an
  arrangement no shape covers (external RPCs via `Provision{Mode: ProvisionExternal, RPCURL}`,
  three-chain fan-in); `Topology.Validate` checks it at `harness.Start`, and chain ids come from
  the reserved-ranges note in `harness/topology/lane.go`. A shape constructor exists only once more
  than one call site needs the arrangement.
