// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	m, err := Load(dir, "1")
	require.NoError(t, err)
	require.Nil(t, m)

	attestation := Client{
		ClientID:             "cli-2",
		Address:              "0x1",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1",
		Attestation: &AttestationParams{
			Attestors:        []string{"0xa1", "0xa2"},
			Threshold:        2,
			InitialHeight:    5,
			InitialTimestamp: 500,
		},
	}
	besu := Client{
		ClientID:             "cli-3",
		Address:              "0x3",
		CounterpartyChainID:  "3",
		CounterpartyClientID: "cli-1",
		Besu: &BesuParams{
			Consensus:         "qbft",
			IBCRouter:         "0xrouter",
			InitialHeight:     112,
			InitialTimestamp:  1788192445,
			InitialStateRoot:  "0xroot",
			InitialValidators: []string{"0xv1", "0xv2"},
			TrustingPeriod:    1209600,
			MaxClockDrift:     15,
			TrustLevel:        TrustLevel{Numerator: 2, Denominator: 3},
		},
	}
	m = New("1", "evm")
	m.Core.Router = "0xabc"
	m.TargetData = map[string]string{"accessManager": "0xdef"}
	m.UpsertClient(attestation)
	m.UpsertClient(besu)
	require.NoError(t, m.Save(dir))

	got, err := Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xabc", got.Core.Router)
	require.Equal(t, 1, got.SchemaVersion)
	require.Equal(t, []Client{attestation, besu}, got.Clients)
	require.Equal(t, config.ClientTypeAttestation, got.Clients[0].Type())
	require.Equal(t, config.ClientTypeBesu, got.Clients[1].Type())

	// upsert replaces, not duplicates
	got.UpsertClient(Client{ClientID: "cli-2", Address: "0x2", Attestation: &AttestationParams{}})
	require.Len(t, got.Clients, 2)
	c, _ := got.Client("cli-2")
	require.Equal(t, "0x2", c.Address)
}

func TestLoadRejectsClientWithoutParams(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"chainId": "1", "clients": [{"clientId": "cli-2", "type": "besu-qbft", "params": {"trustingPeriod": 1}}]}`
	require.NoError(t, os.WriteFile(Path(dir, "1"), []byte(legacy), 0o600))

	_, err := Load(dir, "1")
	require.ErrorContains(t, err, `client "cli-2" records no params`)
}

func TestTokenAndBridgeHelpers(t *testing.T) {
	dir := t.TempDir()
	m := New("1", "evm")
	m.GMP = &GMP{Address: "0xgmp", AccountLogic: "0xlogic", Port: "gmpport"}
	m.EVMSendCallConstructor = "0xctor"
	m.UpsertToken(Token{Symbol: "FOO", Name: "Foo", Address: "0xfoo", Owner: "0xowner"})

	// same identity (symbol+name+owner) updates in place, not append
	m.UpsertToken(Token{Symbol: "FOO", Name: "Foo", Address: "0xfoo2", Owner: "0xowner"})
	require.Len(t, m.Tokens, 1)
	tok, ok := m.TokenByIdentity("FOO", "Foo", "0xowner")
	require.True(t, ok)
	require.Equal(t, "0xfoo2", tok.Address)

	// same symbol, different name is a distinct token (symbol is not unique)
	m.UpsertToken(Token{Symbol: "FOO", Name: "Bar", Address: "0xbar", Owner: "0xowner"})
	require.Len(t, m.Tokens, 2)
	byID, ok := m.TokenByIdentity("FOO", "Bar", "0xOWNER")
	require.True(t, ok)
	require.Equal(t, "0xbar", byID.Address)

	// bridge upsert keyed by token address; unknown address returns false
	require.True(
		t,
		m.UpsertBridge("0xfoo2", Bridge{ClientID: "cli-2", CounterpartyIFT: "0xcp", SendCallConstructor: "0xctor"}),
	)
	require.False(t, m.UpsertBridge("0xmissing", Bridge{ClientID: "cli-2"}))
	tok, _ = m.TokenByAddress("0xfoo2")
	b, ok := tok.Bridge("cli-2")
	require.True(t, ok)
	require.Equal(t, "0xcp", b.CounterpartyIFT)

	// round-trips through disk
	require.NoError(t, m.Save(dir))
	loaded, err := Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xgmp", loaded.GMP.Address)
	require.Equal(t, "0xctor", loaded.EVMSendCallConstructor)
	lt, ok := loaded.TokenByAddress("0xfoo2")
	require.True(t, ok)
	require.Len(t, lt.Bridges, 1)
}
