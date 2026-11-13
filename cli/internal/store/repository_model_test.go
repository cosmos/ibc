// SPDX-License-Identifier: Apache-2.0

package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/cosmos/ibc/cli/internal/tests"
)

// TestRepositoryModel applies random sequences of Repository writes, alone
// and in transactions that commit or roll back, to a backend and to an
// in-memory model of the documented semantics, and compares every read
// against the model after each step. Each backend runs against the model, so
// the two dialects agree wherever both agree with it.
//
// Row ids and created/updated timestamps are backend-assigned and differ
// between dialects (a Postgres upsert conflict still consumes a sequence
// value), so they are compared only through the order they impose.
func TestRepositoryModel(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			db, err := NewSqliteInMemory()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			_, err = db.MigrateUp()
			require.NoError(t, err)

			runRepositoryModel(t, db)
		})
	})

	t.Run("postgres", func(t *testing.T) {
		tests.GuardPostgresTests(t)

		pg := tests.NewPostgresContainer(t)
		const dbName = "repository-model"
		pg.CreateDB(dbName)

		db, err := NewPostgres(context.Background(), pg.URL(dbName))
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		rapid.Check(t, func(t *rapid.T) {
			resetMigrations(t, db)
			runRepositoryModel(t, db)
		})
	})
}

// resetMigrations empties a reused database through its migrations.
func resetMigrations(t *rapid.T, m Migrator) {
	for {
		n, err := m.MigrateDown()
		require.NoError(t, err)
		if n == 0 {
			break
		}
	}

	_, err := m.MigrateUp()
	require.NoError(t, err)
}

func runRepositoryModel(t *rapid.T, store Store) {
	t.Repeat(rapid.StateMachineActions(&repositoryModel{
		ctx:   context.Background(),
		store: store,
		state: newRepoState(),
	}))
}

// The domain is kept small so writes keep colliding on the same keys.
var (
	modelChains   = []string{"1", "8453"}
	modelClients  = []string{"client-0", "client-1"}
	modelTxHashes = []string{"0xaa", "0xbb", "0xcc"}
	modelRelayers = []string{"0x01", "0x02"}
)

// drawTime draws a UTC time at microsecond precision, the precision a
// Postgres timestamptz keeps: the model compares semantics, not precision.
func drawTime(t *rapid.T, label string) time.Time {
	sec := rapid.Int64Range(1_700_000_000, 1_700_100_000).Draw(t, label+".sec")
	usec := rapid.Int64Range(0, 999_999).Draw(t, label+".usec")

	return time.Unix(sec, usec*1000).UTC()
}

func drawKey(t *rapid.T) PacketKey {
	return PacketKey{
		SourceChainID:  rapid.SampledFrom(modelChains).Draw(t, "chain"),
		SourceClientID: rapid.SampledFrom(modelClients).Draw(t, "client"),
		Sequence:       rapid.Uint64Range(1, 4).Draw(t, "sequence"),
	}
}

func drawPacketTx(t *rapid.T) PacketTx {
	return PacketTx{
		Hash:           rapid.SampledFrom(modelTxHashes).Draw(t, "tx.hash"),
		Time:           drawTime(t, "tx.time"),
		RelayerAddress: rapid.SampledFrom(modelRelayers).Draw(t, "tx.relayer"),
	}
}

// repoState the model: what every backend must return.
type repoState struct {
	// rows in insertion order, which is id order on every backend
	rows       []Packet
	watermarks map[[2]string]uint64
	// unresolved sequence -> the height it was first recorded at
	unresolved map[[2]string]map[uint64]uint64
}

func newRepoState() *repoState {
	return &repoState{
		watermarks: make(map[[2]string]uint64),
		unresolved: make(map[[2]string]map[uint64]uint64),
	}
}

func packetKeyOf(p Packet) PacketKey {
	return PacketKey{
		SourceChainID:  p.SourceChainID,
		SourceClientID: p.PacketSourceClientID,
		Sequence:       p.PacketSequenceNumber,
	}
}

func (s *repoState) row(key PacketKey) *Packet {
	for i := range s.rows {
		if packetKeyOf(s.rows[i]) == key {
			return &s.rows[i]
		}
	}
	return nil
}

// repoOp one write, applied to the backend and to the model alike.
type repoOp struct {
	name  string
	apply func(ctx context.Context, repo Repository) error
	model func(s *repoState)
}

func drawOp(t *rapid.T) repoOp {
	key := drawKey(t)

	update := func(name string, apply func(context.Context, Repository) error, model func(*Packet)) repoOp {
		return repoOp{
			name:  fmt.Sprintf("%s %+v", name, key),
			apply: apply,
			model: func(s *repoState) {
				// an update of a key with no row affects nothing
				if p := s.row(key); p != nil {
					model(p)
				}
			},
		}
	}

	switch rapid.IntRange(0, 9).Draw(t, "op") {
	case 0:
		input := UpsertPacket{
			Status: rapid.SampledFrom([]RelayStatus{RelayStatusNotSelected, RelayStatusPending}).
				Draw(t, "status"),
			SourceChainID:             key.SourceChainID,
			DestinationChainID:        rapid.SampledFrom(modelChains).Draw(t, "destinationChain"),
			SourceTxHash:              rapid.SampledFrom(modelTxHashes).Draw(t, "sourceTx"),
			SourceTxTime:              drawTime(t, "sourceTxTime"),
			PacketSequenceNumber:      key.Sequence,
			PacketSourceClientID:      key.SourceClientID,
			PacketDestinationClientID: rapid.SampledFrom(modelClients).Draw(t, "destinationClient"),
			PacketTimeoutTimestamp:    drawTime(t, "timeout"),
		}

		return repoOp{
			name:  fmt.Sprintf("UpsertPacket %+v", input),
			apply: func(ctx context.Context, repo Repository) error { return repo.UpsertPacket(ctx, input) },
			model: func(s *repoState) {
				p := s.row(key)
				if p == nil {
					s.rows = append(s.rows, Packet{
						SourceChainID: key.SourceChainID, PacketSequenceNumber: key.Sequence,
						PacketSourceClientID: key.SourceClientID,
					})
					p = &s.rows[len(s.rows)-1]
				} else if p.Status != RelayStatusNotSelected {
					// only a NOT_SELECTED row is refreshed
					return
				}
				p.Status = input.Status
				p.DestinationChainID = input.DestinationChainID
				p.SourceTxHash = input.SourceTxHash
				p.SourceTxTime = input.SourceTxTime
				p.PacketDestinationClientID = input.PacketDestinationClientID
				p.PacketTimeoutTimestamp = input.PacketTimeoutTimestamp
			},
		}
	case 1:
		status := rapid.SampledFrom(AllRelayStatuses()).Draw(t, "status")
		return update("UpdatePacketStatus "+string(status),
			func(ctx context.Context, repo Repository) error { return repo.UpdatePacketStatus(ctx, key, status) },
			func(p *Packet) { p.Status = status })
	case 2:
		tx := drawPacketTx(t)
		return update(fmt.Sprintf("UpdatePacketRecvTx %+v", tx),
			func(ctx context.Context, repo Repository) error { return repo.UpdatePacketRecvTx(ctx, key, tx) },
			func(p *Packet) {
				p.RecvTxHash, p.RecvTxTime, p.RecvTxRelayerAddress = &tx.Hash, &tx.Time, &tx.RelayerAddress
			})
	case 3:
		return update("ClearPacketRecvTx",
			func(ctx context.Context, repo Repository) error { return repo.ClearPacketRecvTx(ctx, key) },
			func(p *Packet) {
				// the write ack lives in the recv tx, so it goes with it
				p.RecvTxHash, p.RecvTxTime, p.RecvTxRelayerAddress = nil, nil, nil
				p.WriteAckTxHash, p.WriteAckTxTime, p.WriteAckStatus = nil, nil, nil
			})
	case 4:
		ack := WriteAck{
			TxHash: rapid.SampledFrom(modelTxHashes).Draw(t, "writeAck.hash"),
			TxTime: drawTime(t, "writeAck.time"),
			Status: rapid.SampledFrom([]WriteAckStatus{
				WriteAckStatusSuccess, WriteAckStatusError, WriteAckStatusUnknown,
			}).Draw(t, "writeAck.status"),
		}
		return update(fmt.Sprintf("UpdatePacketWriteAck %+v", ack),
			func(ctx context.Context, repo Repository) error { return repo.UpdatePacketWriteAck(ctx, key, ack) },
			func(p *Packet) {
				p.WriteAckTxHash, p.WriteAckTxTime, p.WriteAckStatus = &ack.TxHash, &ack.TxTime, &ack.Status
			})
	case 5:
		tx := drawPacketTx(t)
		return update(fmt.Sprintf("UpdatePacketAckTx %+v", tx),
			func(ctx context.Context, repo Repository) error { return repo.UpdatePacketAckTx(ctx, key, tx) },
			func(p *Packet) {
				p.AckTxHash, p.AckTxTime, p.AckTxRelayerAddress = &tx.Hash, &tx.Time, &tx.RelayerAddress
			})
	case 6:
		return update("ClearPacketAckTx",
			func(ctx context.Context, repo Repository) error { return repo.ClearPacketAckTx(ctx, key) },
			func(p *Packet) { p.AckTxHash, p.AckTxTime, p.AckTxRelayerAddress = nil, nil, nil })
	case 7:
		tx := drawPacketTx(t)
		return update(fmt.Sprintf("UpdatePacketTimeoutTx %+v", tx),
			func(ctx context.Context, repo Repository) error { return repo.UpdatePacketTimeoutTx(ctx, key, tx) },
			func(p *Packet) {
				p.TimeoutTxHash, p.TimeoutTxTime, p.TimeoutTxRelayerAddress = &tx.Hash, &tx.Time, &tx.RelayerAddress
			})
	case 8:
		return update("ClearPacketTimeoutTx",
			func(ctx context.Context, repo Repository) error { return repo.ClearPacketTimeoutTx(ctx, key) },
			func(p *Packet) { p.TimeoutTxHash, p.TimeoutTxTime, p.TimeoutTxRelayerAddress = nil, nil, nil })
	default:
		return drawClearingOp(t, key.SourceChainID, key.SourceClientID)
	}
}

func drawClearingOp(t *rapid.T, chainID, clientID string) repoOp {
	sequences := rapid.SliceOfN(rapid.Uint64Range(1, 8), 0, 4)
	lastProbed := rapid.Uint64Range(0, 10).Draw(t, "lastProbed")
	delta := UnresolvedDelta{
		Add:     sequences.Draw(t, "add"),
		Resolve: sequences.Draw(t, "resolve"),
		Height:  rapid.Uint64Range(0, 6).Draw(t, "height"),
	}
	client := [2]string{chainID, clientID}

	return repoOp{
		name: fmt.Sprintf("SetClearingState %v lastProbed=%d %+v", client, lastProbed, delta),
		apply: func(ctx context.Context, repo Repository) error {
			return repo.SetClearingState(ctx, chainID, clientID, lastProbed, delta)
		},
		model: func(s *repoState) {
			// the watermark only moves forward
			s.watermarks[client] = max(s.watermarks[client], lastProbed)

			unresolved := s.unresolved[client]
			if unresolved == nil {
				unresolved = make(map[uint64]uint64)
				s.unresolved[client] = unresolved
			}

			// an add keeps the height a sequence was first seen at
			for _, sequence := range delta.Add {
				if _, ok := unresolved[sequence]; !ok {
					unresolved[sequence] = delta.Height
				}
			}

			// a resolve below that height is refused
			for _, sequence := range delta.Resolve {
				if height, ok := unresolved[sequence]; ok && height <= delta.Height {
					delete(unresolved, sequence)
				}
			}
		},
	}
}

type repositoryModel struct {
	ctx   context.Context
	store Store
	state *repoState
	log   []string
}

// Write applies one write outside a transaction.
func (m *repositoryModel) Write(t *rapid.T) {
	op := drawOp(t)
	m.log = append(m.log, op.name)

	require.NoError(t, op.apply(m.ctx, m.store), op.name)
	op.model(m.state)
}

var errRollback = errors.New("roll back")

// Transact applies several writes in one transaction, which commits or rolls
// back as a whole.
func (m *repositoryModel) Transact(t *rapid.T) {
	ops := make([]repoOp, rapid.IntRange(1, 4).Draw(t, "ops"))
	names := make([]string, len(ops))
	for i := range ops {
		ops[i] = drawOp(t)
		names[i] = ops[i].name
	}
	rollback := rapid.Bool().Draw(t, "rollback")

	m.log = append(m.log, fmt.Sprintf("Transact rollback=%t [%s]", rollback, strings.Join(names, "; ")))

	err := m.store.Transact(m.ctx, func(repo Repository) error {
		for _, op := range ops {
			if err := op.apply(m.ctx, repo); err != nil {
				return errors.Wrap(err, op.name)
			}
		}
		if rollback {
			return errRollback
		}
		return nil
	})

	if rollback {
		require.ErrorIs(t, err, errRollback)
		return
	}

	require.NoError(t, err)
	for _, op := range ops {
		op.model(m.state)
	}
}

// equal fails unless the backend's read matches the model's, reporting the
// writes that led there.
func (m *repositoryModel) equal(t *rapid.T, read string, want, got any) {
	require.Equal(t, want, got, "%s, after:\n  %s", read, strings.Join(m.log, "\n  "))
}

// normalized drops the fields each backend assigns for itself. It never
// returns nil, so an empty read equals an empty model.
func normalized(rows []Packet) []Packet {
	out := make([]Packet, len(rows))
	for i, p := range rows {
		p.ID, p.CreatedAt, p.UpdatedAt = 0, time.Time{}, time.Time{}
		out[i] = p
	}
	return out
}

func withEmpty(sequences []uint64) []uint64 {
	return append([]uint64{}, sequences...)
}

// Query reads one random page of a random filter, then walks every page of
// the filter and checks they add up to the model's rows.
func (m *repositoryModel) Query(t *rapid.T) {
	optional := func(values []string) *rapid.Generator[string] {
		return rapid.SampledFrom(append([]string{""}, values...))
	}

	filter := PacketFilter{
		Statuses:            rapid.SliceOfDistinct(rapid.SampledFrom(AllRelayStatuses()), rapid.ID).Draw(t, "statuses"),
		SourceChainID:       optional(modelChains).Draw(t, "sourceChain"),
		DestinationChainID:  optional(modelChains).Draw(t, "destinationChain"),
		SourceClientID:      optional(modelClients).Draw(t, "sourceClient"),
		DestinationClientID: optional(modelClients).Draw(t, "destinationClient"),
		SourceTxHash:        optional(modelTxHashes).Draw(t, "sourceTx"),
		SequenceNumber:      rapid.Uint64Range(0, 4).Draw(t, "sequence"),
	}
	limit := rapid.Int64Range(1, 4).Draw(t, "limit")
	read := fmt.Sprintf("ListPackets %+v, paged by %d", filter, limit)

	// a cursor that does not advance would page forever: every row fits in
	// this many pages, plus the empty one that ends the walk
	maxPages := len(m.state.rows)/int(limit) + 2

	var got []Packet

	page := Page{Limit: limit}
	for pages := 0; ; pages++ {
		if pages == maxPages {
			t.Fatalf("%s did not end within %d pages, after:\n  %s", read, maxPages, strings.Join(m.log, "\n  "))
		}

		rows, err := m.store.ListPackets(m.ctx, filter, page)
		require.NoError(t, err)
		require.LessOrEqual(t, int64(len(rows)), limit, read)

		if len(rows) == 0 {
			break
		}
		got = append(got, rows...)
		page.Before = rows[len(rows)-1].ID
	}

	m.equal(t, read, normalized(m.state.filter(filter)), normalized(got))
}

// filter the rows ListPackets returns for f, newest first.
func (s *repoState) filter(f PacketFilter) []Packet {
	var rows []Packet
	for i := len(s.rows) - 1; i >= 0; i-- {
		p := s.rows[i]
		switch {
		case !slices.Contains(f.Statuses, p.Status),
			f.SourceChainID != "" && p.SourceChainID != f.SourceChainID,
			f.DestinationChainID != "" && p.DestinationChainID != f.DestinationChainID,
			f.SourceClientID != "" && p.PacketSourceClientID != f.SourceClientID,
			f.DestinationClientID != "" && p.PacketDestinationClientID != f.DestinationClientID,
			f.SourceTxHash != "" && p.SourceTxHash != f.SourceTxHash,
			f.SequenceNumber != 0 && p.PacketSequenceNumber != f.SequenceNumber:
			continue
		}
		rows = append(rows, p)
	}
	return rows
}

// isDispatchable restates which statuses the dispatcher picks up, so the
// SQL's list is checked against the rules rather than against itself.
func isDispatchable(status RelayStatus) bool {
	switch status {
	case RelayStatusNotSelected, RelayStatusCompleteWithAck, RelayStatusCompleteWithTimeout,
		RelayStatusCompleteWithWriteAckError, RelayStatusFailed:
		return false
	default:
		return true
	}
}

func bySequence(a, b Packet) int {
	return cmp.Or(
		cmp.Compare(a.PacketSequenceNumber, b.PacketSequenceNumber),
		strings.Compare(a.PacketSourceClientID, b.PacketSourceClientID),
	)
}

// Check compares every read the domain allows against the model.
func (m *repositoryModel) Check(t *rapid.T) {
	all, err := m.store.ListPackets(m.ctx, PacketFilter{Statuses: AllRelayStatuses()}, Page{Limit: 1000})
	require.NoError(t, err)
	m.equal(t, "ListPackets(all)",
		normalized(m.state.filter(PacketFilter{Statuses: AllRelayStatuses()})), normalized(all))

	dispatchable, err := m.store.ListDispatchablePackets(m.ctx)
	require.NoError(t, err)
	m.equal(t, "ListDispatchablePackets", normalized(slices.DeleteFunc(slices.Clone(m.state.rows), func(p Packet) bool {
		return !isDispatchable(p.Status)
	})), normalized(dispatchable))

	for _, chainID := range modelChains {
		for _, hash := range modelTxHashes {
			want := slices.DeleteFunc(slices.Clone(m.state.rows), func(p Packet) bool {
				return p.SourceChainID != chainID || p.SourceTxHash != hash
			})
			slices.SortStableFunc(want, bySequence)

			got, err := m.store.ListPacketsBySourceTx(m.ctx, chainID, hash)
			require.NoError(t, err)

			read := fmt.Sprintf("ListPacketsBySourceTx(%s, %s)", chainID, hash)
			// ordered by sequence; rows sharing one across clients come in any order
			require.True(t, slices.IsSortedFunc(got, func(a, b Packet) int {
				return cmp.Compare(a.PacketSequenceNumber, b.PacketSequenceNumber)
			}), "%s not ordered by sequence", read)
			slices.SortStableFunc(got, bySequence)

			m.equal(t, read, normalized(want), normalized(got))
		}

		for _, clientID := range modelClients {
			client := [2]string{chainID, clientID}

			state, err := m.store.GetClearingState(m.ctx, chainID, clientID)
			require.NoError(t, err)

			unresolved := withEmpty(nil)
			for sequence := range m.state.unresolved[client] {
				unresolved = append(unresolved, sequence)
			}
			slices.Sort(unresolved)

			m.equal(t, fmt.Sprintf("GetClearingState(%s, %s)", chainID, clientID),
				ClearingState{LastProbed: m.state.watermarks[client], Unresolved: unresolved},
				ClearingState{LastProbed: state.LastProbed, Unresolved: withEmpty(state.Unresolved)})
		}
	}
}

// QuerySequences reads one client's recorded sequences in a random range.
func (m *repositoryModel) QuerySequences(t *rapid.T) {
	chainID := rapid.SampledFrom(modelChains).Draw(t, "chain")
	clientID := rapid.SampledFrom(modelClients).Draw(t, "client")
	from := rapid.Uint64Range(0, 5).Draw(t, "from")
	to := rapid.Uint64Range(0, 5).Draw(t, "to")

	want := withEmpty(nil)
	for _, p := range m.state.rows {
		if p.SourceChainID == chainID && p.PacketSourceClientID == clientID &&
			p.PacketSequenceNumber >= from && p.PacketSequenceNumber <= to {
			want = append(want, p.PacketSequenceNumber)
		}
	}
	slices.Sort(want)

	got, err := m.store.ListPacketSequences(m.ctx, chainID, clientID, from, to)
	require.NoError(t, err)

	m.equal(t, fmt.Sprintf("ListPacketSequences(%s, %s, %d, %d)", chainID, clientID, from, to), want, withEmpty(got))
}
