-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up

create table if not exists relayer_tx_operations (
    packet_id     bigint NOT NULL references packets (id),
    submission_id bigint NOT NULL references relayer_tx_submissions (id),
    type          text   NOT NULL check (type in ('recv', 'ack', 'timeout')),

    primary key (packet_id, submission_id, type)
);

create index relayer_tx_operations_submission on relayer_tx_operations(submission_id);

-- +migrate Down
drop table if exists relayer_tx_operations;
