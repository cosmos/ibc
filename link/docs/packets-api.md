# Listing packets

The relayer exposes one read API, `Packets`, which lists the packets it is
aware of together with their relay status. It replaces the removed `Status`
endpoint, which could only report on a single source transaction.

## RPC

```proto
rpc Packets(PacketsRequest) returns (PacketsResponse) {}
```

Every filter field is optional, and a packet must match all the fields that are
set. An absent filter matches every packet.

| Filter | Matches |
| --- | --- |
| `source_chain_id` / `destination_chain_id` | the packet's chains |
| `source_client_id` / `destination_client_id` | the packet's clients |
| `state` | the relay state (see below) |
| `source_tx_hash` | the source-chain `SendPacket` transaction only |
| `sequence_number` | the packet sequence |
| `created_from` / `created_to` | unix seconds, inclusive, against discovery time |

`source_tx_hash` is normalized before lookup, so casing does not matter.

## States are not internal statuses

`state` takes the API's `PacketState`, not the relayer's internal relay status.
The relayer tracks nineteen statuses as a packet moves through the pipeline;
the API reports six. Fourteen of the internal statuses are in-flight and all
report as `PACKET_STATE_PENDING`.

Filtering on `PACKET_STATE_PENDING` therefore returns every in-flight packet,
whichever pipeline stage it currently sits in — not only packets in the literal
`PENDING` status. The expansion is derived from the same mapping used to render
a packet's state, so the two cannot disagree.

## Paging

`limit` defaults to 100 and is capped at 1000. `offset` skips packets. Results
are ordered most recent first, and `total` reports how many packets match the
filter, ignoring `limit` and `offset`.

Paging is **not a consistent snapshot**. The relayer is writing to this table
while you read it, so a packet inserted between two requests shifts later pages
and can be seen twice or missed. For a stable view of a moving set, filter to a
closed `created_to` bound.

A cursor over `id` would not fix this and would be worse: ids come from a
sequence, so they contain gaps from rolled-back transactions, and a packet
assigned a lower id can commit after a higher one. A `WHERE id < cursor` pager
would skip those packets permanently rather than merely reordering them.

## Absence is emptiness

A filter that matches nothing returns an empty list, not an error. This
includes a `source_tx_hash` the relayer has never seen. Callers distinguishing
"not indexed yet" from "no packets" should treat an empty result as the former
while the transaction is recent.

## Performance

Only the per-transaction read is indexed. The generic filters are compiled to
one SQL statement whose optional clauses are `COALESCE(param, column)`, which
the query planner cannot satisfy from an index — it plans a sequential scan
even when a matching index exists.

That is acceptable at the sizes this table reaches in practice, and paging with
no filter is cheap because the ordering walks the primary key and stops at
`limit`. A selective filter over a large table is the case that degrades. The
fix, if it becomes necessary, is to specialize the query per filter combination
rather than to add indexes the current statement cannot use.

## CLI

```
ibc relayer packets --state pending
ibc relayer packets --chain-id 1 --tx-hash 0xabc
ibc relayer packets --source-client-id base-0 --limit 20
ibc relayer packets --created-from 2026-08-01T00:00:00Z
```

`--state` accepts `not-selected`, `pending`, `succeeded`, `timed-out`,
`rejected`, and `relay-failed`. Times are RFC3339. Output is JSON.
