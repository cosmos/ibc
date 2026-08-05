# Repository E2E Test Surface

This repository-level surface hosts one black-box acceptance package. Its tests drive IBC Link through its public CLI, config, readiness, relay, and status contracts, and relay real IBC packets. Most use a permissive dummy light client that accepts packets without proof verification; the attested IFT tests instead use attestation clients and managed attestors, including 2-of-3 quorum loss and recovery.

`internal/harness/environment` realizes Chains and protocol resources, including the IBC contract stack and dummy light clients. `internal/e2etest` deploys a test ERC20, a Counter target, and an IFT token per Chain and binds ICS20 transfers, ICS27 GMP calls, and IFT transfers to routes. Tests deploy the applications and start the relayer explicitly, so process restarts, manual relay, fault injection, and teardown remain visible in the behavior under test.

## Acceptance coverage

The root package covers ICS20 transfer, ICS27 GMP, IFT (burn/mint on top of GMP) relay behavior, attested IFT relay, quorum recovery, and timeout behavior, timeout refunds, error acknowledgements, pending-packet status, Relayer and node recovery, cross-route handling, and relaying through an attached RPC that `Environment` does not own. These are all acceptance criteria and run together by default.

## Running the acceptance tests

Run targets from the repository root:

```sh
make doctor-e2e
make build-link
make test-e2e
```

`make build-link` produces `link/bin/ibc`; `IBC_BIN` overrides that path. In dummy-client tests, the real Link Relayer submits recv, ack, and timeout transactions with empty proofs, which the dummy light client accepts.

Execution modes choose providers from each test's declared requirements:

| Mode | Provider policy | Unresolved requirement |
|---|---|---|
| `fast` (default) | Prefer Anvil | Skip |
| `complete` | Prefer Anvil | Fail |
| `production` | Prefer Besu, then Anvil | Fail |

Portable EVM tests therefore use Anvil in fast and complete modes and Besu in production mode.
Tests requiring controlled mining or node lifecycle use Anvil in every mode because Besu does not
provide those harness controls. `complete` runs each test once with the fastest compatible provider;
it does not run every provider permutation.

```sh
make test-e2e
make test-e2e E2E_MODE=complete
make test-e2e E2E_MODE=production
make test-e2e E2E_FLAGS='-run TestIFTTransfer_AutoRelay -count=1'
make test-e2e E2E_MODE=production E2E_FLAGS='-run TestCrossRoute -parallel 1 -count=1'
```

`-e2e.mode` in `E2E_FLAGS` overrides `E2E_MODE`. After a hard crash, use
`make clean-e2e-dry-run` and then `make clean-e2e`.

Every test calls `t.Parallel()` and boots its own environment; the Makefile caps concurrency at four environments. Pass `E2E_FLAGS='-parallel 1 -count=1'` to serialize when debugging.

## Writing a test

The setup sequence is deliberately explicit:

```go
func TestTransfer_AutoRelay(t *testing.T) {
    t.Parallel()
    chains := e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB)
    spec := dummyClientMeshSpec(chains)
    runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
    env := e2etest.Start(t, spec, runtime)
    signers := e2etest.NewSigners(t)
    route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
    driver, deployment := e2etest.Deploy(t, env, signers, route)
    transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
    relayer := e2etest.StartRelayer(t, driver, env)

    transfer, err := transferApp.Send(t.Context(), e2etest.TransferRequest{Amount: big.NewInt(1_000)})
    require.NoError(t, err)
    _, err = e2etest.AwaitState(t.Context(), relayer, transfer.Packet(),
        relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
    require.NoError(t, err)
    require.NoError(t, transfer.VerifyDelivered(t.Context()))
    require.NoError(t, transfer.VerifyEscrowed(t.Context()))
}
```

`Environment` owns Chain clients and protocol resources. A route-bound `e2etest` application binding hides only application ABI, transaction, event, and state mechanics. The test keeps deployment, relayer status, fault injection, manual relay, and application assertions visibly ordered; there is no second aggregate beside the Environment.

`e2etest.NewSigners` creates separate application and relayer identities for the test. Managed Chains fund them through their resolved funding capability; an attached Chain must fund the returned public addresses out of band before `Deploy`. Credentials are written only to protected temporary signer files referenced by alias in the temporary Link configuration.

Declare capabilities instead of naming a provider. For example, a controlled-mining test uses:

```go
chains := e2etest.EVMChains(t, e2etest.EVMRequirements{ControlledMining: true},
    e2etest.ChainA, e2etest.ChainB)
spec := dummyClientMeshSpec(chains)
runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
env := e2etest.Start(t, spec, runtime)

chainB, err := env.Chain(e2etest.ChainB)
require.NoError(t, err)
mining, err := chainB.Mining()
require.NoError(t, err)
```

An invalid mode or provider fails. Fast mode skips when no compatible provider exists; complete and
production modes fail. Startup failures always fail.

## Mining and ownership

Managed Anvil starts with one-second mixed mining: transactions are included immediately and idle
blocks continue to advance finality. Pausing stops all block production. After resume, Anvil is
interval-only, so a transaction may wait up to one second for inclusion.

`Environment` owns and cleans up only managed resources. An attached EVM remains caller-owned even
when the harness can connect to it, and connectivity does not grant mining or node-lifecycle control.

## Provider and topology matrix

[`test-matrix.md`](./test-matrix.md) is generated from real requirement resolution and environment
specs for all three modes. Generation starts the caller-owned Anvil used by the attached-chain test,
so Docker is required.

```sh
make generate-e2e-matrix
make check-e2e-matrix
```

Regenerate the matrix after changing test requirements or topology. The check compares generated
output without modifying the committed file.

## Extending the graph

Build the graph explicitly: `dummyClientMeshSpec` is this package's narrow fixture for a permissive dummy-client mesh, including custom managed or attached Chain declarations that intentionally need every pair connected. It is not a public or general spec composer. For attested, sparse, or other protocol graphs, write the complete `environment.Spec` literal and a matching `environment.Runtime` with every referenced endpoint and authority. Use `e2etest.RuntimeWithProtocolDeployer` only when the spec references `e2etest.ProtocolAuthorityID`, then pass both to `e2etest.Start`. Application deployment and temporary relay policy stay in the test setup that uses them. The test ERC20 and Counter sources live in `internal/harness/environment/solidityibc/contracts`, alongside the pinned solidity-ibc-eureka contracts compiled for the harness bindings.
