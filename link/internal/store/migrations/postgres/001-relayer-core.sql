-- +migrate Up

create table if not exists packets (
    id                           bigserial                PRIMARY KEY,
    created_at                   timestamp with time zone NOT NULL DEFAULT now(),
    updated_at                   timestamp with time zone NOT NULL DEFAULT now(),
    status                       text                     NOT NULL DEFAULT 'PENDING',
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

-- +migrate Down
drop table if exists relay_requests;
drop table if exists packets;
