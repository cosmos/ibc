-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up
alter table relayer_tx_submissions add column nonce bigint not null;
alter table relayer_tx_submissions add column signed_tx blob not null;
alter table relayer_tx_submissions add column last_broadcast_at timestamp;
create index relayer_tx_nonce on relayer_tx_submissions(chain_id, relayer_address, nonce);
create index relayer_tx_pending on relayer_tx_submissions(chain_id, relayer_address, status);

-- +migrate Down
drop index relayer_tx_pending;
drop index relayer_tx_nonce;
alter table relayer_tx_submissions drop column last_broadcast_at;
alter table relayer_tx_submissions drop column signed_tx;
alter table relayer_tx_submissions drop column nonce;
