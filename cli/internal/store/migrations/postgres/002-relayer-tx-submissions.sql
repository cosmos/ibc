-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists relayer_tx_submissions (
    id                    bigserial                PRIMARY KEY,
    tx_hash               text                     NOT NULL,
    chain_id              text                     NOT NULL,
    relayer_address       text                     NOT NULL,
    submitted_at          timestamp with time zone NOT NULL DEFAULT now(),
    resolved_at           timestamp with time zone,
    gas_cost_amount       numeric,
    status                text                     NOT NULL,
    execution_error       text,
    nonce                 bigint                   NOT NULL,
    signed_tx             bytea                    NOT NULL,
    last_broadcast_at     timestamp with time zone,
    resolved_block_number bigint,
    resolved_block_hash   text,

    constraint relayer_tx_submissions_tx_unique
        unique (chain_id, tx_hash)
);

create index relayer_tx_nonce on relayer_tx_submissions(chain_id, relayer_address, nonce);
create index relayer_tx_pending on relayer_tx_submissions(chain_id, relayer_address, status);

-- +migrate Down
drop table if exists relayer_tx_submissions;
