-- SPDX-License-Identifier: Apache-2.0

-- name: GetClientUpdate :one
SELECT * FROM client_updates WHERE chain_id = sqlc.arg(chain_id) AND client_id = sqlc.arg(client_id);

-- name: SaveClientUpdate :exec
INSERT INTO client_updates (chain_id, client_id, tx_hash, submitted_at, relayer_address)
VALUES (sqlc.arg(chain_id), sqlc.arg(client_id), sqlc.arg(tx_hash), sqlc.arg(submitted_at), sqlc.arg(relayer_address));

-- name: ClearClientUpdate :exec
DELETE FROM client_updates WHERE chain_id = sqlc.arg(chain_id) AND client_id = sqlc.arg(client_id);
