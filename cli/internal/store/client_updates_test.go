// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testClientUpdateJournal(t *testing.T, db Store) {
	t.Helper()
	ctx := context.Background()
	pending := PacketTx{Hash: "checkpoint", Time: time.Now().UTC().Truncate(time.Microsecond), RelayerAddress: "wallet"}
	got, err := db.GetClientUpdate(ctx, "host", "client")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(
		t,
		db.Transact(ctx, func(repo Repository) error { return repo.SaveClientUpdate(ctx, "host", "client", pending) }),
	)
	require.Error(t, db.SaveClientUpdate(ctx, "host", "client", pending), "must not overwrite an in-flight submission")
	got, err = db.GetClientUpdate(ctx, "host", "client")
	require.NoError(t, err)
	require.Equal(t, pending, *got)
	got, err = db.GetClientUpdate(ctx, "other", "client")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, db.ClearClientUpdate(ctx, "host", "client"))
	got, err = db.GetClientUpdate(ctx, "host", "client")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestClientUpdateJournalReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.db")
	db, err := NewSqlite(path)
	require.NoError(t, err)
	_, err = db.MigrateUp()
	require.NoError(t, err)
	pending := PacketTx{Hash: "in-flight", Time: time.Now().UTC()}
	require.NoError(t, db.SaveClientUpdate(t.Context(), "host", "client", pending))
	require.NoError(t, db.Close())
	db, err = NewSqlite(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	got, err := db.GetClientUpdate(t.Context(), "host", "client")
	require.NoError(t, err)
	require.Equal(t, pending, *got)
}
