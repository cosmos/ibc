# Repository E2E Tests + Stub SUT Guide for AI Agents

This module contains one root repository-level acceptance package: linear Go tests over the
`internal/harness` surface. Temporary Link behavior lives in `../link/internal/stub` and is
explicitly selected by the ordinary `ibc` binary's composition root.

- Run everything from the repository root: `make test-e2e` for the complete acceptance package, or
  `make test-e2e E2E_FLAGS='-run TestIFTTransfer_AutoRelay -count=1'` for one focused loop. Lanes:
  `E2E_LANE=anvil|anvil-interval|besu`.
- Tests start an `Environment`, hold the temporary Link driver and relayer explicitly, and assert
  through route-bound application bindings. This e2e-only setup lives in `e2etest`; transport-contract
  tests may call the concrete driver or Relayer directly.
- **The stub is a swap ledger entry, not a product.** The Link composition root visibly selects
  temporary handlers. Once a real implementation satisfies the e2e transport contract, change that
  handler selection, delete the unused stub code, and keep every lane green. E2e may import Link's
  public command transport types, generated RPC clients, and signer-keyfile package for transport
  and provisioning, but it must not import `link/internal` or invoke handlers in process.
- Each test gets a fresh environment.
- After a hard crash: `make clean-e2e-dry-run`, then `make clean-e2e` from the repository root.
