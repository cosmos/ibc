# Repository E2E Test Surface

This repository-level surface hosts black-box acceptance suites. The current suites drive IBC Link through its public CLI, config, readiness, relay, and status contracts. The accepted Link harness design is documented in [IBC Environment Architecture](../link/HARNESS-ARCHITECTURE-DESIGN.md); the synthetic traffic is transitional and does not shape that architecture.

`internal/harness/environment` realizes Chains and protocol resources. Tests then deploy the temporary MockIFT, MockGMP, and Counter applications and start the synthetic relayer explicitly, so process restarts, manual relay, fault injection, and teardown remain visible in the behavior under test.

## Suites

| Suite | Package | What it proves |
| --- | --- | --- |
| Setup | `./setup` | Config validation, live RPC checks, typed test-application receipts, and on-chain deployment verification. |
| Relayer flow | `./ibclink` | IFT/GMP happy paths plus pending/status and restart recovery behavior. |
| External chain | `./external` | Relaying through an attached RPC that `Environment` does not own. |
| Negative flow | `./negative` | Representative fault, timeout/refund, and error-ack cases. |

## Running the suite

Run targets from the repository root:

```sh
make doctor-e2e
make build-link
make test-e2e
```

`make build-link` produces `link/bin/ibc`; `IBC_BIN` overrides that path. Link explicitly composes temporary handlers for config validation, test-application deployment, and Relayer execution into this binary.

The same tests can select different Chain declarations:

- `make test-e2e` uses instant-mining Anvil for fast feedback.
- `make test-e2e E2E_LANE=anvil-interval` uses two-second Anvil blocks.
- `make test-e2e E2E_LANE=besu` uses Besu QBFT.
- `make test-e2e E2E_PKGS=./negative` runs the opt-in negative suite.
- `make test-e2e E2E_PKGS=./ibclink E2E_FLAGS='-run TestIFTTransfer_AutoRelay -count=1'` runs one test repeatedly.

`-e2e.lane` in `E2E_FLAGS` overrides `E2E_LANE`. Tests pinned to instant Anvil call `e2etest.RequireAnvilLane(t)` so a matrix runs them only in that lane. After a hard crash, use `make clean-e2e-dry-run` and then `make clean-e2e`.

## Writing a test

The setup sequence is deliberately explicit:

```go
func TestIFTTransfer_AutoRelay(t *testing.T) {
    env := e2etest.Start(t, e2etest.SelectedSuite(t))
    signers := synthetic.NewSigners(t)
    route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
    driver, deployment := synthetic.Deploy(t, env, signers, route)
    ift := synthetic.BindIFT(t, env, deployment, signers, route)
    relayer := synthetic.StartRelayer(t, driver, env)

    transfer, err := ift.Send(t.Context(), testapp.IFTRequest{Amount: big.NewInt(1_000)})
    require.NoError(t, err)
    destination, err := env.Chain(route.Destination)
    require.NoError(t, err)
    _, err = synthetic.AwaitState(
        t.Context(), relayer, transfer.Packet(), relayercmd.PacketComplete, destination.Timing(),
    )
    require.NoError(t, err)
    require.NoError(t, transfer.VerifyDelivered(t.Context()))
    require.NoError(t, transfer.VerifyEscrowed(t.Context()))
}
```

`Environment` owns Chain clients and protocol resources. A route-bound `testapp` binding hides only application ABI, transaction, event, and state mechanics. The test keeps deployment, relayer status, fault injection, manual relay, and application assertions visibly ordered; there is no second aggregate beside the Environment.

`synthetic.NewSigners` creates separate application and relayer identities for the test. Managed Chains fund them through their resolved funding capability; an attached Chain must fund the returned public addresses out of band before `Deploy`. Credentials are written only to protected temporary signer files referenced by alias in the synthetic Link configuration.

Tests assess required controls before startup and then request the resolved capability:

```go
selected := e2etest.SelectedSuite(t)
e2etest.RequireCapabilities(t, selected, environment.Requirements{
    MiningControl: []environment.ChainID{e2etest.ChainB},
})
env := e2etest.Start(t, selected)

chainB, err := env.Chain(e2etest.ChainB)
require.NoError(t, err)
mining, err := chainB.Mining()
require.NoError(t, err)
```

An invalid selection fails; an interchangeable selection that cannot guarantee a requirement skips before acquisition. Startup failures always fail.

## Extending the graph

`e2etest.Suite` contains only an `environment.Spec` and its process-local runtime bindings. `SelectedSuite(t)` supplies the ordinary two-Chain selection for the chosen lane, while exceptional graphs use `e2etest.SuiteFor` directly. Synthetic route configuration belongs to `e2e/internal/synthetic`, not to the Environment selection.

A reusable Environment constructor belongs in `e2etest` only when multiple tests need the same resource graph. Test applications and synthetic relay policy stay in the test setup that uses them.
