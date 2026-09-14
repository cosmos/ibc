-- SPDX-License-Identifier: Apache-2.0

-- +migrate Up
CREATE TABLE client_updates (
 chain_id TEXT NOT NULL,
 client_id TEXT NOT NULL,
 tx_hash TEXT NOT NULL,
 submitted_at BIGINT NOT NULL,
 relayer_address TEXT NOT NULL,
 PRIMARY KEY (chain_id, client_id)
);

-- +migrate Down
DROP TABLE client_updates;
