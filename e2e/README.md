# Repository E2E Test Surface

This repository-level surface hosts one black-box acceptance package. Its tests drive IBC Link through its public CLI, config, readiness, relay, and status contracts, and relay real IBC packets: each Chain runs the real Solidity IBC stack (ICS26Router, ICS20Transfer, ICS27GMP) behind a permissive dummy light client that accepts every packet without proof verification, so the transport is real while attestation stays out of scope.

`internal/harness/environment` realizes Chains and protocol resources, including the IBC contract stack and dummy light clients. `e2etest` deploys a test ERC20, a Counter target, and an IFT token per Chain and binds ICS20 transfers, ICS27 GMP calls, and IFT transfers to routes. Tests deploy the applications and start the relayer explicitly, so process restarts, manual relay, fault injection, and teardown remain visible in the behavior under test.

## Acceptance coverage

The root package covers ICS20 transfer, ICS27 GMP, and IFT (burn/mint on top of GMP) relay behavior, timeout refunds, error acknowledgements, pending-packet status, Relayer and node recovery, cross-route handling, and relaying through an attached RPC that `Environment` does not own. These are all acceptance criteria and run together by default.

## Running the acceptance tests

Run targets from the repository root:

```sh
make doctor-e2e
make build-link
make test-e2e
```

`make build-link` produces `link/bin/ibc`; `IBC_BIN` overrides that path. The real Link Relayer submits recv, ack, and timeout transactions with empty proofs, which the dummy light client accepts.

The same tests can select different Chain declarations:

- `make test-e2e` uses instant-mining Anvil for fast feedback.
- `make test-e2e E2E_LANE=anvil-interval` uses two-second Anvil blocks.
- `make test-e2e E2E_LANE=besu` uses Besu QBFT.
- `make test-e2e E2E_FLAGS='-run TestIFTTransfer_AutoRelay -count=1'` runs one test repeatedly.

`-e2e.lane` in `E2E_FLAGS` overrides `E2E_LANE`. Tests pinned to instant Anvil call `e2etest.RequireAnvilLane(t)` so a matrix runs them only in that lane. After a hard crash, use `make clean-e2e-dry-run` and then `make clean-e2e`.

Every test calls `t.Parallel()` and boots its own environment; the Makefile caps concurrency at four environments. Pass `E2E_FLAGS='-parallel 1 -count=1'` to serialize when debugging.

## Writing a test

The setup sequence is deliberately explicit:

```go
func TestTransfer_AutoRelay(t *testing.T) {
    t.Parallel()
    env := e2etest.Start(t, e2etest.SelectedSuite(t))
    signers := e2etest.NewSigners(t)
    route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
    driver, deployment := e2etest.Deploy(t, env, signers, route)
    transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
    relayer := e2etest.StartRelayer(t, driver, env)

    transfer, err := transferApp.Send(t.Context(), e2etest.TransferRequest{Amount: big.NewInt(1_000)})
    require.NoError(t, err)
    destination, err := env.Chain(route.Destination)
    require.NoError(t, err)
    err = e2etest.AwaitState(t.Context(), relayer, transfer.Packet(),
        relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
    require.NoError(t, err)
    require.NoError(t, transfer.VerifyDelivered(t.Context()))
    require.NoError(t, transfer.VerifyEscrowed(t.Context()))
}
```

`Environment` owns Chain clients and protocol resources. A route-bound `e2etest` application binding hides only application ABI, transaction, event, and state mechanics. The test keeps deployment, relayer status, fault injection, manual relay, and application assertions visibly ordered; there is no second aggregate beside the Environment.

`e2etest.NewSigners` creates separate application and relayer identities for the test. Managed Chains fund them through their resolved funding capability; an attached Chain must fund the returned public addresses out of band before `Deploy`. Credentials are written only to protected temporary signer files referenced by alias in the temporary Link configuration.

Tests that need manual mining run only in the instant-Anvil lane:

```go
selected := e2etest.SelectedSuite(t)
e2etest.RequireAnvilLane(t)
env := e2etest.Start(t, selected)

chainB, err := env.Chain(e2etest.ChainB)
require.NoError(t, err)
mining, err := chainB.Mining()
require.NoError(t, err)
```

An invalid lane fails; a test pinned to another lane skips before acquisition. Startup failures always fail.

## Extending the graph

`e2etest.Suite` contains only an `environment.Spec` and its process-local runtime bindings. `SelectedSuite(t)` supplies the ordinary two-Chain selection for the chosen lane, while exceptional graphs use `e2etest.SuiteFor` directly. Temporary route configuration also lives in `e2etest`, but belongs to each test's explicit setup rather than the Environment selection.

A reusable Environment constructor belongs in `e2etest` only when multiple tests need the same resource graph. Application deployment and temporary relay policy stay in the test setup that uses them. The test ERC20 and Counter sources live in `internal/harness/environment/solidityibc/contracts`, alongside the pinned solidity-ibc-eureka contracts compiled for the harness bindings.
