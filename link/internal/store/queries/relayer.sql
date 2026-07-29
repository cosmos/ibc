-- name: GetRelayRequest :one
SELECT * FROM relay_requests
WHERE source_chain_id = sqlc.arg(chain_id)
AND source_tx_hash = sqlc.arg(tx_hash);

-- name: CreateRelayRequest :exec
INSERT INTO relay_requests (source_chain_id, source_tx_hash)
VALUES (sqlc.arg(chain_id), sqlc.arg(tx_hash))
ON CONFLICT (source_chain_id, source_tx_hash) DO NOTHING;

-- name: CreatePacket :execrows
INSERT INTO packets (
    status,
    source_chain_id,
    destination_chain_id,
    source_tx_hash,
    source_tx_time,
    packet_sequence_number,
    packet_source_client_id,
    packet_destination_client_id,
    packet_timeout_timestamp
) VALUES (
    sqlc.arg(status),
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

-- name: ListPacketsBySourceTx :many
SELECT * FROM packets
WHERE source_chain_id = sqlc.arg(chain_id)
AND source_tx_hash = sqlc.arg(tx_hash)
ORDER BY packet_sequence_number;

-- name: UpdatePacketStatus :exec
UPDATE packets SET
    status = sqlc.arg(status),
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: UpdatePacketRecvTx :exec
UPDATE packets SET
    recv_tx_hash = sqlc.arg(recv_tx_hash),
    recv_tx_time = sqlc.arg(recv_tx_time),
    recv_tx_relayer_address = sqlc.arg(recv_tx_relayer_address),
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: ClearPacketRecvTx :exec
UPDATE packets SET
    recv_tx_hash = NULL,
    recv_tx_time = NULL,
    recv_tx_relayer_address = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: UpdatePacketWriteAck :exec
UPDATE packets SET
    write_ack_tx_hash = sqlc.arg(write_ack_tx_hash),
    write_ack_tx_time = sqlc.arg(write_ack_tx_time),
    write_ack_status = sqlc.arg(write_ack_status),
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: UpdatePacketAckTx :exec
UPDATE packets SET
    ack_tx_hash = sqlc.arg(ack_tx_hash),
    ack_tx_time = sqlc.arg(ack_tx_time),
    ack_tx_relayer_address = sqlc.arg(ack_tx_relayer_address),
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: ClearPacketAckTx :exec
UPDATE packets SET
    ack_tx_hash = NULL,
    ack_tx_time = NULL,
    ack_tx_relayer_address = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: UpdatePacketTimeoutTx :exec
UPDATE packets SET
    timeout_tx_hash = sqlc.arg(timeout_tx_hash),
    timeout_tx_time = sqlc.arg(timeout_tx_time),
    timeout_tx_relayer_address = sqlc.arg(timeout_tx_relayer_address),
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: ClearPacketTimeoutTx :exec
UPDATE packets SET
    timeout_tx_hash = NULL,
    timeout_tx_time = NULL,
    timeout_tx_relayer_address = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE source_chain_id = sqlc.arg(source_chain_id)
AND packet_source_client_id = sqlc.arg(packet_source_client_id)
AND packet_sequence_number = sqlc.arg(packet_sequence_number);

-- name: ListUnfinishedPackets :many
SELECT * FROM packets
WHERE status NOT IN (
    'COMPLETE_WITH_ACK',
    'COMPLETE_WITH_TIMEOUT',
    'COMPLETE_WITH_WRITE_ACK_ERROR',
    'FAILED'
)
ORDER BY id;
