-- +migrate Up

create table if not exists relayer_tx_submissions (
    id              bigserial                          PRIMARY KEY,
    tx_hash         text                               NOT NULL,
    chain_id        text                               NOT NULL,
    tx_type         text                               NOT NULL,
    relayer_address text                               NOT NULL,
    submitted_at    timestamp with time zone           NOT NULL DEFAULT now(),
    resolved_at     timestamp with time zone,
    gas_cost_amount numeric,
    status          text                               NOT NULL,
    execution_error text,

    constraint relayer_tx_submissions_tx_unique
        unique (chain_id, tx_hash)
);

-- +migrate Down
drop table if exists relayer_tx_submissions;
