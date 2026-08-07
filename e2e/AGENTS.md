# Repository E2E Tests Guide for AI Agents

This module contains one root repository-level acceptance package: linear Go tests over the
`internal/harness` surface. Tests relay real IBC packets through the real Link relayer and
Solidity IBC stack (ICS26Router, ICS20Transfer, ICS27GMP), usually with a permissive dummy light
client; the attested IFT tests use attestation clients and managed attestors, including quorum loss
and recovery. `TestAttestedIFTTimeout_Refund` runs only on instant Anvil because it controls mining
explicitly.

- Run everything from the repository root: `make test-e2e` for the complete acceptance package, or
  `make test-e2e E2E_FLAGS='-run TestTransfer_AutoRelay -count=1'` for one focused loop. Lanes:
  `E2E_LANE=anvil|anvil-interval|besu`.
- Tests start an `Environment` (which realizes Chains, the IBC contract stack, and dummy light
  clients), hold the temporary Link driver and relayer explicitly, and assert through
  route-bound application bindings (ICS20 transfers, ICS27 GMP calls). This e2e-only setup
  lives in `internal/e2etest`; transport-contract tests may call the concrete driver or Relayer directly.
- E2e may import Link's public command transport types, generated RPC clients, and signer-keyfile
  package for transport and provisioning, but it must not import `link/internal` or invoke handlers
  in process.
- Each test gets a fresh environment.
- After a hard crash: `make clean-e2e-dry-run`, then `make clean-e2e` from the repository root.
