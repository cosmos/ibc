-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists relay_requests (
    id              bigserial                PRIMARY KEY,
    source_chain_id text                     NOT NULL,
    source_tx_hash  text                     NOT NULL,
    created_at      timestamp with time zone NOT NULL DEFAULT now(),

    constraint relay_requests_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);

-- +migrate Down
drop table if exists relay_requests;
