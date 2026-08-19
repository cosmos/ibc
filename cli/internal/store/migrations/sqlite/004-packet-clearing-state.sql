-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists packet_clearing_state (
    source_chain_id         text      not null,
    packet_source_client_id text      not null,
    last_probed_sequence    integer   not null,
    updated_at              timestamp not null default current_timestamp,

    primary key (source_chain_id, packet_source_client_id)
);

create table if not exists packet_clearing_unresolved (
    source_chain_id         text      not null,
    packet_source_client_id text      not null,
    packet_sequence_number  integer   not null,
    first_seen_at           timestamp not null default current_timestamp,

    primary key (source_chain_id, packet_source_client_id, packet_sequence_number)
);

-- +migrate Down
drop table if exists packet_clearing_unresolved;
drop table if exists packet_clearing_state;
