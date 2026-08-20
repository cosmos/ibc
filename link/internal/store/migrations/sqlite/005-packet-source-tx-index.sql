-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create index if not exists index_packets_source_tx
    on packets (source_chain_id, source_tx_hash);

-- +migrate Down

drop index if exists index_packets_source_tx;
