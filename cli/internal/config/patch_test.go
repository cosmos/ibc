// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func chainPatch() Patch {
	return Patch{
		Chains: []ChainConfig{
			{ChainID: "1", EVM: &EVMChainConfig{ICS26Router: "0xrouter1"}},
			{ChainID: "2", EVM: &EVMChainConfig{ICS26Router: "0xrouter2"}},
		},
		Connections: []ConnectionConfig{{
			Alias:   "1-2",
			ClientA: ClientEnd{ChainID: "1", Signer: "s", ClientID: "cli-1-2", Type: ClientTypeAttestation},
			ClientB: ClientEnd{ChainID: "2", Signer: "s", ClientID: "cli-1-2", Type: ClientTypeAttestation},
		}},
		Attestors: Attestors{{ChainID: "2", Name: "attestor-2", Type: AttestorTypeRemote, GRPC: "a.example.com:3000"}},
	}
}

// A patched chain replaces the configured one outright, so the patch has to
// carry every field worth keeping. render-config builds each chain from the
// configured one for exactly this reason.
// A patched chain replaces the configured one outright, so the patch has to
// carry every field worth keeping. render-config builds each chain from the
// configured one for exactly this reason.
func TestWithPatchReplacesAChainWholesale(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chains = []ChainConfig{{ChainID: "1", EVM: &EVMChainConfig{
		RPC: "https://eth.example.com", WS: "wss://eth.example.com",
	}}}

	carried := chainPatch()
	carried.Chains[0].EVM.RPC = "https://eth.example.com"
	carried.Chains[0].EVM.WS = "wss://eth.example.com"

	merged, _ := cfg.WithPatch(carried)
	require.Equal(t, "https://eth.example.com", merged.Chains[0].EVM.RPC)
	require.Equal(t, "0xrouter1", merged.Chains[0].EVM.ICS26Router)

	// and a patch that omits them drops them
	dropped, _ := cfg.WithPatch(chainPatch())
	require.Empty(t, dropped.Chains[0].EVM.RPC)
	require.Empty(t, dropped.Chains[0].EVM.WS)
}

func TestWithPatchAppendsNewSections(t *testing.T) {
	merged, conflicts := DefaultConfig().WithPatch(chainPatch())

	require.Empty(t, conflicts, "nothing existed to change")
	require.Len(t, merged.Chains, 2)
	require.Equal(t, "0xrouter2", merged.Chains[1].EVM.ICS26Router)
	require.Len(t, merged.Relayer.Connections, 1)
	require.Len(t, merged.Attestors, 1)
}

func TestWithPatchIsIdempotent(t *testing.T) {
	once, _ := DefaultConfig().WithPatch(chainPatch())
	twice, conflicts := once.WithPatch(chainPatch())

	require.Empty(t, conflicts)
	require.Equal(t, once, twice)
}

func TestWithPatchReplacesChangedEntries(t *testing.T) {
	once, _ := DefaultConfig().WithPatch(chainPatch())

	changed := chainPatch()
	changed.Chains[0].EVM.ICS26Router = "0xdifferent"

	merged, conflicts := once.WithPatch(changed)
	require.Equal(t, []Conflict{{Kind: "chain", ID: "1"}}, conflicts)
	require.Equal(t, "0xdifferent", merged.Chains[0].EVM.ICS26Router)
}

func TestWithPatchReportsConflicts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chains = []ChainConfig{{ChainID: "1", EVM: &EVMChainConfig{ICS26Router: "0xold"}}}

	merged, conflicts := cfg.WithPatch(Patch{
		Chains: []ChainConfig{{ChainID: "1", EVM: &EVMChainConfig{ICS26Router: "0xnew"}}},
	})

	require.Len(t, conflicts, 1)
	require.Equal(t, "chain", conflicts[0].Kind)
	require.Equal(t, "1", conflicts[0].ID)
	require.Equal(t, "chain 1", conflicts[0].String())
	require.Equal(t, "0xnew", merged.Chains[0].EVM.ICS26Router)
}

func remoteAttestor(name, grpc string) AttestorConfig {
	return AttestorConfig{ChainID: "2", Name: name, Type: AttestorTypeRemote, GRPC: grpc}
}

func TestWithPatchKeepsRemoteAttestorsThatDifferByHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{remoteAttestor("watcher", "a.example.com:3000")}

	merged, conflicts := cfg.WithPatch(Patch{
		Attestors: Attestors{remoteAttestor("watcher", "b.example.com:3000")},
	})

	require.Empty(t, conflicts, "a different host is a different attestor, not a conflict")
	require.Len(t, merged.Attestors, 2)
}

func TestWithPatchMatchesRemoteAttestorByNameAndHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{remoteAttestor("watcher", "a.example.com:3000")}

	merged, conflicts := cfg.WithPatch(Patch{
		Attestors: Attestors{remoteAttestor("watcher", "a.example.com:3000")},
	})

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 1, "same name and host is the same attestor")
}

func TestWithPatchMatchesLocalAttestorByNameAlone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{{ChainID: "1", Name: "watcher", Type: AttestorTypeLocal, Signer: "k1"}}

	merged, conflicts := cfg.WithPatch(Patch{
		Attestors: Attestors{{ChainID: "1", Name: "watcher", Type: AttestorTypeLocal, Signer: "k2"}},
	})

	require.Len(t, merged.Attestors, 1, "local names are unique, so this is the same attestor")
	require.Equal(t, []Conflict{{Kind: "attestor", ID: "local watcher"}}, conflicts)
	require.Equal(t, "k2", merged.Attestors[0].Signer)
}

func TestWithPatchSeparatesLocalAndRemoteOfTheSameName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{{ChainID: "1", Name: "watcher", Type: AttestorTypeLocal, Signer: "k1"}}

	merged, conflicts := cfg.WithPatch(Patch{
		Attestors: Attestors{remoteAttestor("watcher", "a.example.com:3000")},
	})

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 2)
}
