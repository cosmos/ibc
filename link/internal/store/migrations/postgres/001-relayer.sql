-- +migrate Up

create table if not exists packets (
    id                           bigserial                PRIMARY KEY,
    created_at                   timestamp with time zone NOT NULL DEFAULT now(),
    updated_at                   timestamp with time zone NOT NULL DEFAULT now(),
    status                       text                     NOT NULL,
    status_text                  text,
    source_chain_id              text                     NOT NULL,
    destination_chain_id         text                     NOT NULL,
    source_tx_hash               text                     NOT NULL,
    source_tx_time               timestamp with time zone NOT NULL,
    source_tx_finalized_time     timestamp with time zone,
    packet_sequence_number       bigint                   NOT NULL,
    packet_source_client_id      text                     NOT NULL,
    packet_destination_client_id text                     NOT NULL,
    packet_timeout_timestamp     timestamp with time zone NOT NULL,
    recv_tx_hash                 text,
    recv_tx_time                 timestamp with time zone,
    recv_tx_relayer_address      text,
    write_ack_tx_hash            text,
    write_ack_tx_time            timestamp with time zone,
    write_ack_tx_finalized_time  timestamp with time zone,
    write_ack_status             text,
    ack_tx_hash                  text,
    ack_tx_time                  timestamp with time zone,
    ack_tx_relayer_address       text,
    timeout_tx_hash              text,
    timeout_tx_time              timestamp with time zone,
    timeout_tx_relayer_address   text
);

create unique index if not exists index_packet
    on packets (source_chain_id, packet_sequence_number, packet_source_client_id);

create table if not exists relay_requests (
    id              bigserial                PRIMARY KEY,
    source_chain_id text                     NOT NULL,
    source_tx_hash  text                     NOT NULL,
    created_at      timestamp with time zone NOT NULL DEFAULT now(),

    constraint relay_requests_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);

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

create table if not exists packet_tx_submissions (
    packet_id   bigint NOT NULL references packets (id),
    submission_id bigint NOT NULL references relayer_tx_submissions (id),

    primary key (packet_id, submission_id)
);

-- +migrate Down
drop table if exists packet_tx_submissions;
drop table if exists relayer_tx_submissions;
drop table if exists relay_requests;
drop table if exists packets;
