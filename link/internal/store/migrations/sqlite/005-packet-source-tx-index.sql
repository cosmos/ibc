-- SPDX-License-Identifier: Apache-2.0

-- Serves ListPacketsBySourceTx, the per-transaction read. Verified with
-- EXPLAIN QUERY PLAN against a populated table:
--
--   SEARCH packets USING INDEX index_packets_source_tx
--       (source_chain_id=? AND source_tx_hash=?)
--
-- The generic ListPackets filters are deliberately not indexed. Their optional
-- filters are expressed as COALESCE(param, column), which the planner cannot
-- use an index for -- it reports SCAN packets even with matching indexes
-- present and statistics analyzed. Adding indexes for those columns would slow
-- every write while never being read, so they are left out until the query
-- shape can use them.

-- +migrate Up

create index if not exists index_packets_source_tx
    on packets (source_chain_id, source_tx_hash);

-- +migrate Down

drop index if exists index_packets_source_tx;
