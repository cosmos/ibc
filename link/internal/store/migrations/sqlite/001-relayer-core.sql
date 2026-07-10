-- +migrate Up

create table if not exists transfers (
    id                           integer   primary key,
    created_at                   timestamp not null default current_timestamp,
    updated_at                   timestamp not null default current_timestamp,
    status                       text      not null default 'PENDING' check (status in (
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
    )),
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
    write_ack_status             text      check (write_ack_status in ('SUCCESS', 'ERROR', 'UNKNOWN')),
    ack_tx_hash                  text,
    ack_tx_time                  timestamp,
    ack_tx_relayer_address       text,
    timeout_tx_hash              text,
    timeout_tx_time              timestamp,
    timeout_tx_relayer_address   text
);

create unique index if not exists index_transfer_packet
    on transfers (source_chain_id, packet_sequence_number, packet_source_client_id);

create table if not exists relay_requests (
    id              integer   primary key,
    source_chain_id text      not null,
    source_tx_hash  text      not null,
    created_at      timestamp not null default current_timestamp,

    constraint relay_requests_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);

-- +migrate Down
drop table if exists relay_requests;
drop table if exists transfers;
