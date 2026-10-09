-- SPDX-License-Identifier: Apache-2.0

-- name: InsertTxAttempt :one
INSERT INTO relayer_tx_submissions
(tx_hash, chain_id, tx_type, relayer_address, submitted_at, status, nonce, signed_tx)
VALUES (sqlc.arg(tx_hash), sqlc.arg(chain_id), sqlc.arg(tx_type), sqlc.arg(relayer_address), sqlc.arg(submitted_at), 'PENDING', sqlc.arg(nonce), sqlc.arg(signed_tx))
RETURNING id;

-- name: LinkTxPacket :execrows
INSERT INTO packet_tx_submissions (packet_id, submission_id)
SELECT id, sqlc.arg(submission_id) FROM packets
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(source_client_id)
AND packet_sequence_number = sqlc.arg(sequence);

-- name: ListTxPackets :many
SELECT p.source_chain_id, p.packet_source_client_id, p.packet_sequence_number
FROM packets p JOIN packet_tx_submissions s ON p.id = s.packet_id
WHERE s.submission_id = sqlc.arg(submission_id);

-- name: ListPendingTxAttempts :many
SELECT * FROM relayer_tx_submissions
WHERE chain_id = sqlc.arg(chain_id) AND relayer_address = sqlc.arg(relayer_address)
AND status = 'PENDING'
ORDER BY nonce, id;

-- name: ListTxNoncesFrom :many
SELECT DISTINCT nonce FROM relayer_tx_submissions
WHERE chain_id = sqlc.arg(chain_id) AND relayer_address = sqlc.arg(relayer_address)
AND nonce >= sqlc.arg(mined_nonce)
ORDER BY nonce;

-- name: ListTxResolutionsFrom :many
SELECT DISTINCT nonce, resolved_block_number, resolved_block_hash FROM relayer_tx_submissions
WHERE chain_id = sqlc.arg(chain_id) AND relayer_address = sqlc.arg(relayer_address)
AND nonce >= sqlc.arg(mined_nonce)
AND status IN ('CONFIRMED', 'REVERTED', 'REPLACED', 'CONSUMED')
AND resolved_block_number IS NOT NULL AND resolved_block_hash IS NOT NULL
ORDER BY nonce;

-- name: ReopenTxAttempts :exec
UPDATE relayer_tx_submissions
SET status = 'PENDING', resolved_at = NULL, last_broadcast_at = NULL,
resolved_block_number = NULL, resolved_block_hash = NULL
WHERE chain_id = sqlc.arg(chain_id) AND relayer_address = sqlc.arg(relayer_address)
AND nonce = sqlc.arg(nonce) AND status IN ('CONFIRMED', 'REVERTED', 'REPLACED', 'CONSUMED')
AND resolved_block_number = sqlc.arg(resolved_block_number)
AND resolved_block_hash = sqlc.arg(resolved_block_hash);

-- name: GetTxAttempt :one
SELECT * FROM relayer_tx_submissions
WHERE chain_id = sqlc.arg(chain_id) AND tx_hash = sqlc.arg(tx_hash);

-- name: HasPendingPacketSubmission :one
SELECT EXISTS (
SELECT 1 FROM relayer_tx_submissions a
JOIN packet_tx_submissions s ON a.id = s.submission_id
JOIN packets p ON p.id = s.packet_id
WHERE p.source_chain_id = sqlc.arg(source_chain_id)
AND p.packet_source_client_id = sqlc.arg(source_client_id)
AND p.packet_sequence_number = sqlc.arg(sequence)
AND a.tx_type = sqlc.arg(tx_type) AND a.status = 'PENDING'
);

-- name: ResolveTxNonce :exec
UPDATE relayer_tx_submissions
SET status = CASE
    WHEN sqlc.arg(status) = 'CONSUMED' OR tx_hash = sqlc.arg(tx_hash) THEN sqlc.arg(status)
    ELSE 'REPLACED'
END, resolved_at = sqlc.arg(resolved_at),
resolved_block_number = sqlc.arg(resolved_block_number), resolved_block_hash = sqlc.arg(resolved_block_hash)
WHERE chain_id = sqlc.arg(chain_id) AND relayer_address = sqlc.arg(relayer_address)
AND nonce = sqlc.arg(nonce) AND status = 'PENDING';

-- name: SetTxBroadcast :exec
UPDATE relayer_tx_submissions
SET last_broadcast_at = sqlc.arg(last_broadcast_at)
WHERE chain_id = sqlc.arg(chain_id) AND tx_hash = sqlc.arg(tx_hash)
AND status = 'PENDING';
