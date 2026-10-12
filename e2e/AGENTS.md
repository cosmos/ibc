<!-- SPDX-License-Identifier: Apache-2.0 -->

# Repository E2E Tests Guide for AI Agents

This module contains one root repository-level acceptance package: linear Go tests over the
`internal/harness` surface. Tests relay real IBC packets through the real relayer and
Solidity IBC stack (ICS26Router, ICS20Transfer, ICS27GMP) with attestation light clients and
managed attestors.

- Run from the repository root: `make -C e2e test` uses fast mode;
  `make -C e2e test E2E_MODE=complete|production` selects another mode, and
  `E2E_FLAGS='-run TestTransfer_AutoRelay -count=1'` focuses a run. `-e2e.mode` in `E2E_FLAGS`
  overrides `E2E_MODE`.
- Tests declare portable EVM, controlled-mining, or node-lifecycle requirements. Fast mode may
  skip an unresolved requirement; complete and production modes fail it. Fast and complete prefer
  Anvil, while production prefers Besu where the requirements allow it.
- Tests start an `Environment` (which realizes Chains, the IBC contract stack, attestation light
  clients, and attestors), hold the temporary IBC CLI driver and relayer explicitly, and assert through
  route-bound application bindings (ICS20 transfers, ICS27 GMP calls, IFT transfers). The bindings live in
  `internal/harness/traffic`, which does not depend on `testing`; `internal/e2etest` holds only the `go test` concerns
  (mode and requirement selection, the matrix, `-e2e.*` flags, skipping) and the `testing.TB` wrappers that
  `README.md` describes. Transport-contract tests may call the concrete driver or Relayer directly.
- E2e may import the IBC CLI's public command transport types, generated RPC clients, and signer-keyfile
  package for transport and provisioning, but it must not import `cli/internal` or invoke handlers
  in process.
- Each test gets a fresh environment. Managed Anvil uses one-second mixed mining; after mining is
  paused and resumed it is interval-only, so transaction inclusion may take one second.
- `Environment` cleans up managed resources only. Attached chains remain caller-owned and expose no
  harness mining or node-lifecycle controls.
- Run `make -C e2e generate-matrix` after changing requirements or topology, and
  `make -C e2e check-matrix` to check the committed matrix. Both require Docker.
- After a hard crash: `make -C e2e clean-dry-run`, then `make -C e2e clean` from the repository root.
- `cmd/loadtest` is a load-test binary over the same harness (`make -C e2e run-relayer` /
  `run-attestor`); its [README](cmd/loadtest/README.md) covers usage and design. It is not a
  `go test` suite: unit tests cover its logic, and a change is verified by a small run
  (`run-relayer LOADTEST_FLAGS='--packets 40 --wallets 10 --per-block 5'`, `run-attestor
  LOADTEST_FLAGS='--duration 5s'`) followed by `docker ps --filter label=ibc-cli-e2e=true`
  showing nothing left behind.
