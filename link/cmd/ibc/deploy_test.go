// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/config"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

// newLocalSignerConfig generates a real local secp256k1 key, stores it to a
// temp keyfile, and returns both the config entry referencing it and its
// derived EVM address -- attestor resolution derives addresses the same way,
// so tests need a real key to exercise it end to end.
func newLocalSignerConfig(t *testing.T, alias string) (config.SignerConfig, string) {
	t.Helper()

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), alias+".json")
	require.NoError(t, key.StoreToFile(path))

	address, err := signer.PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	return config.SignerConfig{Alias: alias, Type: config.SignerLocal, File: path}, address
}

func TestResolveDeployerAlias(t *testing.T) {
	chain := config.ChainConfig{ChainID: "1", Deployer: "cfg-alias"}

	require.Equal(t, "flag-alias", resolveDeployerAlias(chain, "flag-alias"))
	require.Equal(t, "cfg-alias", resolveDeployerAlias(chain, ""))

	chain.Deployer = ""
	require.Empty(t, resolveDeployerAlias(chain, ""))
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
	watcherSigner, watcherAddress := newLocalSignerConfig(t, "attestor-watching-2")

	// client ids follow defaultClientID's real convention: both ends of a
	// connection share the same sorted "link-<a>-<b>" name.
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "link-1-2", Type: "attestation", Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "link-1-2",
		Params: map[string]any{
			"threshold": float64(2),
			"attestors": []any{watcherAddress, "0xUnresolvedAddress"},
		},
	})
	// stray client tracking another chain must not pair
	a.UpsertClient(manifest.Client{
		ClientID: "link-1-9", Type: "attestation",
		CounterpartyChainID: "9", CounterpartyClientID: "link-1-9",
	})
	// a second, custom-named connection between the same chain pair --
	// exercises the alias seqno suffix, and reuses the same attestor address
	// watching chain 2 to exercise cross-connection attestor deduplication.
	a.UpsertClient(manifest.Client{
		ClientID: "custom-a", Type: "attestation", Address: "0xca2",
		CounterpartyChainID: "2", CounterpartyClientID: "custom-b",
		Params: map[string]any{
			"threshold": float64(2),
			"attestors": []any{watcherAddress},
		},
	})
	b := manifest.New("2", "evm")
	b.Core.Router = "0xrouterB"
	b.UpsertClient(manifest.Client{
		ClientID: "link-1-2", Type: "attestation", Address: "0xcb",
		CounterpartyChainID: "1", CounterpartyClientID: "link-1-2",
		Params: map[string]any{"threshold": float64(2)},
	})
	b.UpsertClient(manifest.Client{
		ClientID: "custom-b", Type: "attestation", Address: "0xcb2",
		CounterpartyChainID: "1", CounterpartyClientID: "custom-a",
		// no configured attestors: must not surface as an attestors: entry
		Params: map[string]any{"threshold": float64(2)},
	})

	unreferencedSigner, _ := newLocalSignerConfig(t, "unused-signer")
	cfg := config.Config{
		Chains: []config.ChainConfig{
			{ChainID: "1", EVM: &config.EVMChainConfig{RPC: "http://a", ICS26Router: "0xstale"}},
		},
		Signers: config.Signers{watcherSigner, unreferencedSigner},
	}
	out, _, err := renderRelayConfig(cfg, a, b)
	require.NoError(t, err)

	// chain 1 is declared: its config is copied, router updated, rpc kept
	require.Len(t, out.Chains, 2)
	require.Equal(t, "1", out.Chains[0].ChainID)
	require.Equal(t, "0xrouterA", out.Chains[0].EVM.ICS26Router)
	require.Equal(t, "http://a", out.Chains[0].EVM.RPC)
	// chain 2 is undeclared: minimal entry with just the router
	require.Equal(t, "2", out.Chains[1].ChainID)
	require.Equal(t, "0xrouterB", out.Chains[1].EVM.ICS26Router)
	require.Empty(t, out.Chains[1].EVM.RPC)

	require.Len(t, out.Relayer.Connections, 2)
	conn := out.Relayer.Connections[0]
	require.Equal(t, "1-2", conn.Alias)
	require.Equal(t, "link-1-2", conn.ClientA.ClientID)
	require.Equal(t, "1", conn.ClientA.ChainID)
	require.Equal(t, "link-1-2", conn.ClientB.ClientID)
	require.Equal(t, "2", conn.ClientB.ChainID)

	// second connection between the same chain pair gets a seqno suffix
	conn2 := out.Relayer.Connections[1]
	require.Equal(t, "1-2-1", conn2.Alias)
	require.Equal(t, "custom-a", conn2.ClientA.ClientID)
	require.Equal(t, "custom-b", conn2.ClientB.ClientID)

	// A's first client has two attestor addresses: one resolves to a
	// configured signer, one doesn't -- both still get an entry, watching
	// chain 2 (the chain that client tracks). A's second client (custom-a)
	// reuses watcherAddress and must not produce a duplicate entry.
	require.Len(t, out.Attestors, 2)
	require.Equal(t, "2", out.Attestors[0].ChainID)
	require.Equal(t, "attestor-2-"+watcherAddress, out.Attestors[0].Name)
	require.Equal(t, "attestor-watching-2", out.Attestors[0].Signer)
	require.Equal(t, config.AttestorTypeLocal, out.Attestors[0].Type)
	require.EqualValues(t, 1, out.Attestors[0].FinalityOffset)
	require.Equal(t, "attestor-2-0xUnresolvedAddress", out.Attestors[1].Name)
	require.Empty(t, out.Attestors[1].Signer)
	require.EqualValues(t, 1, out.Attestors[1].FinalityOffset)

	// no mutual pair: B has no client tracking A back
	empty := manifest.New("2", "evm")
	empty.Core.Router = "0xrouterB"
	_, _, err = renderRelayConfig(cfg, a, empty)
	require.ErrorContains(t, err, "no mutual client pair")

	// mismatched back-reference: B's client points at a different A client
	mismatched := manifest.New("2", "evm")
	mismatched.Core.Router = "0xrouterB"
	mismatched.UpsertClient(manifest.Client{
		ClientID: "link-1", Type: "attestation",
		CounterpartyChainID: "1", CounterpartyClientID: "link-other",
	})
	_, _, err = renderRelayConfig(cfg, a, mismatched)
	require.ErrorContains(t, err, "no mutual client pair")
}

// goccy silently drops comments whose path doesn't resolve, so assert the
// paths agree with the emitted document rather than just with each other.
// CollectComments' own logic is unit-tested directly in config.
func TestRenderConfigEmitsComments(t *testing.T) {
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "link-1-2", Type: "attestation",
		CounterpartyChainID: "2", CounterpartyClientID: "link-1-2",
		// unresolvable address: exercises the attestor signer TODO
		Params: map[string]any{"attestors": []any{"0xUnresolvedAddress"}},
	})
	b := manifest.New("2", "evm")
	// chain 2's router is left blank on purpose: exercises the ics26Router
	// TODO alongside the signer TODOs in the same render
	b.UpsertClient(manifest.Client{
		ClientID: "link-1-2", Type: "attestation",
		CounterpartyChainID: "1", CounterpartyClientID: "link-1-2",
	})

	out, comments, err := renderRelayConfig(config.Config{}, a, b)
	require.NoError(t, err)

	rendered := captureStdout(t, func() {
		require.NoError(t, printYAMLWithComments(out, comments))
	})

	require.Contains(t, rendered, `signer: "" # TODO: signers[] alias that submits relay txs on chainA`)
	require.Contains(t, rendered, `signer: "" # TODO: signers[] alias that submits relay txs on chainB`)
	require.Contains(t, rendered, `ics26Router: "" # TODO: fill in`)
	require.Contains(t, rendered, `signer: "" # TODO: signers[] alias backing this attestor's key`)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	require.NoError(t, w.Close())

	bz, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(bz)
}
