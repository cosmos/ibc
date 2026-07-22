# Repository E2E Tests + Stub SUT Guide for AI Agents

This module contains one root repository-level acceptance package: linear Go tests over the
`internal/harness` surface. Tests relay real IBC packets through the real Solidity IBC stack
(ICS26Router, ICS20Transfer, ICS27GMP) with a permissive dummy light client; temporary Link
relay behavior lives in `../link/internal/ibcrelay` and is explicitly selected by the ordinary
`ibc` binary's composition root.

- Run everything from the repository root: `make test-e2e` for the complete acceptance package, or
  `make test-e2e E2E_FLAGS='-run TestTransfer_AutoRelay -count=1'` for one focused loop. Lanes:
  `E2E_LANE=anvil|anvil-interval|besu`.
- Tests start an `Environment` (which realizes Chains, the IBC contract stack, and dummy light
  clients), hold the temporary Link driver and relayer explicitly, and assert through
  route-bound application bindings (ICS20 transfers, ICS27 GMP calls). This e2e-only setup
  lives in `e2etest`; transport-contract tests may call the concrete driver or Relayer directly.
- **The ibcrelay package is a swap ledger entry, not a product.** The Link composition root
  visibly selects temporary handlers. It relays with empty proofs, which only the dummy light
  client accepts. Once a real implementation satisfies the e2e transport contract, change that
  handler selection, delete the unused ibcrelay code it replaces, and keep every lane green.
  E2e may import Link's public command transport types, generated RPC clients, and
  signer-keyfile package for transport and provisioning, but it must not import
  `link/internal` or invoke handlers in process.
- Each test gets a fresh environment.
- After a hard crash: `make clean-e2e-dry-run`, then `make clean-e2e` from the repository root.
