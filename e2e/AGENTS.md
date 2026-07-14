# Repository E2E Tests + Stub SUT Guide for AI Agents

Two things live in this module: repository-level e2e packages (linear Go tests over the `internal/harness`
surface) and `stub/` — the TEMPORARY stand-in `ibc` binary that fills e2e wire contracts the real
Link binary (`../link/cmd/ibc`) does not yet satisfy.

- Run everything from the repository root: `make test-e2e` for the smoke suite,
  `make test-e2e E2E_PKGS=./negative` for negative cases, or
  `make test-e2e E2E_PKGS=./ibclink E2E_FLAGS='-run TestIFTTransfer_AutoRelay -count=1'` for one loop.
  Lanes: `E2E_LANE=anvil|anvil-interval|besu`.
- Tests start an `Environment`, hold the synthetic driver and relayer explicitly, and assert through
  route-bound test-application bindings. Wire-contract tests may call the concrete driver or relayer directly.
- **The stub is a swap ledger entry, not a product.** Each `Driver` method in
  `internal/harness/ibclink/runner.go` selects the real or stub binary; long-lived command selection lives
  in `daemon.go`. Once the real implementation satisfies the e2e wire contract, change that
  method's binary selection, delete the stub piece it replaces, and keep every lane green. The root
  e2e module is black-box infrastructure and must not import `link/internal`; the stub independently
  implements the wire behavior it stands in for.
- Each test gets a fresh environment.
- After a hard crash: `make clean-e2e-dry-run`, then `make clean-e2e` from the repository root.
