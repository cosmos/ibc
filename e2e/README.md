<!-- SPDX-License-Identifier: Apache-2.0 -->

# Repository E2E Test Surface

This repository-level surface hosts one black-box acceptance package. Its tests drive IBC CLI through its public CLI, config, readiness, relay, and status contracts, and relay real IBC packets through attestation light clients and, between two Besu chains, through Besu QBFT light clients; the quorum test additionally exercises 2-of-3 quorum loss and recovery.

`internal/harness/environment` realizes Chains and protocol resources, including the IBC contract stack, attestation and Besu QBFT light clients, and attestor processes. `internal/harness/traffic` deploys a test ERC20, a Counter target, and an IFT token per Chain and binds ICS20 transfers, ICS27 GMP calls, and IFT transfers to routes, without depending on `testing`; `internal/e2etest` resolves each test's provider requirements into an Environment and wraps the traffic layer for `testing.TB`, failing the test on error and registering cleanup. Tests deploy the applications and start the relayer explicitly, so process restarts, manual relay, fault injection, and teardown remain visible in the behavior under test.

## Acceptance coverage

The root package covers ICS20 transfer, ICS27 GMP, IFT (burn/mint on top of GMP) relay behavior, timeout refunds, error acknowledgements, pending-packet status, Relayer and node recovery, attestor quorum loss and recovery, cross-route handling, and relaying through an attached RPC that `Environment` does not own. These are all acceptance criteria and run together by default.

Besu QBFT light clients are covered through a two-chain Besu mesh, which requires the Besu provider and therefore skips in fast mode. `TestTransferBesuQBFT_AutoRelay` exercises client updates from sealed headers, membership proofs and acknowledgement; `TestTransferBesuQBFT_TimeoutRefund` exercises the non-membership proof of a timeout, letting the packet expire in real time; `TestDeployBesuQBFTConnection` deploys the clients through `ibc deploy client besu-qbft`. Client expiry is not exercised end-to-end: it would need the Besu clock moved past the trusting period.

## Running the acceptance tests

Run targets from the repository root:

```sh
make -C e2e doctor
make -C e2e test
```

The test target builds `cli/bin/ibc`; `IBC_BIN` overrides that path. The real relayer collects attestor signatures and submits recv, ack, and timeout transactions with attestation proofs, which the attestation light clients verify.

If tests fail to find the Docker socket (OrbStack, Colima, Desktop), set `DOCKER_HOST` from the active CLI context:

```sh
export DOCKER_HOST="$(docker context inspect | jq -r '.[0].Endpoints.docker.Host')"
```

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
make -C e2e test
make -C e2e test E2E_MODE=complete
make -C e2e test E2E_MODE=production
make -C e2e test E2E_FLAGS='-run TestIFTTransfer_AutoRelay -count=1'
make -C e2e test E2E_MODE=production E2E_FLAGS='-run TestCrossRoute -parallel 1 -count=1'
```

`-e2e.mode` in `E2E_FLAGS` overrides `E2E_MODE`. After a hard crash, use
`make -C e2e clean-dry-run` and then `make -C e2e clean`.

Every environment-backed test calls `t.Parallel()` and boots its own environment; the Makefile caps concurrency at four environments. Pass `E2E_FLAGS='-parallel 1 -count=1'` to serialize when debugging.

## Writing a test

The setup sequence is deliberately explicit:

```go
func TestTransfer_AutoRelay(t *testing.T) {
    t.Parallel()
    chains := e2etest.EVMChains(t, e2etest.EVMRequirements{}, traffic.ChainA, traffic.ChainB)
    spec, runtime := traffic.AttestedMesh(chains...)
    env := e2etest.Start(t, spec, runtime)
    sender := e2etest.NewSigner(t)
    relayerSigner := e2etest.NewSigner(t)
    route := traffic.AtoB(traffic.ChainA, traffic.ChainB)
    driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
    transferApp, err := traffic.NewTransfer(env, deployment, sender, route)
    require.NoError(t, err)
    relayer := e2etest.StartRelayer(t, driver, env)
    ctx := t.Context()

    transfer, err := transferApp.Send(ctx, traffic.TransferRequest{Amount: big.NewInt(1_234_000)})
    require.NoError(t, err)
    require.NoError(t, transfer.VerifyEscrowed(ctx))

    _, err = traffic.AwaitState(ctx, relayer, transfer.PacketTx(),
        relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
    require.NoError(t, err)
    require.NoError(t, transfer.VerifyDelivered(ctx))
}
```

`Environment` owns Chain clients and protocol resources. A route-scoped traffic application hides only application ABI, transaction, event, and state mechanics. The test keeps deployment, relayer status, fault injection, manual relay, and application assertions visibly ordered; there is no second aggregate beside the Environment.

`e2etest.NewSigner` creates an independent identity; tests create one per role and pass them explicitly — `Deploy` takes the deployer and relayer signers, the app constructors take the sender (the signer that deployed the apps). Managed Chains fund them through their resolved funding capability; an attached Chain must fund the public addresses out of band before `Deploy`. Credentials are written only to protected temporary signer files referenced by alias in the temporary IBC CLI configuration.

`e2etest` wraps for `testing.TB` the traffic calls that own resources, plus `NewSigner`, which nearly every test calls (see `internal/e2etest/bindings.go`, and `Start` in `start.go`): each fails the test on error, and ownership stays in `t.Cleanup`: the directory holding the IBC CLI configuration comes from `t.TempDir()`, and `StartRelayer` and `StartProver` register their stops, so cleanup stops processes before removing their directories and before closing the Environment. Everything else — routes, app constructors, sends, status waits, manual relay — tests call on `internal/harness/traffic` directly and check with `require.NoError`. Code outside `go test` calls `internal/harness/traffic` for the resource-owning steps too and owns them itself: it passes the directory, stops the relayer it started, and closes the Environment.

Declare capabilities instead of naming a provider. For example, a controlled-mining test uses:

```go
chains := e2etest.EVMChains(t, e2etest.EVMRequirements{ControlledMining: true},
    traffic.ChainA, traffic.ChainB)
spec, runtime := traffic.AttestedMesh(chains...)
env := e2etest.Start(t, spec, runtime)

chainB, err := env.Chain(traffic.ChainB)
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

Bootstrapping a Besu QBFT light client reads validators from the sealed block header through
the `ETH` RPC API; it does not require the `QBFT` RPC API.

## Provider and topology matrix

[`test-matrix.md`](./test-matrix.md) is generated from real requirement resolution and environment
specs for all three modes. Generation starts the caller-owned Anvil used by the attached-chain test,
so Docker is required.

```sh
make -C e2e generate-matrix
make -C e2e check-matrix
```

Regenerate the matrix after changing test requirements or topology. The check compares generated
output without modifying the committed file.

## Extending the graph

`traffic.AttestedMesh` builds a fully connected attested mesh over the given chains; `traffic.QBFTMesh` builds the same graph with Besu QBFT Clients and no Attestors, and requires every chain to run Besu. For sparse graphs or custom attestor topologies (see `TestIFTTransfer_MultiAttestorQuorum`), write the `environment.Spec` and matching `environment.Runtime` literals yourself, with every referenced endpoint and authority. Use `traffic.RuntimeWithProtocolDeployer` only when the spec references `traffic.ProtocolAuthorityID`, then pass both to `e2etest.Start`. Application deployment and temporary relay policy stay in the test setup that uses them. The test ERC20 and Counter sources live in `internal/harness/environment/solidityibc/contracts`, alongside the pinned solidity-ibc-eureka contracts compiled for the harness bindings.

## Debugging

To preserve attestor configs, logs, relayer logs, and other workspace files
that cleanup normally removes, enable `E2E_DUMP`:

```sh
make -C e2e test E2E_DUMP=true
```

```sh
export E2E_DUMP=true
make -C e2e test E2E_FLAGS='-run TestTransfer_AutoRelay -count=1 -v -parallel 1'
```

`E2E_DUMP_DIR` overrides the output directory (default `./dumps/`). Be aware that
dumps are verbose — a single test run produces many files across configs, diagnostics,
and relayer homes, so a full suite run accumulates a sizeable tree quickly.

The directory layout groups files by OS process and run timestamp:

```
dumps/
  pid-<pid>/                            # go test pid
    <unix-nano>/                        # environment workspace files
    <unix-nano>/<TestName>/             # per-test relayer config home
```
