<!-- SPDX-License-Identifier: Apache-2.0 -->

## ADR-011: Auto-relay Chain Watching

**Status:** `Draft`
**Date:** 2026-08-11
**Tags:** `relayer` `watcher` `evm` `reorg`

---

### Context

IBC Link discovers packets only when a caller supplies a transaction hash.
FOU-385 requires the relayer to find `SendPacket` events by watching the chain
instead.

`relayer.Service.Relay` reads the receipt for a `(chainID, txHash)`, turns
`SendPacket` logs into `store.CreatePacket` rows, and writes them.
`dispatch.RelayDispatcher` polls `ListUnfinishedPackets` every five seconds and
pushes what it finds into the per-route pipeline. The two are coupled only
through the store, so the watcher is a second producer of packet rows rather
than a second relaying path.

``` 
manual:        Relay(chainID, txHash) ──┐ 
                                        ├──> packets table ──> dispatcher ──> etc
auto:          chain watcher ───────────┘ 
```

Everything downstream is unchanged. The pipeline already gates on send
finality, checks whether someone else delivered the packet, and checks whether
the commitment is gone.

Requirements:

- Discover `SendPacket` events by watching the chain.
- Persist discovery progress so a restart resumes rather than rescanning or
skipping.
- Detect reorgs and correct the index.
- Watch only the configured subset of a chain's clients, and tolerate that
subset changing.

The design below starts with a forward scanner that satisfies the first two
requirements, then adds what the third and fourth need. Each section states the
problem the next one solves.

### Forward Scanning

**Shape**

One goroutine per chain that sources at least one auto-relay-enabled route. A
chain has one log stream, one height, and one block history, so per-route or
per-client loops would duplicate every `eth_getLogs` call and every
block-identity record.

Lives at `internal/relay/watcher`, started and stopped by
`relayer.Service.Start`/`Stop` alongside the dispatcher.

**Chain client addition**

`chains.Client` is per-transaction today. Forward scanning needs one new
method: a range scan for send packets, taking an explicit set of source client
ids. `SendPacket` indexes `clientId`, so `FilterSendPacket` filters server-side
on the topic. It returns `v2.PacketEvent` plus the transaction hash and block
number of each log, which the manual path gets from the receipt. The method is
chain-agnostic in shape, so a non-EVM client can implement it without changing
the watcher.

**The loop**

The watcher keeps a cursor per chain: the next height it has not scanned. Per
tick:

1. Read head.
2. If the cursor is above head, sleep.
3. The range is `[cursor, min(head, cursor + chunk)]`.
4. Fetch `SendPacket` logs for the range, filtered to the watched client ids.
5. In one transaction, write packets and advance the cursor past the range top.
6. Repeat immediately if the range was chunk-limited, otherwise sleep.

The cursor advance and the packet writes share one `store.Transact` call, so a
crash mid-range replays the range harmlessly. Cancelling the context aborts
before the transaction commits, with the same result.

Step 6 makes this a catch-up loop as well as a tail-follower. After downtime it
grinds forward in chunks without sleeping until it reaches head.

**Cost**

Cost is per range rather than per block: one head call and one log query. One
`eth_getLogs` per range covers the whole chain regardless of client count,
since there is one cursor and therefore one range.

`source_tx_time` is the exception. The packets table stores it and the orphan
deadline depends on it. Logs carry a block number but not a timestamp, so a
range needs one extra `GetBlockHeader` call per distinct block containing a
matched log. That is zero on most ticks and proportional to packets found
during catch-up.

**Provider range limits**

Providers cap `eth_getLogs` by block span or result count and the errors are
not standardised. Halve the chunk size on a range-too-large or too-many-results
error, retry the same range, and recover toward the configured size after
sustained success. This is the likeliest source of integration friction with a
real provider and should be built in from the start.

Halving needs a floor. Because the errors are not standardised, a range the
provider refuses for some other reason lands in this path too, and halving a
range it will never serve converges on a single block it still will never serve.
Below the floor, a range that still fails is not too large, and the watcher
terminates rather than retrying. Unavailable History below covers what that case
actually is.

This much is complete and correct on a chain that does not reorg, which
includes every e2e environment. The next two sections cover what it gets wrong
on a real one.

### Client Scoping

The loop above filters logs to a set of client ids. That set has to be the
source clients of routes where `autoRelay.enabled` is true and whose source
chain is that chain.

Scoping is a correctness requirement rather than a bandwidth optimisation.
`PipelineSet.Pipeline` rejects a packet ingested for a client with no route,
and `SubmitWaitingUnfinishedPackets` treats submission failure as permanent and
marks the row `FAILED`. On a shared router an unscoped watcher fills the
database with failed rows for other operators' packets. The manual path avoids
this because a human only submits transactions they care about, and
`packetsFromEvents` skips unconfigured clients with a warning.

Client ids are a dynamic type, so the log topic is their keccak hash and the
filter matches exact ids only. The watcher cannot discover clients it is not
configured for. Discovery of new clients is out of scope.

Client ids go into a single OR-list in topic position one. Providers differ on
how long a topic array they accept, so the watcher caps the list and splits a
larger set across several queries over the same range. If splitting dominates,
drop the filter and discard non-matching logs client-side.

Because the watched set comes from config, it changes when config changes. What
happens when a client joins or leaves is covered under First-run Backfill.

### Reorg Handling

**What the watcher owes**

`CheckSendFinality` blocks delivery until the send transaction is finalized, so
reorg handling is not what keeps us from relaying a packet that no longer
exists. The watcher's job is keeping the index honest, which matters for the
status API, for not leaking stuck rows, and for not skipping a packet that only
exists on the canonical branch.

That is why the scan ceiling is a configurable offset rather than a fixed
choice. Step 1 above reads head; from here on the ceiling is `head -
confirmations`. Scanning to head makes packets visible in the status API
seconds after they are sent, waiting in the pipeline for finality. Raising
`confirmations` trades that visibility for a smaller rollback surface. The code
is the same either way. Default 0.

**Remembering what we scanned**

A bare cursor cannot detect a reorg. Knowing the next height to scan says
nothing about whether the blocks already scanned are still canonical, so the
watcher has to keep the identity of what it scanned.

```sql -- bounded ring of anchors, one per scanned range; the newest sits
directly below the cursor scan_anchors(chain_id, height, block_hash,
pk(chain_id, height)) ```

There is no stored watermark. A chain's next scan height is `max(height) + 1`
over its anchors, which buys four things:

- The parent-hash check below needs the newest anchor directly below the next
height to scan. Deriving the cursor makes that definitional rather than an
invariant the loop maintains.
- Rollback is `DELETE FROM scan_anchors WHERE chain_id = ? AND height > fork`,
with no off-by-one and no second write to clamp.
- A crash leaves a consistent cursor, since advancing it and recording block
identity are the same insert.
- "Never scanned" is no anchors. "Halted on a reorg deeper than the horizon" is
anchors present with no canonical ancestor. No flag is needed to tell them
apart.

We write one anchor per scanned range rather than per block. The watcher
fetches logs over ranges and never fetches blocks, so a header call per block
would dominate its cost. An anchor is the range's highest block. At the tip a
range is one or two blocks, so anchors are effectively per block; during
catch-up they are sparse over history that will not reorg. The ring is trimmed
to the reorg horizon.

Reading a block's identity needs one more chain client addition.
`GetBlockHeader` returns height and timestamp today; `v2.BlockHeader` gains
hash and parent hash.

**Detection**

Before scanning a range, fetch the header at the range bottom. Its parent hash
must match the newest anchor, which sits at `cursor - 1`. A mismatch means the
branch we anchored to is gone.

A ceiling below the cursor is the second signal. It means either node lag or a
chain that shortened under us. Fetch the header at the newest anchor's height:
the same hash means lag, and a different or missing hash means the branch
changed.

A missing header is only evidence of a branch change when the height is above
head. At or below head the canonical chain has a block there and the provider
is not serving it, which Unavailable History covers.

**Correction**

Correction is a read-only walk followed by a single write. From the newest
anchor, fetch the header at its height and compare hashes. A mismatch means
that anchor is on the orphaned branch, so step to the previous anchor and
repeat. Stop at the first anchor still matching canonical and delete everything
above it. The cursor becomes `fork + 1` and the next tick scans forward,
re-deriving whatever survived. No packets are touched, and a crash during the
walk has written nothing.

Rolling back to an anchor overshoots the exact fork block. Rescanning is
idempotent, so the cost is one extra range scan, and near the tip anchors are
per block anyway.

**A reorg mid-scan**

The parent-hash check is sound at the instant it runs. A node serves the
canonical block at a height, so if the canonical block at the cursor has our
anchor as its parent then our anchor is canonical. The sequence of requests
around it is not atomic.

Nothing is written until the commit, so the risk is committing a range that was
already stale. A fork above the range bottom, say at 150 while scanning
100–199, means the logs come from the branch being replaced while the anchor at
199 comes from the new one. The next tick chains 200 onto an anchor that
matches, finds nothing wrong, and leaves those packets derived from an orphaned
branch.

Closing this needs both ends of the range read before the log query and re-read
after it. The bottom is already read before, for the parent-hash check, so it
only needs the re-read. The top needs both. Reading the top after the log query
and re-reading later puts both reads on the same side of what they bracket, so
a fork between the log query and that first read passes every check.
Single-block ranges hide this because the top and bottom are the same header.
Chunked catch-up ranges do not.

With that ordering, a fork inside the range changes the top block's hash and a
fork below it changes the bottom's parent hash. Either way the range is
discarded and the tick restarts, after which the parent-hash check either
passes and rescans against the new branch, or fails and rolls back. A
replacement branch too short to reach the range top reads as a missing header
and is also a discard.

A fork that lands and reverts entirely inside the bracket is not handled.

`confirmations` defaults to 0, which makes one assumption load-bearing. "A node
serves the canonical block at a height" holds per node, not across a
load-balanced pool where consecutive requests hit members with different views,
and the anchor is written from one node and re-read from another. A member a
block or two behind makes the re-read discard most ranges, so the discard path
needs a consecutive-discard counter that warns and backs off rather than
spinning. Operators on pooled public endpoints should raise `confirmations`.

**The complete loop**

Detection, correction and the bracket turn the six steps above into nine. Per
tick, per chain:

1. Read head. The scan ceiling is `head - confirmations`.
2. The cursor is `max(scan_anchors.height) + 1`. If it is above the ceiling,
run 2a and sleep. 2a. Fetch the header at the newest anchor's height. The same
hash means node lag; a different or missing hash means the branch changed, so
take the reorg path.
3. The range is `[cursor, min(ceiling, cursor + chunk)]`.
4. Fetch the header at the range bottom. Its parent hash must match the newest
anchor. A mismatch means a reorg.
5. Fetch the header at the range top. Single-block ranges reuse step 4's
header. This read must precede the log query.
6. Fetch `SendPacket` logs for the range, filtered to the watched client ids.
7. Re-read both ends. The bottom's parent hash must still match the newest
anchor, and the top's hash must still match step 5. If either moved, discard
the range and restart the tick.
8. In one transaction, write packets, insert the anchor at the range top, and
trim anchors past the horizon. The anchor insert is the cursor advance.
9. Repeat immediately if the range was chunk-limited, otherwise sleep.

Cost per range is now one head call, two header calls, two re-reads, and one
log query.

**What rollback does not fix**

Deleting anchors puts the cursor back on the canonical branch. It does nothing
about packet rows already written, which is why it can stay a single `DELETE`.
Three cases need handling elsewhere.

**Rediscovery overwrites the row.** Packets are keyed by `(source_chain_id,
packet_source_client_id, packet_sequence_number)`. A reorg can change what sits
under that key two ways. The same packet re-included in a different transaction
leaves a stale `source_tx_hash`. A reorg that reorders two sends from one
client moves a sequence number onto a different packet entirely. The packets
table stores no payload and `BatchRecvPacket` re-reads it from `source_tx_hash`
at relay time, so both cases surface the same way:
`findPacketEventAtOrBeforeHeight` cannot find the sequence in that transaction
and the transfer stalls, while the packet that actually exists is never relayed
because a row already occupies its key.

`CreatePacket`'s conflict clause becomes `DO UPDATE` guarded on
`packets.recv_tx_hash IS NULL`, replacing every column derived from the send
transaction rather than the transaction fields alone. The canonical chain is
right by construction, so no comparison logic is needed.

A packet that already has a recv transaction but an orphaned source is a bad
state, and overwriting it destroys the record of what we submitted. Leave it
and alarm on it.

**A packet can vanish entirely.** If the send transaction is never re-included
the row is a ghost, and the dispatcher re-pushes it every five seconds forever.

This belongs in the pipeline. `IsTxFinalized` already returns
`v2.ErrTxNotFound` when the receipt is missing, `CheckSendFinality.Cancel`
already ages decisions off `tr.SourceTxTime`, and a processor terminates by
setting `tr.Status`, which `pipeline/processor_mw.go` short-circuits on. A send
transaction still missing after a threshold well past the reorg horizon is
orphaned.

We use a distinct `ORPHANED` status. Reusing `FAILED` avoids a migration and
already excludes the row from `ListUnfinishedPackets`, but "the chain
reorganised" and "we could not relay this" are different operational signals.
`ORPHANED` costs one migration and a wire enum value.

**A reorg can be deeper than the horizon.** If the walk reaches the oldest
anchor without a common ancestor, we cannot tell what is canonical. The watcher
for that chain deletes nothing, stops, logs loudly, and reports unhealthy.

Deleting nothing makes the halt durable and removes the need for a halt flag.
The state that caused it is still on disk, so a restart re-detects it. Recovery
is an operator action through `ibc relayer sync reset`.

The dispatcher keeps relaying rows already in the table during the halt,
including ones from the orphaned branch. A reorg past the horizon is deeper
than finality, so `CheckSendFinality` is not protecting them. The halt stops
the scan and nothing else. The runbook should say in-flight packets are not
held.

### Unavailable History

The reorg machinery above assumes that asking a node for a height either returns
the canonical block or tells us the chain is shorter than that. A third answer
exists: the block is canonical and the provider will not serve it. That is what
the watcher meets after enough downtime that the cursor falls outside the
provider's history window.

Left alone this reads as a reorg. Step 4 fetches the header at the range bottom,
gets nothing, and treats it as a branch change. The walk then fetches the header
at each anchor's height, every one of them at or below the cursor and every one
of them unavailable, so every comparison mismatches and the walk exhausts the
ring. The chain halts as a reorg deeper than the horizon.

The halt itself is the right outcome. Nothing is deleted, nothing is relayed
from a bad index, and a restart re-detects it. The reason is wrong, and the
reason is what an operator acts on: `reset --height H` is the natural response to
a deep reorg and it accomplishes nothing here, because every height below the tip
is equally unavailable.

**Telling the two apart**

Compare the failing height to head, which step 1 already read.

- Above head, the chain really is shorter than our anchor. This is the reorg case
  that 2a exists for.
- At or below head with the header unavailable, the canonical chain has a block
  there and the provider is not serving it. Walking backward will never find a
  common ancestor, because the problem is not the branch.

A single failed header read is an RPC blip, so this terminates only on a failure
that persists across retries. The walk therefore needs head threaded into it, and
a persistent unavailable header at or below head terminates as its own halt
reason rather than as a reorg.

**The case that does not halt**

The trace above assumes headers go missing first. In practice they usually
survive and logs do not. A full node keeps headers and receipts and prunes state,
so both `eth_getBlockByNumber` and `eth_getLogs` keep working over old history.
Hosted providers are where the limit bites, and they typically cap `eth_getLogs`
lookback on non-archive plans while serving headers fine.

In that case steps 4 and 5 succeed and step 6 fails. The error is neither
range-too-large nor too-many-results, but the errors are not standardised, so it
lands in the halving path anyway and the watcher retries at minimum chunk
forever. There is no halt, no unhealthy report, and no reorg detection. Lag
warnings fire and the process looks alive, which makes this the worse of the two
failures and the reason the halving floor exists.

**Chain client**

The watcher cannot make this distinction today without matching on error
strings. `GetBlockHeader` returns `errors.Errorf("header is nil for height %d")`
when the header is nil and a wrapped RPC error otherwise, with no sentinel
either way. `internal/types/v2/errors.go` already carries `ErrTxNotFound` for the
same shape of problem on the transaction side, so `ErrBlockNotFound` alongside it
is the consistent fix.

**Recovery**

Two operator actions:

- `ibc relayer sync reset <chain> --finalized` clears the anchors and lets the
  next start seed at the ceiling. The window is written off.
- Point the chain at an archive endpoint and restart. The halt left the anchors
  in place, so the watcher resumes at the same cursor, the parent-hash check
  passes against a provider that will serve the block, and it grinds forward.
  Switch back once it has caught up.

The second needs no command, because nothing about the sync state was wrong. It
is also rarely worth doing. If the relayer was down long enough for history to
age out, those packets are past their timeout timestamps, so recovering them
writes rows that reach `BatchTimeoutPacket` and cost gas on the source chain.
Re-seeding is the default and the runbook should say so.

This is a ceiling on halt-and-wait as a strategy. The halt is durable because the
state that caused it stays on disk, but durability does not help when the thing
the halt was protecting has aged out of the provider. Past some amount of
downtime, re-seeding is the only recovery regardless of what the anchors say.

### First-run Backfill

One cursor per chain leaves a gap when the watched set changes. Enable a client
on Monday and the cursor is already past Friday's blocks, so everything that
client sent in between is invisible.

`ibc relayer run --from-time <RFC3339>` closes it. The flag applies only to
clients the watcher has never scanned for. A client already being relayed on
ignores it entirely, so the flag is a one-time backfill per client rather than a
statement about the chain.

**Deciding what "never scanned for" means**

The chain's anchors cannot answer this. They say the chain was scanned, not which
clients were in the filter at the time, so a client added to an already-scanned
chain looks identical to one that has been watched since the start. The watcher
needs a record per client.

```sql
-- one row per client the watcher has ever scanned for
client_sync(chain_id, client_id, next_height, target_height, pk(chain_id, client_id))
```

The row is written the first time a client appears in a chain's watched set, and
it is never deleted. Its presence is the "seen before" record, which is why
completion is `next_height` reaching `target_height` rather than removing the
row. On every later start the row already exists, so `--from-time` is skipped for
that client without any comparison against the flag's value.

Both bounds are pinned at creation. `next_height` is the height `--from-time`
resolves to and `target_height` is the chain cursor at that moment, so the row
covers `[next_height, target_height)` while the tip scan covers
`[target_height, …)`. The two are contiguous and the tip scan never waits on the
backfill. Started without `--from-time`, the row is written with
`next_height == target_height`: seen, nothing to scan.

**Resolving a time to a height**

Bisect on `GetBlockHeader`, which already returns height and timestamp, and take
the first block whose timestamp is at or after the requested time. EVM block
timestamps strictly increase, so the search is sound. It costs about
`log2(head)` header calls per new client per start, which is roughly two dozen on
a chain with sixteen million blocks.

Two clamps. A time before genesis resolves to the chain's first block. A time
after head resolves to the ceiling, which produces an empty range.

**Two ordering requirements**

The row has to be written before that client's first tip scan in the same tick.
Otherwise the cursor advances between reading it and writing the row, and
`[stale target, real cursor)` is never covered for that client.

The resolved height can land above the chain cursor when the watcher is far
behind: resolve to block 5000 while the cursor sits at 1000 and there is nothing
below the cursor to recover. Write the row complete. The tip scan already has the
client in its filter from the cursor upward and will cover more history than the
flag asked for, which is harmless.

**Draining**

Backfill rows run after the tip scan on the same tick, one chunk each. Take
`[next_height, min(target_height, next_height + chunk)]`, fetch logs filtered to
the row's client, then write packets and advance `next_height` in one
transaction. They write no anchors and do no parent-hash checking, because every
height they cover is already anchored by the tip scan and a reorg reaching into
that window is corrected by the tip scan's rollback. Running the tip scan first
keeps a large backfill from starving live routes.

Enabling several clients at once with one `--from-time` produces rows with
identical ranges and different filters. Those can share a single log query per
range.

**The flag persists across restarts**

`--from-time` lives in whatever starts the process, so it stays set unless
someone removes it. A client enabled six months later backfills from the same
fixed date, and that range grows without bound. Recovered packets past their
timeout timestamps reach `BatchTimeoutPacket` and cost gas on the source chain.

Putting this on `run` rather than a separate command means there is no
confirmation prompt to gate it behind. What is left is logging: on each new
client, log the requested time, the resolved height, and the block count at warn
before the first chunk runs.

### Watched Set Changes

- **First run for a chain.** No anchors, so seed the cursor at the height
`--from-time` resolves to, or at the ceiling without it. Every watched client
gets a `client_sync` row written complete, since the chain cursor already covers
everything the flag asked for.
- **Enabling a client on a scanned chain.** Gets a `client_sync` row covering
`[resolved height, cursor)`, or a complete row without the flag. The client joins
the topic filter on the next tick either way.
- **Disabling a client.** Removed from the topic filter. The `client_sync` row
stays, so re-enabling later does not backfill again. Packets already in flight
keep being dispatched and, with the route gone from the pipeline set, get marked
`FAILED`. This hazard predates this work and should be fixed separately.
- **Re-enabling after a long gap.** Resumes at the tip, because the row already
exists. Packets from the disabled period are past their timeout timestamps, so
recovering them would mean paying for timeout submissions. An operator who wants
them anyway deletes the row and restarts with `--from-time`.

### Changes Outside the Watcher

**Status API.** `Service.Status` returns not-found unless a `relay_requests`
row exists, and auto-discovered packets have none. `Status` falls back to
packet rows. The alternative is writing a `relay_requests` row per discovered
transaction, which leaves the API untouched but overloads a table that means "a
human asked for this".

**`CheckSendFinality` gains an orphan deadline.** A send transaction still not
found past the threshold becomes a terminal status rather than a retry. It
wants its own constant, not the 30-minute `nodeLagWarningAfter`.

**`CreatePacket` changes its conflict clause** to `DO UPDATE` guarded on
`packets.recv_tx_hash IS NULL`. This is a no-op for the manual path, which
rewrites identical values. Both store implementations discard the `:execrows`
return, so nothing depends on the old semantics.

**Manual and automatic coexist.** Both producers write the same rows with the
same conflict handling, and `Deduper` refuses a transfer already in flight.
Routes with `autoRelay.enabled: false` behave as they do today, per
`TestTransfer_ManualRelay`.

**The e2e harness stand-in goes away.** `observeStatus` in
`e2e/internal/e2etest/traffic_relayer.go` calls `Relay` itself on non-manual
routes, standing in for on-chain packet discovery. Removing that branch turns
`TestTransfer_AutoRelay`, `TestGMPCall_AutoRelay` and
`TestIFTTransfer_AutoRelay` into real coverage without rewriting them.

### Configuration

`autoRelay` reduces to a single flag:

```yaml
connections:
  - alias: "eth-base"
    clientA:
      chainId: "1"
      clientId: "base-0"
      autoRelay:
        enabled: true
    clientB:
      chainId: "8453"
      clientId: "ethereum-0"
```

It sits on the client end rather than the connection because auto-relay is
directional: the block above watches what chain 1 sends and leaves chain 8453
to the API. The watched set is derived from these blocks, so there is no second
place to keep in sync and no way to express "watch a client that is not part of
a connection".

**`autoRelay.lookback` is removed.** History below a chain's cursor is reached
through `--from-time` on `ibc relayer run`. Keeping `lookback` would mean two
mechanisms covering the same blocks, one counting backward in blocks per route and
one forward from a timestamp per process, and the config one applies on every
restart rather than once per client.

The field exists in `config.AutoRelayConfig` today and nothing reads it, so
removing it deletes a struct field, one line of `internal/config/testdata/sample.yml`,
and one assertion in `relayer_test.go`.

Additions, per chain and all optional, under `relayer.chainOverrides[]`: poll
interval, scan chunk size, confirmations offset, reorg horizon, maximum client
ids per log query, and the orphan deadline.

The orphan deadline is per chain and applies to the manual path too, being a
property of the source chain rather than of how the packet was found. Default
it to a comfortable multiple of the reorg horizon in wall-clock terms. Erring
long delays writing off a dead packet; erring short terminates live packets
during an ordinary node outage.

`docs/configuration.md` needs a section for the whole `autoRelay` block, which
it currently lacks.

### Operations

**Single writer.** One process per chain writes `scan_anchors`, and concurrent
watchers are not made safe. Multi-process is not a new condition, since two
dispatchers already poll the same unfinished packets, but duplicate dispatch is
idempotent while `scan_anchors` is derived state whose correctness depends on
an invariant. Active-active needs a lease or leader election and is a separate
ticket.

**Observability.** Watcher lag (`head - cursor`) at debug every tick, with a
warning on sustained increase. Reorg detections, rollback depth, consecutive
step 7 discards, and orphan-deadline terminations at warn with chain and height
range. A persistent nonzero discard count means an inconsistent RPC endpoint.
No metrics stack in this ticket; structured logging, and optionally the
readiness JSON gaining watcher state.

The two halts carry distinct reasons, because they have distinct runbook
entries: a reorg deeper than the horizon, and history the provider will not
serve. Both report unhealthy. A chunk size sitting at the halving floor is the
early warning for the second one and is worth logging on its own.

**Shutdown.** Follow the dispatcher's `Start`/`Stop` pattern where `Stop`
blocks until the loop exits.

**`ibc relayer sync`**

One path above needs an operator to move sync state: restarting a chain halted on
a reorg deeper than the horizon. That is otherwise a hand-edit to the database.

A sibling of `ibc relayer run`, reading the database from config the way `ibc
migrate` does:

```shell
# cursor, head, lag, and outstanding backfills
ibc relayer sync status

# delete anchors above H, or all of them
ibc relayer sync reset <chain> [--height H | --finalized]
```

`status` gets used routinely and should be built first. It is also the only way
an operator can pick an argument for `reset`, and the only way to see whether a
`--from-time` backfill is still draining.

`reset` with no anchors left means the next start seeds like a first run, so
`--finalized` means "clear and let it re-seed". This is also the recovery for a
cursor outside the provider's history window, where it is the only option that
works. Deleting anchors above a height is what the rollback path already does, so
there is no new store method.

`reset` mutates state a running watcher owns. It should refuse to run when it can
tell the relayer is live, and the help text should say to stop it first. Detecting
that is not free without a lease, so we use a confirmation prompt and a warning
rather than a check that pretends to be reliable.

### Testing

**Unit**, against a fake chain client driving a scripted chain. The reorg cases
matter most, since they are close to impossible to trigger on demand in e2e.

- Forward scan, restart resuming from the anchors, and chunk-limited catch-up.
- A shallow reorg re-including the same packets, a reorg dropping a packet, a
reorg moving a packet to a different transaction, and a reorg deeper than the
horizon.
- Mid-scan branch changes: a fork above the range bottom appearing between the
top header read and the log query, a fork below the bottom in the same window,
and a replacement branch shorter than the range top. Each asserts the range is
discarded and that the following tick commits against the new branch. A test
checking only "no rows written" passes against a watcher that silently stopped.
- Step 5/6 ordering. Script the fork to land between the log query and where
the read would be, so the test fails if the top header read moves below the log
query. Single-block cases keep passing, so this needs its own test.
- Rollback leaves the cursor at `fork + 1` and the next tick's parent check
passes. A walk exhausting the ring deletes no anchors and halts. A restart
after that halt re-detects rather than re-seeding.
- Traffic for unconfigured clients produces no rows. A client enabled after the
cursor advanced starts at the tip without `--from-time` and gets a complete
`client_sync` row. With `--from-time` it gets a row covering
`[resolved, cursor)`, drains it without affecting the cursor, and stops at its
target. A backfill larger than the chunk size does not delay the tip scan.
- `--from-time` is ignored for a client that already has a row, including when
the flag names an earlier time than the row was created with. A row written
before that client's first tip scan covers the full window; one written after
would leave `[stale target, real cursor)` uncovered, which is the assertion that
pins the ordering.
- Time resolution: a timestamp between two blocks picks the later one, one before
genesis picks the first block, and one after head produces an empty range and a
complete row.
- Orphan deadline: a permanently missing source transaction reaches the
terminal status, and one reappearing within the deadline does not.
- Unavailable history. A scripted chain that serves head but returns not-found
for every header at or below the cursor halts on the data-availability reason
rather than walking the ring, and a transient not-found that clears on retry does
not halt at all. A chain that serves headers but refuses the log query stops at
the halving floor instead of retrying at minimum chunk, and halts on the same
reason.

**Store**, against SQLite and Postgres per `store_test.go`: cursor derivation,
the rollback delete, `client_sync` insert-if-absent and advance, and the
rediscovery upsert including the `recv_tx_hash IS NULL` guard.

**End to end.** Removing the harness stand-in makes the three existing
auto-relay tests meaningful. Add one reorg test against Anvil using
`evm_snapshot`/`evm_revert` to rewind past a send and mine a different branch,
asserting the packet is orphaned and no relay transaction is submitted. Keep it
to one, since the mechanics are slow and the unit tests carry the coverage.

### Options Considered

Enabling a client after its chain's cursor has advanced leaves a window of
blocks that client sent in but the watcher never scanned. How that window gets
covered is the main design decision here.

### Option A: Per-route `lookback` in configuration

Keep `autoRelay.lookback` and create a backfill row whenever a watched client's
configured lookback starts below the chain cursor.

- **Pros:**
	- Enabling a client backfills without touching the start command.
	- The window is expressed next to the route it belongs to.
- **Cons:**
	- Applies on every restart rather than once per client, so the row has
	to be compared against config forever rather than just existing.
	- A block count is the wrong unit for an operator, who knows when
	something happened and not at what height.
	- Different routes on one chain can ask for different depths, so the
	chain needs a rule for reconciling them.

### Option B: Rewind the chain cursor

Set the chain cursor back to the requested height and let the existing loop
re-cover the range.

- **Pros:**
	- No second table and no second branch in the tick.
	- Reuses the rollback delete, so there is no new store method.
- **Cons:**
	- Clients already running at the tip stop being scanned for the
	duration of the catch-up. For a range of any size this is an outage for
	routes that were working, and their live packets can pass their timeout
	timestamps while invisible.

### Option C: `--from-time` on `ibc relayer run`

Resolve the timestamp to a height once per client the watcher has not seen
before, and record the result in `client_sync`.

- **Pros:**
	- Backfill rows drain alongside the tip scan, so live routes keep
	running.
	- One flag covers every chain in the config, because a timestamp is
	chain-independent in a way a height is not.
	- Ignored for known clients, so a restart is never a scan of history.
	- Answers "which clients have we scanned for" as a side effect, which
	the chain's anchors cannot do.
- **Cons:**
	- Needs a per-client row that is never deleted, since its presence is
	the record.
	- The row must be written before that client's first tip scan in the
	same tick, or `[stale target, real cursor)` is never covered for it.
	- A daemon flag has no confirmation prompt, so a large backfill starts
	on a restart with only a log line in front of it.
	- Left set in a unit file, it applies to every client added later from
	the same fixed date.

### Decision

Option C.

Option A and Option C both need a per-client row, so the row is not what
separates them. What separates them is when the flag is read: Option A consults
config on every start and has to keep deciding whether this client's window is
already covered, while Option C reads the flag only when the row is absent. That
makes "first time" a property of the database rather than a comparison, and it is
the reason the row can be dumb.

Time rather than height because an operator knows when to relay from, not at what
block, and because one timestamp is meaningful across every chain in the config
while one height is not. Resolution costs about two dozen header calls per new
client per start.

Option B stays rejected for the reason it was before. A chain-level rewind trades
an outage on working routes for the convenience of not having a second table.

### Tradeoffs Accepted

- A chain's first run and every newly enabled client relay from the tip unless
`--from-time` is passed. There is no default lookback.
- The `client_sync` row is permanent. It is one row per client per chain, so the
table stays small, but it never gets cleaned up.
- Recovering the disabled period for a client that was watched before means
deleting its row by hand and restarting. No command covers that.
- A large backfill starts from a process restart with no interactive
confirmation. Logging the resolved height and block count is the only guard.
- Backfill rows compete with the tip scan for the chain's tick. Ordering the tip
scan first bounds the effect to one chunk per tick.

### Sequencing

The sections above are in dependency order, and the work follows them.

1. Chain client range scan, plus `v2.ErrBlockNotFound` so callers can tell a
missing block from a broken endpoint.
2. Migrations, store methods, and the `CreatePacket` conflict clause.
3. Forward scanning and client scoping, wired into bootstrap behind
`autoRelay.enabled`, including the chunk-halving floor. Drop
`autoRelay.lookback` here, since this is the step that would otherwise have
consumed it.
4. Block identity, reorg detection, the mid-scan bracket, rollback, the rewind
check, and `ibc relayer sync reset`. The halt is unrecoverable without it. The
above-head test that separates a shortened chain from unavailable history belongs
here, since it is part of the walk.
5. `CheckSendFinality` orphan deadline.
6. `client_sync` rows, time-to-height resolution, and `--from-time` on
`ibc relayer run`. The rows come first: written complete on every new client, they
make the watched set observable before the flag exists to fill them.
7. Status API fallback, harness stand-in removal, and configuration docs.

`ibc relayer sync status` is worth pulling forward to step 3, where it becomes
the easiest way to see whether the watcher is working.

The first three are independently safe to merge, since a watcher without reorg
handling is correct on any chain that does not reorg. Step 5 is independent of
the watcher and could land first, since it fixes ghost packets on the manual
path too.

### Open Questions

- Should the scan ceiling default to head or finalized head? A default of 0
assumes a single or consistent RPC endpoint.
- Does `relay_requests` record human intent, or every transaction the relayer
has seen? This decides whether the status API fallback is right.
- How long is the orphan deadline? It has to clear the deepest plausible reorg
on the slowest chain we relay from, and it is the only thing between a ghost
packet and an infinite retry loop. A chain-level default derived from the reorg
horizon is probably right.
