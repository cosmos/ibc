# Coordinated environment reset (deferred)

Status: **still-claimed** — no `Snapshotter` capability and no `run.Reset` exist in the tree today.

## Motivation

The POC uses a fresh environment per test. Fixture setup is one atomic transaction per managed EVM chain,
but repeated node and daemon startup still makes a shared environment attractive for larger suites. A shared
environment would deploy once and revert to a post-deploy snapshot between tests. The team chose to defer it.

## What a reset must coordinate

Reverting the chain alone is not a reset — the relayer's state and the harness's own baselines would
drift out from under it. A real implementation needs to keep in lockstep:

- **Chain state** (balances, contract storage, height) — reverted by the chain provider.
- **The relayer's SQLite DB** — packet rows and sequences must rewind with the chain, or the daemon
  either thinks already-relayed packets are pending or collides with reused sequences.
- **The daemon's in-memory subscriptions** — must be stopped across the revert and restarted.
- **Deployment metadata** — fixture addresses survive a revert taken after deploy, so `Deployment` and
  its `onchain.Reader`s can be reused rather than rebuilt.
- **App-assertion baselines** — `Prepare*` snapshots (escrow balance, Counter value) must be re-taken
  post-revert, not carried over.
- **Diagnostics bundle partitioning** — one `diag.Bundle` currently spans one env; a shared env needs
  per-test partitioning so a failure names the offending test's slice, not the whole package's history.

The natural shape is an optional `Snapshotter` capability (alongside `BlockController`/`FaultInjector`)
that anvil could back with `evm_snapshot`/`evm_revert`, a DB restore by file copy while the daemon is
stopped, and a `run.Reset(ctx)` that sequences: stop daemon → revert chain(s) → restore DB → start
daemon → re-await readiness. Only providers that advertise `Snapshotter` could back a shared env; the
entrypoint would need to fail loudly, not silently fall back to fresh-per-test, when asked to share an
env whose chains can't reset.

Not every family can cheaply support this: besu has no snapshot API, sandboxd (a real CometBFT node)
has no snapshot/revert analogue, and an out-of-band external chain should never be reset at all. Any of
those would need to stay fresh-per-test even after anvil gained the capability.

## Open risks

- Sequence reuse: a chain revert rewinds the on-chain sequence counter, so the DB restore and chain
  revert must be atomic-in-effect or the store risks a duplicate/ghost row.
- Daemon resubscription must re-derive cursors from the restored DB, not backfill against pre-revert
  chain history that no longer exists.
- In-flight packets at reset time could be orphaned by the revert; reset likely needs to quiesce first.
- `evm_revert` restores chain time too, which interacts with `AdvanceTime` — a shared lane would need to
  reset even after read-only-looking tests that touched the clock.
- Reset and `KEEP_AFTER_TEST`/`keepOnClose` must compose: a kept shared env should stop resetting.

Ship only once a test that deliberately dirties everything (mints, bumps state, advances time, leaves a
pending packet) then resets and passes the standard happy path exactly as a fresh env would — a faster
suite that resets incompletely is worse than a slow one.
