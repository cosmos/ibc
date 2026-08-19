-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists packet_clearing_state (
    source_chain_id         text   NOT NULL,
    packet_source_client_id text   NOT NULL,
    last_probed_sequence    bigint NOT NULL,
    updated_at              timestamp with time zone NOT NULL default now(),

    primary key (source_chain_id, packet_source_client_id)
);

create table if not exists packet_clearing_unresolved (
    source_chain_id         text   NOT NULL,
    packet_source_client_id text   NOT NULL,
    packet_sequence_number  bigint NOT NULL,
    first_seen_at           timestamp with time zone NOT NULL default now(),

    primary key (source_chain_id, packet_source_client_id, packet_sequence_number)
);

-- +migrate Down
drop table if exists packet_clearing_unresolved;
drop table if exists packet_clearing_state;
