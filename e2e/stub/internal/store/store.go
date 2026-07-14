// Package store is sqlite persistence shared by test app deployment, the relay daemon, and status API.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"

	_ "modernc.org/sqlite" // registers the cgo-free "sqlite" database/sql driver
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS test_app_deployments (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    data       TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE TABLE IF NOT EXISTS packets (
    packet_id      TEXT PRIMARY KEY,
    route_id       TEXT NOT NULL,
    app_type       TEXT NOT NULL,
    sequence       INTEGER NOT NULL,
    state          TEXT NOT NULL,
    source_tx_hash TEXT NOT NULL COLLATE NOCASE DEFAULT '',
    recv_tx_hash   TEXT NOT NULL DEFAULT '',
    reason         TEXT NOT NULL DEFAULT '',
    receiver       TEXT NOT NULL DEFAULT '',
    amount         TEXT NOT NULL DEFAULT '',
    target         TEXT NOT NULL DEFAULT '',
    payload        TEXT NOT NULL DEFAULT '',
    timeout_ts     TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at     INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS packets_source_tx_hash_idx ON packets(source_tx_hash COLLATE NOCASE);
CREATE TABLE IF NOT EXISTS relay_requests (
    source_chain_id TEXT NOT NULL,
    source_tx_hash  TEXT NOT NULL,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(source_chain_id, source_tx_hash)
);`

var ErrNoTestApps = errors.New("no test app deployment found; run `ibc test-apps deploy` first")

type Store struct {
	db *sql.DB
}

type Packet struct {
	PacketID     string
	RouteID      string
	AppType      wire.AppType
	Sequence     uint64
	State        wire.PacketState
	SourceTxHash string
	EffectTxHash string
	Reason       string

	Receiver         string
	Amount           string
	Target           string
	Payload          string
	TimeoutTimestamp string
}

type RelayRequestKey struct {
	SourceChainID string
	SourceTxHash  string
}

// Terminal rows are immutable: Mark* is a no-op once complete/timed_out/error_ack.
var notTerminalClause = fmt.Sprintf(
	"state NOT IN ('%s', '%s', '%s')",
	wire.PacketComplete, wire.PacketTimedOut, wire.PacketErrorAck,
)

func Open(path string) (*Store, error) {
	if err := wire.ValidateDB(wire.DB{Type: wire.DBTypeSQLite, URL: path}); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create db dir for %q: %w", path, err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite %q: %w", path, err)
	}
	st := &Store{db: db}
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

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) ensureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: ensure schema: %w", err)
	}
	return nil
}

func (s *Store) SaveTestApps(ctx context.Context, deployment wire.TestAppDeployment) error {
	data, err := json.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("store: encode test app deployment: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO test_app_deployments(id, data) VALUES(1, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, created_at = unixepoch()`, string(data))
	if err != nil {
		return fmt.Errorf("store: save test app deployment: %w", err)
	}
	return nil
}

func (s *Store) LoadTestApps(ctx context.Context) (deployment wire.TestAppDeployment, found bool, err error) {
	var data string
	err = s.db.QueryRowContext(ctx, `SELECT data FROM test_app_deployments WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return wire.TestAppDeployment{}, false, nil
	}
	if err != nil {
		return wire.TestAppDeployment{}, false, fmt.Errorf("store: load test app deployment: %w", err)
	}
	if err := json.Unmarshal([]byte(data), &deployment); err != nil {
		return wire.TestAppDeployment{}, false, fmt.Errorf("store: decode test app deployment: %w", err)
	}
	return deployment, true, nil
}

func (s *Store) RequireTestApps(ctx context.Context) (wire.TestAppDeployment, error) {
	deployment, found, err := s.LoadTestApps(ctx)
	if err != nil {
		return wire.TestAppDeployment{}, err
	}
	if !found {
		return wire.TestAppDeployment{}, ErrNoTestApps
	}
	return deployment, nil
}

// ON CONFLICT DO NOTHING: rediscovered packets converge; terminal rows never regress to pending.
func (s *Store) InsertPending(ctx context.Context, p Packet) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO packets(
			packet_id, route_id, app_type, sequence, state, source_tx_hash,
			receiver, amount, target, payload, timeout_ts
		)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(packet_id) DO NOTHING`,
		p.PacketID, p.RouteID, string(p.AppType), p.Sequence, string(wire.PacketPending), p.SourceTxHash,
		p.Receiver, p.Amount, p.Target, p.Payload, p.TimeoutTimestamp)
	if err != nil {
		return fmt.Errorf("store: insert pending packet %s: %w", p.PacketID, err)
	}
	return nil
}

func (s *Store) RequestRelay(ctx context.Context, sourceChainID, sourceTxHash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO relay_requests(source_chain_id, source_tx_hash)
		VALUES(?, ?)
		ON CONFLICT(source_chain_id, source_tx_hash) DO NOTHING`, sourceChainID, sourceTxHash)
	if err != nil {
		return fmt.Errorf("store: request relay for %s/%s: %w", sourceChainID, sourceTxHash, err)
	}
	return nil
}

func (s *Store) RelayRequests(ctx context.Context) (map[RelayRequestKey]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_chain_id, source_tx_hash FROM relay_requests`)
	if err != nil {
		return nil, fmt.Errorf("store: query relay requests: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := map[RelayRequestKey]bool{}
	for rows.Next() {
		var k RelayRequestKey
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

func (s *Store) MarkComplete(ctx context.Context, packetID, effectTxHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE packets SET state = ?, recv_tx_hash = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(wire.PacketComplete), effectTxHash, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s complete: %w", packetID, err)
	}
	return nil
}

func (s *Store) MarkTimedOut(ctx context.Context, packetID, refundTxHash, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE packets SET state = ?, recv_tx_hash = ?, reason = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(wire.PacketTimedOut), refundTxHash, reason, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s timed_out: %w", packetID, err)
	}
	return nil
}

func (s *Store) MarkErrorAck(ctx context.Context, packetID, effectTxHash, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE packets SET state = ?, recv_tx_hash = ?, reason = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(wire.PacketErrorAck), effectTxHash, reason, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s error_ack: %w", packetID, err)
	}
	return nil
}

func (s *Store) Packets(ctx context.Context, packetFilter string) ([]Packet, error) {
	return s.query(ctx, packetFilter)
}

func (s *Store) PacketsBySourceTx(ctx context.Context, sourceTxHash string) ([]Packet, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT packet_id, route_id, app_type, sequence, state, source_tx_hash, recv_tx_hash, reason,
		       receiver, amount, target, payload, timeout_ts
		FROM packets
		WHERE source_tx_hash = ? COLLATE NOCASE
		ORDER BY created_at, sequence`, sourceTxHash)
	if err != nil {
		return nil, fmt.Errorf("store: query packets by source tx: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func (s *Store) PendingPackets(ctx context.Context) ([]Packet, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT packet_id, route_id, app_type, sequence, state, source_tx_hash, recv_tx_hash, reason,
		       receiver, amount, target, payload, timeout_ts
		FROM packets
		WHERE state = ?
		ORDER BY created_at, sequence`, string(wire.PacketPending))
	if err != nil {
		return nil, fmt.Errorf("store: query pending packets: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func (s *Store) query(ctx context.Context, packetFilter string) ([]Packet, error) {
	q := `SELECT packet_id, route_id, app_type, sequence, state, source_tx_hash, recv_tx_hash, reason,
	             receiver, amount, target, payload, timeout_ts FROM packets`
	var args []any
	if packetFilter != "" {
		q += " WHERE packet_id = ?"
		args = append(args, packetFilter)
	}
	q += " ORDER BY created_at, sequence"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query packets: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func scanPackets(rows *sql.Rows) ([]Packet, error) {
	var out []Packet
	for rows.Next() {
		var (
			p     Packet
			state string
			app   string
		)
		if err := rows.Scan(
			&p.PacketID, &p.RouteID, &app, &p.Sequence, &state, &p.SourceTxHash, &p.EffectTxHash, &p.Reason,
			&p.Receiver, &p.Amount, &p.Target, &p.Payload, &p.TimeoutTimestamp,
		); err != nil {
			return nil, fmt.Errorf("store: scan packet: %w", err)
		}
		p.AppType = wire.AppType(app)
		p.State = wire.PacketState(state)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate packets: %w", err)
	}
	return out, nil
}
