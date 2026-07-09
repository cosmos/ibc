-- +migrate Up

-- replaces the pre-release relay_submissions table
drop table if exists relay_submissions;

create type relay_status as enum (
    'PENDING',
    'AWAITING_SEND_FINALITY',
    'CHECK_RECV_PACKET_DELIVERY',
    'GET_RECV_PACKET',
    'DELIVER_RECV_PACKET',
    'WAIT_FOR_WRITE_ACK',
    'AWAITING_WRITE_ACK_FINALITY',
    'CHECK_ACK_PACKET_DELIVERY',
    'GET_ACK_PACKET',
    'DELIVER_ACK_PACKET',
    'AWAITING_TIMEOUT_FINALITY',
    'CHECK_TIMEOUT_PACKET_DELIVERY',
    'GET_TIMEOUT_PACKET',
    'DELIVER_TIMEOUT_PACKET',
    'COMPLETE_WITH_ACK',
    'COMPLETE_WITH_WRITE_ACK_SUCCESS',
    'COMPLETE_WITH_WRITE_ACK_ERROR',
    'COMPLETE_WITH_TIMEOUT',
    'FAILED'
);

create type write_ack_status as enum ('SUCCESS', 'ERROR', 'UNKNOWN');

create table if not exists transfers (
    id                           bigserial                PRIMARY KEY,
    created_at                   timestamp with time zone NOT NULL DEFAULT now(),
    updated_at                   timestamp with time zone NOT NULL DEFAULT now(),
    status                       relay_status             NOT NULL DEFAULT 'PENDING',
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
    write_ack_status             write_ack_status,
    ack_tx_hash                  text,
    ack_tx_time                  timestamp with time zone,
    ack_tx_relayer_address       text,
    timeout_tx_hash              text,
    timeout_tx_time              timestamp with time zone,
    timeout_tx_relayer_address   text
);

create unique index if not exists index_transfer_packet
    on transfers (source_chain_id, packet_sequence_number, packet_source_client_id);

create index if not exists idx_transfers_recv_time_chain_ids
    on transfers (recv_tx_time, source_chain_id, destination_chain_id)
    include (source_tx_time);

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
drop table if exists transfers;
drop type if exists write_ack_status;
drop type if exists relay_status;
