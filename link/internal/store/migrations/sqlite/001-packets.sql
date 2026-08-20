-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists packets (
    id                           integer   primary key,
    created_at                   timestamp not null default current_timestamp,
    updated_at                   timestamp not null default current_timestamp,
    status                       text      not null,
    source_chain_id              text      not null,
    destination_chain_id         text      not null,
    source_tx_hash               text      not null,
    source_tx_time               timestamp not null,
    packet_sequence_number       integer   not null,
    packet_source_client_id      text      not null,
    packet_destination_client_id text      not null,
    packet_timeout_timestamp     timestamp not null,
    recv_tx_hash                 text,
    recv_tx_time                 timestamp,
    recv_tx_relayer_address      text,
    write_ack_tx_hash            text,
    write_ack_tx_time            timestamp,
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

-- Serves the per-transaction packet read.
create index if not exists index_packets_source_tx
    on packets (source_chain_id, source_tx_hash);

-- +migrate Down
drop table if exists packets;
