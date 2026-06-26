-- name: GetRelaySubmission :one
SELECT * FROM relay_submissions
WHERE source_chain_id = sqlc.arg(chain_id)
AND source_tx_hash = sqlc.arg(tx_hash);

-- name: InsertRelaySubmission :exec
INSERT INTO relay_submissions (source_chain_id, source_tx_hash)
VALUES (sqlc.arg(chain_id), sqlc.arg(tx_hash))
ON CONFLICT (source_chain_id, source_tx_hash) DO NOTHING;
