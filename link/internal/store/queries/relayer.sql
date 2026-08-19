/*
 * SPDX-License-Identifier: Apache-2.0
 */

-- name: GetRelayRequest :one
SELECT * FROM relay_requests
WHERE source_chain_id = sqlc.arg(chain_id)
AND source_tx_hash = sqlc.arg(tx_hash);

-- name: CreateRelayRequest :exec
INSERT INTO relay_requests (source_chain_id, source_tx_hash)
VALUES (sqlc.arg(chain_id), sqlc.arg(tx_hash))
ON CONFLICT (source_chain_id, source_tx_hash) DO NOTHING;

-- name: UpsertPacket :exec
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
ON CONFLICT (source_chain_id, packet_sequence_number, packet_source_client_id) DO UPDATE SET
    status = EXCLUDED.status,
    destination_chain_id = EXCLUDED.destination_chain_id,
    source_tx_hash = EXCLUDED.source_tx_hash,
    source_tx_time = EXCLUDED.source_tx_time,
    packet_destination_client_id = EXCLUDED.packet_destination_client_id,
    packet_timeout_timestamp = EXCLUDED.packet_timeout_timestamp,
    updated_at = CURRENT_TIMESTAMP
WHERE packets.status = 'NOT_SELECTED'
AND EXCLUDED.status IN ('NOT_SELECTED', 'PENDING');

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

-- name: ListDispatchablePackets :many
SELECT * FROM packets
WHERE status NOT IN (
    'NOT_SELECTED',
    'COMPLETE_WITH_ACK',
    'COMPLETE_WITH_TIMEOUT',
    'COMPLETE_WITH_WRITE_ACK_ERROR',
    'FAILED'
)
ORDER BY id;

-- Shared filter shape for ListPackets and CountPackets.
--
-- Optional filters use COALESCE(param, column) rather than
-- (param IS NULL OR column = param): the latter names each parameter twice,
-- which makes sqlc emit explicitly numbered placeholders, and those collide
-- with the placeholders sqlc.slice injects for statuses. Every filter column
-- is NOT NULL, so COALESCE degrades to a self-comparison when the filter is
-- absent.
--
-- The status set is passed as one comma-delimited string rather than
-- sqlc.slice: slice expansion injects unnumbered placeholders, which collide
-- with the numbered placeholders sqlc emits for the other parameters. Relay
-- statuses are fixed identifiers containing no commas, so delimiting is safe.
-- It does forgo an index on status; see the packets index migration.
--
-- The status set is always applied, so callers pass every known status when
-- they want no status filter.
--
-- Callers bind row_limit as one more than the page they want. The extra row is
-- a probe: if it comes back there is another page, and it is dropped before
-- returning. That keeps the query O(page) -- the planner stops once it has
-- enough rows -- where reporting an exact total would force it to visit every
-- matching row on every request.

-- name: ListPackets :many
SELECT * FROM packets
WHERE ',' || sqlc.arg(statuses) || ',' LIKE '%,' || status || ',%'
AND source_chain_id = COALESCE(sqlc.narg(source_chain_id), source_chain_id)
AND destination_chain_id = COALESCE(sqlc.narg(destination_chain_id), destination_chain_id)
AND packet_source_client_id = COALESCE(sqlc.narg(source_client_id), packet_source_client_id)
AND packet_destination_client_id = COALESCE(sqlc.narg(destination_client_id), packet_destination_client_id)
AND source_tx_hash = COALESCE(sqlc.narg(source_tx_hash), source_tx_hash)
AND packet_sequence_number = COALESCE(sqlc.narg(sequence_number), packet_sequence_number)
AND created_at >= COALESCE(sqlc.narg(created_from), created_at)
AND created_at <= COALESCE(sqlc.narg(created_to), created_at)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
