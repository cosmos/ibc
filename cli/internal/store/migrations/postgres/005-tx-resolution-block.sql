-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up
alter table relayer_tx_submissions add column resolved_block_number bigint;
alter table relayer_tx_submissions add column resolved_block_hash text;

-- +migrate Down
alter table relayer_tx_submissions drop column resolved_block_hash;
alter table relayer_tx_submissions drop column resolved_block_number;
