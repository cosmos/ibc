package ibcrelay

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"

	_ "modernc.org/sqlite" // registers the cgo-free "sqlite" database/sql driver
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS relay_packets (
    packet_id      TEXT PRIMARY KEY,
    route_id       TEXT NOT NULL,
    packet_json    TEXT NOT NULL,
    state          TEXT NOT NULL,
    source_tx_hash TEXT NOT NULL COLLATE NOCASE DEFAULT '',
    recv_tx_hash   TEXT NOT NULL DEFAULT '',
    ack_tx_hash    TEXT NOT NULL DEFAULT '',
    reason         TEXT NOT NULL DEFAULT '',
    ack_hex        TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at     INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS relay_packets_source_tx_hash_idx ON relay_packets(source_tx_hash COLLATE NOCASE);
CREATE TABLE IF NOT EXISTS relay_requests (
    source_chain_id TEXT NOT NULL,
    source_tx_hash  TEXT NOT NULL,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(source_chain_id, source_tx_hash)
);`

// Terminal rows are immutable: Mark* is a no-op once complete/timed_out/error_ack.
var notTerminalClause = fmt.Sprintf(
	"state NOT IN ('%s', '%s', '%s')",
	relayercmd.PacketComplete, relayercmd.PacketTimedOut, relayercmd.PacketErrorAck,
)

type relayStore struct {
	db *sql.DB
}

type storedPacket struct {
	PacketID     string
	RouteID      string
	Packet       ics26router.IICS26RouterMsgsPacket
	State        relayercmd.PacketState
	SourceTxHash string
	RecvTxHash   string
	AckTxHash    string
	Reason       string
	AckHex       string
}

type relayRequestKey struct {
	SourceChainID string
	SourceTxHash  string
}

func openStore(path string) (*relayStore, error) {
	if err := configcmd.ValidateDB(configcmd.DB{Type: configcmd.DBTypeSQLite, URL: path}); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create db dir for %q: %w", path, err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite %q: %w", path, err)
	}
	st := &relayStore{db: db}
	if err := st.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

// WAL + busy_timeout: daemon writes while status API/CLI read across processes.
func sqliteDSN(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

func (s *relayStore) Close() error { return s.db.Close() }

func (s *relayStore) ensureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: ensure schema: %w", err)
	}
	return nil
}

// ON CONFLICT DO NOTHING: rediscovered packets converge; terminal rows never regress to pending.
func (s *relayStore) InsertPending(ctx context.Context, p storedPacket) error {
	packetJSON, err := json.Marshal(p.Packet)
	if err != nil {
		return fmt.Errorf("store: encode packet %s: %w", p.PacketID, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO relay_packets(
			packet_id, route_id, packet_json, state, source_tx_hash
		)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(packet_id) DO NOTHING`,
		p.PacketID, p.RouteID, string(packetJSON), string(relayercmd.PacketPending), p.SourceTxHash)
	if err != nil {
		return fmt.Errorf("store: insert pending packet %s: %w", p.PacketID, err)
	}
	return nil
}

func (s *relayStore) RequestRelay(ctx context.Context, sourceChainID, sourceTxHash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO relay_requests(source_chain_id, source_tx_hash)
		VALUES(?, ?)
		ON CONFLICT(source_chain_id, source_tx_hash) DO NOTHING`, sourceChainID, sourceTxHash)
	if err != nil {
		return fmt.Errorf("store: request relay for %s/%s: %w", sourceChainID, sourceTxHash, err)
	}
	return nil
}

func (s *relayStore) RelayRequests(ctx context.Context) (map[relayRequestKey]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_chain_id, source_tx_hash FROM relay_requests`)
	if err != nil {
		return nil, fmt.Errorf("store: query relay requests: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := map[relayRequestKey]bool{}
	for rows.Next() {
		var k relayRequestKey
		if err := rows.Scan(&k.SourceChainID, &k.SourceTxHash); err != nil {
			return nil, fmt.Errorf("store: scan relay request: %w", err)
		}
		out[k] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate relay requests: %w", err)
	}
	return out, nil
}

// MarkReceived records the destination recv/ack capture while leaving state pending.
func (s *relayStore) MarkReceived(ctx context.Context, packetID, recvTx, ackHex string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE relay_packets SET recv_tx_hash = ?, ack_hex = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		recvTx, ackHex, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s received: %w", packetID, err)
	}
	return nil
}

func (s *relayStore) MarkComplete(ctx context.Context, packetID, ackTx string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE relay_packets SET state = ?, ack_tx_hash = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(relayercmd.PacketComplete), ackTx, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s complete: %w", packetID, err)
	}
	return nil
}

func (s *relayStore) MarkTimedOut(ctx context.Context, packetID, timeoutTx, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE relay_packets SET state = ?, ack_tx_hash = ?, reason = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(relayercmd.PacketTimedOut), timeoutTx, reason, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s timed_out: %w", packetID, err)
	}
	return nil
}

func (s *relayStore) MarkErrorAck(ctx context.Context, packetID, ackTx, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE relay_packets SET state = ?, ack_tx_hash = ?, reason = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(relayercmd.PacketErrorAck), ackTx, reason, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s error_ack: %w", packetID, err)
	}
	return nil
}

func (s *relayStore) Packets(ctx context.Context, packetFilter string) ([]storedPacket, error) {
	return s.query(ctx, packetFilter)
}

func (s *relayStore) PacketsBySourceTx(ctx context.Context, sourceTxHash string) ([]storedPacket, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT packet_id, route_id, packet_json, state, source_tx_hash,
		       recv_tx_hash, ack_tx_hash, reason, ack_hex
		FROM relay_packets
		WHERE source_tx_hash = ? COLLATE NOCASE
		ORDER BY created_at, packet_id`, sourceTxHash)
	if err != nil {
		return nil, fmt.Errorf("store: query packets by source tx: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func (s *relayStore) PendingPackets(ctx context.Context) ([]storedPacket, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT packet_id, route_id, packet_json, state, source_tx_hash,
		       recv_tx_hash, ack_tx_hash, reason, ack_hex
		FROM relay_packets
		WHERE state = ?
		ORDER BY created_at, packet_id`, string(relayercmd.PacketPending))
	if err != nil {
		return nil, fmt.Errorf("store: query pending packets: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func (s *relayStore) query(ctx context.Context, packetFilter string) ([]storedPacket, error) {
	q := `SELECT packet_id, route_id, packet_json, state, source_tx_hash,
	             recv_tx_hash, ack_tx_hash, reason, ack_hex
	      FROM relay_packets`
	var args []any
	if packetFilter != "" {
		q += " WHERE packet_id = ?"
		args = append(args, packetFilter)
	}
	q += " ORDER BY created_at, packet_id"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query packets: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func scanPackets(rows *sql.Rows) ([]storedPacket, error) {
	var out []storedPacket
	for rows.Next() {
		var (
			p          storedPacket
			packetJSON string
			state      string
		)
		if err := rows.Scan(
			&p.PacketID, &p.RouteID, &packetJSON, &state, &p.SourceTxHash,
			&p.RecvTxHash, &p.AckTxHash, &p.Reason, &p.AckHex,
		); err != nil {
			return nil, fmt.Errorf("store: scan packet: %w", err)
		}
		if err := json.Unmarshal([]byte(packetJSON), &p.Packet); err != nil {
			return nil, fmt.Errorf("store: decode packet %s: %w", p.PacketID, err)
		}
		p.State = relayercmd.PacketState(state)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate packets: %w", err)
	}
	return out, nil
}

func (p storedPacket) effectTxHash() string {
	if p.State == relayercmd.PacketPending {
		return p.RecvTxHash
	}
	return p.AckTxHash
}
