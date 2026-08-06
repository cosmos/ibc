package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

func TestResolveDeployerAlias(t *testing.T) {
	chain := config.ChainConfig{ChainID: "1", Deployer: "cfg-alias"}

	require.Equal(t, "flag-alias", resolveDeployerAlias(chain, "flag-alias"))
	require.Equal(t, "cfg-alias", resolveDeployerAlias(chain, ""))

	chain.Deployer = ""
	require.Equal(t, "", resolveDeployerAlias(chain, ""))
}

func TestStatusChains(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{Chains: []config.ChainConfig{{ChainID: "1"}, {ChainID: "2"}}}

	// explicit unknown chain is an error, not "no manifest"
	_, err := statusChains(cfg, dir, "barfoo")
	require.ErrorContains(t, err, `chain "barfoo" not declared in config`)

	// explicit known chain
	chains, err := statusChains(cfg, dir, "2")
	require.NoError(t, err)
	require.Equal(t, []string{"2"}, chains)

	// no flag: union of config chains and manifest files, deduped and sorted
	require.NoError(t, manifest.New("2", "evm").Save(dir))
	require.NoError(t, manifest.New("5", "evm").Save(dir))
	chains, err = statusChains(cfg, dir, "")
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2", "5"}, chains)
}

func TestRenderRelayConfig(t *testing.T) {
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "link-2", Type: "attestation", Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "link-1",
		Params: map[string]any{"threshold": float64(2)},
	})
	// stray client tracking another chain must not pair
	a.UpsertClient(manifest.Client{
		ClientID: "link-9", Type: "attestation",
		CounterpartyChainID: "9", CounterpartyClientID: "link-1",
	})
	b := manifest.New("2", "evm")
	b.Core.Router = "0xrouterB"
	b.UpsertClient(manifest.Client{
		ClientID: "link-1", Type: "attestation", Address: "0xcb",
		CounterpartyChainID: "1", CounterpartyClientID: "link-2",
		Params: map[string]any{"threshold": float64(2)},
	})

	out, err := renderRelayConfig(a, b)
	require.NoError(t, err)

	require.Len(t, out.Relayer.Clients, 2)
	ca := out.Relayer.Clients[0]
	require.Equal(t, "1-link-2", ca.Alias)
	require.Equal(t, "link-2", ca.ClientID)
	require.Equal(t, "1", ca.ChainID)
	require.Equal(t, "2", ca.CounterpartyChainID)
	require.Equal(t, "link-1", ca.CounterpartyClientID)
	require.NotNil(t, ca.AttestorSet)
	require.Equal(t, 2, ca.AttestorSet.Threshold)
	require.Equal(t, "link-1", out.Relayer.Clients[1].ClientID)

	// no mutual pair: B has no client tracking A back
	empty := manifest.New("2", "evm")
	empty.Core.Router = "0xrouterB"
	_, err = renderRelayConfig(a, empty)
	require.ErrorContains(t, err, "no mutual client pair")

	// mismatched back-reference: B's client points at a different A client
	mismatched := manifest.New("2", "evm")
	mismatched.Core.Router = "0xrouterB"
	mismatched.UpsertClient(manifest.Client{
		ClientID: "link-1", Type: "attestation",
		CounterpartyChainID: "1", CounterpartyClientID: "link-other",
	})
	_, err = renderRelayConfig(a, mismatched)
	require.ErrorContains(t, err, "no mutual client pair")
}
