// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func deploymentConfig() DeploymentConfig {
	return DeploymentConfig{
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

func TestReconcileDeploymentAppendsNewSections(t *testing.T) {
	merged, conflicts, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Chains, 2)
	require.Equal(t, "0xrouter2", merged.Chains[1].EVM.ICS26Router)
	require.Len(t, merged.Relayer.Connections, 1)
	require.Len(t, merged.Attestors, 1)
}

func TestReconcileDeploymentIsIdempotent(t *testing.T) {
	once, _, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)
	twice, conflicts, err := once.ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Equal(t, once, twice)
}

func TestReconcileDeploymentReplacesChangedEntries(t *testing.T) {
	once, _, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)

	changed := deploymentConfig()
	changed.Chains[0].EVM.ICS26Router = "0xdifferent"

	merged, conflicts, err := once.ReconcileDeployment(changed)
	require.NoError(t, err)

	require.Equal(t, []Conflict{{Kind: "chain", ID: "1"}}, conflicts)
	require.Equal(t, "chain 1", conflicts[0].String())
	require.Equal(t, "0xdifferent", merged.Chains[0].EVM.ICS26Router)
}

func TestReconcileDeploymentReplacesAChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chains = []ChainConfig{{ChainID: "1", EVM: &EVMChainConfig{
		RPC: "https://eth.example.com", WS: "wss://eth.example.com",
	}}}

	dropped, _, err := cfg.ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)
	require.Empty(t, dropped.Chains[0].EVM.RPC)
	require.Empty(t, dropped.Chains[0].EVM.WS)

	carried := deploymentConfig()
	carried.Chains[0].EVM.RPC = "https://eth.example.com"
	carried.Chains[0].EVM.WS = "wss://eth.example.com"

	kept, _, err := cfg.ReconcileDeployment(carried)
	require.NoError(t, err)
	require.Equal(t, "https://eth.example.com", kept.Chains[0].EVM.RPC)
	require.Equal(t, "wss://eth.example.com", kept.Chains[0].EVM.WS)
}

func TestReconcileDeploymentKeepsRemoteAttestorsThatDifferByHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{remoteAttestor("watcher", "a.example.com:3000")}

	merged, conflicts, err := cfg.ReconcileDeployment(DeploymentConfig{
		Attestors: Attestors{remoteAttestor("watcher", "b.example.com:3000")},
	})
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 2)
}

func TestReconcileDeploymentMatchesRemoteAttestorByNameAndHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{remoteAttestor("watcher", "a.example.com:3000")}

	merged, conflicts, err := cfg.ReconcileDeployment(DeploymentConfig{
		Attestors: Attestors{remoteAttestor("watcher", "a.example.com:3000")},
	})
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 1)
}

func TestReconcileDeploymentRejectsLocalAttestorNameCollision(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{localAttestor("watcher", "k1")}
	_, _, err := cfg.ReconcileDeployment(DeploymentConfig{Attestors: Attestors{localAttestor("watcher", "k2")}})
	require.ErrorContains(t, err, "already names a different")
}

func TestReconcileDeploymentPreservesSemanticIdentities(t *testing.T) {
	cfg := DefaultConfig()
	original := deploymentConfig()
	original.Connections[0].Alias = "my-route"
	disabled := false
	original.Connections[0].ClientA.AutoRelay.Enabled = &disabled
	original.Attestors = Attestors{localAttestor("my-watcher", "key")}
	original.Attestors[0].FinalityOffset = 42
	cfg, _, err := cfg.ReconcileDeployment(original)
	require.NoError(t, err)
	incoming := deploymentConfig()
	incoming.Connections[0].ClientA, incoming.Connections[0].ClientB = incoming.Connections[0].ClientB, incoming.Connections[0].ClientA
	incoming.Connections[0].ClientA.Signer = ""
	incoming.Connections[0].ClientB.Signer = ""
	incoming.Attestors = Attestors{localAttestor("generated-watcher", "key")}
	merged, conflicts, err := cfg.ReconcileDeployment(incoming)
	require.NoError(t, err)
	require.Empty(t, conflicts)
	require.Equal(t, cfg, merged)
	incoming.Connections[0].ClientB.Signer = "override"
	merged, conflicts, err = cfg.ReconcileDeployment(incoming)
	require.NoError(t, err)
	require.Equal(t, "override", merged.Relayer.Connections[0].ClientA.Signer)
	require.Len(t, conflicts, 1)
}

func TestReconcileDeploymentRejectsConnectionCollisions(t *testing.T) {
	cfg, _, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)
	incoming := deploymentConfig()
	incoming.Connections[0].ClientA.ClientID = "different"
	_, _, err = cfg.ReconcileDeployment(incoming)
	require.ErrorContains(t, err, "duplicate client")
	incoming.Connections[0].Alias = "different-alias"
	_, _, err = cfg.ReconcileDeployment(incoming)
	require.ErrorContains(t, err, "duplicate client")
}

func TestReconcileDeploymentSeparatesLocalAndRemoteOfTheSameName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{localAttestor("watcher", "k1")}

	merged, conflicts, err := cfg.ReconcileDeployment(DeploymentConfig{
		Attestors: Attestors{remoteAttestor("watcher", "a.example.com:3000")},
	})
	require.NoError(t, err)

	require.Empty(t, conflicts)
	require.Len(t, merged.Attestors, 2)
}

func TestReconcileDeploymentRejectsClientTypeChange(t *testing.T) {
	for _, routerChanged := range []bool{false, true} {
		name := "same router"
		if routerChanged {
			name = "replacement router"
		}
		t.Run(name, func(t *testing.T) {
			cfg, _, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
			require.NoError(t, err)
			incoming := deploymentConfig()
			if routerChanged {
				incoming.Chains[0].EVM.ICS26Router = "0xreplacement"
			}
			// Exercise the transition policy without claiming another on-chain
			// client type is currently supported by deployment tooling.
			incoming.Connections[0].ClientA.Type = ClientType("future-client-type")
			_, _, err = cfg.ReconcileDeployment(incoming)
			require.ErrorContains(
				t,
				err,
				`client "cli-1-2" on chain "1" has type "attestation", manifest has "future-client-type"`,
			)
			require.ErrorContains(t, err, "automatic client-type changes are not supported")
			require.ErrorContains(t, err, "explicitly update the client's type and compatible params")
			require.Equal(t, "0xrouter1", cfg.Chains[0].EVM.ICS26Router)
			require.Equal(t, ClientTypeAttestation, cfg.Relayer.Connections[0].ClientA.Type)
		})
	}
}

func TestReconcileDeploymentPreservesRemoteProver(t *testing.T) {
	cfg, _, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)
	cfg.Relayer.Connections[0].ClientA.Type = ClientTypeRemote
	cfg.Relayer.Connections[0].ClientA.Params = []byte("url: http://prover:8080")
	merged, conflicts, err := cfg.ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)
	require.Empty(t, conflicts)
	require.Equal(t, cfg, merged)
}

func TestReconcileDeploymentResolvesDraftAttestor(t *testing.T) {
	for _, tc := range []struct {
		name, existingSigner, incomingSigner, wantSigner string
	}{
		{name: "fill", incomingSigner: "key", wantSigner: "key"},
		{name: "retain", existingSigner: "key", wantSigner: "key"},
		{name: "unresolved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Attestors = Attestors{localAttestor("watcher", tc.existingSigner)}
			cfg.Attestors[0].FinalityOffset = 42
			incoming := DeploymentConfig{Attestors: Attestors{localAttestor("watcher", tc.incomingSigner)}}
			merged, conflicts, err := cfg.ReconcileDeployment(incoming)
			require.NoError(t, err)
			require.Empty(t, conflicts, "filling a missing value does not overwrite a setting")
			require.Len(t, merged.Attestors, 1)
			want := cfg.Attestors[0]
			want.Signer = tc.wantSigner
			require.Equal(t, want, merged.Attestors[0])
			require.Equal(t, tc.existingSigner, cfg.Attestors[0].Signer, "must not mutate the input")
			again, conflicts, err := merged.ReconcileDeployment(incoming)
			require.NoError(t, err)
			require.Empty(t, conflicts)
			require.Equal(t, merged, again)
		})
	}
}

func TestReconcileDeploymentDoesNotGuessUnresolvedAttestorIdentity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Attestors = Attestors{localAttestor("unresolved", "")}
	named := DeploymentConfig{Attestors: Attestors{localAttestor("different-name", "key")}}
	merged, _, err := cfg.ReconcileDeployment(named)
	require.NoError(t, err)
	require.Len(t, merged.Attestors, 2)
	require.Empty(t, merged.Attestors[0].Signer)

	incoming := localAttestor("unresolved", "key")
	incoming.ChainID = "different-chain"
	_, _, err = cfg.ReconcileDeployment(DeploymentConfig{Attestors: Attestors{incoming}})
	require.ErrorContains(t, err, "already names a different")
}

func TestReconcileDeploymentRejectsResolutionToExistingAttestorIdentity(t *testing.T) {
	for _, draftFirst := range []bool{true, false} {
		cfg := DefaultConfig()
		cfg.Attestors = Attestors{localAttestor("resolved", "key"), localAttestor("draft", "")}
		if draftFirst {
			cfg.Attestors[0], cfg.Attestors[1] = cfg.Attestors[1], cfg.Attestors[0]
		}
		_, _, err := cfg.ReconcileDeployment(DeploymentConfig{Attestors: Attestors{localAttestor("draft", "key")}})
		require.ErrorContains(t, err, "duplicate local attestor signer")
	}
}

// Existing aliases are reserved even when their connections are not in the deployment.
func TestReconcileDeploymentAllocatesUnusedAliases(t *testing.T) {
	cfg, _, err := DefaultConfig().ReconcileDeployment(deploymentConfig())
	require.NoError(t, err)
	cfg.Relayer.Connections[0].Alias = "1-2-1"
	incoming := deploymentConfig()
	first := incoming.Connections[0]
	first.ClientA.ClientID = "new-a"
	first.ClientB.ClientID = "new-b"
	second := first
	second.ClientA.ClientID = "another-a"
	second.ClientB.ClientID = "another-b"
	incoming.Connections = []ConnectionConfig{first, second, incoming.Connections[0]}
	merged, conflicts, err := cfg.ReconcileDeployment(incoming)
	require.NoError(t, err)
	require.Empty(t, conflicts)
	require.Len(t, merged.Relayer.Connections, 3)
	require.Equal(t, "1-2-1", merged.Relayer.Connections[0].Alias)
	require.Equal(t, "1-2", merged.Relayer.Connections[1].Alias)
	require.Equal(t, "1-2-2", merged.Relayer.Connections[2].Alias)
	again, conflicts, err := merged.ReconcileDeployment(incoming)
	require.NoError(t, err)
	require.Empty(t, conflicts)
	require.Equal(t, merged, again)
}
