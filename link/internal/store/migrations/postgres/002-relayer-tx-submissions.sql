-- +migrate Up

create type ibcv2_relayer_tx_submission_type as enum ('RECV', 'ACK', 'TIMEOUT');

create type ibcv2_relayer_tx_submission_status as enum ('PENDING', 'CONFIRMED', 'FAILED');

create table if not exists ibcv2_relayer_tx_submissions (
    id              bigserial                          PRIMARY KEY,
    tx_hash         text                               NOT NULL,
    chain_id        text                               NOT NULL,
    tx_type         ibcv2_relayer_tx_submission_type   NOT NULL,
    relayer_address text                               NOT NULL,
    submitted_at    timestamp with time zone           NOT NULL DEFAULT now(),
    resolved_at     timestamp with time zone,
    gas_cost_amount numeric,
    status          ibcv2_relayer_tx_submission_status NOT NULL DEFAULT 'PENDING',
    execution_error text,

    constraint ibcv2_relayer_tx_submissions_tx_unique
        unique (chain_id, tx_hash)
);

create table if not exists ibcv2_transfer_tx_submissions (
    transfer_id   bigint NOT NULL references ibcv2_transfers (id),
    submission_id bigint NOT NULL references ibcv2_relayer_tx_submissions (id),

    primary key (transfer_id, submission_id)
);

-- +migrate Down
drop table if exists ibcv2_transfer_tx_submissions;
drop table if exists ibcv2_relayer_tx_submissions;
drop type if exists ibcv2_relayer_tx_submission_status;
drop type if exists ibcv2_relayer_tx_submission_type;
