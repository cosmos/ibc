-- +migrate Up

create table if not exists ibcv2_relayer_tx_submissions (
    id              integer   primary key,
    tx_hash         text      not null,
    chain_id        text      not null,
    tx_type         text      not null check (tx_type in ('RECV', 'ACK', 'TIMEOUT')),
    relayer_address text      not null,
    submitted_at    timestamp not null default current_timestamp,
    resolved_at     timestamp,
    gas_cost_amount text,
    status          text      not null default 'PENDING' check (status in ('PENDING', 'CONFIRMED', 'FAILED')),
    execution_error text,

    constraint ibcv2_relayer_tx_submissions_tx_unique
        unique (chain_id, tx_hash)
);

create table if not exists ibcv2_transfer_tx_submissions (
    transfer_id   integer not null references ibcv2_transfers (id),
    submission_id integer not null references ibcv2_relayer_tx_submissions (id),

    primary key (transfer_id, submission_id)
);

-- +migrate Down
drop table if exists ibcv2_transfer_tx_submissions;
drop table if exists ibcv2_relayer_tx_submissions;
