-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists relayer_tx_submissions (
    id              integer   primary key,
    tx_hash         text      not null,
    chain_id        text      not null,
    tx_type         text      not null,
    relayer_address text      not null,
    submitted_at    timestamp not null default current_timestamp,
    resolved_at     timestamp,
    gas_cost_amount text,
    status          text      not null,
    execution_error text,

    constraint relayer_tx_submissions_tx_unique
        unique (chain_id, tx_hash)
);

-- +migrate Down
drop table if exists relayer_tx_submissions;
