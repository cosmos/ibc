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

func TestTokenAndBridgeHelpers(t *testing.T) {
	dir := t.TempDir()
	m := New("1", "evm")
	m.GMP = &GMP{Address: "0xgmp", AccountLogic: "0xlogic", Port: "gmpport"}
	m.SendCallConstructor = "0xctor"
	m.UpsertToken(Token{Symbol: "FOO", Name: "Foo", Address: "0xfoo", Owner: "0xowner"})

	// update in place, not append
	m.UpsertToken(Token{Symbol: "FOO", Name: "Foo", Address: "0xfoo2", Owner: "0xowner"})
	require.Len(t, m.Tokens, 1)
	tok, ok := m.Token("FOO")
	require.True(t, ok)
	require.Equal(t, "0xfoo2", tok.Address)

	// bridge upsert on a known symbol
	require.True(
		t,
		m.UpsertBridge("FOO", Bridge{ClientID: "link-2", CounterpartyIFT: "0xcp", SendCallConstructor: "0xctor"}),
	)
	require.False(t, m.UpsertBridge("BAR", Bridge{ClientID: "link-2"}))
	tok, _ = m.Token("FOO")
	b, ok := tok.Bridge("link-2")
	require.True(t, ok)
	require.Equal(t, "0xcp", b.CounterpartyIFT)

	// round-trips through disk
	require.NoError(t, m.Save(dir))
	loaded, err := Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xgmp", loaded.GMP.Address)
	require.Equal(t, "0xctor", loaded.SendCallConstructor)
	lt, ok := loaded.Token("FOO")
	require.True(t, ok)
	require.Len(t, lt.Bridges, 1)
}
