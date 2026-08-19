-- SPDX-License-Identifier: Apache-2.0

-- Serves ListPacketsBySourceTx. The generic ListPackets filters use
-- COALESCE(param, column), which no index can satisfy -- EXPLAIN reports a
-- scan even with matching indexes -- so indexing those columns would cost
-- writes and never be read.

-- +migrate Up

create index if not exists index_packets_source_tx
    on packets (source_chain_id, source_tx_hash);

-- +migrate Down

drop index if exists index_packets_source_tx;
