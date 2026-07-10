# E2E Tests + Stub SUT Guide for AI Agents

Two things live in this module: the e2e test packages (linear Go tests over the `../harness`
surface) and `stub/` — the TEMPORARY stand-in `ibc` binary that fills the wire surface the real
binary (`../cmd/ibc`) does not implement yet.

- Run everything from `link/`: `make test-e2e` for the smoke suite,
  `make test-e2e E2E_PKGS=./negative` for negative cases, or
  `make test-e2e E2E_PKGS=./ibclink E2E_FLAGS='-run TestIFTTransfer -count=1'` for one loop.
  Lanes: `E2E_LANE=anvil|anvil-interval|besu`.
- Tests assert via the harness surface (`run.IFT(...)`, `out.VerifyComplete`,
  `out.VerifyPendingStable`), never via `wire` directly.
- **The stub is a swap ledger entry, not a product.** The routing table in
  `../harness/ibclink/runner.go` says which commands the real binary already serves. When real
  functionality lands in `../cmd/ibc` / `../internal`, flip the entry, delete the stub piece it
  replaces, and keep every lane green. The stub SHOULD import real link packages
  (`internal/config`, ...) wherever they exist — it is the one place allowed to cross into
  `link/internal`.
- Each test gets a fresh environment; there is no shared-env reset (see `docs/reset-design.md`).
- After a hard crash: `make clean-e2e-dry-run`, then `make clean-e2e` (from `link/`).
