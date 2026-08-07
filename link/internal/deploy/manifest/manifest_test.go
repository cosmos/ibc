package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	m, err := Load(dir, "1")
	require.NoError(t, err)
	require.Nil(t, m)

	m = New("1", "evm")
	m.Core.Router = "0xabc"
	m.TargetData = map[string]string{"accessManager": "0xdef"}
	m.UpsertClient(
		Client{
			ClientID:             "link-2",
			Type:                 "attestation",
			Address:              "0x1",
			CounterpartyChainID:  "2",
			CounterpartyClientID: "link-1",
		},
	)
	require.NoError(t, m.Save(dir))

	got, err := Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xabc", got.Core.Router)
	require.Equal(t, 1, got.SchemaVersion)

	c, ok := got.Client("link-2")
	require.True(t, ok)
	require.Equal(t, "0x1", c.Address)

	// upsert replaces, not duplicates
	got.UpsertClient(Client{ClientID: "link-2", Type: "attestation", Address: "0x2"})
	require.Len(t, got.Clients, 1)
	c, _ = got.Client("link-2")
	require.Equal(t, "0x2", c.Address)
}
