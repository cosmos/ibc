# Plan: `/packets` endpoint and `ibc relayer packets`

## Context

The relayer exposes one read API today, `Status`, and it is scoped to a single
source transaction:

```proto
rpc Status(StatusRequest) returns (StatusResponse) {}
message StatusRequest { string tx_hash = 1; string source_chain_id = 2; }
```

`Service.Status` (`link/internal/service/relayer/service.go:311`) validates the
arguments, confirms a relay request exists for that transaction, then calls
`ListPacketsBySourceTx`. An operator who wants to answer "what is this relayer
working on right now" or "which packets failed today" has no way to ask.

**Outcome:** a `Packets` RPC that lists every packet the relayer knows about,
filtered by chain, client, status, transaction hash, and sequence; a
`ibc relayer packets` CLI command; and removal of `Status` and
`ibc relayer status` once callers have migrated.

`Packets` must be a strict superset: `filter{source_chain_id, source_tx_hash}`
has to return exactly what `Status` returned for the same inputs.

## Design decisions

**D1 — A new `Packets` RPC rather than widening `Status`.** The two coexist
during migration, so the e2e harness and any operator scripts can move without a
flag day. `Status` is deleted in its own phase.

**D2 — Filters stay in sqlc, using `sqlc.narg`.** Queries are the single source
of truth for both engines: `link/internal/store/sqlc.yml` generates
`repository/sqlite` and `repository/postgres` from one `queries/relayer.sql`.
Optional filters become nullable params:

```sql
-- name: ListPackets :many
SELECT * FROM packets
WHERE (sqlc.narg(source_chain_id) IS NULL OR source_chain_id = sqlc.narg(source_chain_id))
  AND (sqlc.narg(status)          IS NULL OR status          = sqlc.narg(status))
  ...
ORDER BY id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
```

The alternative — building SQL dynamically in Go — would give better query plans
per filter combination but forks the two engines and abandons generated types.
Not worth it at this scale; see the index note under Risks.

Note `query_parameter_limit: 2` in `sqlc.yml` means anything past two arguments
already generates a params struct, which is what we want here.

**D3 — Pagination is mandatory.** `Status` returns at most the packets in one
transaction. An unfiltered `Packets` returns the entire table. Use
`limit` (default 100, max 1000) and `offset`, ordered `id DESC` so the default
view is "most recent first".

Deliberately *not* a keyset cursor on `id`: `packets.id` is `bigserial`, so it
has gaps from rolled-back transactions, and a row assigned a lower id can commit
after a higher one. A cursor of `id < last_seen` would silently skip those rows
forever. Offset pagination skews under concurrent inserts but never permanently
drops a row. Document the caveat rather than implying exactness.

**D4 — Empty list, not `NotFound`.** `Status` returns connect `CodeNotFound`
when the transaction was never submitted, and the e2e harness depends on it
(`ibclink.IsStatusNotFound`, `daemon.go:288`). List semantics should return an
empty result instead. This is a real behavioural change and the harness
migration in Phase 3 has to absorb it.

**D5 — `source_tx_hash` is its own filter, distinct from a general `tx_hash`.**
A packet's hash can appear in four roles (send, recv, ack, timeout). To keep the
superset guarantee exact, `source_tx_hash` matches only the send transaction —
reproducing `Status` — while an optional `tx_hash` matches any role for
operators chasing a hash they cannot attribute.

## Proto

```proto
rpc Packets(PacketsRequest) returns (PacketsResponse) {}

message PacketsRequest {
  PacketFilter filter = 1;
  uint32 limit  = 2;   // default 100, capped at 1000
  uint32 offset = 3;
}

message PacketFilter {
  optional string source_chain_id       = 1;
  optional string destination_chain_id  = 2;
  optional string source_client_id      = 3;
  optional string destination_client_id = 4;
  optional PacketState state            = 5;
  optional string source_tx_hash        = 6;  // send tx only; reproduces Status
  optional string tx_hash               = 7;  // matches send, recv, ack, or timeout
  optional uint64 sequence_number       = 8;
}

message PacketsResponse {
  repeated PacketStatus packets = 1;
  // Rows matching the filter ignoring limit/offset. Costs a second COUNT
  // query; omitted when the caller sets include_total = false.
  uint64 total = 2;
}
```

`PacketStatus` is reused unchanged so the superset claim is structural, not just
behavioural. Two additive fields are worth including for a listing view:
`created_at` and `updated_at`.

## Phases

Each phase leaves the tree building and tests green.

### Phase 1 — Query, service, handler

- `link/internal/store/queries/relayer.sql`: add `ListPackets` and
  `CountPackets`. Run `sqlc generate` (both engines regenerate from this file).
- `link/internal/store/store_sqlite.go` / `store_postgres.go`: add `ListPackets`
  to both, following `ListPacketsBySourceTx`.
- `link/internal/service/relayer/service.go`: add `Packets(ctx, filter, page)`,
  extending the `Store` interface (line 43). Reuse `validateRelayArgs`' hash
  normalisation for the tx-hash filters so lookups stay case-insensitive —
  that is why `Status` is case-insensitive today and the new endpoint must match.
- `link/internal/server/relayer_handler.go`: add the `Packets` handler.
- `proto/link/relayer.proto` + `make proto-gen`.

**Done when:** `Packets` with `{source_chain_id, source_tx_hash}` returns byte-
identical `PacketStatus` values to `Status` for the same transaction, asserted
in a test that calls both.

### Phase 2 — CLI

- `link/cmd/ibc/relayer.go`: add `cmdRelayerPackets` with flags mapping to the
  filter fields plus `--limit` / `--offset`. The existing generic `relayerCall`
  helper works unchanged.
- Leave `ibc relayer status` in place, marked deprecated in its `Short` text.

**Done when:** `ibc relayer packets --source-chain-id X --source-tx-hash Y`
prints the same JSON payload as `ibc relayer status` for the same arguments.

### Phase 3 — Migrate the e2e harness

- `e2e/internal/harness/ibclink/daemon.go`: reimplement `PacketStatuses` on
  `Packets`; replace `probeStatusEndpoint` (`daemon.go:154`) with a `Packets`
  probe.
- `IsStatusNotFound` (`daemon.go:288`) has no equivalent — callers must treat an
  empty result as "not yet indexed". `e2etest/traffic_relayer.go:observeStatus`
  is the one place that matters; `AwaitState` already documents returning
  `(nil, nil)` when the transaction is not indexed, so the shape survives.

**Done when:** the full e2e suite passes with no remaining `Status` call.

### Phase 4 — Remove `Status`

- Delete the RPC and its messages from the proto, `Service.Status`, the handler,
  `cmdRelayerStatus`, and `IsStatusNotFound`.
- `ListPacketsBySourceTx` becomes unused unless `Packets` reuses it; delete or
  keep deliberately.

**Done when:** no reference to `Status` remains outside changelog/docs, and
`make build-link && make test-e2e` pass.

### Phase 5 — Indexes and docs

The only index on `packets` today is
`unique (source_chain_id, packet_sequence_number, packet_source_client_id)`
(`migrations/*/001-packets.sql`). Every new filter therefore sequential-scans.
Add parallel migrations under `migrations/sqlite` and `migrations/postgres` for:

- `(source_chain_id, source_tx_hash)` — the `Status`-equivalent path
- `(status)` — the "show me everything pending / failed" query
- `(packet_source_client_id)` — per-client views

Document the endpoint and its pagination caveat in `link/docs/`.

## Verification

```bash
cd link && go build ./... && go test ./...
make lint
cd .. && make test-e2e
```

Behavioural checks:

1. `Packets{source_chain_id, source_tx_hash}` equals `Status` output for the
   same transaction, including ordering.
2. An unknown transaction hash returns an empty list, not an error.
3. `limit`/`offset` page deterministically over a seeded table.
4. Each filter narrows correctly, and filters combine as AND.
5. Both engines produce identical results — extend `store_test.go`, which
   already exercises sqlite and postgres.

## Risks

- **One query plan for every filter combination.** `sqlc.narg` guards mean the
  planner cannot specialise. Acceptable at current table sizes; Phase 5's
  indexes are the mitigation, and `EXPLAIN` on the common filters should be
  checked before declaring it done.
- **Removing `Status` is a breaking API change.** The `link` module has no
  `link/*` version tags, so the cost is limited to in-repo callers today — this
  gets much more expensive after a `link/v1.0.0` tag.
- **`NotFound` becomes an empty list.** Any caller distinguishing the two needs
  updating; the harness is the known one.
- **Pagination skew.** Concurrent inserts shift offsets between pages. Ordering
  `id DESC` keeps the common "recent packets" view stable at the head, which is
  where operators look.

## Not in scope

Streaming or subscription APIs; aggregate statistics endpoints; exposing
`relayer_tx_submissions` (gas, failure reasons, latency) — a natural follow-up,
since that is the table operators will want joined against these packets.
