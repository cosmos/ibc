-- +migrate Up

create table if not exists packets (
    id                           integer   primary key,
    created_at                   timestamp not null default current_timestamp,
    updated_at                   timestamp not null default current_timestamp,
    status                       text      not null default 'PENDING',
    status_text                  text,
    source_chain_id              text      not null,
    destination_chain_id         text      not null,
    source_tx_hash               text      not null,
    source_tx_time               timestamp not null,
    source_tx_finalized_time     timestamp,
    packet_sequence_number       integer   not null,
    packet_source_client_id      text      not null,
    packet_destination_client_id text      not null,
    packet_timeout_timestamp     timestamp not null,
    recv_tx_hash                 text,
    recv_tx_time                 timestamp,
    recv_tx_relayer_address      text,
    write_ack_tx_hash            text,
    write_ack_tx_time            timestamp,
    write_ack_tx_finalized_time  timestamp,
    write_ack_status             text,
    ack_tx_hash                  text,
    ack_tx_time                  timestamp,
    ack_tx_relayer_address       text,
    timeout_tx_hash              text,
    timeout_tx_time              timestamp,
    timeout_tx_relayer_address   text
);

create unique index if not exists index_packet
    on packets (source_chain_id, packet_sequence_number, packet_source_client_id);

create table if not exists relay_requests (
    id              integer   primary key,
    source_chain_id text      not null,
    source_tx_hash  text      not null,
    created_at      timestamp not null default current_timestamp,

    constraint relay_requests_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);

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

create table if not exists packet_tx_submissions (
    packet_id   integer not null references packets (id),
    submission_id integer not null references relayer_tx_submissions (id),

    primary key (packet_id, submission_id)
);

-- +migrate Down
drop table if exists packet_tx_submissions;
drop table if exists relayer_tx_submissions;
drop table if exists relay_requests;
drop table if exists packets;
