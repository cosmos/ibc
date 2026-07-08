-- name: GetRelayRequest :one
SELECT * FROM ibcv2_relay_requests
WHERE source_chain_id = sqlc.arg(chain_id)
AND source_tx_hash = sqlc.arg(tx_hash);

-- name: CreateRelayRequest :exec
INSERT INTO ibcv2_relay_requests (source_chain_id, source_tx_hash)
VALUES (sqlc.arg(chain_id), sqlc.arg(tx_hash))
ON CONFLICT (source_chain_id, source_tx_hash) DO NOTHING;

-- name: InsertTransfer :execrows
INSERT INTO ibcv2_transfers (
    source_chain_id,
    destination_chain_id,
    source_tx_hash,
    source_tx_time,
    packet_sequence_number,
    packet_source_client_id,
    packet_destination_client_id,
    packet_timeout_timestamp
) VALUES (
    sqlc.arg(source_chain_id),
    sqlc.arg(destination_chain_id),
    sqlc.arg(source_tx_hash),
    sqlc.arg(source_tx_time),
    sqlc.arg(packet_sequence_number),
    sqlc.arg(packet_source_client_id),
    sqlc.arg(packet_destination_client_id),
    sqlc.arg(packet_timeout_timestamp)
)
ON CONFLICT (source_chain_id, packet_sequence_number, packet_source_client_id) DO NOTHING;

-- name: ListTransfersBySourceTx :many
SELECT * FROM ibcv2_transfers
WHERE source_chain_id = sqlc.arg(chain_id)
AND source_tx_hash = sqlc.arg(tx_hash)
ORDER BY packet_sequence_number;
