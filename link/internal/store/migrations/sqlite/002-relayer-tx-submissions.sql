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

create table if not exists transfer_tx_submissions (
    transfer_id   integer not null references transfers (id),
    submission_id integer not null references relayer_tx_submissions (id),

    primary key (transfer_id, submission_id)
);

-- +migrate Down
drop table if exists transfer_tx_submissions;
drop table if exists relayer_tx_submissions;
