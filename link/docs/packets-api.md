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
are ordered most recent first, and `has_more` reports whether further packets
match beyond the returned page — page until it is false.

There is deliberately no exact count. Producing one would require visiting every
matching packet on every request, which is the expensive part; `has_more` is
answered by fetching one row past the page and discarding it, so a request costs
about as much as the page it returns.

Paging is **not a consistent snapshot**. The relayer is writing to this table
while you read it, so a packet inserted between two requests shifts later pages
and can be seen twice. Ordering is newest first, so new arrivals land at the
front and push earlier pages down.

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

What keeps that acceptable is that the scan stops early. Ordering walks the
primary key and the query asks for only one row more than the page, so a listing
whose matches are recent reads roughly `limit` rows rather than the table. The
case that degrades is a selective filter whose matches are old, where the scan
runs until it finds enough of them.

This is why the API reports `has_more` rather than a count: any exact total
would force every request to visit every match, removing the early stop and
making the cheap case as expensive as the worst one. If filtering does become
too slow, the fix is to specialize the query per filter combination rather than
to add indexes the current statement cannot use.

## CLI

```
ibc relayer packets --state pending
ibc relayer packets --chain-id 1 --tx-hash 0xabc
ibc relayer packets --source-client-id base-0 --limit 20
```

`--state` accepts `not-selected`, `pending`, `succeeded`, `timed-out`,
`rejected`, and `relay-failed`. Output is JSON.
