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

func localAttestor(name, signer string) AttestorConfig {
	return AttestorConfig{ChainID: "1", Name: name, Type: AttestorTypeLocal, Signer: signer}
}

func remoteAttestor(name, grpc string) AttestorConfig {
	return AttestorConfig{ChainID: "2", Name: name, Type: AttestorTypeRemote, GRPC: grpc}
}

func TestWithPatchAppendsNewSections(t *testing.T) {
	merged, conflicts, err := DefaultConfig().WithPatch(chainPatch())
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Chains, 2)
	require.Equal(t, "0xrouter2", merged.Chains[1].EVM.ICS26Router)
	require.Len(t, merged.Relayer.Connections, 1)
	require.Len(t, merged.Attestors, 1)
}

func TestWithPatchIsIdempotent(t *testing.T) {
	once, _, err := DefaultConfig().WithPatch(chainPatch())
	require.NoError(t, err)
	twice, conflicts, err := once.WithPatch(chainPatch())
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Equal(t, once, twice)
}

func TestWithPatchReplacesChangedEntries(t *testing.T) {
	once, _, err := DefaultConfig().WithPatch(chainPatch())
	require.NoError(t, err)

	changed := chainPatch()
	changed.Chains[0].EVM.ICS26Router = "0xdifferent"

	merged, conflicts, err := once.WithPatch(changed)
	require.NoError(t, err)

	require.Equal(t, []Conflict{{Kind: "chain", ID: "1"}}, conflicts)
	require.Equal(t, "chain 1", conflicts[0].String())
	require.Equal(t, "0xdifferent", merged.Chains[0].EVM.ICS26Router)
}

func TestWithPatchReplacesAChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chains = []ChainConfig{{ChainID: "1", EVM: &EVMChainConfig{
		RPC: "https://eth.example.com", WS: "wss://eth.example.com",
	}}}

	dropped, _, err := cfg.WithPatch(chainPatch())
	require.NoError(t, err)
	require.Empty(t, dropped.Chains[0].EVM.RPC)
	require.Empty(t, dropped.Chains[0].EVM.WS)

	carried := chainPatch()
	carried.Chains[0].EVM.RPC = "https://eth.example.com"
	carried.Chains[0].EVM.WS = "wss://eth.example.com"

	kept, _, err := cfg.WithPatch(carried)
	require.NoError(t, err)
	require.Equal(t, "https://eth.example.com", kept.Chains[0].EVM.RPC)
	require.Equal(t, "wss://eth.example.com", kept.Chains[0].EVM.WS)
}

func TestWithPatchKeepsRemoteAttestorsThatDifferByHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{remoteAttestor("watcher", "a.example.com:3000")}

	merged, conflicts, err := cfg.WithPatch(Patch{
		Attestors: Attestors{remoteAttestor("watcher", "b.example.com:3000")},
	})
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 2)
}

func TestWithPatchMatchesRemoteAttestorByNameAndHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{remoteAttestor("watcher", "a.example.com:3000")}

	merged, conflicts, err := cfg.WithPatch(Patch{
		Attestors: Attestors{remoteAttestor("watcher", "a.example.com:3000")},
	})
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 1)
}

func TestWithPatchRejectsLocalAttestorNameCollision(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{localAttestor("watcher", "k1")}
	_, _, err := cfg.WithPatch(Patch{Attestors: Attestors{localAttestor("watcher", "k2")}})
	require.ErrorContains(t, err, "already names a different")
}

func TestWithPatchPreservesSemanticIdentities(t *testing.T) {
	cfg := DefaultConfig()
	original := chainPatch()
	original.Connections[0].Alias = "my-route"
	disabled := false
	original.Connections[0].ClientA.AutoRelay.Enabled = &disabled
	original.Attestors = Attestors{localAttestor("my-watcher", "key")}
	original.Attestors[0].FinalityOffset = 42
	cfg, _, err := cfg.WithPatch(original)
	require.NoError(t, err)
	incoming := chainPatch()
	incoming.Connections[0].ClientA, incoming.Connections[0].ClientB = incoming.Connections[0].ClientB, incoming.Connections[0].ClientA
	incoming.Connections[0].ClientA.Signer = ""
	incoming.Connections[0].ClientB.Signer = ""
	incoming.Attestors = Attestors{localAttestor("generated-watcher", "key")}
	merged, conflicts, err := cfg.WithPatch(incoming)
	require.NoError(t, err)
	require.Empty(t, conflicts)
	require.Equal(t, cfg, merged)
	incoming.Connections[0].ClientB.Signer = "override"
	merged, conflicts, err = cfg.WithPatch(incoming)
	require.NoError(t, err)
	require.Equal(t, "override", merged.Relayer.Connections[0].ClientA.Signer)
	require.Len(t, conflicts, 1)
}

func TestWithPatchRejectsConnectionCollisions(t *testing.T) {
	cfg, _, err := DefaultConfig().WithPatch(chainPatch())
	require.NoError(t, err)
	incoming := chainPatch()
	incoming.Connections[0].ClientA.ClientID = "different"
	_, _, err = cfg.WithPatch(incoming)
	require.ErrorContains(t, err, "alias")
	incoming.Connections[0].Alias = "different-alias"
	_, _, err = cfg.WithPatch(incoming)
	require.ErrorContains(t, err, "duplicate client")
}

func TestWithPatchSeparatesLocalAndRemoteOfTheSameName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{localAttestor("watcher", "k1")}

	merged, conflicts, err := cfg.WithPatch(Patch{
		Attestors: Attestors{remoteAttestor("watcher", "a.example.com:3000")},
	})
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 2)
}

func TestWithPatchPreservesRemoteProver(t *testing.T) {
	cfg, _, err := DefaultConfig().WithPatch(chainPatch())
	require.NoError(t, err)
	cfg.Relayer.Connections[0].ClientA.Type = ClientTypeRemote
	cfg.Relayer.Connections[0].ClientA.Params = []byte("url: http://prover:8080")
	merged, conflicts, err := cfg.WithPatch(chainPatch())
	require.NoError(t, err)
	require.Empty(t, conflicts)
	require.Equal(t, cfg, merged)
}
