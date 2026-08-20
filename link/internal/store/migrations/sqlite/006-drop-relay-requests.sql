-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up
drop table if exists relay_requests;

-- +migrate Down
create table if not exists relay_requests (
    id              integer   primary key,
    source_chain_id text      not null,
    source_tx_hash  text      not null,
    created_at      timestamp not null default current_timestamp,

    constraint relay_requests_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);
