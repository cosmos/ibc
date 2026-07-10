// Package store is the stub's SQLite persistence shared by `deploy`, the relay daemon, and the
// status API. It owns the schema and exposes typed read/write helpers over these tables:
//
//   - deployments: the single persisted wire.Deployment (per-chain fixture addresses + routes),
//     so the daemon can resolve a route's MockIFT address without re-running deploy.
//   - packets: one row per relayed packet (id, route, sequence, state, and the source/effect tx
//     hashes), the lifecycle ledger the status API serves and a restart resumes pending work from.
//   - relay_requests: idempotent manual-relay submissions keyed by source chain + source tx hash, so a
//     restarted daemon resumes requested-but-undelivered manual packets like any other pending work.
//
// Several short-lived processes plus the long-lived daemon open the same DB at once, so the SQLite path
// enables WAL + a busy timeout to make cross-process sharing safe.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"

	_ "modernc.org/sqlite" // registers the cgo-free "sqlite" database/sql driver
)

// schemaSQL creates the current POC schema. Open applies it idempotently so every stub command can use
// the store without depending on a prior stub-side migrate command.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS deployments (
    id         INTEGER PRIMARY KEY CHECK (id = 1), -- single-row table: the current deployment
    data       TEXT NOT NULL,                      -- wire.Deployment encoded as JSON
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

// ErrNoDeployment is returned by RequireDeployment when `deploy` has not persisted a deployment yet, so
// every caller surfaces the same "run deploy first" guidance instead of re-spelling it.
var ErrNoDeployment = errors.New("no deployment found; run `ibc deploy` first")

// Store is a handle to the relayer DB.
type Store struct {
	db *sql.DB
}

// Packet is one row of the packets ledger. State is the typed wire enum so callers compare against
// wire.PacketComplete etc. rather than bare strings.
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

// RelayRequestKey is the persisted identity of one manual relay request.
type RelayRequestKey struct {
	SourceChainID string
	SourceTxHash  string
}

// The Mark* writers below append this WHERE fragment so terminal immutability is enforced by the store
// itself, not by relay-loop ordering: once a packet reaches ANY terminal state, a late/out-of-order tick
// or a repeat call cannot regress its state or rewrite its recorded effect hash — the repeat is a 0-row
// no-op. The state values are compile-time constants (not user input), so they are inlined into the
// fragment rather than bound.
var notTerminalClause = fmt.Sprintf(
	"state NOT IN ('%s', '%s', '%s')",
	wire.PacketComplete, wire.PacketTimedOut, wire.PacketErrorAck,
)

// Open opens the cgo-free sqlite file DB and idempotently creates the stub schema. It creates the
// parent directory and enables a busy timeout (so a concurrent writer waits rather than failing with
// "database is locked") and WAL (so the status API can read while the relayer writes).
func Open(path string) (*Store, error) {
	if err := wire.ValidateDB(wire.DB{Type: wire.DBTypeSQLite, URL: path}); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	// sql.Open creates the DB file but not its parent dir; make the parent so a path under a fresh run
	// dir opens cleanly.
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

// sqliteDSN builds the modernc DSN. The pragmas matter because the DB is shared by multiple processes
// (the daemon writes discovered + delivered rows, the status API/CLI read): busy_timeout makes a contended
// write retry for up to 5s instead of erroring, and WAL lets readers run concurrently with the writer.
func sqliteDSN(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// Close closes the underlying DB handle.
func (s *Store) Close() error { return s.db.Close() }

// ensureSchema creates the current schema if absent.
func (s *Store) ensureSchema(ctx context.Context) error {
	for _, stmt := range splitStatements(schemaSQL) {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store: ensure schema: %w", err)
		}
	}
	return nil
}

// splitStatements breaks a `;`-separated DDL script into individual statements, dropping blank ones.
// The schema's CREATE TABLE bodies contain no inner semicolons (the CHECK/DEFAULT expressions are
// semicolon-free), so a plain split is exact here.
func splitStatements(schema string) []string {
	parts := strings.Split(schema, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// SaveDeployment upserts the single deployment row with dep encoded as JSON. `deploy` calls it so the
// daemon can later resolve fixture addresses without re-deploying.
func (s *Store) SaveDeployment(ctx context.Context, dep wire.Deployment) error {
	data, err := json.Marshal(dep)
	if err != nil {
		return fmt.Errorf("store: encode deployment: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO deployments(id, data) VALUES(1, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, created_at = unixepoch()`, string(data))
	if err != nil {
		return fmt.Errorf("store: save deployment: %w", err)
	}
	return nil
}

// LoadDeployment returns the persisted deployment. found is false (with a nil error) when `deploy` has
// not run yet, so callers can give a precise "run deploy first" message instead of a decode error.
func (s *Store) LoadDeployment(ctx context.Context) (dep wire.Deployment, found bool, err error) {
	var data string
	err = s.db.QueryRowContext(ctx, `SELECT data FROM deployments WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return wire.Deployment{}, false, nil
	}
	if err != nil {
		return wire.Deployment{}, false, fmt.Errorf("store: load deployment: %w", err)
	}
	if err := json.Unmarshal([]byte(data), &dep); err != nil {
		return wire.Deployment{}, false, fmt.Errorf("store: decode deployment: %w", err)
	}
	return dep, true, nil
}

// RequireDeployment loads the single persisted deployment, returning ErrNoDeployment when `deploy` has
// not run. It is the shared "open store already done, now get the deployment or fail with run-deploy-
// first" step the relay daemon needs before it can resolve fixtures and reach readiness.
func (s *Store) RequireDeployment(ctx context.Context) (wire.Deployment, error) {
	dep, found, err := s.LoadDeployment(ctx)
	if err != nil {
		return wire.Deployment{}, err
	}
	if !found {
		return wire.Deployment{}, ErrNoDeployment
	}
	return dep, nil
}

// InsertPending records a packet in the pending state if it does not already exist. It is the discovery
// idempotency anchor: the relay loop's source scan re-derives the same deterministic packet id every tick,
// so a rediscovered packet converges on one row (`ON CONFLICT DO NOTHING`), and a re-insert never regresses
// a packet already marked terminal back to pending. Rows are authored here, by discovery from chain
// state
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

// RequestRelay records a manual relay request. It is idempotent so callers may safely retry a request
// for an already-known source tx.
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

// RelayRequests returns the current manual relay request set.
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

// MarkComplete moves a packet to the complete state and records its terminal effect tx hash. The
// terminal guard makes a repeat call a no-op (the first recorded effect hash stands) and keeps a
// late/out-of-order tick from flipping a refunded or error-acked packet to complete.
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

// MarkTimedOut moves a packet to the timed_out terminal state with a human reason, recording the refund tx
// in recv_tx_hash (the timeout's on-chain effect). The terminal guard keeps it from regressing a delivered
// or otherwise terminal packet.
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

// MarkErrorAck moves a packet to the error_ack terminal state with a human reason (the message was delivered
// on the destination but the inner target call reverted). It records the deliver tx as the terminal effect —
// the delivery did happen on-chain; the inner call is what failed. The terminal guard prevents regression.
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

// Packets returns the ledger, optionally filtered to a single packet id, ordered by insertion then
// sequence. An empty filter means "all". This backs the status API.
func (s *Store) Packets(ctx context.Context, packetFilter string) ([]Packet, error) {
	return s.query(ctx, packetFilter)
}

// PacketsBySourceTx returns packets discovered from sourceTxHash. Hash comparison is case-insensitive
// so hash formatting differences do not affect lookup.
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

// PendingPackets returns all non-terminal work the relayer should reconcile.
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
