package stub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
	"github.com/cosmos/ibc/link/cmd/testappcmd"

	_ "modernc.org/sqlite" // registers the cgo-free "sqlite" database/sql driver
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS stub_test_app_deployments (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    data       TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE TABLE IF NOT EXISTS stub_packets (
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
CREATE INDEX IF NOT EXISTS stub_packets_source_tx_hash_idx ON stub_packets(source_tx_hash COLLATE NOCASE);
CREATE TABLE IF NOT EXISTS stub_relay_requests (
    source_chain_id TEXT NOT NULL,
    source_tx_hash  TEXT NOT NULL,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(source_chain_id, source_tx_hash)
);`

var errNoTestApps = errors.New("no test app deployment found; run `ibc test-apps deploy` first")

type stubStore struct {
	db *sql.DB
}

type storedPacket struct {
	PacketID     string
	RouteID      string
	AppType      relayercmd.AppType
	Sequence     uint64
	State        relayercmd.PacketState
	SourceTxHash string
	EffectTxHash string
	Reason       string

	Receiver         string
	Amount           string
	Target           string
	Payload          string
	TimeoutTimestamp string
}

type relayRequestKey struct {
	SourceChainID string
	SourceTxHash  string
}

// Terminal rows are immutable: Mark* is a no-op once complete/timed_out/error_ack.
var notTerminalClause = fmt.Sprintf(
	"state NOT IN ('%s', '%s', '%s')",
	relayercmd.PacketComplete, relayercmd.PacketTimedOut, relayercmd.PacketErrorAck,
)

func openStore(path string) (*stubStore, error) {
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
	st := &stubStore{db: db}
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

func (s *stubStore) Close() error { return s.db.Close() }

func (s *stubStore) ensureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: ensure schema: %w", err)
	}
	return nil
}

func (s *stubStore) SaveTestApps(ctx context.Context, deployment testappcmd.Deployment) error {
	data, err := json.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("store: encode test app deployment: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO stub_test_app_deployments(id, data) VALUES(1, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, created_at = unixepoch()`, string(data))
	if err != nil {
		return fmt.Errorf("store: save test app deployment: %w", err)
	}
	return nil
}

func (s *stubStore) LoadTestApps(ctx context.Context) (deployment testappcmd.Deployment, found bool, err error) {
	var data string
	err = s.db.QueryRowContext(ctx, `SELECT data FROM stub_test_app_deployments WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return testappcmd.Deployment{}, false, nil
	}
	if err != nil {
		return testappcmd.Deployment{}, false, fmt.Errorf("store: load test app deployment: %w", err)
	}
	if err := json.Unmarshal([]byte(data), &deployment); err != nil {
		return testappcmd.Deployment{}, false, fmt.Errorf("store: decode test app deployment: %w", err)
	}
	return deployment, true, nil
}

func (s *stubStore) RequireTestApps(ctx context.Context) (testappcmd.Deployment, error) {
	deployment, found, err := s.LoadTestApps(ctx)
	if err != nil {
		return testappcmd.Deployment{}, err
	}
	if !found {
		return testappcmd.Deployment{}, errNoTestApps
	}
	return deployment, nil
}

// ON CONFLICT DO NOTHING: rediscovered packets converge; terminal rows never regress to pending.
func (s *stubStore) InsertPending(ctx context.Context, p storedPacket) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stub_packets(
			packet_id, route_id, app_type, sequence, state, source_tx_hash,
			receiver, amount, target, payload, timeout_ts
		)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(packet_id) DO NOTHING`,
		p.PacketID, p.RouteID, string(p.AppType), p.Sequence, string(relayercmd.PacketPending), p.SourceTxHash,
		p.Receiver, p.Amount, p.Target, p.Payload, p.TimeoutTimestamp)
	if err != nil {
		return fmt.Errorf("store: insert pending packet %s: %w", p.PacketID, err)
	}
	return nil
}

func (s *stubStore) RequestRelay(ctx context.Context, sourceChainID, sourceTxHash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stub_relay_requests(source_chain_id, source_tx_hash)
		VALUES(?, ?)
		ON CONFLICT(source_chain_id, source_tx_hash) DO NOTHING`, sourceChainID, sourceTxHash)
	if err != nil {
		return fmt.Errorf("store: request relay for %s/%s: %w", sourceChainID, sourceTxHash, err)
	}
	return nil
}

func (s *stubStore) RelayRequests(ctx context.Context) (map[relayRequestKey]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_chain_id, source_tx_hash FROM stub_relay_requests`)
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

func (s *stubStore) MarkComplete(ctx context.Context, packetID, effectTxHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE stub_packets SET state = ?, recv_tx_hash = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(relayercmd.PacketComplete), effectTxHash, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s complete: %w", packetID, err)
	}
	return nil
}

func (s *stubStore) MarkTimedOut(ctx context.Context, packetID, refundTxHash, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE stub_packets SET state = ?, recv_tx_hash = ?, reason = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(relayercmd.PacketTimedOut), refundTxHash, reason, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s timed_out: %w", packetID, err)
	}
	return nil
}

func (s *stubStore) MarkErrorAck(ctx context.Context, packetID, effectTxHash, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE stub_packets SET state = ?, recv_tx_hash = ?, reason = ?, updated_at = unixepoch()
		WHERE packet_id = ? AND `+notTerminalClause,
		string(relayercmd.PacketErrorAck), effectTxHash, reason, packetID)
	if err != nil {
		return fmt.Errorf("store: mark packet %s error_ack: %w", packetID, err)
	}
	return nil
}

func (s *stubStore) Packets(ctx context.Context, packetFilter string) ([]storedPacket, error) {
	return s.query(ctx, packetFilter)
}

func (s *stubStore) PacketsBySourceTx(ctx context.Context, sourceTxHash string) ([]storedPacket, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT packet_id, route_id, app_type, sequence, state, source_tx_hash, recv_tx_hash, reason,
		       receiver, amount, target, payload, timeout_ts
		FROM stub_packets
		WHERE source_tx_hash = ? COLLATE NOCASE
		ORDER BY created_at, sequence`, sourceTxHash)
	if err != nil {
		return nil, fmt.Errorf("store: query packets by source tx: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func (s *stubStore) PendingPackets(ctx context.Context) ([]storedPacket, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT packet_id, route_id, app_type, sequence, state, source_tx_hash, recv_tx_hash, reason,
		       receiver, amount, target, payload, timeout_ts
		FROM stub_packets
		WHERE state = ?
		ORDER BY created_at, sequence`, string(relayercmd.PacketPending))
	if err != nil {
		return nil, fmt.Errorf("store: query pending packets: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanPackets(rows)
}

func (s *stubStore) query(ctx context.Context, packetFilter string) ([]storedPacket, error) {
	q := `SELECT packet_id, route_id, app_type, sequence, state, source_tx_hash, recv_tx_hash, reason,
	             receiver, amount, target, payload, timeout_ts FROM stub_packets`
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

func scanPackets(rows *sql.Rows) ([]storedPacket, error) {
	var out []storedPacket
	for rows.Next() {
		var (
			p     storedPacket
			state string
			app   string
		)
		if err := rows.Scan(
			&p.PacketID, &p.RouteID, &app, &p.Sequence, &state, &p.SourceTxHash, &p.EffectTxHash, &p.Reason,
			&p.Receiver, &p.Amount, &p.Target, &p.Payload, &p.TimeoutTimestamp,
		); err != nil {
			return nil, fmt.Errorf("store: scan packet: %w", err)
		}
		p.AppType = relayercmd.AppType(app)
		p.State = relayercmd.PacketState(state)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate packets: %w", err)
	}
	return out, nil
}
