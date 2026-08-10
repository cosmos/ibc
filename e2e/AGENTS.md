<!-- SPDX-License-Identifier: Apache-2.0 -->

# Repository E2E Tests Guide for AI Agents

This module contains one root repository-level acceptance package: linear Go tests over the
`internal/harness` surface. Tests relay real IBC packets through the real Link relayer and
Solidity IBC stack (ICS26Router, ICS20Transfer, ICS27GMP) with attestation light clients and
managed attestors.

- Run from the repository root: `make test-e2e` uses fast mode;
  `make test-e2e E2E_MODE=complete|production` selects another mode, and
  `E2E_FLAGS='-run TestTransfer_AutoRelay -count=1'` focuses a run. `-e2e.mode` in `E2E_FLAGS`
  overrides `E2E_MODE`.
- Tests declare portable EVM, controlled-mining, or node-lifecycle requirements. Fast mode may
  skip an unresolved requirement; complete and production modes fail it. Fast and complete prefer
  Anvil, while production prefers Besu where the requirements allow it.
- Tests start an `Environment` (which realizes Chains, the IBC contract stack, attestation light
  clients, and attestors), hold the temporary Link driver and relayer explicitly, and assert through
  route-bound application bindings (ICS20 transfers, ICS27 GMP calls, IFT transfers). This e2e-only setup
  lives in `internal/e2etest`; transport-contract tests may call the concrete driver or Relayer directly.
- E2e may import Link's public command transport types, generated RPC clients, and signer-keyfile
  package for transport and provisioning, but it must not import `link/internal` or invoke handlers
  in process.
- Each test gets a fresh environment. Managed Anvil uses one-second mixed mining; after mining is
  paused and resumed it is interval-only, so transaction inclusion may take one second.
- `Environment` cleans up managed resources only. Attached chains remain caller-owned and expose no
  harness mining or node-lifecycle controls.
- Run `make generate-e2e-matrix` after changing requirements or topology, and
  `make check-e2e-matrix` to check the committed matrix. Both require Docker.
- After a hard crash: `make clean-e2e-dry-run`, then `make clean-e2e` from the repository root.
