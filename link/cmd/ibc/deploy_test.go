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

func TestMergeManifests(t *testing.T) {
	// existing carries provenance and metadata Discover can't reconstruct
	existing := manifest.New("1", "evm")
	existing.Core.Router = "0xoldrouter"
	existing.Provenance.Deployer = "0xdeployer"
	existing.Provenance.ContractsVersion = "v1.2.3"
	existing.Provenance.TxHashes = map[string]string{"core": "0xcoretx"}
	existing.UpsertClient(manifest.Client{
		ClientID:             "link-2",
		Type:                 "attestation",
		Address:              "0xoldclient",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
		Params:               map[string]any{"threshold": float64(1)},
	})

	// discovered reflects live chain state, with fields Discover leaves
	// empty (counterpartyChainId, params) unset
	discovered := manifest.New("1", "evm")
	discovered.Core.Router = "0xnewrouter"
	discovered.TargetData = map[string]string{"accessManager": "0xam"}
	discovered.UpsertClient(manifest.Client{
		ClientID:             "link-2",
		Address:              "0xnewclient",
		CounterpartyClientID: "link-1",
	})

	merged := mergeManifests(existing, discovered)

	// chain-derived facts come from discovered
	require.Equal(t, "0xnewrouter", merged.Core.Router)
	require.Equal(t, "0xam", merged.TargetData["accessManager"])

	// provenance is preserved from existing
	require.Equal(t, "0xdeployer", merged.Provenance.Deployer)
	require.Equal(t, "v1.2.3", merged.Provenance.ContractsVersion)
	require.Equal(t, "0xcoretx", merged.Provenance.TxHashes["core"])

	// client keeps discovered's live address, but existing's metadata
	c, ok := merged.Client("link-2")
	require.True(t, ok)
	require.Equal(t, "0xnewclient", c.Address)
	require.Equal(t, "link-1", c.CounterpartyClientID)
	require.Equal(t, "2", c.CounterpartyChainID)
	require.Equal(t, "attestation", c.Type)
	require.Equal(t, map[string]any{"threshold": float64(1)}, c.Params)
}

func TestMergeManifestsNilExisting(t *testing.T) {
	discovered := manifest.New("1", "evm")
	discovered.Core.Router = "0xrouter"
	require.Same(t, discovered, mergeManifests(nil, discovered))
}

func TestRenderDeploymentConfig(t *testing.T) {
	m := manifest.New("1", "evm")
	m.Core.Router = "0xrouter"
	m.UpsertClient(manifest.Client{
		ClientID:             "link-2",
		Type:                 "attestation",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
		Params:               map[string]any{"threshold": float64(1)},
	})

	cfg := config.Config{Chains: []config.ChainConfig{{ChainID: "1", EVM: &config.EVMChainConfig{RPC: "http://rpc"}}}}
	out := renderDeploymentConfig(cfg, []*manifest.Manifest{m})

	require.Len(t, out.Chains, 1)
	require.Equal(t, "0xrouter", out.Chains[0].EVM.ICS26Router)
	require.Equal(t, "http://rpc", out.Chains[0].EVM.RPC)

	require.Len(t, out.Relayer.Clients, 1)
	client := out.Relayer.Clients[0]
	require.Equal(t, "1-link-2", client.Alias)
	require.Equal(t, "link-2", client.ClientID)
	require.Equal(t, "1", client.ChainID)
	require.Equal(t, "2", client.CounterpartyChainID)
	require.Equal(t, "link-1", client.CounterpartyClientID)
	require.NotNil(t, client.AttestorSet)
	require.Equal(t, 1, client.AttestorSet.Threshold)
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
